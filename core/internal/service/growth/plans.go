package growth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"billionmail-core/internal/service/batch_mail"
	"billionmail-core/internal/service/governance"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/gconv"
)

type Market struct {
	Country  string `json:"country"`
	Language string `json:"language"`
	Timezone string `json:"timezone"`
	City     string `json:"city,omitempty"`
}
type PlanConfig struct {
	Model           string            `json:"model"`
	ProductIDs      []int64           `json:"product_ids"`
	Markets         []Market          `json:"markets"`
	IntentKeywords  []string          `json:"intent_keywords"`
	Requirement     string            `json:"requirement"`
	ExcludeKeywords []string          `json:"exclude_keywords"`
	Senders         []string          `json:"senders"`
	DailyLeads      int               `json:"daily_leads"`
	DailySend       int               `json:"daily_send"`
	SearchLimit     int               `json:"search_limit"`
	SearchResults   int               `json:"search_results"`
	SearchChannel   string            `json:"search_channel"`
	PageLimit       int               `json:"page_limit"`
	Cadence         string            `json:"cadence"`
	Timezone        string            `json:"timezone"`
	LocalTime       string            `json:"local_time"`
	SendStart       string            `json:"send_start"`
	SendEnd         string            `json:"send_end"`
	CooldownDays    int               `json:"cooldown_days"`
	AutoSend        bool              `json:"auto_send"`
	Warmup          bool              `json:"warmup"`
	Unsubscribe     *bool             `json:"unsubscribe,omitempty"`
	TrackOpen       bool              `json:"track_open"`
	TrackClick      bool              `json:"track_click"`
	Policy          governance.Policy `json:"policy"`
}

func (c PlanConfig) resultsPerQuery() int {
	if c.SearchResults == 0 {
		return 10
	}
	return c.SearchResults
}
func (c PlanConfig) searchChannel() string {
	if c.SearchChannel == "" {
		return "search"
	}
	return c.SearchChannel
}

func (c PlanConfig) deliveryOptions() batch_mail.TaskDeliveryOptions {
	return batch_mail.TaskDeliveryOptions{Warmup: c.Warmup, Unsubscribe: c.Unsubscribe == nil || *c.Unsubscribe, TrackOpen: c.TrackOpen, TrackClick: c.TrackClick}
}

func (c PlanConfig) validate(ctx context.Context, p *governance.Principal, companyID int64) error {
	if e := validateSelectedTextModel(ctx, c.Model); e != nil {
		return e
	}
	if c.resultsPerQuery() < 1 || c.resultsPerQuery() > 20 || (c.searchChannel() != "search" && c.searchChannel() != "places" && c.searchChannel() != "both") {
		return errors.New("invalid Serper search channel or result limit")
	}
	if len(c.IntentKeywords) > 50 {
		return errors.New("at most 50 intent keywords")
	}
	if strings.TrimSpace(c.Requirement) == "" || len(c.Requirement) > 20000 {
		return errors.New("enter a prospecting requirement (maximum 20000 bytes)")
	}
	if len(c.ProductIDs) > 100 {
		return errors.New("select no more than 100 products")
	}
	if len(c.Markets) == 0 || len(c.Markets) > 30 {
		return errors.New("select 1-30 markets")
	}
	if c.DailyLeads < 1 || c.DailyLeads > 1000 || c.DailySend < 1 || c.DailySend > 500 || c.SearchLimit < 1 || c.SearchLimit > 20 || c.PageLimit < 1 || c.PageLimit > 10 || c.CooldownDays < 1 {
		return errors.New("invalid outreach limits")
	}
	if _, e := company(ctx, p, companyID, "outreach:manage", true); e != nil {
		return e
	}
	if _, e := (batch_mail.ScheduleSpec{Cadence: "daily", Timezone: c.Timezone, LocalTime: c.LocalTime}).Next(time.Now()); e != nil {
		return e
	}
	if _, e := time.Parse("15:04", c.SendStart); e != nil {
		return e
	}
	if _, e := time.Parse("15:04", c.SendEnd); e != nil {
		return e
	}
	if c.SendStart >= c.SendEnd {
		return errors.New("send window must start before it ends")
	}
	for _, market := range c.Markets {
		if len(market.City) > 300 {
			return errors.New("city is too long")
		}
		if len(market.Country) != 2 || market.Country != strings.ToUpper(market.Country) || market.Language == "" {
			return errors.New("use ISO country codes and a language")
		}
		if _, e := time.LoadLocation(market.Timezone); e != nil {
			return e
		}
	}
	for _, id := range c.ProductIDs {
		r, e := Product(ctx, p, id, "product:list")
		if e != nil {
			return e
		}
		if r["company_id"].Int64() != companyID || !r["published"].Bool() {
			return errors.New("select published products from this company")
		}
	}
	if len(c.Senders) == 0 || len(c.Senders) > 30 {
		return errors.New("select sender mailboxes")
	}
	for _, sender := range c.Senders {
		if e := governance.SenderAllowed(ctx, p, sender); e != nil {
			return e
		}
	}
	return c.Policy.Validate()
}
func SavePlan(ctx context.Context, p *governance.Principal, id int64, data g.Map) (int64, error) {
	if e := require(p, "outreach:manage"); e != nil {
		return 0, e
	}
	companyID := gconv.Int64(data["company_id"])
	name := cleanText(gconv.String(data["name"]), 200)
	if name == "" {
		return 0, errors.New("plan name required")
	}
	var c PlanConfig
	if e := gconv.Scan(data["config"], &c); e != nil {
		return 0, e
	}
	if id > 0 {
		r, e := scoped(ctx, p, "bm_growth_plans", id, "outreach:manage")
		if e != nil {
			return 0, e
		}
		companyID = r["company_id"].Int64()
	}
	if e := c.validate(ctx, p, companyID); e != nil {
		return 0, e
	}
	next, e := (batch_mail.ScheduleSpec{Cadence: "daily", Timezone: c.Timezone, LocalTime: c.LocalTime}).Next(time.Now())
	if e != nil {
		return 0, e
	}
	values := g.Map{"name": name, "company_id": companyID, "config": jsonText(c), "next_at": next, "active": false}
	if id > 0 {
		values["version"] = gdb.Raw("version+1")
		_, e = g.DB().Model("bm_growth_plans").Ctx(ctx).Where("id", id).Data(values).Update()
		return id, e
	}
	if e = p.CheckOwnership(ctx, "outreach:manage", p.DeptID, p.ID); e != nil {
		return 0, e
	}
	values["dept_id"], values["owner_id"] = p.DeptID, p.ID
	return g.DB().Model("bm_growth_plans").Ctx(ctx).Data(values).InsertAndGetId()
}
func PlanControl(ctx context.Context, p *governance.Principal, id int64, action string) (int64, error) {
	row, e := scoped(ctx, p, "bm_growth_plans", id, "outreach:run")
	if e != nil {
		return 0, e
	}
	if action == "pause" {
		_, e = g.DB().Model("bm_growth_plans").Ctx(ctx).Where("id", id).Data(g.Map{"active": false}).Update()
		return 0, e
	}
	if action != "enable" && action != "run" {
		return 0, errors.New("invalid plan action")
	}
	var c PlanConfig
	if e = json.Unmarshal(row["config"].Bytes(), &c); e != nil {
		return 0, e
	}
	if e = c.validate(ctx, p, row["company_id"].Int64()); e != nil {
		return 0, e
	}
	if c.AutoSend {
		if e = require(p, "outreach:publish"); e != nil {
			return 0, e
		}
	}
	_, e = g.DB().Model("bm_growth_plans").Ctx(ctx).Where("id", id).Data(g.Map{"active": true, "error": ""}).Update()
	if e != nil {
		return 0, e
	}
	if action == "run" {
		return manualRun(ctx, p, row)
	}
	return 0, nil
}

// A deliberate manual run after completion must not return yesterday's/today's
// completed result. Concurrent clicks reuse only the currently active snapshot.
func manualRun(ctx context.Context, p *governance.Principal, plan gdb.Record) (int64, error) {
	var id int64
	e := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if _, e := tx.Exec("SELECT pg_advisory_xact_lock(614820,hashtext($1))", plan["id"].String()); e != nil {
			return e
		}
		old, e := tx.Model("bm_growth_runs").Where("plan_id", plan["id"]).Where("config=?::jsonb", plan["config"].String()).WhereIn("status", []string{"queued", "running"}).Order("id DESC").Value("id")
		if e != nil {
			return e
		}
		if !old.IsEmpty() {
			id = old.Int64()
			return nil
		}
		id, e = insertRun(tx, p, plan, time.Now().UTC().Truncate(time.Microsecond))
		return e
	})
	return id, e
}
func insertRun(tx gdb.TX, p *governance.Principal, plan gdb.Record, due time.Time) (int64, error) {
	id, e := tx.Model("bm_growth_runs").Data(g.Map{"plan_id": plan["id"], "dept_id": p.DeptID, "owner_id": p.ID, "due_at": due, "config": plan["config"].String()}).InsertAndGetId()
	if e != nil {
		return 0, e
	}
	_, e = tx.Model("bm_growth_jobs").Data(g.Map{"dept_id": p.DeptID, "owner_id": p.ID, "kind": "research", "resource_id": fmt.Sprint(id), "dedupe_key": fmt.Sprintf("research:%d", id), "payload": jsonText(g.Map{"run_id": id})}).Insert()
	return id, e
}
func newRun(ctx context.Context, p *governance.Principal, plan gdb.Record, due time.Time) (int64, error) {
	var id int64
	e := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		old, e := tx.Model("bm_growth_runs").Where("plan_id", plan["id"]).Where("due_at", due).Value("id")
		if e != nil {
			return e
		}
		if !old.IsEmpty() {
			id = old.Int64()
			return nil
		}
		id, e = insertRun(tx, p, plan, due)
		return e
	})
	return id, e
}
func approvedContext(ctx context.Context, p *governance.Principal, companyID int64, ids []int64) (g.Map, error) {
	permission, mode := "product:list", "products"
	if len(ids) == 0 {
		permission, mode = "outreach:run", "company"
	}
	c, e := company(ctx, p, companyID, permission, true)
	if e != nil {
		return nil, e
	}
	products := []g.Map{}
	versions := g.Map{}
	for _, id := range ids {
		r, e := Product(ctx, p, id, "product:list")
		if e != nil {
			return nil, e
		}
		if r["company_id"].Int64() != companyID || !r["published"].Bool() {
			return nil, errors.New("product publication revoked")
		}
		products = append(products, jsonMap(r["published_fields"]))
		versions[fmt.Sprint(id)] = r["version"].Int()
	}
	materials, e := g.DB().Model("bm_growth_assets").Ctx(ctx).Where("company_id", companyID).Where("published", true).Where("visibility", "public").Where("ai_allowed", true).WhereNull("deleted_at").Fields("id,filename,category,extracted,content,version").Limit(20).All()
	if e != nil {
		return nil, e
	}
	return g.Map{"context_mode": mode, "company_id": companyID, "company_name": c["name"].String(), "brand": c["brand"].String(), "company": jsonMap(c["published_profile"]), "company_version": c["version"].Int(), "products": products, "product_versions": versions, "materials": materials}, nil
}
