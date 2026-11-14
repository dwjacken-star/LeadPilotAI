package social

import (
	"billionmail-core/internal/service/governance"
	"context"
	"github.com/gogf/gf/v2/frame/g"
)

func AccountOptions(ctx context.Context, p *governance.Principal, page int, keyword string, ids []int64, platforms ...[]string) (g.Map, error) {
	permission := "social:plan:manage"
	publishing := true
	if !p.Has(permission) {
		permission = "social:publish"
		if !p.Has(permission) {
			permission = "social:plan:view"
			publishing = false
		}
	}
	predicate, e := accountPredicate(ctx, p, permission, "a", publishing)
	if e != nil {
		return nil, e
	}
	if page < 1 {
		page = 1
	}
	q := g.DB().Model("bm_social_accounts a").Ctx(ctx).Where(predicate).Where("a.status", "connected")
	if len(platforms) > 0 && len(platforms[0]) > 0 {
		q = q.WhereIn("a.platform", platforms[0])
	}
	if keyword != "" {
		q = q.Where("a.name ILIKE ?", "%"+str(keyword, 100)+"%")
	}
	if len(ids) > 0 && len(ids) <= 30 {
		q = q.WhereIn("a.id", ids)
	}
	n, e := q.Clone().Count()
	if e != nil {
		return nil, e
	}
	q = q.Fields("a.id,a.name,a.platform,a.target_type,a.publish_mode,a.status,a.capabilities").Order("a.id DESC")
	if len(ids) > 0 {
		q = q.Limit(30)
	} else {
		q = q.Page(page, 20)
	}
	rows, e := q.All()
	return g.Map{"list": rows, "total": n}, e
}
