package contact

import (
	"billionmail-core/api/contact/v1"
	"billionmail-core/internal/consts"
	"billionmail-core/internal/model/entity"
	"billionmail-core/internal/service/contact"
	"billionmail-core/internal/service/contactimport"
	"billionmail-core/internal/service/governance"
	"billionmail-core/internal/service/public"
	"billionmail-core/internal/service/rbac"
	"context"
	"errors"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"strings"
)

func (c *ControllerV1) CreateGroup(ctx context.Context, req *v1.CreateGroupReq) (res *v1.CreateGroupRes, err error) {
	res = &v1.CreateGroupRes{}
	fail := func(e error) (*v1.CreateGroupRes, error) { res.Code = 400; res.SetError(e); return res, nil }
	principal, e := governance.Load(ctx, rbac.GetCurrentAccountId(ctx))
	if e != nil {
		return fail(e)
	}
	if !principal.Super {
		return fail(errors.New("Group administration access denied"))
	}
	keywords, e := public.NormalizeImportKeywords(req.FilterKeywords)
	if e != nil {
		return fail(e)
	}
	var contacts []*entity.Contact
	filtered := 0
	invalid, pending := 0, 0
	if req.CreateType == 2 || req.CreateType == 3 {
		if req.FileData != "" {
			if req.FileType != "xlsx" {
				return fail(errors.New("xlsx_required"))
			}
			scan, parseErr := contactimport.Confirmed(ctx, principal, req.FileData, keywords, req.ValidationSender, req.PreviewToken)
			e = parseErr
			if e != nil {
				return fail(e)
			}
			if e = scan.ValidateConfirmation(req.PreviewToken, req.SkipInvalid, req.KeepPending); e != nil {
				return fail(e)
			}
			contacts = scan.ContactsForImport(req.KeepPending)
			filtered = scan.Filtered
			invalid, pending = scan.Invalid, scan.Pending
			if len(contacts) == 0 && filtered == 0 {
				return fail(errors.New("No valid contacts found in file"))
			}
		} else if req.CreateType == 3 {
			return fail(errors.New("File content cannot be empty"))
		}
	}
	groupID, imported := req.GroupId, 0
	if req.CreateType == 1 || req.CreateType == 2 {
		req.Name = strings.TrimSpace(req.Name)
		if req.Name == "" {
			return fail(errors.New("Group name is required"))
		}
		exists, e := contact.CheckGroupNameExists(ctx, req.Name)
		if e != nil {
			return fail(e)
		}
		if exists {
			return fail(errors.New(public.LangCtx(ctx, "The group name already exists")))
		}
	} else {
		group, e := contact.GetGroup(ctx, groupID)
		if e != nil {
			return fail(e)
		}
		if groupID <= 0 || group == nil || group.Id == 0 {
			return fail(errors.New("Group does not exist"))
		}
	}
	e = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if req.CreateType == 1 || req.CreateType == 2 {
			var e error
			groupID, e = contact.CreateGroup(ctx, req.Name, req.Description, req.DoubleOptin)
			if e != nil {
				return e
			}
			if _, e = tx.Model("bm_contact_groups").Where("id", groupID).Data(g.Map{"dept_id": principal.DeptID, "owner_id": principal.ID}).Update(); e != nil {
				return e
			}
		}
		for _, item := range contacts {
			item.GroupId = groupID
			item.Status = 1
		}
		if len(contacts) > 0 {
			var e error
			imported, e = contact.BatchCreateContactsIgnoreDuplicate(ctx, contacts)
			return e
		}
		return nil
	})
	if e != nil {
		return fail(e)
	}
	res.Data = g.Map{"group_id": groupID, "imported_count": imported, "filtered_count": filtered, "invalid_count": invalid, "pending_count": pending}
	_ = public.WriteLog(ctx, public.LogParams{Type: consts.LOGTYPE.ContactsGroup, Log: "Created/imported group: " + req.Name, Data: res.Data})
	message := "Group created successfully"
	if req.CreateType == 2 && req.FileData != "" {
		message = "Group created and contacts imported successfully"
	}
	if req.CreateType == 3 {
		message = "Contacts imported to existing group successfully"
	}
	res.SetSuccess(public.LangCtx(ctx, message))
	return res, nil
}
