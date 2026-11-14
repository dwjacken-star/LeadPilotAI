package growth

import (
	"billionmail-core/internal/service/governance"
	"context"
	"errors"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/gconv"
)

// Social credentials use the existing backed-up master key, never a browser key.
func SocialCredential(value string, decode bool) (string, error) { return protect(value, decode) }
func SocialPublicFields(fields g.Map) g.Map                      { return publicProductFields(fields) }
func SocialPublicFacts(extracted g.Map) g.Map {
	facts := []g.Map{}
	for _, fact := range gconv.Maps(extracted["facts"]) {
		key := gconv.String(fact["key"])
		if len(publicProductFields(g.Map{key: true})) > 0 {
			facts = append(facts, fact)
		}
	}
	return g.Map{"summary": extracted["summary"], "facts": facts}
}
func SocialLibraryOptions(ctx context.Context, p *governance.Principal, companyID int64, kind string, page int, keyword string, ids []string) (g.Map, error) {
	permission, table, fields, label := "product:list", "bm_growth_products", "id,name,sku", "name"
	if kind == "assets" {
		permission, table, fields, label = "material:list", "bm_growth_assets", "id,filename,mime,category", "filename"
	} else if kind != "products" {
		return nil, errors.New("invalid library kind")
	}
	if _, e := company(ctx, p, companyID, permission, true); e != nil {
		return nil, e
	}
	if page < 1 {
		page = 1
	}
	q := g.DB().Model(table).Ctx(ctx).Where("company_id", companyID).Where("published", true).WhereNull("deleted_at")
	if kind == "assets" {
		q = q.Where("visibility", "public").Where("ai_allowed", true).WhereNot("category", "internal")
	}
	if keyword != "" {
		if kind == "products" {
			q = q.Where("name ILIKE ? OR sku ILIKE ?", "%"+cleanText(keyword, 100)+"%", "%"+cleanText(keyword, 100)+"%")
		} else {
			q = q.Where(label+" ILIKE ?", "%"+cleanText(keyword, 100)+"%")
		}
	}
	if len(ids) > 0 {
		if len(ids) > 100 {
			return nil, errors.New("select at most 100 items")
		}
		q = q.WhereIn("id", ids)
	}
	n, e := q.Clone().Count()
	if e != nil {
		return nil, e
	}
	q = q.Fields(fields).Order("id DESC")
	if len(ids) > 0 {
		q = q.Limit(100)
	} else {
		q = q.Page(page, 20)
	}
	rows, e := q.All()
	return g.Map{"list": rows, "total": n}, e
}
func SocialModels(ctx context.Context, p *governance.Principal) (g.Map, error) {
	if p == nil || (!p.Has("social:content:edit") && !p.Has("social:interaction:analyze")) {
		return nil, errors.New("social AI permission required")
	}
	c, e := config(ctx)
	if e != nil {
		return nil, e
	}
	options, model, warning := textModelOptions(ctx, c)
	_, _, enabledError := effectiveTextConfig(ctx, model)
	return g.Map{"enabled": enabledError == nil, "model": model, "models": options, "warning": warning}, nil
}
func SocialAI(ctx context.Context, p *governance.Principal, purpose string, input AIInput, out interface{}) error {
	permission := "social:content:edit"
	if purpose == "social_classification" {
		permission = "social:interaction:analyze"
	}
	if p == nil || !p.Has(permission) {
		return errors.New("social AI permission required")
	}
	if purpose != "social_material" && purpose != "social_compose" && purpose != "social_classification" {
		return errors.New("invalid social AI purpose")
	}
	if e := validateSelectedTextModel(ctx, input.Model); e != nil {
		return e
	}
	return IntelligenceProvider.Extract(ctx, p, purpose, input, out)
}
func SocialSources(ctx context.Context, p *governance.Principal, companyID int64, products []int64, assets []string) (g.Map, error) {
	permission := "social:content:edit"
	if !p.Has(permission) {
		permission = "social:content:review"
	}
	if !p.Has(permission) {
		permission = "social:publish"
	}
	c, e := company(ctx, p, companyID, permission, true)
	if e != nil {
		return nil, e
	}
	out := g.Map{"company_id": companyID, "company_name": c["name"].String(), "brand": c["brand"].String(), "company": jsonMap(c["published_profile"]), "company_version": c["version"].Int()}
	ps := []g.Map{}
	as := []g.Map{}
	for _, id := range products {
		r, e := Product(ctx, p, id, "product:list")
		if e != nil {
			return nil, e
		}
		if r["company_id"].Int64() != companyID || !r["published"].Bool() {
			return nil, errors.New("select published products from this company")
		}
		ps = append(ps, g.Map{"id": id, "version": r["version"].Int(), "fields": jsonMap(r["published_fields"])})
	}
	for _, id := range assets {
		r, e := Asset(ctx, p, id, "material:list")
		if e != nil {
			return nil, e
		}
		if r["company_id"].Int64() != companyID || !r["published"].Bool() || r["visibility"].String() != "public" || !r["ai_allowed"].Bool() || r["category"].String() == "internal" {
			return nil, errors.New("select approved AI-enabled materials")
		}
		as = append(as, g.Map{"id": id, "version": r["version"].Int(), "filename": r["filename"].String(), "extracted": SocialPublicFacts(jsonMap(r["extracted"]))})
	}
	out["products"], out["materials"] = ps, as
	return out, nil
}
