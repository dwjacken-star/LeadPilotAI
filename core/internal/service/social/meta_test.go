package social

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/jpeg"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"billionmail-core/internal/service/governance"
	"billionmail-core/internal/service/growth"
	"billionmail-core/internal/testsupport"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/gconv"
	"github.com/google/uuid"
)

func TestMetaVideoContainersResumeAfterRestart(t *testing.T) {
	for _, platform := range []string{"instagram", "threads"} {
		t.Run(platform, func(t *testing.T) {
			kind := "video"
			mediaType := "VIDEO"
			statusKey := "status"
			if platform == "instagram" {
				kind = "reels"
				mediaType = "REELS"
				statusKey = "status_code"
			}
			p := metaProgress{MediaType: kind}
			durable := ""
			posts, polls := 0, 0
			save := func() error { durable = encode(p); return nil }
			call := func(method, path string, body g.Map, mutation bool) (g.Map, error) {
				if method == "GET" {
					polls++
					if polls == 1 {
						return g.Map{statusKey: "IN_PROGRESS"}, nil
					}
					return g.Map{statusKey: "FINISHED"}, nil
				}
				if !mutation || p.Pending == "" || !strings.Contains(durable, `"pending"`) {
					t.Fatal("unfenced video mutation")
				}
				posts++
				if strings.HasSuffix(path, "_publish") {
					return g.Map{"id": "9001"}, nil
				}
				if body["video_url"] != "https://media.example/video" || body["media_type"] != mediaType || body["image_url"] != nil {
					t.Fatal("incorrect video container request", body)
				}
				return g.Map{"id": "8001"}, nil
			}
			for attempt := 0; attempt < 4; attempt++ {
				result, e := advanceMeta(platform, "123", "reviewed", []string{"https://media.example/video"}, &p, save, call)
				if e == nil {
					if result.ID != "9001" {
						t.Fatal(result)
					}
					break
				}
				var remote *RemoteError
				if !errors.As(e, &remote) || remote.Kind != "media_processing" {
					t.Fatal(e)
				}
				p = metaProgress{}
				must(t, json.Unmarshal([]byte(durable), &p))
			}
			if posts != 2 || p.RemoteID != "9001" {
				t.Fatal("video did not resume without recreation", posts, p)
			}
		})
	}
}

func TestMetaFacebookVideoStagesSurviveRestart(t *testing.T) {
	for _, kind := range []string{"video", "reels"} {
		t.Run(kind, func(t *testing.T) {
			p := metaProgress{}
			durable := ""
			posts := 0
			save := func() error { durable = encode(p); return nil }
			call := func(method, path string, body g.Map, mutation bool) (g.Map, error) {
				if method == "GET" {
					if strings.HasPrefix(path, "upload:") {
						return g.Map{"file_offset": 0}, nil
					}
					publishing := g.Map{"status": "not_started"}
					if p.FinishSent {
						publishing = g.Map{"status": "completed", "publish_status": "published"}
					}
					uploaded := "not_started"
					processingState := "not_started"
					bytes := int64(0)
					videoState := "uploading"
					if p.Uploaded {
						uploaded = "completed"
						processingState = "completed"
						bytes = 10
						videoState = "ready"
					}
					return g.Map{"from": g.Map{"id": "123"}, "status": g.Map{"video_status": videoState, "uploading_phase": g.Map{"status": uploaded, "bytes_transfered": bytes}, "processing_phase": g.Map{"status": processingState}, "publishing_phase": publishing}}, nil
				}
				if !mutation || p.Pending == "" || !strings.Contains(durable, `"pending"`) {
					t.Fatal("unfenced Facebook video mutation")
				}
				posts++
				switch {
				case path == "456/uploads":
					return g.Map{"id": "upload:test"}, nil
				case path == "upload:test":
					if gconv.Int64(body["offset"]) != 0 {
						t.Fatal(body)
					}
					return g.Map{"h": "handle"}, nil
				case strings.HasPrefix(path, "graph-video:"):
					if body["fbuploader_video_file_chunk"] != "handle" {
						t.Fatal(body)
					}
					return g.Map{"id": "9001"}, nil
				case strings.HasPrefix(path, "rupload:"):
					if body["file_size"] != int64(10) || gconv.Int64(body["offset"]) != 0 {
						t.Fatal(body)
					}
					return g.Map{"success": true}, nil
				case body["upload_phase"] == "start":
					return g.Map{"video_id": "9001", "upload_url": "https://rupload.facebook.com/video-upload/9001"}, nil
				case body["upload_phase"] == "finish":
					if body["video_state"] != "PUBLISHED" || body["video_id"] != "9001" || body["is_ai_generated"] != true {
						t.Fatal(body)
					}
					return g.Map{"success": true}, nil
				}
				t.Fatal("unexpected video stage", path, body)
				return nil, nil
			}
			for attempt := 0; attempt < 6; attempt++ {
				result, e := advanceFacebookVideo("123", "456", kind, "title", "reviewed", 10, true, &p, save, call)
				if e == nil {
					if result.ID != "9001" {
						t.Fatal(result)
					}
					break
				}
				var remote *RemoteError
				if !errors.As(e, &remote) || remote.Kind != "media_processing" {
					t.Fatal(e)
				}
				p = metaProgress{}
				must(t, json.Unmarshal([]byte(durable), &p))
			}
			if posts != 3 || p.RemoteID != "9001" || p.Offset != 10 {
				t.Fatal("unexpected restart-safe stage counts", posts, p)
			}
			_, e := advanceFacebookVideo("123", "456", kind, "title", "reviewed", 10, true, &p, save, call)
			must(t, e)
			if posts != 3 {
				t.Fatal("published video replayed")
			}
		})
	}
}

func TestMetaFacebookUnknownVideoOperationsReconcile(t *testing.T) {
	for _, offset := range []int64{0, 4} {
		p := metaProgress{UploadSession: "upload:test", Pending: "upload_video"}
		posts := 0
		save := func() error { return nil }
		call := func(method, path string, body g.Map, mutation bool) (g.Map, error) {
			if method == "GET" {
				return g.Map{"file_offset": offset}, nil
			}
			posts++
			if gconv.Int64(body["offset"]) != 4 || p.Pending != "upload_video" {
				t.Fatal("unconfirmed offset retry", body)
			}
			return g.Map{"h": "handle"}, nil
		}
		_, e := advanceFacebookVideo("123", "456", "video", "", "", 10, false, &p, save, call)
		var remote *RemoteError
		if offset == 0 {
			if !errors.As(e, &remote) || !remote.Unknown || posts != 0 {
				t.Fatal("unknown upload blindly retried", e)
			}
		} else if posts != 1 || p.Offset != 10 || p.FileHandle != "handle" {
			t.Fatal("confirmed partial offset not resumed", e, p)
		}
	}
	p := metaProgress{VideoID: "9001", UploadURL: "https://rupload.facebook.com/video-upload/9001", Pending: "finish_reel"}
	posts := 0
	call := func(method, path string, body g.Map, mutation bool) (g.Map, error) {
		if method != "GET" {
			posts++
			t.Fatal("unknown finish must not be replayed")
		}
		return g.Map{"from": g.Map{"id": "123"}, "status": g.Map{"publishing_phase": g.Map{"status": "completed", "publish_status": "published"}}}, nil
	}
	result, e := advanceFacebookVideo("123", "456", "reels", "", "", 10, false, &p, func() error { return nil }, call)
	must(t, e)
	if result.ID != "9001" || posts != 0 {
		t.Fatal("failed read-only publication reconciliation")
	}
	p = metaProgress{Pending: "publish_page_video", UploadSession: "upload:test", FileHandle: "handle"}
	_, e = advanceFacebookVideo("123", "456", "video", "", "", 10, false, &p, func() error { return nil }, call)
	var remote *RemoteError
	if !errors.As(e, &remote) || !remote.Unknown {
		t.Fatal("unknown Page video publication must remain fenced", e)
	}
	for _, raw := range []string{"https://evil.example/video-upload/9001", "http://rupload.facebook.com/video-upload/9001", "https://rupload.facebook.com/video-upload/9002", "https://rupload.facebook.com/video-upload/9001?token=x"} {
		if validReelUpload(raw, "9001") {
			t.Fatal("untrusted uploader accepted", raw)
		}
	}
	if !validUploadSession("upload:abc==?sig=signature") || validUploadSession("upload:abc?redirect=https://evil.example") {
		t.Fatal("unsafe upload session signature handling")
	}
	if !validUploadSession("upload:"+strings.Repeat("a", 2048)) || validUploadSession("upload:"+strings.Repeat("a", 2049)) || !validUploadSession("upload:a?sig="+strings.Repeat("a", 2048)) || validUploadSession("upload:a?sig="+strings.Repeat("a", 2049)) || validUploadSession("upload:a?sig="+strings.Repeat("%61", 2049)) {
		t.Fatal("upload session/signature bounds changed")
	}
	t.Run("lost_upload_session_receipt_preserves_fence", func(t *testing.T) {
		p := metaProgress{}
		durable := ""
		saves, posts := 0, 0
		save := func() error {
			saves++
			if saves == 3 {
				return errors.New("receipt storage unavailable")
			}
			durable = encode(p)
			return nil
		}
		call := func(string, string, g.Map, bool) (g.Map, error) { posts++; return g.Map{"id": "upload:test"}, nil }
		_, e := advanceFacebookVideo("123", "456", "video", "", "", 10, false, &p, save, call)
		var remote *RemoteError
		if !errors.As(e, &remote) || !remote.Unknown {
			t.Fatal("lost receipt was treated as safe retry", e)
		}
		p = metaProgress{}
		must(t, json.Unmarshal([]byte(durable), &p))
		_, e = advanceFacebookVideo("123", "456", "video", "", "", 10, false, &p, save, call)
		if !errors.As(e, &remote) || !remote.Unknown || posts != 1 {
			t.Fatal("lost upload session was recreated", e, posts)
		}
	})
}

func TestMetaVideoInspectionLimits(t *testing.T) {
	raw := g.Map{"format": g.Map{"format_name": "mov,mp4", "duration": "4"}, "streams": []g.Map{{"index": 0, "codec_type": "video", "codec_name": "h264", "width": 720, "height": 1280, "pix_fmt": "yuv420p", "avg_frame_rate": "24/1", "r_frame_rate": "24/1", "bit_rate": "2000000", "has_b_frames": 0, "field_order": "progressive"}}}
	packets := []g.Map{}
	for i := 0; i < 96; i++ {
		flag := "__"
		if i%48 == 0 {
			flag = "K_"
		}
		packets = append(packets, g.Map{"stream_index": 0, "duration_time": strconv.FormatFloat(1.0/24, 'f', 6, 64), "pts_time": strconv.FormatFloat(float64(i)/24, 'f', 6, 64), "flags": flag, "size": "10000"})
	}
	raw["packets"] = packets
	m, e := parseVideoProbe([]byte(encode(raw)))
	must(t, e)
	m.FastStart = true
	for _, platform := range []string{"instagram", "threads", "facebook"} {
		kind := "video"
		if platform == "instagram" {
			kind = "reels"
		}
		must(t, validateVideo(platform, kind, m))
	}
	must(t, validateVideo("facebook", "reels", m))
	m.Duration = 91
	if validateVideo("facebook", "reels", m) == nil {
		t.Fatal("oversized Reel accepted")
	}
	must(t, validateVideo("threads", "video", m))
	packets[3]["duration_time"] = "0.1"
	raw["packets"] = packets
	m, e = parseVideoProbe([]byte(encode(raw)))
	must(t, e)
	m.FastStart = true
	if validateVideo("threads", "video", m) == nil {
		t.Fatal("variable frame rate accepted")
	}
	t.Setenv("PATH", filepath.Join(".tools", "missing-video-inspector"))
	if _, e = inspectVideo(context.Background(), "unused.mp4"); e == nil || !strings.Contains(e.Error(), "ffprobe required") {
		t.Fatal("missing inspector did not gate video", e)
	}
}

func TestMetaVideoInspectorContract(t *testing.T) {
	path := os.Getenv("BM_TEST_META_VIDEO_FILE")
	if path == "" {
		t.Skip("set BM_TEST_META_VIDEO_FILE to a local synthetic 720x1280 H264/yuv420p 24fps 4s MP4, bf0, closed GOP <=5s, fast-start, no edit lists")
	}
	m, e := inspectVideo(context.Background(), path)
	must(t, e)
	if m.Width != 720 || m.Height != 1280 || math.Abs(m.FPS-24) > 0.001 || math.Abs(m.Duration-4) > 0.1 {
		t.Fatal("unexpected contract fixture", m)
	}
	must(t, validateVideo("instagram", "reels", m))
	must(t, validateVideo("threads", "video", m))
	must(t, validateVideo("facebook", "video", m))
	must(t, validateVideo("facebook", "reels", m))
}

func TestMetaContainerProgressSurvivesRestart(t *testing.T) {
	for _, platform := range []string{"instagram", "threads"} {
		t.Run(platform, func(t *testing.T) {
			progress := metaProgress{AIGenerated: true}
			durable := ""
			saves := 0
			save := func() error { durable = encode(progress); saves++; return nil }
			postCalls, finals := 0, 0
			call := func(method, path string, body g.Map, mutation bool) (g.Map, error) {
				if method == "GET" {
					if platform == "instagram" {
						return g.Map{"status_code": "FINISHED"}, nil
					}
					return g.Map{"status": "FINISHED"}, nil
				}
				if !mutation || progress.Pending == "" || !strings.Contains(durable, `"pending"`) {
					t.Fatal("remote mutation was not durably fenced")
				}
				postCalls++
				if strings.HasSuffix(path, "_publish") {
					finals++
					if body["creation_id"] != "container" {
						t.Fatal("wrong parent container")
					}
					return g.Map{"id": "published"}, nil
				}
				if body["media_type"] == "CAROUSEL" {
					if body["children"] != "child-1,child-2" {
						t.Fatal("wrong child order")
					}
					label := "is_ai_generated"
					if platform == "threads" {
						label = "is_made_with_ai"
					}
					if body[label] != true {
						t.Fatal("AI media disclosure missing from parent")
					}
					return g.Map{"id": "container"}, nil
				}
				if !gconv.Bool(body["is_carousel_item"]) {
					t.Fatal("child flag absent")
				}
				if body["is_ai_generated"] != nil || body["is_made_with_ai"] != nil {
					t.Fatal("AI disclosure incorrectly applied to carousel child")
				}
				return g.Map{"id": "child-" + gconv.String(postCalls)}, nil
			}
			var err error
			var remote *RemoteError
			for attempt := 0; attempt < 5 && progress.Container == ""; attempt++ {
				before := postCalls
				_, err = advanceMeta(platform, "123", "approved", []string{"https://media.example/1", "https://media.example/2"}, &progress, save, call)
				if !errors.As(err, &remote) || remote.Kind != "media_processing" || postCalls-before > 1 {
					t.Fatal("unbounded container preparation", err, postCalls-before)
				}
				progress = metaProgress{}
				must(t, json.Unmarshal([]byte(durable), &progress))
			}
			if progress.Container == "" || postCalls != 3 {
				t.Fatal("container preparation failed", postCalls)
			}
			result, err := advanceMeta(platform, "123", "approved", []string{"https://media.example/1", "https://media.example/2"}, &progress, save, call)
			must(t, err)
			if result.ID != "published" || finals != 1 || saves < 6 {
				t.Fatal("restart did not resume safely", result, finals, saves)
			}
			_, err = advanceMeta(platform, "123", "approved", nil, &progress, save, call)
			must(t, err)
			if finals != 1 {
				t.Fatal("published receipt was replayed")
			}
		})
	}
}

func TestMetaUnknownMutationNeverRetries(t *testing.T) {
	for _, pending := range []string{"", "publish_container"} {
		progress := metaProgress{Pending: pending}
		calls := 0
		call := func(string, string, g.Map, bool) (g.Map, error) {
			calls++
			return nil, &RemoteError{Kind: "connection_failed", Unknown: true}
		}
		save := func() error { return nil }
		_, err := advanceMeta("threads", "123", "text", nil, &progress, save, call)
		var remote *RemoteError
		if !errors.As(err, &remote) || !remote.Unknown || progress.Pending == "" {
			t.Fatal("uncertain mutation not fenced", err)
		}
		before := calls
		_, err = advanceMeta("threads", "123", "text", nil, &progress, save, call)
		if !errors.As(err, &remote) || !remote.Unknown || calls != before {
			t.Fatal("unknown operation retried", err, calls)
		}
		t.Run("receipt commit failure preserves durable fence", func(t *testing.T) {
			progress := metaProgress{Container: "container", CreatedAt: time.Now().Unix()}
			durable := ""
			saves, posts := 0, 0
			save := func() error {
				saves++
				if saves == 2 {
					return errors.New("fixture receipt persistence failure")
				}
				durable = encode(progress)
				return nil
			}
			call := func(method, _ string, _ g.Map, _ bool) (g.Map, error) {
				if method == "GET" {
					return g.Map{"status": "FINISHED"}, nil
				}
				posts++
				return g.Map{"id": "published"}, nil
			}
			_, err := advanceMeta("threads", "123", "text", nil, &progress, save, call)
			var remote *RemoteError
			if !errors.As(err, &remote) || !remote.Unknown || posts != 1 {
				t.Fatal("receipt failure not uncertain", err, posts)
			}
			progress = metaProgress{}
			must(t, json.Unmarshal([]byte(durable), &progress))
			_, err = advanceMeta("threads", "123", "text", nil, &progress, save, call)
			if !errors.As(err, &remote) || !remote.Unknown || posts != 1 {
				t.Fatal("restart replayed uncommitted receipt", err, posts)
			}
		})
	}
}

func TestMetaProcessingIsBoundedAndPublishedIsNotReplayed(t *testing.T) {
	for _, status := range []string{"IN_PROGRESS", "ERROR", "EXPIRED", "PUBLISHED"} {
		progress := metaProgress{Container: "container", CreatedAt: time.Now().Unix()}
		finals := 0
		call := func(method, path string, _ g.Map, _ bool) (g.Map, error) {
			if method == "POST" {
				finals++
				return g.Map{"id": "bad"}, nil
			}
			return g.Map{"status": status}, nil
		}
		var err error
		for i := 0; i < 10; i++ {
			_, err = advanceMeta("threads", "123", "text", nil, &progress, func() error { return nil }, call)
		}
		if err == nil || finals != 0 {
			t.Fatal("non-ready or already-published media finalized", status, err, finals)
		}
		if status == "IN_PROGRESS" && progress.Polls != 10 {
			t.Fatal("processing budget not persisted")
		}
	}
}

func TestMetaMediaRulesAndSignedCapabilities(t *testing.T) {
	var jpegData bytes.Buffer
	must(t, jpeg.Encode(&jpegData, image.NewRGBA(image.Rect(0, 0, 640, 640)), nil))
	media := []MediaFile{{ID: "task:image", MIME: "image/jpeg", Data: jpegData.Bytes()}}
	must(t, validateMetaContent("instagram", "caption", "", media))
	if validateMetaContent("instagram", "caption", "", nil) == nil {
		t.Fatal("Instagram text-only allowed")
	}
	if validateMetaContent("threads", strings.Repeat("x", 501), "", nil) == nil {
		t.Fatal("oversize Threads text allowed")
	}
	media[0].Data = []byte("not an image")
	if validateMetaContent("instagram", "caption", "", media) == nil {
		t.Fatal("invalid image accepted")
	}
	key := []byte(strings.Repeat("k", 32))
	sig := mediaSignature(key, 1, 0, 123, "digest")
	for _, other := range []string{mediaSignature(key, 2, 0, 123, "digest"), mediaSignature(key, 1, 1, 123, "digest"), mediaSignature(key, 1, 0, 124, "digest"), mediaSignature(key, 1, 0, 123, "changed")} {
		if sig == other {
			t.Fatal("signed capability was not bound to exact resource")
		}
	}
	if endpoint("unknown", PlatformConfig{}, "me") != "" {
		t.Fatal("unknown provider fell through to LinkedIn")
	}
	if len(metaConfigurationBlockers("instagram", PlatformConfig{}, true)) == 0 {
		t.Fatal("missing integration was ready")
	}
	if len(metaConfigurationBlockers("threads", PlatformConfig{}, true)) == 0 {
		t.Fatal("missing Threads application was ready")
	}
}

func TestMetaThreadsGatedLifecycle(t *testing.T) {
	testsupport.Database(t)
	ctx := context.Background()
	root, err := filepath.Abs(".meta-fixture-" + uuid.NewString())
	must(t, err)
	must(t, os.MkdirAll(root, 0700))
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	t.Setenv("MAPLEMAIL_GROWTH_DATA", root)
	p, err := governance.Load(ctx, 1)
	must(t, err)
	oldClient := httpClient
	t.Cleanup(func() { httpClient = oldClient })
	cfg, err := config(ctx)
	must(t, err)
	httpClient = &http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("disabled platform called provider")
		return nil, nil
	})}
	if _, err = BeginAuthorization(ctx, p, "threads"); err == nil {
		t.Fatal("unconfigured Threads authorization enabled")
	}
	readiness, err := MetaReadiness(ctx, p)
	must(t, err)
	if readiness["external_access"] != "unverified" || readiness["signed_media_configured"] != false || readiness["signed_media_confirmed"] != false {
		t.Fatal("unconfigured external access or media readiness claimed", readiness)
	}
	if enabled, ok := readiness["video_publishing"].(bool); ok && enabled {
		t.Fatal("unconfigured video publishing claimed enabled", readiness)
	}
	availability, err := Availability(ctx, p)
	must(t, err)
	for _, platform := range []string{"facebook", "instagram", "threads"} {
		blockers := gconv.Strings(readiness[platform+"_video_blockers"])
		if !has(blockers, "video_publishing_disabled") || !has(blockers, "publishing_application_configuration_required") || !has(blockers, "verified_signed_media_endpoint_required") {
			t.Fatal("unconfigured video prerequisites were not enforced", platform, blockers)
		}
		if has(blockers, "ffprobe_inspection_required") == gconv.Bool(readiness["video_inspector_available"]) {
			t.Fatal("inspector readiness and gate disagree", platform, blockers)
		}
		value := object(availability[platform])
		if value["videos"] != false || value["reels"] != false {
			t.Fatal("unconfigured video advertised as available", platform, value)
		}
		if platform != "facebook" {
			if value["enabled"] != false || value["publishing"] != false {
				t.Fatal("unconfigured Meta platform advertised as enabled", platform, value)
			}
			auth := gconv.Strings(object(readiness[platform])["authorization_blockers"])
			if !has(auth, "authorization_disabled") || !has(auth, "application_configuration_required") {
				t.Fatal("unconfigured authorization blockers missing", platform, auth)
			}
		}
	}
	cfg.Threads = PlatformConfig{Enabled: true, ClientID: "threads-app", ClientSecret: "threads-secret", RedirectURL: "https://admin.example/social/callback", Version: "v1.0", DailyCalls: 100, Scopes: []string{"threads_basic", "threads_content_publish"}, TestAccessConfirmed: true}
	cfg.MediaFetchURL = "https://admin.example/api/social/media"
	must(t, SaveSettings(ctx, p, cfg))
	cfg, err = config(ctx)
	must(t, err)
	cfg.Threads.ClientSecret = ""
	cfg.Threads.Publishing = true
	cfg.MediaFetchConfirmed = true
	must(t, SaveSettings(ctx, p, cfg))
	finals, creates := 0, 0
	fetchURL := ""
	publishingGranted := true
	httpClient = &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "graph.threads.com" {
			t.Fatal("Threads used another provider host", r.URL.Host)
		}
		switch {
		case r.URL.Path == "/oauth/access_token":
			return response(200, `{"access_token":"short","expires_in":3600}`), nil
		case r.URL.Path == "/access_token":
			return response(200, `{"access_token":"long","expires_in":5184000}`), nil
		case strings.HasSuffix(r.URL.Path, "/me/permissions"):
			if !publishingGranted {
				return response(200, `{"data":[{"permission":"threads_basic","status":"granted"}]}`), nil
			}
			return response(200, `{"data":[{"permission":"threads_basic","status":"granted"},{"permission":"threads_content_publish","status":"granted"}]}`), nil
		case strings.HasSuffix(r.URL.Path, "/me"):
			return response(200, `{"id":"123","username":"fixture"}`), nil
		case strings.HasSuffix(r.URL.Path, "/threads_publish"):
			finals++
			return response(200, `{"id":"receipt-123"}`), nil
		case strings.HasSuffix(r.URL.Path, "/threads"):
			creates++
			var body g.Map
			must(t, json.NewDecoder(r.Body).Decode(&body))
			fetchURL = gconv.String(body["image_url"])
			return response(200, `{"id":"container-123"}`), nil
		case strings.HasSuffix(r.URL.Path, "/container-123"):
			return response(200, `{"status":"FINISHED"}`), nil
		default:
			t.Fatal("unexpected provider request", r.Method, r.URL.Path)
			return nil, nil
		}
	})}
	auth, err := BeginAuthorization(ctx, p, "threads")
	must(t, err)
	link, err := url.Parse(gconv.String(auth["url"]))
	must(t, err)
	if link.Host != "threads.com" {
		t.Fatal("wrong independent OAuth host")
	}
	_, err = CompleteAuthorization(ctx, link.Query().Get("state"), "code", "")
	must(t, err)
	_, err = ConfirmAuthorization(ctx, p, gconv.Int64(auth["id"]), []string{"123"}, false)
	must(t, err)
	account, err := g.DB().Model("bm_social_accounts").Where("platform", "threads").One()
	must(t, err)
	company, err := growth.SaveCompany(ctx, p, 0, g.Map{"name": "Meta fixture", "profile": g.Map{"business": "Parts"}})
	must(t, err)
	must(t, growth.PublishCompany(ctx, p, company))
	planConfig := PlanConfig{Platforms: []string{"threads"}, Accounts: []int64{account["id"].Int64()}, Markets: []Market{{Country: "US", Language: "en", Timezone: "UTC", LocalTime: "10:00:00"}}, Cadence: "once", DailyPosts: 3, MinIntervalMinutes: 5}
	id, err := SavePlan(ctx, p, 0, "Threads fixture", company, planConfig)
	must(t, err)
	must(t, ManualVariants(ctx, p, id))
	value, err := g.DB().Model("bm_social_variants").Where("plan_id", id).Value("id")
	must(t, err)
	var picture bytes.Buffer
	must(t, jpeg.Encode(&picture, image.NewRGBA(image.Rect(0, 0, 640, 640)), nil))
	asset, err := UploadPostImage(ctx, p, value.Int64(), "approved.jpg", bytes.NewReader(picture.Bytes()))
	must(t, err)
	must(t, SaveVariant(ctx, p, value.Int64(), g.Map{"body": "Approved fixture text", "images": []string{gconv.String(asset["ref"])}}))
	must(t, ApproveVariant(ctx, p, value.Int64()))
	must(t, RunNow(ctx, p, id))
	_, err = ProcessDelivery(ctx)
	must(t, err)
	delivery, err := g.DB().Model("bm_social_deliveries").Where("plan_id", id).One()
	must(t, err)
	if delivery["status"].String() != "retry" || creates != 1 || finals != 0 {
		t.Fatal("container was not durably staged", delivery, creates, finals)
	}
	fetch, err := url.Parse(fetchURL)
	must(t, err)
	query := fetch.Query()
	fetchMedia := func(signature string, expiry int64) (MediaFile, error) {
		return SignedDeliveryMedia(ctx, delivery["id"].Int64(), gconv.Int(query.Get("index")), expiry, query.Get("digest"), signature)
	}
	served, err := fetchMedia(query.Get("signature"), gconv.Int64(query.Get("expires")))
	must(t, err)
	if !bytes.Equal(served.Data, picture.Bytes()) {
		t.Fatal("signed endpoint returned different private media")
	}
	if _, err = fetchMedia(strings.Repeat("0", 64), gconv.Int64(query.Get("expires"))); err == nil {
		t.Fatal("tampered capability accepted")
	}
	if _, err = fetchMedia(query.Get("signature"), time.Now().Add(-time.Minute).Unix()); err == nil {
		t.Fatal("expired capability accepted")
	}
	_, err = g.DB().Model("bm_social_accounts").Where("id", account["id"]).Data("status", "reconnect").Update()
	must(t, err)
	if _, err = fetchMedia(query.Get("signature"), gconv.Int64(query.Get("expires"))); err == nil {
		t.Fatal("revoked account could still expose private media")
	}
	_, err = g.DB().Model("bm_social_accounts").Where("id", account["id"]).Data("status", "connected").Update()
	must(t, err)
	_, err = g.DB().Model("bm_social_deliveries").Where("id", delivery["id"]).Data(g.Map{"status": "publishing", "lease_until": time.Now().Add(-time.Minute)}).Update()
	must(t, err)
	must(t, recoverWork(ctx))
	publishingGranted = false
	_, err = ProcessDelivery(ctx)
	must(t, err)
	delivery, err = g.DB().Model("bm_social_deliveries").Where("plan_id", id).One()
	must(t, err)
	if delivery["status"].String() != "blocked" || finals != 0 {
		t.Fatal("revoked provider publishing permission was ignored", delivery, finals)
	}
	publishingGranted = true
	must(t, DeliveryControl(ctx, p, delivery["id"].Int64(), "retry", g.Map{}))
	_, err = ProcessDelivery(ctx)
	must(t, err)
	delivery, err = g.DB().Model("bm_social_deliveries").Where("plan_id", id).One()
	must(t, err)
	if delivery["status"].String() != "published" || creates != 1 || finals != 1 {
		t.Fatal("restart duplicated or lost publishing", delivery, creates, finals)
	}
	if _, err = ProcessDelivery(ctx); err != nil || finals != 1 {
		t.Fatal("terminal receipt replayed", err, finals)
	}
}
