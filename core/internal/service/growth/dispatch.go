package growth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"billionmail-core/internal/service/batch_mail"
	"billionmail-core/internal/service/governance"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

func MessageControl(ctx context.Context, p *governance.Principal, id int64, action string) error {
	r, e := scoped(ctx, p, "bm_growth_messages", id, "outreach:publish")
	if e != nil {
		return e
	}
	if action == "cancel" {
		if r["status"].String() == "sent" || r["status"].String() == "unknown" {
			return errors.New("sent/uncertain messages cannot be cancelled or resent")
		}
		_, e = g.DB().Model("bm_growth_messages").Where("id", id).Data(g.Map{"status": "cancelled"}).Update()
		return e
	}
	if action != "approve" || r["status"].String() != "draft" {
		return errors.New("only a draft can be approved")
	}
	if !p.Has("campaign:publish") {
		return errors.New("campaign publishing permission also required")
	}
	_, e = g.DB().Model("bm_growth_messages").Where("id", id).Data(g.Map{"status": "approved"}).Update()
	if e != nil {
		return e
	}
	return childJob(ctx, p, "dispatch", r["run_id"].String(), fmt.Sprintf("dispatch-approved:%d", id), r["run_id"].Int64(), nil)
}
func sendWindow(c PlanConfig, language, country string, now time.Time) (time.Time, error) {
	zone := c.Timezone
	for _, m := range c.Markets {
		if m.Country == country {
			zone = m.Timezone
			break
		}
	}
	loc, e := time.LoadLocation(zone)
	if e != nil {
		return time.Time{}, e
	}
	local := now.In(loc)
	start, e := time.Parse("15:04", c.SendStart)
	if e != nil {
		return time.Time{}, e
	}
	end, e := time.Parse("15:04", c.SendEnd)
	if e != nil {
		return time.Time{}, e
	}
	first := time.Date(local.Year(), local.Month(), local.Day(), start.Hour(), start.Minute(), 0, 0, loc)
	last := time.Date(local.Year(), local.Month(), local.Day(), end.Hour(), end.Minute(), 0, 0, loc)
	if local.Before(first) {
		return first, nil
	}
	if !local.Before(last) {
		return first.AddDate(0, 0, 1), nil
	}
	return now, nil
}
func dispatchRun(ctx context.Context, p *governance.Principal, runID int64) error {
	scope, c, e := runContext(ctx, p, runID)
	if e != nil {
		return e
	}
	if e = require(p, "outreach:publish"); e != nil {
		return e
	}
	for _, permission := range []string{"campaign:create", "campaign:publish", "contact:create", "contact:list", "template:create", "template:list"} {
		if e = require(p, permission); e != nil {
			return e
		}
	}
	messages, e := g.DB().Model("bm_growth_messages").Ctx(ctx).Where("run_id", runID).Where("status", "approved").Order("id").Limit(c.DailySend).All()
	if e != nil {
		return e
	}
	for _, message := range messages {
		lead, e := scoped(ctx, p, "bm_growth_leads", message["lead_id"].Int64(), "outreach:publish")
		if e != nil {
			return e
		}
		if lead["suppressed"].Bool() || !lead["replied_at"].IsNil() {
			continue
		}
		if lead["state"].String() != "verified" {
			if lead["state"].String() == "invalid" || lead["state"].String() == "unconfirmed" {
				_, e = g.DB().Model("bm_growth_messages").Ctx(ctx).Where("id", message["id"]).Where("status", "approved").Data(g.Map{"status": "cancelled", "error": "recipient verification requires review: " + lead["state"].String()}).Update()
				if e != nil {
					return e
				}
			}
			continue
		}
		when, e := sendWindow(c, lead["language"].String(), lead["country"].String(), time.Now())
		if e != nil {
			return e
		}
		if when.After(time.Now().Add(time.Second)) {
			job, e := Queue(ctx, p, "dispatch", fmt.Sprint(runID), fmt.Sprintf("dispatch-window:%d:%d", runID, when.Unix()), g.Map{"run_id": runID})
			if e != nil {
				return e
			}
			_, e = g.DB().Model("bm_growth_jobs").Where("id", job).Where("status", "queued").Data(g.Map{"next_at": when}).Update()
			if e != nil {
				return e
			}
			continue
		}
		if !lead["verified_until"].Time().After(time.Now()) {
			_, _ = g.DB().Model("bm_growth_leads").Where("id", lead["id"]).Data(g.Map{"state": "collected"}).Update()
			if e = childJob(ctx, p, "verify", lead["id"].String(), fmt.Sprintf("verify-expired:%d:%d", lead["id"].Int64(), time.Now().Unix()/300), runID, nil); e != nil {
				return e
			}
			continue
		}
		e = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
			if _, e := tx.Exec("SELECT pg_advisory_xact_lock(614810)"); e != nil {
				return e
			}
			current, e := tx.Model("bm_growth_messages").Where("id", message["id"]).One()
			if e != nil {
				return e
			}
			if current["status"].String() != "approved" {
				return nil
			}
			loc, _ := time.LoadLocation(c.Timezone)
			today := time.Now().In(loc)
			midnight := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, loc)
			n, e := tx.Model("bm_growth_messages").Where("plan_id", scope["plan_id"]).WhereIn("status", []string{"queued", "sent", "unknown"}).Where("COALESCE(queued_at,created_at)>=?", midnight).Count()
			if e != nil {
				return e
			}
			if n >= c.DailySend {
				next := midnight.AddDate(0, 0, 1)
				key := fmt.Sprintf("dispatch-quota:%d:%s", runID, next.Format("2006-01-02"))
				_, e = tx.Model("bm_growth_jobs").Data(g.Map{"dept_id": p.DeptID, "owner_id": p.ID, "kind": "dispatch", "resource_id": fmt.Sprint(runID), "dedupe_key": key, "payload": jsonText(g.Map{"run_id": runID}), "next_at": next}).InsertIgnore()
				return e
			}
			// Cooldown spans all plans within this department; an unknown prior send
			// reserves the contact too, avoiding duplicate mail after uncertain delivery.
			n, e = tx.Model("bm_growth_messages m").LeftJoin("bm_growth_leads l", "l.id=m.lead_id").Where("m.dept_id", p.DeptID).Where("l.email", lead["email"]).WhereIn("m.status", []string{"queued", "sent", "unknown"}).Where("COALESCE(m.queued_at,m.created_at)>?", time.Now().AddDate(0, 0, -c.CooldownDays)).Count()
			if e != nil {
				return e
			}
			if n > 0 {
				_, e = tx.Model("bm_growth_messages").Where("id", message["id"]).Data(g.Map{"status": "cancelled", "error": "contact cooldown: another plan already reserved this recipient"}).Update()
				return e
			}
			cursor, e := tx.Model("bm_growth_plans").Where("id", scope["plan_id"]).Value("sender_cursor")
			if e != nil {
				return e
			}
			sender := c.Senders[int(cursor.Int64()%int64(len(c.Senders)))]
			if e = governance.SenderAllowed(ctx, p, sender); e != nil {
				return e
			}
			groupName := fmt.Sprintf("AI Outreach %d", message["plan_id"].Int64())
			group, e := tx.Model("bm_contact_groups").Where("dept_id", p.DeptID).Where("name", groupName).Value("id")
			if e != nil {
				return e
			}
			groupID := group.Int64()
			if groupID == 0 {
				groupID, e = tx.Model("bm_contact_groups").Data(g.Map{"name": groupName, "dept_id": p.DeptID, "owner_id": p.ID, "token": fmt.Sprintf("growth-%d-%d", p.DeptID, message["plan_id"].Int64())}).InsertAndGetId()
				if e != nil {
					return e
				}
			}
			if _, e = tx.Model("bm_contacts").Data(g.Map{"group_id": groupID, "email": lead["email"], "dept_id": p.DeptID, "owner_id": p.ID, "active": 1, "status": 1, "attribs": jsonText(g.Map{"growth_lead_id": lead["id"].Int64()})}).InsertIgnore(); e != nil {
				return e
			}
			templateName := groupName + " personal"
			template, e := tx.Model("email_templates").Where("dept_id", p.DeptID).Where("temp_name", templateName).Value("id")
			if e != nil {
				return e
			}
			templateID := template.Int64()
			if templateID == 0 {
				templateID, e = tx.Model("email_templates").Data(g.Map{"temp_name": templateName, "content": "<p>Personalized outreach</p>", "dept_id": p.DeptID, "owner_id": p.ID}).InsertAndGetId()
				if e != nil {
					return e
				}
			}
			// A single-contact campaign uses the existing SMTP policy, tracking and
			// auditing engine; the frozen individual subject/body is resolved at handoff.
			contact, e := tx.Model("bm_contacts").Where("group_id", groupID).Where("email", lead["email"]).Value("id")
			if e != nil {
				return e
			}
			policy := c.Policy
			policy.Timezone = c.Timezone
			for _, market := range c.Markets {
				if market.Country == lead["country"].String() {
					policy.Timezone = market.Timezone
					break
				}
			}
			policy.WindowStart, policy.WindowEnd = c.SendStart, c.SendEnd
			taskID, e := batch_mail.CreateGovernedTask(ctx, tx, p, batch_mail.CampaignConfig{Name: fmt.Sprintf("AI Outreach #%d / %s", runID, lead["email"].String()), GroupID: groupID, TemplateID: templateID, Sender: sender, Senders: []string{sender}, Subject: message["subject"].String(), Policy: policy}, true, contact.Int64())
			if e != nil {
				return e
			}
			if e = batch_mail.SetTaskDeliveryOptions(ctx, taskID, c.deliveryOptions()); e != nil {
				return e
			}
			if _, e = tx.Model("recipient_info").Where("task_id", taskID).Data(g.Map{"outreach_message_id": message["id"].Int64()}).Update(); e != nil {
				return e
			}
			_, e = tx.Model("bm_growth_messages").Where("id", message["id"]).Data(g.Map{"status": "queued", "task_id": taskID, "queued_at": time.Now()}).Update()
			if e != nil {
				return e
			}
			if _, e = tx.Model("bm_growth_leads").Where("id", lead["id"]).Data(g.Map{"assigned_sender": sender}).Update(); e != nil {
				return e
			}
			_, e = tx.Model("bm_growth_plans").Where("id", scope["plan_id"]).Data(g.Map{"sender_cursor": gdb.Raw("sender_cursor+1")}).Update()
			return e
		})
		if e != nil {
			return e
		}
	}
	remaining, e := g.DB().Model("bm_growth_messages").Ctx(ctx).Where("run_id", runID).Where("status", "approved").Count()
	if e != nil {
		return e
	}
	if remaining > 0 {
		loc, _ := time.LoadLocation(c.Timezone)
		now := time.Now().In(loc)
		next := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, 1)
		key := fmt.Sprintf("dispatch-quota:%d:%s", runID, next.Format("2006-01-02"))
		job, e := Queue(ctx, p, "dispatch", fmt.Sprint(runID), key, g.Map{"run_id": runID})
		if e != nil {
			return e
		}
		_, e = g.DB().Model("bm_growth_jobs").Ctx(ctx).Where("id", job).Where("status", "queued").Data(g.Map{"next_at": next}).Update()
		return e
	}
	return nil
}

func snapshotPlan(raw string) string {
	var value interface{}
	if json.Unmarshal([]byte(raw), &value) != nil {
		return raw
	}
	return jsonText(value)
}
