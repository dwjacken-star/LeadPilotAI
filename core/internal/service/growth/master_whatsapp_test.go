package growth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"strings"
	"testing"

	"billionmail-core/internal/service/governance"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/gconv"
)

type masterWAEventTransport func(*http.Request) (*http.Response, error)

func (f masterWAEventTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func exerciseMasterWhatsAppTemplateAndSignedEvents(t *testing.T) {
	ctx := context.Background()
	t.Setenv("MAPLEMAIL_GROWTH_DATA", t.TempDir())
	p, e := governance.Load(ctx, 1)
	must(t, e)
	must(t, MasterSeed(ctx))
	cid, e := SaveCompany(ctx, p, 0, g.Map{"name": "WhatsApp fixture", "profile": g.Map{"business": "Business services"}})
	must(t, e)
	must(t, PublishCompany(ctx, p, cid))
	template := MasterWATemplate{Name: "fixture_intro", Language: "en_US", MarketLanguage: "en", Body: "Hello. {{1}}", Variables: []string{"body"}}
	must(t, masterSaveWA(ctx, p, cid, g.Map{"enabled": true, "phone_number_id": "1234567890", "api_version": "v24.0", "token": "fixture-access-token", "app_secret": "fixture-app-secret", "verify_token": "fixture-verify-token", "templates": []MasterWATemplate{template}}))
	masked, e := masterWASettings(ctx, p, cid)
	must(t, e)
	if strings.Contains(jsonText(masked), "fixture-access-token") || !gconv.Bool(masked["has_token"]) {
		t.Fatal("credential masking failed")
	}
	prospect, e := g.DB().Model("bm_gm_prospects").Data(g.Map{"company_id": cid, "dept_id": p.DeptID, "owner_id": p.ID, "company": "Opted-in business", "website": "https://wa-fixture.example", "email": "buyer@wa-fixture.example", "phone": "+12025550101", "country": "US", "language": "en", "state": "verified", "score": 90, "contact_permission": "allowed", "permission_evidence": "Fixture explicit opt-in", "wa_opt_in": true}).InsertAndGetId()
	must(t, e)
	pack, e := g.DB().Model("bm_gm_packs").Where("slug", "general").Value("id")
	must(t, e)
	c := MasterDefaultConfig("WF04")
	c.Requirement = "Approved fixture introduction"
	c.Cadence = "manual"
	c.Channels = []string{"whatsapp"}
	c.ProspectIDs = []int64{prospect}
	c.Budget.Tokens = 500000
	workflow, e := masterSaveWorkflow(ctx, p, 0, g.Map{"company_id": cid, "pack_id": pack.Int64(), "name": "WhatsApp fixture", "code": "WF04", "config": c})
	must(t, e)
	oldAI, oldHTTP := IntelligenceProvider, masterWAHTTP
	IntelligenceProvider = &masterFixtureAI{}
	calls := 0
	masterWAHTTP = &http.Client{Transport: masterWAEventTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Host != "graph.facebook.com" || !strings.HasSuffix(r.URL.Path, "/1234567890/messages") {
			t.Fatal("unexpected official host/path", r.URL)
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"type":"template"`) || !strings.Contains(string(body), "fixture_intro") || r.Header.Get("Authorization") != "Bearer fixture-access-token" {
			t.Fatal("invalid template request")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"messages":[{"id":"fixture-wa-id"}]}`))}, nil
	})}
	t.Cleanup(func() { IntelligenceProvider, masterWAHTTP = oldAI, oldHTTP })
	run, e := MasterWorkflowControl(ctx, p, workflow, "run")
	must(t, e)
	for i := 0; i < 5; i++ {
		_, e = MasterRunOne(ctx)
		must(t, e)
	}
	if calls != 0 {
		t.Fatal("template sent before approval")
	}
	content, e := g.DB().Model("bm_gm_contents").Where("run_id", run).One()
	must(t, e)
	if !strings.HasPrefix(content["body"].String(), "Hello. ") {
		t.Fatal("preview did not render actual template")
	}
	if e = masterContentControl(ctx, p, content["id"].Int64(), "save", g.Map{"subject": "changed", "body": "unapproved template"}); e == nil {
		t.Fatal("template preview and payload could diverge")
	}
	must(t, masterContentControl(ctx, p, content["id"].Int64(), "approve", g.Map{}))
	for i := 0; i < 4; i++ {
		_, e = MasterRunOne(ctx)
		must(t, e)
	}
	if calls != 1 {
		t.Fatal("template submission count", calls)
	}
	result, status := MasterWAWebhook(ctx, cid, "subscribe", "fixture-verify-token", "12345", "", nil)
	if status != 200 || result != "12345" {
		t.Fatal("verification challenge failed")
	}
	_, status = MasterWAWebhook(ctx, cid, "subscribe", "wrong", "12345", "", nil)
	if status != 403 {
		t.Fatal("invalid verification accepted")
	}
	payload := []byte(`{"object":"whatsapp_business_account","entry":[{"changes":[{"value":{"metadata":{"phone_number_id":"1234567890"},"messages":[{"id":"incoming-fixture","from":"12025550101","type":"text","text":{"body":"unsubscribe"}}]}}]}]}`)
	_, status = MasterWAWebhook(ctx, cid, "", "", "", "sha256=wrong", payload)
	if status != 403 {
		t.Fatal("unsigned payload accepted")
	}
	mac := hmac.New(sha256.New, []byte("fixture-app-secret"))
	mac.Write(payload)
	signature := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	for i := 0; i < 2; i++ {
		_, status = MasterWAWebhook(ctx, cid, "", "", "", signature, payload)
		if status != 200 {
			t.Fatal("signed webhook failed", status)
		}
	}
	count, e := g.DB().Model("bm_gm_events").Where("event_key", "whatsapp:1234567890:incoming-fixture").Count()
	must(t, e)
	if count != 1 {
		t.Fatal("replayed webhook duplicated inquiry")
	}
	r, e := g.DB().Model("bm_gm_prospects").Where("id", prospect).One()
	must(t, e)
	if !r["suppressed"].Bool() || r["wa_opt_in"].Bool() {
		t.Fatal("opt-out did not immediately block further sends")
	}
}
