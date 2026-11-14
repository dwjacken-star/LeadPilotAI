package growth

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"billionmail-core/internal/service/emailvalidation"
	"billionmail-core/internal/service/governance"
	"billionmail-core/internal/service/mail_service"
	"billionmail-core/internal/service/outreachguard"
	"billionmail-core/internal/testsupport"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/xuri/excelize/v2"
)

type fakeIntelligence struct{}

func (fakeIntelligence) Extract(ctx context.Context, p *governance.Principal, purpose string, in AIInput, out interface{}) error {
	values := map[string]string{
		"keywords":               `{"queries":[{"country":"US","language":"en","query":"automotive AC parts distributors USA"}]}`,
		"lead_qualification":     `{"company":"Fixture Importers","country":"US","country_evidence":"USA automotive distributor","language":"en","relevant":true,"confidence":0.97,"evidence":"USA automotive distributor","summary":"Automotive parts importer"}`,
		"outreach_draft":         `{"subject":"A/C parts for Fixture Importers","paragraphs":["Dear Fixture Importers,","We supply automotive A/C parts.","Would you like a catalog?"],"language":"en","reason":"Automotive distributor"}`,
		"inquiry_classification": `{"kind":"inquiry","confidence":0.98,"summary":"Customer asks for a quotation","language":"en","reason":"Direct request"}`,
		"crm_reply_draft":        `{"paragraphs":["Thank you for your inquiry.","Could you share the OE number?"],"missing_facts":["Confirmed price"],"language":"en"}`,
		"material_recognition":   `{"category":"company_profile","summary":"Company evidence","facts":[{"key":"business","value":"Automotive parts","evidence":"Automotive parts","location":"page 1"}],"sku_candidates":[]}`,
	}
	s, ok := values[purpose]
	if !ok {
		return fmt.Errorf("unexpected fake purpose: %s", purpose)
	}
	return json.Unmarshal([]byte(s), out)
}
func (fakeIntelligence) Search(context.Context, *governance.Principal, string, string) ([]SearchSource, error) {
	return []SearchSource{{URL: "https://fixture-business.example/contact", Title: "Fixture importer"}}, nil
}
func must(t *testing.T, e error) {
	t.Helper()
	if e != nil {
		t.Fatal(e)
	}
}

func TestGrowthIntegratedLifecycle(t *testing.T) {
	testsupport.Database(t)
	ctx := context.Background()
	t.Setenv("MAPLEMAIL_GROWTH_DATA", t.TempDir())
	p, e := governance.Load(ctx, 1)
	must(t, e)
	oldAI, oldFetch := IntelligenceProvider, FetchWebsite
	oldSearch, oldCollect := SearchBusinesses, CollectWebsite
	SearchBusinesses = func(ctx context.Context, p *governance.Principal, q string, m Market, limit int, channel string) ([]SearchSource, error) {
		return fakeIntelligence{}.Search(ctx, p, q, m.Country)
	}
	CollectWebsite = func(ctx context.Context, p *governance.Principal, url string) (WebPage, error) {
		return FetchWebsite(ctx, url)
	}
	must(t, SaveSerperSettings(ctx, p, SerperConfig{Enabled: true, APIKey: "fixture-serper-secret", DailySearches: 1000, DailyScrapes: 1000, DailyCredits: 100000}))
	IntelligenceProvider = fakeIntelligence{}
	FetchWebsite = func(context.Context, string) (WebPage, error) {
		return WebPage{URL: "https://fixture-business.example/contact", Text: "USA automotive distributor. buyer@fixture-business.example", Emails: []string{"buyer@fixture-business.example", "skip@fixture-business.example"}}, nil
	}
	t.Cleanup(func() {
		IntelligenceProvider = oldAI
		FetchWebsite = oldFetch
		SearchBusinesses = oldSearch
		CollectWebsite = oldCollect
	})
	companyID, e := SaveCompany(ctx, p, 0, g.Map{"name": "BESTMAPLE", "brand": "MapleMail", "domains": []string{"example.com"}, "profile": g.Map{"business": "Automotive parts", "internal_cost": "secret"}})
	must(t, e)
	must(t, PublishCompany(ctx, p, companyID))
	var productID int64
	t.Run("all worksheets and 3000 rows preserve text codes and manual fields", func(t *testing.T) {
		book := excelize.NewFile()
		book.SetSheetName("Sheet1", "产品")
		book.NewSheet("Additional")
		for sheetIndex, sheet := range []string{"产品", "Additional"} {
			headers := []interface{}{"SKU", "Name", "Category", "OE", "Cost"}
			must(t, book.SetSheetRow(sheet, "A1", &headers))
			for i := 1; i <= 1500; i++ {
				row := []interface{}{fmt.Sprintf("%06d", sheetIndex*1500+i), "A/C Compressor", "Automotive", "0012345", "12.50"}
				must(t, book.SetSheetRow(sheet, fmt.Sprintf("A%d", i+1), &row))
			}
		}
		buf, e := book.WriteToBuffer()
		must(t, e)
		book.Close()
		uploaded, e := Upload(ctx, p, companyID, "products.xlsx", "products", bytes.NewReader(buf.Bytes()), false)
		must(t, e)
		id := uploaded["id"].(string)
		must(t, ingest(ctx, p, id))
		preview, e := ImportRows(ctx, p, id, 2)
		must(t, e)
		if len(preview["list"].(gdb.Result)) != 20 || preview["total"] != 3000 {
			t.Fatal("paged workbook preview invalid", preview["total"])
		}
		n, e := g.DB().Model("bm_growth_products").Where("company_id", companyID).Count()
		must(t, e)
		if n != 3000 {
			t.Fatalf("expected 3000 products, got %d", n)
		}
		r, e := g.DB().Model("bm_growth_products").Where("sku", "000001").One()
		must(t, e)
		productID = r["id"].Int64()
		if jsonMap(r["fields"])["oe"] != "0012345" {
			t.Fatal("lost zero-padded OE", r)
		}
		duplicate, e := Upload(ctx, p, companyID, "products.xlsx", "products", bytes.NewReader(buf.Bytes()), false)
		must(t, e)
		if duplicate["id"] != id {
			t.Fatal("duplicate upload not idempotent")
		}
		must(t, SaveProductTest(ctx, p, productID, companyID))
		asset, e := Asset(ctx, p, id, "material:list")
		must(t, e)
		_, _, e = applyProductRow(ctx, p, asset, "产品", 2, "000001", "Original name", g.Map{"oe": "other"}, false)
		must(t, e)
		r, e = Product(ctx, p, productID, "product:list")
		must(t, e)
		if r["name"].String() != "Manually reviewed compressor" || jsonMap(r["fields"])["oe"] != "0012345" {
			t.Fatal("manual data overwritten", r)
		}
		must(t, PublishProduct(ctx, p, productID, true))
		r, e = Product(ctx, p, productID, "product:list")
		must(t, e)
		if strings.Contains(jsonText(jsonMap(r["published_fields"])), "internal_cost") {
			t.Fatal("cost leaked")
		}
	})
	t.Run("safe image archive and explicit SKU association", func(t *testing.T) {
		var imageData bytes.Buffer
		must(t, png.Encode(&imageData, image.NewRGBA(image.Rect(0, 0, 12, 10))))
		var archive bytes.Buffer
		writer := zip.NewWriter(&archive)
		entry, e := writer.Create("产品/000001_main.png")
		must(t, e)
		_, e = entry.Write(imageData.Bytes())
		must(t, e)
		must(t, writer.Close())
		a, e := saveBlob(ctx, p, companyID, "images.zip", "", "products", bytes.NewReader(archive.Bytes()), false)
		must(t, e)
		must(t, ingest(ctx, p, a["id"].String()))
		n, e := g.DB().Model("bm_growth_product_assets").Where("product_id", productID).Count()
		must(t, e)
		if n != 1 {
			t.Fatal("explicit image not linked", n)
		}
		for _, name := range []string{"../secret.png", "C:/secret.png", "/etc/passwd", "a/../../x.png"} {
			if safeArchiveName(name) {
				t.Fatal("unsafe ZIP path accepted", name)
			}
		}
		if !imageMatchesSKU("SKU-10_main.jpg", "SKU-10") || imageMatchesSKU("SKU-100_main.jpg", "SKU-10") {
			t.Fatal("ambiguous SKU match")
		}
	})
	t.Run("department isolation and published-only sharing", func(t *testing.T) {
		_, e := g.DB().Exec(ctx, `INSERT INTO bm_departments(id,name,active) VALUES(77,'Other',true);INSERT INTO account(username,password,email,status,dept_id) VALUES('reader','test','',1,77)`)
		must(t, e)
		reader := &governance.Principal{ID: 2, DeptID: 77, Roles: []governance.Role{{Scope: "department", Permissions: []string{"company:list", "product:list", "product:export", "material:list", "material:download"}}}}
		if _, e = Product(ctx, reader, productID, "product:list"); e == nil {
			t.Fatal("cross-department product leaked")
		}
		must(t, GrantCompany(ctx, p, companyID, []int64{77}))
		r, e := Product(ctx, reader, productID, "product:list")
		must(t, e)
		if strings.Contains(jsonText(r), "secret") || strings.Contains(jsonText(r), "internal_cost") {
			t.Fatal("internal product values leaked", r)
		}
		list, e := List(ctx, reader, "products", companyID, 1, "", "published")
		must(t, e)
		if list["total"] != 1 {
			t.Fatal("draft products shared", list["total"])
		}
		companies, e := Companies(ctx, reader, "company:list")
		must(t, e)
		if len(companies) != 1 {
			t.Fatal("shared company not available")
		}
		must(t, GrantCompany(ctx, p, companyID, nil))
		if _, e = Product(ctx, reader, productID, "product:list"); e == nil {
			t.Fatal("revoked share still usable")
		}
	})
	var runID, messageID, leadID int64
	t.Run("research to verified frozen personalized campaign without external calls", func(t *testing.T) {
		_, e := g.DB().Exec(ctx, `INSERT INTO mailbox(username,password,password_encode,full_name,maildir,local_part,domain,active) VALUES('sender2@example.com','test','6447567a64413d3d','Test','test2/','sender2','example.com',1)`)
		must(t, e)
		c := PlanConfig{ProductIDs: []int64{productID}, Markets: []Market{{Country: "US", Language: "en", Timezone: "UTC"}}, Requirement: "Find automotive A/C distributors", ExcludeKeywords: []string{"skip"}, Senders: []string{"sender@example.com", "sender2@example.com"}, DailyLeads: 10, DailySend: 10, SearchLimit: 3, PageLimit: 2, Timezone: "UTC", LocalTime: "09:00", SendStart: "00:00", SendEnd: "23:59", CooldownDays: 30, AutoSend: true}
		planID, e := SavePlan(ctx, p, 0, g.Map{"name": "Test market", "company_id": companyID, "config": c})
		must(t, e)
		planReader := &governance.Principal{ID: p.ID, DeptID: p.DeptID, Roles: []governance.Role{{Scope: "department", Permissions: []string{"outreach:list"}}}}
		planList, e := List(ctx, planReader, "plans", 0, 1, "", "")
		must(t, e)
		listed := planList["list"].(gdb.Result)
		if len(listed) != 1 || listed[0]["company_name"].String() != "BESTMAPLE" {
			t.Fatal("plan did not include company name without company:list", listed)
		}
		runID, e = PlanControl(ctx, p, planID, "run")
		must(t, e)
		for i := 0; i < 15; i++ {
			yes, e := RunOne(ctx)
			must(t, e)
			if !yes {
				break
			}
		}
		lead, e := g.DB().Model("bm_growth_leads").Where("plan_id", planID).One()
		must(t, e)
		leadID = lead["id"].Int64()
		if leadID == 0 || lead["email"].String() != "buyer@fixture-business.example" || lead["state"].String() != "verified" {
			jobs, _ := g.DB().Model("bm_growth_jobs").All()
			t.Fatalf("pipeline did not verify: %v jobs=%v", lead, jobs)
		}
		m, e := g.DB().Model("bm_growth_messages").Where("run_id", runID).One()
		must(t, e)
		messageID = m["id"].Int64()
		if m["status"].String() != "queued" || m["task_id"].Int64() == 0 {
			jobs, _ := g.DB().Model("bm_growth_jobs").All()
			t.Fatalf("dispatch not queued: %v jobs=%v", m, jobs)
		}
		subject, body, e := outreachguard.Content(ctx, p, messageID)
		must(t, e)
		if subject != "A/C parts for Fixture Importers" || !strings.Contains(body, "<p>Dear Fixture Importers,</p>") {
			t.Fatal("personalization lost", subject, body)
		}
		limit, e := outreachguard.Limit(ctx, messageID)
		must(t, e)
		if limit.Policy.PerDay != 10 || !strings.HasPrefix(limit.Key, "growth-plan:") {
			t.Fatal("plan limit does not span single-contact tasks", limit)
		}
		n, e := g.DB().Model("recipient_info").Where("outreach_message_id", messageID).Count()
		must(t, e)
		if n != 1 {
			t.Fatal("not single-contact delivery", n)
		}
		must(t, dispatchRun(ctx, p, runID))
		n, e = g.DB().Model("recipient_info").Where("outreach_message_id", messageID).Count()
		must(t, e)
		if n != 1 {
			t.Fatal("duplicate delivery")
		}
		_, e = g.DB().Model("bm_growth_plans").Where("id", planID).Data(g.Map{"active": false}).Update()
		must(t, e)
		if _, _, e = outreachguard.Content(ctx, p, messageID); e == nil {
			t.Fatal("paused plan still sends")
		}
		_, e = g.DB().Model("bm_growth_plans").Where("id", planID).Data(g.Map{"active": true}).Update()
		must(t, e)
	})
	t.Run("catch-all and invalid addresses never become send candidates", func(t *testing.T) {
		_, e := g.DB().Model("bm_growth_leads").Where("id", leadID).Data(g.Map{"state": "unconfirmed"}).Update()
		must(t, e)
		if _, _, e = outreachguard.Content(ctx, p, messageID); e == nil {
			t.Fatal("unconfirmed result with future expiry remained sendable")
		}
		_, e = g.DB().Model("bm_growth_leads").Where("id", leadID).Data(g.Map{"state": "verified"}).Update()
		must(t, e)
		oldProbe := emailvalidation.Default.Probe
		defer func() { emailvalidation.Default.Probe = oldProbe }()
		plan, e := g.DB().Model("bm_growth_runs").Where("id", runID).Value("plan_id")
		must(t, e)
		for _, mode := range []struct{ status, code, state, address string }{{emailvalidation.Unknown, "catch_all", "unconfirmed", "unknown@catchall-fixture.test"}, {emailvalidation.Invalid, "recipient_not_found", "invalid", "bad@invalid-fixture.test"}} {
			emailvalidation.Default.Probe = func(context.Context, emailvalidation.Result, string, string) emailvalidation.Result {
				return emailvalidation.Result{Status: mode.status, Code: mode.code, Stage: "smtp", CheckedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour)}
			}
			id, e := g.DB().Model("bm_growth_leads").Data(g.Map{"plan_id": plan.Int64(), "company_id": companyID, "dept_id": p.DeptID, "owner_id": p.ID, "email": mode.address, "assigned_sender": "sender@example.com"}).InsertAndGetId()
			must(t, e)
			must(t, verifyLead(ctx, p, id, runID))
			r, e := g.DB().Model("bm_growth_leads").Where("id", id).One()
			must(t, e)
			if r["state"].String() != mode.state {
				t.Fatal("uncertain/invalid result handled incorrectly", r)
			}
			n, e := g.DB().Model("bm_growth_messages").Where("lead_id", id).Count()
			must(t, e)
			if n != 0 {
				t.Fatal("uncertain or invalid address drafted", mode.address)
			}
		}
	})
	t.Run("changed materials invalidate the old draft and regeneration preserves its snapshot", func(t *testing.T) {
		must(t, PublishProduct(ctx, p, productID, true))
		if _, _, e = outreachguard.Content(ctx, p, messageID); e == nil {
			t.Fatal("changed product did not invalidate old frozen message")
		}
		oldMessage := messageID
		newRun, e := RegenerateMessage(ctx, p, oldMessage)
		must(t, e)
		same, e := RegenerateMessage(ctx, p, oldMessage)
		must(t, e)
		if same != newRun {
			t.Fatal("regeneration not idempotent")
		}
		for i := 0; i < 8; i++ {
			yes, e := RunOne(ctx)
			must(t, e)
			if !yes {
				break
			}
		}
		m, e := g.DB().Model("bm_growth_messages").Where("run_id", newRun).One()
		must(t, e)
		if m["status"].String() != "queued" || m["id"].Int64() == oldMessage {
			t.Fatal("new immutable draft not queued", m)
		}
		messageID = m["id"].Int64()
		l, e := g.DB().Model("bm_growth_leads").Where("id", leadID).One()
		must(t, e)
		if l["assigned_sender"].String() != "sender2@example.com" {
			t.Fatal("invalid addresses consumed a sender rotation slot", l["assigned_sender"])
		}
		if _, _, e = outreachguard.Content(ctx, p, messageID); e != nil {
			t.Fatal(e)
		}
		old, e := g.DB().Model("bm_growth_messages").Where("id", oldMessage).One()
		must(t, e)
		if old["status"].String() != "cancelled" || old["html"].String() == "" {
			t.Fatal("old snapshot was lost", old)
		}
	})
	t.Run("daily plan cap defers remaining approved messages to the next day", func(t *testing.T) {
		old := FetchWebsite
		FetchWebsite = func(context.Context, string) (WebPage, error) {
			return WebPage{URL: "https://quota-fixture.example/contact", Text: "USA automotive distributor. quota1@quota-fixture.test quota2@quota-fixture.test", Emails: []string{"quota1@quota-fixture.test", "quota2@quota-fixture.test"}}, nil
		}
		defer func() { FetchWebsite = old }()
		c := PlanConfig{ProductIDs: []int64{productID}, Markets: []Market{{Country: "US", Language: "en", Timezone: "UTC"}}, Requirement: "Find parts importers", Senders: []string{"sender@example.com", "sender2@example.com"}, DailyLeads: 10, DailySend: 1, SearchLimit: 1, PageLimit: 1, Timezone: "UTC", LocalTime: "09:00", SendStart: "00:00", SendEnd: "23:59", CooldownDays: 1, AutoSend: true}
		id, e := SavePlan(ctx, p, 0, g.Map{"name": "Quota fixture", "company_id": companyID, "config": c})
		must(t, e)
		run, e := PlanControl(ctx, p, id, "run")
		must(t, e)
		for i := 0; i < 12; i++ {
			yes, e := RunOne(ctx)
			must(t, e)
			if !yes {
				break
			}
		}
		queued, e := g.DB().Model("bm_growth_messages").Where("plan_id", id).Where("status", "queued").Count()
		must(t, e)
		approved, e := g.DB().Model("bm_growth_messages").Where("plan_id", id).Where("status", "approved").Count()
		must(t, e)
		if queued != 1 || approved != 1 {
			jobs, _ := g.DB().Model("bm_growth_jobs").All()
			t.Fatal("daily cap lost remaining messages", queued, approved, jobs)
		}
		future, e := g.DB().Model("bm_growth_jobs").Where("resource_id", fmt.Sprint(run)).Where("kind", "dispatch").Where("status", "queued").Where("next_at>now()").Count()
		must(t, e)
		if future != 1 {
			t.Fatal("no next-day dispatch", future)
		}
	})
	t.Run("inquiry classification pauses outreach and forwards once; AI reply stays a draft", func(t *testing.T) {
		customer, e := g.DB().Model("bm_crm_customers").Data(g.Map{"dept_id": p.DeptID, "owner_id": p.ID, "email": "buyer@fixture-business.example", "name": "Fixture buyer"}).InsertAndGetId()
		must(t, e)
		msg, e := g.DB().Model("bm_crm_messages").Data(g.Map{"customer_id": customer, "dept_id": p.DeptID, "owner_id": p.ID, "mailbox": "sender@example.com", "direction": "incoming", "from_address": "buyer@fixture-business.example", "to_address": "sender@example.com", "subject": "Quotation request", "body_text": "Please quote 100 A/C compressors.", "body_loaded": true, "status": "received"}).InsertAndGetId()
		must(t, e)
		ruleID, e := SaveInboxRule(ctx, p, 0, g.Map{"company_id": companyID, "mailbox": "sender@example.com", "forward_to": "sales@example.com", "active": true})
		must(t, e)
		oldLoad, oldRaw, oldForward := LoadInboxMessage, LoadRawMessage, ForwardMail
		LoadInboxMessage = func(ctx context.Context, p *governance.Principal, id int64) (gdb.Record, error) {
			return g.DB().Model("bm_crm_messages").Where("id", id).One()
		}
		LoadRawMessage = func(context.Context, *governance.Principal, int64) ([]byte, error) {
			return []byte("Subject: Original\r\n\r\nOriginal MIME"), nil
		}
		sent := 0
		ForwardMail = func(ctx context.Context, sender, to string, m mail_service.Message) error {
			sent++
			if to != "sales@example.com" || len(m.Attachments) != 1 || m.Attachments[0].MIME != "message/rfc822" {
				t.Fatal("original forwarding lost", m)
			}
			return nil
		}
		defer func() { LoadInboxMessage, LoadRawMessage, ForwardMail = oldLoad, oldRaw, oldForward }()
		must(t, classifyMessage(ctx, p, msg, ruleID))
		inquiry, e := g.DB().Model("bm_growth_inquiries").Where("message_id", msg).One()
		must(t, e)
		if inquiry["kind"].String() != "inquiry" {
			t.Fatal(inquiry)
		}
		must(t, forwardInquiry(ctx, p, inquiry["id"].Int64()))
		must(t, forwardInquiry(ctx, p, inquiry["id"].Int64()))
		if sent != 1 {
			t.Fatal("duplicate forward", sent)
		}
		if _, _, e = outreachguard.Content(ctx, p, messageID); e == nil {
			t.Fatal("replied customer still receives outreach")
		}
		l, e := g.DB().Model("bm_growth_leads").Where("id", leadID).One()
		must(t, e)
		if l["replied_at"].IsNil() {
			t.Fatal("reply did not stop sequence")
		}
		draft, e := ReplyDraft(ctx, p, msg, companyID)
		must(t, e)
		if draft["requires_confirmation"] != true || sent != 1 {
			t.Fatal("AI draft triggered send", draft)
		}
		if hardReplyKind(g.Map{"auto-submitted": "auto-replied"}, "buyer@example.com", "Automatic reply", "Away") != "automated" {
			t.Fatal("auto reply not excluded")
		}
		if hardReplyKind(g.Map{}, "mailer-daemon@example.com", "Failed", "bad address") != "bounce" {
			t.Fatal("bounce not detected")
		}
		if hardReplyKind(g.Map{}, "buyer@example.com", "Stop", "Please unsubscribe") != "unsubscribe" {
			t.Fatal("unsubscribe not detected")
		}
	})
	t.Run("Responses contract, incomplete responses, encryption and global usage budget", func(t *testing.T) {
		var incomplete atomic.Bool
		api := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet && r.URL.Path == "/models" {
				if r.Header.Get("Authorization") != "Bearer local-test-secret" {
					t.Error("saved discovery credential missing")
				}
				io.WriteString(w, `{"data":[{"id":"fixture-model"}]}`)
				return
			}
			var body g.Map
			must(t, json.NewDecoder(r.Body).Decode(&body))
			if r.URL.Path != "/responses" || r.Header.Get("Authorization") != "Bearer local-test-secret" || body["store"] != false {
				t.Error("invalid Responses contract", body)
			}
			if incomplete.Load() {
				io.WriteString(w, `{"status":"incomplete","output":[]}`)
				return
			}
			io.WriteString(w, `{"status":"completed","usage":{"input_tokens":12,"output_tokens":8},"output":[{"type":"message","content":[{"type":"output_text","text":"{\"value\":\"ok\"}"}]}]}`)
		}))
		defer api.Close()
		oldClient := providerClient
		providerClient = api.Client()
		defer func() { providerClient = oldClient }()
		must(t, SaveSettings(ctx, p, AIConfig{Enabled: true, BaseURL: api.URL, Model: "fixture-model", APIKey: "local-test-secret", Search: true, DailyCalls: 1, DailyTokens: 50000, MaxOutput: 256}))
		models, e := Models(ctx, p, api.URL, "")
		must(t, e)
		if len(models) != 1 || models[0].Value != "fixture-model" {
			t.Fatal("model discovery with saved key failed", models)
		}
		if _, e = Models(ctx, p, "https://different.example/v1", ""); e == nil {
			t.Fatal("saved secret forwarded to a new provider")
		}
		if e = SaveSettings(ctx, p, AIConfig{BaseURL: "https://different.example/v1", DailyCalls: 1, DailyTokens: 50000, MaxOutput: 256}); e == nil {
			t.Fatal("saved secret reused after provider URL change")
		}
		settings, e := Settings(ctx, p)
		must(t, e)
		if strings.Contains(jsonText(settings), "local-test-secret") {
			t.Fatal("key exposed")
		}
		raw, e := g.DB().Model("bm_options").Where("name", "growth_ai_config").Value("value")
		must(t, e)
		if strings.Contains(raw.String(), "local-test-secret") {
			t.Fatal("unencrypted key persisted")
		}
		var out struct {
			Value string `json:"value"`
		}
		e = (&responsesProvider{}).Extract(ctx, p, "test", AIInput{Instruction: "test", Data: g.Map{}, Schema: schema(g.Map{"value": strSchema()})}, &out)
		must(t, e)
		if out.Value != "ok" {
			t.Fatal(out)
		}
		if e = (&responsesProvider{}).Extract(ctx, p, "test", AIInput{}, &out); e == nil || !strings.Contains(e.Error(), "budget") {
			t.Fatal("budget not enforced", e)
		}
		must(t, SaveSettings(ctx, p, AIConfig{Enabled: true, BaseURL: api.URL, Model: "fixture-model", Search: true, DailyCalls: 3, DailyTokens: 50000, MaxOutput: 256}))
		incomplete.Store(true)
		if e = (&responsesProvider{}).Extract(ctx, p, "test", AIInput{}, &out); e == nil || !strings.Contains(e.Error(), "incomplete") {
			t.Fatal("incomplete AI response was accepted", e)
		}
	})
	t.Run("plan delivery options reach actual campaigns and warmup associations", func(t *testing.T) { exercisePlanDeliveryOptions(t, ctx, p, companyID, productID) })
	t.Run("every management list and detail query executes with pagination", func(t *testing.T) {
		for _, kind := range []string{"products", "assets", "jobs", "plans", "runs", "leads", "messages", "inquiries", "inbox_rules"} {
			page, e := List(ctx, p, kind, companyID, 1, "", "")
			must(t, e)
			if page["list"] == nil {
				t.Fatal("null list", kind)
			}
		}
		if _, e := Stats(ctx, p); e != nil {
			t.Fatal(e)
		}
	})
	t.Run("upload templates round trip without importing instruction examples", exerciseUploadTemplateRoundTrip)
	t.Run("optional products persist, research, draft and reach guarded delivery", func(t *testing.T) { exerciseOptionalProducts(t, ctx, p) })
	t.Run("Serper search scrape evidence budgets and company candidates", func(t *testing.T) { exerciseSerperPipeline(t, ctx, p) })
	t.Run("campaign authoring selected model budget and no automatic send", func(t *testing.T) { exerciseCampaignAuthor(t, ctx, p) })
	t.Run("social image discovery multipart edit usage and private errors", func(t *testing.T) { exerciseSocialImageProvider(t, ctx, p) })
	t.Run("Industry Growth Master acceptance", exerciseMasterIndustryNeutralLifecycle)
	t.Run("text provider routing and record deletion", exerciseTextProviders)
 t.Run("central registry platform routing",func(t *testing.T){exerciseRegistryRoutesSelectedPlatformAndModel(t,ctx,p)})
}

func TestPublicCollectionBoundaries(t *testing.T) {
	for _, value := range []string{"null", "{}", "invalid"} {
		m := jsonMap(g.NewVar(value))
		m["test"] = "safe"
		if m["test"] != "safe" {
			t.Fatal("null metadata is not editable")
		}
	}
	for _, u := range []string{"http://127.0.0.1/", "http://169.254.169.254/", "http://100.100.100.200/", "http://10.0.0.1/", "http://[::1]/", "file:///etc/passwd", "https://user:secret@example.com/", "https://example.com:22/"} {
		if _, e := safeURL(u); e == nil {
			t.Fatal("unsafe URL accepted", u)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, _, e := fetchPublic(ctx, "http://localhost/"); e == nil {
		t.Fatal("private DNS address was fetched")
	}
	if robotsAllows("User-agent: *\nDisallow: /private", "/private/contact") {
		t.Fatal("robots exclusion ignored")
	}
	if !robotsAllows("User-agent: *\nDisallow: /private", "/contact") {
		t.Fatal("public contact page blocked")
	}
}
func SaveProductTest(ctx context.Context, p *governance.Principal, id, companyID int64) error {
	_, e := SaveProduct(ctx, p, id, g.Map{"company_id": companyID, "sku": "000001", "name": "Manually reviewed compressor", "category": "Automotive", "fields": g.Map{"oe": "0012345", "internal_cost": "12.50"}})
	return e
}
