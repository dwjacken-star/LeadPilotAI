package growth

import (
	"billionmail-core/internal/service/governance"
	"context"
	"errors"
	"github.com/gogf/gf/v2/frame/g"
	"strings"
)

func masterOptions(ctx context.Context, p *governance.Principal, cid int64, kind string, page int, keyword string, ids []int64) (g.Map, error) {
	if e := require(p, "intelligence:view"); e != nil {
		return nil, e
	}
	table, permission, search, fields := "", "intelligence:view", "", ""
	switch kind {
	case "companies":
		table, search, fields = "bm_growth_companies", "name", "id,name"
	case "products":
		table, permission, search, fields = "bm_growth_products", "product:list", "sku||' '||name", "id,name,sku"
	case "prospects":
		table, search, fields = "bm_gm_prospects", "company||' '||email", "id,company,email,state,score"
	case "rules":
		table, permission, search, fields = "bm_growth_inbox_rules", "inquiry:list", "mailbox", "id,mailbox"
	default:
		return nil, errors.New("invalid option list")
	}
	if kind != "companies" {
		if _, e := company(ctx, p, cid, "intelligence:view", false); e != nil {
			return nil, e
		}
	}
	scope, e := p.Predicate(ctx, permission, "")
	if e != nil {
		return nil, e
	}
	q := g.DB().Model(table).Ctx(ctx).Where(scope)
	if kind == "companies" || kind == "products" {
		q = q.WhereNull("deleted_at")
	}
	if kind != "companies" {
		q = q.Where("company_id", cid)
	}
	if kind == "products" {
		q = q.Where("published", true)
	}
	if kind == "prospects" {
		q = q.Where("state", "verified").Where("suppressed", false)
	}
	if keyword != "" {
		q = q.Where("("+search+") ILIKE ?", "%"+cleanText(keyword, 100)+"%")
	}
	if len(ids) > 0 {
		if len(ids) > 100 {
			return nil, errors.New("at most 100 choices")
		}
		q = q.WhereIn("id", ids)
	}
	n, e := q.Clone().Count()
	if e != nil {
		return nil, e
	}
	if page < 1 {
		page = 1
	}
	q = q.Fields(fields).Order("id DESC")
	if len(ids) > 0 {
		q = q.Limit(100)
	} else {
		q = q.Page(page, 20)
	}
	rows, e := q.All()
	options := []g.Map{}
	for _, r := range rows {
		label := r["name"].String()
		switch kind {
		case "products":
			label = r["sku"].String() + " · " + label
		case "prospects":
			label = strings.TrimSpace(r["company"].String() + " · " + r["email"].String())
		case "rules":
			label = r["mailbox"].String()
		}
		options = append(options, g.Map{"value": r["id"].Int64(), "label": label})
	}
	return g.Map{"list": options, "total": n}, e
}
