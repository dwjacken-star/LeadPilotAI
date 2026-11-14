package growth

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"billionmail-core/internal/service/governance"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/gconv"
)

type MasterWATemplate struct {
	Name           string   `json:"name"`
	Language       string   `json:"language"`
	MarketLanguage string   `json:"market_language"`
	Body           string   `json:"body"`
	Variables      []string `json:"variables"`
}
type MasterWAConfig struct {
	Enabled       bool               `json:"enabled"`
	PhoneNumberID string             `json:"phone_number_id"`
	Version       string             `json:"api_version"`
	Token         string             `json:"token,omitempty"`
	AppSecret     string             `json:"app_secret,omitempty"`
	VerifyToken   string             `json:"verify_token,omitempty"`
	Templates     []MasterWATemplate `json:"templates"`
}

var masterGraphVersion = regexp.MustCompile(`^v[0-9]{1,3}\.0$`)
var masterDigits = regexp.MustCompile(`^[0-9]{5,30}$`)
var masterPhoneChars = regexp.MustCompile(`[^0-9]`)

func masterPhone(value string) string {
	v := masterPhoneChars.ReplaceAllString(value, "")
	if len(v) < 8 || len(v) > 15 {
		return ""
	}
	return v
}

var masterWAHTTP = &http.Client{Timeout: 40 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

func masterWA(ctx context.Context, cid int64) (MasterWAConfig, error) {
	var c MasterWAConfig
	v, e := g.DB().Model("bm_gm_wa_accounts").Ctx(ctx).Where("company_id", cid).Value("config")
	if e == nil && !v.IsEmpty() {
		e = json.Unmarshal(v.Bytes(), &c)
	}
	return c, e
}
func masterWASettings(ctx context.Context, p *governance.Principal, cid int64) (g.Map, error) {
	if _, e := company(ctx, p, cid, "intelligence:settings", false); e != nil {
		return nil, e
	}
	c, e := masterWA(ctx, cid)
	if e != nil {
		return nil, e
	}
	hasToken, hasSecret, hasVerify := c.Token != "", c.AppSecret != "", c.VerifyToken != ""
	c.Token, c.AppSecret, c.VerifyToken = "", "", ""
	return g.Map{"config": c, "has_token": hasToken, "has_secret": hasSecret, "has_verify_token": hasVerify, "webhook_path": fmt.Sprintf("/growth/webhooks/whatsapp/%d", cid)}, nil
}
func masterSaveWA(ctx context.Context, p *governance.Principal, cid int64, data g.Map) error {
	if _, e := company(ctx, p, cid, "intelligence:settings", false); e != nil {
		return e
	}
	var c MasterWAConfig
	if e := gconv.Scan(data, &c); e != nil {
		return e
	}
	if c.Enabled && (!masterDigits.MatchString(c.PhoneNumberID) || !masterGraphVersion.MatchString(c.Version) || len(c.Templates) == 0) {
		return errors.New("WhatsApp phone number ID, API version and approved templates required")
	}
	if len(c.Templates) > 50 {
		return errors.New("at most 50 WhatsApp template mappings")
	}
	for _, t := range c.Templates {
		if !masterKey.MatchString(t.Name) || t.Language == "" || t.MarketLanguage == "" || t.Body == "" || len(t.Body) > 10000 || len(t.Variables) > 10 {
			return errors.New("invalid WhatsApp template mapping")
		}
		for i, v := range t.Variables {
			if v != "subject" && v != "body" {
				return errors.New("template variables may map to subject or body")
			}
			if !strings.Contains(t.Body, fmt.Sprintf("{{%d}}", i+1)) {
				return errors.New("template body must include every configured variable placeholder")
			}
		}
	}
	old, e := masterWA(ctx, cid)
	if e != nil {
		return e
	}
	for _, pair := range []struct {
		V   *string
		Old string
	}{{&c.Token, old.Token}, {&c.AppSecret, old.AppSecret}, {&c.VerifyToken, old.VerifyToken}} {
		if *pair.V == "" {
			*pair.V = pair.Old
		} else {
			if len(*pair.V) > 5000 || strings.ContainsAny(*pair.V, "\r\n") {
				return errors.New("invalid channel credential")
			}
			*pair.V, e = protect(*pair.V, false)
			if e != nil {
				return e
			}
		}
	}
	if c.Enabled && (c.Token == "" || c.AppSecret == "" || c.VerifyToken == "") {
		return errors.New("WhatsApp access token, app secret and verification token required")
	}
	_, e = g.DB().Model("bm_gm_wa_accounts").Ctx(ctx).Data(g.Map{"company_id": cid, "dept_id": p.DeptID, "owner_id": p.ID, "config": jsonText(c), "updated_at": time.Now()}).OnConflict("company_id").Save()
	return e
}
func masterWATemplate(ctx context.Context, cid int64, language, subject, body string) (MasterWATemplate, []string, string, error) {
	c, e := masterWA(ctx, cid)
	if e != nil {
		return MasterWATemplate{}, nil, "", e
	}
	if !c.Enabled {
		return MasterWATemplate{}, nil, "", errors.New("WhatsApp configuration required")
	}
	for _, t := range c.Templates {
		if t.MarketLanguage != language {
			continue
		}
		rendered := t.Body
		params := []string{}
		for i, key := range t.Variables {
			value := body
			if key == "subject" {
				value = subject
			}
			value = strings.Join(strings.Fields(value), " ")
			if len([]rune(value)) > 1024 {
				return t, nil, "", errors.New("WhatsApp template variable exceeds 1024 characters")
			}
			params = append(params, value)
			rendered = strings.ReplaceAll(rendered, fmt.Sprintf("{{%d}}", i+1), value)
		}
		if strings.Contains(rendered, "{{") {
			return t, nil, "", errors.New("unmapped WhatsApp template variable")
		}
		return t, params, rendered, nil
	}
	return MasterWATemplate{}, nil, "", errors.New("approved WhatsApp template mapping required for selected language")
}
func (x *masterExec) whatsappHandoff(content gdb.Record) error {
	if e := x.check(); e != nil {
		return e
	}
	cfg, e := masterWA(x.Ctx, x.Run["company_id"].Int64())
	if e != nil {
		return e
	}
	if !cfg.Enabled {
		return errors.New("WhatsApp configuration required")
	}
	pr, e := masterRecord(x.Ctx, x.P, "bm_gm_prospects", content["prospect_id"].Int64(), "intelligence:publish")
	if e != nil {
		return e
	}
	if !pr["wa_opt_in"].Bool() || pr["contact_permission"].String() != "allowed" || pr["suppressed"].Bool() || masterPhone(pr["phone"].String()) != content["target"].String() {
		return errors.New("WhatsApp permission revoked")
	}
	snapshot := jsonMap(content["snapshot"])
	var t MasterWATemplate
	if e = gconv.Scan(snapshot["wa_template"], &t); e != nil {
		return e
	}
	found := false
	for _, current := range cfg.Templates {
		if masterHash(current) == masterHash(t) {
			found = true
		}
	}
	if !found || gconv.String(snapshot["wa_phone_id"]) != cfg.PhoneNumberID {
		return errors.New("WhatsApp template or sending account changed; regenerate content")
	}
	params := gconv.Strings(snapshot["wa_parameters"])
	components := []g.Map{}
	if len(params) > 0 {
		p := []g.Map{}
		for _, value := range params {
			p = append(p, g.Map{"type": "text", "text": value})
		}
		components = append(components, g.Map{"type": "body", "parameters": p})
	}
	payload := g.Map{"messaging_product": "whatsapp", "to": content["target"].String(), "type": "template", "template": g.Map{"name": t.Name, "language": g.Map{"code": t.Language}, "components": components}}
	var usage int64
	e = g.DB().Transaction(x.Ctx, func(ctx context.Context, tx gdb.TX) error {
		current, e := tx.GetOne(`SELECT status FROM bm_gm_contents WHERE id=$1 FOR UPDATE`, content["id"])
		if e != nil {
			return e
		}
		if current["status"].String() != "approved" {
			return &masterWait{State: "waiting_external", Reason: "WhatsApp result requires reconciliation", Until: time.Now().Add(time.Minute)}
		}
		usage, e = x.reserve("send", 0)
		if e != nil {
			return e
		}
		_, e = tx.Model("bm_gm_contents").Where("id", content["id"]).Data(g.Map{"status": "sending", "sending_at": time.Now()}).Update()
		return e
	})
	if e != nil {
		return e
	}
	key, e := protect(cfg.Token, true)
	if e != nil {
		return e
	}
	raw, _ := json.Marshal(payload)
	req, e := http.NewRequestWithContext(x.Ctx, "POST", "https://graph.facebook.com/"+cfg.Version+"/"+cfg.PhoneNumberID+"/messages", bytes.NewReader(raw))
	if e != nil {
		return e
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)
	response, e := masterWAHTTP.Do(req)
	state, reason, external := "unknown", "WhatsApp result unknown; verify before any resend", ""
	if e == nil {
		defer response.Body.Close()
		body, readErr := io.ReadAll(io.LimitReader(response.Body, 1024*1024))
		if readErr == nil && response.StatusCode >= 200 && response.StatusCode < 300 {
			var r struct {
				Messages []struct {
					ID string `json:"id"`
				} `json:"messages"`
			}
			if json.Unmarshal(body, &r) == nil && len(r.Messages) > 0 {
				state, reason, external = "accepted", "", r.Messages[0].ID
			}
		} else if response.StatusCode >= 400 && response.StatusCode < 500 {
			state = "failed"
			reason = fmt.Sprintf("WhatsApp rejected template message (HTTP %d); check approved template and account permission", response.StatusCode)
		}
	}
	_, err := g.DB().Model("bm_gm_contents").Ctx(context.WithoutCancel(x.Ctx)).Where("id", content["id"]).Where("status", "sending").Data(g.Map{"status": state, "error": reason, "external_id": external}).Update()
	_, _ = g.DB().Model("bm_gm_usage").Ctx(context.WithoutCancel(x.Ctx)).Where("id", usage).Data("status", state).Update()
	return err
}
func MasterWAWebhook(ctx context.Context, companyID int64, mode, verify, challenge, signature string, body []byte) (string, int) {
	c, e := masterWA(ctx, companyID)
	if e != nil || !c.Enabled {
		return "channel not configured", 404
	}
	if mode != "" {
		token, e := protect(c.VerifyToken, true)
		if e != nil || mode != "subscribe" || subtle.ConstantTimeCompare([]byte(verify), []byte(token)) != 1 {
			return "invalid verification", 403
		}
		return challenge, 200
	}
	secret, e := protect(c.AppSecret, true)
	if e != nil {
		return "channel configuration error", 503
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if subtle.ConstantTimeCompare([]byte(signature), []byte(expected)) != 1 {
		return "invalid signature", 403
	}
	var payload struct {
		Object string `json:"object"`
		Entry  []struct {
			Changes []struct {
				Value struct {
					Metadata struct {
						PhoneID string `json:"phone_number_id"`
					} `json:"metadata"`
					Messages []struct {
						ID   string `json:"id"`
						From string `json:"from"`
						Type string `json:"type"`
						Text struct {
							Body string `json:"body"`
						} `json:"text"`
					} `json:"messages"`
					Statuses []struct {
						ID     string `json:"id"`
						Status string `json:"status"`
					} `json:"statuses"`
				} `json:"value"`
			} `json:"changes"`
		} `json:"entry"`
	}
	if json.Unmarshal(body, &payload) != nil || payload.Object != "whatsapp_business_account" {
		return "invalid payload", 400
	}
	account, e := g.DB().Model("bm_gm_wa_accounts").Ctx(ctx).Where("company_id", companyID).One()
	if e != nil {
		return "storage unavailable", 503
	}
	for _, entry := range payload.Entry {
		for _, change := range entry.Changes {
			v := change.Value
			if v.Metadata.PhoneID != c.PhoneNumberID {
				continue
			}
			for _, message := range v.Messages {
				if message.ID == "" {
					continue
				}
				text := message.Text.Body
				if message.Type != "text" {
					text = "[" + message.Type + "]"
				}
				kind := hardReplyKind(g.Map{}, message.From, "", text)
				if kind == "" {
					kind = "review"
				}
				e = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
					// Process opt-out immediately; the AI workflow may be paused or unavailable.
					if kind == "unsubscribe" {
						if _, e := tx.Model("bm_gm_prospects").Where("company_id", companyID).Where("regexp_replace(phone,'[^0-9]','','g')=?", message.From).Data(g.Map{"wa_opt_in": false, "suppressed": true, "contact_permission": "blocked"}).Update(); e != nil {
							return e
						}
					}
					_, e := tx.Model("bm_gm_events").Data(g.Map{"company_id": companyID, "dept_id": account["dept_id"], "owner_id": account["owner_id"], "channel": "whatsapp", "event_key": "whatsapp:" + c.PhoneNumberID + ":" + message.ID, "external_id": message.ID, "sender": message.From, "body": cleanText(text, 12000), "kind": kind, "expires_at": time.Now().AddDate(0, 3, 0)}).OnConflict("event_key").InsertIgnore()
					return e
				})
				if e != nil {
					return "storage unavailable", 503
				}
			}
			for _, status := range v.Statuses {
				if !strings.Contains("|sent|delivered|read|failed|", "|"+status.Status+"|") {
					continue
				}
				allowed := []string{"accepted", "queued", "unknown", "sending", "failed"}
				if status.Status == "delivered" {
					allowed = append(allowed, "sent")
				}
				if status.Status == "read" {
					allowed = append(allowed, "sent", "delivered")
				}
				_, e = g.DB().Model("bm_gm_contents").Ctx(ctx).Where("company_id", companyID).Where("channel", "whatsapp").Where("external_id", status.ID).WhereIn("status", allowed).Data("status", status.Status).Update()
				if e != nil {
					return "storage unavailable", 503
				}
			}
		}
	}
	return "EVENT_RECEIVED", 200
}
