package social

import (
	"context"
	"errors"
	"fmt"
	"time"

	"billionmail-core/internal/service/governance"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

func savePlanOnce(ctx context.Context, p *governance.Principal, key, name string, companyID int64, c PlanConfig) (int64, error) {
	if len(key) < 16 || len(key) > 100 {
		return 0, errors.New("invalid draft request key")
	}
	if e := require(p, "social:plan:manage"); e != nil {
		return 0, e
	}
	var id int64
	e := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if _, e := tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, fmt.Sprintf("social-draft:%d:%d:%s", p.ID, p.DeptID, key)); e != nil {
			return e
		}
		v, e := tx.Model("bm_social_plans").Where("owner_id", p.ID).Where("dept_id", p.DeptID).Where("request_key", key).Value("id")
		if e != nil {
			return e
		}
		id, e = SavePlan(ctx, p, v.Int64(), name, companyID, c)
		if e != nil {
			return e
		}
		_, e = tx.Model("bm_social_plans").Where("id", id).Data("request_key", key).Update()
		return e
	})
	return id, e
}

// Remove the task from operations while retaining published receipts and metrics.
// Deleting a local task never deletes a post on the social platform.
func deletePlan(ctx context.Context, p *governance.Principal, plan gdb.Record) error {
	id := plan["id"].Int64()
	return g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		live, e := tx.GetOne(`SELECT status FROM bm_social_plans WHERE id=$1 FOR UPDATE`, id)
		if e != nil {
			return e
		}
		if live["status"].String() == "generating" {
			return errors.New("wait for content generation before deleting")
		}
		n, e := tx.Model("bm_social_deliveries").Where("plan_id", id).Where("status", "publishing").Count()
		if e != nil {
			return e
		}
		if n > 0 {
			return errors.New("wait for in-flight publishing before deleting")
		}
		n, e = tx.Model("bm_social_jobs").Where("plan_id", id).Where("status", "running").Count()
		if e != nil {
			return e
		}
		if n > 0 {
			return errors.New("wait for running material or image processing before deleting")
		}
		if _, e = tx.Model("bm_social_plans").Where("id", id).Data(g.Map{"status": "deleted", "next_at": nil, "updated_at": time.Now()}).Update(); e != nil {
			return e
		}
		if _, e = tx.Model("bm_social_deliveries").Where("plan_id", id).WhereIn("status", []string{"scheduled", "manual_due", "retry", "blocked"}).Data(g.Map{"status": "cancelled", "error": "Task deleted by operator"}).Update(); e != nil {
			return e
		}
		if _, e = tx.Model("bm_social_jobs").Where("plan_id", id).WhereIn("status", []string{"queued", "failed", "configuration_required"}).Data(g.Map{"status": "cancelled", "error": "Task deleted by operator"}).Update(); e != nil {
			return e
		}
		return securityAudit(ctx, p, "delete_plan", id)
	})
}
