package growth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"billionmail-core/internal/service/emailvalidation"
	"billionmail-core/internal/service/governance"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/gconv"
	"github.com/xuri/excelize/v2"
)

type masterFixtureAI struct{ Calls atomic.Int64 }

func (f *masterFixtureAI) Extract(ctx context.Context, p *governance.Principal, purpose string, in AIInput, out interface{}) error {
	f.Calls.Add(1)
	raw, _ := json.Marshal(in.Data)
	data := g.Map{}
	_ = json.Unmarshal(raw, &data)
	var result interface{}
	switch purpose {
	case "master_MASTER":
		strategies := []g.Map{}
		for _, m := range gconv.Maps(data["markets"]) {
			strategies = append(strategies, g.Map{"country": m["country"], "language": m["language"], "focus": "Public B2B procurement", "buyer_types": []string{"business buyer"}, "search_queries": 1, "priority": "normal", "reason": "Approved business requirement"})
		}
		result = g.Map{"goal": "Verified business growth", "markets": strategies, "risks": []string{"Evidence uncertainty"}, "human_checkpoints": []string{"First contact and publication"}}
	case "master_S02", "master_S03", "master_S04":
		rows := gconv.Maps(data["evidence"])
		findings := []g.Map{}
		if len(rows) > 0 {
			findings = append(findings, g.Map{"title": "Observed public demand", "summary": "A documented sample signal", "evidence_ids": []int64{gconv.Int64(rows[0]["id"])}, "excerpt": "public demand", "confidence": .95, "uncertainty": "Sample coverage only"})
		}
		result = g.Map{"findings": findings, "scope": "Configured public sample"}
	case "master_S05":
		result = g.Map{"queries": []g.Map{{"country": "US", "language": "en", "query": "verified business buyer United States"}}}
	case "master_S06":
		result = g.Map{"company": "Fixture Buyers", "country": "US", "country_evidence": "United States", "language": "en", "summary": "Professional business buyer", "relevant": true, "confidence": .96, "phone": "+12025550100", "need": "Supplier quotation", "need_evidence": "request for quotation"}
	case "master_S09":
		ids := []int64{}
		for _, v := range gconv.Maps(data["approved_offerings"]) {
			ids = append(ids, gconv.Int64(v["id"]))
		}
		result = g.Map{"product_ids": ids, "reason": "Approved supply scope matches published requirement", "missing_conditions": []string{"Confirm final specification"}, "confidence": .9}
	case "master_S10":
		result = g.Map{"subject": "Business introduction", "paragraphs": []string{"Dear purchasing team,", "We can discuss our approved business offering.", "Could you share your requirements?"}, "language": data["language"]}
	case "master_S12":
		result = g.Map{"kind": "inquiry", "confidence": .98, "summary": "A customer requests a quotation", "language": "en", "needs_human": true, "paragraphs": []string{"Thank you for your inquiry.", "Please confirm your requirements so our team can prepare a quotation."}}
	case "master_S13":
		result = g.Map{"recommendations": []string{"Review unconfirmed addresses without automatically contacting them"}}
	default:
		return fmt.Errorf("unexpected master purpose: %s", purpose)
	}
	encoded, _ := json.Marshal(result)
	return json.Unmarshal(encoded, out)
}
func exerciseMasterIndustryNeutralLifecycle(t *testing.T) {
	ctx := context.Background()
	t.Setenv("MAPLEMAIL_GROWTH_DATA", t.TempDir())
	p, e := governance.Load(ctx, 1)
	must(t, e)
	must(t, MasterSeed(ctx))
	ai := &masterFixtureAI{}
	oldAI, oldSearch, oldPage, oldVerify := IntelligenceProvider, MasterSearchSources, CollectWebsite, MasterVerifyEmail
	IntelligenceProvider = ai
	var searches, probes atomic.Int64
	MasterSearchSources = func(ctx context.Context, p *governance.Principal, q string, m Market, n int, news bool) ([]SearchSource, error) {
		searches.Add(1)
		return []SearchSource{{URL: "https://fixture-buyer.example", Title: "Fixture Buyers public demand", Description: "public demand: request for quotation in the United States", PublishedAt: "2026-11-10", Kind: "search"}}, nil
	}
	CollectWebsite = func(ctx context.Context, p *governance.Principal, u string) (WebPage, error) {
		return WebPage{URL: u, Text: "United States professional business buyer. public demand request for quotation. +12025550100. buyer@fixture-buyer.example invalid@fixture-buyer.example catch@fixture-buyer.example filter@fixture-buyer.example", Emails: []string{"buyer@fixture-buyer.example", "invalid@fixture-buyer.example", "catch@fixture-buyer.example", "filter@fixture-buyer.example"}}, nil
	}
	MasterVerifyEmail = func(ctx context.Context, email, sender string, deep bool) emailvalidation.Result {
		probes.Add(1)
		s := emailvalidation.Valid
		code := "smtp_accepted"
		if strings.HasPrefix(email, "invalid") {
			s = emailvalidation.Invalid
			code = "mailbox_not_found"
		}
		if strings.HasPrefix(email, "catch") {
			s = emailvalidation.Unknown
			code = "catch_all"
		}
		return emailvalidation.Result{Status: s, Code: code, Stage: "smtp", CheckedAt: time.Now(), ExpiresAt: time.Now().Add(24 * time.Hour)}
	}
	t.Cleanup(func() {
		IntelligenceProvider, MasterSearchSources, CollectWebsite, MasterVerifyEmail = oldAI, oldSearch, oldPage, oldVerify
	})
	cid, e := SaveCompany(ctx, p, 0, g.Map{"name": "Generic business fixture", "profile": g.Map{"business": "Approved products and professional services"}})
	must(t, e)
	must(t, PublishCompany(ctx, p, cid))
	packRow, e := g.DB().Model("bm_gm_packs").Where("slug", "business_services").One()
	must(t, e)
	packID := packRow["id"].Int64()
	var pack MasterPack
	must(t, json.Unmarshal(packRow["config"].Bytes(), &pack))
	customID, e := masterSavePack(ctx, p, packID, g.Map{"name": "Fixture services", "slug": "fixture_services", "config": pack})
	must(t, e)
	product, e := SaveProduct(ctx, p, 0, g.Map{"company_id": cid, "industry_pack_id": customID, "sku": "SVC-0001", "name": "Professional support", "fields": g.Map{"service_scope": "Business support", "coverage": []string{"US"}, "delivery_days": 10}})
	must(t, e)
	must(t, PublishProduct(ctx, p, product, true))
	if _, e = SaveProduct(ctx, p, 0, g.Map{"company_id": cid, "industry_pack_id": customID, "sku": "BAD", "name": "Bad service", "fields": g.Map{}}); e == nil {
		t.Fatal("required industry field was not validated")
	}
	runUntil := func(t *testing.T, id int64, wanted ...string) g.Map {
		t.Helper()
		for i := 0; i < 25; i++ {
			r, e := g.DB().Model("bm_gm_runs").Where("id", id).One()
			must(t, e)
			for _, s := range wanted {
				if r["status"].String() == s {
					return gconv.Map(r)
				}
			}
			if r["status"].String() == "failed" || r["status"].String() == "configuration_required" || r["status"].String() == "budget_exhausted" {
				t.Fatalf("run stopped unexpectedly: %s: %s", r["status"].String(), r["error"].String())
			}
			_, e = MasterRunOne(ctx)
			must(t, e)
		}
		last, _ := g.DB().Model("bm_gm_runs").Where("id", id).One()
		t.Fatal("run did not reach requested state", last["status"].String(), last["step_index"].Int(), last["error"].String())
		return nil
	}
	makeWorkflow := func(t *testing.T, code string, extra func(*MasterConfig)) int64 {
		t.Helper()
		c := MasterDefaultConfig(code)
		c.Requirement = "Find public business buyers for approved services"
		c.ProductIDs = []int64{product}
		c.Senders = []string{"sender@example.com"}
		c.ExcludeKeywords = []string{"filter@"}
		c.Cadence = "manual"
		c.Budget.Tokens = 1000000
		if extra != nil {
			extra(&c)
		}
		id, e := masterSaveWorkflow(ctx, p, 0, g.Map{"company_id": cid, "pack_id": customID, "name": "Fixture " + code, "code": code, "config": c})
		must(t, e)
		return id
	}
	var prospectID int64
	t.Run("WF03 discovery verification scoring matching CRM and source cache", func(t *testing.T) {
		w := makeWorkflow(t, "WF03", nil)
		id, e := MasterWorkflowControl(ctx, p, w, "run")
		must(t, e)
		same, e := MasterWorkflowControl(ctx, p, w, "run")
		must(t, e)
		if id != same {
			t.Fatal("concurrent/manual action did not reuse active run")
		}
		runUntil(t, id, "completed")
		rows, e := g.DB().Model("bm_gm_prospects").Where("company_id", cid).WhereNot("email", "").All()
		must(t, e)
		if len(rows) != 4 {
			t.Fatal("public emails missing", len(rows))
		}
		for _, r := range rows {
			switch r["email"].String() {
			case "buyer@fixture-buyer.example":
				if r["state"].String() != "verified" || r["crm_id"].Int64() == 0 || r["score"].Float64() < 60 {
					t.Fatal("verified prospect not synchronized", r)
				}
				prospectID = r["id"].Int64()
				if !strings.Contains(r["matching"].String(), fmt.Sprint(product)) {
					t.Fatal("approved offering not matched")
				}
			case "invalid@fixture-buyer.example":
				if r["state"].String() != "invalid" {
					t.Fatal("invalid imported")
				}
			case "catch@fixture-buyer.example":
				if r["state"].String() != "unconfirmed" || r["crm_id"].Int64() > 0 {
					t.Fatal("catch-all was imported")
				}
			case "filter@fixture-buyer.example":
				if r["state"].String() != "filtered" {
					t.Fatal("keyword filter ignored")
				}
			}
		}
		if probes.Load() != 3 {
			t.Fatal("filtered mailbox was probed", probes.Load())
		}
		if searches.Load() != 1 {
			t.Fatal("unexpected search count")
		}
	})
	t.Run("WF01 and WF02 evidence findings and market-neutral templates", func(t *testing.T) {
		for _, code := range []string{"WF01", "WF02"} {
			w := makeWorkflow(t, code, nil)
			id, e := MasterWorkflowControl(ctx, p, w, "run")
			must(t, e)
			runUntil(t, id, "completed")
		}
		if searches.Load() > 3 {
			t.Fatal("search cache not reused")
		}
	})
	t.Run("WF04 pauses for exact approval and queues one native email", func(t *testing.T) {
		w := makeWorkflow(t, "WF04", func(c *MasterConfig) {
			c.Channels = []string{"email"}
			c.ProspectIDs = []int64{prospectID}
			c.Policy.MinIntervalMS = 60000
		})
		id, e := MasterWorkflowControl(ctx, p, w, "run")
		must(t, e)
		runUntil(t, id, "waiting_approval")
		content, e := g.DB().Model("bm_gm_contents").Where("run_id", id).One()
		must(t, e)
		if e = masterContentControl(ctx, p, content["id"].Int64(), "approve", g.Map{}); e == nil {
			t.Fatal("contact without documented permission was approved")
		}
		must(t, masterProspectControl(ctx, p, prospectID, g.Map{"contact_permission": "allowed", "permission_evidence": "Fixture approved B2B outreach policy", "wa_opt_in": false}))
		must(t, masterContentControl(ctx, p, content["id"].Int64(), "approve", g.Map{}))
		before := probes.Load()
		runUntil(t, id, "completed")
		if probes.Load() != before {
			t.Fatal("sending stage repeated SMTP verification")
		}
		current, e := g.DB().Model("bm_gm_contents").Where("id", content["id"]).One()
		must(t, e)
		if current["task_id"].Int64() == 0 || current["status"].String() != "queued" {
			t.Fatal("native email task not queued", current)
		}
		must(t, MasterDeliveryGuard(ctx, p, current["id"].Int64()))
		task, e := g.DB().Model("email_tasks").Where("id", current["task_id"]).One()
		must(t, e)
		if task["unsubscribe"].Int() != 1 || !strings.Contains(task["snapshot_content"].String(), "<p>Dear purchasing team,") {
			t.Fatal("delivery options or rich text were lost")
		}
		if e = masterContentControl(ctx, p, current["id"].Int64(), "approve", g.Map{}); e == nil {
			t.Fatal("already queued content could be approved again")
		}
		must(t, masterProspectControl(ctx, p, prospectID, g.Map{"contact_permission": "blocked", "permission_evidence": "Stop contacting"}))
		if e = MasterDeliveryGuard(ctx, p, current["id"].Int64()); e == nil {
			t.Fatal("revoked permission passed actual SMTP guard")
		}
		_, e = g.DB().Model("recipient_info").Where("master_content_id", current["id"]).Data(g.Map{"delivery_state": "skipped", "last_error": "permission revoked"}).Update()
		must(t, e)
		must(t, masterReconcile(ctx))
		latest, e := g.DB().Model("bm_gm_contents").Where("id", current["id"]).One()
		must(t, e)
		if latest["status"].String() != "skipped" {
			t.Fatal("skipped native delivery remained queued")
		}
		_, e = g.DB().Model("bm_gm_contents").Where("id", current["id"]).Data("status", "cancelled").Update()
		must(t, e)
	})
	t.Run("WF06 reports measured results without invented ROI", func(t *testing.T) {
		w := makeWorkflow(t, "WF06", nil)
		id, e := MasterWorkflowControl(ctx, p, w, "run")
		must(t, e)
		runUntil(t, id, "completed")
		r, e := g.DB().Model("bm_gm_steps").Where("run_id", id).Where("skill", "S13").One()
		must(t, e)
		if gconv.Map(jsonMap(r["output"]))["roi"] != nil {
			t.Fatal("invented ROI")
		}
	})
	t.Run("WF05 inbound inquiry creates one reviewed draft and follow-up", func(t *testing.T) {
		w := makeWorkflow(t, "WF05", nil)
		workflow, e := g.DB().Model("bm_gm_workflows").Where("id", w).One()
		must(t, e)
		customer, e := g.DB().Model("bm_crm_customers").Where("email", "buyer@fixture-buyer.example").Value("id")
		must(t, e)
		event, e := g.DB().Model("bm_gm_events").Data(g.Map{"company_id": cid, "dept_id": p.DeptID, "owner_id": p.ID, "channel": "email", "event_key": "fixture-inquiry", "sender": "buyer@fixture-buyer.example", "body": "Please send a quotation", "kind": "inquiry", "confidence": .98, "payload": jsonText(g.Map{"customer_id": customer.Int64()})}).InsertAndGetId()
		must(t, e)
		id, e := masterNewRun(ctx, p, workflow, "event:fixture", event)
		must(t, e)
		runUntil(t, id, "completed")
		r, e := g.DB().Model("bm_gm_events").Where("id", event).One()
		must(t, e)
		if r["status"].String() != "processed" || !strings.Contains(r["analysis"].String(), "reply_draft_html") {
			t.Fatal("inquiry draft missing")
		}
		n, e := g.DB().Model("bm_crm_followups").Where("customer_id", customer.Int64()).Count()
		must(t, e)
		if n != 1 {
			t.Fatal("follow-up missing or duplicated", n)
		}
	})
	t.Run("daily budget atomic claims and explicit resume", func(t *testing.T) {
		w := makeWorkflow(t, "WF03", func(c *MasterConfig) { c.Budget.Calls = 1 })
		id, e := MasterWorkflowControl(ctx, p, w, "run")
		must(t, e)
		for i := 0; i < 4; i++ {
			_, e = MasterRunOne(ctx)
			must(t, e)
		}
		r, e := g.DB().Model("bm_gm_runs").Where("id", id).One()
		must(t, e)
		if r["status"].String() != "budget_exhausted" {
			t.Fatal("daily cap was not enforced", r["status"].String())
		}
		must(t, masterRunControl(ctx, p, id, "cancel"))
	})
	t.Run("department scope and read-only roles cannot mutate", func(t *testing.T) {
		viewer := &governance.Principal{ID: p.ID, DeptID: p.DeptID, Roles: []governance.Role{{Scope: "all", Permissions: []string{"intelligence:view"}}}}
		if _, e = masterSavePack(ctx, viewer, 0, g.Map{}); e == nil {
			t.Fatal("viewer changed industry configuration")
		}
		outside := &governance.Principal{ID: 999, DeptID: 999, Roles: []governance.Role{{Scope: "department", Permissions: []string{"intelligence:view", "intelligence:manage"}}}}
		if _, e = masterRecord(ctx, outside, "bm_gm_prospects", prospectID, "intelligence:view"); e == nil {
			t.Fatal("cross department prospect leaked")
		}
		if _, e = masterOptions(ctx, outside, cid, "prospects", 1, "", nil); e == nil {
			t.Fatal("cross department options leaked")
		}
	})
	t.Run("concurrent run actions create one batch", func(t *testing.T) {
		w := makeWorkflow(t, "WF01", nil)
		var group sync.WaitGroup
		ids := make(chan int64, 5)
		errs := make(chan error, 5)
		for i := 0; i < 5; i++ {
			group.Add(1)
			go func() { defer group.Done(); id, e := MasterWorkflowControl(ctx, p, w, "run"); ids <- id; errs <- e }()
		}
		group.Wait()
		close(ids)
		close(errs)
		for e := range errs {
			must(t, e)
		}
		first := int64(0)
		for id := range ids {
			if first == 0 {
				first = id
			}
			if first != id {
				t.Fatal("duplicate run")
			}
		}
		must(t, masterRunControl(ctx, p, first, "cancel"))
	})
	t.Run("typed XLSX preview confirms only valid new services", func(t *testing.T) {
		book := excelize.NewFile()
		header := []interface{}{"SKU", "Name", "Category", "service_scope", "coverage", "delivery_days"}
		must(t, book.SetSheetRow("Sheet1", "A1", &header))
		for i, row := range [][]interface{}{{"000001", "Service one", "Services", "Support", "US,CA", "5"}, {"000002", "Missing scope", "Services", "", "US", "abc"}, {"000001", "Duplicate", "Services", "Support", "US", "2"}} {
			must(t, book.SetSheetRow("Sheet1", fmt.Sprintf("A%d", i+2), &row))
		}
		buffer, e := book.WriteToBuffer()
		must(t, e)
		book.Close()
		preview, e := MasterPreviewWorkbook(ctx, p, cid, customID, "services.xlsx", bytes.NewReader(buffer.Bytes()))
		must(t, e)
		if gconv.Int(preview["valid"]) != 1 || gconv.Int(preview["invalid"]) != 1 || gconv.Int(preview["duplicate"]) != 1 {
			t.Fatal("invalid preview counts", preview)
		}
		result, e := masterConfirmImport(ctx, p, gconv.String(preview["token"]))
		must(t, e)
		if gconv.Int(result["imported"]) != 1 {
			t.Fatal("invalid import count")
		}
		again, e := masterConfirmImport(ctx, p, gconv.String(preview["token"]))
		must(t, e)
		if gconv.Int(again["imported"]) != 1 {
			t.Fatal("confirmation is not idempotent")
		}
		r, e := g.DB().Model("bm_growth_products").Where("company_id", cid).Where("sku", "000001").One()
		must(t, e)
		if r["published"].Bool() {
			t.Fatal("imported offering auto-published")
		}
		if _, e = MasterWorkbookTemplate(ctx, p, cid, customID); e != nil {
			t.Fatal(e)
		}
	})
	t.Run("approved exact FAQ uses the native reply API once", func(t *testing.T) {
		w := makeWorkflow(t, "WF05", func(c *MasterConfig) {
			c.AutoReply = true
			c.ReplyRules = []MasterReplyRule{{Questions: []string{"What are your opening hours?"}, Answer: "Our support team is available Monday to Friday.", Language: "en"}}
		})
		workflow, e := g.DB().Model("bm_gm_workflows").Where("id", w).One()
		must(t, e)
		customer, e := g.DB().Model("bm_crm_customers").Where("email", "buyer@fixture-buyer.example").Value("id")
		must(t, e)
		message, e := g.DB().Model("bm_crm_messages").Data(g.Map{"customer_id": customer.Int64(), "dept_id": p.DeptID, "owner_id": p.ID, "mailbox": "sender@example.com", "direction": "incoming", "from_address": "buyer@fixture-buyer.example", "body_text": "What are your opening hours?", "body_loaded": true, "message_id": "<fixture-faq@example.com>"}).InsertAndGetId()
		must(t, e)
		eventID, e := g.DB().Model("bm_gm_events").Data(g.Map{"company_id": cid, "dept_id": p.DeptID, "owner_id": p.ID, "channel": "email", "event_key": "fixture-approved-faq", "sender": "buyer@fixture-buyer.example", "body": "What are your opening hours?", "kind": "inquiry", "payload": jsonText(g.Map{"message_id": message, "customer_id": customer.Int64()})}).InsertAndGetId()
		must(t, e)
		runID, e := masterNewRun(ctx, p, workflow, "fixture-faq", eventID)
		must(t, e)
		_, e = g.DB().Model("bm_gm_runs").Where("id", runID).Data(g.Map{"status": "running", "lease_token": "fixture-faq-lease", "lease_until": time.Now().Add(time.Minute)}).Update()
		must(t, e)
		run, e := g.DB().Model("bm_gm_runs").Where("id", runID).One()
		must(t, e)
		snapshot, e := masterDecodeSnapshot(run)
		must(t, e)
		event, e := g.DB().Model("bm_gm_events").Where("id", eventID).One()
		must(t, e)
		x := &masterExec{Ctx: ctx, P: p, Run: run, Snapshot: snapshot, Skill: "S12", Settings: masterDefaultSkill()}
		original := MasterReplyMail
		calls := 0
		MasterReplyMail = func(ctx context.Context, p *governance.Principal, parent int64, html, key string) (int64, error) {
			calls++
			if parent != message || !strings.Contains(html, "Monday to Friday") {
				t.Fatal("reply escaped its approved rule")
			}
			return 12345, nil
		}
		defer func() { MasterReplyMail = original }()
		analysis := g.Map{}
		for i := 0; i < 2; i++ {
			id, e := x.autoReply(event, analysis, "en", false)
			must(t, e)
			if id != 12345 {
				t.Fatal("FAQ reply not linked")
			}
		}
		if calls != 1 {
			t.Fatal("FAQ reply duplicated", calls)
		}
		analysis["automatic_reply_status"] = "unknown"
		_, e = x.autoReply(event, analysis, "en", false)
		must(t, e)
		if calls != 1 || gconv.String(analysis["automatic_reply_status"]) != "unknown" {
			t.Fatal("uncertain reply was resent or marked successful")
		}
		must(t, masterRunControl(ctx, p, runID, "cancel"))
	})
	t.Run("WhatsApp template and signed events", exerciseMasterWhatsAppTemplateAndSignedEvents)
}
func TestMasterSchedulesAndScoring(t *testing.T) {
	faq := MasterConfig{AutoReply: true, ReplyRules: []MasterReplyRule{{Questions: []string{"What are your opening hours?"}, Answer: "Monday to Friday", Language: "en"}}}
	if _, ok := masterReplyRule(faq, "What are your opening hours?", "en", false); !ok {
		t.Fatal("exact FAQ was not selected")
	}
	for _, q := range []string{"What are your opening hours and prices?", "Please quote a price", "Different question"} {
		if _, ok := masterReplyRule(faq, q, "en", false); ok {
			t.Fatal("non-FAQ or contractual question was auto-replied")
		}
	}
	if _, ok := masterReplyRule(faq, "What are your opening hours?", "en", true); ok {
		t.Fatal("human takeover was ignored")
	}
	c := MasterDefaultConfig("WF01")
	c.Timezone = "America/New_York"
	c.LocalTime = "06:11:37"
	now := time.Date(2026, 11, 1, 8, 0, 0, 0, time.UTC)
	next, e := masterNext(c, now)
	must(t, e)
	if next.In(mustLocation(t, c.Timezone)).Format("15:04:05") != "06:11:37" {
		t.Fatal("seconds or timezone lost")
	}
	c.Cadence = "6h"
	next, e = masterNext(c, now)
	must(t, e)
	if !next.After(now) {
		t.Fatal("nonfuture schedule")
	}
	pack := MasterPack{ScoreWeights: map[string]int{"relevance": 30, "market": 20, "contact": 20, "intent": 20, "completeness": 10}, MinConfidence: .8}
	score, _ := masterScore(pack, g.Map{}, "unconfirmed", "")
	if score != 0 {
		t.Fatal("missing facts received score")
	}
	if !masterExcluded("Buyer@Example.com", []string{"BUYER@"}) {
		t.Fatal("case-insensitive filter failed")
	}
}
func mustLocation(t *testing.T, value string) *time.Location {
	t.Helper()
	loc, e := time.LoadLocation(value)
	must(t, e)
	return loc
}
