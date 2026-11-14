package growth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"billionmail-core/internal/service/governance"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/gconv"
)

func init() {
	governance.PermissionCatalog = append(governance.PermissionCatalog, "intelligence:view", "intelligence:manage", "intelligence:run", "intelligence:review", "intelligence:publish", "intelligence:settings", "intelligence:export")
}
func masterAudit(ctx context.Context, p *governance.Principal, companyID, runID, id int64, action string, details g.Map) error {
	_, e := g.DB().Model("bm_gm_audit").Ctx(ctx).Data(g.Map{"company_id": companyID, "dept_id": p.DeptID, "owner_id": p.ID, "run_id": nullableID(runID), "action": action, "resource_id": id, "details": jsonText(details)}).Insert()
	return e
}
func masterRecord(ctx context.Context, p *governance.Principal, table string, id int64, permission string) (gdb.Record, error) {
	if e := require(p, permission); e != nil {
		return nil, e
	}
	if !strings.Contains("|bm_gm_workflows|bm_gm_runs|bm_gm_prospects|bm_gm_contents|bm_gm_events|bm_gm_evidence|", "|"+table+"|") {
		return nil, errors.New("invalid resource")
	}
	scope, e := p.Predicate(ctx, permission, "")
	if e != nil {
		return nil, e
	}
	r, e := g.DB().Model(table).Ctx(ctx).Where("id", id).WhereNull("deleted_at").Where(scope).One()
	if e != nil {
		return nil, e
	}
	if len(r) == 0 {
		return nil, errors.New("resource not found or access denied")
	}
	if _, e = company(ctx, p, r["company_id"].Int64(), permission, false); e != nil {
		return nil, e
	}
	return r, nil
}
func MasterSeed(ctx context.Context) error {
	packs := []struct {
		Slug, Name string
		Config     MasterPack
	}{
		{"general", "General products and services", MasterPack{Description: "Industry-neutral baseline; configure terminology, buyer types and fields before automation.", ObjectType: "both", Terms: []string{"business products and services"}, BuyerTypes: []string{"business buyer", "distributor"}, IntentTerms: []string{"request for quotation", "procurement", "supplier wanted"}, ScoreWeights: map[string]int{"relevance": 30, "market": 20, "contact": 20, "intent": 20, "completeness": 10}, MinScore: 60, MinConfidence: .8, Tone: "professional, concise and factual", ForbiddenClaims: []string{"unverified certification", "invented price or delivery promise"}}},
		{"auto_parts", "Automotive parts", MasterPack{ObjectType: "product", Terms: []string{"automotive parts", "replacement parts"}, BuyerTypes: []string{"parts importer", "distributor", "repair chain"}, IntentTerms: []string{"parts RFQ", "replacement supplier"}, Fields: []MasterField{{Key: "oe", Label: "OE reference", Type: "text"}, {Key: "fitment", Label: "Vehicle compatibility evidence", Type: "text"}, {Key: "specification", Label: "Specification", Type: "text"}}, ScoreWeights: map[string]int{"relevance": 35, "market": 20, "contact": 20, "intent": 15, "completeness": 10}, MinScore: 60, MinConfidence: .85, Tone: "professional B2B", ForbiddenClaims: []string{"unsupported vehicle fitment", "unverified OE equivalence"}}},
		{"business_services", "Business services", MasterPack{ObjectType: "service", Terms: []string{"business services"}, BuyerTypes: []string{"business procurement", "operations team"}, IntentTerms: []string{"service provider wanted", "service RFP"}, Fields: []MasterField{{Key: "service_scope", Label: "Service scope", Type: "text", Required: true}, {Key: "coverage", Label: "Delivery region", Type: "list"}, {Key: "delivery_days", Label: "Delivery days", Type: "number", Unit: "days"}}, ScoreWeights: map[string]int{"relevance": 30, "market": 15, "contact": 20, "intent": 25, "completeness": 10}, MinScore: 60, MinConfidence: .8, Tone: "consultative and factual", ForbiddenClaims: []string{"guaranteed financial results", "unsupported service levels"}}},
	}
	for _, p := range packs {
		_, e := g.DB().Model("bm_gm_packs").Ctx(ctx).Data(g.Map{"slug": p.Slug, "name": p.Name, "config": jsonText(p.Config)}).OnConflict("dept_id,slug").InsertIgnore()
		if e != nil {
			return e
		}
	}
	_, e := g.DB().Exec(ctx, `INSERT INTO bm_gm_pack_versions(pack_id,version,name,config) SELECT id,version,name,config FROM bm_gm_packs ON CONFLICT DO NOTHING`)
	return e
}
func masterPack(ctx context.Context, p *governance.Principal, id int64) (gdb.Record, MasterPack, error) {
	var cfg MasterPack
	scope, e := p.Predicate(ctx, "intelligence:view", "")
	if e != nil {
		return nil, cfg, e
	}
	row, e := g.DB().Model("bm_gm_packs").Ctx(ctx).Where("id", id).Where("active", true).Where("(dept_id=0 OR (" + scope + "))").One()
	if e != nil {
		return nil, cfg, e
	}
	if len(row) == 0 {
		return nil, cfg, errors.New("industry pack unavailable")
	}
	e = json.Unmarshal(row["config"].Bytes(), &cfg)
	return row, cfg, e
}
func masterSavePack(ctx context.Context, p *governance.Principal, id int64, data g.Map) (int64, error) {
	if e := require(p, "intelligence:manage"); e != nil {
		return 0, e
	}
	name := cleanText(gconv.String(data["name"]), 150)
	slug := gconv.String(data["slug"])
	if name == "" || !masterKey.MatchString(slug) {
		return 0, errors.New("industry name and lowercase identifier required")
	}
	var cfg MasterPack
	if e := gconv.Scan(data["config"], &cfg); e != nil {
		return 0, e
	}
	if e := cfg.validate(); e != nil {
		return 0, e
	}
	e := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		values := g.Map{"name": name, "slug": slug, "config": jsonText(cfg)}
		if id > 0 {
			row, _, e := masterPack(ctx, p, id)
			if e != nil {
				return e
			}
			if row["dept_id"].Int64() == 0 {
				id = 0
			} else {
				if e = p.CheckOwnership(ctx, "intelligence:manage", row["dept_id"].Int64(), row["owner_id"].Int64()); e != nil {
					return e
				}
			}
		}
		if id == 0 {
			if e := p.CheckOwnership(ctx, "intelligence:manage", p.DeptID, p.ID); e != nil {
				return e
			}
			values["dept_id"], values["owner_id"] = p.DeptID, p.ID
			var e error
			id, e = tx.Model("bm_gm_packs").Data(values).InsertAndGetId()
			if e != nil {
				return e
			}
		} else {
			values["version"] = gdb.Raw("version+1")
			if _, e := tx.Model("bm_gm_packs").Where("id", id).Data(values).Update(); e != nil {
				return e
			}
		}
		_, e := tx.Exec(`INSERT INTO bm_gm_pack_versions(pack_id,version,name,config) SELECT id,version,name,config FROM bm_gm_packs WHERE id=$1`, id)
		return e
	})
	return id, e
}
func (c MasterConfig) validate(ctx context.Context, p *governance.Principal, companyID int64, code string) error {
	if _, ok := masterFlows[code]; !ok {
		return errors.New("invalid workflow")
	}
	if _, e := company(ctx, p, companyID, "intelligence:view", false); e != nil {
		return e
	}
	if len(c.Markets) == 0 || len(c.Markets) > 30 || strings.TrimSpace(c.Requirement) == "" || len(c.Requirement) > 12000 {
		return errors.New("requirement and 1-30 markets required")
	}
	if c.MaxItems < 1 || c.MaxItems > 100 || c.QueriesPerMarket < 1 || c.QueriesPerMarket > 5 || c.CooldownDays < 1 || c.CooldownDays > 3650 || len(c.ProductIDs) > 100 || len(c.ProspectIDs) > 100 {
		return errors.New("invalid workflow limits")
	}
	seen := map[string]bool{}
	for _, m := range c.Markets {
		if !regexpCountry(m.Country) || m.Language == "" {
			return errors.New("select a country and language")
		}
		if _, e := time.LoadLocation(m.Timezone); e != nil {
			return e
		}
		k := m.Country + ":" + m.Language
		if seen[k] {
			return errors.New("duplicate market")
		}
		seen[k] = true
	}
	if _, e := masterNext(c, time.Now()); e != nil {
		return e
	}
	b := c.Budget
	if b.Calls < 1 || b.Calls > 10000 || b.Tokens < 1000 || b.Tokens > 100000000 || b.Searches < 1 || b.Searches > 10000 || b.Scrapes < 1 || b.Scrapes > 10000 || b.Sends < 1 || b.Sends > 10000 || b.CostMicros < 0 || b.AIInputMicrosPerMillion < 0 || b.AIOutputMicrosPerMillion < 0 || b.SearchMicros < 0 || b.ScrapeMicros < 0 {
		return errors.New("invalid daily budget")
	}
	if b.CostMicros > 0 && (b.AIInputMicrosPerMillion == 0 || b.AIOutputMicrosPerMillion == 0 || b.SearchMicros == 0 || b.ScrapeMicros == 0) {
		return errors.New("configure provider unit prices before enabling a monetary ceiling")
	}
	for _, ch := range c.Channels {
		if ch != "email" && ch != "facebook" && ch != "linkedin" && ch != "whatsapp" {
			return errors.New("unsupported channel")
		}
	}
	if code == "WF04" && len(c.Channels) == 0 {
		return errors.New("select at least one channel")
	}
	if c.AutoReply {
		if code != "WF05" || len(c.ReplyRules) == 0 || len(c.ReplyRules) > 50 || c.ReplyApprovedBy < 1 {
			return errors.New("automatic email replies require approved FAQ rules on an inbound workflow")
		}
		for _, r := range c.ReplyRules {
			if r.Language == "" || r.Answer == "" || len(r.Answer) > 5000 || masterContractual(r.Answer) || len(r.Questions) == 0 || len(r.Questions) > 20 {
				return errors.New("FAQ rules require language, exact questions and a non-contractual approved answer")
			}
			for _, q := range r.Questions {
				if len(q) < 3 || len(q) > 500 || masterContractual(q) {
					return errors.New("pricing and contractual questions must be reviewed manually")
				}
			}
		}
	}
	for _, sender := range c.Senders {
		if e := governance.SenderAllowed(ctx, p, sender); e != nil {
			return e
		}
	}
	for _, ch := range c.Channels {
		if ch == "email" && len(c.Senders) == 0 {
			return errors.New("select sender mailboxes")
		}
		if (ch == "facebook" || ch == "linkedin") && len(c.SocialAccounts) == 0 {
			return errors.New("select authorized social accounts")
		}
	}
	if (code == "WF02" || code == "WF03") && len(c.Senders) == 0 {
		return errors.New("select a mailbox for SMTP verification")
	}
	if e := c.Policy.Validate(); e != nil {
		return e
	}
	return masterValidateSelection(ctx, p, companyID, c)
}
func regexpCountry(s string) bool {
	return len(s) == 2 && s[0] >= 'A' && s[0] <= 'Z' && s[1] >= 'A' && s[1] <= 'Z'
}
func masterValidateSelection(ctx context.Context, p *governance.Principal, companyID int64, c MasterConfig) error {
	if _, e := masterImageFacts(ctx, p, companyID, c.ImageAssetIDs); e != nil {
		return e
	}
	for _, id := range c.ProductIDs {
		r, e := Product(ctx, p, id, "product:list")
		if e != nil {
			return e
		}
		if r["company_id"].Int64() != companyID || !r["published"].Bool() {
			return errors.New("select approved offerings belonging to this company")
		}
	}
	for _, id := range c.ProspectIDs {
		r, e := masterRecord(ctx, p, "bm_gm_prospects", id, "intelligence:view")
		if e != nil {
			return e
		}
		if r["company_id"].Int64() != companyID {
			return errors.New("prospect belongs to another company")
		}
	}
	for _, id := range c.InboxRuleIDs {
		r, e := scoped(ctx, p, "bm_growth_inbox_rules", id, "inquiry:manage")
		if e != nil {
			return e
		}
		if r["company_id"].Int64() != companyID {
			return errors.New("mailbox rule belongs to another company")
		}
	}
	if len(c.SocialAccounts) > 0 {
		if MasterSocialCheck == nil {
			return errors.New("social integration unavailable")
		}
		if e := MasterSocialCheck(ctx, p, c.SocialAccounts); e != nil {
			return e
		}
		platforms := map[string]bool{}
		for _, id := range c.SocialAccounts {
			platform, e := MasterSocialPlatform(ctx, p, id)
			if e != nil {
				return e
			}
			platforms[platform] = true
		}
		for _, ch := range c.Channels {
			if (ch == "facebook" || ch == "linkedin") && !platforms[ch] {
				return errors.New("select an authorized account for every selected social channel")
			}
		}
		if len(c.SocialAccounts)*len(c.Markets) > c.MaxItems {
			return errors.New("social account and market combinations exceed the workflow item limit")
		}
	}
	return nil
}
func masterSaveWorkflow(ctx context.Context, p *governance.Principal, id int64, data g.Map) (int64, error) {
	if e := require(p, "intelligence:manage"); e != nil {
		return 0, e
	}
	cid, packID := gconv.Int64(data["company_id"]), gconv.Int64(data["pack_id"])
	name, code := cleanText(gconv.String(data["name"]), 200), gconv.String(data["code"])
	if name == "" {
		return 0, errors.New("workflow name required")
	}
	var cfg MasterConfig
	if e := gconv.Scan(data["config"], &cfg); e != nil {
		return 0, e
	}
	if cfg.AutoReply {
		if e := masterRequired(p, "intelligence:review", "crm:reply", "intelligence:publish"); e != nil {
			return 0, e
		}
		cfg.ReplyApprovedBy = p.ID
	} else {
		cfg.ReplyApprovedBy = 0
	}
	if id > 0 {
		r, e := masterRecord(ctx, p, "bm_gm_workflows", id, "intelligence:manage")
		if e != nil {
			return 0, e
		}
		cid = r["company_id"].Int64()
	}
	if _, e := company(ctx, p, cid, "intelligence:manage", false); e != nil {
		return 0, e
	}
	if _, _, e := masterPack(ctx, p, packID); e != nil {
		return 0, e
	}
	if e := cfg.validate(ctx, p, cid, code); e != nil {
		return 0, e
	}
	at, _ := masterNext(cfg, time.Now())
	v := g.Map{"company_id": cid, "pack_id": packID, "name": name, "code": code, "config": jsonText(cfg), "active": false, "next_at": at, "error": ""}
	if id > 0 {
		v["version"] = gdb.Raw("version+1")
		_, e := g.DB().Model("bm_gm_workflows").Ctx(ctx).Where("id", id).Data(v).Update()
		return id, e
	}
	if e := p.CheckOwnership(ctx, "intelligence:manage", p.DeptID, p.ID); e != nil {
		return 0, e
	}
	v["dept_id"], v["owner_id"] = p.DeptID, p.ID
	return g.DB().Model("bm_gm_workflows").Ctx(ctx).Data(v).InsertAndGetId()
}
func masterSkills(ctx context.Context, companyID int64) (map[string]MasterSkillConfig, error) {
	out := map[string]MasterSkillConfig{}
	for _, s := range MasterSkills {
		out[s.ID] = masterDefaultSkill()
	}
	rows, e := g.DB().Model("bm_gm_skill_configs").Ctx(ctx).Where("company_id", companyID).All()
	if e != nil {
		return nil, e
	}
	for _, r := range rows {
		var c MasterSkillConfig
		if json.Unmarshal(r["config"].Bytes(), &c) != nil {
			return nil, errors.New("invalid saved skill configuration")
		}
		out[r["skill"].String()] = c
	}
	return out, nil
}
func masterSaveSkill(ctx context.Context, p *governance.Principal, cid int64, data g.Map) error {
	if _, e := company(ctx, p, cid, "intelligence:manage", false); e != nil {
		return e
	}
	key := gconv.String(data["skill"])
	found := false
	for _, s := range MasterSkills {
		found = found || s.ID == key
	}
	if !found {
		return errors.New("unknown skill")
	}
	var c MasterSkillConfig
	if e := gconv.Scan(data["config"], &c); e != nil {
		return e
	}
	if c.TimeoutSeconds < 10 || c.TimeoutSeconds > 600 || c.Retries < 0 || c.Retries > 3 || c.MaxItems < 1 || c.MaxItems > 100 || c.MaxOutput < 256 || c.MaxOutput > 8000 || len(c.Model) > 200 || strings.ContainsAny(c.Model, "\r\n") {
		return errors.New("invalid skill execution limits")
	}
	_, e := g.DB().Exec(ctx, `INSERT INTO bm_gm_skill_configs(company_id,skill,dept_id,owner_id,config) VALUES($1,$2,$3,$4,$5) ON CONFLICT(company_id,skill) DO UPDATE SET config=excluded.config,version=bm_gm_skill_configs.version+1`, cid, key, p.DeptID, p.ID, jsonText(c))
	return e
}
func MasterAction(ctx context.Context, p *governance.Principal, action string, id, cid int64, page int, keyword, status string, data g.Map) (interface{}, error) {
	if e := require(p, "intelligence:view"); e != nil {
		return nil, e
	}
	if data == nil {
		data = g.Map{}
	}
	switch action {
	case "catalog":
		scope, e := p.Predicate(ctx, "intelligence:view", "")
		if e != nil {
			return nil, e
		}
		packs, e := g.DB().Model("bm_gm_packs").Ctx(ctx).Where("active", true).WhereNull("deleted_at").Where("dept_id=0 OR (" + scope + ")").Order("dept_id DESC,id").All()
		if e != nil {
			return nil, e
		}
		companies, e := Companies(ctx, p, "intelligence:view")
		return g.Map{"packs": packs, "skills": MasterSkills, "flows": masterFlows, "companies": companies, "defaults": map[string]MasterConfig{"WF01": MasterDefaultConfig("WF01"), "WF02": MasterDefaultConfig("WF02"), "WF03": MasterDefaultConfig("WF03"), "WF04": MasterDefaultConfig("WF04"), "WF05": MasterDefaultConfig("WF05"), "WF06": MasterDefaultConfig("WF06")}}, e
	case "models":
		c, e := config(ctx)
		if e != nil {
			return nil, e
		}
		options, model, warning := textModelOptions(ctx, c)
		return g.Map{"models": options, "default": model, "warning": warning}, nil
	case "delete_skill":
		configs, e := masterSkills(ctx, cid)
		if e != nil {
			return nil, e
		}
		key := gconv.String(data["skill"])
		cfg, ok := configs[key]
		if !ok {
			return nil, errors.New("unknown skill")
		}
		cfg.Enabled = false
		cfg.Removed = true
		return nil, masterSaveSkill(ctx, p, cid, g.Map{"skill": key, "config": cfg})
	case "delete":
		return nil, DeleteRecord(ctx, p, "master", gconv.String(data["resource"]), id)
	case "expand_keywords":
		return ExpandIntentKeywords(ctx, p, data)
	case "save_pack":
		return masterSavePack(ctx, p, id, data)
	case "options":
		return masterOptions(ctx, p, cid, gconv.String(data["resource"]), page, keyword, gconv.Int64s(data["ids"]))
	case "save_workflow":
		return masterSaveWorkflow(ctx, p, id, data)
	case "skills":
		if _, e := company(ctx, p, cid, "intelligence:view", false); e != nil {
			return nil, e
		}
		return masterSkills(ctx, cid)
	case "save_skill":
		return nil, masterSaveSkill(ctx, p, cid, data)
	case "workflow_control":
		return MasterWorkflowControl(ctx, p, id, gconv.String(data["operation"]))
	case "run_control":
		return nil, masterRunControl(ctx, p, id, gconv.String(data["operation"]))
	case "detail":
		return masterDetail(ctx, p, gconv.String(data["resource"]), id)
	case "content_control":
		return nil, masterContentControl(ctx, p, id, gconv.String(data["operation"]), data)
	case "prospect_control":
		return nil, masterProspectControl(ctx, p, id, data)
	case "list":
		return masterList(ctx, p, cid, gconv.String(data["resource"]), page, keyword, status)
	case "overview":
		return masterOverview(ctx, p, cid)
	case "wa_settings":
		return masterWASettings(ctx, p, cid)
	case "save_wa_settings":
		return nil, masterSaveWA(ctx, p, cid, data)
	case "offering_validate":
		_, pack, e := masterPack(ctx, p, gconv.Int64(data["pack_id"]))
		if e != nil {
			return nil, e
		}
		values := gconv.Map(data["fields"])
		return g.Map{"errors": masterFieldErrors(pack, values)}, nil
	case "confirm_import":
		return masterConfirmImport(ctx, p, gconv.String(data["token"]))
	case "import_rows":
		return masterImportRows(ctx, p, gconv.String(data["token"]), page)
	case "emergency_stop":
		if !p.Super {
			return nil, errors.New("administrator required")
		}
		_, e := g.DB().Model("bm_options").Ctx(ctx).Data(g.Map{"name": "growth_master_stop", "value": jsonText(gconv.Bool(data["enabled"]))}).OnConflict("name").Save()
		if e == nil {
			e = masterAudit(ctx, p, 0, 0, 0, "emergency_stop", g.Map{"enabled": gconv.Bool(data["enabled"])})
		}
		return nil, e
	}
	return nil, errors.New("unknown growth master action")
}

var masterTables = map[string]string{"workflows": "bm_gm_workflows", "runs": "bm_gm_runs", "prospects": "bm_gm_prospects", "contents": "bm_gm_contents", "events": "bm_gm_events", "evidence": "bm_gm_evidence", "audit": "bm_gm_audit"}

func masterList(ctx context.Context, p *governance.Principal, cid int64, kind string, page int, keyword, status string) (g.Map, error) {
	table, ok := masterTables[kind]
	if !ok {
		return nil, errors.New("invalid list")
	}
	scope, e := p.Predicate(ctx, "intelligence:view", "r")
	if e != nil {
		return nil, e
	}
	if cid > 0 {
		if _, e = company(ctx, p, cid, "intelligence:view", false); e != nil {
			return nil, e
		}
	}
	q := g.DB().Model(table+" r").Ctx(ctx).WhereNull("r.deleted_at").InnerJoin("bm_growth_companies c", "c.id=r.company_id AND c.deleted_at IS NULL").Where(scope)
	if cid > 0 {
		q = q.Where("r.company_id", cid)
	}
	if status != "" {
		key := "r.status"
		if kind == "prospects" {
			key = "r.state"
		}
		if kind != "workflows" && kind != "evidence" && kind != "audit" {
			q = q.Where(key, status)
		}
	}
	if keyword != "" {
		key := map[string]string{"workflows": "r.name", "prospects": "r.company||' '||r.email||' '||r.website", "contents": "r.subject||' '||r.body", "evidence": "r.title||' '||r.url", "events": "r.sender||' '||r.body", "audit": "r.action", "runs": "r.error"}[kind]
		q = q.Where("("+key+") ILIKE ?", "%"+cleanText(keyword, 100)+"%")
	}
	n, e := q.Clone().Count()
	if e != nil {
		return nil, e
	}
	if page < 1 {
		page = 1
	}
	fields := "r.*,c.name AS company_name"
	if kind == "workflows" {
		q = q.LeftJoin("bm_gm_packs ip", "ip.id=r.pack_id")
		fields += ",ip.name AS pack_name"
	}
	if kind == "runs" {
		q = q.LeftJoin("bm_gm_workflows w", "w.id=r.workflow_id")
		fields += ",w.name AS workflow_name,w.code"
	}
	rows, e := q.Fields(fields).Order("r.id DESC").Page(page, 20).All()
	return g.Map{"list": rows, "total": n}, e
}
func masterDetail(ctx context.Context, p *governance.Principal, kind string, id int64) (g.Map, error) {
	table, ok := masterTables[kind]
	if !ok || kind == "audit" {
		return nil, errors.New("invalid detail")
	}
	r, e := masterRecord(ctx, p, table, id, "intelligence:view")
	if e != nil {
		return nil, e
	}
	out := g.Map{"record": r}
	if kind == "runs" {
		out["steps"], e = g.DB().Model("bm_gm_steps").Ctx(ctx).Where("run_id", id).Order("skill").All()
		if e != nil {
			return nil, e
		}
		out["usage"], e = g.DB().GetOne(ctx, `SELECT coalesce(sum(tokens),0) tokens,coalesce(sum(cost_micros),0) estimated_cost_micros,count(*) operations FROM bm_gm_usage WHERE run_id=$1`, id)
	}
	return out, e
}
func masterOverview(ctx context.Context, p *governance.Principal, cid int64) (g.Map, error) {
	scope, e := p.Predicate(ctx, "intelligence:view", "r")
	if e != nil {
		return nil, e
	}
	out := g.Map{"as_of": time.Now(), "metrics": g.Map{}}
	for _, kind := range []string{"workflows", "runs", "prospects", "contents", "events", "evidence"} {
		q := g.DB().Model(masterTables[kind]+" r").Ctx(ctx).InnerJoin("bm_growth_companies c", "c.id=r.company_id AND c.deleted_at IS NULL").Where(scope)
		if cid > 0 {
			q = q.Where("r.company_id", cid)
		}
		n, e := q.Count()
		if e != nil {
			return nil, e
		}
		out["metrics"].(g.Map)[kind] = n
	}
	v, e := g.DB().Model("bm_options").Ctx(ctx).Where("name", "growth_master_stop").Value("value")
	if e != nil {
		return nil, e
	}
	out["stopped"] = v.Bool()
	return out, nil
}
func masterProspectControl(ctx context.Context, p *governance.Principal, id int64, data g.Map) error {
	r, e := masterRecord(ctx, p, "bm_gm_prospects", id, "intelligence:manage")
	if e != nil {
		return e
	}
	permission := gconv.String(data["contact_permission"])
	if permission != "allowed" && permission != "blocked" && permission != "unreviewed" {
		return errors.New("invalid contact permission")
	}
	evidence := cleanText(gconv.String(data["permission_evidence"]), 2000)
	if permission == "allowed" && evidence == "" {
		return errors.New("record the permission or applicable outreach policy evidence")
	}
	_, e = g.DB().Model("bm_gm_prospects").Ctx(ctx).Where("id", id).Data(g.Map{"contact_permission": permission, "permission_evidence": evidence, "wa_opt_in": gconv.Bool(data["wa_opt_in"]), "suppressed": permission == "blocked", "updated_at": time.Now()}).Update()
	if e == nil {
		e = masterAudit(ctx, p, r["company_id"].Int64(), 0, id, "prospect_permission", g.Map{"permission": permission, "evidence": evidence, "wa_opt_in": gconv.Bool(data["wa_opt_in"])})
	}
	return e
}

// Native social operations are registered by the social package, avoiding an import cycle.
var MasterSocialCheck func(context.Context, *governance.Principal, []int64) error
var MasterSocialPlatform func(context.Context, *governance.Principal, int64) (string, error)
var MasterSocialHandoff func(context.Context, *governance.Principal, gdb.Record, MasterSnapshot) (int64, error)
var MasterSocialReceipt func(context.Context, *governance.Principal, int64) (g.Map, error)

func masterRequired(p *governance.Principal, permissions ...string) error {
	for _, code := range permissions {
		if e := require(p, code); e != nil {
			return e
		}
	}
	return nil
}
func masterDecodeSnapshot(r gdb.Record) (MasterSnapshot, error) {
	var s MasterSnapshot
	e := json.Unmarshal(r["snapshot"].Bytes(), &s)
	if e == nil && len(masterFlows[s.Code]) == 0 {
		e = fmt.Errorf("invalid saved workflow %s", s.Code)
	}
	return s, e
}
