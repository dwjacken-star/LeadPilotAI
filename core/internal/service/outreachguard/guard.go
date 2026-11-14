package outreachguard

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"billionmail-core/internal/service/governance"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// Content runs immediately before SMTP handoff as well as during rendering.
// It performs authorization/state checks only, never recipient network probes.
func Content(ctx context.Context, p *governance.Principal, id int64) (string, string, error) {
	if !p.Has("outreach:publish") {
		return "", "", errors.New("outreach publishing permission revoked")
	}
	scope, e := p.Predicate(ctx, "outreach:publish", "m")
	if e != nil {
		return "", "", e
	}
	r, e := g.DB().Model("bm_growth_messages m").Ctx(ctx).LeftJoin("bm_growth_leads l", "l.id=m.lead_id").LeftJoin("bm_growth_plans p", "p.id=m.plan_id").Fields("m.*,l.state verification_state,l.suppressed,l.replied_at,l.reply_pending,l.verified_until,p.active,p.config plan_config").Where("m.id", id).WhereNull("m.deleted_at").WhereNull("l.deleted_at").WhereNull("p.deleted_at").Where(scope).One()
	if e != nil || len(r) == 0 {
		return "", "", errors.New("outreach message unavailable")
	}
	if r["verification_state"].String() != "verified" || !r["active"].Bool() || r["suppressed"].Bool() || !r["replied_at"].IsNil() || r["reply_pending"].Bool() || !r["verified_until"].Time().After(time.Now()) || r["status"].String() != "queued" {
		return "", "", errors.New("outreach paused, replied, unverified or already handled")
	}
	var snapshot struct {
		Materials struct {
			ContextMode     string         `json:"context_mode"`
			CompanyID       int64          `json:"company_id"`
			CompanyVersion  int            `json:"company_version"`
			ProductVersions map[string]int `json:"product_versions"`
			Materials       []struct {
				ID      string `json:"id"`
				Version int    `json:"version"`
			} `json:"materials"`
		} `json:"materials"`
		PlanConfig string `json:"plan_config"`
	}
	if e = json.Unmarshal(r["snapshot"].Bytes(), &snapshot); e != nil {
		return "", "", e
	}
	if snapshot.PlanConfig != "" && snapshot.PlanConfig != r["plan_config"].String() {
		return "", "", errors.New("plan changed; regenerate this message")
	}
	permission := "product:list"
	if snapshot.Materials.ContextMode == "company" {
		permission = "outreach:publish"
	}
	allowed, e := p.Predicate(ctx, permission, "c")
	if e != nil {
		return "", "", e
	}
	n, e := g.DB().Model("bm_growth_companies c").Ctx(ctx).Where("c.id", snapshot.Materials.CompanyID).Where("c.version", snapshot.Materials.CompanyVersion).WhereNull("c.deleted_at").Where("(" + allowed + fmt.Sprintf(" OR EXISTS(SELECT 1 FROM bm_growth_grants x WHERE x.company_id=c.id AND x.dept_id=%d AND x.use_materials))", p.DeptID)).Count()
	if e != nil || n == 0 {
		return "", "", errors.New("company materials changed or access revoked")
	}
	for product, version := range snapshot.Materials.ProductVersions {
		n, e = g.DB().Model("bm_growth_products").Ctx(ctx).Where("id", product).Where("company_id", snapshot.Materials.CompanyID).Where("published", true).WhereNull("deleted_at").Where("version", version).Count()
		if e != nil || n == 0 {
			return "", "", errors.New("product materials changed; regenerate this message")
		}
	}
	for _, asset := range snapshot.Materials.Materials {
		n, e = g.DB().Model("bm_growth_assets").Ctx(ctx).Where("id", asset.ID).Where("version", asset.Version).Where("published", true).Where("visibility", "public").WhereNull("deleted_at").Count()
		if e != nil || n == 0 {
			return "", "", errors.New("published material withdrawn")
		}
	}
	return r["subject"].String(), r["html"].String(), nil
}
func Finish(ctx context.Context, id int64, state, reason string) {
	if id <= 0 {
		return
	}
	status := map[string]string{"accepted": "sent", "retry": "queued", "unknown": "unknown", "failed": "failed", "skipped": "failed"}[state]
	if status == "" {
		return
	}
	_ = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		r, e := tx.Model("bm_growth_messages").Where("id", id).One()
		if e != nil || len(r) == 0 {
			return e
		}
		if _, e = tx.Model("bm_growth_messages").Where("id", id).Data(g.Map{"status": status, "error": reason}).Update(); e != nil {
			return e
		}
		if state == "accepted" {
			_, e = tx.Model("bm_growth_leads").Where("id", r["lead_id"]).Data(g.Map{"state": "contacted", "last_contact_at": time.Now()}).Update()
		}
		return e
	})
}

// A plan spans many one-contact campaigns. Its shared limit makes the plan's
// send interval, concurrency and daily ceiling apply across all those tasks.
func Limit(ctx context.Context, id int64) (governance.Limit, error) {
	r, e := g.DB().Model("bm_growth_messages m").Ctx(ctx).LeftJoin("bm_growth_plans p", "p.id=m.plan_id").Fields("p.id,p.config").Where("m.id", id).One()
	if e != nil || len(r) == 0 {
		return governance.Limit{}, errors.New("outreach plan unavailable")
	}
	var c struct {
		Policy    governance.Policy `json:"policy"`
		DailySend int               `json:"daily_send"`
	}
	if e = json.Unmarshal(r["config"].Bytes(), &c); e != nil {
		return governance.Limit{}, e
	}
	if c.DailySend < 1 {
		return governance.Limit{}, errors.New("invalid plan daily send limit")
	}
	if c.Policy.PerDay == 0 || c.Policy.PerDay > c.DailySend {
		c.Policy.PerDay = c.DailySend
	}
	c.Policy.Timezone, c.Policy.WindowStart, c.Policy.WindowEnd = "", "", ""
	c.Policy.Weekdays = nil
	return governance.Limit{Key: fmt.Sprintf("growth-plan:%d", r["id"].Int64()), Policy: c.Policy}, nil
}
