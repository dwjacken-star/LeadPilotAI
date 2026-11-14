package social

import (
	"context"
	"errors"
	"time"

	"billionmail-core/internal/service/governance"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

func DeleteRecord(ctx context.Context, p *governance.Principal, kind string, id int64) error {
	if kind == "plans" {
		return PlanControl(ctx, p, id, "delete")
	}
	if kind == "accounts" {
		if e := AccountControl(ctx, p, id, "disconnect", g.Map{}); e != nil {
			return e
		}
		_, e := g.DB().Model("bm_social_accounts").Ctx(ctx).Where("id", id).Data("status", "deleted").Update()
		return e
	}
	defs := map[string]string{"deliveries": "bm_social_deliveries", "jobs": "bm_social_jobs", "interactions": "bm_social_interactions", "reports": "bm_social_metrics"}
	table, ok := defs[kind]
	if !ok {
		return errors.New("unsupported social deletion")
	}
	return g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		row, e := tx.Model(table).Where("id", id).One()
		if e != nil {
			return e
		}
		if len(row) == 0 {
			return errors.New("record unavailable")
		}
		if kind == "interactions" {
			if _, e = accountAccess(ctx, p, row["account_id"].Int64(), "social:interaction:manage", false); e != nil {
				return e
			}
		} else if kind == "reports" {
			if _, e = scoped(ctx, p, "bm_social_accounts", row["account_id"].Int64(), "social:account:manage"); e != nil {
				return e
			}
		} else if row["plan_id"].Int64() > 0 {
			if _, e = scoped(ctx, p, "bm_social_plans", row["plan_id"].Int64(), "social:plan:manage"); e != nil {
				return e
			}
			if _, e = tx.GetOne(`SELECT id FROM bm_social_plans WHERE id=$1 FOR UPDATE`, row["plan_id"]); e != nil {
				return e
			}
		} else {
			if _, e = scoped(ctx, p, "bm_social_accounts", row["account_id"].Int64(), "social:account:manage"); e != nil {
				return e
			}
		}
		// Claiming and deletion lock the same row, preventing a queued request from starting afterwards.
		row, e = tx.GetOne("SELECT * FROM "+table+" WHERE id=$1 FOR UPDATE", id)
		if e != nil {
			return e
		}
		if row["status"].String() == "running" || row["status"].String() == "publishing" {
			return errors.New("wait for in-flight processing before deleting")
		}
		values := g.Map{"deleted_at": time.Now()}
		if kind == "interactions" {
			values = g.Map{"deleted": true, "body": "", "actor_name": "", "actor_id": "", "analysis": "{}"}
		}
		if kind == "jobs" {
			values["status"] = "cancelled"
		}
		if kind == "deliveries" && (row["status"].String() == "scheduled" || row["status"].String() == "manual_due" || row["status"].String() == "retry" || row["status"].String() == "blocked") {
			values["status"] = "cancelled"
		}
		if _, e = tx.Model(table).Where("id", id).Data(values).Update(); e != nil {
			return e
		}
		return securityAudit(ctx, p, "delete_"+kind, id)
	})
}
