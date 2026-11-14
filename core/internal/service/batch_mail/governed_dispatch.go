package batch_mail

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/textproto"
	"strings"
	"sync/atomic"
	"time"

	"billionmail-core/internal/model/entity"
	"billionmail-core/internal/service/deliveryprefs"
	"billionmail-core/internal/service/domains"
	"billionmail-core/internal/service/governance"
	"billionmail-core/internal/service/mail_service"
	"billionmail-core/internal/service/maillog_stat"
	"billionmail-core/internal/service/outreachguard"
	"billionmail-core/internal/service/public"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/google/uuid"
)

type CampaignConfig struct {
	AuthoringConfig *entity.TaskAuthoring `json:"authoring_config,omitempty"`
	HTMLContent     *string               `json:"html_content,omitempty"`
	Senders         []string              `json:"senders"`
	GroupIDs        []int64               `json:"group_ids"`
	CCEmail         string                `json:"cc_email"`
	DeptID          int64                 `json:"dept_id"`
	Name            string                `json:"name"`
	GroupID         int64                 `json:"group_id"`
	TemplateID      int64                 `json:"template_id"`
	Sender          string                `json:"sender"`
	Subject         string                `json:"subject"`
	FullName        string                `json:"full_name"`
	StartAt         int64                 `json:"start_at"`
	TagIDs          []int64               `json:"tag_ids"`
	TagLogic        string                `json:"tag_logic"`
	Policy          governance.Policy     `json:"policy"`
	TrackOpen       bool                  `json:"track_open"`
	TrackClick      bool                  `json:"track_click"`
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func ValidateCampaign(ctx context.Context, p *governance.Principal, c CampaignConfig) error {
	if e := ValidateAuthoring(c.AuthoringConfig); e != nil {
		return e
	}
	if !p.Has("campaign:create") {
		return errors.New("缺少创建营销任务权限")
	}
	if err := p.CheckOwnership(ctx, "campaign:create", p.DeptID, p.ID); err != nil {
		return err
	}
	if strings.TrimSpace(c.Subject) == "" || strings.ContainsAny(c.Subject+c.FullName, "\r\n") {
		return errors.New("邮件主题不能为空，且邮件头不能包含换行")
	}
	pool, err := NormalizeSenders(c.Senders, c.Sender)
	if err != nil {
		return err
	}
	if err := ValidCC(c.CCEmail); err != nil {
		return err
	}
	if err := c.Policy.Validate(); err != nil {
		return err
	}
	groups := NormalizeGroups(c.GroupIDs, c.GroupID)
	if len(groups) == 0 {
		return errors.New("Select at least one recipient group")
	}
	for _, id := range groups {
		if err = p.Check(ctx, "bm_contact_groups", id, "contact:list"); err != nil {
			return err
		}
	}
	if _, err = ResolveCampaignContent(ctx, p, c.TemplateID, c.HTMLContent); err != nil {
		return err
	}
	if err = CheckSenderPool(ctx, p, pool); err != nil {
		return err
	}
	for _, id := range c.TagIDs {
		if err = p.Check(ctx, "bm_tags", id, "contact:list"); err != nil {
			return err
		}
		n, e := g.DB().Model("bm_tags").Where("id", id).WhereIn("group_id", groups).Count()
		if e != nil {
			return e
		}
		if n == 0 {
			return errors.New("Tag is outside the selected groups")
		}
	}
	if c.TagLogic != "" && c.TagLogic != "AND" && c.TagLogic != "OR" && c.TagLogic != "NOT" {
		return errors.New("标签逻辑必须为AND、OR或NOT")
	}
	return nil
}

// CreateGovernedTask snapshots content and recipients in the same transaction as
// the run record. singleContact is used for one subscriber's automation step.
func CreateGovernedTask(ctx context.Context, tx gdb.TX, p *governance.Principal, c CampaignConfig, publish bool, singleContact int64) (int64, error) {
	if err := ValidateCampaign(ctx, p, c); err != nil {
		return 0, err
	}
	if publish && !p.Has("campaign:publish") {
		return 0, errors.New("缺少发布营销任务权限")
	}
	if publish {
		if err := p.CheckOwnership(ctx, "campaign:publish", p.DeptID, p.ID); err != nil {
			return 0, err
		}
	}
	content, err := ResolveCampaignContent(ctx, p, c.TemplateID, c.HTMLContent)
	if err != nil {
		return 0, err
	}
	b, _ := json.Marshal(c.Policy)
	pause := 1
	if publish {
		pause = 0
	}
	start := c.StartAt
	if start == 0 {
		start = time.Now().Unix()
	}
	name := c.Name
	if name == "" {
		name = c.Subject
	}
	pool, _ := NormalizeSenders(c.Senders, c.Sender)
	groups := NormalizeGroups(c.GroupIDs, c.GroupID)
	poolJSON, _ := json.Marshal(pool)
	groupJSON, _ := json.Marshal(groups)
	tagJSON, _ := json.Marshal(c.TagIDs)
	authoringJSON, _ := json.Marshal(c.AuthoringConfig)
	if c.AuthoringConfig == nil {
		authoringJSON = []byte(`{}`)
	}
	id, err := tx.Model("email_tasks").Data(g.Map{"authoring_config": string(authoringJSON), "tag_ids": string(tagJSON), "tag_logic": c.TagLogic, "task_name": name, "addresser": pool[0], "senders": string(poolJSON), "group_id": groups[0], "group_ids": string(groupJSON), "cc_email": c.CCEmail, "subject": c.Subject, "full_name": c.FullName, "template_id": c.TemplateID, "dept_id": p.DeptID, "owner_id": p.ID, "recipient_count": 0, "threads": 1, "unsubscribe": 1, "track_open": boolInt(c.TrackOpen && c.CCEmail == ""), "track_click": boolInt(c.TrackClick && c.CCEmail == ""), "start_time": start, "pause": pause, "active": 1, "snapshot_content": content, "send_policy": string(b)}).InsertAndGetId()
	if err != nil {
		return 0, err
	}
	where, err := campaignAudience(c)
	if err != nil {
		return 0, err
	}
	if singleContact > 0 {
		where += fmt.Sprintf(" AND c.id=%d", singleContact)
	}
	_, err = tx.Exec(`WITH audience AS(SELECT DISTINCT ON(lower(trim(c.email))) c.* FROM bm_contacts c WHERE `+where+` ORDER BY lower(trim(c.email)),c.id), ranked AS(SELECT *,row_number() OVER(ORDER BY lower(trim(email))) seq FROM audience) INSERT INTO recipient_info(task_id,recipient,message_id,contact_snapshot,assigned_sender,recipient_group_ids) SELECT $1,lower(trim(r.email)),'',to_jsonb(r),($2::jsonb)->>((seq-1)%jsonb_array_length($2::jsonb))::int,$3::jsonb FROM ranked r ON CONFLICT DO NOTHING`, id, string(poolJSON), string(groupJSON))
	if err != nil {
		return 0, err
	}
	_, err = tx.Exec(`UPDATE email_tasks SET recipient_count=(SELECT count(*) FROM recipient_info WHERE task_id=$1) WHERE id=$1`, id)
	return id, err
}

var dispatching atomic.Bool
var dispatchWorkers = make(chan struct{}, 32)
var newGovernedSender = mail_service.NewEmailSenderWithLocal

type governedJob struct {
	Record gdb.Record
	Lease  string
}

func DispatchGoverned(ctx context.Context) {
	if !dispatching.CompareAndSwap(false, true) {
		return
	}
	defer dispatching.Store(false)
	defer func() {
		_, err := g.DB().Exec(ctx, `UPDATE email_tasks t SET task_process=2 WHERE t.task_process IN (0,1) AND t.pause=0 AND t.start_time<=extract(epoch from now()) AND NOT EXISTS(SELECT 1 FROM recipient_info r WHERE r.task_id=t.id AND r.delivery_state IN ('pending','retry','claimed','sending','unknown','validation_hold'))`)
		if err != nil {
			g.Log().Error(ctx, err)
		}
	}()
	// A claimed row is safe to retry; a row which entered SMTP may already have
	// been accepted and requires reconciliation, never an automatic blind resend.
	_, err := g.DB().Exec(ctx, `UPDATE recipient_info SET delivery_state=CASE WHEN delivery_state='sending' THEN 'unknown' ELSE 'retry' END,is_sent=CASE WHEN delivery_state='sending' THEN 3 ELSE 0 END,last_error='工作进程租约过期，请核对发送结果',lease_token=NULL WHERE delivery_state IN ('claimed','sending') AND lease_until<now()`)
	if err != nil {
		g.Log().Error(ctx, err)
		return
	}
	for i := 0; i < 8; i++ {
		select {
		case dispatchWorkers <- struct{}{}:
		default:
			return
		}
		job, err := claimGoverned(ctx)
		if err != nil || job == nil {
			<-dispatchWorkers
			if err != nil {
				g.Log().Error(ctx, err)
			}
			return
		}
		go func() { defer func() { <-dispatchWorkers }(); processGoverned(context.Background(), job) }()
	}
	_, err = g.DB().Exec(ctx, `UPDATE email_tasks t SET task_process=2 WHERE t.task_process IN (0,1) AND t.pause=0 AND t.start_time<=extract(epoch from now()) AND NOT EXISTS(SELECT 1 FROM recipient_info r WHERE r.task_id=t.id AND r.delivery_state IN ('pending','retry','claimed','sending','unknown','validation_hold'))`)
	if err != nil {
		g.Log().Error(ctx, err)
	}
}
func claimGoverned(ctx context.Context) (job *governedJob, err error) {
	err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if _, e := tx.Exec(`SELECT pg_advisory_xact_lock(614806)`); e != nil {
			return e
		}
		row, e := tx.GetOne(`SELECT t.*,r.id AS delivery_id,r.recipient,r.contact_snapshot,r.attempts,r.assigned_sender,r.recipient_group_ids,r.outreach_message_id,r.master_content_id FROM email_tasks t JOIN bm_departments d ON d.id=t.dept_id JOIN LATERAL(SELECT * FROM recipient_info WHERE task_id=t.id AND delivery_state IN ('pending','retry') AND next_attempt_at<=now() ORDER BY id LIMIT 1 FOR UPDATE SKIP LOCKED) r ON true WHERE t.task_process IN (0,1) AND t.pause=0 AND t.start_time<=extract(epoch from now()) AND t.next_attempt_at<=now() ORDER BY d.last_dispatch_at NULLS FIRST,t.priority DESC,t.last_dispatch_at NULLS FIRST,t.id LIMIT 1 FOR UPDATE OF t SKIP LOCKED`)
		if e != nil || len(row) == 0 {
			return e
		}
		lease := uuid.NewString()
		_, e = tx.Exec(`UPDATE recipient_info SET delivery_state='claimed',is_sent=2,lease_token=$2,lease_until=now()+interval '5 minutes' WHERE id=$1`, row["delivery_id"], lease)
		if e != nil {
			return e
		}
		_, e = tx.Exec(`UPDATE email_tasks SET task_process=1,last_dispatch_at=now(),snapshot_content=COALESCE(snapshot_content,(SELECT content FROM email_templates WHERE id=email_tasks.template_id)) WHERE id=$1`, row["id"])
		if e != nil {
			return e
		}
		_, e = tx.Exec(`UPDATE bm_departments SET last_dispatch_at=now() WHERE id=$1`, row["dept_id"])
		job = &governedJob{row, lease}
		return e
	})
	return
}
func finishGoverned(ctx context.Context, j *governedJob, state, reason, message string, next time.Time) {
	sent := 0
	if state == "accepted" {
		sent = 1
	}
	if state == "failed" || state == "skipped" || state == "unknown" {
		sent = 3
	}
	result, err := g.DB().Exec(ctx, `UPDATE recipient_info SET delivery_state=$3,is_sent=$4,last_error=$5,message_id=CASE WHEN $6='' THEN message_id ELSE $6 END,next_attempt_at=$7,sent_time=CASE WHEN $3='accepted' THEN extract(epoch from now())::int ELSE sent_time END,lease_token=NULL,lease_until=NULL WHERE id=$1 AND lease_token=$2`, j.Record["delivery_id"], j.Lease, state, sent, reason, message, next)
	if err != nil {
		g.Log().Error(ctx, "保存发送结果失败", err)
	}
	if err == nil {
		affected, _ := result.RowsAffected()
		if affected > 0 {
			outreachguard.Finish(ctx, j.Record["outreach_message_id"].Int64(), state, reason)
		}
	}
}
func deferGoverned(ctx context.Context, j *governedJob, d *governance.Deferred) {
	finishGoverned(ctx, j, "retry", d.Reason, "", d.Until)
	// One contact/provider being delayed must not block other eligible recipients.
	if d.Reason == "等待收件人触达间隔" || strings.Contains(d.Reason, "provider:") || d.Reason == "等待IP或服务商预热额度" {
		return
	}
	_, err := g.DB().Exec(ctx, `UPDATE email_tasks SET wait_reason=$2,next_attempt_at=$3 WHERE id=$1`, j.Record["id"], d.Reason, d.Until)
	if err != nil {
		g.Log().Error(ctx, err)
	}
}
func processGoverned(ctx context.Context, j *governedJob) {
	row := j.Record
	if value := row["assigned_sender"].String(); value != "" {
		row["addresser"] = g.NewVar(value)
	}
	var snapshot entity.Contact
	_ = json.Unmarshal(row["contact_snapshot"].Bytes(), &snapshot)
	if snapshot.GroupId > 0 {
		row["group_id"] = g.NewVar(snapshot.GroupId)
	}
	taskID := row["id"].Int64()
	now := time.Now()
	p, err := governance.Load(ctx, row["owner_id"].Int64())
	if err == nil && p.Super {
		p.DeptID = row["dept_id"].Int64()
		n, e := g.DB().Model("bm_departments").Where("id", p.DeptID).Where("active", true).Count()
		if e != nil || n == 0 {
			err = errors.New("任务所属部门停用")
		}
	}
	if err != nil || p.DeptID != row["dept_id"].Int64() {
		deferGoverned(ctx, j, &governance.Deferred{now.Add(time.Hour), "负责人或部门不可用，需要重新授权"})
		return
	}
	if !p.Has("campaign:publish") {
		deferGoverned(ctx, j, &governance.Deferred{now.Add(time.Hour), "负责人发布权限已撤回"})
		return
	}
	if e := p.Check(ctx, "email_tasks", taskID, "campaign:publish"); e != nil {
		deferGoverned(ctx, j, &governance.Deferred{now.Add(time.Hour), e.Error()})
		return
	}
	if err = governance.SenderAllowed(ctx, p, row["addresser"].String()); err != nil {
		deferGoverned(ctx, j, &governance.Deferred{now.Add(time.Hour), err.Error()})
		return
	}
	live, e := g.DB().GetOne(ctx, `SELECT t.*,r.delivery_state FROM email_tasks t JOIN recipient_info r ON r.task_id=t.id WHERE t.id=$1 AND r.id=$2 AND r.lease_token=$3`, taskID, row["delivery_id"], j.Lease)
	if e != nil || len(live) == 0 {
		return
	}
	if live["pause"].Int() != 0 || live["task_process"].Int() > 1 {
		finishGoverned(ctx, j, "retry", "任务已暂停或取消", "", now)
		return
	}
	if live["automation_enrollment_id"].Int64() > 0 {
		n, e := g.DB().GetValue(ctx, `SELECT count(*) FROM bm_automation_enrollments e JOIN bm_automation_rules r ON r.id=e.rule_id WHERE e.id=$1 AND r.active`, live["automation_enrollment_id"])
		if e != nil || n.Int() == 0 {
			finishGoverned(ctx, j, "skipped", "自动化规则已停用", "", now)
			return
		}
	}
	var contact entity.Contact
	_ = json.Unmarshal(row["contact_snapshot"].Bytes(), &contact)
	n, err := g.DB().Model("bm_contacts").Where("group_id", row["group_id"]).Where("lower(trim(email)) = ?", row["recipient"]).Where("active", 1).Where("status", 1).Count()
	if err != nil {
		deferGoverned(ctx, j, &governance.Deferred{now.Add(time.Minute), "检查订阅状态失败"})
		return
	}
	if n == 0 {
		finishGoverned(ctx, j, "skipped", "已退订、删除或尚未确认", "", now)
		return
	}
	blocked, e := g.DB().GetValue(ctx, `SELECT (SELECT count(*) FROM abnormal_recipient WHERE lower(recipient)=lower($1))+(SELECT count(*) FROM mailstat_complaints WHERE lower(recipient)=lower($1))`, row["recipient"])
	if e != nil {
		deferGoverned(ctx, j, &governance.Deferred{now.Add(time.Minute), "检查禁止发送名单失败"})
		return
	}
	if blocked.Int() > 0 {
		finishGoverned(ctx, j, "skipped", "收件地址在异常或投诉名单中", "", now)
		return
	}
	if contact.Email == "" {
		_ = g.DB().Model("bm_contacts").Where("group_id", row["group_id"]).Where("lower(trim(email)) = ?", row["recipient"]).Scan(&contact)
	}
	policy := governance.DecodePolicy(live["send_policy"].String())
	identity, identityErr := mail_service.OutboundIdentity(row["addresser"].String())
	if identityErr != nil {
		finishGoverned(ctx, j, "failed", identityErr.Error(), "", now)
		return
	}
	suppressed, suppressionErr := deliveryprefs.IsSuppressed(ctx, identity.From, row["recipient"].String())
	if suppressionErr != nil {
		deferGoverned(ctx, j, &governance.Deferred{now.Add(time.Minute), "退订状态检查暂不可用"})
		return
	}
	if suppressed {
		finishGoverned(ctx, j, "skipped", "收件人已退订，停止发送", "", now)
		return
	}
	serverIP, _ := public.GetServerIP()
	// Preserve the existing IP/provider warmup ceiling, in addition to new quotas.
	if stat, _ := GetGovernedWarmup(ctx, taskID, serverIP, row["recipient"].String()); stat != nil {
		deferGoverned(ctx, j, stat)
		return
	}
	limits, err := governance.Limits(ctx, p.DeptID, row["addresser"].String(), serverIP, public.GetMailProviderGroup(row["recipient"].String()), taskID, policy)
	if err == nil && row["outreach_message_id"].Int64() > 0 {
		var limit governance.Limit
		limit, err = outreachguard.Limit(ctx, row["outreach_message_id"].Int64())
		if err == nil {
			limits = append(limits, limit)
		}
	}
	if err != nil {
		deferGoverned(ctx, j, &governance.Deferred{now.Add(time.Minute), "读取发送策略失败"})
		return
	}
	sender, err := newGovernedSender(row["addresser"].String())
	if err != nil {
		finishGoverned(ctx, j, "failed", err.Error(), "", now)
		return
	}
	defer sender.Close()
	if err = sender.Connect(); err != nil {
		attempts := row["attempts"].Int() + 1
		_, _ = g.DB().Exec(ctx, `UPDATE recipient_info SET attempts=$2 WHERE id=$1 AND lease_token=$3`, row["delivery_id"], attempts, j.Lease)
		if attempts >= 5 {
			finishGoverned(ctx, j, "failed", "SMTP连接失败："+err.Error(), "", now)
		} else {
			finishGoverned(ctx, j, "retry", "SMTP连接失败："+err.Error(), "", now.Add(time.Duration(1<<uint(attempts))*time.Minute))
		}
		return
	}
	unsubscribe := ""
	if live["unsubscribe"].Int() == 1 {
		token, tokenErr := GenerateUnsubscribeJWT(row["recipient"].String(), row["template_id"].Int(), int(taskID), row["group_id"].Int())
		if tokenErr != nil {
			finishGoverned(ctx, j, "failed", "生成退订链接失败", "", now)
			return
		}
		unsubscribe = fmt.Sprintf("%s/unsubscribe_new.html?jwt=%s", domains.GetBaseURL(), token)
	}
	var task entity.EmailTask
	_ = g.DB().Model("email_tasks").Where("id", taskID).Scan(&task)
	task.Addresser = row["addresser"].String()
	task.GroupId = row["group_id"].Int()
	content, err := GetTemplateEngine().RenderEmailTemplate(ctx, live["snapshot_content"].String(), &contact, &task, unsubscribe)
	if err != nil {
		finishGoverned(ctx, j, "failed", "模板渲染失败："+err.Error(), "", now)
		return
	}
	subject, err := GetTemplateEngine().RenderEmailTemplate(ctx, task.Subject, &contact, &task, unsubscribe)
	if err != nil {
		finishGoverned(ctx, j, "failed", err.Error(), "", now)
		return
	}
	if row["outreach_message_id"].Int64() > 0 {
		subject, content, err = outreachguard.Content(ctx, p, row["outreach_message_id"].Int64())
		if err != nil {
			finishGoverned(ctx, j, "skipped", err.Error(), "", now)
			return
		}
	}
	mid := sender.GenerateMessageID()
	tracker := maillog_stat.NewMailTracker(content, int(taskID), mid, row["recipient"].String(), domains.GetBaseURL())
	if task.TrackOpen == 1 {
		tracker.AppendTrackingPixel()
	}
	if task.TrackClick == 1 {
		tracker.TrackLinks()
	}
	content = tracker.GetHTML()
	if task.Unsubscribe == 1 {
		_, subscribe, linkErr := DeliveryLinks(ctx, row["recipient"].String(), row["template_id"].Int(), int(taskID), row["group_id"].Int())
		if linkErr != nil {
			finishGoverned(ctx, j, "failed", linkErr.Error(), "", now)
			return
		}
		content = AppendDeliveryLinks(content, unsubscribe, subscribe)
	}
	message := mail_service.NewMessage(subject, content)
	message.SetMessageID(mid)
	message.SetRealName(task.FullName)
	if task.Unsubscribe == 1 {
		message.SetHeader("List-Unsubscribe", "<"+unsubscribe+">")
	}
	currentPrincipal, authErr := governance.Load(ctx, p.ID)
	if authErr != nil || !currentPrincipal.Has("campaign:publish") {
		deferGoverned(ctx, j, &governance.Deferred{time.Now().Add(time.Hour), "发送权限已撤回"})
		return
	}
	if currentPrincipal.Super {
		currentPrincipal.DeptID = p.DeptID
	}
	if currentPrincipal.DeptID != p.DeptID {
		deferGoverned(ctx, j, &governance.Deferred{time.Now().Add(time.Hour), "负责人部门已变更"})
		return
	}
	if authErr = currentPrincipal.Check(ctx, "email_tasks", taskID, "campaign:publish"); authErr != nil {
		deferGoverned(ctx, j, &governance.Deferred{time.Now().Add(time.Hour), authErr.Error()})
		return
	}
	if authErr = governance.SenderAllowed(ctx, currentPrincipal, task.Addresser); authErr != nil {
		deferGoverned(ctx, j, &governance.Deferred{time.Now().Add(time.Hour), authErr.Error()})
		return
	}
	if row["outreach_message_id"].Int64() > 0 {
		if _, _, authErr = outreachguard.Content(ctx, currentPrincipal, row["outreach_message_id"].Int64()); authErr != nil {
			finishGoverned(ctx, j, "skipped", authErr.Error(), "", now)
			return
		}
	}
	if authErr = checkMasterDelivery(ctx, currentPrincipal, row["master_content_id"].Int64()); authErr != nil {
		finishGoverned(ctx, j, "skipped", authErr.Error(), "", now)
		return
	}
	release, err := governance.Reserve(ctx, limits, p.DeptID, row["recipient"].String(), j.Lease)
	if err != nil {
		var d *governance.Deferred
		if !errors.As(err, &d) {
			d = &governance.Deferred{now.Add(time.Minute), "共享额度暂不可用"}
		}
		deferGoverned(ctx, j, d)
		return
	}
	defer release()
	// Fence the last DB transition against cancellation and stale workers.
	result, err := g.DB().Exec(ctx, `UPDATE recipient_info r SET delivery_state='sending',attempts=attempts+1,message_id=$3 WHERE r.id=$1 AND r.lease_token=$2 AND EXISTS(SELECT 1 FROM email_tasks t WHERE t.id=r.task_id AND t.pause=0 AND t.task_process IN (0,1)) AND EXISTS(SELECT 1 FROM bm_contacts c WHERE c.group_id=$4 AND lower(trim(c.email))=lower($5) AND c.active=1 AND c.status=1)`, row["delivery_id"], j.Lease, mid, row["group_id"], row["recipient"])
	if err != nil {
		return
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		finishGoverned(ctx, j, "retry", "任务已暂停", "", now)
		return
	}
	_, err = g.DB().Exec(ctx, `INSERT INTO bm_send_attempts(delivery_id,task_id,dept_id,sender,recipient,message_id,outcome) VALUES($1,$2,$3,$4,$5,$6,'sending')`, row["delivery_id"], taskID, p.DeptID, task.Addresser, row["recipient"], mid)
	if err != nil {
		finishGoverned(ctx, j, "failed", "无法持久化发送尝试", "", now)
		return
	}
	recipients := []string{row["recipient"].String()}
	cc := live["cc_email"].String()
	if cc != "" && !strings.EqualFold(cc, recipients[0]) {
		message.SetHeader("To", recipients[0])
		message.SetHeader("Cc", cc)
		recipients = append(recipients, cc)
	}
	err = sender.SendOnce(message, recipients)
	state, reason := "accepted", ""
	next := now
	if err != nil {
		reason = err.Error()
		state = "unknown"
		var smtpError *textproto.Error
		if errors.As(err, &smtpError) {
			state = "failed"
			if smtpError.Code >= 400 && smtpError.Code < 500 && row["attempts"].Int() < 4 {
				state = "retry"
				next = now.Add(time.Duration(1<<uint(row["attempts"].Int())) * time.Minute)
			}
		}
	}
	finishGoverned(ctx, j, state, reason, mid, next)
	_, e = g.DB().Exec(ctx, `UPDATE bm_send_attempts SET outcome=$2,error=$3 WHERE message_id=$1`, mid, state, reason)
	if e != nil {
		g.Log().Error(ctx, e)
	}
}
