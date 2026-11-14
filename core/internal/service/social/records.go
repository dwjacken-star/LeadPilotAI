package social

import (
	"context"
	"errors"
	"fmt"
	"time"

	"billionmail-core/internal/service/governance"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/gconv"
)

func accountPredicate(ctx context.Context, p *governance.Principal, permission, alias string, publish bool) (string, error) {
	if e := require(p, permission); e != nil {
		return "", e
	}
	predicate, e := p.Predicate(ctx, permission, alias)
	if e != nil {
		return "", e
	}
	if p.Super {
		return predicate, nil
	}
	if alias != "" {
		alias += "."
	}
	column := "can_read"
	if publish {
		column = "can_publish"
	}
	return "((" + predicate + fmt.Sprintf(" AND %sdept_id=%d) OR EXISTS(SELECT 1 FROM bm_social_account_grants x WHERE x.account_id=%sid AND x.dept_id=%d AND x.%s))", alias, p.DeptID, alias, p.DeptID, column), nil
}
func List(ctx context.Context, p *governance.Principal, kind string, planID int64, page int, keyword, status string, filters ...QueryFilter) (g.Map, error) {
	f, e := queryFilter(filters)
	if e != nil {
		return nil, e
	}
	if page < 1 {
		page = 1
	}
	keyword = str(keyword, 100)
	var q *gdb.Model
	switch kind {
	case "accounts":
		predicate, e := accountPredicate(ctx, p, "social:account:view", "a", false)
		if e != nil {
			return nil, e
		}
		q = g.DB().Model("bm_social_accounts a").Ctx(ctx).Where(predicate).Where("a.status<>?", "deleted").Fields("a.id,a.platform,a.target_type,a.publish_mode,a.external_id,a.provider_name,a.name,a.dept_id,a.owner_id,a.expires_at,a.scopes,a.capabilities,a.status,a.error,a.last_sync_at,a.next_sync_at")
		if keyword != "" {
			q = q.Where("a.name ILIKE ?", "%"+keyword+"%")
		}
		if status != "" {
			q = q.Where("a.status", status)
		}
	case "plans":
		predicate, e := p.Predicate(ctx, "social:plan:view", "p")
		if e != nil {
			return nil, e
		}
		q = g.DB().Model("bm_social_plans p").Ctx(ctx).Where(predicate).LeftJoin("bm_growth_companies c", "c.id=p.company_id").Fields("p.*,c.name AS company_name")
		if keyword != "" {
			q = q.Where("p.name ILIKE ?", "%"+keyword+"%")
		}
		if status != "" {
			q = q.Where("p.status", status)
		}
	case "deliveries":
		predicate, e := p.Predicate(ctx, "social:plan:view", "p")
		if e != nil {
			return nil, e
		}
		q = g.DB().Model("bm_social_deliveries d").Ctx(ctx).InnerJoin("bm_social_plans p", "p.id=d.plan_id").InnerJoin("bm_social_accounts a", "a.id=d.account_id").InnerJoin("bm_social_variants v", "v.id=d.variant_id").Where(predicate).Fields("d.id,d.plan_id,d.variant_id,d.account_id,d.status,d.publish_mode,d.manual_confirmed_at,d.manual_confirmed_by,d.remote_id,d.remote_url,d.error,d.due_at,d.submitted_at,d.published_at,d.attempts,d.sync_state,d.sync_error,d.last_sync_at,p.name AS plan_name,a.name AS account_name,a.platform,a.target_type,v.country,v.language,v.title")
		if planID > 0 {
			q = q.Where("d.plan_id", planID)
		}
		if keyword != "" {
			q = q.Where("p.name ILIKE ? OR a.name ILIKE ?", "%"+keyword+"%", "%"+keyword+"%")
		}
		if status != "" {
			q = q.Where("d.status", status)
		}
	case "jobs":
		predicate, e := p.Predicate(ctx, "social:plan:view", "j")
		if e != nil {
			return nil, e
		}
		q = g.DB().Model("bm_social_jobs j").Ctx(ctx).LeftJoin("bm_social_plans p", "p.id=j.plan_id").LeftJoin("bm_social_accounts a", "a.id=j.account_id").Where(predicate).Fields("j.id,j.plan_id,j.account_id,j.kind,j.status,j.error,j.attempts,j.updated_at,p.name AS plan_name")
		if planID > 0 {
			q = q.Where("j.plan_id", planID)
		}
		if status != "" {
			q = q.Where("j.status", status)
		}
	case "interactions":
		predicate, e := accountPredicate(ctx, p, "social:interaction:view", "a", false)
		if e != nil {
			return nil, e
		}
		q = g.DB().Model("bm_social_interactions i").Ctx(ctx).InnerJoin("bm_social_accounts a", "a.id=i.account_id").InnerJoin("bm_social_deliveries d", "d.id=i.delivery_id").InnerJoin("bm_social_plans p", "p.id=d.plan_id").Where(predicate).Where("i.expires_at>now()").Where("i.deleted", false).Fields("i.id,i.account_id,i.delivery_id,i.platform,i.kind,i.actor_id,CASE WHEN i.profile_expires_at>now() THEN i.actor_name ELSE '' END AS actor_name,i.body,i.parent_id,i.state,i.assigned_to,(SELECT username FROM account WHERE account_id=i.assigned_to) AS assigned_name,i.occurred_at,i.expires_at,i.last_seen_at,CASE WHEN i.analysis_expires_at>now() THEN i.analysis ELSE '{}'::jsonb END AS analysis,a.name AS account_name,p.name AS plan_name,d.remote_url")
		if planID > 0 {
			q = q.Where("d.plan_id", planID)
		}
		if keyword != "" {
			q = q.Where("i.body ILIKE ?", "%"+keyword+"%")
		}
		if status != "" {
			q = q.Where("i.state", status)
		}
	default:
		return nil, errors.New("unknown social list")
	}
	switch kind {
	case "plans":
		q = q.Where("p.status<>?", "deleted")
		if f.Platform != "" {
			q = q.Where("p.config->'platforms' @> ?::jsonb", encode([]string{f.Platform}))
		}
		if f.AccountID > 0 {
			q = q.Where("p.config->'accounts' @> ?::jsonb", encode([]int64{f.AccountID}))
		}
	case "jobs":
		q = q.Where("p.id IS NULL OR p.status<>'deleted'")
		if f.AccountID > 0 {
			q = q.Where("j.account_id=? OR (j.account_id IS NULL AND p.config->'accounts' @> ?::jsonb)", f.AccountID, encode([]int64{f.AccountID}))
		}
		if f.Platform != "" {
			q = q.Where("a.platform=? OR (j.account_id IS NULL AND p.config->'platforms' @> ?::jsonb)", f.Platform, encode([]string{f.Platform}))
		}
	default:
		q = filterAccounts(q, "a", f)
		if kind != "accounts" {
			q = q.Where("p.status<>?", "deleted")
		}
	}
	if kind == "deliveries" {
		q = q.WhereNull("d.deleted_at")
	}
	if kind == "jobs" {
		q = q.WhereNull("j.deleted_at")
	}
	orders := map[string]string{"accounts": "a.id DESC", "plans": "p.id DESC", "deliveries": "d.id DESC", "jobs": "j.id DESC", "interactions": "i.id DESC"}
	rows, count, e := q.Order(orders[kind]).Page(page, 20).AllAndCount(false)
	if rows == nil {
		rows = gdb.Result{}
	}
	return g.Map{"list": rows, "total": count, "page_size": 20}, e
}
func Detail(ctx context.Context, p *governance.Principal, id int64) (g.Map, error) {
	plan, e := scoped(ctx, p, "bm_social_plans", id, "social:plan:view")
	if e != nil {
		return nil, e
	}
	variants, e := g.DB().Model("bm_social_variants").Where("plan_id", id).Where("plan_version", plan["version"]).Fields("id,plan_id,plan_version,platform,country,language,timezone,title,body,link,images,version,state,approved_at").Order("platform,country,language").All()
	if e != nil {
		return nil, e
	}
	assets, e := g.DB().Model("bm_social_assets").Where("plan_id", id).Fields("id,plan_id,filename,mime,size,parent_id,category,status,error,approved,version,ai_allowed,extracted").Order("created_at,id").All()
	if e != nil {
		return nil, e
	}
	return g.Map{"plan": plan, "variants": variants, "assets": assets}, nil
}
func Reports(ctx context.Context, p *governance.Principal, planID int64, filters ...QueryFilter) (g.Map, error) {
	f, err := queryFilter(filters)
	if err != nil {
		return nil, err
	}
	predicate, e := accountPredicate(ctx, p, "social:report:view", "a", false)
	if e != nil {
		return nil, e
	}
	q := g.DB().Model("bm_social_metrics m").Ctx(ctx).InnerJoin("bm_social_deliveries d", "d.id=m.delivery_id").InnerJoin("bm_social_accounts a", "a.id=m.account_id").InnerJoin("bm_social_plans p", "p.id=d.plan_id").InnerJoin("bm_social_variants v", "v.id=d.variant_id").Where(predicate).Where("m.expires_at>now()").WhereNull("m.deleted_at").Fields("m.id,m.account_id,m.delivery_id,m.counts,m.availability,m.as_of,m.expires_at,d.sync_state,d.sync_error,d.last_sync_at,p.name AS plan_name,a.name AS account_name,a.platform,v.country AS target_country,v.language,d.remote_url")
	if planID > 0 {
		q = q.Where("d.plan_id", planID)
	}
	q = filterAccounts(q, "a", f)
	rows, e := q.Order("m.as_of DESC").Limit(500).All()
	if e != nil {
		return nil, e
	}
	return g.Map{"rows": rows, "limit": 500, "as_of": time.Now(), "country_basis": "configured_target_country", "reach_is_not_cross_post_unique": true, "source": "official_platform_aggregate", "member_export_supported": false}, nil
}
func Calendar(ctx context.Context, p *governance.Principal, from, to string, filters ...QueryFilter) (gdb.Result, error) {
	f, err := queryFilter(filters)
	if err != nil {
		return nil, err
	}
	predicate, e := p.Predicate(ctx, "social:plan:view", "p")
	if e != nil {
		return nil, e
	}
	start, e := time.Parse(time.RFC3339, from)
	if e != nil {
		return nil, errors.New("calendar start must be RFC3339")
	}
	end, e := time.Parse(time.RFC3339, to)
	if e != nil || end.Before(start) || end.Sub(start) > 93*24*time.Hour {
		return nil, errors.New("calendar range must be within 93 days")
	}
	return filterAccounts(g.DB().Model("bm_social_deliveries d").Ctx(ctx).InnerJoin("bm_social_plans p", "p.id=d.plan_id").InnerJoin("bm_social_accounts a", "a.id=d.account_id").InnerJoin("bm_social_variants v", "v.id=d.variant_id").Where(predicate).Where("p.status<>?", "deleted").WhereNull("d.deleted_at"), "a", f).Where("d.due_at>=? AND d.due_at<?", start, end).Fields("d.id,d.plan_id,d.status,d.due_at,d.error,d.publish_mode,d.manual_confirmed_at,d.manual_confirmed_by,p.name AS plan_name,a.name AS account_name,a.platform,a.target_type,v.country,v.language,v.title").Order("d.due_at,d.id").Limit(1000).All()
}
func RetryJob(ctx context.Context, p *governance.Principal, id int64) error {
	row, e := g.DB().Model("bm_social_jobs").Where("id", id).WhereNull("deleted_at").One()
	if e != nil {
		return e
	}
	if len(row) == 0 {
		return errors.New("job unavailable")
	}
	permission := "social:content:edit"
	if row["kind"].String() == "sync" {
		permission = "social:interaction:view"
	}
	if row["kind"].String() == "analyze" {
		permission = "social:interaction:analyze"
	}
	if row["plan_id"].Int64() > 0 {
		if _, e = scoped(ctx, p, "bm_social_plans", row["plan_id"].Int64(), permission); e != nil {
			return e
		}
	} else if row["account_id"].Int64() > 0 {
		if _, e = accountAccess(ctx, p, row["account_id"].Int64(), permission, false); e != nil {
			return e
		}
	} else {
		return errors.New("job resource unavailable")
	}
	if row["status"].String() != "failed" && row["status"].String() != "configuration_required" {
		return errors.New("job is not retryable")
	}
	_, e = g.DB().Model("bm_social_jobs").Where("id", id).Data(g.Map{"status": "queued", "owner_id": p.ID, "dept_id": p.DeptID, "next_at": time.Now(), "error": ""}).Update()
	return e
}
func Departments(ctx context.Context, p *governance.Principal) (gdb.Result, error) {
	if !p.Super {
		return nil, errors.New("administrator required")
	}
	return g.DB().Model("bm_departments").Ctx(ctx).Where("active", true).Fields("id,name").Order("id").All()
}
func Assignees(ctx context.Context, p *governance.Principal, accountID int64) ([]g.Map, error) {
	if _, e := accountAccess(ctx, p, accountID, "social:interaction:manage", false); e != nil {
		return nil, e
	}
	rows, e := g.DB().Model("account").Where("status", 1).Fields("account_id,username").Limit(200).All()
	if e != nil {
		return nil, e
	}
	out := []g.Map{}
	for _, row := range rows {
		other, e := governance.Load(ctx, row["account_id"].Int64())
		if e != nil {
			continue
		}
		if _, e = accountAccess(ctx, other, accountID, "social:interaction:view", false); e == nil {
			out = append(out, g.Map{"id": row["account_id"].Int64(), "name": row["username"].String()})
		}
	}
	return out, nil
}

func DeliveryDetail(ctx context.Context, p *governance.Principal, id int64) (gdb.Record, error) {
	row, e := g.DB().Model("bm_social_deliveries d").Ctx(ctx).InnerJoin("bm_social_accounts a", "a.id=d.account_id").Where("d.id", id).WhereNull("d.deleted_at").Fields("d.*,a.platform,a.target_type,a.name AS account_name").One()
	if e != nil {
		return nil, e
	}
	if len(row) == 0 {
		return nil, errors.New("delivery unavailable")
	}
	if _, e = scoped(ctx, p, "bm_social_plans", row["plan_id"].Int64(), "social:plan:view"); e != nil {
		return nil, e
	}
	delete(row, "lease_token")
	if platform := row["platform"].String(); platform == "instagram" || platform == "threads" || (platform == "facebook" && gconv.String(object(row["snapshot"])["video"]) != "") {
		progress, err := g.DB().Model("bm_social_provider_operations").Ctx(ctx).Where("delivery_id", id).Value("progress")
		if err != nil {
			return nil, err
		}
		if !progress.IsEmpty() {
			value := object(progress)
			safe := g.Map{}
			for _, key := range []string{"pending", "polls", "created_at", "media_type", "offset", "uploaded", "finish_sent", "container", "remote_id", "video_id", "ready_children"} {
				if v, ok := value[key]; ok {
					safe[key] = v
				}
			}
			row["provider_progress"] = g.NewVar(safe)
		}
	}
	return row, nil
}
