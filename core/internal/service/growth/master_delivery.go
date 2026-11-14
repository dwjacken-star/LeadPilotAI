package growth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"billionmail-core/internal/service/batch_mail"
	"billionmail-core/internal/service/deliveryprefs"
	"billionmail-core/internal/service/governance"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/gconv"
	"github.com/google/uuid"
)

func init() { batch_mail.MasterDeliveryGuard = MasterDeliveryGuard }
func MasterSocialDeliveryGuard(ctx context.Context, p *governance.Principal, id int64) error {
	if id == 0 {
		return nil
	}
	r, e := masterRecord(ctx, p, "bm_gm_contents", id, "intelligence:publish")
	if e != nil {
		return e
	}
	if r["status"].String() != "queued" || r["approved_hash"].String() != masterContentFingerprint(r) {
		return errors.New("Growth Master approval changed")
	}
	run, e := masterRecord(ctx, p, "bm_gm_runs", r["run_id"].Int64(), "intelligence:run")
	if e != nil {
		return e
	}
	if masterStopped(ctx) || run["status"].String() == "paused" || run["status"].String() == "cancelled" {
		return errors.New("Growth Master execution paused")
	}
	if e = masterReviewStillValid(ctx, r); e != nil {
		return e
	}
	s, e := masterDecodeSnapshot(run)
	if e != nil {
		return e
	}
	return (&masterExec{Ctx: ctx, P: p, Run: run, Snapshot: s}).sourceCurrent()
}

func masterSources(ctx context.Context, p *governance.Principal, cid int64, ids []int64) (g.Map, error) {
	c, e := company(ctx, p, cid, "intelligence:view", false)
	if e != nil {
		return nil, e
	}
	products := []g.Map{}
	versions := g.Map{}
	for _, id := range ids {
		r, e := Product(ctx, p, id, "product:list")
		if e != nil {
			return nil, e
		}
		if r["company_id"].Int64() != cid || !r["published"].Bool() {
			return nil, errors.New("offering publication revoked")
		}
		products = append(products, g.Map{"id": id, "version": r["version"].Int(), "fields": jsonMap(r["published_fields"])})
		versions[fmt.Sprint(id)] = r["version"].Int()
	}
	assets := []g.Map{}
	if p.Has("material:list") {
		rows, e := g.DB().Model("bm_growth_assets").Ctx(ctx).Where("company_id", cid).Where("published", true).Where("visibility", "public").Where("ai_allowed", true).WhereNull("deleted_at").WhereNot("category", "internal").Order("id").Limit(12).All()
		if e != nil {
			return nil, e
		}
		for _, r := range rows {
			assets = append(assets, g.Map{"id": r["id"].String(), "version": r["version"].Int(), "filename": r["filename"].String(), "facts": SocialPublicFacts(jsonMap(r["extracted"])), "content": cleanText(r["content"].String(), 4000)})
		}
	}
	return g.Map{"company_id": cid, "company_name": c["name"].String(), "company_version": c["version"].Int(), "brand": c["brand"].String(), "company": jsonMap(c["published_profile"]), "products": products, "product_versions": versions, "materials": assets}, nil
}
func (x *masterExec) sourceCurrent() error {
	current, e := masterSources(x.Ctx, x.P, x.Run["company_id"].Int64(), x.Snapshot.Config.ProductIDs)
	if e != nil {
		return e
	}
	current["images"], e = masterImageFacts(x.Ctx, x.P, x.Run["company_id"].Int64(), x.Snapshot.Config.ImageAssetIDs)
	if e != nil {
		return e
	}
	if masterHash(current) != masterHash(x.Snapshot.Sources) {
		return &masterWait{State: "configuration_required", Reason: "Approved company/product/material facts changed; cancel this run and regenerate content"}
	}
	return nil
}
func masterContentFingerprint(r gdb.Record) string {
	return masterHash(g.Map{"channel": r["channel"].String(), "target": r["target"].String(), "country": r["country"].String(), "language": r["language"].String(), "subject": r["subject"].String(), "body": r["body"].String(), "html": r["html"].String(), "images": gconv.Strings(r["images"].Interface()), "snapshot": jsonMap(r["snapshot"])})
}
func (x *masterExec) compose() (g.Map, error) {
	if e := x.sourceCurrent(); e != nil {
		return nil, e
	}
	c := x.Snapshot.Config
	prospects := gdb.Result{}
	for _, ch := range c.Channels {
		if ch == "email" || ch == "whatsapp" {
			q := g.DB().Model("bm_gm_prospects").Ctx(x.Ctx).Where("company_id", x.Run["company_id"]).Where("dept_id", x.P.DeptID).Where("state", "verified").Where("suppressed", false).WhereNull("replied_at").Where("score>=?", x.Snapshot.Pack.MinScore)
			if len(c.ProspectIDs) > 0 {
				q = q.WhereIn("id", c.ProspectIDs)
			}
			scope, e := x.P.Predicate(x.Ctx, "intelligence:view", "")
			if e != nil {
				return nil, e
			}
			var e2 error
			prospects, e2 = q.Where(scope).Order("score DESC,id").Limit(x.limit()).All()
			if e2 != nil {
				return nil, e2
			}
			break
		}
	}
	count := 0
	makeContent := func(channel, target string, prospect gdb.Record, m Market) error {
		existing, e := g.DB().Model("bm_gm_contents").Ctx(x.Ctx).Where("run_id", x.Run["id"]).Where("channel", channel).Where("target", target).Where("country", m.Country).Where("language", m.Language).Count()
		if e != nil {
			return e
		}
		if existing > 0 {
			count++
			return nil
		}
		var result struct {
			Subject    string   `json:"subject"`
			Paragraphs []string `json:"paragraphs"`
			Language   string   `json:"language"`
		}
		var customer interface{} = g.Map{}
		pid := int64(0)
		if len(prospect) > 0 {
			pid = prospect["id"].Int64()
			customer = g.Map{"company": prospect["company"].String(), "facts": jsonMap(prospect["facts"]), "matching": jsonMap(prospect["matching"])}
			if e = x.linkProspect(pid); e != nil {
				return e
			}
		}
		if e = x.ai("Create concise factual marketing content for the specified channel, country and language. Use only approved company/product/service facts; customer evidence is context, never supplier claims. Respect industry forbidden claims. No prices, legal commitments, unsupported certifications or invented compatibility. Return plain text paragraphs without HTML, scripts or template directives. Subject is a short email subject or social post title; language must equal the requested code. Content will be reviewed by a human before dispatch.", g.Map{"industry": x.Snapshot.Pack, "approved_facts": x.Snapshot.Sources, "customer": customer, "requirement": c.Requirement, "channel": channel, "country": m.Country, "language": m.Language}, schema(g.Map{"subject": strSchema(), "paragraphs": stringsSchema(), "language": strSchema()}), &result); e != nil {
			return e
		}
		body := strings.TrimSpace(strings.Join(result.Paragraphs, "\n\n"))
		if result.Subject == "" || strings.ContainsAny(result.Subject, "\r\n") || body == "" || len(body) > 30000 || result.Language != m.Language || strings.Contains(body, "{{") {
			return errors.New("invalid generated marketing content")
		}
		if channel == "linkedin" && len([]rune(body)) > 2800 {
			return errors.New("LinkedIn content exceeds channel limit")
		}
		snapshot := g.Map{"workflow_version": x.Snapshot.WorkflowVersion, "sources": x.Snapshot.Sources, "pack_id": x.Snapshot.PackID, "pack_version": x.Snapshot.PackVersion, "config": c, "model": x.Settings.Model, "channel_target": target}
		if channel == "whatsapp" {
			template, params, rendered, e := masterWATemplate(x.Ctx, x.Run["company_id"].Int64(), m.Language, result.Subject, body)
			if e != nil {
				return e
			}
			wa, e := masterWA(x.Ctx, x.Run["company_id"].Int64())
			if e != nil {
				return e
			}
			snapshot["wa_template"], snapshot["wa_parameters"], snapshot["wa_phone_id"] = template, params, wa.PhoneNumberID
			body = rendered
			result.Paragraphs = strings.Split(rendered, "\n\n")
		}
		values := g.Map{"run_id": x.Run["id"], "company_id": x.Run["company_id"], "dept_id": x.P.DeptID, "owner_id": x.P.ID, "prospect_id": nullableID(pid), "channel": channel, "target": target, "country": m.Country, "language": m.Language, "subject": cleanText(result.Subject, 300), "body": body, "html": paragraphHTML(result.Paragraphs), "images": "[]", "snapshot": jsonText(snapshot), "fingerprint": "pending"}
		if channel == "facebook" || channel == "linkedin" {
			refs := []string{}
			for _, id := range c.ImageAssetIDs {
				refs = append(refs, "library:"+id)
			}
			values["images"] = jsonText(refs)
		}
		_, e = g.DB().Model("bm_gm_contents").Ctx(x.Ctx).Data(values).OnConflict("run_id,channel,target,country,language").InsertIgnore()
		if e == nil {
			count++
		}
		return e
	}
	for _, ch := range c.Channels {
		if ch == "email" || ch == "whatsapp" {
			for _, p := range prospects {
				if ch == "whatsapp" && (!p["wa_opt_in"].Bool() || p["phone"].String() == "") {
					continue
				}
				for _, m := range c.Markets {
					if m.Country != p["country"].String() {
						continue
					}
					target := p["email"].String()
					if ch == "whatsapp" {
						target = masterPhone(p["phone"].String())
					}
					if target == "" {
						continue
					}
					if e := makeContent(ch, target, p, m); e != nil {
						return nil, e
					}
					break
				}
			}
		} else {
			for _, account := range c.SocialAccounts {
				if MasterSocialPlatform == nil {
					return nil, errors.New("social integration unavailable")
				}
				platform, e := MasterSocialPlatform(x.Ctx, x.P, account)
				if e != nil {
					return nil, e
				}
				if platform != ch {
					continue
				}
				for _, m := range c.Markets {
					if e := makeContent(ch, fmt.Sprint(account), nil, m); e != nil {
						return nil, e
					}
				}
			}
		}
	}
	return g.Map{"content_count": count, "review_required": true, "explanation": "No content is sent until its exact version and recipient/channel target are approved"}, nil
}
func masterContentControl(ctx context.Context, p *governance.Principal, id int64, operation string, data g.Map) error {
	permission := "intelligence:review"
	if operation == "save" {
		permission = "intelligence:manage"
	}
	r, e := masterRecord(ctx, p, "bm_gm_contents", id, permission)
	if e != nil {
		return e
	}
	if !strings.Contains("|draft|approved|rejected|blocked|", "|"+r["status"].String()+"|") {
		return errors.New("content was already handed off and cannot be edited or approved again")
	}
	return g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		current, e := tx.GetOne(`SELECT * FROM bm_gm_contents WHERE id=$1 FOR UPDATE`, id)
		if e != nil {
			return e
		}
		if !strings.Contains("|draft|approved|rejected|blocked|", "|"+current["status"].String()+"|") {
			return errors.New("content is already queued")
		}
		values := g.Map{}
		switch operation {
		case "save":
			if current["channel"].String() == "whatsapp" {
				return errors.New("regenerate WhatsApp content from its approved template mapping")
			}
			subject, body := strings.TrimSpace(gconv.String(data["subject"])), strings.TrimSpace(gconv.String(data["body"]))
			if subject == "" || strings.ContainsAny(subject, "\r\n") || body == "" || len(body) > 30000 {
				return errors.New("valid subject and content required")
			}
			if current["channel"].String() == "linkedin" && len([]rune(body)) > 2800 {
				return errors.New("LinkedIn content exceeds channel limit")
			}
			values = g.Map{"subject": cleanText(subject, 300), "body": body, "html": paragraphHTML(strings.Split(body, "\n\n")), "status": "draft", "approved_hash": "", "approved_by": nil, "approved_at": nil, "error": ""}
		case "reject":
			values = g.Map{"status": "rejected", "approved_hash": "", "approved_by": p.ID, "approved_at": time.Now(), "error": cleanText(gconv.String(data["reason"]), 1000)}
		case "approve":
			run, e := masterRecord(ctx, p, "bm_gm_runs", r["run_id"].Int64(), "intelligence:view")
			if e != nil {
				return e
			}
			s, e := masterDecodeSnapshot(run)
			if e != nil {
				return e
			}
			x := &masterExec{Ctx: ctx, P: p, Run: run, Snapshot: s}
			if e = x.sourceCurrent(); e != nil {
				return e
			}
			if r["channel"].String() == "email" || r["channel"].String() == "whatsapp" {
				pr, e := masterRecord(ctx, p, "bm_gm_prospects", r["prospect_id"].Int64(), "intelligence:review")
				if e != nil {
					return e
				}
				if pr["contact_permission"].String() != "allowed" || pr["permission_evidence"].String() == "" || pr["suppressed"].Bool() || pr["state"].String() != "verified" || !pr["replied_at"].IsNil() {
					return errors.New("recipient verification and contact permission evidence required")
				}
				if r["channel"].String() == "whatsapp" && !pr["wa_opt_in"].Bool() {
					return errors.New("WhatsApp opt-in required")
				}
			}
			values = g.Map{"status": "approved", "approved_hash": masterContentFingerprint(current), "approved_by": p.ID, "approved_at": time.Now(), "error": ""}
		default:
			return errors.New("invalid content action")
		}
		if _, e = tx.Model("bm_gm_contents").Where("id", id).Data(values).Update(); e != nil {
			return e
		}
		if e = masterAudit(ctx, p, r["company_id"].Int64(), r["run_id"].Int64(), id, "content_"+operation, g.Map{}); e != nil {
			return e
		}
		if operation == "approve" || operation == "reject" {
			_, e = tx.Model("bm_gm_runs").Where("id", r["run_id"]).Where("status", "waiting_approval").Data(g.Map{"status": "queued", "next_at": time.Now(), "error": ""}).Update()
		}
		return e
	})
}
func (x *masterExec) dispatch() (g.Map, error) {
	if e := masterRequired(x.P, "intelligence:publish"); e != nil {
		return nil, e
	}
	if e := x.sourceCurrent(); e != nil {
		return nil, e
	}
	rows, e := g.DB().Model("bm_gm_contents").Ctx(x.Ctx).Where("run_id", x.Run["id"]).Order("id").All()
	if e != nil {
		return nil, e
	}
	pending := 0
	for _, r := range rows {
		if r["status"].String() == "draft" || r["status"].String() == "blocked" {
			pending++
		}
	}
	if pending > 0 {
		return g.Map{"pending_review": pending}, &masterWait{State: "waiting_approval", Reason: fmt.Sprintf("%d content versions await review", pending)}
	}
	queued := 0
	for _, r := range rows {
		state := r["status"].String()
		if state == "rejected" || state == "cancelled" {
			continue
		}
		if state != "approved" {
			queued++
			continue
		}
		if e = x.check(); e != nil {
			return nil, e
		}
		if r["approved_hash"].String() == "" || r["approved_hash"].String() != masterContentFingerprint(r) {
			return nil, errors.New("content changed after approval")
		}
		reviewer, e := governance.Load(x.Ctx, r["approved_by"].Int64())
		if e != nil || !reviewer.Has("intelligence:review") {
			return nil, errors.New("reviewer authorization revoked")
		}
		switch r["channel"].String() {
		case "email":
			e = x.emailHandoff(r)
		case "facebook", "linkedin":
			if MasterSocialHandoff == nil {
				return nil, errors.New("social integration not configured")
			}
			e = x.socialHandoff(r)
		case "whatsapp":
			e = x.whatsappHandoff(r)
		default:
			e = errors.New("unsupported channel")
		}
		if e != nil {
			return nil, e
		}
		queued++
	}
	return g.Map{"handed_off": queued, "total_content": len(rows), "receipt_note": "queued/accepted is distinct from delivery or publication"}, nil
}
func (x *masterExec) emailHandoff(content gdb.Record) error {
	if e := masterRequired(x.P, "campaign:create", "campaign:publish", "contact:list"); e != nil {
		return e
	}
	return g.DB().Transaction(x.Ctx, func(ctx context.Context, tx gdb.TX) error {
		if _, e := tx.Exec(`SELECT pg_advisory_xact_lock(615102)`); e != nil {
			return e
		}
		r, e := tx.GetOne(`SELECT * FROM bm_gm_contents WHERE id=$1 FOR UPDATE`, content["id"])
		if e != nil {
			return e
		}
		if r["status"].String() != "approved" {
			return nil
		}
		pr, e := masterRecord(ctx, x.P, "bm_gm_prospects", r["prospect_id"].Int64(), "intelligence:publish")
		if e != nil {
			return e
		}
		if pr["state"].String() != "verified" || pr["contact_permission"].String() != "allowed" || pr["suppressed"].Bool() || !pr["replied_at"].IsNil() || pr["email"].String() != r["target"].String() {
			return errors.New("recipient permission or verification was revoked")
		}
		c := x.Snapshot.Config
		cursor, e := tx.Model("bm_gm_workflows").Where("id", x.Run["workflow_id"]).Value("sender_cursor")
		if e != nil {
			return e
		}
		sender := c.Senders[int(cursor.Int64()%int64(len(c.Senders)))]
		if e = governance.SenderAllowed(ctx, x.P, sender); e != nil {
			return e
		}
		blocked, e := deliveryprefs.IsSuppressed(ctx, sender, pr["email"].String())
		if e != nil {
			return e
		}
		if blocked {
			return errors.New("recipient has unsubscribed or is suppressed")
		}
		recent, e := tx.Model("bm_gm_contents").Where("company_id", x.Run["company_id"]).Where("dept_id", x.P.DeptID).Where("target", r["target"]).Where("channel", "email").WhereNot("id", r["id"]).WhereIn("status", []string{"queued", "accepted", "sent", "unknown"}).Where("created_at>?", time.Now().AddDate(0, 0, -c.CooldownDays)).Count()
		if e != nil {
			return e
		}
		if recent > 0 {
			return &masterWait{State: "waiting_approval", Reason: "Recipient cooldown: another workflow has already reserved this contact"}
		}
		// An inbound human-looking message immediately stops follow-up marketing.
		replied, e := tx.Model("bm_crm_messages").Where("dept_id", x.P.DeptID).Where("direction", "incoming").Where("lower(from_address)=?", strings.ToLower(r["target"].String())).Where("created_at>?", x.Run["created_at"].Time()).Count()
		if e != nil {
			return e
		}
		if replied > 0 {
			return errors.New("recipient replied; review the inquiry before further outreach")
		}
		if _, e = x.reserve("send", 0); e != nil {
			return e
		}
		name := fmt.Sprintf("Growth Master / %d", x.Run["company_id"].Int64())
		group, e := tx.Model("bm_contact_groups").Where("dept_id", x.P.DeptID).Where("name", name).Value("id")
		if e != nil {
			return e
		}
		gid := group.Int64()
		if gid == 0 {
			gid, e = tx.Model("bm_contact_groups").Data(g.Map{"name": name, "dept_id": x.P.DeptID, "owner_id": x.P.ID, "token": "gm-" + uuid.NewString()[:20]}).InsertAndGetId()
			if e != nil {
				return e
			}
		}
		if _, e = tx.Model("bm_contacts").Data(g.Map{"group_id": gid, "email": r["target"].String(), "dept_id": x.P.DeptID, "owner_id": x.P.ID, "active": 1, "status": 1, "attribs": jsonText(g.Map{"master_prospect_id": pr["id"].Int64()})}).InsertIgnore(); e != nil {
			return e
		}
		contact, e := tx.Model("bm_contacts").Where("group_id", gid).Where("email", r["target"]).Value("id")
		if e != nil {
			return e
		}
		html := r["html"].String()
		policy := c.Policy
		policy.Timezone = c.Timezone
		for _, m := range c.Markets {
			if m.Country == r["country"].String() {
				policy.Timezone = m.Timezone
				break
			}
		}
		tid, e := batch_mail.CreateGovernedTask(ctx, tx, x.P, batch_mail.CampaignConfig{Name: fmt.Sprintf("Growth Master #%d", r["id"].Int64()), GroupID: gid, HTMLContent: &html, Sender: sender, Senders: []string{sender}, Subject: r["subject"].String(), CCEmail: c.CCEmail, Policy: policy, TrackOpen: c.TrackOpen, TrackClick: c.TrackClick}, true, contact.Int64())
		if e != nil {
			return e
		}
		if e = batch_mail.SetTaskDeliveryOptions(ctx, tid, batch_mail.TaskDeliveryOptions{Warmup: c.Warmup, Unsubscribe: c.Unsubscribe, TrackOpen: c.TrackOpen && c.CCEmail == "", TrackClick: c.TrackClick && c.CCEmail == ""}); e != nil {
			return e
		}
		if _, e = tx.Model("recipient_info").Where("task_id", tid).Data("master_content_id", r["id"].Int64()).Update(); e != nil {
			return e
		}
		if _, e = tx.Model("bm_gm_contents").Where("id", r["id"]).Data(g.Map{"status": "queued", "task_id": tid}).Update(); e != nil {
			return e
		}
		if _, e = tx.Model("bm_gm_workflows").Where("id", x.Run["workflow_id"]).Data("sender_cursor", gdb.Raw("sender_cursor+1")).Update(); e != nil {
			return e
		}
		_, e = tx.Model("bm_gm_prospects").Where("id", pr["id"]).Data("last_contact_at", time.Now()).Update()
		return e
	})
}
func (x *masterExec) socialHandoff(r gdb.Record) error {
	return g.DB().Transaction(x.Ctx, func(ctx context.Context, tx gdb.TX) error {
		current, e := tx.GetOne(`SELECT * FROM bm_gm_contents WHERE id=$1 FOR UPDATE`, r["id"])
		if e != nil {
			return e
		}
		if current["status"].String() != "approved" {
			return nil
		}
		if _, e = x.reserve("send", 0); e != nil {
			return e
		}
		id, e := MasterSocialHandoff(ctx, x.P, current, x.Snapshot)
		if e != nil {
			return e
		}
		_, e = tx.Model("bm_gm_contents").Where("id", r["id"]).Data(g.Map{"status": "queued", "task_id": id}).Update()
		return e
	})
}

// MasterDeliveryGuard is called at actual SMTP handoff as well as queue creation.
func MasterDeliveryGuard(ctx context.Context, p *governance.Principal, id int64) error {
	if id == 0 {
		return nil
	}
	r, e := masterRecord(ctx, p, "bm_gm_contents", id, "intelligence:publish")
	if e != nil {
		return e
	}
	if r["approved_hash"].String() != masterContentFingerprint(r) || r["status"].String() != "queued" {
		return errors.New("Growth Master content approval is no longer valid")
	}
	run, e := masterRecord(ctx, p, "bm_gm_runs", r["run_id"].Int64(), "intelligence:run")
	if e != nil {
		return e
	}
	if masterStopped(ctx) || run["status"].String() == "cancelled" || run["status"].String() == "paused" {
		return errors.New("Growth Master run paused or cancelled")
	}
	s, e := masterDecodeSnapshot(run)
	if e != nil {
		return e
	}
	x := &masterExec{Ctx: ctx, P: p, Run: run, Snapshot: s}
	if e = x.sourceCurrent(); e != nil {
		return e
	}
	pr, e := masterRecord(ctx, p, "bm_gm_prospects", r["prospect_id"].Int64(), "intelligence:publish")
	if e != nil {
		return e
	}
	if pr["state"].String() != "verified" || pr["suppressed"].Bool() || pr["contact_permission"].String() != "allowed" || !pr["replied_at"].IsNil() {
		return errors.New("recipient is no longer eligible")
	}
	if e = masterReviewStillValid(ctx, r); e != nil {
		return e
	}
	if replied, e := masterRecipientReplied(ctx, r); e != nil {
		return e
	} else if replied {
		return errors.New("recipient replied; marketing stopped pending inquiry review")
	}
	return nil
}
func masterReconcile(ctx context.Context) error {
	if _, e := g.DB().Exec(ctx, `UPDATE bm_gm_contents SET status='unknown',error='WhatsApp worker stopped; verify provider result before any resend' WHERE status='sending' AND sending_at<now()-interval '2 minutes'`); e != nil {
		return e
	}
	_, e := g.DB().Exec(ctx, `UPDATE bm_gm_contents c SET status=CASE WHEN r.delivery_state='unknown' THEN 'unknown' WHEN r.delivery_state IN('failed','cancelled','suppressed','bounced','skipped') THEN r.delivery_state WHEN r.delivery_state='accepted' OR r.is_sent=1 THEN 'accepted' ELSE c.status END,error=coalesce(nullif(r.last_error,''),c.error) FROM recipient_info r WHERE r.master_content_id=c.id AND c.channel='email' AND c.status='queued'`)
	if e != nil {
		return e
	}
	rows, e := g.DB().Model("bm_gm_contents").Ctx(ctx).WhereIn("channel", []string{"facebook", "linkedin"}).Where("status", "queued").Where("task_id IS NOT NULL").Limit(50).All()
	if e != nil {
		return e
	}
	if MasterSocialReceipt != nil {
		for _, r := range rows {
			p, e := workerPrincipal(ctx, r["owner_id"].Int64(), r["dept_id"].Int64(), "intelligence:view")
			if e != nil {
				continue
			}
			receipt, e := MasterSocialReceipt(ctx, p, r["task_id"].Int64())
			if e != nil {
				continue
			}
			state := gconv.String(receipt["status"])
			if state != "" {
				_, e = g.DB().Model("bm_gm_contents").Ctx(ctx).Where("id", r["id"]).Data(g.Map{"status": state, "external_id": gconv.String(receipt["external_id"]), "error": gconv.String(receipt["error"])}).Update()
				if e != nil {
					return e
				}
			}
		}
	}
	return nil
}

func masterReviewStillValid(ctx context.Context, r gdb.Record) error {
	reviewer, e := governance.Load(ctx, r["approved_by"].Int64())
	if e != nil {
		return e
	}
	if !reviewer.Has("intelligence:review") {
		return errors.New("content reviewer permission revoked")
	}
	if reviewer.Super {
		reviewer.DeptID = r["dept_id"].Int64()
	}
	return reviewer.CheckOwnership(ctx, "intelligence:review", r["dept_id"].Int64(), r["owner_id"].Int64())
}
func masterRecipientReplied(ctx context.Context, r gdb.Record) (bool, error) {
	rows, e := g.DB().Model("bm_crm_messages").Ctx(ctx).Where("dept_id", r["dept_id"]).Where("direction", "incoming").Where("lower(from_address)=?", strings.ToLower(r["target"].String())).Where("created_at>?", r["created_at"].Time()).Order("id DESC").Limit(20).All()
	if e != nil {
		return false, e
	}
	for _, m := range rows {
		body := m["body_text"].String()
		if body == "" {
			body = stripTags.ReplaceAllString(m["body_html"].String(), " ")
		}
		kind := hardReplyKind(jsonMap(m["source_headers"]), m["from_address"].String(), m["subject"].String(), newReplyText(body))
		if kind != "automated" {
			return true, nil
		}
	}
	return false, nil
}
