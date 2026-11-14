package social

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"net/url"
	"os/exec"
	"regexp"
	"strings"
	"unicode/utf8"

	"billionmail-core/internal/service/governance"
	"billionmail-core/internal/service/growth"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/gconv"
)

var threadsLinks = regexp.MustCompile(`(?i)(?:https?://|www\.)[^\s]+`)

func validMediaBase(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.Hostname() != "" && u.User == nil && u.RawQuery == "" && u.Fragment == "" && u.Path == "/api/social/media"
}

func metaConfigurationBlockers(platform string, c PlatformConfig, publishing bool) []string {
	blockers := []string{}
	if !c.Enabled {
		blockers = append(blockers, "authorization_disabled")
	}
	if c.ClientID == "" || c.ClientSecret == "" || !validRedirect(c.RedirectURL) {
		blockers = append(blockers, "application_configuration_required")
	}
	if !c.TestAccessConfirmed {
		blockers = append(blockers, "app_role_test_access_unconfirmed")
	}
	if platform == "instagram" {
		if c.AuthMode != "unified" || c.ConfigID == "" {
			blockers = append(blockers, "unified_login_configuration_required")
		}
		if !has(c.Scopes, "instagram_basic") || !has(c.Scopes, "instagram_content_publish") {
			blockers = append(blockers, "instagram_publishing_scopes_required")
		}
	} else if platform == "threads" {
		if !has(c.Scopes, "threads_basic") || !has(c.Scopes, "threads_content_publish") {
			blockers = append(blockers, "threads_publishing_scopes_required")
		}
	}
	if publishing && !c.Publishing {
		blockers = append(blockers, "publishing_disabled")
	}
	return blockers
}

func saveMetaSettings(c *Config, old Config) error {
	for _, platform := range []string{"instagram", "threads"} {
		target, prior := &c.Instagram, old.Instagram
		if platform == "threads" {
			target, prior = &c.Threads, old.Threads
		}
		if target.Version == "" {
			target.Version = prior.Version
		}
		if target.DailyCalls == 0 {
			target.DailyCalls = prior.DailyCalls
		}
		if target.DailyCalls < 1 || target.DailyCalls > 10000 {
			return errors.New("invalid Meta API budget")
		}
		if platform == "instagram" && !metaVersion.MatchString(target.Version) || platform == "threads" && target.Version != "v1.0" {
			return errors.New("invalid Meta API version")
		}
		if target.ClientSecret == "" {
			if target.ClientID != prior.ClientID && prior.ClientSecret != "" {
				return errors.New("enter new secret when changing Meta application")
			}
			target.ClientSecret = prior.ClientSecret
		} else {
			var err error
			target.ClientSecret, err = growth.SocialCredential(strings.TrimSpace(target.ClientSecret), false)
			if err != nil {
				return err
			}
		}
		target.VerifyToken = ""
		allowed := []string{"instagram_basic", "instagram_content_publish"}
		if platform == "threads" {
			allowed = []string{"threads_basic", "threads_content_publish"}
		}
		for _, scope := range target.Scopes {
			if !has(allowed, scope) {
				return errors.New("phase one requests publishing permissions only; messaging and ads are not enabled")
			}
		}
		if target.Enabled {
			if reasons := metaConfigurationBlockers(platform, *target, false); len(reasons) > 0 {
				return errors.New("configuration required: " + strings.Join(reasons, ", "))
			}
		}
		if target.Publishing && !target.Enabled {
			return errors.New("enable platform authorization before publishing")
		}
	}
	if c.MediaFetchURL != "" && !validMediaBase(c.MediaFetchURL) {
		return errors.New("use HTTPS /api/social/media without query or fragment")
	}
	if c.MediaFetchURL != old.MediaFetchURL {
		c.MediaFetchConfirmed = false
	}
	if (c.Instagram.Publishing || c.Threads.Publishing) && (!validMediaBase(c.MediaFetchURL) || !c.MediaFetchConfirmed) {
		return errors.New("verify the signed media endpoint before enabling new-platform publishing")
	}
	for _, platform := range []string{"facebook", "instagram", "threads"} {
		if c.platform(platform).VideoPublishing {
			if reasons := videoConfigurationBlockers(*c, platform); len(reasons) > 0 {
				return errors.New("video configuration required: " + strings.Join(reasons, ", "))
			}
		}
	}
	return nil
}

func videoConfigurationBlockers(c Config, platform string) []string {
	if platform != "facebook" && platform != "instagram" && platform != "threads" {
		return []string{"video_platform_not_supported"}
	}
	v := c.platform(platform)
	out := []string{}
	if !v.VideoPublishing {
		out = append(out, "video_publishing_disabled")
	}
	if !v.Enabled || !v.Publishing || v.ClientID == "" || v.ClientSecret == "" {
		out = append(out, "publishing_application_configuration_required")
	}
	if !v.TestAccessConfirmed {
		out = append(out, "verified_app_role_test_access_required")
	}
	if platform == "facebook" {
		for _, s := range []string{"pages_show_list", "pages_read_engagement", "pages_manage_posts"} {
			if !has(v.Scopes, s) {
				out = append(out, "facebook_video_scopes_required")
				break
			}
		}
	} else {
		out = append(out, metaConfigurationBlockers(platform, v, true)...)
	}
	if !validMediaBase(c.MediaFetchURL) || !c.MediaFetchConfirmed {
		out = append(out, "verified_signed_media_endpoint_required")
	}
	if _, e := exec.LookPath("ffprobe"); e != nil {
		out = append(out, "ffprobe_inspection_required")
	}
	return out
}

func requireVideoAccount(ctx context.Context, account gdb.Record) error {
	c, e := config(ctx)
	if e != nil {
		return e
	}
	platform := account["platform"].String()
	if blockers := videoConfigurationBlockers(c, platform); len(blockers) > 0 {
		return errors.New("video configuration required: " + strings.Join(blockers, ", "))
	}
	if platform == "facebook" && (account["target_type"].String() != "page" || publishingMode(account) != "automatic") {
		return errors.New("Facebook automatic video/Reels publishing requires a Page target")
	}
	if platform == "facebook" && (account["upload_token"].String() == "" || account["upload_token_fingerprint"].String() != authorizationFingerprint(c.Facebook, "page")) {
		return errors.New("reconnect Facebook after enabling verified video preparation to retain the encrypted User upload token and revalidate Page tasks")
	}
	return nil
}

func MetaReadiness(ctx context.Context, p *governance.Principal) (g.Map, error) {
	if err := require(p, "social:settings"); err != nil {
		return nil, err
	}
	if !p.Super {
		return nil, errors.New("administrator required")
	}
	c, err := config(ctx)
	if err != nil {
		return nil, err
	}
	_, probeErr := exec.LookPath("ffprobe")
	out := g.Map{"external_access": "unverified", "dashboard_verification": "required", "signed_media_configured": validMediaBase(c.MediaFetchURL), "signed_media_confirmed": c.MediaFetchConfirmed, "video_inspector_available": probeErr == nil, "video_publishing": "disabled_unless_all_configuration_account_and_inspection_gates_pass", "video_inspection_limits": "MP4 <=128 MB, 20-second/8-MB-output ffprobe, H264/yuv420p fixed 24-60fps, no B frames, closed GOP <=5s; no automatic transcoding"}
	for _, platform := range []string{"instagram", "threads"} {
		out[platform] = g.Map{"authorization_blockers": metaConfigurationBlockers(platform, c.platform(platform), false), "publishing_blockers": metaConfigurationBlockers(platform, c.platform(platform), true)}
	}
	accounts, err := g.DB().Model("bm_social_accounts").Ctx(ctx).WhereIn("platform", []string{"facebook", "instagram", "threads"}).Where("status<>?", "deleted").Order("id").Limit(50).All()
	if err != nil {
		return nil, err
	}
	accountReadiness := []g.Map{}
	for _, account := range accounts {
		platform := account["platform"].String()
		var scopes []string
		_ = json.Unmarshal(account["scopes"].Bytes(), &scopes)
		accountReadiness = append(accountReadiness, g.Map{"id": account["id"].Int64(), "platform": platform, "target_type": account["target_type"].String(), "credential_present": account["token"].String() != "", "cached_publish_scope": gconv.Bool(accountCapabilities(platform, account["target_type"].String(), scopes)["publish"]), "status": account["status"].String(), "page_tasks": "unverified", "page_tasks_policy": "required_remote_CREATE_CONTENT_before_each_facebook_video_mutation", "user_upload_credential_present": account["upload_token"].String() != ""})
	}
	out["accounts"] = accountReadiness
	for _, platform := range []string{"facebook", "instagram", "threads"} {
		out[platform+"_video_blockers"] = videoConfigurationBlockers(c, platform)
	}
	out["facebook_reels_disclosure"] = "Page Reels are public; explicit reviewer-confirmed public sharing required"
	for _, account := range accounts {
		if account["platform"].String() == "facebook" {
			out["facebook_user_token_required"] = true
			break
		}
	}
	return out, nil
}

func validateMetaContent(platform, body, link string, media []MediaFile, videos ...*MediaFile) error {
	hasVideo := len(videos) > 0 && videos[0] != nil
	if hasVideo && len(media) > 0 {
		return errors.New("phase one requires one video without image carousel mixing")
	}
	text := strings.TrimSpace(body)
	if link != "" {
		text += "\n\n" + link
	}
	limit := 2200
	if platform == "threads" {
		limit = 500
	}
	if utf8.RuneCountInString(text) > limit {
		return errors.New("post exceeds platform caption limit")
	}
	if platform == "threads" {
		if len(text) > 500 {
			return errors.New("phase-one Threads text exceeds the conservative 500-byte UTF-8 budget")
		}
		unique := map[string]bool{}
		for _, link := range threadsLinks.FindAllString(text, -1) {
			unique[link] = true
		}
		if len(unique) > 5 {
			return errors.New("Threads permits at most five unique links")
		}
	}
	if platform == "instagram" && len(media) == 0 && !hasVideo {
		return errors.New("Instagram requires an image; text-only publishing is not supported")
	}
	if platform == "threads" && text == "" && len(media) == 0 && !hasVideo {
		return errors.New("Threads requires text or media")
	}
	// Phase one deliberately keeps the existing composer ceiling of ten images.
	if len(media) > 10 {
		return errors.New("phase one supports at most ten images")
	}
	for _, item := range media {
		cfg, kind, err := image.DecodeConfig(bytes.NewReader(item.Data))
		if err != nil || cfg.Width < 320 || cfg.Width > 1440 || cfg.Height <= 0 || len(item.Data) > 8*1024*1024 {
			return errors.New("image must be at most 8 MB and width 320-1440 pixels")
		}
		ratio := float64(cfg.Width) / float64(cfg.Height)
		if platform == "instagram" && (kind != "jpeg" || ratio < 0.8 || ratio > 1.91) {
			return errors.New("Instagram requires JPEG images with aspect ratio 4:5 to 1.91:1")
		}
		if platform == "threads" && ((kind != "jpeg" && kind != "png") || ratio > 10 || ratio < 0.1) {
			return errors.New("Threads requires JPEG/PNG images within 10:1 aspect ratio")
		}
	}
	return nil
}
