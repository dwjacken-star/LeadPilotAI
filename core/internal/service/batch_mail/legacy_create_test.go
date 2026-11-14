package batch_mail

import (
	v1 "billionmail-core/api/batch_mail/v1"
	"billionmail-core/internal/service/public"
	"context"
	"fmt"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"testing"
	"time"
)

func exerciseLegacyCreateTask(t *testing.T, ctx context.Context) {
	t.Helper()
	ctx = context.WithValue(ctx, "accountId", int64(1))
	_ = public.Cache.Set(ctx, "ServerIP", "127.0.0.1", time.Hour)
	groupID, err := g.DB().Model("bm_contact_groups").Data(g.Map{"name": "legacy-create", "token": "legacy-create"}).InsertAndGetId()
	if err != nil {
		t.Fatal(err)
	}
	for _, email := range []string{"legacy-a@example.com", "legacy-b@example.com"} {
		if _, err := g.DB().Model("bm_contacts").Data(g.Map{"group_id": groupID, "email": email, "active": 1, "status": 1}).Insert(); err != nil {
			t.Fatal(err)
		}
	}
	for _, enabled := range []int{0, 1} {
		req := &v1.CreateTaskReq{Subject: fmt.Sprintf("legacy-create-%d", enabled), Senders: []string{"sender@example.com"}, GroupIds: []int{int(groupID)}, TemplateId: 1, StartTime: int(time.Now().Add(48 * time.Hour).Unix()), Warmup: enabled, Remark: fmt.Sprintf("remark-%d", enabled), IsRecord: 1, TrackOpen: 1, TrackClick: 1}
		req.Unsubscribe = enabled
		id, e := CreateTaskWithRecipients(ctx, req, 0)
		if e != nil || id < 1 {
			t.Fatalf("actual create path failed with warmup %d: %d %v", enabled, id, e)
		}
		stored, e := GetTaskInfo(ctx, id)
		if e != nil || stored == nil || stored.Warmup != enabled || stored.Unsubscribe != enabled || stored.Remark != req.Remark || stored.IsRecord != req.IsRecord {
			t.Fatalf("flags did not round-trip: %+v %v", stored, e)
		}
		recipients, e := g.DB().Model("recipient_info").Where("task_id", id).Count()
		if e != nil || recipients != 2 {
			t.Fatal("recipient snapshot missing", recipients, e)
		}
		if e = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error { return SetTaskWarmup(ctx, int64(id), 1-enabled) }); e != nil {
			t.Fatal(e)
		}
		info, e := GetTaskInfo(ctx, id)
		if e != nil || info.Warmup != 1-enabled {
			t.Fatal("warmup edit did not persist", e)
		}
		if e = SetTaskWarmup(ctx, int64(id), 0); e != nil {
			t.Fatal(e)
		}
	}
	before, _ := g.DB().Model("email_tasks").Count()
	_, e := CreateTaskWithRecipients(ctx, &v1.CreateTaskReq{Subject: "invalid-warmup", GroupId: int(groupID), Addresser: "sender@example.com", TemplateId: 1, StartTime: int(time.Now().Add(time.Hour).Unix()), Warmup: 2}, 0)
	after, _ := g.DB().Model("email_tasks").Count()
	if e == nil || before != after {
		t.Fatal("failed flag save left a partial task", e, before, after)
	}
}
