package social

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/gconv"
)

var httpClient = &http.Client{Timeout: 45 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

type RemoteError struct {
	Kind       string
	Status     int
	Unknown    bool
	RetryAfter time.Duration
	Code       int
	Subcode    int
	Detail     string
}

func (e *RemoteError) Error() string {
	message := fmt.Sprintf("social API %s (HTTP %d, code %d/%d)", e.Kind, e.Status, e.Code, e.Subcode)
	if e.Detail != "" {
		message += ": " + e.Detail
	}
	return message
}

var remoteCredentialPattern = regexp.MustCompile(`[A-Za-z0-9_-]{40,}`)

// Provider messages can echo credentials. Retain useful diagnostics without
// returning authorization codes, client secrets or bearer tokens to logs/UI.
func remoteDetail(req *http.Request, message string) string {
	secrets := []string{strings.TrimPrefix(strings.TrimPrefix(req.Header.Get("Authorization"), "Bearer "), "OAuth ")}
	values := req.URL.Query()
	if req.GetBody != nil {
		if body, err := req.GetBody(); err == nil {
			b, _ := io.ReadAll(io.LimitReader(body, 1024*1024))
			body.Close()
			if form, err := url.ParseQuery(string(b)); err == nil {
				for key, list := range form {
					values[key] = append(values[key], list...)
				}
			}
		}
	}
	for _, key := range []string{"client_secret", "code", "access_token", "fb_exchange_token", "refresh_token", "input_token"} {
		secrets = append(secrets, values[key]...)
	}
	for _, secret := range secrets {
		if secret != "" {
			message = strings.ReplaceAll(message, secret, "[redacted]")
			message = strings.ReplaceAll(message, url.QueryEscape(secret), "[redacted]")
		}
	}
	message = remoteCredentialPattern.ReplaceAllString(message, "[redacted]")
	message = strings.Join(strings.Fields(message), " ")
	runes := []rune(message)
	if len(runes) > 512 {
		message = string(runes[:512]) + "..."
	}
	return message
}
func endpoint(platform string, c PlatformConfig, path string) string {
	if platform == "facebook" {
		return "https://graph.facebook.com/" + c.Version + "/" + strings.TrimLeft(path, "/")
	}
	switch platform {
	case "instagram":
		return "https://graph.facebook.com/" + c.Version + "/" + strings.TrimLeft(path, "/")
	case "threads":
		return "https://graph.threads.com/" + c.Version + "/" + strings.TrimLeft(path, "/")
	case "linkedin":
		return "https://api.linkedin.com/rest/" + strings.TrimLeft(path, "/")
	default:
		return ""
	}
}
func request(ctx context.Context, platform, token, method, path string, body interface{}, posting bool) (g.Map, http.Header, error) {
	c, e := requirePlatform(ctx, platform, posting)
	if e != nil {
		return nil, nil, e
	}
	if e = reserveAPI(ctx, platform); e != nil {
		return nil, nil, e
	}
	var data io.Reader
	if body != nil {
		b, e := json.Marshal(body)
		if e != nil {
			return nil, nil, e
		}
		data = bytes.NewReader(b)
	}
	req, e := http.NewRequestWithContext(ctx, method, endpoint(platform, c, path), data)
	if e != nil {
		return nil, nil, errors.New("invalid social API request")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	if platform == "linkedin" {
		req.Header.Set("Linkedin-Version", c.Version)
		req.Header.Set("X-Restli-Protocol-Version", "2.0.0")
	}
	return execute(req, posting)
}
func execute(req *http.Request, posting bool) (g.Map, http.Header, error) {
	res, e := httpClient.Do(req)
	if e != nil {
		return nil, nil, &RemoteError{Kind: "connection_failed", Unknown: posting}
	}
	defer res.Body.Close()
	b, e := io.ReadAll(io.LimitReader(res.Body, 8*1024*1024+1))
	if e != nil || len(b) > 8*1024*1024 {
		return nil, res.Header, &RemoteError{Kind: "invalid_response", Status: res.StatusCode, Unknown: posting}
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		kind := "request_failed"
		if res.StatusCode == 401 {
			kind = "authorization_expired"
		}
		if res.StatusCode == 403 {
			kind = "permission_denied"
		}
		if res.StatusCode == 429 {
			kind = "rate_limit"
		}
		seconds, _ := strconv.Atoi(res.Header.Get("Retry-After"))
		if seconds < 60 {
			seconds = 60
		}
		remote := &RemoteError{Kind: kind, Status: res.StatusCode, Unknown: posting && res.StatusCode >= 500, RetryAfter: time.Duration(seconds) * time.Second}
		var payload struct {
			Error struct {
				Code    int    `json:"code"`
				Subcode int    `json:"error_subcode"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(b, &payload) == nil {
			remote.Code = payload.Error.Code
			remote.Subcode = payload.Error.Subcode
			remote.Detail = remoteDetail(req, payload.Error.Message)
			if remote.Code == 190 {
				remote.Kind = "authorization_expired"
			}
			if remote.Code == 4 || remote.Code == 17 || remote.Code == 32 || remote.Code == 613 {
				remote.Kind = "rate_limit"
			}
		}
		return nil, res.Header, remote
	}
	out := g.Map{}
	if len(b) > 0 {
		if json.Unmarshal(b, &out) != nil {
			return nil, res.Header, &RemoteError{Kind: "invalid_response", Status: res.StatusCode, Unknown: posting}
		}
	}
	if errData, ok := out["error"]; ok {
		code := gconv.Int(gconv.Map(errData)["code"])
		kind := "request_failed"
		if code == 190 {
			kind = "authorization_expired"
		}
		if code == 4 || code == 17 || code == 32 || code == 613 {
			kind = "rate_limit"
		}
		details := gconv.Map(errData)
		return nil, res.Header, &RemoteError{Kind: kind, Status: res.StatusCode, RetryAfter: time.Minute, Code: code, Subcode: gconv.Int(details["error_subcode"]), Detail: remoteDetail(req, gconv.String(details["message"]))}
	}
	return out, res.Header, nil
}
func oauthRequest(ctx context.Context, platform string, values url.Values) (g.Map, error) {
	if !platformOK(platform) {
		return nil, errors.New("invalid OAuth platform")
	}
	raw := "https://www.linkedin.com/oauth/v2/accessToken"
	if platform == "facebook" || platform == "instagram" {
		c, e := requirePlatform(ctx, platform, false)
		if e != nil {
			return nil, e
		}
		raw = "https://graph.facebook.com/" + c.Version + "/oauth/access_token"
	}
	if platform == "threads" {
		raw = "https://graph.threads.com/oauth/access_token"
	}
	req, e := http.NewRequestWithContext(ctx, "POST", raw, strings.NewReader(values.Encode()))
	if e != nil {
		return nil, e
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	out, _, e := execute(req, false)
	return out, e
}
func scopeList(raw string) []string {
	raw = strings.ReplaceAll(raw, ",", " ")
	return strings.Fields(raw)
}
func has(scopes []string, scope string) bool {
	for _, s := range scopes {
		if s == scope {
			return true
		}
	}
	return false
}
func capabilities(platform string, scopes []string) g.Map {
	if platform == "instagram" || platform == "threads" {
		basic, posting := "instagram_basic", "instagram_content_publish"
		if platform == "threads" {
			basic, posting = "threads_basic", "threads_content_publish"
		}
		canPublish := has(scopes, basic) && has(scopes, posting)
		return g.Map{"publish": canPublish, "images": canPublish, "comments": false, "reactions": false, "metrics": false, "webhook": false, "videos": false, "read_posts": canPublish}
	}
	if platform == "facebook" {
		return g.Map{"publish": has(scopes, "pages_manage_posts"), "comments": has(scopes, "pages_read_engagement") && has(scopes, "pages_read_user_content"), "reactions": has(scopes, "pages_read_engagement"), "metrics": has(scopes, "read_insights"), "webhook": has(scopes, "pages_manage_metadata"), "images": has(scopes, "pages_manage_posts")}
	}
	if platform != "linkedin" {
		return g.Map{"publish": false, "images": false, "comments": false, "reactions": false, "metrics": false, "webhook": false}
	}
	return g.Map{"publish": has(scopes, "w_organization_social"), "comments": has(scopes, "r_organization_social_feed"), "reactions": has(scopes, "r_organization_social_feed"), "metrics": has(scopes, "r_organization_admin") || has(scopes, "rw_organization_admin"), "webhook": false, "images": has(scopes, "w_organization_social")}
}

func accountCapabilities(platform, targetType string, scopes []string) g.Map {
	if platform == "linkedin" && targetType == "person" {
		return g.Map{"publish": has(scopes, "w_member_social"), "images": has(scopes, "w_member_social"), "comments": false, "reactions": false, "metrics": false, "webhook": false, "read_posts": false}
	}
	return capabilities(platform, scopes)
}

type ConnectedPage struct {
	ExternalID, Name, Token, ProviderUser, ProviderName string
	UploadToken                                         string
	Scopes                                              []string
	BusinessID                                          string
}

var linkedInSubject = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

func discoverLinkedInMember(ctx context.Context, token string, scopes []string) ([]ConnectedPage, error) {
	if !has(scopes, "openid") || !has(scopes, "profile") || !has(scopes, "w_member_social") {
		return nil, errors.New("LinkedIn personal authorization requires openid, profile and w_member_social")
	}
	if e := reserveAPI(ctx, "linkedin"); e != nil {
		return nil, e
	}
	req, e := http.NewRequestWithContext(ctx, "GET", "https://api.linkedin.com/v2/userinfo", nil)
	if e != nil {
		return nil, e
	}
	req.Header.Set("Authorization", "Bearer "+token)
	info, _, e := execute(req, false)
	if e != nil {
		return nil, e
	}
	subject := gconv.String(info["sub"])
	name := str(gconv.String(info["name"]), 200)
	if !linkedInSubject.MatchString(subject) || name == "" {
		return nil, errors.New("LinkedIn returned an invalid member identity")
	}
	urn := "urn:li:person:" + subject
	return []ConnectedPage{{ExternalID: urn, Name: name, Token: token, ProviderUser: urn, ProviderName: name, Scopes: scopes}}, nil
}

var facebookTaskPattern = regexp.MustCompile(`^[A-Z_]+$`)

func facebookCanCreateContent(tasks []string) bool {
	// New Pages Experience uses PROFILE_PLUS_* tasks. Read/advertising access
	// alone must never be treated as content publishing authority.
	for _, task := range []string{"CREATE_CONTENT", "MANAGE", "PROFILE_PLUS_FULL_CONTROL", "PROFILE_PLUS_CREATE_CONTENT"} {
		if has(tasks, task) {
			return true
		}
	}
	return false
}

func discoverPages(ctx context.Context, platform, token string, scopes []string) ([]ConnectedPage, error) {
	pages := []ConnectedPage{}
	listedFacebookPages := 0
	facebookTasks := map[string]bool{}
	if platform == "facebook" {
		perms, _, e := request(ctx, platform, token, "GET", "me/permissions", nil, false)
		if e != nil {
			return nil, e
		}
		scopes = nil
		for _, v := range gconv.Maps(perms["data"]) {
			if v["status"] == "granted" {
				scopes = append(scopes, gconv.String(v["permission"]))
			}
		}
		me, _, e := request(ctx, platform, token, "GET", "me?fields=id,name", nil, false)
		if e != nil {
			return nil, e
		}
		cursor := ""
		for i := 0; i < 20; i++ {
			q := url.Values{"fields": {"id,name,access_token,tasks"}, "limit": {"100"}}
			if cursor != "" {
				q.Set("after", cursor)
			}
			res, _, e := request(ctx, platform, token, "GET", "me/accounts?"+q.Encode(), nil, false)
			if e != nil {
				return nil, e
			}
			for _, v := range gconv.Maps(res["data"]) {
				listedFacebookPages++
				tasks := gconv.Strings(v["tasks"])
				for _, task := range tasks {
					if len(task) < 40 && facebookTaskPattern.MatchString(task) {
						facebookTasks[task] = true
					}
				}
				if !facebookCanCreateContent(tasks) {
					continue
				}
				pages = append(pages, ConnectedPage{ExternalID: gconv.String(v["id"]), Name: gconv.String(v["name"]), Token: gconv.String(v["access_token"]), UploadToken: token, ProviderUser: gconv.String(me["id"]), Scopes: scopes})
			}
			paging := gconv.Map(res["paging"])
			if paging["next"] == nil {
				break
			}
			next := gconv.String(gconv.Map(paging["cursors"])["after"])
			if next == "" || next == cursor {
				break
			}
			cursor = next
		}
		if len(pages) == 0 && listedFacebookPages == 0 {
			pages, e = facebookGranularPages(ctx, token, gconv.String(me["id"]), scopes)
			if e != nil {
				return nil, e
			}
		}
		for i := range pages {
			pages[i].ProviderName = str(gconv.String(me["name"]), 200)
		}
	} else {
		if len(scopes) == 0 {
			return nil, errors.New("LinkedIn token did not return granted scopes; verify app products and reconnect")
		}
		for start := 0; start < 1000; start += 100 {
			q := url.Values{"q": {"roleAssignee"}, "state": {"APPROVED"}, "start": {strconv.Itoa(start)}, "count": {"100"}}
			res, _, e := request(ctx, platform, token, "GET", "organizationAcls?"+q.Encode(), nil, false)
			if e != nil {
				return nil, e
			}
			rows := gconv.Maps(res["elements"])
			for _, v := range rows {
				role := gconv.String(v["role"])
				if role != "ADMINISTRATOR" && role != "CONTENT_ADMIN" {
					continue
				}
				urn := gconv.String(v["organization"])
				if !strings.HasPrefix(urn, "urn:li:organization:") {
					continue
				}
				id := strings.TrimPrefix(urn, "urn:li:organization:")
				info, _, e := request(ctx, platform, token, "GET", "organizations/"+url.PathEscape(id)+"?projection=(id,localizedName,vanityName)", nil, false)
				if e != nil {
					return nil, e
				}
				name := gconv.String(info["localizedName"])
				if name == "" {
					name = gconv.String(info["vanityName"])
				}
				if name == "" {
					name = urn
				}
				duplicate := false
				for _, p := range pages {
					if p.ExternalID == urn {
						duplicate = true
					}
				}
				if !duplicate {
					pages = append(pages, ConnectedPage{ExternalID: urn, Name: name, Token: token, ProviderUser: gconv.String(v["roleAssignee"]), Scopes: scopes})
				}
			}
			if len(rows) < 100 {
				break
			}
		}
	}
	if len(pages) == 0 {
		if platform == "facebook" {
			tasks := []string{}
			for task := range facebookTasks {
				tasks = append(tasks, task)
			}
			sort.Strings(tasks)
			return nil, fmt.Errorf("no authorized company Pages found (listed %d Pages; tasks: %s); check administrator role and app permissions", listedFacebookPages, strings.Join(tasks, ","))
		}
		return nil, errors.New("no authorized company Pages found; check administrator role and app permissions")
	}
	for i := range pages {
		if pages[i].ProviderName == "" {
			pages[i].ProviderName = pages[i].ProviderUser
		}
	}
	return pages, nil
}

type PublishInput struct {
	Body, Title, Link string
	Images            []MediaFile
	Video             *MediaFile
	MediaType         string
}
type MediaFile struct {
	ID, Filename, MIME string
	Data               []byte
	AIGenerated        bool
	Path               string
	SHA256             string
	Size               int64
	Video              *VideoManifest
}
type PublishResult struct{ ID, URL, State string }

func uploadImage(ctx context.Context, platform, token, owner string, image MediaFile) (string, error) {
	if platform == "facebook" {
		c, e := requirePlatform(ctx, platform, true)
		if e != nil {
			return "", e
		}
		if e = reserveAPI(ctx, platform); e != nil {
			return "", e
		}
		var b bytes.Buffer
		writer := multipart.NewWriter(&b)
		writer.WriteField("published", "false")
		part, e := writer.CreateFormFile("source", image.Filename)
		if e != nil {
			return "", e
		}
		part.Write(image.Data)
		writer.Close()
		req, e := http.NewRequestWithContext(ctx, "POST", endpoint(platform, c, url.PathEscape(owner)+"/photos"), &b)
		if e != nil {
			return "", e
		}
		req.Header.Set("Content-Type", writer.FormDataContentType())
		req.Header.Set("Authorization", "Bearer "+token)
		res, _, e := execute(req, false)
		if e != nil {
			return "", e
		}
		id := gconv.String(res["id"])
		if id == "" {
			return "", errors.New("Facebook image ID missing")
		}
		return id, nil
	}
	initialized, _, e := request(ctx, platform, token, "POST", "images?action=initializeUpload", g.Map{"initializeUploadRequest": g.Map{"owner": owner}}, false)
	if e != nil {
		return "", e
	}
	value := gconv.Map(initialized["value"])
	upload := gconv.String(value["uploadUrl"])
	id := gconv.String(value["image"])
	u, e := url.Parse(upload)
	if e != nil || u.Scheme != "https" || u.User != nil || u.Hostname() != "www.linkedin.com" || !strings.HasPrefix(u.Path, "/dms-uploads/") || !strings.HasPrefix(id, "urn:li:image:") {
		return "", errors.New("invalid LinkedIn signed upload destination")
	}
	req, e := http.NewRequestWithContext(ctx, "PUT", upload, bytes.NewReader(image.Data))
	if e != nil {
		return "", e
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", image.MIME)
	res, e := httpClient.Do(req)
	if e != nil {
		return "", errors.New("LinkedIn image upload failed")
	}
	io.Copy(io.Discard, io.LimitReader(res.Body, 4096))
	res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return "", errors.New("LinkedIn image upload rejected")
	}
	// Member publishing tokens are write-only for versioned Images API calls.
	if strings.HasPrefix(owner, "urn:li:person:") {
		return id, nil
	}
	for attempt := 0; attempt < 6; attempt++ {
		meta, _, e := request(ctx, platform, token, "GET", "images/"+url.PathEscape(id), nil, false)
		if e != nil {
			return "", e
		}
		status := gconv.String(meta["status"])
		if status == "AVAILABLE" {
			return id, nil
		}
		if status == "PROCESSING_FAILED" {
			return "", errors.New("LinkedIn image processing failed")
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(time.Second):
		}
	}
	return "", &RemoteError{Kind: "media_processing", RetryAfter: time.Minute}
}
func publish(ctx context.Context, platform, token, owner string, in PublishInput) (PublishResult, error) {
	if in.Video != nil {
		return PublishResult{}, errors.New("durable inspected video delivery required")
	}
	if platform != "facebook" && platform != "linkedin" {
		return PublishResult{}, errors.New("persistent Meta publishing operation required")
	}
	var result PublishResult
	media := []string{}
	for _, image := range in.Images {
		id, e := uploadImage(ctx, platform, token, owner, image)
		if e != nil {
			return result, e
		}
		media = append(media, id)
	}
	body := in.Body
	if in.Link != "" && len(media) > 0 {
		body += "\n\n" + in.Link
	}
	if platform == "facebook" {
		payload := g.Map{"message": body, "published": true}
		if len(media) == 0 && in.Link != "" {
			payload["link"] = in.Link
		}
		if len(media) > 0 {
			attached := []g.Map{}
			for _, id := range media {
				attached = append(attached, g.Map{"media_fbid": id})
			}
			payload["attached_media"] = attached
		}
		res, _, e := request(ctx, platform, token, "POST", url.PathEscape(owner)+"/feed", payload, true)
		if e != nil {
			return result, e
		}
		result.ID = gconv.String(res["id"])
		result.URL = "https://www.facebook.com/" + result.ID
		result.State = "published"
	} else {
		payload := g.Map{"author": owner, "commentary": body, "visibility": "PUBLIC", "distribution": g.Map{"feedDistribution": "MAIN_FEED", "targetEntities": []interface{}{}, "thirdPartyDistributionChannels": []interface{}{}}, "lifecycleState": "PUBLISHED", "isReshareDisabledByAuthor": false}
		if len(media) == 1 {
			payload["content"] = g.Map{"media": g.Map{"id": media[0], "altText": in.Title}}
		} else if len(media) > 1 {
			images := []g.Map{}
			for _, id := range media {
				images = append(images, g.Map{"id": id, "altText": in.Title})
			}
			payload["content"] = g.Map{"multiImage": g.Map{"images": images}}
		} else if in.Link != "" {
			payload["content"] = g.Map{"article": g.Map{"source": in.Link, "title": in.Title}}
		}
		res, headers, e := request(ctx, platform, token, "POST", "posts", payload, true)
		if e != nil {
			return result, e
		}
		result.ID = headers.Get("x-restli-id")
		if result.ID == "" {
			result.ID = gconv.String(res["id"])
		}
		result.URL = "https://www.linkedin.com/feed/update/" + result.ID
		result.State = "submitted"
	}
	if result.ID == "" {
		return result, &RemoteError{Kind: "missing_post_id", Unknown: true}
	}
	return result, nil
}
func readPost(ctx context.Context, platform, token, id, owner string) (PublishResult, error) {
	if platform == "instagram" || platform == "threads" {
		return readMetaPost(ctx, platform, token, id, owner)
	}
	if platform != "facebook" && platform != "linkedin" {
		return PublishResult{}, errors.New("invalid platform")
	}
	if platform == "facebook" {
		res, _, e := request(ctx, platform, token, "GET", url.PathEscape(id)+"?fields=id,from,permalink_url,is_published", nil, false)
		if e != nil {
			return PublishResult{}, e
		}
		if gconv.String(gconv.Map(res["from"])["id"]) != owner {
			return PublishResult{}, errors.New("post author mismatch")
		}
		state := "submitted"
		if gconv.Bool(res["is_published"]) {
			state = "published"
		}
		return PublishResult{ID: gconv.String(res["id"]), URL: gconv.String(res["permalink_url"]), State: state}, nil
	}
	res, _, e := request(ctx, platform, token, "GET", "posts/"+url.PathEscape(id), nil, false)
	if e != nil {
		return PublishResult{}, e
	}
	if gconv.String(res["author"]) != owner {
		return PublishResult{}, errors.New("post author mismatch")
	}
	state := "submitted"
	if res["lifecycleState"] == "PUBLISHED" {
		state = "published"
	}
	if res["lifecycleState"] == "PUBLISH_FAILED" {
		state = "failed"
	}
	return PublishResult{ID: id, URL: "https://www.linkedin.com/feed/update/" + id, State: state}, nil
}
