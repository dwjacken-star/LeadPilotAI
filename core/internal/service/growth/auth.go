package growth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"billionmail-core/internal/service/governance"
	"github.com/gogf/gf/v2/container/gvar"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

var Permissions = []string{"company:list", "company:manage", "company:publish", "material:list", "material:upload", "material:recognize", "material:manage", "material:publish", "material:download", "material:delete", "product:list", "product:import", "product:manage", "product:publish", "product:export", "outreach:list", "outreach:manage", "outreach:run", "outreach:publish", "outreach:export", "inquiry:list", "inquiry:manage", "crm:ai_draft", "growth:settings"}

func init() { governance.PermissionCatalog = append(governance.PermissionCatalog, Permissions...) }

func require(p *governance.Principal, code string) error {
	if p == nil || !p.Has(code) {
		return fmt.Errorf("permission required: %s", code)
	}
	return nil
}
func company(ctx context.Context, p *governance.Principal, id int64, code string, useShared bool) (gdb.Record, error) {
	if e := require(p, code); e != nil {
		return nil, e
	}
	scope, e := p.Predicate(ctx, code, "c")
	if e != nil {
		return nil, e
	}
	if useShared {
		scope = "(" + scope + fmt.Sprintf(" OR EXISTS(SELECT 1 FROM bm_growth_grants x WHERE x.company_id=c.id AND x.dept_id=%d AND x.use_materials))", p.DeptID)
	}
	r, e := g.DB().Model("bm_growth_companies c").Ctx(ctx).Where("c.id", id).WhereNull("c.deleted_at").Where(scope).One()
	if e != nil {
		return nil, e
	}
	if len(r) == 0 {
		return nil, errors.New("company not found or access denied")
	}
	return r, nil
}
func scoped(ctx context.Context, p *governance.Principal, table string, id int64, permission string) (gdb.Record, error) {
	if e := require(p, permission); e != nil {
		return nil, e
	}
	scope, e := p.Predicate(ctx, permission, "")
	if e != nil {
		return nil, e
	}
	allowed := map[string]bool{"bm_growth_candidates": true, "bm_growth_plans": true, "bm_growth_leads": true, "bm_growth_messages": true, "bm_growth_runs": true, "bm_growth_inquiries": true, "bm_growth_inbox_rules": true, "bm_growth_jobs": true}
	if !allowed[table] {
		return nil, errors.New("unsupported resource")
	}
	r, e := g.DB().Model(table).Ctx(ctx).Where("id", id).WhereNull("deleted_at").Where(scope).One()
	if e != nil {
		return nil, e
	}
	if len(r) == 0 {
		return nil, errors.New("resource not found or access denied")
	}
	return r, nil
}
func jsonText(value interface{}) string { b, _ := json.Marshal(value); return string(b) }
func jsonMap(value *gvar.Var) g.Map {
	r := g.Map{}
	if value != nil {
		_ = json.Unmarshal(value.Bytes(), &r)
	}
	if r == nil {
		r = g.Map{}
	}
	return r
}
func allowedFields(values map[string]interface{}, keys ...string) g.Map {
	out := g.Map{}
	for _, key := range keys {
		if v, ok := values[key]; ok {
			out[key] = v
		}
	}
	return out
}
func audit(ctx context.Context, p *governance.Principal, action string, id int64) error {
	return governance.Audit(ctx, p, "growth:"+action, id, g.Map{})
}
func cleanText(v string, limit int) string {
	v = strings.TrimSpace(v)
	if len([]rune(v)) > limit {
		return string([]rune(v)[:limit])
	}
	return v
}
