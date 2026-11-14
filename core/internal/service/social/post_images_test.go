package social

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"strings"
	"testing"
	"time"

	"billionmail-core/internal/service/governance"
	"billionmail-core/internal/service/growth"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/gconv"
)

func postImageFixture(t *testing.T, shade uint8) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	img.Set(1, 1, color.RGBA{R: shade, A: 255})
	var buf bytes.Buffer
	must(t, png.Encode(&buf, img))
	return buf.Bytes()
}

type fixtureImageEditor struct {
	t     *testing.T
	data  []byte
	calls int
}

func (f *fixtureImageEditor) Models(ctx context.Context, p *governance.Principal) (g.Map, error) {
	return g.Map{"enabled": true, "models": []growth.ModelOption{{Value: "fixture-image", Label: "Fixture"}}}, nil
}

func (f *fixtureImageEditor) Edit(ctx context.Context, p *governance.Principal, in growth.ImageEditInput) ([]byte, error) {
	f.calls++
	if !strings.Contains(in.Prompt, "exact product geometry") || in.Size != "1024x1024" || len(in.Image) == 0 {
		f.t.Fatal("image editing request lost fidelity instructions or source")
	}
	return f.data, nil
}
func exercisePostImageWorkflow(t *testing.T, ctx context.Context, p *governance.Principal, company int64) {
	t.Helper()
	c := PlanConfig{Platforms: []string{"facebook"}, Markets: []Market{{Country: "US", Language: "en", Timezone: "America/New_York", LocalTime: "09:20:37"}}, StartDate: time.Now().AddDate(0, 0, 2).Format("2006-01-02"), Cadence: "once", DailyPosts: 3, MinIntervalMinutes: 5}
	planID, e := SavePlan(ctx, p, 0, "Image workflow fixture", company, c)
	must(t, e)
	must(t, ManualVariants(ctx, p, planID))
	row, e := g.DB().Model("bm_social_variants").Where("plan_id", planID).One()
	must(t, e)
	id := row["id"].Int64()
	original := postImageFixture(t, 12)
	other := postImageFixture(t, 33)
	first, e := UploadPostImage(ctx, p, id, "one.png", bytes.NewReader(original))
	must(t, e)
	second, e := UploadPostImage(ctx, p, id, "two.png", bytes.NewReader(other))
	must(t, e)
	refs := []string{gconv.String(second["ref"]), gconv.String(first["ref"])}
	if _, e = UploadPostImage(ctx, p, id, "fake.png", strings.NewReader("not an image")); e == nil {
		t.Fatal("accepted fake image")
	}
	if _, e = UploadPostImage(ctx, &governance.Principal{ID: 100, DeptID: 99}, id, "x.png", bytes.NewReader(other)); e == nil {
		t.Fatal("cross-department upload")
	}
	must(t, SaveVariant(ctx, p, id, g.Map{"body": "Product offer", "images": refs}))
	plan, e := scoped(ctx, p, "bm_social_plans", planID, "social:publish")
	must(t, e)
	if _, e = Media(ctx, p, plan, refs); e == nil {
		t.Fatal("unreviewed image allowed to publish")
	}
	must(t, ApproveVariant(ctx, p, id))
	media, e := Media(ctx, p, plan, refs)
	must(t, e)
	if len(media) != 2 || media[0].ID != refs[0] || !bytes.Equal(media[1].Data, original) {
		t.Fatal("multi-image order or original bytes changed")
	}
	credential, e := growth.SocialCredential("fixture-image-account", false)
	must(t, e)
	accountID, e := g.DB().Model("bm_social_accounts").Data(g.Map{"platform": "facebook", "external_id": "post-images-fixture-page", "name": "Image fixture Page", "dept_id": p.DeptID, "owner_id": p.ID, "token": credential, "capabilities": encode(g.Map{"publish": true, "images": true})}).InsertAndGetId()
	must(t, e)
	c.Accounts = []int64{accountID}
	_, e = SavePlan(ctx, p, planID, "Image workflow fixture", company, c)
	must(t, e)
	must(t, Schedule(ctx, p, planID))
	delivery, e := g.DB().Model("bm_social_deliveries").Where("plan_id", planID).One()
	must(t, e)
	if delivery["due_at"].GTime().Second() != 37 || encode(gconv.Strings(object(delivery["snapshot"])["images"])) != encode(refs) {
		t.Fatal("scheduled timestamp seconds or image snapshot was lost", delivery)
	}
	old := growth.SocialImageEditor
	fake := &fixtureImageEditor{t: t, data: postImageFixture(t, 99)}
	growth.SocialImageEditor = fake
	defer func() { growth.SocialImageEditor = old }()
	queued, e := QueuePostImages(ctx, p, id, []string{refs[1]}, "fixture-image", "clean studio background", "1024x1024")
	must(t, e)
	jobID := gconv.Int64s(queued["jobs"])[0]
	if _, e = QueuePostImages(ctx, p, id, []string{refs[1]}, "fixture-image", "clean studio background", "1024x1024"); e == nil {
		t.Fatal("duplicate active image job accepted")
	}
	for attempt := 0; attempt < 20; attempt++ {
		worked, e := ProcessJob(ctx)
		must(t, e)
		job, e := g.DB().Model("bm_social_jobs").Where("id", jobID).One()
		must(t, e)
		if job["status"].String() == "completed" {
			break
		}
		if !worked || attempt == 19 {
			t.Fatal("image worker did not complete", job["status"], job["error"])
		}
	}
	if fake.calls != 1 {
		t.Fatal("AI image was charged more than once")
	}
	result, e := PostImages(ctx, p, id)
	must(t, e)
	candidates := gconv.Maps(result["candidates"])
	if len(candidates) != 1 || gconv.Bool(candidates[0]["approved"]) {
		t.Fatal("AI candidate not retained for review", result)
	}
	saved, e := g.DB().Model("bm_social_variants").Where("id", id).One()
	must(t, e)
	if encode(gconv.Strings(saved["images"])) != encode(refs) || saved["state"].String() != "approved" {
		t.Fatal("image processing silently replaced reviewed content")
	}
	unchanged, e := g.DB().Model("bm_social_deliveries").Where("id", delivery["id"]).One()
	must(t, e)
	if unchanged["snapshot"].String() != delivery["snapshot"].String() || unchanged["status"].String() != "scheduled" {
		t.Fatal("image processing changed an existing schedule")
	}
	originalAsset, e := Asset(ctx, p, gconv.String(first["id"]), "social:content:edit")
	must(t, e)
	if originalAsset["sha256"].String() == "" {
		t.Fatal("original was lost")
	}
	candidateRef := "task:" + gconv.String(candidates[0]["id"])
	must(t, SaveVariant(ctx, p, id, g.Map{"body": "Product offer", "images": []string{candidateRef, refs[0]}}))
	state, e := g.DB().Model("bm_social_deliveries").Where("id", delivery["id"]).Value("status")
	must(t, e)
	if state.String() != "cancelled" {
		t.Fatal("editing images did not cancel the old pending delivery")
	}
	if _, e = Media(ctx, p, plan, []string{candidateRef}); e == nil {
		t.Fatal("AI candidate published before review")
	}
	must(t, ApproveVariant(ctx, p, id))
	_, e = Media(ctx, p, plan, []string{candidateRef})
	must(t, e)
	// Exercise real official-provider payload builders with fake HTTP only.
	oldHTTP := httpClient
	defer func() { httpClient = oldHTTP }()
	for _, platform := range []string{"facebook", "linkedin"} {
		uploaded := 0
		posted := false
		httpClient = &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
			if strings.HasSuffix(r.URL.Path, "/photos") {
				uploaded++
				return response(200, `{"id":"photo-`+gconv.String(uploaded)+`"}`), nil
			}
			if strings.HasSuffix(r.URL.Path, "/images") && r.URL.Query().Get("action") == "initializeUpload" {
				uploaded++
				return response(200, `{"value":{"image":"urn:li:image:`+gconv.String(uploaded)+`","uploadUrl":"https://www.linkedin.com/dms-uploads/fixture"}}`), nil
			}
			if r.Method == "PUT" {
				return response(201, ``), nil
			}
			if strings.Contains(r.URL.Path, "/images/") {
				return response(200, `{"status":"AVAILABLE"}`), nil
			}
			if strings.HasSuffix(r.URL.Path, "/feed") || strings.HasSuffix(r.URL.Path, "/posts") {
				var body map[string]interface{}
				must(t, json.NewDecoder(r.Body).Decode(&body))
				posted = true
				if platform == "facebook" {
					if encode(body["attached_media"]) != `[{"media_fbid":"photo-1"},{"media_fbid":"photo-2"}]` {
						t.Fatal("Facebook image order", body)
					}
				} else {
					if !strings.Contains(encode(body), `"multiImage":{"images":[{"altText":"Products","id":"urn:li:image:1"},{"altText":"Products","id":"urn:li:image:2"}]}`) {
						t.Fatal("LinkedIn multi-image payload", body)
					}
				}
				return response(200, `{"id":"post-1"}`), nil
			}
			t.Fatalf("unexpected provider request %s %s", r.Method, r.URL.Path)
			return nil, nil
		})}
		_, e = publish(ctx, platform, "fixture", "urn:li:organization:42", PublishInput{Title: "Products", Body: "Product offer", Images: media})
		must(t, e)
		if !posted || uploaded != 2 {
			t.Fatal("images not published together with body", platform, uploaded, posted)
		}
	}
}

func TestPublishingSecondsAndLegacyTimes(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	m := Market{Timezone: "America/New_York", LocalTime: "09:20:37"}
	due, e := marketDue(m, "2027-01-10", now)
	must(t, e)
	if due.UTC().Format("15:04:05") != "14:20:37" {
		t.Fatal("second precision or timezone lost", due)
	}
	m.LocalTime = "09:20"
	due, e = marketDue(m, "2027-01-10", now)
	must(t, e)
	if due.Second() != 0 {
		t.Fatal("legacy minutes not supported")
	}
	for _, value := range []string{"25:00:00", "09:20:99", "09:20:37junk", "9:20:37"} {
		if _, e = parsePublishingTime(value); e == nil {
			t.Fatal("invalid clock accepted", value)
		}
	}
	m.LocalTime = "02:30:17"
	if _, e = marketDue(m, "2027-03-14", now); e == nil {
		t.Fatal("nonexistent DST time accepted")
	}
}
