package growth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"billionmail-core/internal/service/governance"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/gconv"
)

func Companies(ctx context.Context, p *governance.Principal, code string) (gdb.Result, error) {
	if e := require(p, code); e != nil {
		return nil, e
	}
	scope, e := p.Predicate(ctx, code, "c")
	if e != nil {
		return nil, e
	}
	rows, err := g.DB().Model("bm_growth_companies c").Ctx(ctx).Fields("c.*").WhereNull("c.deleted_at").Where("(" + scope + fmt.Sprintf(" OR EXISTS(SELECT 1 FROM bm_growth_grants x WHERE x.company_id=c.id AND x.dept_id=%d AND x.use_materials))", p.DeptID)).Order("c.name").Limit(200).All()
	for _, r := range rows {
		if _, e := company(ctx, p, r["id"].Int64(), code, false); e != nil {
			r["profile"] = r["published_profile"]
		}
	}
	return rows, err
}
func SaveCompany(ctx context.Context, p *governance.Principal, id int64, data g.Map) (int64, error) {
	if e := require(p, "company:manage"); e != nil {
		return 0, e
	}
	name := cleanText(gconv.String(data["name"]), 200)
	if name == "" {
		return 0, errors.New("company name required")
	}
	values := g.Map{"name": name, "brand": cleanText(gconv.String(data["brand"]), 200), "domains": jsonText(gconv.Strings(data["domains"])), "profile": jsonText(data["profile"])}
	if id > 0 {
		if _, e := company(ctx, p, id, "company:manage", false); e != nil {
			return 0, e
		}
		values["version"] = gdb.Raw("version+1")
		_, e := g.DB().Model("bm_growth_companies").Ctx(ctx).Where("id", id).Data(values).Update()
		return id, e
	}
	if e := p.CheckOwnership(ctx, "company:manage", p.DeptID, p.ID); e != nil {
		return 0, e
	}
	values["dept_id"], values["owner_id"] = p.DeptID, p.ID
	return g.DB().Model("bm_growth_companies").Ctx(ctx).Data(values).InsertAndGetId()
}
func PublishCompany(ctx context.Context, p *governance.Principal, id int64) error {
	r, e := company(ctx, p, id, "company:publish", false)
	if e != nil {
		return e
	}
	_, e = g.DB().Model("bm_growth_companies").Ctx(ctx).Where("id", id).Data(g.Map{"published_profile": jsonText(publicProductFields(jsonMap(r["profile"]))), "version": gdb.Raw("version+1")}).Update()
	if e == nil {
		e = version(ctx, p, id, "company", fmt.Sprint(id), r["version"].Int()+1, jsonMap(r["profile"]))
	}
	return e
}
func GrantCompany(ctx context.Context, p *governance.Principal, id int64, depts []int64) error {
	if _, e := company(ctx, p, id, "company:manage", false); e != nil {
		return e
	}
	// Granting cross-department access is an administrator action, separate from
	// merely being allowed to maintain a company's own-department records.
	if !p.Super {
		return errors.New("administrator required to share company materials")
	}
	return g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if _, e := tx.Model("bm_growth_grants").Where("company_id", id).Delete(); e != nil {
			return e
		}
		for _, dept := range depts {
			if dept <= 0 {
				continue
			}
			if _, e := tx.Model("bm_growth_grants").Data(g.Map{"company_id": id, "dept_id": dept, "use_materials": true}).OnConflict("company_id,dept_id").Save(); e != nil {
				return e
			}
		}
		return nil
	})
}
func version(ctx context.Context, p *governance.Principal, companyID int64, kind, id string, v int, data interface{}) error {
	_, e := g.DB().Model("bm_growth_versions").Ctx(ctx).Data(g.Map{"company_id": companyID, "resource": kind, "resource_id": id, "version": v, "snapshot": jsonText(data), "account_id": p.ID}).OnConflict("resource,resource_id,version").Save()
	return e
}
func List(ctx context.Context, p *governance.Principal, kind string, companyID int64, page int, keyword, status string) (g.Map, error) {
	defs := map[string][3]string{"assets": {"bm_growth_assets", "material:list", "filename"}, "products": {"bm_growth_products", "product:list", "sku||' '||name"}, "plans": {"bm_growth_plans", "outreach:list", "name"}, "runs": {"bm_growth_runs", "outreach:list", "id::text"}, "candidates": {"bm_growth_candidates", "outreach:list", "company||' '||website||' '||address||' '||emails::text"}, "leads": {"bm_growth_leads", "outreach:list", "email||' '||company"}, "messages": {"bm_growth_messages", "outreach:list", "subject"}, "inquiries": {"bm_growth_inquiries", "inquiry:list", "summary"}, "jobs": {"bm_growth_jobs", "material:list", "kind||' '||error"}, "inbox_rules": {"bm_growth_inbox_rules", "inquiry:manage", "mailbox"}}
	def, ok := defs[kind]
	if !ok {
		return nil, errors.New("unknown list")
	}
	if e := require(p, def[1]); e != nil {
		return nil, e
	}
	scope, e := p.Predicate(ctx, def[1], "")
	if e != nil {
		return nil, e
	}
	q := g.DB().Model(def[0]).Ctx(ctx).WhereNull("deleted_at")
	if companyID > 0 && (kind == "runs" || kind == "messages") {
		q = q.Where("plan_id IN (SELECT id FROM bm_growth_plans WHERE company_id=?)", companyID)
	}
	if companyID > 0 && (kind == "assets" || kind == "products") {
		if _, e = company(ctx, p, companyID, def[1], true); e != nil {
			return nil, e
		}
		q = q.Where("company_id", companyID).WhereNull("deleted_at")
		if _, e = company(ctx, p, companyID, def[1], false); e != nil {
			q = q.Where("published", true)
			if kind == "assets" {
				q = q.Where("visibility", "public")
			}
		}
	} else {
		q = q.Where(scope)
		if kind == "assets" || kind == "products" {
			q = q.WhereNull("deleted_at")
		}
		if companyID > 0 && kind != "runs" && kind != "messages" && kind != "jobs" {
			q = q.Where("company_id", companyID)
		}
	}
	if keyword != "" {
		q = q.Where("("+def[2]+") ILIKE ?", "%"+cleanText(keyword, 200)+"%")
	}
	if kind == "candidates" && status == "with_email" {
		q = q.Where("jsonb_array_length(emails)>0")
		status = ""
	}
	if status != "" {
		column := "status"
		if kind == "leads" || kind == "inquiries" || kind == "candidates" {
			column = "state"
		}
		if kind == "products" {
			q = q.Where("published", status == "published")
		} else if kind != "plans" && kind != "inbox_rules" {
			q = q.Where(column, status)
		}
	}
	n, e := q.Count()
	if e != nil {
		return nil, e
	}
	if page < 1 {
		page = 1
	}
	if kind == "runs" || kind == "leads" || kind == "messages" || kind == "candidates" {
		q = q.Fields(def[0] + ".*,(SELECT name FROM bm_growth_plans plan WHERE plan.id=" + def[0] + ".plan_id) plan_name")
	}
	if kind == "leads" || kind == "candidates" {
		q = q.Fields(def[0] + ".*,(SELECT name FROM bm_growth_plans plan WHERE plan.id=" + def[0] + ".plan_id) plan_name,(SELECT name FROM bm_growth_companies c WHERE c.id=" + def[0] + ".company_id) company_name")
	}
	if kind == "plans" {
		q = q.Fields("bm_growth_plans.*,(SELECT c.name FROM bm_growth_companies c WHERE c.id=bm_growth_plans.company_id) company_name")
	}
	if kind == "runs" {
		q = q.Fields("bm_growth_runs.*,(SELECT name FROM bm_growth_plans plan WHERE plan.id=bm_growth_runs.plan_id) plan_name,(" + strings.ReplaceAll(runStatsSQL, "r.id", "bm_growth_runs.id") + ") live_stats")
	}
	rows, e := q.Order("id DESC").Page(page, 20).All()
	if rows == nil {
		rows = gdb.Result{}
	}
	// File storage paths must never be returned to clients, including readers.
	if kind == "assets" {
		for _, row := range rows {
			delete(row, "storage_key")
			delete(row, "content")
		}
	}
	if kind == "products" {
		for _, r := range rows {
			if _, err := company(ctx, p, r["company_id"].Int64(), def[1], false); err != nil {
				sanitizeProduct(r)
			}
		}
	}
	return g.Map{"list": rows, "total": n}, e
}
func Product(ctx context.Context, p *governance.Principal, id int64, permission string) (gdb.Record, error) {
	r, e := g.DB().Model("bm_growth_products").Ctx(ctx).Where("id", id).WhereNull("deleted_at").One()
	if e != nil || len(r) == 0 {
		return nil, errors.New("product not found")
	}
	shared := permission == "product:list" || permission == "product:export"
	if _, e = company(ctx, p, r["company_id"].Int64(), permission, shared); e != nil {
		return nil, e
	}
	if shared {
		if _, e = company(ctx, p, r["company_id"].Int64(), permission, false); e != nil {
			if r["published"].Bool() {
				sanitizeProduct(r)
				return r, nil
			}
			return nil, errors.New("product is not published")
		}
	}
	return r, nil
}
func SaveProduct(ctx context.Context, p *governance.Principal, id int64, data g.Map) (int64, error) {
	var old gdb.Record
	var e error
	companyID := gconv.Int64(data["company_id"])
	if id > 0 {
		old, e = Product(ctx, p, id, "product:manage")
		if e != nil {
			return 0, e
		}
		companyID = old["company_id"].Int64()
	}
	if _, e = company(ctx, p, companyID, "product:manage", false); e != nil {
		return 0, e
	}
	sku := cleanText(gconv.String(data["sku"]), 128)
	name := cleanText(gconv.String(data["name"]), 300)
	if sku == "" || name == "" {
		return 0, errors.New("SKU and product name required")
	}
	fields := gconv.Map(data["fields"])
	if fields == nil {
		fields = g.Map{}
	}
	packID := gconv.Int64(data["industry_pack_id"])
	if packID == 0 && len(old) > 0 {
		packID = old["industry_pack_id"].Int64()
	}
	packVersion := 0
	if packID > 0 {
		packRow, pack, err := masterPack(ctx, p, packID)
		if err != nil {
			return 0, err
		}
		if problems := masterFieldErrors(pack, fields); len(problems) > 0 {
			return 0, errors.New(strings.Join(problems, "; "))
		}
		packVersion = packRow["version"].Int()
	}
	values := g.Map{"sku": sku, "name": name, "category": cleanText(gconv.String(data["category"]), 100), "fields": jsonText(fields)}
	if packID > 0 {
		values["industry_pack_id"], values["industry_pack_version"] = packID, packVersion
	}
	if id > 0 {
		keys := []string{"name", "category"}
		for k := range fields {
			keys = append(keys, k)
		}
		values["manual_keys"] = jsonText(keys)
		values["version"] = gdb.Raw("version+1")
		_, e = g.DB().Model("bm_growth_products").Ctx(ctx).Where("id", id).Data(values).Update()
		return id, e
	}
	values["company_id"], values["dept_id"], values["owner_id"] = companyID, p.DeptID, p.ID
	return g.DB().Model("bm_growth_products").Ctx(ctx).Data(values).InsertAndGetId()
}
func PublishProduct(ctx context.Context, p *governance.Principal, id int64, publish bool) error {
	r, e := Product(ctx, p, id, "product:publish")
	if e != nil {
		return e
	}
	if publish && r["industry_pack_id"].Int64() > 0 {
		_, pack, err := masterPack(ctx, p, r["industry_pack_id"].Int64())
		if err != nil {
			return err
		}
		if problems := masterFieldErrors(pack, jsonMap(r["fields"])); len(problems) > 0 {
			return errors.New(strings.Join(problems, "; "))
		}
	}
	snapshot := g.Map{"sku": r["sku"].String(), "name": r["name"].String(), "category": r["category"].String(), "specifications": publicProductFields(jsonMap(r["fields"]))}
	_, e = g.DB().Model("bm_growth_products").Ctx(ctx).Where("id", id).Data(g.Map{"published": publish, "published_fields": jsonText(snapshot), "version": gdb.Raw("version+1")}).Update()
	if e == nil {
		e = version(ctx, p, r["company_id"].Int64(), "product", fmt.Sprint(id), r["version"].Int()+1, snapshot)
	}
	return e
}
func publicProductFields(fields g.Map) g.Map {
	out := g.Map{}
	for k, v := range fields {
		key := strings.ToLower(k)
		if strings.Contains(key, "cost") || strings.Contains(key, "purchase") || strings.Contains(key, "internal") || strings.Contains(key, "bank") || strings.Contains(key, "成本") || strings.Contains(key, "采购") || strings.Contains(key, "password") || strings.Contains(key, "secret") || strings.Contains(key, "margin") || strings.Contains(key, "银行") || strings.Contains(key, "内部") {
			continue
		}
		if nested, ok := v.(map[string]interface{}); ok {
			v = publicProductFields(nested)
		}
		out[k] = v
	}
	return out
}
func Queue(ctx context.Context, p *governance.Principal, kind, resource, key string, payload interface{}) (int64, error) {
	if kind == "ingest" {
		old, e := g.DB().Model("bm_growth_jobs").Ctx(ctx).Where("kind", kind).Where("resource_id", resource).WhereIn("status", []string{"queued", "running"}).Value("id")
		if e != nil {
			return 0, e
		}
		if !old.IsEmpty() {
			return old.Int64(), nil
		}
	}
	if key != "" {
		old, e := g.DB().Model("bm_growth_jobs").Ctx(ctx).Where("dedupe_key", key).Value("id")
		if e != nil {
			return 0, e
		}
		if !old.IsEmpty() {
			return old.Int64(), nil
		}
	}
	id, e := g.DB().Model("bm_growth_jobs").Ctx(ctx).Data(g.Map{"dept_id": p.DeptID, "owner_id": p.ID, "kind": kind, "resource_id": resource, "dedupe_key": key, "payload": jsonText(payload)}).InsertAndGetId()
	if e != nil {
		q := g.DB().Model("bm_growth_jobs").Ctx(ctx)
		if kind == "ingest" {
			q = q.Where("kind", kind).Where("resource_id", resource).WhereIn("status", []string{"queued", "running"})
		} else {
			q = q.Where("dedupe_key", key)
		}
		old, err := q.Value("id")
		if err == nil && !old.IsEmpty() {
			return old.Int64(), nil
		}
	}
	return id, e
}
func JobControl(ctx context.Context, p *governance.Principal, id int64, action string) error {
	r, e := scoped(ctx, p, "bm_growth_jobs", id, jobPermission(ctx, id))
	if e != nil {
		return e
	}
	if action != "retry" && action != "cancel" {
		return errors.New("invalid job operation")
	}
	state := r["status"].String()
	if action == "retry" && state != "failed" && state != "configuration_required" && state != "cancelled" {
		return errors.New("only failed/stopped jobs can be retried")
	}
	values := g.Map{"status": "cancelled", "lease_token": nil, "lease_until": nil, "updated_at": time.Now()}
	if action == "retry" {
		values["status"], values["next_at"], values["error"] = "queued", time.Now(), ""
		values["attempts"], values["progress"] = 0, 0
	}
	_, e = g.DB().Model("bm_growth_jobs").Ctx(ctx).Where("id", id).Data(values).Update()
	return e
}
