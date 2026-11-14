package emailvalidation

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/gogf/gf/v2/frame/g"
)

type databaseCache struct{}

func (databaseCache) GetMany(ctx context.Context, keys []string) map[string]Result {
	values := map[string]Result{}
	rows, err := g.DB().Model("bm_email_verifications").Ctx(ctx).Fields("cache_key,result").WhereIn("cache_key", keys).Where("expires_at>?", time.Now()).All()
	if err != nil {
		return values
	}
	for _, row := range rows {
		var r Result
		if json.Unmarshal(row["result"].Bytes(), &r) == nil && time.Now().Before(r.ExpiresAt) {
			values[row["cache_key"].String()] = r
		}
	}
	return values
}

func (databaseCache) Get(ctx context.Context, key string) (Result, bool) {
	value, err := g.DB().Model("bm_email_verifications").Ctx(ctx).Where("cache_key", key).Where("expires_at>?", time.Now()).Value("result")
	var r Result
	if err != nil || value == nil || value.IsEmpty() || json.Unmarshal(value.Bytes(), &r) != nil {
		return r, false
	}
	return r, time.Now().Before(r.ExpiresAt)
}
func (databaseCache) Put(ctx context.Context, key string, r Result) {
	email := ""
	if strings.HasPrefix(key, "smtp:") {
		parts := strings.SplitN(key, ":", 3)
		if len(parts) == 3 {
			email = parts[2]
		}
	}
	_, err := g.DB().Model("bm_email_verifications").Ctx(ctx).Data(g.Map{"cache_key": key, "email": email, "stage": r.Stage, "status": r.Status, "code": r.Code, "result": encode(r), "checked_at": r.CheckedAt, "expires_at": r.ExpiresAt}).OnConflict("cache_key").Save()
	if err != nil {
		g.Log().Error(ctx, "Save recipient validation result", err)
	}
}

var Default = func() *Engine {
	e := New(newFallbackResolver(net.DefaultResolver))
	e.Store = databaseCache{}
	return e
}()

func Mode(ctx context.Context) string {
	return "import_only"
}
func Check(ctx context.Context, email, sender string, deep bool) Result {
	address, domain, okay := Normalize(email)
	if !okay {
		return result(Invalid, "invalid_email", "dns")
	}
	dns := Default.Check(ctx, address, sender, false)
	if dns.Status != Valid || !deep {
		return dns
	}
	// For domains hosted by this very server, the live mailbox/alias database is
	// authoritative and avoids probing our own MX or relying on its catch-all policy.
	owned, err := g.DB().Model("domain").Ctx(ctx).Where("domain", domain).Where("active", 1).Count()
	if err != nil {
		return result(Unknown, "verification_storage", "smtp")
	}
	if owned > 0 {
		exists, err := g.DB().GetValue(ctx, `SELECT (SELECT count(*) FROM mailbox WHERE username=$1 AND active=1)+(SELECT count(*) FROM alias WHERE address=$1 AND active=1 AND goto<>'')`, address)
		if err != nil {
			return result(Unknown, "verification_storage", "smtp")
		}
		r := result(Invalid, "recipient_not_found", "smtp")
		if exists.Int() > 0 {
			r = result(Valid, "local_recipient", "smtp")
		} else if wildcard, e := g.DB().Model("alias").Ctx(ctx).Where("address", "@"+domain).Where("active", 1).Where("goto<>''").Count(); e != nil {
			r = result(Unknown, "verification_storage", "smtp")
		} else if wildcard > 0 {
			r = result(Unknown, "catch_all", "smtp")
		}
		_, senderDomain, _ := Normalize(sender)
		databaseCache{}.Put(ctx, "smtp:"+senderDomain+":"+address, r)
		return r
	}
	return Default.Check(ctx, address, sender, true)
}

type Rejection struct {
	Email  string
	Result Result
}

func (r *Rejection) Error() string {
	return fmt.Sprintf("Recipient validation %s: %s (%s)", r.Result.Status, r.Email, r.Result.Code)
}
func Require(ctx context.Context, sender string, recipients []string) error {
	for _, email := range recipients {
		r := Check(ctx, email, sender, true)
		if r.Status != Valid {
			return &Rejection{email, r}
		}
	}
	return nil
}
func RequireDomains(ctx context.Context, emails []string) error {
	domains := []string{}
	seen := map[string]bool{}
	for _, email := range emails {
		_, domain, okay := Normalize(email)
		if !okay {
			return &Rejection{email, result(Invalid, "invalid_email", "dns")}
		}
		if !seen[domain] {
			seen[domain] = true
			domains = append(domains, domain)
		}
	}
	checked := Default.Domains(ctx, domains)
	for _, email := range emails {
		_, domain, _ := Normalize(email)
		r := checked[domain]
		if r.Status != Valid {
			return &Rejection{email, r}
		}
	}
	return nil
}
func InvalidateRecipient(ctx context.Context, email string) {
	InvalidateRecipients(ctx, []string{email})
}
func InvalidateRecipients(ctx context.Context, emails []string) {
	addresses, domains := map[string]bool{}, map[string]bool{}
	for _, email := range emails {
		normalized, domain, okay := Normalize(email)
		if okay {
			addresses[normalized] = true
			domains[domain] = true
		}
	}
	if len(addresses) == 0 {
		return
	}
	Default.mu.Lock()
	for key := range Default.cached {
		parts := strings.SplitN(key, ":", 3)
		if len(parts) == 2 && parts[0] == "dns" && domains[parts[1]] || len(parts) == 3 && ((parts[0] == "smtp" && addresses[parts[2]]) || (parts[0] == "catchall" && domains[parts[2]])) {
			delete(Default.cached, key)
		}
	}
	Default.mu.Unlock()
	a, d := []string{}, []string{}
	for value := range addresses {
		a = append(a, value)
	}
	for value := range domains {
		d = append(d, value)
	}
	ab, _ := json.Marshal(a)
	db, _ := json.Marshal(d)
	_, err := g.DB().Exec(ctx, `DELETE FROM bm_email_verifications WHERE email IN(SELECT value FROM jsonb_array_elements_text($1::jsonb)) OR cache_key IN(SELECT 'dns:'||value FROM jsonb_array_elements_text($2::jsonb)) OR(cache_key LIKE 'catchall:%' AND split_part(cache_key,':',3) IN(SELECT value FROM jsonb_array_elements_text($2::jsonb)))`, string(ab), string(db))
	if err != nil {
		g.Log().Error(ctx, err)
	}
}
