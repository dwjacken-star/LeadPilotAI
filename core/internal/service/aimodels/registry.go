// Package aimodels is the only runtime source of AI provider credentials and model choices.
package aimodels

import (
	"billionmail-core/internal/service/aicredentials"
	"billionmail-core/internal/service/governance"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gogf/gf/v2/frame/g"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const optionName = "ai_model_registry"

type Model struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Enabled      bool     `json:"enabled"`
	Capabilities []string `json:"capabilities"`
	MaxOutput    int      `json:"max_output"`
}
type Provider struct {
	ID      string  `json:"id"`
	Name    string  `json:"name"`
	BaseURL string  `json:"base_url"`
	APIKey  string  `json:"api_key,omitempty"`
	HasKey  bool    `json:"has_key,omitempty"`
	Enabled bool    `json:"enabled"`
	Format  string  `json:"format"`
	Models  []Model `json:"models"`
}
type Registry struct {
	Version         int64             `json:"version"`
	Providers       []Provider        `json:"providers"`
	Defaults        map[string]string `json:"defaults"`
	TemplateAliases map[string]string `json:"template_aliases,omitempty"`
	QwenDefaults    bool              `json:"qwen_defaults,omitempty"`
}
type Option struct {
	Value        string   `json:"value"`
	Label        string   `json:"label"`
	ProviderID   string   `json:"provider_id"`
	ProviderName string   `json:"provider_name"`
	ModelID      string   `json:"model_id"`
	ModelName    string   `json:"model_name"`
	Disabled     bool     `json:"disabled"`
	Capabilities []string `json:"capabilities"`
}
type Resolved struct {
	Provider  Provider
	Model     Model
	Reference string
}

func Reference(provider, model string) string { return "ai::" + provider + "::" + model }
func Read(ctx context.Context) (Registry, bool, error) {
	r := Registry{Providers: []Provider{}, Defaults: map[string]string{}, TemplateAliases: map[string]string{}}
	v, e := g.DB().Model("bm_options").Ctx(ctx).Where("name", optionName).Value("value")
	if e != nil {
		return r, false, e
	}
	if v.IsEmpty() {
		return r, false, nil
	}
	e = json.Unmarshal(v.Bytes(), &r)
	if e == nil {
		initializeDefaults(&r)
	}
	return r, true, e
}
func Active(ctx context.Context) bool { _, ok, e := Read(ctx); return ok && e == nil }
func HasCapability(m Model, kind string) bool {
	for _, c := range m.Capabilities {
		if c == kind || (kind == "text" && (c == "vision" || c == "chat")) {
			return true
		}
	}
	return false
}

func preferredModel(r Registry, kind string, qwenOnly bool) string {
	for _, p := range r.Providers {
		if !p.Enabled || p.APIKey == "" || p.BaseURL == "" {
			continue
		}
		for _, m := range p.Models {
			if !m.Enabled || !HasCapability(m, kind) {
				continue
			}
			id := strings.ToLower(m.ID)
			if !qwenOnly || strings.HasPrefix(id, "qwen") || strings.HasPrefix(id, "qvq") {
				return Reference(p.ID, m.ID)
			}
		}
	}
	return ""
}

func defaultModel(r Registry, kind string) string {
	if selected := r.Defaults[kind]; selected != "" {
		if resolved, e := resolve(r, selected, kind); e == nil {
			return resolved.Reference
		}
	}
	if selected := preferredModel(r, kind, true); selected != "" {
		return selected
	}
	return preferredModel(r, kind, false)
}

func initializeDefaults(r *Registry) {
	if r.Defaults == nil {
		r.Defaults = map[string]string{}
	}
	for _, kind := range []string{"text", "chat", "vision", "image", "embedding"} {
		// Upgrade existing defaults once; later administrator choices remain authoritative.
		if !r.QwenDefaults {
			if selected := preferredModel(*r, kind, true); selected != "" {
				r.Defaults[kind] = selected
			}
		}
		r.Defaults[kind] = defaultModel(*r, kind)
	}
	r.QwenDefaults = true
}

func resolve(r Registry, selected, kind string) (Resolved, error) {
	if selected == "" {
		selected = defaultModel(r, kind)
	}
	var pid, mid string
	if strings.HasPrefix(selected, "ai::") {
		parts := strings.SplitN(selected, "::", 3)
		if len(parts) != 3 {
			return Resolved{}, errors.New("ai_registry_model_invalid")
		}
		pid, mid = parts[1], parts[2]
		if alias := r.TemplateAliases[pid]; alias != "" {
			pid = alias
		}
	} else if parts := strings.SplitN(selected, "::", 2); len(parts) == 2 {
		pid = "text_" + parts[0]
		mid = parts[1]
	} else {
		pid = "growth_default"
		mid = selected
	}
	for _, p := range r.Providers {
		if p.ID != pid {
			continue
		}
		for _, m := range p.Models {
			if m.ID != mid {
				continue
			}
			if !p.Enabled || p.APIKey == "" || p.BaseURL == "" || !m.Enabled || !HasCapability(m, kind) {
				return Resolved{}, errors.New("ai_registry_model_disabled_or_incompatible")
			}
			return Resolved{p, m, Reference(p.ID, m.ID)}, nil
		}
	}
	return Resolved{}, errors.New("ai_registry_model_unavailable")
}
func Resolve(ctx context.Context, selected, kind string) (Resolved, error) {
	r, ok, e := Read(ctx)
	if e != nil {
		return Resolved{}, e
	}
	if !ok {
		return Resolved{}, errors.New("ai_registry_not_initialized")
	}
	return resolve(r, selected, kind)
}
func TemplateProvider(ctx context.Context, id string) (Provider, error) {
	r, ok, e := Read(ctx)
	if e != nil || !ok {
		return Provider{}, errors.New("ai_registry_not_initialized")
	}
	if alias := r.TemplateAliases[id]; alias != "" {
		id = alias
	}
	for _, p := range r.Providers {
		if p.ID == id {
			return p, nil
		}
	}
	return Provider{}, errors.New("ai_registry_provider_unavailable")
}
func Catalog(ctx context.Context, kind string) (g.Map, error) {
	if kind == "" {
		kind = "text"
	}
	if !strings.Contains("|text|chat|vision|image|embedding|", "|"+kind+"|") {
		return nil, errors.New("ai_registry_capability_invalid")
	}
	r, ok, e := Read(ctx)
	if e != nil {
		return nil, e
	}
	options := []Option{}
	all := []Option{}
	aliases := map[string]string{}
	for _, p := range r.Providers {
		for _, m := range p.Models {
			if !HasCapability(m, kind) {
				continue
			}
			option := Option{Value: Reference(p.ID, m.ID), Label: p.Name + " · " + m.Name, ProviderID: p.ID, ProviderName: p.Name, ModelID: m.ID, ModelName: m.Name, Disabled: !p.Enabled || p.APIKey == "" || p.BaseURL == "" || !m.Enabled, Capabilities: m.Capabilities}
			all = append(all, option)
			if !option.Disabled {
				options = append(options, option)
			}
			for alias, target := range r.TemplateAliases {
				if target == p.ID {
					aliases[Reference(alias, m.ID)] = option.Value
				}
			}
			if p.ID == "growth_default" {
				aliases[m.ID] = option.Value
			}
			if p.ID == "text_openai" {
				aliases["openai::"+m.ID] = option.Value
			}
			if p.ID == "text_qwen" {
				aliases["qwen::"+m.ID] = option.Value
			}
		}
	}
	return g.Map{"initialized": ok, "models": options, "all_models": all, "legacy_aliases": aliases, "model": defaultModel(r, kind), "enabled": len(options) > 0, "warning": ""}, nil
}
func Settings(ctx context.Context, p *governance.Principal) (Registry, error) {
	if p == nil || !p.Super {
		return Registry{}, errors.New("administrator required")
	}
	r, _, e := Read(ctx)
	for i := range r.Providers {
		r.Providers[i].HasKey = r.Providers[i].APIKey != ""
		r.Providers[i].APIKey = ""
	}
	return r, e
}

var providerID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,80}$`)

func ValidateURL(base string) error {
	u, e := url.Parse(base)
	if e != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("HTTPS provider base URL required")
	}
	return nil
}
func Save(ctx context.Context, p *governance.Principal, r Registry) error {
	if p == nil || !p.Super {
		return errors.New("administrator required")
	}
	old, ok, e := Read(ctx)
	if e != nil {
		return e
	}
	if ok && old.Version != r.Version {
		return errors.New("ai_registry_changed_reload")
	}
	if len(r.Providers) > 40 {
		return errors.New("ai_registry_provider_limit")
	}
	seen := map[string]bool{}
	for i := range r.Providers {
		x := &r.Providers[i]
		x.ID = strings.TrimSpace(x.ID)
		x.Name = strings.TrimSpace(x.Name)
		x.BaseURL = strings.TrimRight(strings.TrimSpace(x.BaseURL), "/")
		if !providerID.MatchString(x.ID) || seen[x.ID] || x.Name == "" {
			return errors.New("ai_registry_provider_invalid")
		}
		seen[x.ID] = true
		if x.Format != "responses" && x.Format != "chat_completions" {
			return errors.New("ai_registry_format_invalid")
		}
		if x.BaseURL != "" {
			if e = ValidateURL(x.BaseURL); e != nil {
				return e
			}
		}
		var previous Provider
		for _, v := range old.Providers {
			if v.ID == x.ID {
				previous = v
				break
			}
		}
		if x.APIKey == "" {
			if previous.BaseURL != x.BaseURL && previous.APIKey != "" {
				return errors.New("ai_registry_new_endpoint_key_required")
			}
			x.APIKey = previous.APIKey
		} else {
			x.APIKey, e = aicredentials.Protect(strings.TrimSpace(x.APIKey), false)
			if e != nil {
				return e
			}
		}
		if x.Enabled && (x.APIKey == "" || x.BaseURL == "") {
			return errors.New("ai_registry_provider_credentials_required")
		}
		x.HasKey = false
		if len(x.Models) > 200 {
			return errors.New("ai_registry_model_limit")
		}
		models := map[string]bool{}
		for j := range x.Models {
			m := &x.Models[j]
			if m.ID == "" || len(m.ID) > 250 || strings.ContainsAny(m.ID, "\r\n\x00") || models[m.ID] {
				return errors.New("ai_registry_model_invalid")
			}
			models[m.ID] = true
			if m.Name == "" {
				m.Name = m.ID
			}
			if m.MaxOutput < 256 {
				m.MaxOutput = 4000
			}
			if m.MaxOutput > 131072 {
				return errors.New("ai_registry_output_limit_invalid")
			}
			for _, cap := range m.Capabilities {
				if !strings.Contains("|text|chat|vision|image|embedding|", "|"+cap+"|") {
					return errors.New("ai_registry_capability_invalid")
				}
			}
		}
	}
	r.TemplateAliases = old.TemplateAliases
	if r.Defaults == nil {
		r.Defaults = map[string]string{}
	}
	for kind, value := range r.Defaults {
		if value != "" {
			if _, e = resolve(r, value, kind); e != nil {
				return e
			}
		}
	}
	r.QwenDefaults = true
	initializeDefaults(&r)
	r.Version = old.Version + 1
	raw, e := json.Marshal(r)
	if e != nil {
		return e
	}
	if ok {
		result, err := g.DB().Exec(ctx, `UPDATE bm_options SET value=$1 WHERE name=$2 AND (value::jsonb->>'version')::bigint=$3`, string(raw), optionName, old.Version)
		if err != nil {
			return err
		}
		n, _ := result.RowsAffected()
		if n != 1 {
			return errors.New("ai_registry_changed_reload")
		}
	} else {
		_, e = g.DB().Exec(ctx, `INSERT INTO bm_options(name,value) VALUES($1,$2)`, optionName, string(raw))
		if e != nil {
			return e
		}
	}
	return governance.Audit(ctx, p, "ai_registry_save", 0, g.Map{"providers": len(r.Providers), "version": r.Version})
}
func Discover(ctx context.Context, p *governance.Principal, provider Provider) ([]Model, error) {
	if p == nil || !p.Super {
		return nil, errors.New("administrator required")
	}
	if e := ValidateURL(provider.BaseURL); e != nil {
		return nil, e
	}
	if provider.APIKey == "" {
		r, _, e := Read(ctx)
		if e != nil {
			return nil, e
		}
		for _, old := range r.Providers {
			if old.ID == provider.ID && old.BaseURL == strings.TrimRight(provider.BaseURL, "/") {
				provider.APIKey, e = aicredentials.Protect(old.APIKey, true)
				if e != nil {
					return nil, e
				}
			}
		}
		if provider.APIKey == "" {
			return nil, errors.New("API key required")
		}
	}
	req, e := http.NewRequestWithContext(ctx, "GET", strings.TrimRight(provider.BaseURL, "/")+"/models", nil)
	if e != nil {
		return nil, e
	}
	req.Header.Set("Authorization", "Bearer "+provider.APIKey)
	client := http.Client{Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, e := client.Do(req)
	if e != nil {
		return nil, errors.New("ai_registry_connection_failed")
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("AI models HTTP %d", res.StatusCode)
	}
	raw, e := io.ReadAll(io.LimitReader(res.Body, 4*1024*1024))
	if e != nil {
		return nil, e
	}
	var data struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if e = json.Unmarshal(raw, &data); e != nil {
		return nil, e
	}
	models := []Model{}
	for _, v := range data.Data {
		if len(models) >= 200 {
			break
		}
		models = append(models, Model{v.ID, v.ID, true, InferCapabilities(v.ID), 4000})
	}
	return models, nil
}
func InferCapabilities(id string) []string {
	lower := strings.ToLower(id)
	if strings.Contains(lower, "embedding") {
		return []string{"embedding"}
	}
	if strings.Contains(lower, "image") || strings.Contains(lower, "dall-e") || strings.Contains(lower, "flux") {
		return []string{"image"}
	}
	if strings.Contains(lower, "whisper") || strings.Contains(lower, "tts") || strings.Contains(lower, "audio") {
		return []string{}
	}
	caps := []string{"text", "chat"}
	if strings.Contains(lower, "gpt-4") || strings.Contains(lower, "gpt-5") || strings.Contains(lower, "gemini") || strings.Contains(lower, "vision") || strings.Contains(lower, "-vl") {
		caps = append(caps, "vision")
	}
	return caps
}
