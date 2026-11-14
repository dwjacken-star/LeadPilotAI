package social

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"billionmail-core/internal/service/governance"
	"billionmail-core/internal/service/growth"
	"billionmail-core/internal/testsupport"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/gconv"
	"github.com/xuri/excelize/v2"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func response(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}
}
func must(t *testing.T, e error) {
	t.Helper()
	if e != nil {
		t.Fatal(e)
	}
}

type fixtureAI struct{ t *testing.T }

type failedComposeAI struct{}

func (failedComposeAI) Extract(context.Context, *governance.Principal, string, growth.AIInput, interface{}) error {
	return errors.New("AI returned invalid structured fields")
}

func (f fixtureAI) Extract(ctx context.Context, p *governance.Principal, purpose string, in growth.AIInput, out interface{}) error {
	value := ""
	switch purpose {
	case "social_material":
		value = `{"category":"company_profile","summary":"Automotive supplier","facts":[{"key":"business","value":"Automotive parts","evidence":"Automotive parts"}]}`
	case "social_compose":
		value = fmt.Sprintf(`{"title":"Parts supply","body":"Reliable automotive parts.\nContact our team.","language":%q}`, gconv.String(gconv.Map(in.Data)["language"]))
	case "social_classification":
		if strings.Contains(encode(in.Data), "actor") {
			f.t.Fatal("identity sent to classifier")
		}
		value = `{"topic":"quotation","sentiment":"neutral","priority":"high","confidence":0.99,"evidence":"quote"}`
	default:
		return errors.New("unexpected purpose")
	}
	return json.Unmarshal([]byte(value), out)
}
func TestSocialLifecycle(t *testing.T) {
	testsupport.Database(t)
	ctx := context.Background()
	t.Setenv("MAPLEMAIL_GROWTH_DATA", t.TempDir())
	p, e := governance.Load(ctx, 1)
	must(t, e)
	originalClient, originalAI := httpClient, growth.IntelligenceProvider
	t.Cleanup(func() { httpClient = originalClient; growth.IntelligenceProvider = originalAI })
	growth.IntelligenceProvider = fixtureAI{t}
	cfg, e := config(ctx)
	must(t, e)
	t.Run("unconfigured defaults never call platform", func(t *testing.T) {
		httpClient = &http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
			t.Fatal("external request while disabled")
			return nil, nil
		})}
		if _, e := BeginAuthorization(ctx, p, "facebook"); e == nil {
			t.Fatal("authorization enabled by default")
		}
		if _, e := publish(ctx, "facebook", "fixture", "page", PublishInput{Body: "test"}); e == nil {
			t.Fatal("publishing enabled by default")
		}
	})
	cfg.Facebook.Enabled = true
	cfg.Facebook.Publishing = true
	cfg.Facebook.ClientID = "fixture-app"
	cfg.Facebook.ClientSecret = "fixture-secret"
	cfg.Facebook.RedirectURL = "https://admin.example.com/social/callback"
	cfg.Facebook.VerifyToken = "fixture-verify"
	cfg.LinkedIn.Enabled = true
	cfg.LinkedIn.Publishing = true
	cfg.LinkedIn.ClientID = "fixture-linkedin"
	cfg.LinkedIn.ClientSecret = "fixture-linkedin-secret"
	cfg.LinkedIn.RedirectURL = cfg.Facebook.RedirectURL
	must(t, SaveSettings(ctx, p, cfg))
	t.Run("official OAuth exchanges only on pinned hosts and saves actual grants", func(t *testing.T) {
		calls := 0
		httpClient = &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
			calls++
			if r.URL.Host != "graph.facebook.com" {
				t.Fatal("unexpected OAuth host", r.URL.Host)
			}
			switch {
			case strings.HasSuffix(r.URL.Path, "/oauth/access_token"):
				return response(200, `{"access_token":"fixture-user-token","expires_in":3600}`), nil
			case strings.HasSuffix(r.URL.Path, "/me/permissions"):
				return response(200, `{"data":[{"permission":"pages_show_list","status":"granted"},{"permission":"pages_manage_posts","status":"granted"},{"permission":"pages_read_user_content","status":"declined"}]}`), nil
			case strings.HasSuffix(r.URL.Path, "/me/accounts"):
				return response(200, `{"data":[{"id":"222","name":"Authorized Page","access_token":"fixture-specific-page-token","tasks":["CREATE_CONTENT"]}]}`), nil
			case strings.HasSuffix(r.URL.Path, "/me"):
				return response(200, `{"id":"fixture-user"}`), nil
			}
			t.Fatal("unexpected discovery", r.URL)
			return nil, nil
		})}
		auth, e := BeginAuthorization(ctx, p, "facebook")
		must(t, e)
		u, e := url.Parse(gconv.String(auth["url"]))
		must(t, e)
		state := u.Query().Get("state")
		n, e := CompleteAuthorization(ctx, state, "fixture-code", "")
		must(t, e)
		if n != 1 {
			t.Fatal("Page not discovered")
		}
		beforeConfirmation, e := g.DB().Model("bm_social_accounts").Where("external_id", "222").Count()
		must(t, e)
		if beforeConfirmation != 0 {
			t.Fatal("callback imported a Page without operator confirmation")
		}
		_, e = ConfirmAuthorization(ctx, p, gconv.Int64(auth["id"]), []string{"222"}, false)
		must(t, e)
		account, e := g.DB().Model("bm_social_accounts").Where("external_id", "222").One()
		must(t, e)
		if account["token"].String() == "fixture-specific-page-token" || gconv.Bool(object(account["capabilities"])["comments"]) {
			t.Fatal("credential or granted scope mismatch")
		}
		before := calls
		if _, e = CompleteAuthorization(ctx, state, "fixture-code", ""); e == nil || calls != before {
			t.Fatal("callback replay called provider")
		}
	})
	t.Run("credentials encrypted and callback state single use", func(t *testing.T) {
		stored, e := config(ctx)
		must(t, e)
		if stored.Facebook.ClientSecret == "fixture-secret" {
			t.Fatal("secret stored unencrypted")
		}
		safe, e := Settings(ctx, p)
		must(t, e)
		if strings.Contains(encode(safe), "fixture-secret") {
			t.Fatal("secret exposed")
		}
		auth, e := BeginAuthorization(ctx, p, "facebook")
		must(t, e)
		u, e := url.Parse(gconv.String(auth["url"]))
		must(t, e)
		state := u.Query().Get("state")
		if len(state) < 32 {
			t.Fatal("weak state")
		}
		if _, e = CompleteAuthorization(ctx, state, "", "denied"); e == nil {
			t.Fatal("denial ignored")
		}
		if _, e = CompleteAuthorization(ctx, state, "fixture-code", ""); e == nil {
			t.Fatal("state replay")
		}
		if e = FacebookWebhook(ctx, "sha256=forged", []byte(`{}`)); e == nil {
			t.Fatal("forged webhook accepted")
		}
		if _, e = WebhookChallenge(ctx, "bad", "challenge"); e == nil {
			t.Fatal("wrong verification token")
		}
	})
	company, e := growth.SaveCompany(ctx, p, 0, g.Map{"name": "Fixture supplier", "brand": "MapleMail", "profile": g.Map{"business": "Automotive parts"}})
	must(t, e)
	must(t, growth.PublishCompany(ctx, p, company))
	c := PlanConfig{Platforms: []string{"facebook"}, Markets: []Market{{Country: "US", Language: "en", Timezone: "America/New_York", LocalTime: "09:00"}}, Requirement: "Introduce our products", StartDate: time.Now().AddDate(0, 0, 2).Format("2006-01-02"), Cadence: "once", DailyPosts: 3, MinIntervalMinutes: 5, SyncInteractions: true}
	plan, e := SavePlan(ctx, p, 0, "Social fixture", company, c)
	must(t, e)
	var sheetID string
	t.Run("task workbook preview keeps sheets and text SKUs", func(t *testing.T) {
		book := excelize.NewFile()
		defer book.Close()
		_, e := book.NewSheet("Second")
		must(t, e)
		for _, sheet := range book.GetSheetList() {
			headers := []interface{}{"SKU", "Name", "OE"}
			must(t, book.SetSheetRow(sheet, "A1", &headers))
			for n := 0; n < 25; n++ {
				row := []interface{}{fmt.Sprintf("%06d", n+1), "Compressor", "0012345"}
				must(t, book.SetSheetRow(sheet, fmt.Sprintf("A%d", n+2), &row))
			}
		}
		buf, e := book.WriteToBuffer()
		must(t, e)
		asset, e := Upload(ctx, p, plan, "products.xlsx", buf, true)
		must(t, e)
		sheetID = gconv.String(asset["id"])
		worked, e := ProcessJob(ctx)
		must(t, e)
		if !worked {
			t.Fatal("ingestion not queued")
		}
		detail, e := MaterialDetail(ctx, p, sheetID, 2, "")
		must(t, e)
		if gconv.Int(detail["total"]) != 50 || len(gconv.Maps(detail["rows"])) != 20 {
			t.Fatal("preview pagination", encode(detail))
		}
		rows := gconv.Maps(detail["rows"])
		if gconv.String(rows[0]["sku"]) != "000021" {
			t.Fatal("leading zeros lost", rows[0])
		}
		must(t, MaterialControl(ctx, p, sheetID, "approve", nil))
		n, e := g.DB().Model("bm_growth_assets").Where("company_id", company).Count()
		must(t, e)
		if n != 0 {
			t.Fatal("task upload leaked into library")
		}
	})
	t.Run("zip traversal rejected and job records failure", func(t *testing.T) {
		var data bytes.Buffer
		z := zip.NewWriter(&data)
		f, e := z.Create("../escape.txt")
		must(t, e)
		_, e = f.Write([]byte("private"))
		must(t, e)
		must(t, z.Close())
		asset, e := Upload(ctx, p, plan, "bad.zip", &data, true)
		must(t, e)
		_, e = ProcessJob(ctx)
		must(t, e)
		row, e := Asset(ctx, p, gconv.String(asset["id"]), "social:content:edit")
		must(t, e)
		if row["status"].String() != "failed" {
			t.Fatal("unsafe archive accepted")
		}
	})
	t.Run("department and file isolation require explicit Page grant", func(t *testing.T) {
		outsider := &governance.Principal{ID: 99, DeptID: p.DeptID + 100, Roles: []governance.Role{{Scope: "department", Permissions: Permissions}}}
		if _, e := Detail(ctx, outsider, plan); e == nil {
			t.Fatal("cross department plan")
		}
		if _, e := MaterialDetail(ctx, outsider, sheetID, 1, ""); e == nil {
			t.Fatal("private file leak")
		}
		if _, _, _, e := MaterialFile(ctx, outsider, sheetID); e == nil {
			t.Fatal("download leak")
		}
		if _, e := Settings(ctx, outsider); e == nil {
			t.Fatal("settings exposed")
		}
	})
	token, e := growth.SocialCredential("fixture-page-token", false)
	must(t, e)
	accountID, e := g.DB().Model("bm_social_accounts").Data(g.Map{"platform": "facebook", "external_id": "123", "name": "Fixture Page", "dept_id": p.DeptID, "owner_id": p.ID, "token": token, "scopes": encode(cfg.Facebook.Scopes), "capabilities": encode(g.Map{"publish": true, "comments": true, "reactions": true})}).InsertAndGetId()
	must(t, e)
	c.Accounts = []int64{accountID}
	t.Run("global data scope still requires cross-department Page grant", func(t *testing.T) {
		_, e := g.DB().Exec(ctx, `INSERT INTO bm_departments(id,name,active) VALUES(22,'Other team',true)`)
		must(t, e)
		other := &governance.Principal{ID: 99, DeptID: 22, Roles: []governance.Role{{Scope: "all", Permissions: Permissions}}}
		if _, e = accountAccess(ctx, other, accountID, "social:publish", true); e == nil {
			t.Fatal("global data scope bypassed Page team")
		}
		if e = AccountControl(ctx, p, accountID, "grant", g.Map{"dept_id": 22, "can_read": true, "can_publish": true}); e == nil {
			t.Fatal("grant without team confirmation")
		}
		must(t, AccountControl(ctx, p, accountID, "grant", g.Map{"dept_id": 22, "can_read": true, "can_publish": true, "eligible_page_team": true}))
		_, e = accountAccess(ctx, other, accountID, "social:publish", true)
		must(t, e)
		must(t, AccountControl(ctx, p, accountID, "revoke_grant", g.Map{"dept_id": 22}))
		if _, e = accountAccess(ctx, other, accountID, "social:publish", true); e == nil {
			t.Fatal("revoked grant remained usable")
		}
	})
	_, e = SavePlan(ctx, p, plan, "Social fixture", company, c)
	must(t, e)
	t.Run("failed generation can start a fresh job without remaining stuck", func(t *testing.T) {
		id, e := SavePlan(ctx, p, 0, "Generation retry fixture", company, c)
		must(t, e)
		original := growth.IntelligenceProvider
		defer func() { growth.IntelligenceProvider = original }()
		growth.IntelligenceProvider = failedComposeAI{}
		must(t, PlanControl(ctx, p, id, "generate"))
		if e = PlanControl(ctx, p, id, "generate"); e == nil {
			t.Fatal("concurrent generation accepted")
		}
		worked, e := ProcessJob(ctx)
		must(t, e)
		if !worked {
			t.Fatal("generation was not queued")
		}
		failed, e := g.DB().Model("bm_social_jobs").Where("plan_id", id).Where("kind", "generate").One()
		must(t, e)
		state, e := g.DB().Model("bm_social_plans").Where("id", id).One()
		must(t, e)
		if failed["status"].String() != "failed" || state["status"].String() != "draft" || state["error"].String() != "AI returned invalid structured fields" {
			t.Fatal("generation failure did not leave a retryable draft")
		}
		growth.IntelligenceProvider = original
		must(t, PlanControl(ctx, p, id, "generate"))
		jobs, e := g.DB().Model("bm_social_jobs").Where("plan_id", id).Where("kind", "generate").Order("id").All()
		must(t, e)
		if len(jobs) != 2 || jobs[0]["status"].String() != "failed" || jobs[1]["status"].String() != "queued" || gconv.Int(object(jobs[1]["payload"])["version"]) <= state["version"].Int() {
			t.Fatal("retry reused the failed generation job")
		}
		worked, e = ProcessJob(ctx)
		must(t, e)
		if !worked {
			t.Fatal("new generation job was not processed")
		}
		state, e = g.DB().Model("bm_social_plans").Where("id", id).One()
		must(t, e)
		count, e := g.DB().Model("bm_social_variants").Where("plan_id", id).Where("plan_version", state["version"]).Count()
		must(t, e)
		if state["status"].String() != "review" || state["error"].String() != "" || count != 1 {
			t.Fatal("retry did not complete and clear the previous error")
		}
	})
	t.Run("AI versions use source snapshot and scheduling edits keep approval", func(t *testing.T) {
		must(t, PlanControl(ctx, p, plan, "generate"))
		_, e := ProcessJob(ctx)
		must(t, e)
		detail, e := Detail(ctx, p, plan)
		must(t, e)
		variants := gconv.Maps(detail["variants"])
		if len(variants) != 1 {
			t.Fatal("no AI content", encode(detail))
		}
		id := gconv.Int64(variants[0]["id"])
		must(t, ApproveVariant(ctx, p, id))
		c.StartDate = time.Now().AddDate(0, 0, 3).Format("2006-01-02")
		_, e = SavePlan(ctx, p, plan, "Renamed", company, c)
		must(t, e)
		detail, e = Detail(ctx, p, plan)
		must(t, e)
		if len(gconv.Maps(detail["variants"])) != 1 {
			t.Fatal("schedule edit invalidates content")
		}
		must(t, Schedule(ctx, p, plan))
		must(t, Schedule(ctx, p, plan))
		n, e := g.DB().Model("bm_social_deliveries").Where("plan_id", plan).Count()
		must(t, e)
		if n != 1 {
			t.Fatal("duplicate schedule", n)
		}
	})
	var deliveryID int64
	t.Run("worker publishes frozen content once and exposes safe lists", func(t *testing.T) {
		calls := 0
		httpClient = &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
			calls++
			if r.URL.Host != "graph.facebook.com" || !strings.HasSuffix(r.URL.Path, "/123/feed") {
				t.Fatalf("wrong official endpoint %s", r.URL)
			}
			if r.Header.Get("Authorization") != "Bearer fixture-page-token" {
				t.Fatal("token not attached")
			}
			var body g.Map
			must(t, json.NewDecoder(r.Body).Decode(&body))
			if !strings.Contains(gconv.String(body["message"]), "\n") {
				t.Fatal("paragraphs lost")
			}
			return response(200, `{"id":"123_456"}`), nil
		})}
		_, e := g.DB().Model("bm_social_deliveries").Where("plan_id", plan).Data(g.Map{"due_at": time.Now().Add(-time.Minute), "next_at": time.Now().Add(-time.Minute)}).Update()
		must(t, e)
		_, e = ProcessDelivery(ctx)
		must(t, e)
		row, e := g.DB().Model("bm_social_deliveries").Where("plan_id", plan).One()
		must(t, e)
		deliveryID = row["id"].Int64()
		if row["status"].String() != "published" {
			t.Fatal("not published", encode(row))
		}
		_, e = ProcessDelivery(ctx)
		must(t, e)
		if calls != 1 {
			t.Fatal("duplicate publish", calls)
		}
		for _, kind := range []string{"plans", "accounts", "deliveries", "jobs", "interactions"} {
			result, e := List(ctx, p, kind, 0, 1, "", "")
			must(t, e)
			if strings.Contains(encode(result), "fixture-page-token") {
				t.Fatal("token leaked in list")
			}
		}
		_, e = DeliveryDetail(ctx, p, deliveryID)
		must(t, e)
	})
	t.Run("ambiguous network outcomes require explicit confirmation", func(t *testing.T) {
		httpClient = &http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("timeout with private details") })}
		_, e := publish(ctx, "facebook", "fixture", "123", PublishInput{Body: "post"})
		var remote *RemoteError
		if !errors.As(e, &remote) || !remote.Unknown || strings.Contains(e.Error(), "private") {
			t.Fatal("ambiguous result unsafe", e)
		}
		_, e = g.DB().Model("bm_social_deliveries").Where("id", deliveryID).Data(g.Map{"status": "unknown", "remote_id": "", "remote_url": ""}).Update()
		must(t, e)
		if e = DeliveryControl(ctx, p, deliveryID, "retry", g.Map{}); e == nil {
			t.Fatal("blind retry allowed")
		}
	})
	t.Run("image adapters upload actual bytes and reject untrusted upload host", func(t *testing.T) {
		steps := []string{}
		httpClient = &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
			steps = append(steps, r.Method+" "+r.URL.Host+r.URL.Path)
			if r.URL.Host == "graph.facebook.com" {
				must(t, r.ParseMultipartForm(1024))
				if r.FormValue("published") != "false" {
					t.Fatal("image unexpectedly published separately")
				}
				f, _, e := r.FormFile("source")
				must(t, e)
				raw, e := io.ReadAll(f)
				f.Close()
				must(t, e)
				if string(raw) != "image-bytes" {
					t.Fatal("upload bytes lost")
				}
				return response(200, `{"id":"photo-1"}`), nil
			}
			if r.Method == "PUT" {
				if r.URL.Host != "www.linkedin.com" || r.Header.Get("Authorization") != "Bearer fixture" {
					t.Fatal("upload host or auth")
				}
				return response(201, ""), nil
			}
			if r.Method == "POST" {
				return response(200, `{"value":{"image":"urn:li:image:1","uploadUrl":"https://www.linkedin.com/dms-uploads/fixture"}}`), nil
			}
			return response(200, `{"status":"AVAILABLE"}`), nil
		})}
		image := MediaFile{Data: []byte("image-bytes"), Filename: "product.png", MIME: "image/png"}
		id, e := uploadImage(ctx, "facebook", "fixture", "123", image)
		must(t, e)
		if id != "photo-1" {
			t.Fatal("Facebook image")
		}
		_, e = uploadImage(ctx, "linkedin", "fixture", "urn:li:organization:42", image)
		must(t, e)
		if len(steps) != 4 {
			t.Fatal("image upload contract", steps)
		}
		httpClient = &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.Host != "api.linkedin.com" {
				t.Fatal("credential leaked to upload host")
			}
			return response(200, `{"value":{"image":"urn:li:image:2","uploadUrl":"https://evil.example/upload"}}`), nil
		})}
		if _, e = uploadImage(ctx, "linkedin", "fixture", "urn:li:organization:42", image); e == nil {
			t.Fatal("untrusted upload host accepted")
		}
	})
	t.Run("LinkedIn version and post author are explicit", func(t *testing.T) {
		httpClient = &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.Host != "api.linkedin.com" || r.Header.Get("Linkedin-Version") != "202610" {
				t.Fatal("wrong LinkedIn request")
			}
			var body g.Map
			must(t, json.NewDecoder(r.Body).Decode(&body))
			if body["author"] != "urn:li:organization:42" || body["commentary"] != "Professional post" {
				t.Fatal("wrong post payload", body)
			}
			res := response(201, "")
			res.Header.Set("x-restli-id", "urn:li:share:99")
			return res, nil
		})}
		result, e := publish(ctx, "linkedin", "fixture", "urn:li:organization:42", PublishInput{Body: "Professional post"})
		must(t, e)
		if result.ID != "urn:li:share:99" || result.State != "submitted" {
			t.Fatal("submission assumed published")
		}
	})
	t.Run("engagement sync follows cursors on official hosts and stores reported counts", func(t *testing.T) {
		_, e := g.DB().Model("bm_social_deliveries").Where("id", deliveryID).Data(g.Map{"status": "published", "remote_id": "123_456"}).Update()
		must(t, e)
		commentCalls := 0
		httpClient = &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.Host != "graph.facebook.com" {
				t.Fatal("paging leaked token to a third party")
			}
			switch {
			case strings.HasSuffix(r.URL.Path, "/comments"):
				commentCalls++
				if commentCalls == 1 {
					return response(200, `{"data":[{"id":"comment-1","message":"Please quote this part","from":{"id":"member-1","name":"Provided Name"}}],"paging":{"next":"https://evil.example/collect-token","cursors":{"after":"page-two"}}}`), nil
				}
				if r.URL.Query().Get("after") != "page-two" {
					t.Fatal("cursor not used")
				}
				return response(200, `{"data":[]}`), nil
			case strings.HasSuffix(r.URL.Path, "/reactions"):
				return response(200, `{"data":[]}`), nil
			default:
				return response(200, `{"id":"123_456","reactions":{"summary":{"total_count":5}},"comments":{"summary":{"total_count":1}}}`), nil
			}
		})}
		must(t, SyncAccount(ctx, p, accountID))
		if commentCalls != 2 {
			t.Fatal("comment pagination incomplete", commentCalls)
		}
		metrics, e := g.DB().Model("bm_social_metrics").Where("delivery_id", deliveryID).One()
		must(t, e)
		if gconv.Int(object(metrics["counts"])["reactions"]) != 5 || object(metrics["availability"])["shares"] == "available" {
			t.Fatal("missing metrics treated as zero", encode(metrics))
		}
		report, e := Reports(ctx, p, plan)
		must(t, e)
		if len(gconv.Maps(report["rows"])) != 1 {
			t.Fatal("aggregate report missing")
		}
		list, e := List(ctx, p, "interactions", 0, 1, "", "")
		must(t, e)
		if gconv.Int(list["total"]) != 1 {
			t.Fatal("interaction not visible")
		}
	})
	t.Run("engagement is idempotent and AI expires with source", func(t *testing.T) {
		account, e := g.DB().Model("bm_social_accounts").Where("id", accountID).One()
		must(t, e)
		delivery, e := g.DB().Model("bm_social_deliveries").Where("id", deliveryID).One()
		must(t, e)
		event := Interaction{ID: "comment-1", Kind: "comment", ActorID: "member-1", ActorName: "Provided Name", Body: "Please quote this part", Occurred: time.Now()}
		must(t, persistInteraction(ctx, p, account, delivery, event))
		must(t, persistInteraction(ctx, p, account, delivery, event))
		n, e := g.DB().Model("bm_social_interactions").Count()
		must(t, e)
		if n != 1 {
			t.Fatal("duplicate engagement")
		}
		settings, e := config(ctx)
		must(t, e)
		settings.Facebook.ClientSecret = ""
		settings.Facebook.VerifyToken = ""
		settings.LinkedIn.ClientSecret = ""
		settings.AnalysisEnabled = true
		settings.AnalysisProviderApproved = true
		settings.AnalysisModel = ""
		if e = SaveSettings(ctx, p, settings); e == nil {
			t.Fatal("analysis enabled without model")
		}
		settings.AnalysisModel = "fixture-model"
		must(t, SaveSettings(ctx, p, settings))
		_, e = g.DB().Model("bm_options").Data(g.Map{"name": "growth_ai_config", "value": encode(g.Map{"model": "fixture-model"})}).OnConflict("name").Save()
		must(t, e)
		row, e := g.DB().Model("bm_social_interactions").One()
		must(t, e)
		must(t, AnalyzeInteraction(ctx, p, row["id"].Int64()))
		row, e = g.DB().Model("bm_social_interactions").One()
		must(t, e)
		if object(row["analysis"])["topic"] != "quotation" {
			t.Fatal("analysis not stored")
		}
		event.Body = "Updated question"
		must(t, persistInteraction(ctx, p, account, delivery, event))
		row, e = g.DB().Model("bm_social_interactions").One()
		must(t, e)
		if len(object(row["analysis"])) != 0 {
			t.Fatal("stale analysis after edit")
		}
		_, e = g.DB().Model("bm_social_interactions").Where("id", row["id"]).Data("profile_expires_at", time.Now().Add(-time.Hour)).Update()
		must(t, e)
		must(t, PurgeExpired(ctx))
		row, e = g.DB().Model("bm_social_interactions").One()
		must(t, e)
		if row["actor_name"].String() != "" {
			t.Fatal("expired profile retained")
		}
		_, e = g.DB().Model("bm_social_interactions").Where("id", row["id"]).Data("expires_at", time.Now().Add(-time.Hour)).Update()
		must(t, e)
		must(t, PurgeExpired(ctx))
		n, e = g.DB().Model("bm_social_interactions").Count()
		must(t, e)
		if n != 0 {
			t.Fatal("expired content retained")
		}
	})
	t.Run("DST local time and invalid values", func(t *testing.T) {
		m := Market{Country: "US", Language: "en", Timezone: "America/New_York", LocalTime: "09:00"}
		winter, e := marketDue(m, "2027-01-10", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
		must(t, e)
		summer, e := marketDue(m, "2027-07-10", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
		must(t, e)
		if winter.UTC().Hour() != 14 || summer.UTC().Hour() != 13 {
			t.Fatal("DST ignored")
		}
		bad := c
		bad.Markets = []Market{{Country: "US", Language: "en", Timezone: "Bad/Zone", LocalTime: "09:00"}}
		if validateConfig(bad) == nil {
			t.Fatal("invalid timezone")
		}
	})
	t.Run("post images retain order originals and human review", func(t *testing.T) { exercisePostImageWorkflow(t, ctx, p, company) })
	t.Run("run now respects review idempotency and recurrence", func(t *testing.T) { exerciseRunNow(t, ctx, p, company) })
	t.Run("Industry Growth Master acceptance", exerciseMasterNativeSocialHandoff)
	t.Run("management filters draft idempotency and deletion", func(t *testing.T) { exerciseSocialManagement(t, ctx, p, company) })
	t.Run("Facebook post errors do not invalidate a valid Page", func(t *testing.T) { exerciseFacebookSyncIsolation(t, ctx, p, company) })
}
