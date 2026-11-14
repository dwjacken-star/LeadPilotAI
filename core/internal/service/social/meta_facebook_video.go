package social

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"billionmail-core/internal/service/growth"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/gconv"
)

var videoIDPattern = regexp.MustCompile(`^[0-9]{1,40}$`)
var uploadSessionPattern = regexp.MustCompile(`^upload:[A-Za-z0-9_+=:/-]+$`)
var uploadSignaturePattern = regexp.MustCompile(`^[A-Za-z0-9_+=/-]+$`)

func validUploadSession(raw string) bool {
	if len(raw) > len("upload:")+2048+len("?sig=")+3*2048 {
		return false
	}
	head, query, _ := strings.Cut(raw, "?")
	payloadLength := len(head) - len("upload:")
	if payloadLength < 1 || payloadLength > 2048 || !uploadSessionPattern.MatchString(head) {
		return false
	}
	if query == "" {
		return true
	}
	values, e := url.ParseQuery(query)
	signature := values.Get("sig")
	return e == nil && len(values) == 1 && len(values["sig"]) == 1 && len(signature) >= 1 && len(signature) <= 2048 && uploadSignaturePattern.MatchString(signature)
}

func uploadSessionPath(raw string) string {
	head, query, _ := strings.Cut(raw, "?")
	path := url.PathEscape(head)
	if query != "" {
		values, _ := url.ParseQuery(query)
		path += "?" + values.Encode()
	}
	return path
}

func validReelUpload(raw, id string) bool {
	u, e := url.Parse(raw)
	if e != nil || u.Scheme != "https" || u.Host != "rupload.facebook.com" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || !videoIDPattern.MatchString(id) {
		return false
	}
	return u.Path == "/video-upload/"+id || regexp.MustCompile(`^/video-upload/v[1-9][0-9]?\.0/`+id+`$`).MatchString(u.Path)
}

func advanceFacebookVideo(owner, appID, kind, title, text string, size int64, ai bool, p *metaProgress, save func() error, call metaCall) (PublishResult, error) {
	if !videoIDPattern.MatchString(owner) || !videoIDPattern.MatchString(appID) || (kind != "video" && kind != "reels") || size <= 0 || size > MaxUploadBytes {
		return PublishResult{}, errors.New("invalid bounded Facebook video operation")
	}
	if p.RemoteID != "" {
		return PublishResult{ID: p.RemoteID, State: "published"}, nil
	}
	if p.CreatedAt == 0 {
		p.CreatedAt = time.Now().Unix()
		if e := save(); e != nil {
			return PublishResult{}, e
		}
	}
	if time.Now().Unix()-p.CreatedAt > 23*60*60 {
		return PublishResult{}, errors.New("video operation expired; review and reconcile before rescheduling")
	}
	if p.Offset < 0 || p.Offset > size {
		return PublishResult{}, errors.New("invalid persisted upload offset")
	}
	unknown := func() (PublishResult, error) {
		return PublishResult{}, &RemoteError{Kind: "video_operation_requires_reconciliation", Unknown: true}
	}
	wait := func() (PublishResult, error) {
		p.Polls++
		if e := save(); e != nil {
			return PublishResult{}, e
		}
		if p.Polls >= 10 {
			if p.Pending != "" || p.FinishSent {
				return unknown()
			}
			return PublishResult{}, errors.New("video polling limit reached")
		}
		return PublishResult{}, processing()
	}
	commit := func() error {
		p.Pending = ""
		if e := save(); e != nil {
			return &RemoteError{Kind: "video_receipt_storage_failed", Unknown: true}
		}
		return nil
	}
	mutate := func(action, path string, body g.Map) (g.Map, error) {
		p.Pending = action
		if e := save(); e != nil {
			return nil, e
		}
		result, e := call("POST", path, body, true)
		if e != nil {
			var remote *RemoteError
			if errors.As(e, &remote) && !remote.Unknown && remote.Status >= 400 {
				if e := commit(); e != nil {
					return nil, e
				}
				return nil, remote
			}
			return nil, &RemoteError{Kind: "video_mutation_outcome_unknown", Unknown: true}
		}
		return result, nil
	}
	if kind == "video" {
		if p.Pending != "" && p.Pending != "upload_video" {
			return unknown()
		}
		if p.UploadSession == "" {
			result, e := mutate("create_video_upload", appID+"/uploads", g.Map{"file_name": "approved.mp4", "file_length": size, "file_type": "video/mp4"})
			if e != nil {
				return PublishResult{}, e
			}
			p.UploadSession = gconv.String(result["id"])
			if !validUploadSession(p.UploadSession) {
				return unknown()
			}
			if e = commit(); e != nil {
				return PublishResult{}, e
			}
			return PublishResult{}, processing()
		}
		if !validUploadSession(p.UploadSession) {
			return unknown()
		}
		if p.FileHandle == "" {
			status, e := call("GET", p.UploadSession, nil, false)
			if e != nil {
				return PublishResult{}, e
			}
			offset := gconv.Int64(status["file_offset"])
			if status["file_offset"] == nil || offset < p.Offset || offset > size {
				return unknown()
			}
			if p.Pending == "upload_video" && offset <= p.Offset {
				return unknown()
			}
			p.Offset = offset
			if offset == size {
				if gconv.String(status["h"]) == "" {
					return unknown()
				}
				p.FileHandle = gconv.String(status["h"])
				p.Uploaded = true
				if e = commit(); e != nil {
					return PublishResult{}, e
				}
				return PublishResult{}, processing()
			}
			if p.Pending != "" {
				if e = commit(); e != nil {
					return PublishResult{}, e
				}
			}
			result, e := mutate("upload_video", p.UploadSession, g.Map{"offset": p.Offset})
			if e != nil {
				return PublishResult{}, e
			}
			p.FileHandle = gconv.String(result["h"])
			if p.FileHandle == "" {
				return unknown()
			}
			p.Offset = size
			p.Uploaded = true
			if e = commit(); e != nil {
				return PublishResult{}, e
			}
			return PublishResult{}, processing()
		}
		if p.VideoID == "" {
			result, e := mutate("publish_page_video", "graph-video:"+owner+"/videos", g.Map{"title": title, "description": text, "fbuploader_video_file_chunk": p.FileHandle})
			if e != nil {
				return PublishResult{}, e
			}
			p.VideoID = gconv.String(result["id"])
			if !videoIDPattern.MatchString(p.VideoID) {
				return unknown()
			}
			p.FinishSent = true
			if e = commit(); e != nil {
				return PublishResult{}, e
			}
			return PublishResult{}, processing()
		}
	} else {
		if p.Pending != "" && p.Pending != "upload_reel" && p.Pending != "finish_reel" {
			return unknown()
		}
		if p.VideoID == "" {
			result, e := mutate("create_reel_upload", owner+"/video_reels", g.Map{"upload_phase": "start"})
			if e != nil {
				return PublishResult{}, e
			}
			p.VideoID = gconv.String(result["video_id"])
			p.UploadURL = gconv.String(result["upload_url"])
			if !validReelUpload(p.UploadURL, p.VideoID) {
				return unknown()
			}
			if e = commit(); e != nil {
				return PublishResult{}, e
			}
			return PublishResult{}, processing()
		}
		if !validReelUpload(p.UploadURL, p.VideoID) {
			return unknown()
		}
	}
	status, e := call("GET", p.VideoID+"?fields=status,from,published", nil, false)
	if e != nil {
		return PublishResult{}, e
	}
	state := object(status["status"])
	uploading := object(state["uploading_phase"])
	publishing := object(state["publishing_phase"])
	processingPhase := object(state["processing_phase"])
	if gconv.String(state["video_status"]) == "error" || gconv.String(state["video_status"]) == "expired" || gconv.String(processingPhase["status"]) == "error" || gconv.String(publishing["status"]) == "error" {
		if p.Pending != "" || p.FinishSent {
			return unknown()
		}
		return PublishResult{}, errors.New("video processing failed or expired")
	}
	if gconv.String(publishing["publish_status"]) == "published" && gconv.String(publishing["status"]) == "completed" {
		if gconv.String(object(status["from"])["id"]) != owner {
			return unknown()
		}
		p.RemoteID = p.VideoID
		if e = commit(); e != nil {
			return PublishResult{}, e
		}
		return PublishResult{ID: p.RemoteID, State: "published"}, nil
	}
	if kind == "video" || p.FinishSent || p.Pending == "finish_reel" {
		return wait()
	}
	offset := gconv.Int64(uploading["bytes_transfered"])
	if offset < p.Offset || offset > size {
		return unknown()
	}
	uploaded := gconv.String(uploading["status"]) == "completed" || gconv.String(uploading["status"]) == "complete"
	if p.Pending == "upload_reel" {
		if !uploaded && offset <= p.Offset {
			return unknown()
		}
		p.Offset = offset
		p.Uploaded = uploaded && offset == size
		if uploaded && offset != size {
			return unknown()
		}
		if e = commit(); e != nil {
			return PublishResult{}, e
		}
	}
	if !p.Uploaded {
		p.Offset = offset
		if uploaded && offset == size {
			p.Uploaded = true
			if e = save(); e != nil {
				return PublishResult{}, e
			}
		} else {
			result, e := mutate("upload_reel", "rupload:"+p.UploadURL, g.Map{"offset": p.Offset, "file_size": size})
			if e != nil {
				return PublishResult{}, e
			}
			if !gconv.Bool(result["success"]) {
				return unknown()
			}
			p.Offset = size
			p.Uploaded = true
			if e = commit(); e != nil {
				return PublishResult{}, e
			}
			return PublishResult{}, processing()
		}
	}
	if gconv.String(processingPhase["status"]) != "completed" || gconv.String(state["video_status"]) != "ready" {
		return wait()
	}
	result, e := mutate("finish_reel", owner+"/video_reels", g.Map{"upload_phase": "finish", "video_id": p.VideoID, "video_state": "PUBLISHED", "title": title, "description": text, "is_ai_generated": ai})
	if e != nil {
		return PublishResult{}, e
	}
	if !gconv.Bool(result["success"]) {
		return unknown()
	}
	p.FinishSent = true
	if e = commit(); e != nil {
		return PublishResult{}, e
	}
	return PublishResult{}, processing()
}

func validateFacebookVideoTarget(ctx context.Context, account gdb.Record, userToken string, c PlatformConfig) error {
	secret, e := growth.SocialCredential(c.ClientSecret, true)
	if e != nil {
		return e
	}
	result, _, e := request(ctx, "facebook", c.ClientID+"|"+secret, "GET", "debug_token?"+url.Values{"input_token": {userToken}}.Encode(), nil, false)
	if e != nil {
		return e
	}
	info := object(result["data"])
	scopes := gconv.Strings(info["scopes"])
	if !gconv.Bool(info["is_valid"]) || gconv.String(info["app_id"]) != c.ClientID || gconv.String(info["user_id"]) != account["provider_user"].String() {
		return errors.New("Facebook User upload token identity/application mismatch")
	}
	for _, scope := range []string{"pages_show_list", "pages_read_engagement", "pages_manage_posts"} {
		if !has(scopes, scope) {
			return errors.New("Facebook video publishing scope revoked")
		}
	}
	cursor := ""
	for page := 0; page < 3; page++ {
		path := "me/accounts?fields=id,tasks&limit=100"
		if cursor != "" {
			path += "&after=" + url.QueryEscape(cursor)
		}
		result, _, e = request(ctx, "facebook", userToken, "GET", path, nil, false)
		if e != nil {
			return e
		}
		for _, target := range gconv.Maps(result["data"]) {
			if gconv.String(target["id"]) == account["external_id"].String() && facebookCanCreateContent(gconv.Strings(target["tasks"])) {
				return nil
			}
		}
		paging := object(result["paging"])
		cursor = gconv.String(object(paging["cursors"])["after"])
		if paging["next"] == nil || cursor == "" {
			break
		}
	}

	return errors.New("Facebook Page CREATE_CONTENT task not verified")
}

func readFacebookVideo(ctx context.Context, token, id, owner string) (PublishResult, error) {
	if !videoIDPattern.MatchString(id) {
		return PublishResult{}, errors.New("enter the actual numeric Facebook video ID")
	}
	result, _, e := request(ctx, "facebook", token, "GET", id+"?fields=id,status,from,permalink_url", nil, false)
	if e != nil {
		return PublishResult{}, e
	}
	if gconv.String(object(result["from"])["id"]) != owner {
		return PublishResult{}, errors.New("Facebook video owner mismatch")
	}
	state := "submitted"
	publishing := object(object(result["status"])["publishing_phase"])
	if gconv.String(publishing["status"]) == "completed" && gconv.String(publishing["publish_status"]) == "published" {
		state = "published"
	}
	return PublishResult{ID: id, URL: gconv.String(result["permalink_url"]), State: state}, nil
}

func publishFacebookVideo(ctx context.Context, id int64, account gdb.Record, pageToken string, in PublishInput) (PublishResult, error) {
	if in.Video == nil || len(in.Images) > 0 {
		return PublishResult{}, errors.New("one reviewed Facebook video required")
	}
	if in.MediaType == "video" && in.Video.AIGenerated {
		return PublishResult{}, errors.New("AI disclosure for regular Facebook Page videos is unverified; use the supported Reels disclosure or an original video")
	}
	if e := requireVideoAccount(ctx, account); e != nil {
		return PublishResult{}, e
	}
	if e := validateVideo("facebook", in.MediaType, in.Video.Video); e != nil {
		return PublishResult{}, e
	}
	c, e := requirePlatform(ctx, "facebook", true)
	if e != nil {
		return PublishResult{}, e
	}
	userToken, e := growth.SocialCredential(account["upload_token"].String(), true)
	if e != nil {
		return PublishResult{}, e
	}
	storeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	row, e := g.DB().Model("bm_social_provider_operations").Ctx(storeCtx).Where("delivery_id", id).One()
	if e != nil {
		return PublishResult{}, e
	}
	p := metaProgress{}
	if len(row) > 0 {
		if e = gconv.Scan(object(row["progress"]), &p); e != nil {
			return PublishResult{}, e
		}
	}
	fingerprint := hashString(encode(g.Map{"owner": account["external_id"].String(), "media_type": in.MediaType, "title": in.Title, "body": in.Body, "link": in.Link, "media": imageDigests([]MediaFile{*in.Video})}))
	if p.Fingerprint != "" && p.Fingerprint != fingerprint {
		return PublishResult{}, errors.New("immutable Facebook video changed; review and reschedule")
	}
	p.Fingerprint = fingerprint
	p.MediaType = in.MediaType
	save := func() error {
		_, e := g.DB().Model("bm_social_provider_operations").Ctx(storeCtx).Data(g.Map{"delivery_id": id, "progress": encode(p), "updated_at": time.Now()}).OnConflict("delivery_id").Save()
		return e
	}
	text := in.Body
	if in.Link != "" {
		text += "\n\n" + in.Link
	}
	return advanceFacebookVideo(account["external_id"].String(), c.ClientID, in.MediaType, in.Title, text, in.Video.Size, in.Video.AIGenerated, &p, save, func(method, path string, body g.Map, mutation bool) (g.Map, error) {
		if mutation {
			if e := validateFacebookVideoTarget(ctx, account, userToken, c); e != nil {
				return nil, &RemoteError{Kind: "authorization_revalidation_failed", Status: 403}
			}
		}
		if strings.HasPrefix(path, "upload:") || strings.HasPrefix(path, "rupload:") || strings.HasPrefix(path, "graph-video:") {
			if e := reserveAPI(ctx, "facebook"); e != nil {
				return nil, e
			}
			destination := endpoint("facebook", c, path)
			token := userToken
			if strings.HasPrefix(path, "upload:") {
				destination = endpoint("facebook", c, uploadSessionPath(path))
			}
			var data io.Reader
			if strings.HasPrefix(path, "rupload:") {
				destination = strings.TrimPrefix(path, "rupload:")
				token = pageToken
				if !validReelUpload(destination, p.VideoID) {
					return nil, errors.New("untrusted Reels upload destination")
				}
			}
			if strings.HasPrefix(path, "graph-video:") {
				destination = "https://graph-video.facebook.com/" + c.Version + "/" + strings.TrimPrefix(path, "graph-video:")
				token = pageToken
				values := url.Values{}
				for k, v := range body {
					values.Set(k, gconv.String(v))
				}
				data = strings.NewReader(values.Encode())
			} else if method == "POST" {
				file, e := os.Open(in.Video.Path)
				if e != nil {
					return nil, e
				}
				defer file.Close()
				if sha256File(file) != in.Video.SHA256 {
					return nil, errors.New("approved upload bytes changed")
				}
				offset := gconv.Int64(body["offset"])
				if offset < 0 || offset >= in.Video.Size {
					return nil, errors.New("invalid confirmed upload offset")
				}
				data = io.NewSectionReader(file, offset, in.Video.Size-offset)
			}
			req, e := http.NewRequestWithContext(ctx, method, destination, data)
			if e != nil {
				return nil, e
			}
			req.Header.Set("Authorization", "OAuth "+token)
			if strings.HasPrefix(path, "graph-video:") {
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			} else if method == "POST" {
				req.Header.Set("Content-Type", "application/octet-stream")
				req.ContentLength = in.Video.Size - gconv.Int64(body["offset"])
				if strings.HasPrefix(path, "rupload:") {
					req.Header.Set("offset", strconv.FormatInt(gconv.Int64(body["offset"]), 10))
					req.Header.Set("file_size", strconv.FormatInt(in.Video.Size, 10))
				} else {
					req.Header.Set("file_offset", strconv.FormatInt(gconv.Int64(body["offset"]), 10))
				}
			}
			result, _, e := execute(req, mutation)
			return result, e
		}
		token := pageToken
		if path == c.ClientID+"/uploads" {
			token = userToken
		}
		result, _, e := request(ctx, "facebook", token, method, path, body, mutation)
		return result, e
	})
}
