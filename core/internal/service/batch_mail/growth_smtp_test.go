package batch_mail

import (
	"billionmail-core/internal/service/governance"
	"context"
	"encoding/json"
	"fmt"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"io"
	"mime/quotedprintable"
	"net/mail"
	"strings"
	"testing"
	"time"
)

// Uses the existing local fake SMTP server; it never contacts a real recipient.
func exercisePersonalizedOutreach(t *testing.T, ctx context.Context, p *governance.Principal, received <-chan string) {
	t.Helper()
	t.Setenv("MAPLEMAIL_FROM_ADDRESS", "sales@best-maple.com")
	t.Setenv("MAPLEMAIL_REPLY_TO_ADDRESS", "sales@best-maple.com")
	exec := func(query string, args ...interface{}) {
		t.Helper()
		if _, e := g.DB().Exec(ctx, query, args...); e != nil {
			t.Fatal(e)
		}
	}
	exec(`UPDATE email_tasks SET pause=1;INSERT INTO bm_contact_groups(id,name,token,dept_id,owner_id) VALUES(901,'Growth SMTP contacts','growth-smtp',1,1);INSERT INTO bm_growth_companies(id,dept_id,owner_id,name) VALUES(901,1,1,'SMTP fixture company');INSERT INTO bm_growth_products(id,company_id,dept_id,owner_id,sku,name,published,version) VALUES(901,901,1,1,'G-1','Compressor',true,1);INSERT INTO bm_growth_plans(id,company_id,dept_id,owner_id,name,config,active) VALUES(901,901,1,1,'SMTP test','{"daily_send":10,"policy":{"per_minute":10}}',true);INSERT INTO bm_growth_runs(id,plan_id,dept_id,owner_id,due_at,config) VALUES(901,901,1,1,now(),'{}');INSERT INTO mailbox(username,password,password_encode,full_name,maildir,local_part,domain,active) VALUES('growthsender@example.com','test','6447567a64413d3d','Test','growth/','growthsender','example.com',1)`)
	plan, e := g.DB().Model("bm_growth_plans").Where("id", 901).Value("config")
	if e != nil {
		t.Fatal(e)
	}
	snapshot, _ := json.Marshal(g.Map{"plan_config": plan.String(), "materials": g.Map{"company_id": 901, "company_version": 1, "product_versions": g.Map{"901": 1}, "materials": []interface{}{}}})
	for index, sender := range []string{"sender@example.com", "growthsender@example.com"} {
		address := fmt.Sprintf("growth-%d@example.com", index)
		contact, e := g.DB().Model("bm_contacts").Data(g.Map{"email": address, "group_id": 901, "dept_id": 1, "owner_id": 1, "active": 1, "status": 1}).InsertAndGetId()
		if e != nil {
			t.Fatal(e)
		}
		lead, e := g.DB().Model("bm_growth_leads").Data(g.Map{"plan_id": 901, "company_id": 901, "dept_id": 1, "owner_id": 1, "email": address, "state": "verified", "verified_until": time.Now().Add(time.Hour), "assigned_sender": sender}).InsertAndGetId()
		if e != nil {
			t.Fatal(e)
		}
		expected := fmt.Sprintf("<p>Hello customer %d,</p><p>Compressor offer %d.</p><p><a href=\"https://example.com/catalog\">Catalog</a></p>", index, index)
		message, e := g.DB().Model("bm_growth_messages").Data(g.Map{"lead_id": lead, "run_id": 901, "plan_id": 901, "dept_id": 1, "owner_id": 1, "subject": fmt.Sprintf("Personal offer %d", index), "html": expected, "language": "en", "snapshot": string(snapshot), "status": "queued"}).InsertAndGetId()
		if e != nil {
			t.Fatal(e)
		}
		var task int64
		e = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
			var err error
			task, err = CreateGovernedTask(ctx, tx, p, CampaignConfig{GroupID: 901, TemplateID: 1, Sender: sender, Subject: "Placeholder campaign subject"}, true, contact)
			if err != nil {
				return err
			}
			if err = SetTaskDeliveryOptions(ctx, task, TaskDeliveryOptions{Unsubscribe: index == 0, TrackOpen: index == 0, TrackClick: index == 0}); err != nil {
				return err
			}
			_, err = tx.Model("recipient_info").Where("task_id", task).Data(g.Map{"outreach_message_id": message}).Update()
			return err
		})
		if e != nil {
			t.Fatal(e)
		}
		job, e := claimGoverned(ctx)
		if e != nil || job == nil {
			t.Fatal("outreach claim", e)
		}
		if job.Record["outreach_message_id"].Int64() != message {
			t.Fatal("outreach marker not claimed")
		}
		processGoverned(ctx, job)
		delivery, e := g.DB().Model("recipient_info").Where("task_id", task).One()
		// Respect the real shared spacing policy even with a very fast fake SMTP
		// server. A deferred pre-DATA attempt is safe to claim again after its due time.
		for attempt := 0; e == nil && delivery["delivery_state"].String() == "retry" && delivery["attempts"].Int() == 0 && attempt < 3; attempt++ {
			wait := time.Until(delivery["next_attempt_at"].Time()) + 10*time.Millisecond
			if wait > time.Second {
				t.Fatal("unexpected long fixture deferral", delivery)
			}
			if wait > 0 {
				time.Sleep(wait)
			}
			job, e = claimGoverned(ctx)
			if e != nil || job == nil {
				t.Fatal("deferred outreach not claimable", e)
			}
			processGoverned(ctx, job)
			delivery, e = g.DB().Model("recipient_info").Where("task_id", task).One()
		}
		if e != nil || delivery["delivery_state"].String() != "accepted" {
			t.Fatal("personalized SMTP failed", delivery, e)
		}
		if delivery["assigned_sender"].String() != sender {
			t.Fatal("rotating SMTP sender changed", sender, delivery)
		}
		select {
		case raw := <-received:
			parsed, e := mail.ReadMessage(strings.NewReader(raw))
			if e != nil {
				t.Fatal(e)
			}
			body, e := io.ReadAll(quotedprintable.NewReader(parsed.Body))
			if index == 0 {
				if parsed.Header.Get("List-Unsubscribe") == "" || !strings.Contains(string(body), "Unsubscribe") || !strings.Contains(string(body), "/pmta/") || strings.Contains(string(body), `href="https://example.com/catalog"`) {
					t.Fatal("enabled unsubscribe/open tracking missing", raw)
				}
			} else if parsed.Header.Get("List-Unsubscribe") != "" || strings.Contains(string(body), "Unsubscribe") || strings.Contains(string(body), "/pmta/") || !strings.Contains(string(body), `href="https://example.com/catalog"`) {
				t.Fatal("disabled options still instrumented the message", raw)
			}
			if e != nil || !strings.Contains(string(body), fmt.Sprintf("Hello customer %d,", index)) || !strings.Contains(string(body), fmt.Sprintf("Compressor offer %d.", index)) || !strings.Contains(parsed.Header.Get("From"), "sales@best-maple.com") || parsed.Header.Get("Reply-To") != "sales@best-maple.com" || parsed.Header.Get("Subject") != fmt.Sprintf("Personal offer %d", index) {
				t.Fatal("frozen content or sender lost", raw, e)
			}
		case <-time.After(time.Second):
			t.Fatal("no personalized SMTP payload")
		}
		state, e := g.DB().Model("bm_growth_messages").Where("id", message).Value("status")
		if e != nil || state.String() != "sent" {
			t.Fatal("outreach status not finalized", state, e)
		}
		exec(`UPDATE email_tasks SET pause=1 WHERE id=$1`, task)
	}
}
