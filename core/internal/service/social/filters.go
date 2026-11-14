package social

import (
	"context"
	"errors"

	"billionmail-core/internal/service/governance"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

type QueryFilter struct {
	Platform  string `json:"platform"`
	AccountID int64  `json:"account_id"`
}

func queryFilter(filters []QueryFilter) (QueryFilter, error) {
	f := QueryFilter{}
	if len(filters) > 0 {
		f = filters[0]
	}
	if (f.Platform != "" && !platformOK(f.Platform)) || f.AccountID < 0 {
		return f, errors.New("invalid social account or platform filter")
	}
	return f, nil
}

func filterAccounts(q *gdb.Model, alias string, f QueryFilter) *gdb.Model {
	if f.Platform != "" {
		q = q.Where(alias+".platform", f.Platform)
	}
	if f.AccountID > 0 {
		q = q.Where(alias+".id", f.AccountID)
	}
	return q
}

// Manual refresh uses the same persistent, deduplicated queue as the scheduler.
// Read access is checked separately from publishing authorization.
func SyncReports(ctx context.Context, p *governance.Principal, f QueryFilter) (g.Map, error) {
	if _, e := queryFilter([]QueryFilter{f}); e != nil {
		return nil, e
	}
	predicate, e := accountPredicate(ctx, p, "social:report:view", "a", false)
	if e != nil {
		return nil, e
	}
	rows, e := filterAccounts(g.DB().Model("bm_social_accounts a").Ctx(ctx).Where(predicate).Where("a.status", "connected").Where("a.publish_mode", "automatic"), "a", f).Fields("a.id,a.platform,a.target_type,a.publish_mode,a.scopes,a.capabilities").All()
	if e != nil {
		return nil, e
	}
	queued := 0
	for _, row := range rows {
		if !accountSyncSupported(row) {
			continue
		}
		if _, e = requirePlatform(ctx, row["platform"].String(), false); e != nil {
			return nil, e
		}
		if _, e = queue(ctx, p, 0, row["id"].Int64(), "sync", "", g.Map{"permission": "social:report:view"}); e != nil {
			return nil, e
		}
		queued++
	}
	return g.Map{"queued": queued, "sync_minutes": 240}, nil
}

func FilterAccountOptions(ctx context.Context, p *governance.Principal, permission string, f QueryFilter, page int, keyword string) (g.Map, error) {
	allowed := map[string]bool{"social:plan:view": true, "social:account:view": true, "social:interaction:view": true, "social:report:view": true}
	if !allowed[permission] {
		return nil, errors.New("invalid account selector scope")
	}
	if _, e := queryFilter([]QueryFilter{f}); e != nil {
		return nil, e
	}
	predicate, e := accountPredicate(ctx, p, permission, "a", false)
	if e != nil {
		return nil, e
	}
	if page < 1 {
		page = 1
	}
	q := filterAccounts(g.DB().Model("bm_social_accounts a").Ctx(ctx).Where(predicate).Where("a.status<>?", "deleted"), "a", f)
	if permission == "social:report:view" || permission == "social:interaction:view" {
		q = q.Where("a.publish_mode", "automatic")
	}
	if keyword != "" {
		q = q.Where("a.name ILIKE ?", "%"+str(keyword, 100)+"%")
	}
	rows, total, e := q.Fields("a.id,a.name,a.platform,a.status,a.target_type,a.publish_mode").Order("a.id DESC").Page(page, 20).AllAndCount(false)
	if rows == nil {
		rows = gdb.Result{}
	}
	return g.Map{"list": rows, "total": total}, e
}
