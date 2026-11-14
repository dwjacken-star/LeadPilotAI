package social

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"billionmail-core/internal/service/governance"
	"billionmail-core/internal/service/growth"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

func exerciseRunNow(t *testing.T, ctx context.Context, p *governance.Principal, company int64) {
	t.Helper()
	token, e := growth.SocialCredential("run-now-fixture-token", false)
	must(t, e)
	account, e := g.DB().Model("bm_social_accounts").Data(g.Map{"platform": "facebook", "external_id": "run-now-fixture-page", "name": "Run now fixture", "dept_id": p.DeptID, "owner_id": p.ID, "token": token, "capabilities": encode(g.Map{"publish": true})}).InsertAndGetId()
	must(t, e)
	makePlan := func(t *testing.T, cadence string, review bool) (int64, int64) {
		c := PlanConfig{Platforms: []string{"facebook"}, Accounts: []int64{account}, Markets: []Market{{Country: "US", Language: "en", Timezone: "Asia/Shanghai", LocalTime: "10:22:37"}}, StartDate: time.Now().AddDate(0, 0, 14).Format("2006-01-02"), Cadence: cadence, DailyPosts: 3, MinIntervalMinutes: 5}
		id, e := SavePlan(ctx, p, 0, "Run now fixture", company, c)
		must(t, e)
		must(t, ManualVariants(ctx, p, id))
		value, e := g.DB().Model("bm_social_variants").Where("plan_id", id).Value("id")
		must(t, e)
		v := value.Int64()
		must(t, SaveVariant(ctx, p, v, g.Map{"body": "Run now fixture body"}))
		if review {
			must(t, ApproveVariant(ctx, p, v))
		}
		return id, v
	}
	deliveries := func(id int64) gdb.Result {
		rows, e := g.DB().Model("bm_social_deliveries").Where("plan_id", id).All()
		must(t, e)
		return rows
	}
	t.Run("review permission and terminal plan states", func(t *testing.T) {
		id, _ := makePlan(t, "once", false)
		if e = RunNow(ctx, p, id); e == nil {
			t.Fatal("unreviewed content was started")
		}
		if len(deliveries(id)) != 0 {
			t.Fatal("unreviewed batch was queued")
		}
		manager := &governance.Principal{ID: p.ID, DeptID: p.DeptID, Roles: []governance.Role{{Scope: "all", Permissions: []string{"social:plan:manage"}}}}
		if e = PlanControl(ctx, manager, id, "run_now"); e == nil {
			t.Fatal("plan manager without publish permission could run")
		}
		for _, status := range []string{"completed", "archived", "generating"} {
			_, e = g.DB().Model("bm_social_plans").Where("id", id).Data("status", status).Update()
			must(t, e)
			if RunNow(ctx, p, id) == nil {
				t.Fatal("terminal or generating plan was restarted", status)
			}
		}
	})
	t.Run("immediate first run concurrent clicks and no replay", func(t *testing.T) {
		id, _ := makePlan(t, "once", true)
		before := time.Now().Add(-time.Second)
		must(t, PlanControl(ctx, p, id, "run_now"))
		rows := deliveries(id)
		if len(rows) != 1 || rows[0]["due_at"].Time().Before(before) || rows[0]["due_at"].Time().After(time.Now()) || rows[0]["next_at"].Time().After(time.Now()) {
			t.Fatal("not immediately due", rows)
		}
		originalDue := rows[0]["due_at"].Time()
		var wg sync.WaitGroup
		errs := make(chan error, 5)
		for i := 0; i < 5; i++ {
			wg.Add(1)
			go func() { defer wg.Done(); errs <- RunNow(ctx, p, id) }()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			must(t, err)
		}
		rows = deliveries(id)
		if len(rows) != 1 || !rows[0]["due_at"].Time().Equal(originalDue) {
			t.Fatal("concurrent clicks duplicated or delayed the batch")
		}
		old := httpClient
		defer func() { httpClient = old }()
		calls := 0
		httpClient = &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
			calls++
			if r.Method != "POST" || !strings.HasSuffix(r.URL.Path, "/feed") {
				t.Fatal("unexpected immediate publishing request", r.Method, r.URL.Path)
			}
			return response(200, `{"id":"run-now-published"}`), nil
		})}
		worked, e := ProcessDelivery(ctx)
		must(t, e)
		if !worked || calls != 1 {
			t.Fatal("worker did not pick up immediate task", worked, calls)
		}
		rows = deliveries(id)
		if rows[0]["status"].String() != "published" {
			t.Fatal("not published by worker", rows)
		}
		if RunNow(ctx, p, id) == nil || len(deliveries(id)) != 1 {
			t.Fatal("already published batch was replayed")
		}
	})
	t.Run("bring forward and resume without discarding throttling or replaying uncertain outcomes", func(t *testing.T) {
		id, _ := makePlan(t, "once", true)
		must(t, Schedule(ctx, p, id))
		before := deliveries(id)[0]
		must(t, RunNow(ctx, p, id))
		after := deliveries(id)[0]
		if before["id"].Int64() != after["id"].Int64() || !before["cycle_at"].Time().Equal(after["cycle_at"].Time()) || after["due_at"].Time().After(time.Now()) {
			t.Fatal("future row was duplicated instead of advanced")
		}
		if object(before["snapshot"])["body"] != object(after["snapshot"])["body"] {
			t.Fatal("reviewed content changed")
		}
		throttle := time.Now().Add(15 * time.Minute).Truncate(time.Microsecond)
		_, e = g.DB().Model("bm_social_deliveries").Where("id", after["id"]).Data(g.Map{"status": "retry", "next_at": throttle}).Update()
		must(t, e)
		must(t, PlanControl(ctx, p, id, "pause"))
		must(t, RunNow(ctx, p, id))
		if !deliveries(id)[0]["next_at"].Time().Equal(throttle) {
			t.Fatal("manual run bypassed provider retry delay")
		}
		for _, status := range []string{"unknown", "failed", "published"} {
			_, e = g.DB().Model("bm_social_deliveries").Where("id", after["id"]).Data("status", status).Update()
			must(t, e)
			if RunNow(ctx, p, id) == nil {
				t.Fatal("unsafe delivery was retried", status)
			}
		}
		_, e = g.DB().Model("bm_social_deliveries").Where("id", after["id"]).Data("status", "submitted").Update()
		must(t, e)
		must(t, RunNow(ctx, p, id))
		if deliveries(id)[0]["status"].String() != "submitted" {
			t.Fatal("submitted result was replayed")
		}
	})
	t.Run("recurrence uses real immediate date and returns to content review", func(t *testing.T) {
		id, _ := makePlan(t, "daily", true)
		must(t, RunNow(ctx, p, id))
		row := deliveries(id)[0]
		_, e = g.DB().Model("bm_social_deliveries").Where("id", row["id"]).Data("status", "published").Update()
		must(t, e)
		must(t, Tick(ctx))
		plan, e := g.DB().Model("bm_social_plans").Where("id", id).One()
		must(t, e)
		c, e := planConfig(plan)
		must(t, e)
		location, e := time.LoadLocation("Asia/Shanghai")
		must(t, e)
		if c.StartDate != time.Now().In(location).AddDate(0, 0, 1).Format("2006-01-02") || plan["status"].String() != "generating" {
			t.Fatal("next recurrence used old future start date or skipped review", c.StartDate, plan["status"])
		}
		if len(deliveries(id)) != 1 {
			t.Fatal("next unreviewed cycle was auto-published")
		}
	})
	t.Run("an omitted start date receives a stable execution date", func(t *testing.T) {
		id, _ := makePlan(t, "once", true)
		plan, e := g.DB().Model("bm_social_plans").Where("id", id).One()
		must(t, e)
		c, e := planConfig(plan)
		must(t, e)
		c.StartDate = ""
		_, e = SavePlan(ctx, p, id, "Undated run now fixture", company, c)
		must(t, e)
		must(t, RunNow(ctx, p, id))
		must(t, RunNow(ctx, p, id))
		plan, e = g.DB().Model("bm_social_plans").Where("id", id).One()
		must(t, e)
		c, e = planConfig(plan)
		must(t, e)
		if c.StartDate == "" || len(deliveries(id)) != 1 {
			t.Fatal("immediate cycle has no stable date or was duplicated")
		}
	})
}
