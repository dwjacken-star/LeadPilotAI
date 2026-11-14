package social

import (
	"context"
	"sync"
	"testing"
	"time"

	"billionmail-core/internal/service/governance"
	"billionmail-core/internal/service/growth"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/gconv"
)

func exerciseSocialManagement(t *testing.T, ctx context.Context, p *governance.Principal, company int64) {
	// Prevent earlier fixture lanes from becoming due while inspecting this batch.
	_, e := g.DB().Model("bm_social_plans").Where("id>0").Data("status", "paused").Update()
	must(t, e)
	accounts := []int64{}
	for _, platform := range []string{"facebook", "facebook", "linkedin"} {
		id, e := g.DB().Model("bm_social_accounts").Data(g.Map{"platform": platform, "external_id": "management-" + platform + "-" + gconv.String(len(accounts)), "name": "Management " + platform, "dept_id": p.DeptID, "owner_id": p.ID, "token": "fixture", "capabilities": `{"publish":true}`}).InsertAndGetId()
		must(t, e)
		accounts = append(accounts, id)
	}
	c := PlanConfig{Platforms: []string{"facebook", "linkedin"}, Accounts: accounts, Markets: []Market{{Country: "US", Language: "en", Timezone: "America/New_York", LocalTime: "09:12:34"}, {Country: "JP", Language: "ja", Timezone: "Asia/Tokyo", LocalTime: "15:43:21"}}, StartDate: time.Now().AddDate(0, 0, 20).Format("2006-01-02"), Cadence: "once", DailyPosts: 5, MinIntervalMinutes: 5}
	var wg sync.WaitGroup
	ids := make(chan int64, 5)
	errs := make(chan error, 5)
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id, e := SavePlan(ctx, p, 0, "Concurrent draft", company, c, "management-unique-draft-key")
			ids <- id
			errs <- e
		}()
	}
	wg.Wait()
	close(ids)
	close(errs)
	for e := range errs {
		must(t, e)
	}
	var id int64
	for value := range ids {
		if id != 0 && id != value {
			t.Fatal("duplicate draft was created")
		}
		id = value
	}
	for i := 0; i < 3; i++ {
		same, e := SavePlan(ctx, p, id, "Concurrent draft", company, c)
		must(t, e)
		if same != id {
			t.Fatal("draft identity changed")
		}
	}
	// Real scheduling creates every market/account combination, using separate local clocks.
	must(t, ManualVariants(ctx, p, id))
	variants, e := g.DB().Model("bm_social_variants").Where("plan_id", id).All()
	must(t, e)
	for _, v := range variants {
		must(t, SaveVariant(ctx, p, v["id"].Int64(), g.Map{"body": "Approved management fixture"}))
		must(t, ApproveVariant(ctx, p, v["id"].Int64()))
	}
	// Tokens are deliberately fake, but scheduling validates encrypted account credentials.
	for _, aid := range accounts {
		token, e := growth.SocialCredential("management-fixture-token", false)
		must(t, e)
		_, e = g.DB().Model("bm_social_accounts").Where("id", aid).Data("token", token).Update()
		must(t, e)
	}
	must(t, Schedule(ctx, p, id))
	deliveries, e := g.DB().Model("bm_social_deliveries").Where("plan_id", id).All()
	must(t, e)
	if len(deliveries) != 6 {
		t.Fatalf("want 6 market/account deliveries, got %d", len(deliveries))
	}
	for _, d := range deliveries {
		v, e := g.DB().Model("bm_social_variants").Where("id", d["variant_id"]).One()
		must(t, e)
		market := c.Markets[0]
		if v["country"].String() == "JP" {
			market = c.Markets[1]
		}
		due, e := marketTimeAt(market, c.StartDate, time.Now())
		must(t, e)
		if !due.Equal(d["due_at"].Time()) {
			t.Fatal("market-specific publishing clock was changed")
		}
	}
	list, e := List(ctx, p, "deliveries", id, 1, "", "", QueryFilter{Platform: "facebook", AccountID: accounts[0]})
	must(t, e)
	if gconv.Int(list["total"]) != 2 {
		t.Fatal("account/platform filter mixed deliveries", list)
	}
	list, e = List(ctx, p, "plans", 0, 1, "Concurrent draft", "", QueryFilter{AccountID: accounts[2]})
	must(t, e)
	if gconv.Int(list["total"]) != 1 {
		t.Fatal("multi-account task filter lost plan")
	}
	if _, e = Reports(ctx, p, 0, QueryFilter{Platform: "invalid"}); e == nil {
		t.Fatal("invalid platform accepted")
	}
	syncResult, e := SyncReports(ctx, p, QueryFilter{Platform: "facebook", AccountID: accounts[0]})
	must(t, e)
	if gconv.Int(syncResult["queued"]) != 1 {
		t.Fatal("manual sync ignored selected account")
	}
	must(t, DeleteRecord(ctx, p, "deliveries", deliveries[0]["id"].Int64()))
	must(t, RunNow(ctx, p, id))
	removed, e := g.DB().GetOne(ctx, `SELECT status,deleted_at FROM bm_social_deliveries WHERE id=$1`, deliveries[0]["id"])
	must(t, e)
	if removed["deleted_at"].IsNil() || removed["status"].String() != "cancelled" {
		t.Fatal("run now revived a deleted delivery")
	}
	cfg, e := config(ctx)
	must(t, e)
	if cfg.SyncMinutes != 240 {
		t.Fatal("sync interval is not four hours")
	}
	_, e = g.DB().Model("bm_social_deliveries").Where("id", deliveries[1]["id"]).Data("status", "publishing").Update()
	must(t, e)
	if DeleteRecord(ctx, p, "plans", id) == nil {
		t.Fatal("deleted publishing task")
	}
	_, e = g.DB().Model("bm_social_deliveries").Where("id", deliveries[1]["id"]).Data("status", "scheduled").Update()
	must(t, e)
	must(t, DeleteRecord(ctx, p, "plans", id))
	list, e = List(ctx, p, "plans", 0, 1, "Concurrent draft", "")
	must(t, e)
	if gconv.Int(list["total"]) != 0 {
		t.Fatal("deleted task remains visible")
	}
	total, e := g.DB().GetValue(ctx, `SELECT count(*) FROM bm_social_deliveries WHERE plan_id=$1 AND status='cancelled'`, id)
	n := total.Int()
	must(t, e)
	if n != 6 {
		t.Fatal("deleting task left pending work")
	}
	if PlanControl(ctx, p, id, "run_now") == nil {
		t.Fatal("deleted task was restarted")
	}
}
