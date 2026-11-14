package batch_mail

import (
	"billionmail-core/internal/service/governance"
	"context"
	"encoding/json"
	"github.com/gogf/gf/v2/frame/g"
	"testing"
)

func exercisePlansAndAPI(t *testing.T, ctx context.Context, p *governance.Principal) {
	t.Helper()
	config := AutomationConfig{Campaign: CampaignConfig{GroupID: 1, Sender: "sender@example.com"}, Steps: []AutomationStep{{TemplateID: 1, Subject: "welcome", Condition: "always"}, {DelaySeconds: 2, TemplateID: 1, Subject: "followup", Condition: "clicked"}}}
	b, _ := json.Marshal(config)
	id, e := g.DB().Model("bm_automation_rules").Data(g.Map{"dept_id": p.DeptID, "owner_id": p.ID, "name": "flow-test", "group_id": 1, "config": string(b)}).InsertAndGetId()
	if e != nil {
		t.Fatal(e)
	}
	_, e = g.DB().Exec(ctx, `INSERT INTO bm_contacts(email,group_id,active,status) VALUES('new@example.com',1,1,1),('unconfirmed@example.com',1,1,0)`)
	if e != nil {
		t.Fatal(e)
	}
	tickAutomations(ctx)
	tickAutomations(ctx)
	rows, e := g.DB().Model("bm_automation_enrollments").Where("rule_id", id).All()
	if e != nil || len(rows) != 1 {
		t.Fatalf("confirmation enrollment: %v %v", rows, e)
	}
	enrollment := rows[0]
	first := enrollment["last_task_id"].Int64()
	if first == 0 || enrollment["step"].Int() != 1 {
		t.Fatal(enrollment)
	}
	for _, query := range []string{`UPDATE email_tasks SET task_process=2 WHERE id=$1`, `UPDATE recipient_info SET delivery_state='accepted',is_sent=1,sent_time=extract(epoch from now())::int-10 WHERE task_id=$1`, `INSERT INTO mailstat_clicked(campaign_id,recipient) VALUES($1,'new@example.com')`} {
		if _, e = g.DB().Exec(ctx, query, first); e != nil {
			t.Fatal(e)
		}
	}
	if _, e = g.DB().Exec(ctx, `UPDATE bm_automation_enrollments SET next_at=now()-interval '1 minute' WHERE id=$1`, enrollment["id"]); e != nil {
		t.Fatal(e)
	}
	tickAutomations(ctx)
	enrollment, e = g.DB().Model("bm_automation_enrollments").Where("rule_id", id).One()
	if e != nil {
		t.Fatal(e)
	}
	if enrollment["step"].Int() != 2 || enrollment["state"].String() != "finishing" || enrollment["last_task_id"].Int64() == first {
		t.Fatalf("conditional followup did not advance: %v", enrollment)
	}
	_, e = g.DB().Exec(ctx, `UPDATE email_tasks SET task_process=2 WHERE id=$1`, enrollment["last_task_id"])
	if e != nil {
		t.Fatal(e)
	}
	tickAutomations(ctx)
	value, e := g.DB().Model("bm_automation_enrollments").Where("rule_id", id).Value("state")
	if e != nil || value.String() != "completed" {
		t.Fatalf("flow did not complete: %v %v", value, e)
	}
	_, e = g.DB().Exec(ctx, `INSERT INTO api_templates(api_key,api_name,template_id,subject,addresser,group_id) VALUES('api-flow-test','test',1,'API message','sender@example.com',1);INSERT INTO api_mail_logs(api_id,recipient,message_id,addresser) VALUES(1,'a@example.com','api-source','sender@example.com')`)
	if e != nil {
		t.Fatal(e)
	}
	bridgeAPI(ctx)
	row, e := g.DB().Model("api_mail_logs").Where("id", 1).One()
	if e != nil || row["status"].Int() != 1 {
		t.Fatalf("API bridge failed: %v %v", row, e)
	}
	count, e := g.DB().Model("recipient_info").Where("api_log_id", 1).Count()
	if e != nil || count != 1 {
		t.Fatalf("API created duplicate/missing delivery: %d %v", count, e)
	}
	bridgeAPI(ctx)
	count, e = g.DB().Model("recipient_info").Where("api_log_id", 1).Count()
	if e != nil || count != 1 {
		t.Fatal("API bridge not idempotent")
	}
}
