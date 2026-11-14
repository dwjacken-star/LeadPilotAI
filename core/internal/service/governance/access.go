// Package governance contains the shared authorization and persistent sending policy.
// Legacy business routes are intentionally administrator-only; departmental users use
// the scoped workspace API. This prevents unscoped legacy SQL and file APIs bypassing RBAC.
package governance

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

//go:embed migration.sql
var migration string

//go:embed growth.sql
var growthMigration string

//go:embed delivery.sql
var deliveryMigration string

//go:embed statistics.sql
var statisticsMigration string

//go:embed social.sql
var socialMigration string

//go:embed master.sql
var masterMigration string

func Migrate(ctx context.Context) error {
	err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if _, err := tx.Exec("SELECT pg_advisory_xact_lock(614804)"); err != nil {
			return err
		}
		if _, err := tx.Exec(migration); err != nil {
			return fmt.Errorf("department migration: %w", err)
		}
		if _, err := tx.Exec(growthMigration); err != nil {
			return fmt.Errorf("growth migration: %w", err)
		}
		if _, err := tx.Exec(deliveryMigration); err != nil {
			return fmt.Errorf("delivery migration: %w", err)
		}
		if _, err := tx.Exec(statisticsMigration); err != nil {
			return fmt.Errorf("statistics migration: %w", err)
		}
		if _, err := tx.Exec(socialMigration); err != nil {
			return fmt.Errorf("social migration: %w", err)
		}
		if _, err := tx.Exec(masterMigration); err != nil {
			return fmt.Errorf("industry growth migration: %w", err)
		}
		// Existing rows belong to the default department and the original administrator.
		for _, table := range []string{"bm_contact_groups", "bm_contacts", "bm_tags", "email_templates", "email_tasks", "api_templates"} {
			_, err := tx.Exec(fmt.Sprintf(`UPDATE %s SET owner_id=COALESCE((SELECT a.account_id FROM account a JOIN account_role ar USING(account_id) JOIN role r USING(role_id) WHERE r.role_name='admin' ORDER BY a.account_id LIMIT 1),0) WHERE owner_id=0`, table))
			if err != nil {
				return err
			}
		}
		_, err := tx.Exec(`UPDATE recipient_info SET delivery_state='accepted' WHERE is_sent=1 AND delivery_state='pending'`)
		return err
	})
	if err != nil {
		return err
	}
	// Base initializers use ORM before these ALTER TABLE statements. Its cached
	// column list would silently discard new department/permission fields on writes.
	return g.DB().GetCore().ClearTableFieldsAll(ctx)
}

type Role struct {
	ID          int64    `json:"role_id"`
	Name        string   `json:"role_name"`
	Scope       string   `json:"data_scope"`
	Custom      []int64  `json:"custom_depts"`
	Permissions []string `json:"permissions"`
}
type Principal struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	DeptID   int64  `json:"dept_id"`
	Super    bool   `json:"super"`
	Version  int64  `json:"token_version"`
	Roles    []Role `json:"roles"`
}

func Load(ctx context.Context, id int64) (*Principal, error) {
	row, err := g.DB().GetOne(ctx, `SELECT a.* FROM account a JOIN bm_departments d ON d.id=a.dept_id WHERE a.account_id=$1 AND a.status=1 AND d.active=true AND NOT EXISTS(WITH RECURSIVE parents AS(SELECT id,parent_id,active FROM bm_departments WHERE id=d.id UNION ALL SELECT p.id,p.parent_id,p.active FROM bm_departments p JOIN parents c ON p.id=c.parent_id) SELECT 1 FROM parents WHERE NOT active)`, id)
	if err != nil {
		return nil, err
	}
	if len(row) == 0 {
		return nil, errors.New("账号或部门已停用")
	}
	p := &Principal{ID: id, Username: row["username"].String(), DeptID: row["dept_id"].Int64(), Version: row["token_version"].Int64(), Roles: []Role{}}
	rows, err := g.DB().GetAll(ctx, `SELECT r.* FROM role r JOIN account_role ar USING(role_id) WHERE ar.account_id=$1 AND r.status=1`, id)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		role := Role{ID: r["role_id"].Int64(), Name: r["role_name"].String(), Scope: r["data_scope"].String()}
		_ = json.Unmarshal(r["permissions"].Bytes(), &role.Permissions)
		_ = json.Unmarshal(r["custom_depts"].Bytes(), &role.Custom)
		p.Roles = append(p.Roles, role)
		if role.Name == "admin" {
			p.Super = true
		}
	}
	return p, nil
}
func (p *Principal) Has(code string) bool {
	if p.Super {
		return true
	}
	for _, r := range p.Roles {
		if grants(r, code) {
			return true
		}
	}
	return false
}
func grants(r Role, code string) bool {
	for _, v := range r.Permissions {
		if v == code || v == "*" {
			return true
		}
	}
	return false
}

// Predicate computes the UNION of scopes only from roles granting this operation.
func (p *Principal) Predicate(ctx context.Context, code, alias string) (string, error) {
	if p.Super {
		return "TRUE", nil
	}
	if alias != "" {
		alias += "."
	}
	parts := []string{}
	for _, r := range p.Roles {
		if !grants(r, code) {
			continue
		}
		switch r.Scope {
		case "all":
			return "TRUE", nil
		case "self":
			parts = append(parts, fmt.Sprintf("(%sdept_id=%d AND %sowner_id=%d)", alias, p.DeptID, alias, p.ID))
		case "department":
			parts = append(parts, fmt.Sprintf("%sdept_id=%d", alias, p.DeptID))
		case "descendants":
			parts = append(parts, fmt.Sprintf(`%sdept_id IN (WITH RECURSIVE tree AS (SELECT id FROM bm_departments WHERE id=%d UNION ALL SELECT d.id FROM bm_departments d JOIN tree t ON d.parent_id=t.id) SELECT id FROM tree)`, alias, p.DeptID))
		case "custom":
			if len(r.Custom) > 0 {
				ids := []string{}
				for _, id := range r.Custom {
					if id > 0 {
						ids = append(ids, fmt.Sprint(id))
					}
				}
				if len(ids) > 0 {
					parts = append(parts, alias+"dept_id IN ("+strings.Join(ids, ",")+")")
				}
			}
		}
	}
	if len(parts) == 0 {
		return "FALSE", nil
	}
	return "(" + strings.Join(parts, " OR ") + ")", nil
}
func (p *Principal) Check(ctx context.Context, table string, id int64, code string) error {
	allowed := map[string]bool{"bm_contact_groups": true, "bm_contacts": true, "bm_tags": true, "email_templates": true, "email_tasks": true, "bm_campaign_schedules": true, "bm_automation_rules": true}
	if !allowed[table] {
		return errors.New("未知资源")
	}
	where, _ := p.Predicate(ctx, code, "")
	n, err := g.DB().Model(table).Ctx(ctx).Where("id", id).Where(where).Count()
	if err != nil {
		return err
	}
	if n == 0 {
		return errors.New("资源不存在或无权访问")
	}
	return nil
}

// CheckOwnership applies the same operation-bound scope to resources not yet
// inserted, rather than treating possession of a permission as global create access.
func (p *Principal) CheckOwnership(ctx context.Context, code string, dept, owner int64) error {
	predicate, err := p.Predicate(ctx, code, "resource")
	if err != nil {
		return err
	}
	value, err := g.DB().GetValue(ctx, `SELECT EXISTS(SELECT 1 FROM (SELECT $1::bigint dept_id,$2::bigint owner_id) resource WHERE `+predicate+`)`, dept, owner)
	if err != nil {
		return err
	}
	if !value.Bool() {
		return errors.New("该操作的数据范围不包含目标部门或负责人")
	}
	return nil
}
func SenderAllowed(ctx context.Context, p *Principal, sender string) error {
	n, err := g.DB().Model("mailbox").Ctx(ctx).Where("username", strings.ToLower(sender)).Where("active", 1).Count()
	if err != nil {
		return err
	}
	if n == 0 {
		return errors.New("发件邮箱不存在或已停用")
	}
	if p.Super {
		return nil
	}
	n, err = g.DB().Model("bm_resource_grants").Ctx(ctx).Where("dept_id", p.DeptID).Where("sender", strings.ToLower(sender)).Where("active", true).Count()
	if err != nil {
		return err
	}
	if n == 0 {
		return errors.New("所在部门未获此邮箱的发信授权")
	}
	return nil
}
func Audit(ctx context.Context, p *Principal, action string, id int64, detail interface{}) error {
	b, _ := json.Marshal(detail)
	_, err := g.DB().Exec(ctx, `INSERT INTO bm_security_audit(account_id,dept_id,action,resource_id,detail) VALUES($1,$2,$3,$4,$5::jsonb)`, p.ID, p.DeptID, action, id, string(b))
	return err
}

var PermissionCatalog = []string{"contact:list", "contact:create", "contact:update", "contact:delete", "contact:import", "contact:export", "template:list", "template:create", "template:update", "template:delete", "campaign:list", "campaign:create", "campaign:update", "campaign:delete", "campaign:publish", "campaign:pause", "campaign:cancel", "schedule:list", "schedule:manage", "automation:list", "automation:manage", "report:list", "overview:view", "crm:list", "crm:manage", "crm:delete", "crm:sync", "crm:reply"}
