package growth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"billionmail-core/internal/service/governance"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/gconv"
)

// Deletion is persistent and stops future execution; historical receipts are kept.
func DeleteRecord(ctx context.Context, p *governance.Principal, module, kind string, id int64) error {
	defs := map[string][2]string{
		"plans": {"bm_growth_plans", "outreach:manage"}, "runs": {"bm_growth_runs", "outreach:manage"}, "candidates": {"bm_growth_candidates", "outreach:manage"}, "leads": {"bm_growth_leads", "outreach:manage"}, "messages": {"bm_growth_messages", "outreach:manage"}, "inquiries": {"bm_growth_inquiries", "inquiry:manage"}, "inbox_rules": {"bm_growth_inbox_rules", "inquiry:manage"}, "jobs": {"bm_growth_jobs", "outreach:manage"}, "products": {"bm_growth_products", "product:manage"},
	}
	if module == "master" {
		defs = map[string][2]string{"workflows": {"bm_gm_workflows", "intelligence:manage"}, "runs": {"bm_gm_runs", "intelligence:manage"}, "prospects": {"bm_gm_prospects", "intelligence:manage"}, "contents": {"bm_gm_contents", "intelligence:manage"}, "events": {"bm_gm_events", "intelligence:manage"}, "evidence": {"bm_gm_evidence", "intelligence:manage"}, "audit": {"bm_gm_audit", "intelligence:settings"}, "packs": {"bm_gm_packs", "intelligence:settings"}}
	} else if module != "growth" {
		return errors.New("invalid deletion module")
	}
	def, ok := defs[kind]
	if !ok {
		return errors.New("unsupported deletion resource")
	}
	table, permission := def[0], def[1]
	if e := require(p, permission); e != nil {
		return e
	}
	if module == "growth" && kind == "jobs" {
		permission = jobPermission(ctx, id)
		if e := require(p, permission); e != nil {
			return e
		}
	}
	return g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if _, e := tx.Exec(`SELECT pg_advisory_xact_lock(614806)`); e != nil {
			return e
		}
		row, e := tx.GetOne(fmt.Sprintf("SELECT * FROM %s WHERE id=$1 AND deleted_at IS NULL FOR UPDATE", table), id)
		if e != nil {
			return e
		}
		if len(row) == 0 {
			return errors.New("record unavailable")
		}
		if row["dept_id"].Int64() == 0 && kind == "packs" {
			if !p.Super {
				return errors.New("administrator required for built-in industry templates")
			}
		} else if e = p.CheckOwnership(ctx, permission, row["dept_id"].Int64(), row["owner_id"].Int64()); e != nil {
			return e
		}
		if row["status"].String() == "running" || row["status"].String() == "sending" || row["status"].String() == "publishing" {
			return errors.New("wait for active execution before deleting")
		}
		values := g.Map{"deleted_at": time.Now()}
		if module == "master" {
			switch kind {
			case "workflows":
				n, e := tx.Model("bm_gm_runs").Where("workflow_id", id).Where("status", "running").Count()
				if e != nil {
					return e
				}
				if n > 0 {
					return errors.New("pause the active workflow and wait for execution to stop")
				}
				values["active"] = false
				if _, e = tx.Model("bm_gm_runs").Where("workflow_id", id).WhereNotIn("status", []string{"completed", "no_results", "cancelled"}).Data(g.Map{"status": "cancelled", "lease_token": nil, "lease_until": nil}).Update(); e != nil {
					return e
				}
			case "runs":
				values["status"] = "cancelled"
			case "prospects":
				values["suppressed"] = true
			case "contents":
				if e = stopAttachedEmail(ctx, tx, "master_content_id", id); e != nil {
					return e
				}
				n, e := tx.Model("bm_social_deliveries d").InnerJoin("bm_social_plans p", "p.id=d.plan_id").Where("p.master_content_id", id).Where("d.status", "publishing").Count()
				if e != nil {
					return e
				}
				if n > 0 {
					return errors.New("wait for in-flight publishing")
				}
				if _, e = tx.Model("bm_social_plans").Where("master_content_id", id).Data("status", "paused").Update(); e != nil {
					return e
				}
				if row["status"].String() != "accepted" && row["status"].String() != "published" && row["status"].String() != "unknown" {
					values["status"] = "rejected"
				}
			case "packs":
				values["active"] = false
			}
		} else {
			switch kind {
			case "plans", "runs":
				query := "payload->>'run_id'=?"
				arg := fmt.Sprint(id)
				if kind == "plans" {
					query = "payload->>'run_id' IN(SELECT id::text FROM bm_growth_runs WHERE plan_id=?)"
				}
				n, e := tx.Model("bm_growth_jobs").Where(query, arg).Where("status", "running").Count()
				if e != nil {
					return e
				}
				if n > 0 {
					return errors.New("cancel running jobs and wait for them to stop before deleting")
				}
				if _, e = tx.Model("bm_growth_jobs").Where(query, arg).Where("status", "queued").Data("status", "cancelled").Update(); e != nil {
					return e
				}
				messageQuery := "run_id=?"
				if kind == "plans" {
					values["active"] = false
					messageQuery = "plan_id=?"
				} else {
					values["status"] = "cancelled"
				}
				messages, e := tx.Model("bm_growth_messages").Where(messageQuery, id).WhereIn("status", []string{"draft", "approved", "queued"}).All()
				if e != nil {
					return e
				}
				for _, m := range messages {
					if e = stopAttachedEmail(ctx, tx, "outreach_message_id", m["id"].Int64()); e != nil {
						return e
					}
				}
				if _, e = tx.Model("bm_growth_messages").Where(messageQuery, id).WhereIn("status", []string{"draft", "approved", "queued"}).Data("status", "cancelled").Update(); e != nil {
					return e
				}
			case "leads":
				values["suppressed"] = true
			case "messages":
				if e = stopAttachedEmail(ctx, tx, "outreach_message_id", id); e != nil {
					return e
				}
				if row["status"].String() != "sent" && row["status"].String() != "unknown" {
					values["status"] = "cancelled"
				}
			case "inbox_rules":
				values["active"] = false
			case "jobs":
				values["status"] = "cancelled"
			case "products":
				values["published"] = false
			case "candidates":
				values["state"] = "filtered"
			}
		}
		if _, e = tx.Model(table).Where("id", id).Data(values).Update(); e != nil {
			return e
		}
		return governance.Audit(ctx, p, "delete:"+module+":"+kind, id, g.Map{"retained_history": true})
	})
}
func stopAttachedEmail(ctx context.Context, tx gdb.TX, column string, id int64) error {
	n, e := tx.Model("recipient_info").Where(column, id).WhereIn("delivery_state", []string{"claimed", "sending"}).Count()
	if e != nil {
		return e
	}
	if n > 0 {
		return errors.New("wait for in-flight email delivery")
	}
	if _, e = tx.Exec("UPDATE email_tasks SET pause=1 WHERE id IN(SELECT task_id FROM recipient_info WHERE "+column+"=$1)", id); e != nil {
		return e
	}
	_, e = tx.Model("recipient_info").Where(column, id).WhereIn("delivery_state", []string{"pending", "retry", "held"}).Data("delivery_state", "cancelled").Update()
	return e
}

func ExpandIntentKeywords(ctx context.Context, p *governance.Principal, data g.Map) (g.Map, error) {
	if !p.Has("intelligence:manage") && !p.Has("outreach:manage") {
		return nil, errors.New("keyword editing permission required")
	}
	var result struct {
		Keywords []string `json:"keywords"`
	}
	seed := cleanText(gconv.String(data["seed"]), 3000)
	if seed == "" {
		return nil, errors.New("enter seed keywords or purchasing requirements")
	}
	e := IntelligenceProvider.Extract(ctx, p, "intent_keywords", AIInput{Model: gconv.String(data["model"]), MaxOutput: 2000, Instruction: "Expand public B2B purchasing intent search keywords for the specified industry, market and language. Return 5-25 concise, relevant search phrases. Do not invent buyers or procurement facts. Use the requested language. These are suggestions for human selection, not proof of purchase intent.", Data: g.Map{"seed": seed, "industry": cleanText(gconv.String(data["industry"]), 1000), "country": cleanText(gconv.String(data["country"]), 80), "language": cleanText(gconv.String(data["language"]), 30)}, Schema: schema(g.Map{"keywords": stringsSchema()})}, &result)
	if e != nil {
		return nil, e
	}
	seen := map[string]bool{}
	out := []string{}
	for _, word := range result.Keywords {
		word = cleanText(word, 160)
		key := strings.ToLower(word)
		if word != "" && !seen[key] && len(out) < 25 {
			out = append(out, word)
			seen[key] = true
		}
	}
	return g.Map{"keywords": out}, nil
}
