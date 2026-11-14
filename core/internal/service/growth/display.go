package growth

import (
	"context"
	"encoding/json"
	"strings"

	"billionmail-core/internal/service/emailvalidation"
	"billionmail-core/internal/service/governance"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// Only enrich a record after its normal department/owner access check succeeds.
// Related names belong to this already-visible business record; company:list
// permission and the first page of the company selector are not prerequisites.
func recordReferences(ctx context.Context, r gdb.Record) error {
	id := func(key string) int64 {
		if r[key] == nil {
			return 0
		}
		return r[key].Int64()
	}
	names, e := g.DB().GetOne(ctx, `SELECT
 (SELECT name FROM bm_growth_companies WHERE id=$1) company_name,
 (SELECT name FROM bm_growth_plans WHERE id=$2) plan_name,
 (SELECT p.name||' · '||to_char(x.created_at,'YYYY-MM-DD HH24:MI:SS') FROM bm_growth_runs x JOIN bm_growth_plans p ON p.id=x.plan_id WHERE x.id=$3) run_name,
 (SELECT email FROM bm_growth_leads WHERE id=$4) lead_email,
 (SELECT coalesce(nullif(subject,''),remark) FROM email_tasks WHERE id=$5) task_name`, id("company_id"), id("plan_id"), id("run_id"), id("lead_id"), id("task_id"))
	if e != nil {
		return e
	}
	for key, value := range names {
		r[key] = value
	}
	return nil
}

func candidateEmails(ctx context.Context, p *governance.Principal, candidate gdb.Record) ([]g.Map, error) {
	emails := []string{}
	if e := json.Unmarshal(candidate["emails"].Bytes(), &emails); e != nil {
		return nil, e
	}
	if len(emails) == 0 {
		return []g.Map{}, nil
	}
	config, e := g.DB().Model("bm_growth_runs").Ctx(ctx).Where("id", candidate["run_id"]).Value("config")
	if e != nil {
		return nil, e
	}
	var plan PlanConfig
	if e = json.Unmarshal(config.Bytes(), &plan); e != nil {
		return nil, e
	}
	scope, e := p.Predicate(ctx, "outreach:list", "")
	if e != nil {
		return nil, e
	}
	leads, e := g.DB().Model("bm_growth_leads").Ctx(ctx).Where(scope).Where("plan_id", candidate["plan_id"]).WhereIn("email", emails).Fields("bm_growth_leads.*,(SELECT j.error FROM bm_growth_jobs j WHERE j.kind='verify' AND j.resource_id=bm_growth_leads.id::text AND j.status IN('failed','configuration_required') ORDER BY j.id DESC LIMIT 1) verification_error").All()
	if e != nil {
		return nil, e
	}
	byEmail := map[string]gdb.Record{}
	for _, lead := range leads {
		byEmail[lead["email"].String()] = lead
	}
	rows := []g.Map{}
	for _, email := range emails {
		item := g.Map{"email": email, "status": "collected", "reason": "", "keyword": ""}
		if _, ok := collectedEmail(email); !ok {
			item["status"] = "invalid"
			item["reason"] = "invalid_email"
			rows = append(rows, item)
			continue
		}
		for _, word := range plan.ExcludeKeywords {
			if word != "" && strings.Contains(strings.ToLower(email), strings.ToLower(word)) {
				item["status"] = "filtered"
				item["reason"] = "keyword_match"
				item["keyword"] = word
				break
			}
		}
		if item["status"] != "filtered" {
			if lead, ok := byEmail[email]; ok {
				item["status"] = lead["state"].String()
				item["reason"] = lead["error"].String()
				item["lead_id"] = lead["id"].Int64()
				if item["reason"] == "" && item["status"] == "collected" {
					item["reason"] = lead["verification_error"].String()
				}
			} else if candidate["state"].String() == "unmatched" {
				item["status"] = "unmatched"
			} else {
				item["reason"] = candidate["error"].String()
			}
		}
		rows = append(rows, item)
	}
	return rows, nil
}

// URL coordinates can satisfy a permissive mailbox parser (e.g. /@14.5676321).
// Public web email discovery requires a DNS-style, non-numeric final label.
func collectedEmail(value string) (string, bool) {
	email, domain, ok := emailvalidation.Normalize(value)
	if !ok {
		return "", false
	}
	suffix := domain[strings.LastIndexByte(domain, '.')+1:]
	local := email[:strings.LastIndexByte(email, '@')]
	if strings.HasPrefix(local, "/") || strings.HasSuffix(local, "/") || strings.Contains(local, "//") {
		return "", false
	}
	letter := false
	for _, r := range suffix {
		if r >= 'a' && r <= 'z' {
			letter = true
		}
	}
	return email, letter && len(suffix) >= 2
}
