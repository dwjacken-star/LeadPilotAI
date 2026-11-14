package social

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	"billionmail-core/internal/service/governance"
	"billionmail-core/internal/service/growth"
	"billionmail-core/internal/testsupport"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/gconv"
	"github.com/google/uuid"
)

func TestManualConfirmationURLs(t *testing.T) {
	for _, value := range []string{"", "https://facebook.com/posts/123", "https://www.facebook.com/permalink.php?story_fbid=123", "https://m.facebook.com/posts/123"} {
		if _, e := manualExternalURL(value); e != nil {
			t.Fatalf("valid manual post URL %q: %v", value, e)
		}
	}
	for _, value := range []string{"http://facebook.com/123", "https://facebook.com.evil.test/123", "https://evilfacebook.com/123", "https://user:password@facebook.com/123", "https://facebook.com:8443/123", "https://linkedin.com/posts/123", "//facebook.com/123", "https://facebook.com\\@evil.test/123"} {
		if _, e := manualExternalURL(value); e == nil {
			t.Fatalf("unsafe manual post URL accepted: %q", value)
		}
	}
}

func TestLinkedInMemberReadbackCapabilities(t *testing.T) {
	account := gdb.Record{"platform": g.NewVar("linkedin"), "target_type": g.NewVar("person"), "publish_mode": g.NewVar("automatic"), "scopes": g.NewVar(encode([]string{"openid", "w_member_social"})), "capabilities": g.NewVar(encode(g.Map{"publish": true, "images": true}))}
	if linkedInPostReadable(account) || accountSyncSupported(account) {
		t.Fatal("write-only member account attempted readback or synchronization")
	}
	account["scopes"] = g.NewVar(encode([]string{"w_member_social", "r_member_social"}))
	if !linkedInPostReadable(account) || !accountSyncSupported(account) {
		t.Fatal("actual member read scope was ignored")
	}
	account["publish_mode"] = g.NewVar("manual")
	if accountSyncSupported(account) {
		t.Fatal("manual account offered official synchronization")
	}
}

func TestManualDeliveryLifecycle(t *testing.T) {
	testsupport.Database(t)
	ctx := context.Background()
	p, e := governance.Load(ctx, 1)
	must(t, e)
	oldHTTP := httpClient
	t.Cleanup(func() { httpClient = oldHTTP })
	httpClient = &http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("manual delivery called an official platform endpoint")
		return nil, nil
	})}
	company, e := growth.SaveCompany(ctx, p, 0, g.Map{"name": "Manual fixture supplier", "profile": g.Map{"business": "Automotive parts"}})
	must(t, e)
	must(t, growth.PublishCompany(ctx, p, company))
	account, e := g.DB().Model("bm_social_accounts").Data(g.Map{"platform": "facebook", "target_type": "person", "publish_mode": "manual", "external_id": "manual:" + uuid.NewString(), "name": "Manual Facebook person", "dept_id": p.DeptID, "owner_id": p.ID, "token": "", "status": "connected", "capabilities": encode(g.Map{"manual_publish": true})}).InsertAndGetId()
	must(t, e)
	makePlan := func() (int64, int64, int64) {
		c := PlanConfig{Platforms: []string{"facebook"}, Accounts: []int64{account}, Markets: []Market{{Country: "US", Language: "en", Timezone: "UTC", LocalTime: "10:22:37"}}, StartDate: time.Now().AddDate(0, 0, 2).Format("2006-01-02"), Cadence: "once", DailyPosts: 3, MinIntervalMinutes: 5}
		id, e := SavePlan(ctx, p, 0, "Manual delivery fixture", company, c)
		must(t, e)
		must(t, ManualVariants(ctx, p, id))
		value, e := g.DB().Model("bm_social_variants").Where("plan_id", id).Value("id")
		must(t, e)
		v := value.Int64()
		must(t, SaveVariant(ctx, p, v, g.Map{"title": "Manual title", "body": "Reviewed manual body"}))
		must(t, ApproveVariant(ctx, p, v))
		must(t, RunNow(ctx, p, id))
		d, e := g.DB().Model("bm_social_deliveries").Where("plan_id", id).Value("id")
		must(t, e)
		worked, e := ProcessDelivery(ctx)
		must(t, e)
		if !worked {
			t.Fatal("due manual delivery was not processed")
		}
		return id, v, d.Int64()
	}
	t.Run("durable due reminder explicit concurrent confirmation and recurrence", func(t *testing.T) {
		id, v, d := makePlan()
		before, e := DeliveryDetail(ctx, p, d)
		must(t, e)
		if before["status"].String() != "manual_due" || before["publish_mode"].String() != "manual" || before["target_type"].String() != "person" {
			t.Fatal("manual delivery was not promoted honestly", before)
		}
		reminders, e := ManualReminders(ctx, p)
		must(t, e)
		if gconv.Int(reminders["total"]) != 1 {
			t.Fatal("durable reminder missing", reminders)
		}
		must(t, prepareRecurrences(ctx))
		planState, e := g.DB().Model("bm_social_plans").Where("id", id).Value("status")
		must(t, e)
		if planState.String() != "scheduled" {
			t.Fatal("pending manual reminder did not block recurrence")
		}
		if DeliveryControl(ctx, p, d, "manual_confirm", g.Map{}) == nil {
			t.Fatal("implicit confirmation accepted")
		}
		if DeliveryControl(ctx, &governance.Principal{ID: p.ID, DeptID: p.DeptID}, d, "manual_confirm", g.Map{"manual_confirmed": true}) == nil {
			t.Fatal("confirmation bypassed publish permission")
		}
		if DeliveryControl(ctx, p, d, "manual_confirm", g.Map{"manual_confirmed": true, "external_url": "https://facebook.com.evil.test/post"}) == nil {
			t.Fatal("foreign confirmation URL accepted")
		}
		var wg sync.WaitGroup
		errs := make(chan error, 2)
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				errs <- DeliveryControl(ctx, p, d, "manual_confirm", g.Map{"manual_confirmed": true, "external_url": "https://www.facebook.com/posts/123"})
			}()
		}
		wg.Wait()
		close(errs)
		for e := range errs {
			must(t, e)
		}
		confirmed, e := DeliveryDetail(ctx, p, d)
		must(t, e)
		if confirmed["status"].String() != "manual_confirmed" || confirmed["manual_confirmed_at"].IsNil() || confirmed["manual_confirmed_by"].Int64() != p.ID || confirmed["remote_id"].String() != "" || !confirmed["submitted_at"].IsNil() || !confirmed["published_at"].IsNil() || confirmed["snapshot"].String() != before["snapshot"].String() {
			t.Fatal("manual confirmation fabricated platform evidence or changed snapshot", confirmed)
		}
		must(t, SaveVariant(ctx, p, v, g.Map{"body": "Next reviewed manual body"}))
		status, e := g.DB().Model("bm_social_deliveries").Where("id", d).Value("status")
		must(t, e)
		if status.String() != "manual_confirmed" {
			t.Fatal("editing cancelled a completed manual confirmation")
		}
		must(t, prepareRecurrences(ctx))
		planState, e = g.DB().Model("bm_social_plans").Where("id", id).Value("status")
		must(t, e)
		if planState.String() != "completed" {
			t.Fatal("confirmed manual delivery stalled completion")
		}
	})
	t.Run("pause edits and platform operations remain fenced", func(t *testing.T) {
		id, v, d := makePlan()
		must(t, PlanControl(ctx, p, id, "pause"))
		reminders, e := ManualReminders(ctx, p)
		must(t, e)
		if gconv.Int(reminders["total"]) != 0 || DeliveryControl(ctx, p, d, "manual_confirm", g.Map{"manual_confirmed": true}) == nil {
			t.Fatal("paused manual delivery remained actionable")
		}
		must(t, PlanControl(ctx, p, id, "resume"))
		worked, e := ProcessDelivery(ctx)
		must(t, e)
		if !worked {
			t.Fatal("resumed manual delivery was not rechecked")
		}
		for _, operation := range []string{"verify", "retry"} {
			if DeliveryControl(ctx, p, d, operation, g.Map{"remote_id": "123", "confirmed_absent": true}) == nil {
				t.Fatal("manual platform operation accepted", operation)
			}
		}
		must(t, SaveVariant(ctx, p, v, g.Map{"body": "Changed manual body"}))
		status, e := g.DB().Model("bm_social_deliveries").Where("id", d).Value("status")
		must(t, e)
		if status.String() != "cancelled" || DeliveryControl(ctx, p, d, "manual_confirm", g.Map{"manual_confirmed": true}) == nil {
			t.Fatal("changed variant's reminder remained confirmable")
		}
	})
	t.Run("confirmation rechecks source review and account", func(t *testing.T) {
		id, v, d := makePlan()
		_, e := g.DB().Model("bm_social_variants").Where("id", v).Data("state", "review").Update()
		must(t, e)
		if DeliveryControl(ctx, p, d, "manual_confirm", g.Map{"manual_confirmed": true}) == nil {
			t.Fatal("confirmation ignored revoked review")
		}
		_, e = g.DB().Model("bm_social_variants").Where("id", v).Data("state", "approved").Update()
		must(t, e)
		delivery, e := DeliveryDetail(ctx, p, d)
		must(t, e)
		corrupt := object(delivery["snapshot"])
		corrupt["sources"] = g.Map{"changed": true}
		_, e = g.DB().Model("bm_social_deliveries").Where("id", d).Data("snapshot", encode(corrupt)).Update()
		must(t, e)
		if DeliveryControl(ctx, p, d, "manual_confirm", g.Map{"manual_confirmed": true}) == nil {
			t.Fatal("confirmation ignored changed source snapshot")
		}
		_, e = g.DB().Model("bm_social_deliveries").Where("id", d).Data("snapshot", delivery["snapshot"]).Update()
		must(t, e)
		_, e = g.DB().Model("bm_social_accounts").Where("id", account).Data("status", "disconnected").Update()
		must(t, e)
		if DeliveryControl(ctx, p, d, "manual_confirm", g.Map{"manual_confirmed": true}) == nil {
			t.Fatal("confirmation ignored disconnected account")
		}
		_, e = g.DB().Model("bm_social_accounts").Where("id", account).Data("status", "connected").Update()
		must(t, e)
		must(t, PlanControl(ctx, p, id, "delete"))
	})
	t.Run("LinkedIn member submission completes without unauthorized readback", func(t *testing.T) {
		cfg, e := config(ctx)
		must(t, e)
		cfg.LinkedIn.Enabled, cfg.LinkedIn.Publishing = true, true
		cfg.LinkedIn.ClientID, cfg.LinkedIn.ClientSecret = "fixture-client", "fixture-client-secret"
		cfg.LinkedIn.RedirectURL = "https://admin.example.com/social/callback"
		must(t, SaveSettings(ctx, p, cfg))
		token, e := growth.SocialCredential("member-token", false)
		must(t, e)
		member, e := g.DB().Model("bm_social_accounts").Data(g.Map{"platform": "linkedin", "target_type": "person", "publish_mode": "automatic", "external_id": "urn:li:person:fixture", "name": "LinkedIn member fixture", "dept_id": p.DeptID, "owner_id": p.ID, "token": token, "status": "connected", "scopes": encode([]string{"openid", "w_member_social"}), "capabilities": encode(g.Map{"publish": true, "images": true})}).InsertAndGetId()
		must(t, e)
		c := PlanConfig{Platforms: []string{"linkedin"}, Accounts: []int64{member}, Markets: []Market{{Country: "US", Language: "en", Timezone: "UTC", LocalTime: "10:22:37"}}, StartDate: time.Now().AddDate(0, 0, 2).Format("2006-01-02"), Cadence: "once", DailyPosts: 3, MinIntervalMinutes: 5}
		id, e := SavePlan(ctx, p, 0, "LinkedIn member plan", company, c)
		must(t, e)
		must(t, ManualVariants(ctx, p, id))
		v, e := g.DB().Model("bm_social_variants").Where("plan_id", id).Value("id")
		must(t, e)
		must(t, SaveVariant(ctx, p, v.Int64(), g.Map{"body": "Approved LinkedIn member post"}))
		must(t, ApproveVariant(ctx, p, v.Int64()))
		must(t, RunNow(ctx, p, id))
		calls := 0
		httpClient = &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
			if r.Method != http.MethodPost || r.URL.Host != "api.linkedin.com" || r.URL.Path != "/rest/posts" {
				t.Fatal("write-only LinkedIn member invoked a read or organization endpoint", r.URL)
			}
			calls++
			res := response(201, `{}`)
			res.Header.Set("x-restli-id", "urn:li:share:fixture-submitted")
			return res, nil
		})}
		worked, e := ProcessDelivery(ctx)
		must(t, e)
		d, e := g.DB().Model("bm_social_deliveries").Where("plan_id", id).One()
		must(t, e)
		if !worked || calls != 1 || d["status"].String() != "submitted" || d["remote_id"].String() == "" || d["submitted_at"].IsNil() || !d["published_at"].IsNil() {
			t.Fatal("member submission fabricated publication or failed", d)
		}
		if e = SyncAccount(ctx, p, member); e == nil {
			t.Fatal("write-only member offered synchronization")
		}
		if e = DeliveryControl(ctx, p, d["id"].Int64(), "verify", g.Map{}); e == nil {
			t.Fatal("write-only member attempted post verification")
		}
		must(t, prepareRecurrences(ctx))
		state, e := g.DB().Model("bm_social_plans").Where("id", id).Value("status")
		must(t, e)
		if state.String() != "completed" {
			t.Fatal("honest member submitted state stalled completion")
		}
	})
}
