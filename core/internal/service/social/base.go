package social

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"billionmail-core/internal/service/governance"
	"billionmail-core/internal/service/growth"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/gconv"
)

var Permissions = []string{"social:account:view", "social:account:manage", "social:plan:view", "social:plan:manage", "social:content:edit", "social:content:review", "social:publish", "social:interaction:view", "social:interaction:manage", "social:interaction:analyze", "social:report:view", "social:settings"}

func init() { governance.PermissionCatalog = append(governance.PermissionCatalog, Permissions...) }
func require(p *governance.Principal, code string) error {
	if p == nil || !p.Has(code) {
		return fmt.Errorf("permission required: %s", code)
	}
	return nil
}
func encode(v interface{}) string { b, _ := json.Marshal(v); return string(b) }
func object(v interface{}) g.Map {
	out := g.Map{}
	_ = json.Unmarshal([]byte(gconv.String(v)), &out)
	if out == nil {
		return g.Map{}
	}
	return out
}
func str(s string, n int) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) > n {
		s = string(r[:n])
	}
	return s
}
func platformOK(s string) bool {
	return s == "facebook" || s == "linkedin" || s == "instagram" || s == "threads"
}
func sourcePermission(p *governance.Principal) string {
	for _, code := range []string{"social:content:edit", "social:content:review", "social:publish"} {
		if p.Has(code) {
			return code
		}
	}
	return "social:plan:view"
}
func Availability(ctx context.Context, p *governance.Principal) (g.Map, error) {
	allowed := false
	for _, code := range Permissions {
		if p.Has(code) {
			allowed = true
		}
	}
	if !allowed {
		return nil, errors.New("social access required")
	}
	c, e := config(ctx)
	if e != nil {
		return nil, e
	}
	out := g.Map{"facebook": g.Map{"enabled": c.Facebook.Enabled, "publishing": c.Facebook.Publishing}, "linkedin": g.Map{"enabled": c.LinkedIn.Enabled, "publishing": c.LinkedIn.Publishing}, "analysis_enabled": c.AnalysisEnabled && c.AnalysisProviderApproved}
	for _, platform := range []string{"instagram", "threads"} {
		v := c.platform(platform)
		auth := metaConfigurationBlockers(platform, v, false)
		publishing := metaConfigurationBlockers(platform, v, true)
		if !validMediaBase(c.MediaFetchURL) {
			publishing = append(publishing, "signed_media_endpoint_required")
		}
		if !c.MediaFetchConfirmed {
			publishing = append(publishing, "signed_media_endpoint_unverified")
		}
		out[platform] = g.Map{"enabled": v.Enabled && len(auth) == 0, "publishing": v.Publishing && len(publishing) == 0, "blockers": publishing, "external_access": "unverified"}
	}
	for _, platform := range []string{"facebook", "instagram", "threads"} {
		v := out[platform].(g.Map)
		blockers := videoConfigurationBlockers(c, platform)
		v["videos"] = len(blockers) == 0
		v["reels"] = (platform == "facebook" || platform == "instagram") && len(blockers) == 0
		v["video_blockers"] = blockers
	}
	return out, nil
}
func scoped(ctx context.Context, p *governance.Principal, table string, id int64, permission string) (gdb.Record, error) {
	if e := require(p, permission); e != nil {
		return nil, e
	}
	if table != "bm_social_plans" && table != "bm_social_accounts" {
		return nil, errors.New("invalid resource")
	}
	predicate, e := p.Predicate(ctx, permission, "")
	if e != nil {
		return nil, e
	}
	row, e := g.DB().Model(table).Ctx(ctx).Where("id", id).Where(predicate).One()
	if e != nil {
		return nil, e
	}
	if len(row) == 0 || (table == "bm_social_plans" && row["status"].String() == "deleted") {
		return nil, errors.New("resource unavailable or access denied")
	}
	return row, nil
}
func accountAccess(ctx context.Context, p *governance.Principal, id int64, permission string, publish bool) (gdb.Record, error) {
	if e := require(p, permission); e != nil {
		return nil, e
	}
	predicate, e := accountPredicate(ctx, p, permission, "a", publish)
	if e != nil {
		return nil, e
	}
	row, e := g.DB().Model("bm_social_accounts a").Ctx(ctx).Where("a.id", id).Where(predicate).One()
	if e != nil {
		return nil, e
	}
	if len(row) == 0 {
		return nil, errors.New("social account access denied")
	}
	return row, nil
}
func accountPublic(row gdb.Record) gdb.Record {
	for _, k := range []string{"token", "refresh_token", "provider_user", "upload_token", "upload_token_fingerprint"} {
		delete(row, k)
	}
	return row
}
func planConfig(row gdb.Record) (PlanConfig, error) {
	var c PlanConfig
	e := json.Unmarshal(row["config"].Bytes(), &c)
	return c, e
}
func securityAudit(ctx context.Context, p *governance.Principal, action string, id int64) error {
	return governance.Audit(ctx, p, "social:"+action, id, g.Map{})
}

type PlatformConfig struct {
	Enabled             bool     `json:"enabled"`
	Publishing          bool     `json:"publishing"`
	ClientID            string   `json:"client_id"`
	ClientSecret        string   `json:"client_secret,omitempty"`
	RedirectURL         string   `json:"redirect_url"`
	Version             string   `json:"version"`
	Scopes              []string `json:"scopes"`
	DailyCalls          int      `json:"daily_calls"`
	VerifyToken         string   `json:"verify_token,omitempty"`
	AuthMode            string   `json:"auth_mode"`
	ConfigID            string   `json:"config_id"`
	TestAccessConfirmed bool     `json:"test_access_confirmed"`
	VideoPublishing     bool     `json:"video_publishing"`
}
type Config struct {
	Facebook                 PlatformConfig `json:"facebook"`
	LinkedIn                 PlatformConfig `json:"linkedin"`
	Instagram                PlatformConfig `json:"instagram"`
	Threads                  PlatformConfig `json:"threads"`
	MediaFetchURL            string         `json:"media_fetch_url"`
	MediaFetchConfirmed      bool           `json:"media_fetch_confirmed"`
	AnalysisEnabled          bool           `json:"analysis_enabled"`
	AnalysisProviderApproved bool           `json:"analysis_provider_approved"`
	AnalysisModel            string         `json:"analysis_model"`
	SyncMinutes              int            `json:"sync_minutes"`
}

func config(ctx context.Context) (Config, error) {
	c := Config{Facebook: PlatformConfig{Version: "v24.0", DailyCalls: 1000, Scopes: []string{"pages_show_list", "pages_manage_posts", "pages_read_engagement"}}, LinkedIn: PlatformConfig{Version: "202610", DailyCalls: 100, Scopes: []string{"rw_organization_admin", "w_organization_social", "r_organization_social_feed"}}, SyncMinutes: 240}
	v, e := g.DB().Model("bm_options").Ctx(ctx).Where("name", "social_config").Value("value")
	if e != nil {
		return c, e
	}
	if !v.IsEmpty() {
		e = json.Unmarshal(v.Bytes(), &c)
	}
	c.SyncMinutes = 240
	if c.Instagram.Version == "" {
		c.Instagram.Version = "v24.0"
	}
	if c.Instagram.DailyCalls == 0 {
		c.Instagram.DailyCalls = 100
	}
	if c.Threads.Version == "" {
		c.Threads.Version = "v1.0"
	}
	if c.Threads.DailyCalls == 0 {
		c.Threads.DailyCalls = 100
	}
	return c, e
}
func (c Config) platform(platform string) PlatformConfig {
	if platform == "facebook" {
		return c.Facebook
	}
	switch platform {
	case "linkedin":
		return c.LinkedIn
	case "instagram":
		return c.Instagram
	case "threads":
		return c.Threads
	default:
		return PlatformConfig{}
	}
}

var metaVersion = regexp.MustCompile(`^v[1-9][0-9]?\.0$`)
var liVersion = regexp.MustCompile(`^20[2-9][0-9](0[1-9]|1[0-2])$`)

func validRedirect(raw string) bool {
	u, e := url.Parse(raw)
	return e == nil && u.Scheme == "https" && u.Hostname() != "" && u.User == nil && u.RawQuery == "" && u.Fragment == "" && u.Path == "/social/callback"
}
func Settings(ctx context.Context, p *governance.Principal) (g.Map, error) {
	if e := require(p, "social:settings"); e != nil {
		return nil, e
	}
	if !p.Super {
		return nil, errors.New("administrator required")
	}
	c, e := config(ctx)
	if e != nil {
		return nil, e
	}
	keys := g.Map{"facebook_secret": c.Facebook.ClientSecret != "", "linkedin_secret": c.LinkedIn.ClientSecret != "", "instagram_secret": c.Instagram.ClientSecret != "", "threads_secret": c.Threads.ClientSecret != "", "facebook_verify_token": c.Facebook.VerifyToken != ""}
	c.Facebook.ClientSecret = ""
	c.LinkedIn.ClientSecret = ""
	c.Facebook.VerifyToken = ""
	c.Instagram.ClientSecret = ""
	c.Threads.ClientSecret = ""
	c.Instagram.VerifyToken = ""
	c.Threads.VerifyToken = ""
	return g.Map{"config": c, "saved": keys}, nil
}
func SaveSettings(ctx context.Context, p *governance.Principal, c Config) error {
	if e := require(p, "social:settings"); e != nil {
		return e
	}
	if !p.Super {
		return errors.New("administrator required")
	}
	old, e := config(ctx)
	if e != nil {
		return e
	}
	c.SyncMinutes = 240
	for _, platform := range []string{"facebook", "linkedin"} {
		target := &c.Facebook
		prior := old.Facebook
		if platform == "linkedin" {
			target = &c.LinkedIn
			prior = old.LinkedIn
		}
		if e = saveMetaSettings(&c, old); e != nil {
			return e
		}
		if target.DailyCalls < 1 || target.DailyCalls > 100000 {
			return errors.New("invalid daily API budget")
		}
		if platform == "facebook" && !metaVersion.MatchString(target.Version) || platform == "linkedin" && !liVersion.MatchString(target.Version) {
			return errors.New("invalid official API version")
		}
		if target.ClientSecret == "" {
			if target.ClientID != prior.ClientID && prior.ClientSecret != "" {
				return errors.New("enter a new secret when changing application")
			}
			target.ClientSecret = prior.ClientSecret
		} else {
			target.ClientSecret, e = growth.SocialCredential(strings.TrimSpace(target.ClientSecret), false)
			if e != nil {
				return e
			}
		}
		if target.VerifyToken == "" {
			target.VerifyToken = prior.VerifyToken
		} else {
			target.VerifyToken, e = growth.SocialCredential(target.VerifyToken, false)
			if e != nil {
				return e
			}
		}
		if target.Enabled && (strings.TrimSpace(target.ClientID) == "" || target.ClientSecret == "" || !validRedirect(target.RedirectURL)) {
			return errors.New("configure app ID, secret and HTTPS /social/callback URL before enabling")
		}
		if target.Publishing && !target.Enabled {
			return errors.New("enable platform authorization before publishing")
		}
		allowed := map[string]bool{}
		scopes := []string{"pages_show_list", "pages_manage_posts", "pages_read_engagement", "pages_read_user_content", "pages_manage_metadata", "read_insights"}
		if platform == "linkedin" {
			scopes = []string{"openid", "profile", "rw_organization_admin", "r_organization_admin", "w_organization_social", "r_organization_social", "r_organization_social_feed", "w_organization_social_feed", "w_member_social", "r_member_social_feed"}
		}
		for _, s := range scopes {
			allowed[s] = true
		}
		for _, s := range target.Scopes {
			if !allowed[s] {
				return errors.New("unsupported requested permission")
			}
		}
	}
	if c.AnalysisEnabled && (!c.AnalysisProviderApproved || strings.TrimSpace(c.AnalysisModel) == "") {
		return errors.New("approve the AI data processor and select an analysis model")
	}
	_, e = g.DB().Model("bm_options").Ctx(ctx).Data(g.Map{"name": "social_config", "value": encode(c)}).OnConflict("name").Save()
	if e == nil {
		e = securityAudit(ctx, p, "settings", 0)
	}
	return e
}
func requirePlatform(ctx context.Context, platform string, publish bool) (PlatformConfig, error) {
	c, e := config(ctx)
	if e != nil {
		return PlatformConfig{}, e
	}
	if !platformOK(platform) {
		return PlatformConfig{}, errors.New("invalid platform")
	}
	v := c.platform(platform)
	if platform == "instagram" || platform == "threads" {
		if blockers := metaConfigurationBlockers(platform, v, publish); len(blockers) > 0 {
			return v, fmt.Errorf("configuration required: %s", strings.Join(blockers, ", "))
		}
		if publish && (!validMediaBase(c.MediaFetchURL) || !c.MediaFetchConfirmed) {
			return v, errors.New("configuration required: verified signed HTTPS media endpoint")
		}
	}
	if !v.Enabled {
		return v, errors.New("platform not configured or disabled")
	}
	if publish && !v.Publishing {
		return v, errors.New("publishing switch is disabled")
	}
	return v, nil
}
func tokenFor(row gdb.Record) (string, error) {
	if row["publish_mode"].String() == "manual" {
		return "", errors.New("manual publishing target has no API credentials")
	}
	if row["status"].String() != "connected" {
		return "", errors.New("account requires authorization")
	}
	if !row["expires_at"].IsNil() && row["expires_at"].Time().Before(time.Now()) {
		return "", errors.New("authorization expired; reconnect account")
	}
	return growth.SocialCredential(row["token"].String(), true)
}
func reserveAPI(ctx context.Context, platform string) error {
	c, e := config(ctx)
	if e != nil {
		return e
	}
	return g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if _, e := tx.Exec(`SELECT pg_advisory_xact_lock(614846)`); e != nil {
			return e
		}
		n, e := tx.Model("bm_social_api_calls").Where("platform", platform).Where("created_at > now()-interval '24 hours'").Count()
		if e != nil {
			return e
		}
		if n >= c.platform(platform).DailyCalls {
			return &RemoteError{Kind: "rate_limit", RetryAfter: time.Hour}
		}
		_, e = tx.Model("bm_social_api_calls").Data("platform", platform).Insert()
		return e
	})
}
