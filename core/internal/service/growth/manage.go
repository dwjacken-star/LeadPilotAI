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
	"github.com/xuri/excelize/v2"
)

func AssetDetail(ctx context.Context, p *governance.Principal, id string) (g.Map, error) {
	r, e := Asset(ctx, p, id, "material:list")
	if e != nil {
		return nil, e
	}
	delete(r, "storage_key")
	if !p.Has("product:list") {
		return g.Map{"asset": r, "links": gdb.Result{}}, nil
	}
	links, e := g.DB().Model("bm_growth_product_assets a").Ctx(ctx).LeftJoin("bm_growth_products p", "p.id=a.product_id").Where("a.asset_id", id).WhereNull("p.deleted_at").Fields("a.*,p.sku,p.name").All()
	visible := gdb.Result{}
	for _, link := range links {
		if _, err := Product(ctx, p, link["product_id"].Int64(), "product:list"); err == nil {
			visible = append(visible, link)
		}
	}
	return g.Map{"asset": r, "links": visible}, e
}
func ProductDetail(ctx context.Context, p *governance.Principal, id int64) (g.Map, error) {
	r, e := Product(ctx, p, id, "product:list")
	if e != nil {
		return nil, e
	}
	q := g.DB().Model("bm_growth_product_assets x").Ctx(ctx).LeftJoin("bm_growth_assets a", "a.id=x.asset_id").Where("x.product_id", id).WhereNull("a.deleted_at")
	if _, e = company(ctx, p, r["company_id"].Int64(), "product:list", false); e != nil {
		q = q.Where("a.visibility", "public").Where("a.published", true)
	}
	images, e := q.Fields("x.asset_id,x.role,x.confirmed,x.evidence,a.filename,a.mime,a.published").All()
	visible := gdb.Result{}
	for _, item := range images {
		if _, err := Asset(ctx, p, item["asset_id"].String(), "material:list"); err == nil {
			visible = append(visible, item)
		}
	}
	return g.Map{"product": r, "images": visible}, e
}
func Versions(ctx context.Context, p *governance.Principal, companyID int64, kind, id string) (gdb.Result, error) {
	if _, e := company(ctx, p, companyID, "material:manage", false); e != nil {
		return nil, e
	}
	return g.DB().Model("bm_growth_versions").Ctx(ctx).Where("company_id", companyID).Where("resource", kind).Where("resource_id", id).Order("version DESC").Limit(50).All()
}
func Archive(ctx context.Context, p *governance.Principal, kind string, id int64, assetID string) error {
	if kind == "assets" {
		r, e := Asset(ctx, p, assetID, "material:delete")
		if e != nil {
			return e
		}
		_, e = g.DB().Model("bm_growth_assets").Ctx(ctx).Where("id", r["id"]).Data(g.Map{"deleted_at": time.Now(), "published": false, "version": gdb.Raw("version+1")}).Update()
		return e
	}
	if kind == "products" {
		r, e := Product(ctx, p, id, "product:manage")
		if e != nil {
			return e
		}
		_, e = g.DB().Model("bm_growth_products").Ctx(ctx).Where("id", r["id"]).Data(g.Map{"deleted_at": time.Now(), "published": false, "version": gdb.Raw("version+1")}).Update()
		return e
	}
	return errors.New("unsupported archive operation")
}

// ApplyRecognizedCompany stages evidence in a draft. Publication is separate.
func ApplyRecognizedCompany(ctx context.Context, p *governance.Principal, id string) error {
	asset, e := Asset(ctx, p, id, "material:manage")
	if e != nil {
		return e
	}
	c, e := company(ctx, p, asset["company_id"].Int64(), "company:manage", false)
	if e != nil {
		return e
	}
	extracted := jsonMap(asset["extracted"])
	profile := jsonMap(c["profile"])
	for _, raw := range gconv.SliceMap(extracted["facts"]) {
		key, value := gconv.String(raw["key"]), gconv.String(raw["value"])
		if key != "" && value != "" && gconv.String(raw["evidence"]) != "" {
			if _, exists := profile[key]; !exists {
				profile[key] = value
			}
		}
	}
	profile["source_material"] = asset["id"].String()
	_, e = g.DB().Model("bm_growth_companies").Ctx(ctx).Where("id", c["id"]).Data(g.Map{"profile": jsonText(profile), "version": gdb.Raw("version+1")}).Update()
	return e
}
func Export(ctx context.Context, p *governance.Principal, kind string, companyID int64, assetID string) ([]byte, error) {
	if kind=="master_prospects"{return masterExportProspects(ctx,p,companyID,assetID)}
	if kind == "industry_template" {
		return MasterWorkbookTemplate(ctx, p, companyID, gconv.Int64(assetID))
	}
	if kind == "industry_invalid" {
		return masterImportInvalid(ctx, p, assetID)
	}
	book := excelize.NewFile()
	defer book.Close()
	headers := []interface{}{}
	rows := [][]interface{}{}
	switch kind {
	case "products":
		if _, e := company(ctx, p, companyID, "product:export", true); e != nil {
			return nil, e
		}
		list, e := g.DB().Model("bm_growth_products").Ctx(ctx).Where("company_id", companyID).WhereNull("deleted_at").Order("id").Limit(100000).All()
		if e != nil {
			return nil, e
		}
		headers = []interface{}{"SKU", "Name", "Category", "Specifications"}
		for _, r := range list {
			pr, e := Product(ctx, p, r["id"].Int64(), "product:export")
			if e != nil {
				continue
			}
			rows = append(rows, []interface{}{pr["sku"].String(), pr["name"].String(), pr["category"].String(), pr["fields"].String()})
		}
	case "exceptions":
		if _, e := Asset(ctx, p, assetID, "material:download"); e != nil {
			return nil, e
		}
		list, e := g.DB().Model("bm_growth_import_rows").Ctx(ctx).Where("asset_id", assetID).Where("state", "review").Order("sheet,source_row").Limit(100000).All()
		if e != nil {
			return nil, e
		}
		headers = []interface{}{"Sheet", "Row", "SKU", "Reason", "Proposed"}
		for _, r := range list {
			rows = append(rows, []interface{}{r["sheet"].String(), r["source_row"].Int(), r["sku"].String(), r["reason"].String(), r["proposed"].String()})
		}
	case "candidates":
		if e := require(p, "outreach:export"); e != nil {
			return nil, e
		}
		scope, e := p.Predicate(ctx, "outreach:export", "")
		if e != nil {
			return nil, e
		}
		q := g.DB().Model("bm_growth_candidates").Ctx(ctx).Where(scope)
		if companyID > 0 {
			q = q.Where("company_id", companyID)
		}
		list, e := q.Order("id").Limit(100000).All()
		if e != nil {
			return nil, e
		}
		headers = []interface{}{"Company", "Website", "Country", "Address", "Phone", "Emails", "Status", "Facts", "Sources", "Error"}
		for _, r := range list {
			rows = append(rows, []interface{}{r["company"].String(), r["website"].String(), r["country"].String(), r["address"].String(), r["phone"].String(), r["emails"].String(), r["state"].String(), r["facts"].String(), r["sources"].String(), r["error"].String()})
		}
	case "leads":
		if e := require(p, "outreach:export"); e != nil {
			return nil, e
		}
		scope, e := p.Predicate(ctx, "outreach:export", "")
		if e != nil {
			return nil, e
		}
		q := g.DB().Model("bm_growth_leads").Ctx(ctx).Where(scope)
		if companyID > 0 {
			q = q.Where("company_id", companyID)
		}
		list, e := q.Order("id").Limit(100000).All()
		if e != nil {
			return nil, e
		}
		headers = []interface{}{"Email", "Company", "Country", "Language", "Website", "Status", "Verification", "Sources"}
		for _, r := range list {
			rows = append(rows, []interface{}{r["email"].String(), r["company"].String(), r["country"].String(), r["language"].String(), r["website"].String(), r["state"].String(), r["verification"].String(), r["sources"].String()})
		}
	default:
		return nil, errors.New("unsupported workbook")
	}
	if e := book.SetSheetRow("Sheet1", "A1", &headers); e != nil {
		return nil, e
	}
	for i, r := range rows {
		cell, _ := excelize.CoordinatesToCellName(1, i+2)
		if e := book.SetSheetRow("Sheet1", cell, &r); e != nil {
			return nil, e
		}
	}
	book.SetColWidth("Sheet1", "A", "H", 24)
	body, e := book.WriteToBuffer()
	if e != nil {
		return nil, e
	}
	return body.Bytes(), nil
}
func Detail(ctx context.Context, p *governance.Principal, kind string, id int64) (g.Map, error) {
	defs := map[string][2]string{"candidates": {"bm_growth_candidates", "outreach:list"}, "plans": {"bm_growth_plans", "outreach:list"}, "leads": {"bm_growth_leads", "outreach:list"}, "messages": {"bm_growth_messages", "outreach:list"}, "runs": {"bm_growth_runs", "outreach:list"}, "inquiries": {"bm_growth_inquiries", "inquiry:list"}}
	d, ok := defs[kind]
	if !ok {
		return nil, errors.New("unsupported detail")
	}
	r, e := scoped(ctx, p, d[0], id, d[1])
	if e != nil {
		return nil, e
	}
	out := g.Map{"record": r}
	if e = recordReferences(ctx, r); e != nil {
		return nil, e
	}
	if kind == "candidates" {
		out["email_details"], e = candidateEmails(ctx, p, r)
		if e != nil {
			return nil, e
		}
	}
	if kind == "runs" {
		stats, err := g.DB().Model("bm_growth_runs r").Ctx(ctx).Where("r.id", id).Fields("(" + runStatsSQL + ") value").Value()
		if err != nil {
			return nil, err
		}
		out["live_stats"] = jsonMap(stats)
		out["queries"], e = g.DB().Model("bm_growth_queries").Where("run_id", id).Order("id").All()
	}
	if kind == "inquiries" {
		out["forwards"], e = g.DB().Model("bm_growth_forwards").Where("inquiry_id", id).All()
		out["customer_id"], _ = g.DB().Model("bm_crm_messages").Where("id", r["message_id"]).Value("customer_id")
	}
	return out, e
}
func ManualMapping(ctx context.Context, p *governance.Principal, id string, mapping g.Map) error {
	r, e := Asset(ctx, p, id, "material:manage")
	if e != nil {
		return e
	}
	for k, v := range mapping {
		if fmt.Sprint(gconv.Int(k)) != k || gconv.Int(k) < 0 || gconv.Int(k) > 16383 || gconv.String(v) == "" {
			return errors.New("invalid column mapping")
		}
	}
	extracted := jsonMap(r["extracted"])
	extracted["header_mapping"] = mapping
	_, e = g.DB().Model("bm_growth_assets").Where("id", id).Data(g.Map{"extracted": jsonText(extracted)}).Update()
	if e == nil {
		_, e = RecognizeAsset(ctx, p, id)
	}
	return e
}
func Stats(ctx context.Context, p *governance.Principal) (g.Map, error) {
	out := g.Map{}
	for _, entry := range []struct{ key, table, permission, field string }{{"jobs", "bm_growth_jobs", "material:list", "status"}, {"candidates", "bm_growth_candidates", "outreach:list", "state"}, {"leads", "bm_growth_leads", "outreach:list", "state"}, {"messages", "bm_growth_messages", "outreach:list", "status"}, {"inquiries", "bm_growth_inquiries", "inquiry:list", "kind"}} {
		if !p.Has(entry.permission) {
			continue
		}
		scope, e := p.Predicate(ctx, entry.permission, "")
		if e != nil {
			return nil, e
		}
		out[entry.key], e = g.DB().Model(entry.table).Ctx(ctx).Where(scope).Fields(entry.field + ",COUNT(*) count").Group(entry.field).All()
		if e != nil {
			return nil, e
		}
	}
	return out, nil
}
func decodeObject(raw string) (g.Map, error) {
	var result g.Map
	if e := json.Unmarshal([]byte(raw), &result); e != nil {
		return nil, fmt.Errorf("invalid object: %w", e)
	}
	return result, nil
}
func safeDownloadName(name string) string {
	return strings.NewReplacer("\r", "", "\n", "", "\"", "_").Replace(name)
}
