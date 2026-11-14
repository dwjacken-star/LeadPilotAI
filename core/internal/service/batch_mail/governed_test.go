package batch_mail

import (
	"context"
	"sync"
	"testing"
	"time"

	"billionmail-core/internal/service/governance"
	"billionmail-core/internal/testsupport"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

func TestCalendarRecurrence(t *testing.T) {
	s := ScheduleSpec{Cadence: "monthly", Timezone: "Asia/Shanghai", LocalTime: "09:00", MonthDay: 31}
	loc, _ := time.LoadLocation(s.Timezone)
	got, e := s.Next(time.Date(2027, 2, 1, 0, 0, 0, 0, loc))
	want := time.Date(2027, 2, 28, 9, 0, 0, 0, loc)
	if e != nil || !got.Equal(want) {
		t.Fatalf("got %v %v want %v", got, e, want)
	}
	s = ScheduleSpec{Cadence: "daily", Timezone: "America/New_York", LocalTime: "09:00"}
	loc, _ = time.LoadLocation(s.Timezone)
	got, e = s.Next(time.Date(2027, 3, 13, 10, 0, 0, 0, loc))
	if e != nil || got.Hour() != 9 || got.Day() != 14 {
		t.Fatalf("DST drift: %v %v", got, e)
	}
}
func TestPersistentCampaignsAndClaims(t *testing.T) {
	testsupport.Database(t)
	ctx := context.Background()
	p, e := governance.Load(ctx, 1)
	if e != nil {
		t.Fatal(e)
	}
	_, e = g.DB().Exec(ctx, `INSERT INTO bm_contact_groups(name,token) VALUES('campaign-test','token');INSERT INTO email_templates(temp_name,content) VALUES('template-test','hello {{ .Subscriber.Email }}');INSERT INTO bm_contacts(email,group_id,active,status) VALUES('a@example.com',1,1,1),('b@example.com',1,1,1)`)
	if e != nil {
		t.Fatal(e)
	}
	config := CampaignConfig{Name: "integration", GroupID: 1, TemplateID: 1, Sender: "sender@example.com", Subject: "test"}
	var task int64
	e = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		var e error
		task, e = CreateGovernedTask(ctx, tx, p, config, true, 0)
		return e
	})
	if e != nil {
		t.Fatal(e)
	}
	jobs := make(chan *governedJob, 6)
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			j, e := claimGoverned(ctx)
			if e != nil {
				t.Error(e)
			}
			if j != nil {
				jobs <- j
			}
		}()
	}
	wg.Wait()
	close(jobs)
	seen := map[int64]bool{}
	for j := range jobs {
		id := j.Record["delivery_id"].Int64()
		if seen[id] {
			t.Fatal("delivery claimed twice")
		}
		seen[id] = true
	}
	if len(seen) != 2 {
		t.Fatalf("claimed %d deliveries", len(seen))
	}
	_, e = g.DB().Exec(ctx, `UPDATE recipient_info SET delivery_state='sending',lease_until=now()-interval '1 second' WHERE task_id=$1`, task)
	if e != nil {
		t.Fatal(e)
	}
	DispatchGoverned(ctx)
	n, e := g.DB().Model("recipient_info").Where("task_id", task).Where("delivery_state", "unknown").Count()
	if e != nil || n != 2 {
		t.Fatalf("ambiguous sends must not retry: %d %v", n, e)
	}
	var schedule int64
	e = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		var e error
		schedule, e = tx.Model("bm_campaign_schedules").Data(g.Map{"name": "once", "dept_id": 1, "owner_id": 1, "config": config, "cadence": "once", "next_run_at": time.Now().Add(-time.Hour)}).InsertAndGetId()
		return e
	})
	if e != nil {
		t.Fatal(e)
	}
	TickGovernedPlans(ctx)
	TickGovernedPlans(ctx)
	n, e = g.DB().Model("bm_campaign_runs").Where("schedule_id", schedule).Count()
	if e != nil || n != 1 {
		t.Fatalf("one-shot plan generated %d runs: %v", n, e)
	}
	exerciseSMTP(t, ctx, p)
	exercisePlansAndAPI(t, ctx, p)
	exerciseSenderPools(t, ctx, p)
	exerciseLegacyCreateTask(t, ctx)
	exerciseCampaignContent(t, ctx, p)
	exerciseLiveStatsAndValidation(t, ctx)
}
