// Package smtpidentity maintains the default identity and per-sender-domain
// direct SMTP transports without overriding relay or dedicated-IP mappings.
package smtpidentity

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"billionmail-core/internal/consts"
	docker "billionmail-core/internal/service/dockerapi"
	"billionmail-core/internal/service/public"
	"github.com/gogf/gf/v2/frame/g"
)

const transportMap = "hash:/etc/postfix/conf/maplemail_sender_domains"
const beginMarker = "# BEGIN MAPLEMAIL SENDER DOMAINS"
const endMarker = "# END MAPLEMAIL SENDER DOMAINS"

var configMu sync.Mutex
var domainLabel = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

// SenderOrder is a fixed SQL expression: authorization and search filters still
// apply before the default-domain preference. No client input enters this SQL.
const SenderOrder = "CASE WHEN domain=(SELECT value FROM bm_options WHERE name='default_sender_domain' LIMIT 1) THEN 0 ELSE 1 END, username"

func validDomain(domain string) bool {
	if len(domain) > 248 || !strings.Contains(domain, ".") {
		return false
	}
	for _, label := range strings.Split(domain, ".") {
		if !domainLabel.MatchString(label) {
			return false
		}
	}
	return true
}

func serviceName(domain string) string {
	hash := sha256.Sum256([]byte(domain))
	return fmt.Sprintf("maplemail_%x", hash[:8])
}

// replaceSetting handles continuation lines and duplicate assignments. Keeping
// existing relay maps first preserves their precedence over direct transports.
func replaceSetting(content, key, value string) string {
	lines := strings.Split(content, "\n")
	out := make([]string, 0, len(lines)+1)
	skipping := false
	for _, line := range lines {
		continuation := len(line) > 0 && (line[0] == ' ' || line[0] == '\t')
		if skipping && continuation && strings.TrimSpace(line) != "" && !strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		skipping = false
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 && strings.TrimSpace(parts[0]) == key {
			skipping = true
			continue
		}
		out = append(out, line)
	}
	return strings.TrimRight(strings.Join(out, "\n"), "\n") + "\n" + key + " = " + value + "\n"
}

func setting(content, key string) string {
	value := ""
	collect := false
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if line[0] == ' ' || line[0] == '\t' {
			if collect {
				value += " " + trimmed
			}
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		collect = len(parts) == 2 && strings.TrimSpace(parts[0]) == key
		if collect {
			value = strings.TrimSpace(parts[1])
		}
	}
	return value
}

func render(main, master, preferred string, domains []string) (string, string, string, error) {
	if !validDomain(preferred) {
		return "", "", "", fmt.Errorf("invalid default sender domain")
	}
	found := false
	multiple := len(domains) > 1
	var routes, services strings.Builder
	if multiple {
		services.WriteString(beginMarker + "\n")
	}
	for _, domain := range domains {
		if !validDomain(domain) {
			return "", "", "", fmt.Errorf("invalid sender domain: %q", domain)
		}
		if domain == preferred {
			found = true
		}
		if multiple {
			name := serviceName(domain)
			fmt.Fprintf(&routes, "@%s %s:\n", domain, name)
			fmt.Fprintf(&services, "%s unix - - n - - smtp\n  -o smtp_helo_name=mail.%s\n", name, domain)
		}
	}
	if !found {
		return "", "", "", fmt.Errorf("default sender domain is not active")
	}
	if multiple {
		services.WriteString(endMarker + "\n")
	}
	start, end := strings.Index(master, beginMarker), strings.Index(master, endMarker)
	if (start >= 0) != (end >= 0) || (start >= 0 && end < start) {
		return "", "", "", fmt.Errorf("invalid managed SMTP transport block")
	}
	if start >= 0 {
		master = master[:start] + master[end+len(endMarker):]
	}
	master = strings.TrimRight(master, "\n") + "\n" + services.String()
	maps := strings.FieldsFunc(setting(main, "sender_dependent_default_transport_maps"), func(r rune) bool { return r == ',' || r == ' ' || r == '\t' })
	kept := []string{}
	for _, value := range maps {
		if value != transportMap {
			kept = append(kept, value)
		}
	}
	if multiple {
		kept = append(kept, transportMap)
	}
	main = replaceSetting(main, "sender_dependent_default_transport_maps", strings.Join(kept, ", "))
	main = replaceSetting(main, "myorigin", preferred)
	main = replaceSetting(main, "smtp_helo_name", "mail."+preferred)
	if !multiple {
		main = replaceSetting(main, "myhostname", "mail."+preferred)
	}
	return main, master, routes.String(), nil
}

type fileState struct {
	path   string
	data   []byte
	mode   os.FileMode
	exists bool
}

func readState(path string) (fileState, error) {
	s := fileState{path: path, mode: 0644}
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	s.exists, s.mode = true, info.Mode().Perm()
	s.data, err = os.ReadFile(path)
	return s, err
}

func reload(ctx context.Context) error {
	dk, err := docker.NewDockerAPI()
	if err != nil {
		return err
	}
	defer dk.Close()
	commands := [][]string{}
	if _, err := os.Stat(public.AbsPath("../conf/postfix/conf/maplemail_sender_domains")); err == nil {
		commands = append(commands, []string{"postmap", transportMap})
	}
	commands = append(commands, []string{"postfix", "check"}, []string{"postfix", "reload"})
	for _, cmd := range commands {
		result, e := dk.ExecCommandByName(ctx, consts.SERVICES.Postfix, cmd, "root")
		if e != nil {
			return e
		}
		if result == nil || result.ExitCode != 0 {
			return fmt.Errorf("Postfix command failed: %s: %v", cmd[0], result)
		}
	}
	return nil
}

// apply restores configuration on failure, including rebuilding the old map.
func apply(ctx context.Context, dir, preferred string, domains []string, activate func(context.Context) error, publish ...func() error) error {
	states := make([]fileState, 3)
	for i, name := range []string{"main.cf", "master.cf", "conf/maplemail_sender_domains"} {
		s, err := readState(filepath.Join(dir, name))
		if err != nil {
			return err
		}
		states[i] = s
	}
	if !states[0].exists || !states[1].exists {
		return fmt.Errorf("Postfix configuration files are missing")
	}
	main, master, routes, err := render(string(states[0].data), string(states[1].data), preferred, domains)
	if err != nil {
		return err
	}
	contents := []string{main, master, routes}
	if err = os.MkdirAll(filepath.Join(dir, "conf"), 0755); err != nil {
		return err
	}
	for i, content := range contents {
		// Write in place: main.cf/master.cf may be individual Docker bind mounts.
		if err = os.WriteFile(states[i].path, []byte(content), states[i].mode); err != nil {
			break
		}
	}
	if err == nil {
		err = activate(ctx)
	}
	if err == nil && len(publish) > 0 {
		err = publish[0]()
	}
	if err == nil {
		return nil
	}
	for _, s := range states {
		var restoreErr error
		if s.exists {
			restoreErr = os.WriteFile(s.path, s.data, s.mode)
		} else {
			restoreErr = os.Remove(s.path)
			if os.IsNotExist(restoreErr) {
				restoreErr = nil
			}
		}
		if restoreErr != nil {
			err = fmt.Errorf("%w; restore %s: %v", err, s.path, restoreErr)
		}
	}
	// The previous deployment may not have had a map at all; remove its index.
	if !states[2].exists {
		_ = os.Remove(states[2].path + ".db")
	}
	rollbackCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if e := activate(rollbackCtx); e != nil {
		err = fmt.Errorf("%w; configuration reload after rollback: %v", err, e)
	}
	return err
}

func syncLocked(ctx context.Context, preferred string, publish ...func() error) error {
	values, err := g.DB().Model("domain").Ctx(ctx).Where("active", 1).Order("domain").Array("domain")
	if err != nil {
		return err
	}
	if len(values) == 0 {
		return fmt.Errorf("no active sender domains")
	}
	domains := make([]string, len(values))
	found := false
	for i, value := range values {
		domains[i] = value.String()
		if domains[i] == preferred {
			found = true
		}
	}
	if !found {
		return fmt.Errorf("default sender domain is not active")
	}
	return apply(ctx, public.AbsPath("../conf/postfix"), preferred, domains, reload, publish...)
}

func Sync(ctx context.Context) error {
	configMu.Lock()
	defer configMu.Unlock()
	preferred, err := g.DB().Model("bm_options").Ctx(ctx).Where("name", "default_sender_domain").Value("value")
	if err != nil {
		return err
	}
	// A fresh install has no chosen default yet; do not invent one silently.
	if preferred.String() == "" {
		return nil
	}
	return syncLocked(ctx, preferred.String())
}

// SetDefault publishes the option only after the Postfix configuration passes
// validation. A failed database write restores the previous SMTP identity.
func SetDefault(ctx context.Context, domain string) error {
	configMu.Lock()
	defer configMu.Unlock()
	if !validDomain(domain) {
		return fmt.Errorf("invalid sender domain")
	}
	return syncLocked(ctx, domain, func() error {
		_, err := g.DB().Model("bm_options").Ctx(ctx).Data(g.Map{"name": "default_sender_domain", "value": domain}).OnConflict("name").Save()
		return err
	})
}
