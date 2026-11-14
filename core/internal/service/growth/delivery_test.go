package growth

import (
	"context"
	"fmt"
	"testing"
	"time"

	"billionmail-core/internal/service/batch_mail"
	"billionmail-core/internal/service/governance"
	"billionmail-core/internal/service/public"
	"github.com/gogf/gf/v2/frame/g"
)

func exercisePlanDeliveryOptions(t *testing.T, ctx context.Context, p *governance.Principal, companyID, productID int64) {
	t.Helper()
	must(t, public.Cache.Set(ctx, "ServerIP", "127.0.0.1", time.Hour))
	on, off := true, false
	for index, test := range []struct {
		name             string
		unsubscribe      *bool
		warmup, tracking bool
	}{{"legacy", nil, false, false}, {"enabled", &on, true, true}, {"disabled", &off, false, false}} {
		t.Run(test.name, func(t *testing.T) {
			c := PlanConfig{ProductIDs: []int64{productID}, Markets: []Market{{Country: "US", Language: "en", Timezone: "UTC"}}, Requirement: "Find distributors", Senders: []string{"sender@example.com"}, DailyLeads: 10, DailySend: 10, SearchLimit: 1, PageLimit: 1, Timezone: "UTC", LocalTime: "09:00", SendStart: "00:00", SendEnd: "23:59", CooldownDays: 30, Warmup: test.warmup, Unsubscribe: test.unsubscribe, TrackOpen: test.tracking, TrackClick: test.tracking}
			id, e := SavePlan(ctx, p, 0, g.Map{"name": "delivery-options-" + test.name, "company_id": companyID, "config": c})
			must(t, e)
			raw, e := g.DB().Model("bm_growth_plans").Where("id", id).Value("config")
			must(t, e)
			var saved PlanConfig
			must(t, raw.Scan(&saved))
			if saved.deliveryOptions() != c.deliveryOptions() {
				t.Fatal("plan options did not survive persistence", saved.deliveryOptions(), c.deliveryOptions())
			}
			run, e := PlanControl(ctx, p, id, "run")
			must(t, e)
			lead, e := g.DB().Model("bm_growth_leads").Data(g.Map{"plan_id": id, "company_id": companyID, "dept_id": p.DeptID, "owner_id": p.ID, "email": fmt.Sprintf("options-%d@fixture-business.example", index), "country": "US", "language": "en", "state": "verified", "verified_until": time.Now().Add(time.Hour)}).InsertAndGetId()
			must(t, e)
			message, e := g.DB().Model("bm_growth_messages").Data(g.Map{"lead_id": lead, "run_id": run, "plan_id": id, "dept_id": p.DeptID, "owner_id": p.ID, "subject": "Delivery option fixture", "html": "<p>Offer</p>", "language": "en", "snapshot": "{}", "status": "approved"}).InsertAndGetId()
			must(t, e)
			must(t, dispatchRun(ctx, p, run))
			taskID, e := g.DB().Model("bm_growth_messages").Where("id", message).Value("task_id")
			must(t, e)
			row, e := g.DB().Model("email_tasks").Where("id", taskID.Int64()).One()
			must(t, e)
			options := c.deliveryOptions()
			if taskID.Int64() == 0 || row["unsubscribe"].Bool() != options.Unsubscribe || row["track_open"].Bool() != options.TrackOpen || row["track_click"].Bool() != options.TrackClick {
				t.Fatal("plan settings not copied to real sending task", row)
			}
			warmup, e := batch_mail.TaskWarmupEnabled(ctx, taskID.Int64())
			must(t, e)
			if (warmup == 1) != options.Warmup {
				t.Fatal("IP warmup association missing", warmup, options)
			}
			before, e := g.DB().Model("email_tasks").Count()
			must(t, e)
			must(t, dispatchRun(ctx, p, run))
			after, e := g.DB().Model("email_tasks").Count()
			must(t, e)
			if before != after {
				t.Fatal("repeat dispatch duplicated task")
			}
			_, e = PlanControl(ctx, p, id, "pause")
			must(t, e)
		})
	}
}
