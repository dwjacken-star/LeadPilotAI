package email_template

import (
	v1 "billionmail-core/api/email_template/v1"
	"billionmail-core/internal/service/askai"
	"billionmail-core/internal/service/governance"
	"context"
	"errors"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"regexp"
	"strings"
	"time"
)

var chatIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

func scoped(ctx context.Context, permission string) (*gdb.Model, error) {
	return governance.ScopeRequestModel(ctx, g.DB().Model("email_templates"), permission, "")
}
func CheckTemplateNameExists(ctx context.Context, name string) (bool, error) {
	q := g.DB().Model("email_templates").Ctx(ctx).Where("temp_name", name)
	p, e := governance.RequestPrincipal(ctx)
	if e != nil {
		return false, e
	}
	if p != nil {
		q = q.Where("dept_id", p.DeptID)
	}
	n, e := q.Count()
	return n > 0, e
}
func CreateTemplate(ctx context.Context, name string, addType int, content, render, chatID string) (int, error) {
	p, e := governance.RequestPrincipal(ctx)
	if e != nil {
		return 0, e
	}
	data := g.Map{"temp_name": name, "add_type": addType, "content": content, "render": render, "chat_id": chatID, "create_time": time.Now().Unix(), "update_time": time.Now().Unix()}
	if p != nil {
		if !p.Has("template:create") {
			return 0, errors.New("permission required: template:create")
		}
		if e = p.CheckOwnership(ctx, "template:create", p.DeptID, p.ID); e != nil {
			return 0, e
		}
		data["dept_id"], data["owner_id"] = p.DeptID, p.ID
		if chatID != "" {
			if !chatIDPattern.MatchString(chatID) {
				return 0, errors.New("invalid template chat ID")
			}
			if !p.Super {
				temp, err := askai.GetTempChat(chatID)
				if err != nil || temp.OwnerID != p.ID || temp.DeptID != p.DeptID {
					return 0, errors.New("template chat unavailable")
				}
				n, err := g.DB().Model("email_templates").Ctx(ctx).Where("chat_id", chatID).Count()
				if err != nil {
					return 0, err
				}
				if n != 0 {
					return 0, errors.New("template chat already attached")
				}
			}
		}
	}
	id, e := g.DB().Model("email_templates").Ctx(ctx).Data(data).InsertAndGetId()
	return int(id), e
}
func DeleteTemplate(ctx context.Context, id int) error {
	q, e := scoped(ctx, "template:delete")
	if e != nil {
		return e
	}
	r, e := q.Where("id", id).Delete()
	if e != nil {
		return e
	}
	n, e := r.RowsAffected()
	if e == nil && n == 0 {
		return errors.New("template unavailable")
	}
	return e
}
func GetTemplate(ctx context.Context, id int) (*v1.EmailTemplate, error) {
	q, e := scoped(ctx, "template:list")
	if e != nil {
		return nil, e
	}
	var item *v1.EmailTemplate
	e = q.Where("id", id).Scan(&item)
	return item, e
}
func GetTemplatesByID(ctx context.Context, id int) (*v1.EmailTemplate, error) {
	return GetTemplate(ctx, id)
}
func UpdateTemplate(ctx context.Context, id int, name, content, render string) error {
	q, e := scoped(ctx, "template:update")
	if e != nil {
		return e
	}
	data := g.Map{"update_time": time.Now().Unix()}
	if name != "" {
		data["temp_name"] = name
	}
	if content != "" {
		data["content"] = content
	}
	if render != "" {
		data["render"] = render
	}
	result, e := q.Where("id", id).Data(data).Update()
	if e != nil {
		return e
	}
	n, e := result.RowsAffected()
	if e == nil && n == 0 {
		return errors.New("template unavailable")
	}
	return e
}
func GetTemplatesWithPage(ctx context.Context, page, pageSize int, keyword string, addType int) (total int, list []*v1.EmailTemplate, err error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}
	if pageSize > 200 {
		pageSize = 200
	}
	q, err := scoped(ctx, "template:list")
	if err != nil {
		return 0, nil, err
	}
	q = q.Safe()
	if keyword != "" {
		q = q.WhereLike("temp_name", "%"+keyword+"%")
	}
	if addType != -1 {
		q = q.Where("add_type", addType)
	}
	total, err = q.Count()
	if err != nil {
		return 0, nil, err
	}
	list = []*v1.EmailTemplate{}
	err = q.Order("create_time DESC,id DESC").Page(page, pageSize).Scan(&list)
	if err == nil {
		err = templateActionFlags(ctx, list)
	}
	return
}
func GetTemplatesAll(ctx context.Context) ([]*v1.EmailTemplate, error) {
	q, e := scoped(ctx, "template:list")
	if e != nil {
		return nil, e
	}
	items := []*v1.EmailTemplate{}
	e = q.Order("create_time DESC,id DESC").Scan(&items)
	return items, e
}

// A shared legacy chat can touch several rows. Every associated row must be
// within the caller's operation-bound scope before any file access/generation.
func CheckChat(ctx context.Context, chatID, permission string) error {
	if !chatIDPattern.MatchString(chatID) {
		return errors.New("invalid template chat ID")
	}
	p, e := governance.RequestPrincipal(ctx)
	if e != nil {
		return e
	}
	if p == nil {
		return errors.New("authentication required")
	}
	if !p.Has(permission) {
		return errors.New("permission required: " + permission)
	}
	if p.Super {
		return nil
	}
	predicate, e := p.Predicate(ctx, permission, "")
	if e != nil {
		return e
	}
	total, e := g.DB().Model("email_templates").Ctx(ctx).Where("chat_id", chatID).Count()
	if e != nil {
		return e
	}
	allowed, e := g.DB().Model("email_templates").Ctx(ctx).Where("chat_id", chatID).Where(predicate).Count()
	if e != nil {
		return e
	}
	if total == 0 || allowed != total {
		return errors.New("template chat unavailable")
	}
	return nil
}
func CreationOptions(ctx context.Context) (g.Map, error) {
	p, e := governance.RequestPrincipal(ctx)
	if e != nil {
		return nil, e
	}
	if p == nil || !p.Has("template:create") {
		return nil, errors.New("permission required: template:create")
	}
	q := g.DB().Model("domain").Ctx(ctx)
	if !p.Super {
		q = q.Where(`domain IN(SELECT m.domain FROM mailbox m JOIN bm_resource_grants g ON g.sender=m.username WHERE g.dept_id=? AND g.active AND m.active=1)`, p.DeptID)
	}
	fields := "domain,hasbrandinfo"
	if p.Super {
		fields += ",urls"
	}
	domains, e := q.Fields(fields).Order("domain").All()
	if e != nil {
		return nil, e
	}
	// Only non-secret, public brand-domain metadata; global settings remain closed.
	if domains == nil {
		domains = gdb.Result{}
	}
	return g.Map{"domains": domains, "is_configured": len(askai.Models("")) > 0, "can_ai": p.Has("template:update") && p.CheckOwnership(ctx, "template:update", p.DeptID, p.ID) == nil, "can_configure": p.Super}, nil
}
func ValidateDomain(ctx context.Context, domain string) error {
	if strings.ContainsAny(domain, "/\\") || domain == "" {
		return errors.New("valid template domain required")
	}
	p, e := governance.RequestPrincipal(ctx)
	if e != nil {
		return e
	}
	q := g.DB().Model("domain").Ctx(ctx).Where("domain", domain).Where("hasbrandinfo", 1)
	if p != nil && !p.Super {
		q = q.Where(`domain IN(SELECT m.domain FROM mailbox m JOIN bm_resource_grants g ON g.sender=m.username WHERE g.dept_id=? AND g.active AND m.active=1)`, p.DeptID)
	}
	n, e := q.Count()
	if e != nil {
		return e
	}
	if n != 1 {
		return errors.New("approved authorized brand domain required")
	}
	return nil
}
