package growth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"billionmail-core/internal/service/batch_mail"
	"billionmail-core/internal/service/governance"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/gconv"
	"github.com/google/uuid"
)

func sanitizeProduct(r gdb.Record) {
	v := jsonMap(r["published_fields"])
	r["fields"] = g.NewVar(v["specifications"])
	for _, k := range []string{"sku", "name", "category"} {
		r[k] = g.NewVar(v[k])
	}
	delete(r, "manual_keys")
	delete(r, "source_asset")
	delete(r, "source_location")
}
func reservedAddress(ip net.IP) bool {
	for _, cidr := range []string{"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4", "2001:db8::/32"} {
		_, n, _ := net.ParseCIDR(cidr)
		if n.Contains(ip) {
			return true
		}
	}
	return false
}
func kindPermission(kind string) string {
	switch kind {
	case "ingest":
		return "material:recognize"
	case "inbox", "classify", "forward":
		return "inquiry:manage"
	case "dispatch":
		return "outreach:publish"
	default:
		return "outreach:run"
	}
}
func jobPermission(ctx context.Context, id int64) string {
	v, _ := g.DB().Model("bm_growth_jobs").Ctx(ctx).Where("id", id).Value("kind")
	return kindPermission(v.String())
}
func workerPrincipal(ctx context.Context, id, dept int64, permission string) (*governance.Principal, error) {
	p, e := governance.Load(ctx, id)
	if e != nil {
		return nil, e
	}
	if p.Super {
		p.DeptID = dept
	}
	if p.DeptID != dept {
		return nil, errors.New("owner department changed")
	}
	if e = require(p, permission); e != nil {
		return nil, e
	}
	n, e := g.DB().Model("bm_departments").Ctx(ctx).Where("id", dept).Where("active", true).Count()
	if e != nil || n == 0 {
		return nil, errors.New("department disabled")
	}
	return p, nil
}

// RunOne claims a persisted lease. A crashed job is retryable; SMTP forwarding
// has its own at-most-once handoff record and is never blindly retried.
func RunOne(ctx context.Context) (bool, error) {
	token := uuid.NewString()
	r, e := g.DB().GetOne(ctx, `UPDATE bm_growth_jobs SET status='running',attempts=attempts+1,lease_token=$1,lease_until=now()+interval '10 minutes',updated_at=now() WHERE id=(SELECT id FROM bm_growth_jobs WHERE deleted_at IS NULL AND ((status='queued' AND next_at<=now()) OR (status='running' AND lease_until<now())) ORDER BY next_at,id FOR UPDATE SKIP LOCKED LIMIT 1) RETURNING *`, token)
	if e != nil || len(r) == 0 {
		return false, e
	}
	jobCtx, cancel := context.WithTimeout(ctx, 8*time.Minute)
	defer cancel()
	done := make(chan struct{})
	defer close(done)
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-jobCtx.Done():
				return
			case <-ticker.C:
				v, err := g.DB().Model("bm_growth_jobs").Ctx(jobCtx).Where("id", r["id"]).Where("lease_token", token).Where("status", "running").Count()
				_, authErr := workerPrincipal(jobCtx, r["owner_id"].Int64(), r["dept_id"].Int64(), kindPermission(r["kind"].String()))
				if authErr != nil || (err == nil && v == 0) {
					cancel()
					return
				}
			}
		}
	}()
	p, err := workerPrincipal(jobCtx, r["owner_id"].Int64(), r["dept_id"].Int64(), kindPermission(r["kind"].String()))
	if err == nil {
		err = executeJob(jobCtx, p, r)
	}
	status, errorText, progress, next := "completed", "", 100, time.Now()
	if err != nil {
		status, errorText, progress = "failed", cleanText(err.Error(), 1000), 0
		low := strings.ToLower(errorText)
		if strings.Contains(low, "not configured") || strings.Contains(low, "disabled") || strings.Contains(low, "configuration") || strings.Contains(low, "budget") {
			status = "configuration_required"
		} else if r["attempts"].Int() < 3 && !strings.Contains(low, "permission") && !strings.Contains(low, "denied") && !strings.Contains(low, "paused") {
			status = "queued"
			next = time.Now().Add(time.Duration(r["attempts"].Int()*30) * time.Second)
		}
	}
	_, e = g.DB().Model("bm_growth_jobs").Ctx(ctx).Where("id", r["id"]).Where("lease_token", token).Where("status", "running").Data(g.Map{"status": status, "error": errorText, "progress": progress, "next_at": next, "lease_until": nil, "lease_token": nil, "updated_at": time.Now()}).Update()
	return true, e
}
func executeJob(ctx context.Context, p *governance.Principal, r gdb.Record) error {
	data := jsonMap(r["payload"])
	runID := gconv.Int64(data["run_id"])
	id := r["resource_id"].Int64()
	switch r["kind"].String() {
	case "ingest":
		return ingest(WithModel(ctx, gconv.String(data["model"])), p, r["resource_id"].String())
	case "research":
		return research(ctx, p, id)
	case "discover":
		return discover(ctx, p, id)
	case "collect":
		return collect(ctx, p, data)
	case "verify":
		return verifyLead(ctx, p, id, runID)
	case "draft":
		return draftLead(ctx, p, id, runID)
	case "dispatch":
		return dispatchRun(ctx, p, id)
	case "inbox":
		return syncInbox(ctx, p, id)
	case "classify":
		return classifyMessage(ctx, p, id, gconv.Int64(data["rule_id"]))
	case "forward":
		return forwardInquiry(ctx, p, id)
	}
	return errors.New("unknown background job")
}
func Tick(ctx context.Context) error {
	if _, e := g.DB().Exec(ctx, `UPDATE bm_growth_forwards SET status='unknown',error='Forwarding worker stopped before recording a result; verify manually before any resend' WHERE status='sending' AND created_at<now()-interval '10 minutes'`); e != nil {
		return e
	}
	rows, e := g.DB().Model("bm_growth_plans").Ctx(ctx).Where("active", true).Where("next_at<=now()").Limit(20).All()
	if e != nil {
		return e
	}
	for _, r := range rows {
		p, err := workerPrincipal(ctx, r["owner_id"].Int64(), r["dept_id"].Int64(), "outreach:run")
		if err != nil {
			_, _ = g.DB().Model("bm_growth_plans").Where("id", r["id"]).Data(g.Map{"active": false, "error": err.Error()}).Update()
			continue
		}
		var c PlanConfig
		if json.Unmarshal(r["config"].Bytes(), &c) != nil {
			continue
		}
		_, err = newRun(ctx, p, r, r["next_at"].Time())
		if err != nil {
			return err
		}
		next, err := (batch_mail.ScheduleSpec{Cadence: "daily", Timezone: c.Timezone, LocalTime: c.LocalTime}).Next(time.Now())
		if err != nil {
			return err
		}
		_, _ = g.DB().Model("bm_growth_plans").Where("id", r["id"]).Data(g.Map{"next_at": next}).Update()
	}
	rules, e := g.DB().Model("bm_growth_inbox_rules").Ctx(ctx).Where("active", true).Where("next_at<=now()").Limit(30).All()
	if e != nil {
		return e
	}
	for _, r := range rules {
		p, err := workerPrincipal(ctx, r["owner_id"].Int64(), r["dept_id"].Int64(), "inquiry:manage")
		if err != nil {
			_, _ = g.DB().Model("bm_growth_inbox_rules").Where("id", r["id"]).Data(g.Map{"active": false, "error": err.Error()}).Update()
			continue
		}
		_, err = Queue(ctx, p, "inbox", r["id"].String(), fmt.Sprintf("inbox:%d:%d", r["id"].Int64(), time.Now().Unix()/60), g.Map{})
		if err != nil {
			return err
		}
		_, _ = g.DB().Model("bm_growth_inbox_rules").Where("id", r["id"]).Data(g.Map{"next_at": time.Now().Add(time.Minute)}).Update()
	}
	_, e = g.DB().Exec(ctx, `WITH finished AS (SELECT r.id,(`+runStatsSQL+`) summary FROM bm_growth_runs r WHERE r.status IN ('queued','running','attention') AND NOT EXISTS(SELECT 1 FROM bm_growth_jobs j WHERE j.payload->>'run_id'=r.id::text AND j.status IN('queued','running'))) UPDATE bm_growth_runs r SET stats=f.summary,status=CASE WHEN (f.summary->>'failed_jobs')::int>0 OR (f.summary->>'failed_pages')::int>0 THEN 'attention' WHEN (f.summary->>'source_count')::int=0 THEN 'no_results' ELSE 'completed' END,error=CASE WHEN (f.summary->>'failed_jobs')::int>0 THEN 'Some jobs failed or require service configuration; inspect background tasks' WHEN (f.summary->>'failed_pages')::int>0 THEN 'Some company pages could not be collected; inspect candidates' WHEN (f.summary->>'source_count')::int=0 THEN 'Search returned no websites; no email collection or sending occurred' ELSE '' END FROM finished f WHERE r.id=f.id`)
	return e
}

var startOnce sync.Once

func Start(ctx context.Context) {
	startOnce.Do(func() {
		MasterStart(ctx)
		for i := 0; i < 2; i++ {
			go func() {
				tick := time.NewTicker(2 * time.Second)
				defer tick.Stop()
				for {
					select {
					case <-ctx.Done():
						return
					case <-tick.C:
						if _, e := RunOne(ctx); e != nil {
							g.Log().Error(ctx, "growth worker", e)
						}
					}
				}
			}()
		}
		go func() {
			tick := time.NewTicker(30 * time.Second)
			defer tick.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-tick.C:
					if e := Tick(ctx); e != nil {
						g.Log().Error(ctx, "growth schedule", e)
					}
				}
			}
		}()
	})
}
