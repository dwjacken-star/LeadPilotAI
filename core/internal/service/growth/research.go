package growth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/url"
	"strings"
	"time"

	"billionmail-core/internal/service/emailvalidation"
	"billionmail-core/internal/service/governance"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/gconv"
)

func runContext(ctx context.Context, p *governance.Principal, id int64) (map[string]interface{}, PlanConfig, error) {
	run, e := scoped(ctx, p, "bm_growth_runs", id, "outreach:run")
	if e != nil {
		return nil, PlanConfig{}, e
	}
	plan, e := scoped(ctx, p, "bm_growth_plans", run["plan_id"].Int64(), "outreach:run")
	if e != nil {
		return nil, PlanConfig{}, e
	}
	if !plan["active"].Bool() {
		return nil, PlanConfig{}, errors.New("plan paused")
	}
	if snapshotPlan(plan["config"].String()) != snapshotPlan(run["config"].String()) {
		return nil, PlanConfig{}, errors.New("plan configuration changed; start a new run")
	}
	var config PlanConfig
	e = json.Unmarshal(run["config"].Bytes(), &config)
	return g.Map{"run_id": id, "plan_id": plan["id"].Int64(), "company_id": plan["company_id"].Int64(), "plan_config": run["config"].String()}, config, e
}
func childJob(ctx context.Context, p *governance.Principal, kind, resource, key string, runID int64, extra g.Map) error {
	if extra == nil {
		extra = g.Map{}
	}
	extra["run_id"] = runID
	_, e := Queue(ctx, p, kind, resource, key, extra)
	return e
}
func research(ctx context.Context, p *governance.Principal, runID int64) error {
	if e := serperReady(ctx); e != nil {
		return e
	}
	scope, c, e := runContext(ctx, p, runID)
	if e != nil {
		return e
	}
	facts, e := approvedContext(ctx, p, gconv.Int64(scope["company_id"]), c.ProductIDs)
	if e != nil {
		return e
	}
	type queryEntry struct {
		Country  string `json:"country"`
		Language string `json:"language"`
		Query    string `json:"query"`
	}
	var result struct {
		Queries []queryEntry `json:"queries"`
	}
	input := AIInput{Model: c.Model, Instruction: fmt.Sprintf("Generate at most %d diverse B2B prospect search queries using the prospecting requirement and approved company/material facts, for ONLY the requested country/language pairs. If products are selected, use those products; when the product list is empty, use the company business and requirement without assuming any unselected products or specific SKUs. Include local-language buyer/importer/distributor terms. Use the specified city when provided. When the query limit allows it, include every requested country/language pair and distribute queries evenly across markets. Split product/customer-type combinations into focused queries; do not pack every brand and platform into each query. Do not list businesses or invent emails.", c.SearchLimit), Data: g.Map{"products": facts, "markets": c.Markets, "requirement": c.Requirement, "purchase_intent_keywords": c.IntentKeywords}, Schema: schema(g.Map{"queries": g.Map{"type": "array", "items": schema(g.Map{"country": strSchema(), "language": strSchema(), "query": strSchema()})}})}
	existing, e := g.DB().Model("bm_growth_queries").Ctx(ctx).Where("run_id", runID).Order("id").All()
	if e != nil {
		return e
	}
	if len(existing) > 0 {
		for _, r := range existing {
			result.Queries = append(result.Queries, queryEntry{r["country"].String(), r["language"].String(), r["query"].String()})
		}
	} else {
		if e = IntelligenceProvider.Extract(ctx, p, "keywords", input, &result); e != nil {
			return e
		}
	}
	if len(result.Queries) == 0 {
		return errors.New("AI returned no search queries")
	}
	if len(result.Queries) > c.SearchLimit {
		result.Queries = result.Queries[:c.SearchLimit]
	}
	for _, query := range result.Queries {
		allowed := false
		for _, market := range c.Markets {
			if query.Country == market.Country && query.Language == market.Language {
				allowed = true
			}
		}
		if !allowed || strings.TrimSpace(query.Query) == "" {
			return errors.New("AI returned unconfigured market")
		}
	}
	pairs := make([][2]string, 0, len(result.Queries))
	for _, q := range result.Queries {
		pairs = append(pairs, [2]string{q.Country, q.Language})
	}
	if e = validateQueryDistribution(c.Markets, pairs, c.SearchLimit); e != nil {
		return e
	}
	return g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		for _, query := range result.Queries {
			values := g.Map{"run_id": runID, "country": query.Country, "language": query.Language, "query": cleanText(query.Query, 500)}
			if _, e := tx.Model("bm_growth_queries").Data(values).InsertIgnore(); e != nil {
				return e
			}
			id, e := tx.Model("bm_growth_queries").Where(values).Value("id")
			if e != nil {
				return e
			}
			if _, e = tx.Model("bm_growth_jobs").Data(g.Map{"dept_id": p.DeptID, "owner_id": p.ID, "kind": "discover", "resource_id": id.String(), "dedupe_key": fmt.Sprintf("discover:%d", id.Int64()), "payload": jsonText(g.Map{"run_id": runID})}).InsertIgnore(); e != nil {
				return e
			}
		}
		_, e := tx.Model("bm_growth_runs").Where("id", runID).Data(g.Map{"status": "running"}).Update()
		return e
	})
}
func LeadControl(ctx context.Context, p *governance.Principal, id int64, action string) (int64, error) {
	lead, e := scoped(ctx, p, "bm_growth_leads", id, "outreach:manage")
	if e != nil {
		return 0, e
	}
	if action == "suppress" {
		_, e = g.DB().Model("bm_growth_leads").Where("id", id).Data(g.Map{"suppressed": true, "state": "suppressed"}).Update()
		return 0, e
	}
	if action != "verify" && action != "approve" {
		return 0, errors.New("unsupported lead operation")
	}
	if lead["suppressed"].Bool() || !lead["replied_at"].IsNil() {
		return 0, errors.New("lead has replied or is suppressed")
	}
	if action == "approve" {
		_, e = g.DB().Model("bm_growth_leads").Where("id", id).Data(g.Map{"state": "collected"}).Update()
		if e != nil {
			return 0, e
		}
	}
	run, e := g.DB().Model("bm_growth_runs").Where("plan_id", lead["plan_id"]).Order("id DESC").Value("id")
	if e != nil || run.IsEmpty() {
		return 0, errors.New("no plan run available")
	}
	return Queue(ctx, p, "verify", fmt.Sprint(id), "verify-manual:"+fmt.Sprint(id)+":"+fmt.Sprint(time.Now().Unix()), g.Map{"run_id": run.Int64()})
}
func verifyLead(ctx context.Context, p *governance.Principal, id, runID int64) error {
	if _, _, e := runContext(ctx, p, runID); e != nil {
		return e
	}
	lead, e := scoped(ctx, p, "bm_growth_leads", id, "outreach:run")
	if e != nil {
		return e
	}
	if lead["suppressed"].Bool() || !lead["replied_at"].IsNil() {
		return nil
	}
	sender := lead["assigned_sender"].String()
	if e = governance.SenderAllowed(ctx, p, sender); e != nil {
		return e
	}
	r := emailvalidation.Check(ctx, lead["email"].String(), sender, true)
	verification := gconv.Map(r)
	verification["probe_sender"] = sender
	state := "unconfirmed"
	if r.Status == emailvalidation.Invalid {
		state = "invalid"
	}
	if r.Status == emailvalidation.Valid && r.Stage == "smtp" {
		state = "verified"
	}
	_, e = g.DB().Model("bm_growth_leads").Ctx(ctx).Where("id", id).Data(g.Map{"state": state, "verification": jsonText(verification), "verified_until": r.ExpiresAt, "error": r.Code}).Update()
	if e != nil {
		return e
	}
	if state == "verified" {
		previous, e := g.DB().Model("bm_growth_messages").Ctx(ctx).Where("lead_id", id).Where("run_id", runID).Where("status", "approved").Count()
		if e != nil {
			return e
		}
		if previous > 0 {
			return childJob(ctx, p, "dispatch", fmt.Sprint(runID), fmt.Sprintf("dispatch-reverified:%d:%d:%d", runID, id, time.Now().Unix()/300), runID, nil)
		}
		return childJob(ctx, p, "draft", fmt.Sprint(id), fmt.Sprintf("draft:%d:%d", id, runID), runID, nil)
	}
	return nil
}
func paragraphHTML(paragraphs []string) string {
	var out strings.Builder
	for _, s := range paragraphs {
		s = strings.TrimSpace(s)
		if s != "" {
			out.WriteString("<p>" + strings.ReplaceAll(html.EscapeString(s), "\n", "<br>") + "</p>")
		}
	}
	return out.String()
}
func draftLead(ctx context.Context, p *governance.Principal, id, runID int64) error {
	scope, c, e := runContext(ctx, p, runID)
	if e != nil {
		return e
	}
	lead, e := scoped(ctx, p, "bm_growth_leads", id, "outreach:run")
	if e != nil {
		return e
	}
	if lead["state"].String() != "verified" || !lead["verified_until"].Time().After(time.Now()) || lead["suppressed"].Bool() || !lead["replied_at"].IsNil() {
		return errors.New("lead is not eligible")
	}
	contextData, e := approvedContext(ctx, p, gconv.Int64(scope["company_id"]), c.ProductIDs)
	if e != nil {
		return e
	}
	var result struct {
		Subject    string   `json:"subject"`
		Paragraphs []string `json:"paragraphs"`
		Language   string   `json:"language"`
		Reason     string   `json:"reason"`
	}
	input := AIInput{Model: c.Model, Instruction: "Write a concise factual B2B introduction in the customer's website language. Use only approved supplied product/company facts. The prospecting requirement describes campaign goals, not verified product claims. When no products are selected, introduce the company and relevant approved business information without assuming unselected products or specific SKUs. Do not invent prices, stock, certifications, fitment or delivery promises. Return plain text paragraphs, no HTML or template directives. Ask one relevant business question.", Data: g.Map{"approved_materials": contextData, "prospecting_requirement": c.Requirement, "customer": g.Map{"company": lead["company"].String(), "language": lead["language"].String(), "facts": jsonMap(lead["facts"])}}, Schema: schema(g.Map{"subject": strSchema(), "paragraphs": stringsSchema(), "language": strSchema(), "reason": strSchema()})}
	if e = IntelligenceProvider.Extract(ctx, p, "outreach_draft", input, &result); e != nil {
		return e
	}
	if strings.TrimSpace(result.Subject) == "" || strings.ContainsAny(result.Subject, "\r\n") || len(result.Paragraphs) == 0 || strings.Contains(jsonText(result), "{{") {
		return errors.New("invalid generated email")
	}
	// Source text and product versions are frozen with each individual message.
	snapshot := g.Map{"plan_config": scope["plan_config"], "materials": contextData, "customer_sources": lead["sources"].Interface(), "reason": result.Reason, "prompt_version": "outreach-v1"}
	status := "draft"
	if c.AutoSend {
		if e = require(p, "outreach:publish"); e != nil {
			return e
		}
		status = "approved"
	}
	previous, e := g.DB().Model("bm_growth_messages").Where("lead_id", id).Where("run_id", runID).Value("status")
	if e != nil {
		return e
	}
	if !previous.IsEmpty() {
		return nil
	}
	messageID, e := g.DB().Model("bm_growth_messages").Ctx(ctx).Data(g.Map{"lead_id": id, "run_id": runID, "plan_id": lead["plan_id"], "dept_id": p.DeptID, "owner_id": p.ID, "subject": cleanText(result.Subject, 300), "html": paragraphHTML(result.Paragraphs), "language": result.Language, "snapshot": jsonText(snapshot), "status": status}).InsertIgnore()
	if e != nil {
		return e
	}
	_ = messageID
	if c.AutoSend {
		if e = require(p, "outreach:publish"); e != nil {
			return e
		}
		return childJob(ctx, p, "dispatch", fmt.Sprint(runID), fmt.Sprintf("dispatch:%d:%d", runID, id), runID, nil)
	}
	return nil
}
func sourceHost(value string) string {
	u, e := url.Parse(value)
	if e != nil {
		return ""
	}
	return strings.ToLower(u.Hostname())
}
