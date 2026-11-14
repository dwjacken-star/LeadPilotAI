package growth

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"billionmail-core/internal/service/governance"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/gconv"
)

// Regeneration creates a new immutable run/message and preserves the old draft.
// A message that reached or might have reached SMTP can never be regenerated.
func RegenerateMessage(ctx context.Context, p *governance.Principal, id int64) (int64, error) {
	old, e := scoped(ctx, p, "bm_growth_messages", id, "outreach:manage")
	if e != nil {
		return 0, e
	}
	if p.Super {
		copy := *p
		p = &copy
		p.DeptID = old["dept_id"].Int64()
	}
	if e = require(p, "outreach:run"); e != nil {
		return 0, e
	}
	if old["status"].String() == "sent" || old["status"].String() == "unknown" {
		return 0, errors.New("a sent or uncertain message cannot be regenerated")
	}
	plan, e := scoped(ctx, p, "bm_growth_plans", old["plan_id"].Int64(), "outreach:run")
	if e != nil {
		return 0, e
	}
	if !plan["active"].Bool() {
		return 0, errors.New("enable the plan before regenerating a draft")
	}
	lead, e := scoped(ctx, p, "bm_growth_leads", old["lead_id"].Int64(), "outreach:run")
	if e != nil {
		return 0, e
	}
	if lead["suppressed"].Bool() || !lead["replied_at"].IsNil() || lead["reply_pending"].Bool() {
		return 0, errors.New("customer replied or outreach is suppressed")
	}
	var config PlanConfig
	if e = json.Unmarshal(plan["config"].Bytes(), &config); e != nil {
		return 0, e
	}
	if e = config.validate(ctx, p, plan["company_id"].Int64()); e != nil {
		return 0, e
	}
	if config.AutoSend {
		if e = require(p, "outreach:publish"); e != nil {
			return 0, e
		}
	}
	fingerprint, e := g.DB().GetOne(ctx, `SELECT c.version,(SELECT coalesce(sum(version),0) FROM bm_growth_products WHERE company_id=c.id) product_versions,(SELECT coalesce(sum(version),0) FROM bm_growth_assets WHERE company_id=c.id) asset_versions FROM bm_growth_companies c WHERE c.id=$1`, plan["company_id"])
	if e != nil {
		return 0, e
	}
	hash := sha256.Sum256([]byte(plan["config"].String() + jsonText(fingerprint)))
	key := fmt.Sprintf("regenerate:%d:%x", id, hash[:8])
	var runID int64
	e = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if _, e := tx.Exec("SELECT pg_advisory_xact_lock(614813)"); e != nil {
			return e
		}
		job, e := tx.Model("bm_growth_jobs").Where("dedupe_key", key).One()
		if e != nil {
			return e
		}
		if len(job) > 0 {
			runID = gconv.Int64(jsonMap(job["payload"])["run_id"])
			return nil
		}
		current, e := tx.GetOne(`SELECT * FROM bm_growth_messages WHERE id=$1 FOR UPDATE`, id)
		if e != nil {
			return e
		}
		if current["status"].String() == "sent" || current["status"].String() == "unknown" {
			return errors.New("message was sent or has an uncertain result")
		}
		delivery, e := tx.GetOne(`SELECT * FROM recipient_info WHERE outreach_message_id=$1 FOR UPDATE`, id)
		if e != nil {
			return e
		}
		if len(delivery) > 0 {
			state := delivery["delivery_state"].String()
			if state == "accepted" || state == "unknown" || state == "sending" || state == "claimed" {
				return errors.New("SMTP handoff already started; regeneration is blocked")
			}
			if _, e = tx.Model("recipient_info").Where("id", delivery["id"]).Data(g.Map{"delivery_state": "skipped", "is_sent": 3, "last_error": "replaced by a new reviewed generation", "lease_token": nil, "lease_until": nil}).Update(); e != nil {
				return e
			}
			if _, e = tx.Model("email_tasks").Where("id", delivery["task_id"]).Data(g.Map{"pause": 1}).Update(); e != nil {
				return e
			}
		}
		if _, e = tx.Model("bm_growth_messages").Where("id", id).Data(g.Map{"status": "cancelled", "error": "replaced by a new generation; original snapshot retained"}).Update(); e != nil {
			return e
		}
		runID, e = tx.Model("bm_growth_runs").Data(g.Map{"plan_id": plan["id"], "dept_id": p.DeptID, "owner_id": p.ID, "due_at": time.Now(), "config": plan["config"].String(), "status": "running", "stats": jsonText(g.Map{"regenerated_from": id})}).InsertAndGetId()
		if e != nil {
			return e
		}
		_, e = tx.Model("bm_growth_jobs").Data(g.Map{"dept_id": p.DeptID, "owner_id": p.ID, "kind": "verify", "resource_id": lead["id"].String(), "dedupe_key": key, "payload": jsonText(g.Map{"run_id": runID})}).Insert()
		return e
	})
	return runID, e
}
