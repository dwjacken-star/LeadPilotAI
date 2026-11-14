package growth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"billionmail-core/internal/service/governance"
	"billionmail-core/internal/service/outreachguard"
	"github.com/gogf/gf/v2/frame/g"
)

func exerciseOptionalProducts(t *testing.T, ctx context.Context, p *governance.Principal) {
	companyID, e := SaveCompany(ctx, p, 0, g.Map{"name": "Company without product catalogue", "profile": g.Map{"business": "Automotive A/C parts"}})
	must(t, e)
	must(t, PublishCompany(ctx, p, companyID))
	for i, name := range []string{"approved-company.txt", "private-company.txt"} {
		asset, e := saveBlob(ctx, p, companyID, name, "", "company", bytes.NewBufferString(name), true)
		must(t, e)
		_, e = g.DB().Model("bm_growth_assets").Where("id", asset["id"]).Data(g.Map{"published": true, "visibility": map[bool]string{true: "public", false: "internal"}[i == 0], "status": "ready"}).Update()
		must(t, e)
	}
	reader := &governance.Principal{ID: p.ID, DeptID: p.DeptID, Roles: []governance.Role{{Scope: "department", Permissions: []string{"outreach:run", "outreach:publish"}}}}
	if reader.Has("product:list") {
		t.Fatal("fixture unexpectedly has product permissions")
	}
	c := PlanConfig{Markets: []Market{{Country: "US", Language: "en", Timezone: "UTC"}}, Requirement: "Find automotive A/C parts distributors using the company profile", Senders: []string{"sender@example.com"}, DailyLeads: 1, DailySend: 1, SearchLimit: 1, PageLimit: 1, Timezone: "UTC", LocalTime: "09:00", SendStart: "00:00", SendEnd: "23:59", CooldownDays: 30}
	for _, empty := range []bool{false, true} {
		config := g.Map{}
		must(t, json.Unmarshal([]byte(jsonText(c)), &config))
		delete(config, "product_ids")
		if empty {
			config["product_ids"] = []int64{}
		}
		id, e := SavePlan(ctx, p, 0, g.Map{"name": fmt.Sprintf("Optional products %t", empty), "company_id": companyID, "config": config})
		must(t, e)
		stored, e := g.DB().Model("bm_growth_plans").Where("id", id).One()
		must(t, e)
		var saved PlanConfig
		must(t, json.Unmarshal(stored["config"].Bytes(), &saved))
		if len(saved.ProductIDs) != 0 || stored["active"].Bool() {
			t.Fatal("empty products did not persist as an inactive draft")
		}
		updated, e := SavePlan(ctx, p, id, g.Map{"name": "Updated optional plan", "config": config})
		must(t, e)
		if updated != id {
			t.Fatal("updating a plan changed its identity")
		}
		_, e = PlanControl(ctx, p, id, "enable")
		must(t, e)
		runID, e := PlanControl(ctx, p, id, "run")
		must(t, e)
		materials, e := approvedContext(ctx, reader, companyID, saved.ProductIDs)
		must(t, e)
		if materials["context_mode"] != "company" || len(materials["products"].([]g.Map)) != 0 || len(materials["product_versions"].(g.Map)) != 0 || strings.Contains(jsonText(materials), "private-company.txt") {
			t.Fatal("company-only context leaked unselected products or private materials", materials)
		}
		must(t, research(ctx, reader, runID))
		lead, e := g.DB().Model("bm_growth_leads").Data(g.Map{"plan_id": id, "company_id": companyID, "dept_id": p.DeptID, "owner_id": p.ID, "email": fmt.Sprintf("optional-%d@fixture-business.example", id), "country": "US", "language": "en", "state": "verified", "verified_until": time.Now().Add(time.Hour)}).InsertAndGetId()
		must(t, e)
		must(t, draftLead(ctx, reader, lead, runID))
		message, e := g.DB().Model("bm_growth_messages").Where("lead_id", lead).Where("run_id", runID).One()
		must(t, e)
		var frozen struct {
			Materials struct {
				ContextMode string `json:"context_mode"`
			} `json:"materials"`
		}
		must(t, json.Unmarshal(message["snapshot"].Bytes(), &frozen))
		if message.IsEmpty() || message["status"].String() != "draft" || frozen.Materials.ContextMode != "company" {
			t.Fatal("company-only drafting did not freeze its sources", message)
		}
		must(t, MessageControl(ctx, p, message["id"].Int64(), "approve"))
		must(t, dispatchRun(ctx, p, runID))
		subject, body, e := outreachguard.Content(ctx, reader, message["id"].Int64())
		must(t, e)
		if subject == "" || !strings.Contains(body, "<p>") {
			t.Fatal("company-only message could not reach the existing send handoff")
		}
		_, e = PlanControl(ctx, p, id, "pause")
		must(t, e)
		if _, _, e = outreachguard.Content(ctx, reader, message["id"].Int64()); e == nil {
			t.Fatal("paused company-only plan remained sendable")
		}
	}
	bad := c
	bad.Markets = nil
	if e = bad.validate(ctx, p, companyID); e == nil {
		t.Fatal("empty target markets were accepted")
	}
	bad = c
	bad.Markets = make([]Market, 31)
	if e = bad.validate(ctx, p, companyID); e == nil {
		t.Fatal("more than 30 markets were accepted")
	}
	bad = c
	bad.ProductIDs = make([]int64, 101)
	if e = bad.validate(ctx, p, companyID); e == nil {
		t.Fatal("more than 100 products were accepted")
	}
	draft, e := SaveProduct(ctx, p, 0, g.Map{"company_id": companyID, "sku": "unpublished", "name": "Draft"})
	must(t, e)
	bad = c
	bad.ProductIDs = []int64{draft}
	if e = bad.validate(ctx, p, companyID); e == nil {
		t.Fatal("explicit unpublished product bypassed validation")
	}
	_, e = g.DB().Model("bm_growth_companies").Where("id", companyID).Data(g.Map{"version": 999}).Update()
	must(t, e)
	if _, e = approvedContext(ctx, &governance.Principal{ID: 99, DeptID: 99, Roles: reader.Roles}, companyID, nil); e == nil {
		t.Fatal("company-only context crossed department boundaries")
	}
}
