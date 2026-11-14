package webmaildelivery

import (
	"billionmail-core/internal/service/aimodels"
	"billionmail-core/internal/service/crm"
	"billionmail-core/internal/service/governance"
	"billionmail-core/internal/service/growth"
	"context"
	"errors"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/gconv"
	"net/mail"
	"strings"
	"time"
)

// Linking is explicit. A mailbox password never becomes an administrator token.
func Bindings(ctx context.Context, p *governance.Principal, mailbox, keyword string, page int, selectedAccount ...int64) (g.Map, error) {
	if p == nil || !p.Super {
		return nil, errors.New("Administrator access required")
	}
	mailbox = strings.ToLower(strings.TrimSpace(mailbox))
	if page < 1 {
		page = 1
	}
	parents := `NOT EXISTS(WITH RECURSIVE parents AS(SELECT id,parent_id,active FROM bm_departments WHERE id=d.id UNION ALL SELECT p.id,p.parent_id,p.active FROM bm_departments p JOIN parents c ON p.id=c.parent_id) SELECT 1 FROM parents WHERE NOT active)`
	eligible := `a.status=1 AND COALESCE(d.active,false) AND ` + parents
	fields := `a.username,a.dept_id,d.name AS dept_name,COALESCE((` + eligible + `),false) AS eligible,
 EXISTS(SELECT 1 FROM role r JOIN account_role ar USING(role_id) WHERE ar.account_id=a.account_id AND r.status=1 AND r.role_name='admin') AS super,
 EXISTS(SELECT 1 FROM bm_resource_grants g WHERE g.dept_id=a.dept_id AND g.sender=$1 AND g.active) AS has_sender_grant`
	rows, e := g.DB().GetAll(ctx, `SELECT a.account_id,`+fields+` FROM account a JOIN bm_departments d ON d.id=a.dept_id WHERE `+eligible+` AND a.username ILIKE $2 ORDER BY a.account_id LIMIT 20 OFFSET $3`, mailbox, "%"+keyword+"%", (page-1)*20)
	if e != nil {
		return nil, e
	}
	count, e := g.DB().GetValue(ctx, `SELECT count(*) FROM account a JOIN bm_departments d ON d.id=a.dept_id WHERE `+eligible+` AND a.username ILIKE $1`, "%"+keyword+"%")
	if e != nil {
		return nil, e
	}
	binding, e := g.DB().GetOne(ctx, `SELECT b.mailbox,b.account_id,`+fields+` FROM bm_webmail_bindings b LEFT JOIN account a ON a.account_id=b.account_id LEFT JOIN bm_departments d ON d.id=a.dept_id WHERE b.mailbox=$1`, mailbox)
	if e != nil {
		return nil, e
	}
	selected := gdb.Record{}
	if len(selectedAccount) == 1 && selectedAccount[0] > 0 {
		selected, e = g.DB().GetOne(ctx, `SELECT a.account_id,`+fields+` FROM account a LEFT JOIN bm_departments d ON d.id=a.dept_id WHERE a.account_id=$2`, mailbox, selectedAccount[0])
	}
	return g.Map{"accounts": rows, "total": count.Int(), "binding": binding, "selected_account": selected}, e
}

func SaveBinding(ctx context.Context, p *governance.Principal, mailbox string, accountID int64) error {
	return SaveBindingWithGrant(ctx, p, mailbox, accountID, false)
}

// Department-wide authorization is never implicit. Only an administrator's
// explicit choice can create/reactivate a grant, atomically with the binding.
func SaveBindingWithGrant(ctx context.Context, p *governance.Principal, mailbox string, accountID int64, grantDepartment bool, expectedDepartment ...int64) error {
	if p == nil || !p.Super {
		return errors.New("Administrator access required")
	}
	mailbox = strings.ToLower(strings.TrimSpace(mailbox))
	if mailbox == "" || accountID < 0 {
		return errors.New("valid mailbox and account required")
	}
	return g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		deptID := int64(0)
		grantApplied := false
		if accountID == 0 {
			// Unbinding is revocation; allow removal of a stale/inactive binding.
			if _, e := tx.Model("bm_webmail_bindings").Where("mailbox", mailbox).Delete(); e != nil {
				return e
			}
		} else {
			row, e := tx.Model("mailbox").Where("username", mailbox).Where("active", 1).LockUpdate().One()
			if e != nil {
				return e
			}
			if row.IsEmpty() {
				return errors.New("active mailbox required")
			}
			if _, e = tx.Model("account").Where("account_id", accountID).LockUpdate().One(); e != nil {
				return e
			}
			linked, e := governance.Load(ctx, accountID)
			if e != nil {
				return e
			}
			deptID = linked.DeptID
			if grantDepartment && !linked.Super {
				if len(expectedDepartment) != 1 || expectedDepartment[0] <= 0 || expectedDepartment[0] != deptID {
					return errors.New("账号所属部门已变化，请刷新后确认授权")
				}
				if _, e = tx.Exec(`INSERT INTO bm_resource_grants(dept_id,sender,active) VALUES($1,$2,true) ON CONFLICT(dept_id,sender) DO UPDATE SET active=true`, deptID, mailbox); e != nil {
					return e
				}
				grantApplied = true
			}
			if e = governance.SenderAllowed(ctx, linked, mailbox); e != nil {
				return e
			}
			if _, e = tx.Exec(`INSERT INTO bm_webmail_bindings(mailbox,account_id,updated_by) VALUES($1,$2,$3) ON CONFLICT(mailbox) DO UPDATE SET account_id=excluded.account_id,updated_by=excluded.updated_by,updated_at=now()`, mailbox, accountID, p.ID); e != nil {
				return e
			}
		}
		return governance.Audit(ctx, p, "webmail_binding", accountID, g.Map{"mailbox": mailbox, "dept_id": deptID, "grant_department": grantApplied})
	})
}
func mailboxPrincipal(ctx context.Context, mailbox string) (*governance.Principal, error) {
	id, e := g.DB().Model("bm_webmail_bindings").Ctx(ctx).Where("mailbox", mailbox).Value("account_id")
	if e != nil {
		return nil, e
	}
	if id.IsEmpty() {
		return nil, errors.New("mailbox business account is not linked; configure Mailbox business access in the management console")
	}
	p, e := governance.Load(ctx, id.Int64())
	if e != nil {
		return nil, e
	}
	if e = governance.SenderAllowed(ctx, p, mailbox); e != nil {
		return nil, e
	}
	return p, nil
}
func Workbench(ctx context.Context, mailbox string, r Request) (g.Map, error) {
	p, e := mailboxPrincipal(ctx, mailbox)
	if e != nil {
		// Public model metadata is safe for an authenticated mailbox. This does not
		// grant AI execution, CRM access or inherit an administrator's privileges.
		if r.Action == "workbench_options" && strings.Contains(e.Error(), "not linked") {
			catalog, err := aimodels.Catalog(ctx, "text")
			if err != nil {
				return nil, err
			}
			catalog["enabled"] = false
			catalog["warning"] = "mailbox_business_access_required"
			return g.Map{"ai": false, "crm": false, "manage_customers": false, "followup": false, "authoring": catalog}, nil
		}
		return nil, e
	}
	d := r.Data
	if d == nil {
		d = g.Map{}
	}
	switch r.Action {
	case "workbench_options":
		result := g.Map{"crm": p.Has("crm:list"), "manage_customers": p.Has("crm:manage"), "followup": p.Has("crm:manage"), "ai": p.Has("campaign:create")}
		if p.Has("campaign:create") {
			o, err := growth.CampaignOptions(ctx, p)
			if err != nil {
				return nil, err
			}
			result["authoring"] = o
		}
		return result, nil
	case "workbench_ai":
		var brief growth.CampaignBrief
		if e = gconv.Scan(d, &brief); e != nil {
			return nil, e
		}
		return growth.ComposeCampaign(ctx, p, brief)
	case "workbench_templates":
		if !p.Has("campaign:create") {
			return nil, errors.New("campaign authoring permission required")
		}
		predicate, err := p.Predicate(ctx, "campaign:create", "")
		if err != nil {
			return nil, err
		}
		page := gconv.Int(d["page"])
		if page < 1 {
			page = 1
		}
		rows, err := g.DB().Model("email_templates").Ctx(ctx).Where(predicate).Where("temp_name ILIKE ?", "%"+gconv.String(d["keyword"])+"%").Fields("id,temp_name AS name").Order("id DESC").Page(page, 20).All()
		return g.Map{"list": rows}, err
	case "workbench_template":
		if !p.Has("campaign:create") {
			return nil, errors.New("campaign authoring permission required")
		}
		predicate, err := p.Predicate(ctx, "campaign:create", "")
		if err != nil {
			return nil, err
		}
		row, err := g.DB().Model("email_templates").Ctx(ctx).Where(predicate).Where("id", gconv.Int64(d["id"])).Fields("id,temp_name AS name,html_content").One()
		if err != nil {
			return nil, err
		}
		if row.IsEmpty() {
			return nil, errors.New("template unavailable")
		}
		return g.Map{"template": row}, nil
	case "workbench_customer":
		if !p.Has("crm:list") {
			return nil, errors.New("CRM read permission required")
		}
		email := strings.ToLower(strings.TrimSpace(gconv.String(d["email"])))
		parsed, err := mail.ParseAddress(email)
		if err != nil || parsed.Address != email {
			return nil, errors.New("valid customer email required")
		}
		predicate, err := p.Predicate(ctx, "crm:list", "")
		if err != nil {
			return nil, err
		}
		row, err := g.DB().Model("bm_crm_customers").Ctx(ctx).Where(predicate).WhereNull("deleted_at").Where("email", email).Fields("id,email,name,company,country,stage,owner_id,last_activity_at").One()
		if err != nil {
			return nil, err
		}
		out := g.Map{"customer": row}
		if !row.IsEmpty() {
			details, err := crm.Detail(ctx, p, row["id"].Int64())
			if err != nil {
				return nil, err
			}
			out["detail"] = details
		}
		return out, nil
	case "workbench_save_customer":
		idValue := gconv.Int64(d["id"])
		if idValue > 0 {
			prior, err := crm.Customer(ctx, p, idValue, "crm:manage")
			if err != nil {
				return nil, err
			}
			for _, key := range []string{"name", "company", "country", "note"} {
				if _, exists := d[key]; !exists {
					d[key] = prior[key].String()
				}
			}
			if _, exists := d["next_followup_at"]; !exists && !prior["next_followup_at"].IsEmpty() {
				d["next_followup_at"] = prior["next_followup_at"].Time().Format(time.RFC3339)
			}
		}
		id, err := crm.Save(ctx, p, idValue, d)
		return g.Map{"id": id}, err
	case "workbench_followup":
		return g.Map{}, crm.Followup(ctx, p, gconv.Int64(d["id"]), gconv.String(d["content"]))
	}
	return nil, errors.New("unknown mailbox workbench action")
}
