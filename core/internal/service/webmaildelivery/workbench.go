package webmaildelivery

import (
	"billionmail-core/internal/service/aimodels"
	"billionmail-core/internal/service/crm"
	"billionmail-core/internal/service/governance"
	"billionmail-core/internal/service/growth"
	"context"
	"errors"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/gconv"
	"net/mail"
	"strings"
	"time"
)

// Linking is explicit. A mailbox password never becomes an administrator token.
func Bindings(ctx context.Context, p *governance.Principal, mailbox, keyword string, page int) (g.Map, error) {
	if !p.Super {
		return nil, errors.New("Administrator access required")
	}
	if page < 1 {
		page = 1
	}
	rows, e := g.DB().Model("account").Ctx(ctx).Where("status", 1).WhereLike("username", "%"+keyword+"%").Fields("account_id,username").Order("account_id").Page(page, 20).All()
	if e != nil {
		return nil, e
	}
	count, e := g.DB().Model("account").Ctx(ctx).Where("status", 1).WhereLike("username", "%"+keyword+"%").Count()
	if e != nil {
		return nil, e
	}
	binding, e := g.DB().Model("bm_webmail_bindings b").Ctx(ctx).LeftJoin("account a", "a.account_id=b.account_id").Where("b.mailbox", mailbox).Fields("b.mailbox,b.account_id,a.username").One()
	return g.Map{"accounts": rows, "total": count, "binding": binding}, e
}
func SaveBinding(ctx context.Context, p *governance.Principal, mailbox string, accountID int64) error {
	if !p.Super {
		return errors.New("Administrator access required")
	}
	mailbox = strings.ToLower(strings.TrimSpace(mailbox))
	n, e := g.DB().Model("mailbox").Ctx(ctx).Where("username", mailbox).Where("active", 1).Count()
	if e != nil {
		return e
	}
	if n != 1 {
		return errors.New("active mailbox required")
	}
	if accountID == 0 {
		_, e = g.DB().Model("bm_webmail_bindings").Ctx(ctx).Where("mailbox", mailbox).Delete()
	} else {
		linked, err := governance.Load(ctx, accountID)
		if err != nil {
			return err
		}
		if err = governance.SenderAllowed(ctx, linked, mailbox); err != nil {
			return err
		}
		_, e = g.DB().Exec(ctx, `INSERT INTO bm_webmail_bindings(mailbox,account_id,updated_by) VALUES($1,$2,$3) ON CONFLICT(mailbox) DO UPDATE SET account_id=excluded.account_id,updated_by=excluded.updated_by,updated_at=now()`, mailbox, accountID, p.ID)
	}
	if e != nil {
		return e
	}
	return governance.Audit(ctx, p, "webmail_binding", accountID, g.Map{"mailbox": mailbox})
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
