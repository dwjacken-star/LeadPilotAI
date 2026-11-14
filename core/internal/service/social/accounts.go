package social

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"billionmail-core/internal/service/governance"
	"billionmail-core/internal/service/growth"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/gconv"
	"github.com/google/uuid"
)

func hashString(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }
func BeginAuthorization(ctx context.Context, p *governance.Principal, platform string, options ...g.Map) (g.Map, error) {
	if e := require(p, "social:account:manage"); e != nil {
		return nil, e
	}
	if e := p.CheckOwnership(ctx, "social:account:manage", p.DeptID, p.ID); e != nil {
		return nil, e
	}
	c, e := requirePlatform(ctx, platform, false)
	if e != nil {
		return nil, e
	}
	if !validRedirect(c.RedirectURL) {
		return nil, errors.New("configure HTTPS OAuth callback")
	}
	targetType, accountID := "page", int64(0)
	if len(options) > 0 {
		if value := gconv.String(options[0]["target_type"]); value != "" {
			targetType = value
		}
		accountID = gconv.Int64(options[0]["account_id"])
	}
	if platform == "instagram" && targetType == "page" {
		targetType = "professional"
	}
	if platform == "threads" && targetType == "page" {
		targetType = "profile"
	}
	validTarget := (platform == "facebook" && targetType == "page") || (platform == "linkedin" && (targetType == "page" || targetType == "person")) || (platform == "instagram" && targetType == "professional") || (platform == "threads" && targetType == "profile")
	if !validTarget || accountID < 0 {
		return nil, errors.New("invalid OAuth target; Facebook personal publishing is manual")
	}
	if accountID > 0 {
		account, err := scoped(ctx, p, "bm_social_accounts", accountID, "social:account:manage")
		if err != nil {
			return nil, err
		}
		if account["platform"].String() != platform || account["target_type"].String() != targetType || account["publish_mode"].String() != "automatic" || account["status"].String() == "deleted" {
			return nil, errors.New("reconnect target does not match")
		}
	}
	scopes := c.Scopes
	if platform == "linkedin" {
		if targetType == "person" {
			scopes = []string{"openid", "profile", "w_member_social"}
		} else {
			scopes = nil
			for _, scope := range c.Scopes {
				if strings.Contains(scope, "organization") {
					scopes = append(scopes, scope)
				}
			}
			if !has(scopes, "w_organization_social") {
				return nil, errors.New("configure LinkedIn company publishing scopes")
			}
		}
	}
	b := make([]byte, 32)
	if _, e = rand.Read(b); e != nil {
		return nil, e
	}
	state := base64.RawURLEncoding.EncodeToString(b)
	var expected interface{}
	if accountID > 0 {
		expected = accountID
	}
	_, e = g.DB().Model("bm_social_oauth_states").Ctx(ctx).Data(g.Map{"state_hash": hashString(state), "platform": platform, "target_type": targetType, "expected_account_id": expected, "owner_id": p.ID, "dept_id": p.DeptID, "expires_at": time.Now().Add(10 * time.Minute), "app_fingerprint": authorizationFingerprint(c, targetType)}).Insert()
	if e != nil {
		return nil, e
	}
	id, e := g.DB().Model("bm_social_oauth_states").Ctx(ctx).Where("state_hash", hashString(state)).Value("id")
	if e != nil {
		return nil, e
	}
	params := url.Values{"client_id": {c.ClientID}, "redirect_uri": {c.RedirectURL}, "state": {state}, "response_type": {"code"}, "scope": {strings.Join(scopes, " ")}}
	base := "https://www.linkedin.com/oauth/v2/authorization"
	if platform == "facebook" {
		base = "https://www.facebook.com/" + c.Version + "/dialog/oauth"
		params.Set("scope", strings.Join(scopes, ","))
	}
	if platform == "instagram" {
		base = "https://www.facebook.com/dialog/oauth"
		params.Del("scope")
		params.Set("config_id", c.ConfigID)
		params.Set("prompt", "login")
	}
	if platform == "threads" {
		base = "https://threads.com/oauth/authorize"
	}
	return g.Map{"id": id.Int64(), "url": base + "?" + params.Encode(), "expires_in": 600}, nil
}
func authorizationFingerprint(c PlatformConfig, targetType string) string {
	base := g.Map{"id": c.ClientID, "secret": c.ClientSecret, "redirect": c.RedirectURL, "version": c.Version, "scopes": c.Scopes, "target_type": targetType}
	if c.AuthMode != "" || c.ConfigID != "" {
		base["auth_mode"], base["config_id"] = c.AuthMode, c.ConfigID
	}
	return hashString(encode(base))
}

type pendingAuthorization struct {
	Pages         []ConnectedPage `json:"pages"`
	Refresh       string          `json:"refresh"`
	ExpiresAt     *time.Time      `json:"expires_at"`
	RefreshExpiry *time.Time      `json:"refresh_expiry"`
}

func CompleteAuthorization(ctx context.Context, state, code, denied string) (count int, resultError error) {
	if len(state) < 32 || len(state) > 200 {
		return 0, errors.New("invalid or expired authorization state")
	}
	var row gdb.Record
	e := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		var e error
		row, e = tx.GetOne(`SELECT * FROM bm_social_oauth_states WHERE state_hash=$1 AND used_at IS NULL AND status='pending' AND expires_at>now() FOR UPDATE`, hashString(state))
		if e != nil {
			return e
		}
		if len(row) == 0 {
			return errors.New("invalid or expired authorization state")
		}
		_, e = tx.Model("bm_social_oauth_states").Where("state_hash", hashString(state)).Data("used_at", time.Now()).Update()
		return e
	})
	if e != nil {
		return 0, e
	}
	defer func() {
		if resultError != nil {
			_, _ = g.DB().Model("bm_social_oauth_states").Ctx(ctx).Where("state_hash", hashString(state)).Where("status", "pending").Data(g.Map{"status": "failed", "encrypted_payload": "", "error": "authorization failed; start a new connection"}).Update()
		}
	}()
	if denied != "" || code == "" {
		return 0, errors.New("authorization was not granted")
	}
	p, e := governance.Load(ctx, row["owner_id"].Int64())
	if e != nil {
		return 0, e
	}
	if p.DeptID != row["dept_id"].Int64() {
		return 0, errors.New("owner department changed")
	}
	if e = require(p, "social:account:manage"); e != nil {
		return 0, e
	}
	if e = p.CheckOwnership(ctx, "social:account:manage", p.DeptID, p.ID); e != nil {
		return 0, e
	}
	platform := row["platform"].String()
	c, e := requirePlatform(ctx, platform, false)
	if e != nil {
		return 0, e
	}
	targetType := row["target_type"].String()
	if authorizationFingerprint(c, targetType) != row["app_fingerprint"].String() {
		return 0, errors.New("application settings changed; reconnect")
	}
	secret, e := growth.SocialCredential(c.ClientSecret, true)
	if e != nil {
		return 0, errors.New("application credential unavailable")
	}
	response, e := oauthRequest(ctx, platform, url.Values{"grant_type": {"authorization_code"}, "code": {code}, "client_id": {c.ClientID}, "client_secret": {secret}, "redirect_uri": {c.RedirectURL}})
	if e != nil {
		return 0, fmt.Errorf("OAuth code exchange: %w", e)
	}
	token := gconv.String(response["access_token"])
	if token == "" {
		return 0, errors.New("authorization returned no access token")
	}
	if platform == "facebook" {
		extended, err := oauthRequest(ctx, platform, url.Values{"grant_type": {"fb_exchange_token"}, "client_id": {c.ClientID}, "client_secret": {secret}, "fb_exchange_token": {token}})
		if err != nil {
			return 0, fmt.Errorf("OAuth long-lived token exchange: %w", err)
		}
		if next := gconv.String(extended["access_token"]); next != "" {
			token = next
			response = extended
		}
	}
	scopes := scopeList(gconv.String(response["scope"]))
	if platform == "linkedin" && len(scopes) == 0 {
		req, _ := http.NewRequestWithContext(ctx, "POST", "https://www.linkedin.com/oauth/v2/introspectToken", strings.NewReader(url.Values{"client_id": {c.ClientID}, "client_secret": {secret}, "token": {token}}.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		info, _, err := execute(req, false)
		if err != nil {
			return 0, err
		}
		if !gconv.Bool(info["active"]) {
			return 0, errors.New("LinkedIn access token inactive")
		}
		scopes = scopeList(gconv.String(info["scope"]))
	}
	var pages []ConnectedPage
	if platform == "instagram" || platform == "threads" {
		pages, response, e = completeMetaAuthorization(ctx, platform, c, secret, token, response)
	} else if platform == "linkedin" && targetType == "person" {
		pages, e = discoverLinkedInMember(ctx, token, scopes)
	} else {
		pages, e = discoverPages(ctx, platform, token, scopes)
	}
	if e != nil {
		return 0, fmt.Errorf("OAuth Page discovery: %w", e)
	}
	pending := pendingAuthorization{Pages: pages, Refresh: gconv.String(response["refresh_token"])}
	if seconds := gconv.Int64(response["expires_in"]); seconds > 0 {
		expiry := time.Now().Add(time.Duration(seconds) * time.Second)
		pending.ExpiresAt = &expiry
	}
	if seconds := gconv.Int64(response["refresh_token_expires_in"]); seconds > 0 {
		expiry := time.Now().Add(time.Duration(seconds) * time.Second)
		pending.RefreshExpiry = &expiry
	}
	encrypted, e := growth.SocialCredential(encode(pending), false)
	if e != nil {
		return 0, e
	}
	result, e := g.DB().Model("bm_social_oauth_states").Ctx(ctx).Where("state_hash", hashString(state)).Where("status", "pending").Where("expires_at>now()").Data(g.Map{"status": "ready", "encrypted_payload": encrypted}).Update()
	if e != nil {
		return 0, e
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return 0, errors.New("authorization task cancelled or expired")
	}
	return len(pages), nil
}
func CreateManualAccount(ctx context.Context, p *governance.Principal, name string) (g.Map, error) {
	if e := require(p, "social:account:manage"); e != nil {
		return nil, e
	}
	if e := p.CheckOwnership(ctx, "social:account:manage", p.DeptID, p.ID); e != nil {
		return nil, e
	}
	name = str(name, 200)
	if name == "" {
		return nil, errors.New("manual publishing label required")
	}
	id, e := g.DB().Model("bm_social_accounts").Ctx(ctx).Data(g.Map{"platform": "facebook", "target_type": "person", "publish_mode": "manual", "external_id": "manual:" + uuid.NewString(), "name": name, "owner_id": p.ID, "dept_id": p.DeptID, "status": "connected", "capabilities": encode(g.Map{"manual_publish": true})}).InsertAndGetId()
	if e == nil {
		e = securityAudit(ctx, p, "manual_account_created", id)
	}
	return g.Map{"id": id}, e
}

func AccountControl(ctx context.Context, p *governance.Principal, id int64, operation string, data g.Map) error {
	row, e := scoped(ctx, p, "bm_social_accounts", id, "social:account:manage")
	if e != nil {
		return e
	}
	switch operation {
	case "disconnect":
		e = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
			if _, e := tx.Model("bm_social_accounts").Where("id", id).Data(g.Map{"token": "", "refresh_token": "", "upload_token": "", "upload_token_fingerprint": "", "status": "disconnected", "capabilities": "{}", "updated_at": time.Now()}).Update(); e != nil {
				return e
			}
			if _, e := tx.Model("bm_social_interactions").Where("account_id", id).Delete(); e != nil {
				return e
			}
			_, e := tx.Model("bm_social_deliveries").Where("account_id", id).WhereIn("status", []string{"scheduled", "retry", "blocked", "manual_due"}).Data(g.Map{"status": "blocked", "error": "account disconnected"}).Update()
			return e
		})
	case "grant":
		if row["target_type"].String() == "person" {
			return errors.New("personal publishing identities cannot be shared across departments")
		}
		if !p.Super || !gconv.Bool(data["eligible_page_team"]) {
			return errors.New("administrator must confirm the department belongs to the authorized Page team")
		}
		dept := gconv.Int64(data["dept_id"])
		n, err := g.DB().Model("bm_departments").Where("id", dept).Where("active", true).Count()
		if err != nil {
			return err
		}
		if n != 1 {
			return errors.New("department unavailable")
		}
		_, e = g.DB().Model("bm_social_account_grants").Data(g.Map{"account_id": id, "dept_id": dept, "can_read": gconv.Bool(data["can_read"]), "can_publish": gconv.Bool(data["can_publish"])}).OnConflict("account_id,dept_id").Save()
	case "revoke_grant":
		if !p.Super {
			return errors.New("administrator required")
		}
		_, e = g.DB().Model("bm_social_account_grants").Where("account_id", id).Where("dept_id", data["dept_id"]).Delete()
	case "sync":
		if row["publish_mode"].String() == "manual" || row["target_type"].String() == "person" {
			return errors.New("automatic statistics are unavailable for this personal publishing target")
		}
		if _, e = requirePlatform(ctx, row["platform"].String(), false); e != nil {
			return e
		}
		_, e = queue(ctx, p, 0, id, "sync", "", g.Map{})
	case "refresh":
		e = refreshAccount(ctx, row)
	default:
		return errors.New("unsupported account operation")
	}
	if e == nil {
		e = securityAudit(ctx, p, operation, id)
	}
	return e
}
func refreshAccount(ctx context.Context, row gdb.Record) error {
	if row["platform"].String() != "linkedin" || row["refresh_token"].String() == "" {
		return errors.New("refresh token unavailable; reconnect through OAuth")
	}
	if !row["refresh_expires_at"].IsNil() && row["refresh_expires_at"].Time().Before(time.Now()) {
		return errors.New("refresh authorization expired; reconnect")
	}
	c, e := requirePlatform(ctx, "linkedin", false)
	if e != nil {
		return e
	}
	secret, e := growth.SocialCredential(c.ClientSecret, true)
	if e != nil {
		return e
	}
	refresh, e := growth.SocialCredential(row["refresh_token"].String(), true)
	if e != nil {
		return e
	}
	res, e := oauthRequest(ctx, "linkedin", url.Values{"grant_type": {"refresh_token"}, "refresh_token": {refresh}, "client_id": {c.ClientID}, "client_secret": {secret}})
	if e != nil {
		return e
	}
	token := gconv.String(res["access_token"])
	if token == "" || gconv.Int64(res["expires_in"]) <= 0 {
		return errors.New("invalid refresh response")
	}
	encrypted, e := growth.SocialCredential(token, false)
	if e != nil {
		return e
	}
	values := g.Map{"token": encrypted, "expires_at": time.Now().Add(time.Duration(gconv.Int64(res["expires_in"])) * time.Second), "status": "connected", "error": "", "updated_at": time.Now()}
	if next := gconv.String(res["refresh_token"]); next != "" {
		values["refresh_token"], e = growth.SocialCredential(next, false)
		if e != nil {
			return e
		}
	}
	if raw := gconv.String(res["scope"]); raw != "" {
		scopes := scopeList(raw)
		values["scopes"] = encode(scopes)
		values["capabilities"] = encode(accountCapabilities("linkedin", row["target_type"].String(), scopes))
	}
	_, e = g.DB().Model("bm_social_accounts").Where("id", row["id"]).Data(values).Update()
	return e
}
func FacebookWebhook(ctx context.Context, signature string, body []byte) error {
	c, e := requirePlatform(ctx, "facebook", false)
	if e != nil {
		return e
	}
	key, e := growth.SocialCredential(c.ClientSecret, true)
	if e != nil {
		return e
	}
	provided, e := hex.DecodeString(strings.TrimPrefix(signature, "sha256="))
	if e != nil || !strings.HasPrefix(signature, "sha256=") {
		return errors.New("invalid webhook signature")
	}
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write(body)
	if !hmac.Equal(provided, mac.Sum(nil)) {
		return errors.New("invalid webhook signature")
	}
	var data struct {
		Object string `json:"object"`
		Entry  []struct {
			ID string `json:"id"`
		} `json:"entry"`
	}
	if json.Unmarshal(body, &data) != nil || data.Object != "page" {
		return errors.New("invalid webhook payload")
	}
	for _, entry := range data.Entry {
		_, e = g.DB().Model("bm_social_accounts").Where("platform", "facebook").Where("external_id", entry.ID).Where("status", "connected").Data("next_sync_at", time.Now()).Update()
		if e != nil {
			return e
		}
	}
	return nil
}
func WebhookChallenge(ctx context.Context, token, challenge string) (string, error) {
	c, e := requirePlatform(ctx, "facebook", false)
	if e != nil {
		return "", e
	}
	if c.VerifyToken == "" {
		return "", errors.New("webhook not configured")
	}
	saved, e := growth.SocialCredential(c.VerifyToken, true)
	if e != nil || !hmac.Equal([]byte(saved), []byte(token)) {
		return "", errors.New("invalid webhook verification")
	}
	return challenge, nil
}
func FacebookDeauthorize(ctx context.Context, signed string) error {
	c, e := requirePlatform(ctx, "facebook", false)
	if e != nil {
		return e
	}
	key, e := growth.SocialCredential(c.ClientSecret, true)
	if e != nil {
		return e
	}
	parts := strings.Split(signed, ".")
	if len(parts) != 2 {
		return errors.New("invalid signed request")
	}
	sig, e := base64.RawURLEncoding.DecodeString(parts[0])
	if e != nil {
		return e
	}
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(parts[1]))
	if !hmac.Equal(sig, mac.Sum(nil)) {
		return errors.New("invalid signed request")
	}
	b, e := base64.RawURLEncoding.DecodeString(parts[1])
	if e != nil {
		return e
	}
	var data g.Map
	if json.Unmarshal(b, &data) != nil || data["algorithm"] != "HMAC-SHA256" || gconv.String(data["user_id"]) == "" {
		return errors.New("invalid signed request")
	}
	return g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		rows, e := tx.Model("bm_social_accounts").Where("platform", "facebook").Where("provider_user", data["user_id"]).All()
		if e != nil {
			return e
		}
		for _, row := range rows {
			if _, e = tx.Model("bm_social_interactions").Where("account_id", row["id"]).Delete(); e != nil {
				return e
			}
			if _, e = tx.Model("bm_social_accounts").Where("id", row["id"]).Data(g.Map{"token": "", "refresh_token": "", "upload_token": "", "upload_token_fingerprint": "", "status": "disconnected", "capabilities": "{}"}).Update(); e != nil {
				return e
			}
		}
		return nil
	})
}
