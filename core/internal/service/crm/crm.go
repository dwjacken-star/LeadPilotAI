package crm

import (
	"billionmail-core/internal/consts"
	dockerapi "billionmail-core/internal/service/dockerapi"
	"billionmail-core/internal/service/governance"
	"billionmail-core/internal/service/mail_service"
	"billionmail-core/internal/service/public"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/google/uuid"
	"net/mail"
	"strconv"
	"strings"
	"time"
)

func Mailboxes(ctx context.Context, p *governance.Principal) ([]string, error) {
	rows, err := g.DB().Model("mailbox").Ctx(ctx).Fields("username").Where("active", 1).Order("username").All()
	if err != nil {
		return nil, err
	}
	result := []string{}
	for _, row := range rows {
		sender := row["username"].String()
		if governance.SenderAllowed(ctx, p, sender) == nil {
			result = append(result, sender)
		}
	}
	return result, nil
}
func Customer(ctx context.Context, p *governance.Principal, id int64, permission string) (gdb.Record, error) {
	where, err := p.Predicate(ctx, permission, "")
	if err != nil {
		return nil, err
	}
	row, err := g.DB().Model("bm_crm_customers").Ctx(ctx).Where("id", id).WhereNull("deleted_at").Where(where).One()
	if err != nil {
		return nil, err
	}
	if len(row) == 0 {
		return nil, errors.New("CRM customer not found or access denied")
	}
	return row, nil
}
func List(ctx context.Context, p *governance.Principal, page int, keyword, stage string) (g.Map, error) {
	where, err := p.Predicate(ctx, "crm:list", "")
	if err != nil {
		return nil, err
	}
	model := g.DB().Model("bm_crm_customers").Ctx(ctx).WhereNull("deleted_at").Where(where)
	if keyword != "" {
		model = model.Where("(email ILIKE ? OR name ILIKE ? OR company ILIKE ?)", "%"+keyword+"%", "%"+keyword+"%", "%"+keyword+"%")
	}
	if stage != "" {
		model = model.Where("stage", stage)
	}
	total, err := model.Count()
	if err != nil {
		return nil, err
	}
	if page < 1 {
		page = 1
	}
	rows, err := model.Page(page, 20).Order("last_activity_at DESC,id DESC").All()
	return g.Map{"list": rows, "total": total}, err
}
func Save(ctx context.Context, p *governance.Principal, id int64, data map[string]interface{}) (int64, error) {
	if !p.Has("crm:manage") {
		return 0, errors.New("CRM management permission required")
	}
	email := strings.ToLower(strings.TrimSpace(fmt.Sprint(data["email"])))
	parsed, err := mail.ParseAddress(email)
	if err != nil || parsed.Address != email {
		return 0, errors.New("Invalid customer email")
	}
	stage, _ := data["stage"].(string)
	if !strings.Contains("|new|inquiry|quoted|negotiating|won|lost|", "|"+stage+"|") {
		return 0, errors.New("Invalid CRM stage")
	}
	values := g.Map{"email": email, "stage": stage}
	for _, key := range []string{"name", "company", "country", "note"} {
		value, _ := data[key].(string)
		if len(value) > 20000 {
			return 0, errors.New("Customer field is too long")
		}
		values[key] = value
	}
	values["next_followup_at"] = nil
	if value, _ := data["next_followup_at"].(string); value != "" {
		date, err := time.Parse(time.RFC3339, value)
		if err != nil {
			return 0, errors.New("Invalid follow-up date")
		}
		values["next_followup_at"] = date
	}
	if id > 0 {
		if _, err = Customer(ctx, p, id, "crm:manage"); err != nil {
			return 0, err
		}
		_, err = g.DB().Model("bm_crm_customers").Ctx(ctx).Where("id", id).Data(values).Update()
		return id, err
	}
	if err = p.CheckOwnership(ctx, "crm:manage", p.DeptID, p.ID); err != nil {
		return 0, err
	}
	values["dept_id"] = p.DeptID
	values["owner_id"] = p.ID
	// Explicitly adding the same customer restores the retained correspondence.
	previous, err := g.DB().Model("bm_crm_customers").Ctx(ctx).Unscoped().Where("dept_id", p.DeptID).Where("email", email).WhereNotNull("deleted_at").One()
	if err != nil {
		return 0, err
	}
	if len(previous) > 0 {
		where, e := p.Predicate(ctx, "crm:manage", "")
		if e != nil {
			return 0, e
		}
		values["deleted_at"] = nil
		values["deleted_by"] = nil
		result, e := g.DB().Model("bm_crm_customers").Ctx(ctx).Unscoped().Where("id", previous["id"]).Where(where).Data(values).Update()
		if e != nil {
			return 0, e
		}
		n, _ := result.RowsAffected()
		if n == 0 {
			return 0, errors.New("CRM customer not found or access denied")
		}
		return previous["id"].Int64(), nil
	}
	return g.DB().Model("bm_crm_customers").Ctx(ctx).Data(values).InsertAndGetId()
}

// Delete hides CRM records while preserving mailbox messages and reply history.
func Delete(ctx context.Context, p *governance.Principal, id int64) error {
	if !p.Has("crm:delete") {
		return errors.New("CRM deletion permission required")
	}
	customer, err := Customer(ctx, p, id, "crm:delete")
	if err != nil {
		return err
	}
	where, err := p.Predicate(ctx, "crm:delete", "")
	if err != nil {
		return err
	}
	return g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		result, e := tx.Model("bm_crm_customers").Where("id", id).WhereNull("deleted_at").Where(where).Data(g.Map{"deleted_at": time.Now(), "deleted_by": p.ID}).Update()
		if e != nil {
			return e
		}
		n, _ := result.RowsAffected()
		if n == 0 {
			return errors.New("CRM customer not found or access denied")
		}
		_, e = tx.Model("bm_security_audit").Data(g.Map{"account_id": p.ID, "dept_id": customer["dept_id"], "action": "crm:delete", "resource_id": id}).Insert()
		return e
	})
}

func doveadm(ctx context.Context, arguments ...string) ([]map[string]interface{}, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	client, err := dockerapi.NewDockerAPI()
	if err != nil {
		return nil, err
	}
	defer client.Close()
	result, err := client.ExecCommandByName(ctx, consts.SERVICES.Dovecot, append([]string{"doveadm", "-f", "json"}, arguments...), "root")
	if err != nil {
		return nil, errors.New("Mailbox connection failed")
	}
	if result.ExitCode != 0 {
		return nil, errors.New("Unable to read mailbox")
	}
	var rows []map[string]interface{}
	if json.Unmarshal([]byte(result.Output), &rows) != nil {
		return nil, errors.New("Invalid mailbox response")
	}
	return rows, nil
}
func text(row map[string]interface{}, key string) string {
	if row[key] == nil {
		return ""
	}
	return fmt.Sprint(row[key])
}
func Sync(ctx context.Context, p *governance.Principal, mailbox string) (int, error) {
	if !p.Has("crm:sync") {
		return 0, errors.New("CRM mailbox sync permission required")
	}
	if err := governance.SenderAllowed(ctx, p, mailbox); err != nil {
		return 0, err
	}
	status, err := doveadm(ctx, "mailbox", "status", "-u", mailbox, "uidnext uidvalidity", "INBOX")
	if err != nil || len(status) == 0 {
		return 0, errors.New("Mailbox status is unavailable")
	}
	next, _ := strconv.ParseInt(text(status[0], "uidnext"), 10, 64)
	validity := text(status[0], "uidvalidity")
	state, err := g.DB().Model("bm_crm_mailbox_sync").Ctx(ctx).Where("dept_id", p.DeptID).Where("mailbox", mailbox).One()
	if err != nil {
		return 0, err
	}
	first := next - 100
	if first < 1 {
		first = 1
	}
	if state["uidvalidity"].String() == validity && state["last_uid"].Int64() > 0 {
		first = state["last_uid"].Int64() + 1
	}
	if first >= next {
		return 0, nil
	}
	last := first + 99
	if last >= next {
		last = next - 1
	}
	rows, err := doveadm(ctx, "fetch", "-u", mailbox, "uid guid hdr.from hdr.to hdr.subject hdr.message-id hdr.in-reply-to hdr.references date.received body.snippet hdr.auto-submitted hdr.precedence hdr.x-auto-response-suppress hdr.x-maplemail-forward hdr.content-type", "mailbox", "INBOX", "uid", fmt.Sprintf("%d:%d", first, last))
	if err != nil {
		return 0, err
	}
	count := 0
	err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		for _, row := range rows {
			from, e := mail.ParseAddress(decodeHeader(text(row, "hdr.from")))
			if e != nil || strings.EqualFold(from.Address, mailbox) {
				continue
			}
			email := strings.ToLower(from.Address)
			customer, e := tx.GetValue(`INSERT INTO bm_crm_customers(dept_id,owner_id,email,name) VALUES($1,$2,$3,$4) ON CONFLICT(dept_id,email) DO UPDATE SET last_activity_at=now() WHERE bm_crm_customers.deleted_at IS NULL RETURNING id`, p.DeptID, p.ID, email, from.Name)
			if e != nil {
				return e
			}
			if customer == nil || customer.IsEmpty() {
				continue
			}
			values := g.Map{"customer_id": customer.Int64(), "dept_id": p.DeptID, "owner_id": p.ID, "mailbox": mailbox, "direction": "incoming", "source_uid": text(row, "uid"), "source_guid": text(row, "guid"), "message_id": text(row, "hdr.message-id"), "from_address": email, "to_address": mailbox, "subject": decodeHeader(text(row, "hdr.subject")), "in_reply_to": text(row, "hdr.in-reply-to"), "references_header": text(row, "hdr.references"), "body_text": text(row, "body.snippet"), "status": "received", "source_headers": headerJSON(row)}
			date, dateErr := time.Parse("2006-01-02 15:04:05 -0700", text(row, "date.received"))
			if dateErr != nil {
				date, dateErr = time.ParseInLocation("2006-01-02 15:04:05", text(row, "date.received"), time.Local)
			}
			if dateErr == nil {
				values["created_at"] = date
			}
			result, e := tx.Model("bm_crm_messages").Data(values).InsertIgnore()
			if e != nil {
				return e
			}
			added, _ := result.RowsAffected()
			count += int(added)
		}
		_, e := tx.Exec(`INSERT INTO bm_crm_mailbox_sync(dept_id,mailbox,uidvalidity,last_uid) VALUES($1,$2,$3,$4) ON CONFLICT(dept_id,mailbox) DO UPDATE SET uidvalidity=excluded.uidvalidity,last_uid=CASE WHEN bm_crm_mailbox_sync.uidvalidity=excluded.uidvalidity THEN GREATEST(bm_crm_mailbox_sync.last_uid,excluded.last_uid) ELSE excluded.last_uid END,updated_at=now()`, p.DeptID, mailbox, validity, last)
		return e
	})
	return count, err
}

func Detail(ctx context.Context, p *governance.Principal, id int64) (g.Map, error) {
	customer, err := Customer(ctx, p, id, "crm:list")
	if err != nil {
		return nil, err
	}
	allowed, err := Mailboxes(ctx, p)
	if err != nil {
		return nil, err
	}
	messages := gdb.Result{}
	if len(allowed) > 0 {
		messages, err = g.DB().Model("bm_crm_messages").Ctx(ctx).Fields("id,mailbox,direction,subject,from_address,to_address,status,error,created_at,message_id,body_text").Where("customer_id", id).WhereIn("mailbox", allowed).Order("created_at DESC,id DESC").Limit(100).All()
		if err != nil {
			return nil, err
		}
	}
	notes, err := g.DB().Model("bm_crm_followups").Ctx(ctx).Where("customer_id", id).Order("id DESC").Limit(100).All()
	return g.Map{"customer": customer, "messages": messages, "followups": notes}, err
}
func Message(ctx context.Context, p *governance.Principal, id int64) (gdb.Record, error) {
	row, err := g.DB().Model("bm_crm_messages").Ctx(ctx).Where("id", id).One()
	if err != nil {
		return nil, err
	}
	if len(row) == 0 {
		return nil, errors.New("Message not found")
	}
	if _, err = Customer(ctx, p, row["customer_id"].Int64(), "crm:list"); err != nil {
		return nil, err
	}
	if err = governance.SenderAllowed(ctx, p, row["mailbox"].String()); err != nil {
		return nil, err
	}
	if row["direction"].String() == "incoming" && !row["body_loaded"].Bool() {
		results, e := doveadm(ctx, "fetch", "-u", row["mailbox"].String(), "text", "mailbox", "INBOX", "guid", row["source_guid"].String())
		if e != nil {
			return nil, e
		}
		if len(results) == 0 {
			return row, nil
		}
		parsed, e := mail.ReadMessage(strings.NewReader(text(results[0], "text")))
		if e != nil {
			return nil, e
		}
		html, plain := parseBody(parsed.Header, parsed.Body, 0)
		_, err = g.DB().Model("bm_crm_messages").Ctx(ctx).Where("id", id).Data(g.Map{"body_html": html, "body_text": plain, "body_loaded": true, "source_headers": mailHeaderJSON(parsed.Header)}).Update()
		if err != nil {
			return nil, err
		}
		row["body_html"] = g.NewVar(html)
		row["body_text"] = g.NewVar(plain)
		row["body_loaded"] = g.NewVar(true)
		row["source_headers"] = g.NewVar(mailHeaderJSON(parsed.Header))
	}
	return row, nil
}
func Followup(ctx context.Context, p *governance.Principal, id int64, content string) error {
	customer, err := Customer(ctx, p, id, "crm:manage")
	if err != nil {
		return err
	}
	content = strings.TrimSpace(content)
	if content == "" || len(content) > 20000 {
		return errors.New("Invalid follow-up note")
	}
	_, err = g.DB().Model("bm_crm_followups").Ctx(ctx).Data(g.Map{"customer_id": id, "dept_id": customer["dept_id"], "owner_id": p.ID, "content": content}).Insert()
	return err
}

var newSender = mail_service.NewEmailSenderWithLocal

func Reply(ctx context.Context, p *governance.Principal, parentID int64, html, requestKey string) (int64, error) {
	if !p.Has("crm:reply") {
		return 0, errors.New("CRM reply permission required")
	}
	parent, err := Message(ctx, p, parentID)
	if err != nil {
		return 0, err
	}
	if parent["direction"].String() != "incoming" {
		return 0, errors.New("Choose an incoming message to reply")
	}
	if _, err = Customer(ctx, p, parent["customer_id"].Int64(), "crm:reply"); err != nil {
		return 0, err
	}
	if strings.TrimSpace(html) == "" || len(html) > 2*1024*1024 {
		return 0, errors.New("Reply content is empty or too large")
	}
	if _, err = uuid.Parse(requestKey); err != nil {
		return 0, errors.New("Invalid reply request key")
	}
	mailbox := parent["mailbox"].String()
	identity, identityErr := mail_service.OutboundIdentity(mailbox)
	if identityErr != nil {
		return 0, identityErr
	}
	recipient := parent["from_address"].String()
	if err = governance.SenderAllowed(ctx, p, mailbox); err != nil {
		return 0, err
	}
	subject := strings.ReplaceAll(strings.ReplaceAll(parent["subject"].String(), "\r", " "), "\n", " ")
	if !strings.HasPrefix(strings.ToLower(subject), "re:") {
		subject = "Re: " + subject
	}
	sender, err := newSender(mailbox)
	if err != nil {
		return 0, err
	}
	defer sender.Close()
	mid := sender.GenerateMessageID()
	id, err := g.DB().Model("bm_crm_messages").Ctx(ctx).Data(g.Map{"customer_id": parent["customer_id"], "dept_id": parent["dept_id"], "owner_id": p.ID, "mailbox": mailbox, "direction": "outgoing", "message_id": mid, "subject": subject, "from_address": identity.From, "to_address": recipient, "in_reply_to": parent["message_id"], "body_html": html, "status": "prepared", "request_key": requestKey}).InsertAndGetId()
	if err != nil {
		previous, e := g.DB().Model("bm_crm_messages").Ctx(ctx).Where("dept_id", parent["dept_id"]).Where("request_key", requestKey).One()
		if e == nil && len(previous) > 0 && previous["customer_id"].Int64() == parent["customer_id"].Int64() && previous["mailbox"].String() == mailbox {
			return previous["id"].Int64(), nil
		}
		return 0, err
	}
	fail := func(status string, e error) (int64, error) {
		_, _ = g.DB().Model("bm_crm_messages").Ctx(ctx).Where("id", id).Data(g.Map{"status": status, "error": e.Error()}).Update()
		return id, e
	}
	ip, _ := public.GetServerIP()
	limits, err := governance.Limits(ctx, parent["dept_id"].Int64(), mailbox, ip, public.GetMailProviderGroup(recipient), 0, governance.Policy{})
	if err != nil {
		return fail("failed", err)
	}
	release, err := governance.Reserve(ctx, limits, parent["dept_id"].Int64(), "crm:"+recipient, requestKey)
	if err != nil {
		return fail("failed", err)
	}
	defer release()
	if err = sender.Connect(); err != nil {
		return fail("failed", err)
	}
	message := mail_service.NewMessage(subject, html)
	message.MarkTransactional()
	message.SetMessageID(mid)
	reference := parent["message_id"].String()
	if reference != "" && !strings.ContainsAny(reference, "\r\n") && len(reference) < 1000 {
		message.SetHeader("In-Reply-To", reference)
		references := strings.Join(strings.Fields(parent["references_header"].String()), " ")
		if len(references) > 1000 {
			references = ""
		}
		message.SetHeader("References", strings.TrimSpace(references+" "+reference))
	}
	_, _ = g.DB().Model("bm_crm_messages").Ctx(ctx).Where("id", id).Data(g.Map{"status": "sending"}).Update()
	if err = sender.SendOnce(message, []string{recipient}); err != nil {
		return fail("unknown", err)
	}
	_, err = g.DB().Model("bm_crm_messages").Ctx(ctx).Where("id", id).Data(g.Map{"status": "sent"}).Update()
	_ = governance.Audit(ctx, p, "crm:reply", parent["customer_id"].Int64(), g.Map{"message_id": id})
	return id, err
}
