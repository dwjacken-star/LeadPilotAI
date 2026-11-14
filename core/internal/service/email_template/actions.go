package email_template

import (
	v1 "billionmail-core/api/email_template/v1"
	"billionmail-core/internal/service/governance"
	"context"
)

func templateActionFlags(ctx context.Context, items []*v1.EmailTemplate) error {
	if len(items) == 0 {
		return nil
	}
	p, e := governance.RequestPrincipal(ctx)
	if e != nil {
		return e
	}
	ids := make([]int, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.Id)
	}
	allowed := map[string]map[int]bool{}
	for _, code := range []string{"template:update", "template:delete"} {
		allowed[code] = map[int]bool{}
		if p != nil && !p.Has(code) {
			continue
		}
		q, e := scoped(ctx, code)
		if e != nil {
			return e
		}
		rows, e := q.WhereIn("id", ids).Fields("id").All()
		if e != nil {
			return e
		}
		for _, row := range rows {
			allowed[code][row["id"].Int()] = true
		}
	}
	copyAllowed := p == nil || (p.Has("template:create") && p.CheckOwnership(ctx, "template:create", p.DeptID, p.ID) == nil)
	for _, item := range items {
		item.CanUpdate = allowed["template:update"][item.Id]
		item.CanDelete = allowed["template:delete"][item.Id]
		item.CanCopy = copyAllowed
	}
	return nil
}
