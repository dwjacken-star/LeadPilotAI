package governance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

type Policy struct {
	PerMinute              int    `json:"per_minute"`
	PerHour                int    `json:"per_hour"`
	PerDay                 int    `json:"per_day"`
	MinIntervalMS          int    `json:"min_interval_ms"`
	Concurrency            int    `json:"concurrency"`
	RecipientCooldownHours int    `json:"recipient_cooldown_hours"`
	BatchSize              int    `json:"batch_size"`
	BatchRestSeconds       int    `json:"batch_rest_seconds"`
	Timezone               string `json:"timezone"`
	WindowStart            string `json:"window_start"`
	WindowEnd              string `json:"window_end"`
	Weekdays               []int  `json:"weekdays"`
}

func DecodePolicy(s string) Policy { var p Policy; _ = json.Unmarshal([]byte(s), &p); return p }
func (p Policy) Validate() error {
	for _, n := range []int{p.PerMinute, p.PerHour, p.PerDay, p.MinIntervalMS, p.Concurrency, p.RecipientCooldownHours, p.BatchSize, p.BatchRestSeconds} {
		if n < 0 {
			return errors.New("发送策略不能为负数")
		}
	}
	if p.Concurrency > 100 || p.MinIntervalMS > 86400000 || p.RecipientCooldownHours > 8760 || p.BatchRestSeconds > 2592000 {
		return errors.New("发送策略超出允许范围")
	}
	if p.Timezone != "" {
		if _, err := time.LoadLocation(p.Timezone); err != nil {
			return errors.New("无效时区")
		}
	}
	if (p.WindowStart == "") != (p.WindowEnd == "") {
		return errors.New("发送时间窗必须同时设置开始和结束")
	}
	if p.WindowStart != "" {
		a, err := time.Parse("15:04", p.WindowStart)
		if err != nil {
			return err
		}
		b, err := time.Parse("15:04", p.WindowEnd)
		if err != nil {
			return err
		}
		if !a.Before(b) {
			return errors.New("时间窗结束必须晚于开始；跨天请拆分计划")
		}
	}
	for _, d := range p.Weekdays {
		if d < 0 || d > 6 {
			return errors.New("星期范围为0至6")
		}
	}
	return nil
}

// Window returns the first eligible instant, considering the policy's local calendar.
func (p Policy) Window(now time.Time) time.Time {
	zone := p.Timezone
	if zone == "" {
		zone = "Asia/Shanghai"
	}
	loc, err := time.LoadLocation(zone)
	if err != nil {
		return now.Add(24 * time.Hour)
	}
	current := now.In(loc)
	for day := 0; day < 9; day++ {
		date := current.AddDate(0, 0, day)
		allowed := len(p.Weekdays) == 0
		for _, d := range p.Weekdays {
			if d == int(date.Weekday()) {
				allowed = true
			}
		}
		if !allowed {
			continue
		}
		start := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, loc)
		end := start.AddDate(0, 0, 1)
		if p.WindowStart != "" {
			a, _ := time.Parse("15:04", p.WindowStart)
			b, _ := time.Parse("15:04", p.WindowEnd)
			start = time.Date(date.Year(), date.Month(), date.Day(), a.Hour(), a.Minute(), 0, 0, loc)
			end = time.Date(date.Year(), date.Month(), date.Day(), b.Hour(), b.Minute(), 0, 0, loc)
		}
		if current.Before(start) {
			return start
		}
		if current.Before(end) {
			return now
		}
	}
	return now.Add(7 * 24 * time.Hour)
}

type Limit struct {
	Key    string
	Policy Policy
}
type Deferred struct {
	Until  time.Time
	Reason string
}

func (d *Deferred) Error() string { return d.Reason }

func Limits(ctx context.Context, dept int64, sender, ip, provider string, taskID int64, p Policy) ([]Limit, error) {
	limits := []Limit{{fmt.Sprintf("task:%d", taskID), p}}
	scopes := [][2]string{{"global", "*"}, {"department", fmt.Sprint(dept)}, {"sender", strings.ToLower(sender)}, {"ip", ip}, {"provider", provider}}
	for _, s := range scopes {
		r, err := g.DB().GetOne(ctx, `SELECT policy FROM bm_send_policies WHERE scope=$1 AND scope_key IN($2,'*') ORDER BY CASE WHEN scope_key=$2 THEN 0 ELSE 1 END LIMIT 1`, s[0], s[1])
		if err != nil {
			return nil, err
		}
		if len(r) > 0 {
			limits = append(limits, Limit{s[0] + ":" + s[1], DecodePolicy(r["policy"].String())})
		}
	}
	return limits, nil
}

// Reserve checks ALL scopes before consuming ANY quota. PostgreSQL's transaction
// advisory lock makes spacing, rolling quotas and contact cooldown shared by workers.
func Reserve(ctx context.Context, limits []Limit, dept int64, recipient, lease string) (func(), error) {
	err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if _, err := tx.Exec("SELECT pg_advisory_xact_lock(614805)"); err != nil {
			return err
		}
		now := time.Now()
		_, err := tx.Exec(`DELETE FROM bm_send_slots WHERE expires_at < now()`)
		if err != nil {
			return err
		}
		cooldown := 0
		for _, l := range limits {
			if l.Policy.RecipientCooldownHours > cooldown {
				cooldown = l.Policy.RecipientCooldownHours
			}
			if next := l.Policy.Window(now); next.After(now) {
				return &Deferred{next, "等待发送时间窗：" + l.Key}
			}
			if l.Policy.BatchSize > 0 && l.Policy.BatchRestSeconds > 0 {
				row, e := tx.GetOne(`SELECT count,blocked_until FROM bm_batch_state WHERE scope_key=$1`, l.Key)
				if e != nil {
					return e
				}
				if len(row) > 0 && row["count"].Int() >= l.Policy.BatchSize && row["blocked_until"].Time().After(now) {
					return &Deferred{row["blocked_until"].Time(), "等待批间休息：" + l.Key}
				}
			}
			v, err := tx.GetValue(`SELECT next_at FROM bm_rate_spacing WHERE scope_key=$1`, l.Key)
			if err != nil {
				return err
			}
			if !v.IsEmpty() && v.Time().After(now) {
				return &Deferred{v.Time(), "等待发送间隔：" + l.Key}
			}
			if l.Policy.Concurrency > 0 {
				v, err := tx.GetValue(`SELECT count(*) FROM bm_send_slots WHERE $1=ANY(scope_keys)`, l.Key)
				if err != nil {
					return err
				}
				if v.Int() >= l.Policy.Concurrency {
					return &Deferred{now.Add(time.Second), "等待并发额度：" + l.Key}
				}
			}
			for _, q := range []struct {
				n       int
				seconds int
			}{{l.Policy.PerMinute, 60}, {l.Policy.PerHour, 3600}, {l.Policy.PerDay, 86400}} {
				if q.n == 0 {
					continue
				}
				v, err := tx.GetValue(`SELECT reserved_at + ($3 * interval '1 second') FROM bm_rate_events WHERE scope_key=$1 AND reserved_at > now()-($3 * interval '1 second') ORDER BY reserved_at DESC OFFSET $2 LIMIT 1`, l.Key, q.n-1, q.seconds)
				if err != nil {
					return err
				}
				if !v.IsEmpty() {
					return &Deferred{v.Time(), "等待滚动配额：" + l.Key}
				}
			}
		}
		contactKey := fmt.Sprintf("%d:%s", dept, strings.ToLower(strings.TrimSpace(recipient)))
		if cooldown > 0 {
			v, err := tx.GetValue(`SELECT next_at FROM bm_recipient_frequency WHERE scope_key=$1`, contactKey)
			if err != nil {
				return err
			}
			if !v.IsEmpty() && v.Time().After(now) {
				return &Deferred{v.Time(), "等待收件人触达间隔"}
			}
		}
		keys := []string{}
		for _, l := range limits {
			keys = append(keys, l.Key)
			if l.Policy.BatchSize > 0 && l.Policy.BatchRestSeconds > 0 {
				_, e := tx.Exec(`INSERT INTO bm_batch_state(scope_key,count,blocked_until) VALUES($1,1,now()+($3*interval '1 second')) ON CONFLICT(scope_key) DO UPDATE SET count=CASE WHEN bm_batch_state.count >= $2 THEN 1 ELSE bm_batch_state.count+1 END,blocked_until=now()+($3*interval '1 second')`, l.Key, l.Policy.BatchSize, l.Policy.BatchRestSeconds)
				if e != nil {
					return e
				}
			}
			if _, err := tx.Exec(`INSERT INTO bm_rate_events(scope_key) VALUES($1)`, l.Key); err != nil {
				return err
			}
			if l.Policy.MinIntervalMS > 0 {
				if _, err := tx.Exec(`INSERT INTO bm_rate_spacing VALUES($1,now()+($2*interval '1 millisecond')) ON CONFLICT(scope_key) DO UPDATE SET next_at=excluded.next_at`, l.Key, l.Policy.MinIntervalMS); err != nil {
					return err
				}
			}
		}
		b, _ := json.Marshal(keys)
		if _, err := tx.Exec(`INSERT INTO bm_send_slots(token,scope_keys,expires_at) SELECT $1,ARRAY(SELECT jsonb_array_elements_text($2::jsonb)),now()+interval '5 minutes'`, lease, string(b)); err != nil {
			return err
		}
		if cooldown > 0 {
			_, err = tx.Exec(`INSERT INTO bm_recipient_frequency VALUES($1,now()+($2*interval '1 hour')) ON CONFLICT(scope_key) DO UPDATE SET next_at=excluded.next_at`, contactKey, cooldown)
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	return func() {
		_, err := g.DB().Exec(context.Background(), `DELETE FROM bm_send_slots WHERE token=$1`, lease)
		if err != nil {
			g.Log().Error(context.Background(), err)
		}
	}, nil
}
