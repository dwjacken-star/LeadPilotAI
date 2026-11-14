package social

import (
	"context"
	"errors"
	"fmt"
	"time"

	"billionmail-core/internal/service/governance"
	"billionmail-core/internal/service/growth"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/gconv"
)

func init() {
	growth.MasterSocialCheck = func(ctx context.Context, p *governance.Principal, ids []int64) error {
		for _, id := range ids {
			if _, e := accountAccess(ctx, p, id, "social:plan:manage", true); e != nil {
				return e
			}
		}
		return nil
	}
	growth.MasterSocialPlatform = func(ctx context.Context, p *governance.Principal, id int64) (string, error) {
		a, e := accountAccess(ctx, p, id, "social:plan:manage", true)
		if e != nil {
			return "", e
		}
		return a["platform"].String(), nil
	}
	growth.MasterSocialHandoff = func(ctx context.Context, p *governance.Principal, content gdb.Record, s growth.MasterSnapshot) (int64, error) {
		for _, permission := range []string{"social:plan:manage", "social:content:edit", "social:publish"} {
			if e := require(p, permission); e != nil {
				return 0, e
			}
		}
		accountID := gconv.Int64(content["target"].String())
		account, e := accountAccess(ctx, p, accountID, "social:publish", true)
		if e != nil {
			return 0, e
		}
		if account["platform"].String() != content["channel"].String() {
			return 0, errors.New("social account does not match content channel")
		}
		existing, e := g.DB().Model("bm_social_plans").Ctx(ctx).Where("master_content_id", content["id"]).Value("id")
		if e != nil {
			return 0, e
		}
		if existing.Int64() > 0 {
			return existing.Int64(), nil
		}
		reviewer, e := governance.Load(ctx, content["approved_by"].Int64())
		if e != nil {
			return 0, e
		}
		if reviewer.Super {
			reviewer.DeptID = p.DeptID
		}
		if e = require(reviewer, "social:content:review"); e != nil {
			return 0, e
		}
		market := Market{Country: content["country"].String(), Language: content["language"].String(), Timezone: s.Config.Timezone, LocalTime: s.Config.LocalTime}
		for _, m := range s.Config.Markets {
			if m.Country == market.Country && m.Language == market.Language {
				market.Timezone = m.Timezone
			}
		}
		cfg := PlanConfig{Platforms: []string{content["channel"].String()}, Accounts: []int64{accountID}, Products: s.Config.ProductIDs, Markets: []Market{market}, Requirement: s.Config.Requirement, Cadence: "once", DailyPosts: 3, MinIntervalMinutes: 10, SyncInteractions: true}
		cfg.Assets = s.Config.ImageAssetIDs
		id, e := SavePlan(ctx, p, 0, fmt.Sprintf("Growth Master #%d", content["id"].Int64()), content["company_id"].Int64(), cfg)
		if e != nil {
			return 0, e
		}
		if _, e = g.DB().Model("bm_social_plans").Ctx(ctx).Where("id", id).Data("master_content_id", content["id"].Int64()).Update(); e != nil {
			return 0, e
		}
		if e = ManualVariants(ctx, p, id); e != nil {
			return 0, e
		}
		v, e := g.DB().Model("bm_social_variants").Ctx(ctx).Where("plan_id", id).Value("id")
		if e != nil {
			return 0, e
		}
		if e = SaveVariant(ctx, p, v.Int64(), g.Map{"title": content["subject"].String(), "body": content["body"].String(), "images": gconv.Strings(content["images"].Interface())}); e != nil {
			return 0, e
		}
		if e = ApproveVariant(ctx, reviewer, v.Int64()); e != nil {
			return 0, e
		}
		if e = RunNow(ctx, p, id); e != nil {
			return 0, e
		}
		return id, nil
	}
	growth.MasterSocialReceipt = func(ctx context.Context, p *governance.Principal, id int64) (g.Map, error) {
		if _, e := scoped(ctx, p, "bm_social_plans", id, "social:plan:view"); e != nil {
			return nil, e
		}
		d, e := g.DB().Model("bm_social_deliveries d").Ctx(ctx).InnerJoin("bm_social_accounts a", "a.id=d.account_id").Where("d.plan_id", id).WhereNull("d.deleted_at").Fields("d.*,a.platform,a.target_type,a.scopes,a.capabilities").Order("d.id DESC").One()
		if e != nil {
			return nil, e
		}
		if len(d) == 0 {
			return g.Map{}, nil
		}
		state := d["status"].String()
		switch state {
		case "scheduled", "manual_due", "retry", "publishing", "blocked":
			state = "queued"
		case "submitted":
			if linkedInPostReadable(d) || d["remote_id"].String() == "" {
				state = "queued"
			}
		}
		return g.Map{"status": state, "delivery_status": d["status"].String(), "publish_mode": publishingMode(d), "manual_confirmed_at": d["manual_confirmed_at"], "external_id": d["remote_id"].String(), "error": d["error"].String(), "as_of": time.Now()}, nil
	}
}
