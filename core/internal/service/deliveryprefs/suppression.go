package deliveryprefs

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/gogf/gf/v2/frame/g"
	"net/mail"
	"strings"
	"time"
)

func Domain(address string) string {
	parts := strings.Split(strings.ToLower(strings.TrimSpace(address)), "@")
	if len(parts) == 2 {
		return parts[1]
	}
	return strings.ToLower(strings.TrimSpace(address))
}
func DefaultDomain(ctx context.Context) string {
	value, e := g.DB().Model("bm_options").Ctx(ctx).Where("name", "outbound_identity").Value("value")
	if e == nil && !value.IsEmpty() {
		var identity struct {
			From string `json:"from"`
		}
		if json.Unmarshal(value.Bytes(), &identity) == nil && strings.Contains(identity.From, "@") {
			return Domain(identity.From)
		}
	}
	value, e = g.DB().Model("bm_options").Ctx(ctx).Where("name", "default_sender_domain").Value("value")
	if e == nil && !value.IsEmpty() {
		return value.String()
	}
	value, e = g.DB().Model("mailbox").Ctx(ctx).Where("active", 1).Order("username").Value("username")
	if e == nil && !value.IsEmpty() {
		return Domain(value.String())
	}
	return ""
}
func IsSuppressed(ctx context.Context, sender, recipient string) (bool, error) {
	n, e := g.DB().Model("bm_delivery_suppressions").Ctx(ctx).Where("email", strings.ToLower(strings.TrimSpace(recipient))).Where("active", true).Where("(domain=? OR domain='')", Domain(sender)).Count()
	return n > 0, e
}
func Record(ctx context.Context, recipient, domain string, group int, source string) error {
	recipient = strings.ToLower(strings.TrimSpace(recipient))
	parsed, e := mail.ParseAddress(recipient)
	if e != nil || parsed.Address != recipient {
		return errors.New("invalid unsubscribe recipient")
	}
	if domain == "" {
		domain = DefaultDomain(ctx)
	}
	_, e = g.DB().Exec(ctx, `INSERT INTO bm_delivery_suppressions(email,domain,active,group_id,source) VALUES($1,$2,true,$3,$4) ON CONFLICT(email,domain) DO UPDATE SET active=true,group_id=excluded.group_id,source=excluded.source,updated_at=now(),resubscribed_at=NULL`, recipient, Domain(domain), group, source)
	return e
}
func Resubscribe(ctx context.Context, recipient string) error {
	domain := DefaultDomain(ctx)
	_, e := g.DB().Model("bm_delivery_suppressions").Ctx(ctx).Where("email", strings.ToLower(strings.TrimSpace(recipient))).Where("(domain=? OR domain='')", domain).Data(g.Map{"active": false, "updated_at": time.Now(), "resubscribed_at": time.Now()}).Update()
	return e
}
