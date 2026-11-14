package batch_mail

import (
	"context"
	"github.com/gogf/gf/v2/frame/g"
	"testing"
)

func exerciseLiveStatsAndValidation(t *testing.T, ctx context.Context) {
	t.Helper()
	id, err := g.DB().Model("email_tasks").Data(g.Map{"task_name": "live stats", "addresser": "sender@example.com", "template_id": 1, "recipient_count": 100, "dept_id": 1, "owner_id": 1, "pause": 1}).InsertAndGetId()
	if err != nil {
		t.Fatal(err)
	}
	_, err = g.DB().Exec(ctx, `INSERT INTO recipient_info(task_id,recipient,message_id,delivery_state,is_sent) SELECT $1,'buyer-'||i||'@example.com','live-'||i,CASE WHEN i=99 THEN 'retry' WHEN i=100 THEN 'validation_hold' ELSE 'accepted' END,CASE WHEN i<99 THEN 1 ELSE 0 END FROM generate_series(1,100)i`, id)
	if err != nil {
		t.Fatal(err)
	}
	read := func() LiveTaskStats {
		t.Helper()
		stats, e := GetLiveTaskStats(ctx, []int{int(id)})
		if e != nil {
			t.Fatal(e)
		}
		return stats[int(id)]
	}
	_, err = g.DB().Exec(ctx, `UPDATE recipient_info SET message_id='<live-2>' WHERE task_id=$1 AND recipient='buyer-2@example.com'`, id)
	if err != nil {
		t.Fatal(err)
	}
	s := read()
	if s.Progress != 98 || s.Submitted != 98 || s.Failed != 0 || s.Held != 1 || s.Waiting != 2 {
		t.Fatal("98 percent must stay 98, retry is not failure", s)
	}
	_, err = g.DB().Exec(ctx, `INSERT INTO mailstat_message_ids(postfix_message_id,message_id) VALUES('live-cc','live-1'),('live-old','live-2'),('live-new','live-2');INSERT INTO mailstat_send_mails(postfix_message_id,recipient,status,log_time_millis) VALUES('live-cc','copy@example.com','bounced',1000),('live-old','buyer-2@example.com','deferred',1000),('live-new','buyer-2@example.com','sent',2000)`)
	if err != nil {
		t.Fatal(err)
	}
	s = read()
	if s.Failed != 0 || s.Deferred != 0 || s.Delivered != 1 {
		t.Fatal("CC or older retry inflated statistics", s)
	}
	_, err = g.DB().Exec(ctx, `UPDATE mailstat_send_mails SET status='bounced' WHERE postfix_message_id='live-new'`)
	if err != nil {
		t.Fatal(err)
	}
	s = read()
	if s.Failed != 1 || s.Submitted != 97 || s.Progress != 98 {
		t.Fatal("bounce did not replace submission", s)
	}
	// Historical holds stay fenced until explicit operator confirmation.
	if e := RecheckTaskRecipients(ctx, id); e != nil {
		t.Fatal(e)
	}
	value, _ := g.DB().Model("recipient_info").Where("task_id", id).Where("recipient", "buyer-100@example.com").Value("delivery_state")
	if value.String() != "retry" {
		t.Fatal("historical hold was not released", value)
	}
	pause, _ := g.DB().Model("email_tasks").Where("id", id).Value("pause")
	if pause.Int() != 1 {
		t.Fatal("releasing a historical hold resumed a paused task")
	}
}
