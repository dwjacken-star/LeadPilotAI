package growth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"billionmail-core/internal/service/crm"
	"billionmail-core/internal/service/deliveryprefs"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/gconv"
)

func masterScheduleEvents(ctx context.Context) error {
	rows, e := g.DB().Model("bm_gm_workflows").Ctx(ctx).Where("active", true).Where("code", "WF05").Limit(100).All()
	if e != nil {
		return e
	}
	for _, w := range rows {
		p, e := workerPrincipal(ctx, w["owner_id"].Int64(), w["dept_id"].Int64(), "intelligence:run")
		if e != nil {
			continue
		}
		var c MasterConfig
		if json.Unmarshal(w["config"].Bytes(), &c) != nil {
			continue
		}
		for _, id := range c.InboxRuleIDs {
			rule, e := scoped(ctx, p, "bm_growth_inbox_rules", id, "inquiry:manage")
			if e != nil || rule["company_id"].Int64() != w["company_id"].Int64() || !rule["active"].Bool() {
				continue
			}
			_, e = Queue(ctx, p, "inbox", fmt.Sprint(id), fmt.Sprintf("master-inbox:%d:%d", id, time.Now().Unix()/60), g.Map{})
			if e != nil {
				return e
			}
		}
		// Mailbox inquiry classification already runs in the native inbound pipeline.
		// Source messages remain in CRM; this event only links the current company rule.
		scope, e := p.Predicate(ctx, "inquiry:list", "i")
		if e != nil {
			continue
		}
		q := g.DB().Model("bm_growth_inquiries i").Ctx(ctx).InnerJoin("bm_crm_messages m", "m.id=i.message_id").Where("i.company_id", w["company_id"]).Where("i.dept_id", w["dept_id"]).Where(scope).Where("i.created_at>=?", w["created_at"].Time()).Fields("i.*,m.from_address,m.body_text,m.body_html,m.subject,m.customer_id").Order("i.id DESC").Limit(100)
		if len(c.InboxRuleIDs) > 0 {
			q = q.WhereIn("i.rule_id", c.InboxRuleIDs)
		}
		inquiries, e := q.All()
		if e != nil {
			return e
		}
		for _, r := range inquiries {
			body := r["body_text"].String()
			if body == "" {
				body = stripTags.ReplaceAllString(r["body_html"].String(), " ")
			}
			_, e = g.DB().Model("bm_gm_events").Ctx(ctx).Data(g.Map{"company_id": w["company_id"], "dept_id": w["dept_id"], "owner_id": w["owner_id"], "channel": "email", "event_key": fmt.Sprintf("email-inquiry:%d", r["id"].Int64()), "external_id": r["message_id"].String(), "sender": r["from_address"].String(), "body": cleanText(newReplyText(body), 12000), "kind": r["kind"].String(), "confidence": r["confidence"].Float64(), "analysis": r["analysis"].String(), "payload": jsonText(g.Map{"inquiry_id": r["id"].Int64(), "message_id": r["message_id"].Int64(), "customer_id": r["customer_id"].Int64(), "subject": r["subject"].String()})}).OnConflict("event_key").InsertIgnore()
			if e != nil {
				return e
			}
		}
		events, e := g.DB().Model("bm_gm_events").Ctx(ctx).Where("company_id", w["company_id"]).Where("dept_id", w["dept_id"]).Where("status", "new").Where("(expires_at IS NULL OR expires_at>now())").Order("id").Limit(30).All()
		if e != nil {
			return e
		}
		for _, event := range events {
			_, e = masterNewRun(ctx, p, w, fmt.Sprintf("event:%d", event["id"].Int64()), event["id"].Int64())
			if e != nil {
				_, _ = g.DB().Model("bm_gm_workflows").Where("id", w["id"]).Data("error", cleanText(e.Error(), 500)).Update()
				break
			}
		}
	}
	return nil
}
func (x *masterExec) replies() (g.Map, error) {
	if x.Snapshot.EventID == 0 {
		return g.Map{"processed": 0, "note": "Enable this event workflow to process new linked inbox inquiries and signed WhatsApp events"}, nil
	}
	event, e := masterRecord(x.Ctx, x.P, "bm_gm_events", x.Snapshot.EventID, "intelligence:run")
	if e != nil {
		return nil, e
	}
	if !event["expires_at"].IsNil() && event["expires_at"].Time().Before(time.Now()) {
		return nil, errors.New("inbound event expired")
	}
	kind := event["kind"].String()
	analysis := jsonMap(event["analysis"])
	payload := jsonMap(event["payload"])
	if event["channel"].String() != "email" && event["channel"].String() != "whatsapp" {
		return nil, errors.New("this channel does not authorize inquiry profiling or reply generation")
	}
	type classification struct {
		Kind       string   `json:"kind"`
		Confidence float64  `json:"confidence"`
		Summary    string   `json:"summary"`
		Language   string   `json:"language"`
		NeedsHuman bool     `json:"needs_human"`
		Paragraphs []string `json:"paragraphs"`
	}
	result := classification{Kind: kind, Confidence: event["confidence"].Float64(), NeedsHuman: true}
	if kind != "automated" && kind != "unsubscribe" && kind != "bounce" && kind != "unrelated" {
		if e = x.ai("Classify only the new inbound text, ignoring quoted marketing text. kind must be inquiry, automated, unsubscribe, complaint, unrelated or review. For genuine inquiries draft a cautious response using approved facts; never invent price, payment terms, warranty or contractual promises. For pricing, payment, contract, certification or complex technical issues set needs_human=true and request missing details. Never send. Return no paragraphs for automated, unsubscribe or complaint messages.", g.Map{"message": event["body"].String(), "subject": payload["subject"], "approved_facts": x.Snapshot.Sources, "industry": x.Snapshot.Pack}, schema(g.Map{"kind": strSchema(), "confidence": g.Map{"type": "number"}, "summary": strSchema(), "language": strSchema(), "needs_human": g.Map{"type": "boolean"}, "paragraphs": stringsSchema()}), &result); e != nil {
			return nil, e
		}
		if !strings.Contains("|inquiry|automated|unsubscribe|complaint|unrelated|review|", "|"+result.Kind+"|") || result.Confidence < .85 {
			result.Kind = "review"
			result.NeedsHuman = true
		}
		kind = result.Kind
	}
	// Update permission before any draft/follow-up write, including when the model is unavailable.
	if kind == "unsubscribe" || kind == "complaint" || kind == "bounce" {
		if event["channel"].String() == "email" {
			if e = deliveryprefs.Record(x.Ctx, event["sender"].String(), "", 0, "master-inbound-"+kind); e != nil {
				return nil, e
			}
		}
		field := "email"
		if event["channel"].String() == "whatsapp" {
			field = "regexp_replace(phone,'[^0-9]','','g')"
		}
		_, e = g.DB().Model("bm_gm_prospects").Ctx(x.Ctx).Where("company_id", x.Run["company_id"]).Where("dept_id", x.P.DeptID).Where(field+"=?", event["sender"].String()).Data(g.Map{"suppressed": true, "contact_permission": "blocked", "wa_opt_in": false}).Update()
		if e != nil {
			return nil, e
		}
	}
	if kind != "automated" && kind != "unrelated" {
		q := g.DB().Model("bm_gm_prospects").Ctx(x.Ctx).Where("company_id", x.Run["company_id"]).Where("dept_id", x.P.DeptID)
		if event["channel"].String() == "email" {
			q = q.Where("email", strings.ToLower(event["sender"].String()))
		} else {
			q = q.Where("regexp_replace(phone,'[^0-9]','','g')=?", event["sender"].String())
		}
		rows, e := q.All()
		if e != nil {
			return nil, e
		}
		for _, r := range rows {
			if _, e = g.DB().Model("bm_gm_prospects").Ctx(x.Ctx).Where("id", r["id"]).Data("replied_at", time.Now()).Update(); e != nil {
				return nil, e
			}
			if e = x.linkProspect(r["id"].Int64()); e != nil {
				return nil, e
			}
		}
	}
	if kind == "inquiry" || kind == "review" {
		cid := gconv.Int64(payload["customer_id"])
		if cid > 0 {
			if e = masterRequired(x.P, "crm:list", "crm:manage"); e != nil {
				return nil, e
			}
			if _, e = crm.Customer(x.Ctx, x.P, cid, "crm:manage"); e != nil {
				return nil, e
			}
			e = g.DB().Transaction(x.Ctx, func(ctx context.Context, tx gdb.TX) error {
				current, e := tx.GetOne(`SELECT analysis FROM bm_gm_events WHERE id=$1 FOR UPDATE`, event["id"])
				if e != nil {
					return e
				}
				if gconv.Bool(jsonMap(current["analysis"])["followup_created"]) {
					return nil
				}
				_, e = tx.Model("bm_crm_followups").Data(g.Map{"customer_id": cid, "dept_id": x.P.DeptID, "owner_id": x.P.ID, "content": "Growth Master inquiry: " + cleanText(result.Summary, 1000) + "\nManual review required before sending.\n" + strings.Join(result.Paragraphs, "\n\n")}).Insert()
				if e != nil {
					return e
				}
				if _, e = tx.Model("bm_crm_customers").Where("id", cid).WhereIn("stage", []string{"new", "inquiry"}).Data(g.Map{"stage": "inquiry", "next_followup_at": time.Now().Add(24 * time.Hour)}).Update(); e != nil {
					return e
				}
				_, e = tx.Model("bm_gm_events").Where("id", event["id"]).Data("analysis", jsonText(g.Map{"followup_created": true})).Update()
				return e
			})
			if e != nil {
				return nil, e
			}
			analysis["followup_created"] = true
		}
	}
	analysis["classification"] = result
	analysis["reply_draft_html"] = paragraphHTML(result.Paragraphs)
	analysis["human_review_required"] = true
	if kind == "inquiry" {
		if id, err := x.autoReply(event, analysis, result.Language, result.NeedsHuman); err != nil {
			return nil, err
		} else if id > 0 {
			analysis["human_review_required"] = gconv.String(analysis["automatic_reply_status"]) != "sent"
		}
	}
	_, e = g.DB().Model("bm_gm_events").Ctx(x.Ctx).Where("id", event["id"]).Data(g.Map{"kind": kind, "confidence": result.Confidence, "status": "processed", "analysis": jsonText(analysis)}).Update()
	return g.Map{"kind": kind, "needs_human": gconv.Bool(analysis["human_review_required"]), "draft": result.Paragraphs, "event_id": event["id"].Int64()}, e
}
func (x *masterExec) analytics() (g.Map, error) {
	from := time.Now().AddDate(0, 0, -7)
	cid := x.Run["company_id"].Int64()
	dept := x.P.DeptID
	counts, e := g.DB().GetAll(x.Ctx, `SELECT channel,status,count(*) count FROM bm_gm_contents WHERE company_id=$1 AND dept_id=$2 AND created_at>=$3 GROUP BY channel,status ORDER BY channel,status`, cid, dept, from)
	if e != nil {
		return nil, e
	}
	leads, e := g.DB().GetOne(x.Ctx, `SELECT count(*) discovered,count(*) FILTER(WHERE state='verified') verified,count(*) FILTER(WHERE state='unconfirmed') unconfirmed,count(*) FILTER(WHERE state='invalid') invalid,count(*) FILTER(WHERE crm_id IS NOT NULL) crm_linked FROM bm_gm_prospects WHERE company_id=$1 AND dept_id=$2 AND created_at>=$3`, cid, dept, from)
	if e != nil {
		return nil, e
	}
	inquiries, e := g.DB().GetAll(x.Ctx, `SELECT kind,count(*) count FROM bm_gm_events WHERE company_id=$1 AND dept_id=$2 AND created_at>=$3 GROUP BY kind`, cid, dept, from)
	if e != nil {
		return nil, e
	}
	usage, e := g.DB().GetOne(x.Ctx, `SELECT coalesce(sum(tokens),0) tokens,coalesce(sum(cost_micros),0) estimated_cost_micros,count(*) FILTER(WHERE kind='search') searches,count(*) FILTER(WHERE kind='scrape') scrapes FROM bm_gm_usage WHERE company_id=$1 AND dept_id=$2 AND created_at>=$3`, cid, dept, from)
	if e != nil {
		return nil, e
	}
	out := g.Map{"period_start": from, "as_of": time.Now(), "deliveries": counts, "leads": leads, "inquiries": inquiries, "usage": usage, "roi": nil, "roi_reason": "Verified order revenue and attributable margin are not connected", "tracking_note": "SMTP acceptance is not inbox delivery; opens may reflect proxies or privacy features", "recommendations": []string{}}
	tracking, e := g.DB().GetOne(x.Ctx, `WITH tasks AS(SELECT task_id FROM bm_gm_contents WHERE company_id=$1 AND dept_id=$2 AND channel='email' AND task_id IS NOT NULL UNION SELECT m.task_id FROM bm_growth_messages m JOIN bm_growth_plans p ON p.id=m.plan_id WHERE p.company_id=$1 AND m.dept_id=$2 AND m.task_id IS NOT NULL),recipients AS(SELECT r.*,t.track_open,t.track_click FROM recipient_info r JOIN tasks q ON q.task_id=r.task_id JOIN email_tasks t ON t.id=r.task_id WHERE t.start_time>=extract(epoch from $3::timestamptz)) SELECT count(*) recipients,count(*) FILTER(WHERE delivery_state='accepted' OR is_sent=1) accepted,count(*) FILTER(WHERE track_open=1 AND (delivery_state='accepted' OR is_sent=1)) open_tracked,count(*) FILTER(WHERE track_click=1 AND (delivery_state='accepted' OR is_sent=1)) click_tracked,count(*) FILTER(WHERE EXISTS(SELECT 1 FROM mailstat_opened o WHERE o.message_id=btrim(r.message_id,'<>') AND lower(o.recipient)=lower(r.recipient))) opened,count(*) FILTER(WHERE EXISTS(SELECT 1 FROM mailstat_clicked c WHERE c.message_id=btrim(r.message_id,'<>') AND lower(c.recipient)=lower(r.recipient))) clicked FROM recipients r`, cid, dept, from)
	if e != nil {
		return nil, e
	}
	out["email_tracking"] = tracking
	social, e := g.DB().GetAll(x.Ctx, `SELECT a.platform,d.status,count(*) count FROM bm_social_deliveries d JOIN bm_social_plans p ON p.id=d.plan_id JOIN bm_social_accounts a ON a.id=d.account_id WHERE p.company_id=$1 AND p.dept_id=$2 AND d.created_at>=$3 GROUP BY a.platform,d.status`, cid, dept, from)
	if e != nil {
		return nil, e
	}
	out["native_social_deliveries"] = social
	metrics, e := g.DB().GetAll(x.Ctx, `SELECT a.platform,m.counts,m.availability,m.as_of FROM bm_social_metrics m JOIN bm_social_deliveries d ON d.id=m.delivery_id JOIN bm_social_plans p ON p.id=d.plan_id JOIN bm_social_accounts a ON a.id=d.account_id WHERE p.company_id=$1 AND p.dept_id=$2 AND m.expires_at>now() AND m.as_of>=$3 ORDER BY m.as_of DESC LIMIT 100`, cid, dept, from)
	if e != nil {
		return nil, e
	}
	out["social_metrics"] = metrics
	out["attribution_scope"] = "Company-linked native tasks only; native social totals include Growth Master tasks and must not be added to the separate master delivery breakdown. Latest 100 available social metric snapshots; unavailable metrics remain unavailable."
	var result struct {
		Recommendations []string `json:"recommendations"`
	}
	if e = x.ai("Suggest at most five operational improvements using only these measured counts. Do not infer revenue, inbox delivery, market size or causal effects. Recommendations never modify live plans or approve content automatically.", out, schema(g.Map{"recommendations": stringsSchema()}), &result); e != nil {
		return nil, e
	}
	out["recommendations"] = result.Recommendations
	return out, nil
}
