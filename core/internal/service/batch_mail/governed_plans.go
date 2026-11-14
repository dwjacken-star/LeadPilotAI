package batch_mail

import (
	"billionmail-core/internal/service/public"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
	_ "time/tzdata"

	"billionmail-core/internal/service/governance"
	"billionmail-core/internal/service/warmup"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

type ScheduleSpec struct {
	ID        int64          `json:"id"`
	Name      string         `json:"name"`
	Campaign  CampaignConfig `json:"campaign"`
	Cadence   string         `json:"cadence"`
	Timezone  string         `json:"timezone"`
	LocalTime string         `json:"local_time"`
	Weekdays  []int          `json:"weekdays"`
	MonthDay  int            `json:"month_day"`
	NextAt    string         `json:"next_at"`
	EndsAt    string         `json:"ends_at"`
	MaxRuns   int            `json:"max_runs"`
}

func (s ScheduleSpec) Validate() error {
	if s.Cadence != "once" && s.Cadence != "daily" && s.Cadence != "weekly" && s.Cadence != "monthly" {
		return errors.New("无效计划周期")
	}
	if _, e := time.LoadLocation(s.Timezone); e != nil {
		return errors.New("无效时区")
	}
	if _, e := time.Parse("15:04", s.LocalTime); e != nil {
		return errors.New("时间格式应为HH:mm")
	}
	if s.Cadence == "weekly" && len(s.Weekdays) == 0 {
		return errors.New("每周计划必须选择星期")
	}
	for _, d := range s.Weekdays {
		if d < 0 || d > 6 {
			return errors.New("星期无效")
		}
	}
	if s.Cadence == "monthly" && (s.MonthDay < 1 || s.MonthDay > 31) {
		return errors.New("每月日期无效")
	}
	if s.MaxRuns < 0 {
		return errors.New("执行次数不能为负")
	}
	return nil
}

// Next is based on the local calendar, not 24-hour durations, so DST transitions
// and month lengths do not drift the configured wall clock. Missing days clamp.
func (s ScheduleSpec) Next(after time.Time) (time.Time, error) {
	if e := s.Validate(); e != nil {
		return time.Time{}, e
	}
	if s.Cadence == "once" {
		return time.Time{}, nil
	}
	loc, _ := time.LoadLocation(s.Timezone)
	clock, _ := time.Parse("15:04", s.LocalTime)
	local := after.In(loc)
	for i := 0; i < 400; i++ {
		date := local.AddDate(0, 0, i)
		candidate := time.Date(date.Year(), date.Month(), date.Day(), clock.Hour(), clock.Minute(), 0, 0, loc)
		if candidate.Hour() != clock.Hour() || candidate.Minute() != clock.Minute() {
			continue
		}
		if !candidate.After(after) {
			continue
		}
		if s.Cadence == "weekly" {
			ok := false
			for _, d := range s.Weekdays {
				if int(date.Weekday()) == d {
					ok = true
				}
			}
			if !ok {
				continue
			}
		}
		if s.Cadence == "monthly" {
			day := s.MonthDay
			last := time.Date(date.Year(), date.Month()+1, 0, 0, 0, 0, 0, loc).Day()
			if day > last {
				day = last
			}
			if date.Day() != day {
				continue
			}
		}
		return candidate, nil
	}
	return time.Time{}, errors.New("无法计算下次执行时间")
}
func GetGovernedWarmup(ctx context.Context, id int64, ip, recipient string) (*governance.Deferred, error) {
	stat, err := warmup.WarmupCampaign().GetWarmupStatusForCampaign(ctx, id)
	if err != nil {
		return &governance.Deferred{time.Now().Add(time.Minute), "预热状态暂不可用"}, err
	}
	if stat == nil {
		return nil, nil
	}
	allow, seconds, err := warmup.RateLimiter().Allow(ctx, ip, recipient)
	if err != nil {
		return &governance.Deferred{time.Now().Add(time.Minute), "预热状态暂不可用"}, err
	}
	if !allow {
		if seconds < 1 {
			seconds = 1
		}
		return &governance.Deferred{time.Now().Add(time.Duration(seconds) * time.Second), "等待IP或服务商预热额度"}, nil
	}
	return nil, nil
}
func provider(email string) string { // same provider classification as existing sending code
	return public.GetMailProviderGroup(email)
}

func TickGovernedPlans(ctx context.Context) {
	for i := 0; i < 20; i++ {
		var sid int64
		err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
			r, e := tx.GetOne(`SELECT * FROM bm_campaign_schedules WHERE active AND next_run_at<=now() ORDER BY next_run_at LIMIT 1 FOR UPDATE SKIP LOCKED`)
			if e != nil || len(r) == 0 {
				return e
			}
			sid = r["id"].Int64()
			p, e := governance.Load(ctx, r["owner_id"].Int64())
			if e != nil {
				return e
			}
			if p.Super {
				p.DeptID = r["dept_id"].Int64()
			}
			if p.DeptID != r["dept_id"].Int64() || !p.Has("schedule:manage") {
				return errors.New("计划负责人权限已撤回")
			}
			var c CampaignConfig
			if e = json.Unmarshal(r["config"].Bytes(), &c); e != nil {
				return e
			}
			// Default misfire: execute at most one missed occurrence; no overlapping runs.
			v, e := tx.GetValue(`SELECT count(*) FROM email_tasks WHERE schedule_id=$1 AND task_process IN (0,1)`, sid)
			if e != nil {
				return e
			}
			count := r["run_count"].Int()
			if v.Int() == 0 && (r["ends_at"].IsEmpty() || r["ends_at"].Time().After(time.Now())) {
				task, e := CreateGovernedTask(ctx, tx, p, c, true, 0)
				if e != nil {
					return e
				}
				if _, e = tx.Exec(`UPDATE email_tasks SET schedule_id=$2 WHERE id=$1`, task, sid); e != nil {
					return e
				}
				if _, e = tx.Exec(`INSERT INTO bm_campaign_runs(schedule_id,due_at,task_id) VALUES($1,$2,$3)`, sid, r["next_run_at"], task); e != nil {
					return e
				}
				count++
			}
			spec := ScheduleSpec{Cadence: r["cadence"].String(), Timezone: r["timezone"].String(), LocalTime: r["local_time"].String(), MonthDay: r["month_day"].Int()}
			_ = json.Unmarshal(r["weekdays"].Bytes(), &spec.Weekdays)
			next, e := spec.Next(time.Now())
			if e != nil {
				return e
			}
			active := !next.IsZero()
			if r["max_runs"].Int() > 0 && count >= r["max_runs"].Int() {
				active = false
			}
			if !r["ends_at"].IsEmpty() && next.After(r["ends_at"].Time()) {
				active = false
			}
			if next.IsZero() {
				next = time.Now()
			}
			_, e = tx.Exec(`UPDATE bm_campaign_schedules SET next_run_at=$2,active=$3,run_count=$4,last_error='' WHERE id=$1`, sid, next, active, count)
			return e
		})
		if sid == 0 {
			break
		}
		if err != nil {
			_, _ = g.DB().Exec(ctx, `UPDATE bm_campaign_schedules SET active=false,last_error=$2 WHERE id=$1`, sid, err.Error())
			g.Log().Error(ctx, err)
		}
	}
	tickAutomations(ctx)
	bridgeAPI(ctx)
	_, _ = g.DB().Exec(ctx, `DELETE FROM bm_rate_events WHERE reserved_at<now()-interval '25 hours'`)
}

type AutomationStep struct {
	DelaySeconds int    `json:"delay_seconds"`
	TemplateID   int64  `json:"template_id"`
	Subject      string `json:"subject"`
	Condition    string `json:"condition"`
}
type AutomationConfig struct {
	Campaign CampaignConfig   `json:"campaign"`
	Steps    []AutomationStep `json:"steps"`
}

func (a AutomationConfig) Validate(ctx context.Context, p *governance.Principal) error {
	if len(a.Steps) < 1 || len(a.Steps) > 20 {
		return errors.New("自动化步骤需为1至20步")
	}
	for _, s := range a.Steps {
		if s.DelaySeconds < 0 || s.DelaySeconds > 31536000 {
			return errors.New("等待时间无效")
		}
		if s.Condition != "always" && s.Condition != "opened" && s.Condition != "clicked" && s.Condition != "not_opened" {
			return errors.New("步骤条件无效")
		}
		c := a.Campaign
		c.TemplateID = s.TemplateID
		c.Subject = s.Subject
		if err := ValidateCampaign(ctx, p, c); err != nil {
			return err
		}
	}
	return nil
}
func tickAutomations(ctx context.Context) {
	_, _ = g.DB().Exec(ctx, `UPDATE bm_automation_enrollments e SET state='completed' FROM email_tasks t WHERE e.state='finishing' AND t.id=e.last_task_id AND t.task_process=2`)
	// Enrollment is durable and unique per rule version. Confirmation (not merely
	// import) starts the flow. Existing subscribers are not silently enrolled.
	_, err := g.DB().Exec(ctx, `INSERT INTO bm_automation_enrollments(rule_id,contact_id,version,config,next_at) SELECT r.id,c.id,r.version,r.config,now()+((r.config->'steps'->0->>'delay_seconds')::int*interval '1 second') FROM bm_automation_rules r JOIN bm_contacts c ON c.group_id=r.group_id WHERE r.active AND c.active=1 AND c.status=1 AND c.confirmed_at>=r.activated_at ON CONFLICT DO NOTHING`)
	if err != nil {
		g.Log().Error(ctx, err)
		return
	}
	for i := 0; i < 40; i++ {
		found := false
		err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
			r, e := tx.GetOne(`SELECT e.*,r.owner_id,r.dept_id,r.active,c.active contact_active,c.status contact_status,c.email FROM bm_automation_enrollments e JOIN bm_automation_rules r ON r.id=e.rule_id JOIN bm_contacts c ON c.id=e.contact_id WHERE e.state='waiting' AND e.next_at<=now() ORDER BY e.next_at LIMIT 1 FOR UPDATE OF e SKIP LOCKED`)
			if e != nil || len(r) == 0 {
				return e
			}
			found = true
			stop := func(reason string) error {
				_, e := tx.Exec(`UPDATE bm_automation_enrollments SET state='stopped',last_error=$2 WHERE id=$1`, r["id"], reason)
				return e
			}
			if !r["active"].Bool() || r["contact_active"].Int() != 1 || r["contact_status"].Int() != 1 {
				return stop("规则停用或联系人已退订")
			}
			p, e := governance.Load(ctx, r["owner_id"].Int64())
			if e != nil {
				return stop(e.Error())
			}
			if p.Super {
				p.DeptID = r["dept_id"].Int64()
			}
			if p.DeptID != r["dept_id"].Int64() || !p.Has("automation:manage") {
				return stop("负责人权限已撤回")
			}
			var a AutomationConfig
			if e = json.Unmarshal(r["config"].Bytes(), &a); e != nil {
				return stop(e.Error())
			}
			step := r["step"].Int()
			if step >= len(a.Steps) {
				return stop("步骤配置无效")
			}
			s := a.Steps[step]
			if !r["last_task_id"].IsEmpty() {
				v, e := tx.GetOne(`SELECT task_process,pause FROM email_tasks WHERE id=$1`, r["last_task_id"])
				if e != nil {
					return e
				}
				if len(v) == 0 || v["task_process"].Int() == 4 {
					return stop("前序批次已取消或删除")
				}
				if v["task_process"].Int() != 2 {
					_, e = tx.Exec(`UPDATE bm_automation_enrollments SET next_at=now()+interval '1 minute' WHERE id=$1`, r["id"])
					return e
				}
				last, e := tx.GetValue(`SELECT max(sent_time) FROM recipient_info WHERE task_id=$1 AND delivery_state='accepted'`, r["last_task_id"])
				if e != nil {
					return e
				}
				if last.Int64() == 0 {
					return stop("前序邮件未成功提交，停止后续营销")
				}
				until := time.Unix(last.Int64(), 0).Add(time.Duration(s.DelaySeconds) * time.Second)
				if until.After(time.Now()) {
					_, e = tx.Exec(`UPDATE bm_automation_enrollments SET next_at=$2 WHERE id=$1`, r["id"], until)
					return e
				}
			}
			should := true
			if s.Condition != "always" {
				table := "mailstat_opened"
				if s.Condition == "clicked" {
					table = "mailstat_clicked"
				}
				v, e := tx.GetValue(fmt.Sprintf(`SELECT count(*) FROM %s WHERE campaign_id=$1 AND lower(recipient)=lower($2)`, table), r["last_task_id"].Int64(), r["email"])
				if e != nil {
					return e
				}
				should = v.Int() > 0
				if s.Condition == "not_opened" {
					should = !should
				}
			}
			task := r["last_task_id"].Int64()
			if should {
				c := a.Campaign
				c.TemplateID = s.TemplateID
				c.Subject = s.Subject
				task, e = CreateGovernedTask(ctx, tx, p, c, true, r["contact_id"].Int64())
				if e != nil {
					return stop(e.Error())
				}
				if _, e = tx.Exec(`UPDATE email_tasks SET automation_enrollment_id=$2 WHERE id=$1`, task, r["id"]); e != nil {
					return e
				}
			}
			step++
			state := "waiting"
			delay := 0
			if step >= len(a.Steps) {
				state = "finishing"
			} else {
				delay = a.Steps[step].DelaySeconds
			}
			_, e = tx.Exec(`UPDATE bm_automation_enrollments SET step=$2,state=$3,next_at=now()+($4*interval '1 second'),last_task_id=$5,last_error='' WHERE id=$1`, r["id"], step, state, delay, task)
			return e
		})
		if err != nil {
			g.Log().Error(ctx, err)
			break
		}
		if !found {
			break
		}
	}
}
func bridgeAPI(ctx context.Context) {
	for i := 0; i < 50; i++ {
		found := false
		err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
			log, e := tx.GetOne(`SELECT l.*,a.owner_id,a.dept_id,a.template_id,a.group_id,a.subject,a.full_name,a.active,a.expire_time,a.last_key_update_time,a.addresser configured_sender,a.senders authorized_senders,a.cc_email configured_cc,a.track_open configured_track_open,a.track_click configured_track_click FROM api_mail_logs l JOIN api_templates a ON a.id=l.api_id WHERE l.status=0 ORDER BY l.id LIMIT 1 FOR UPDATE OF l SKIP LOCKED`)
			if e != nil || len(log) == 0 {
				return e
			}
			found = true
			fail := func(reason string) error {
				_, e := tx.Exec(`UPDATE api_mail_logs SET status=3,error_message=$2 WHERE id=$1`, log["id"], reason)
				return e
			}
			if log["active"].Int() != 1 {
				return fail("API已停用")
			}
			if expiry := log["expire_time"].Int64(); expiry > 0 && time.Now().Unix() > log["last_key_update_time"].Int64()+expiry {
				return fail("API Key已过期")
			}
			p, e := governance.Load(ctx, log["owner_id"].Int64())
			if e != nil {
				return fail(e.Error())
			}
			if p.DeptID != log["dept_id"].Int64() {
				if p.Super {
					p.DeptID = log["dept_id"].Int64()
				} else {
					return fail("API部门归属不匹配")
				}
			}
			if !PoolContains(DecodeSenders(log["authorized_senders"].String()), log["configured_sender"].String(), log["addresser"].String()) {
				return fail("不允许覆盖API配置的发件邮箱")
			}
			group := log["group_id"].Int64()
			if group == 0 {
				v, e := tx.GetValue(`SELECT group_id FROM bm_contacts WHERE lower(email)=lower($1) AND dept_id=$2 ORDER BY id DESC LIMIT 1`, log["recipient"], p.DeptID)
				if e != nil {
					return e
				}
				group = v.Int64()
			}
			v, e := tx.GetOne(`SELECT * FROM bm_contacts WHERE group_id=$1 AND lower(email)=lower($2) ORDER BY id LIMIT 1`, group, log["recipient"])
			if e != nil {
				return e
			}
			if len(v) == 0 {
				return fail("联系人不存在")
			}
			if v["active"].Int() != 1 || v["status"].Int() != 1 {
				return fail("联系人已退订或尚未确认订阅")
			}
			config := CampaignConfig{TrackOpen: log["configured_track_open"].Int() == 1, TrackClick: log["configured_track_click"].Int() == 1, CCEmail: log["configured_cc"].String(), Name: "API发送", GroupID: group, TemplateID: log["template_id"].Int64(), Sender: log["addresser"].String(), Subject: log["subject"].String(), FullName: log["full_name"].String()}
			task, e := CreateGovernedTask(ctx, tx, p, config, true, v["id"].Int64())
			if e != nil {
				return fail(e.Error())
			}
			snapshot := v.Map()
			var attrs map[string]interface{}
			_ = json.Unmarshal(log["attribs"].Bytes(), &attrs)
			snapshot["attribs"] = attrs
			b, _ := json.Marshal(snapshot)
			_, e = tx.Exec(`UPDATE recipient_info SET api_log_id=$2,contact_snapshot=$3::jsonb WHERE task_id=$1`, task, log["id"], string(b))
			if e != nil {
				return e
			}
			_, e = tx.Exec(`UPDATE api_mail_logs SET status=1,error_message='已加入统一发送队列' WHERE id=$1`, log["id"])
			return e
		})
		if err != nil {
			g.Log().Error(ctx, err)
			break
		}
		if !found {
			break
		}
	}
	_, _ = g.DB().Exec(ctx, `UPDATE api_mail_logs l SET status=CASE WHEN r.delivery_state='accepted' THEN 2 ELSE 3 END,message_id=r.message_id,error_message=r.last_error FROM recipient_info r WHERE r.api_log_id=l.id AND l.status=1 AND r.delivery_state IN ('accepted','failed','skipped','unknown')`)
}
