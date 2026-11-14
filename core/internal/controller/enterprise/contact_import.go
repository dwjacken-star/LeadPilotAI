package enterprise

import (
	"billionmail-core/internal/service/contactimport"
	"billionmail-core/internal/service/governance"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/gconv"
)

func workbookRows(scan *contactimport.Result, group, dept, owner int64, confirmed bool) []g.Map {
	rows := []g.Map{}
	for _, item := range scan.Contacts {
		rows = append(rows, g.Map{"email": item.Email, "group_id": group, "dept_id": dept, "owner_id": owner, "active": item.Active, "status": gconv.Int(confirmed), "attribs": jsonText(item.Attribs)})
	}
	return rows
}
func insertWorkbookRows(ctx context.Context, tx gdb.TX, rows []g.Map) (int64, error) {
	var inserted int64
	for start := 0; start < len(rows); start += 500 {
		end := start + 500
		if end > len(rows) {
			end = len(rows)
		}
		result, e := tx.Model("bm_contacts").OnConflict("group_id,email").InsertIgnore(rows[start:end])
		if e != nil {
			return 0, e
		}
		n, _ := result.RowsAffected()
		inserted += n
	}
	return inserted, nil
}
func readImport(ctx context.Context, p *governance.Principal, r *Request, data string) (*contactimport.Result, error) {
	var scan *contactimport.Result
	var e error
	if r.Action=="preview_import" {scan,e=contactimport.ReadForPrincipal(ctx,p,data,gconv.Strings(r.Data["filter_keywords"]),text(r.Data,"validation_sender"))} else {scan,e=contactimport.Confirmed(ctx,p,data,gconv.Strings(r.Data["filter_keywords"]),text(r.Data,"validation_sender"),text(r.Data,"preview_token"))}
	if e != nil {
		return nil, e
	}
	if r.Action != "preview_import" {
		if e = scan.ValidateConfirmation(gconv.String(r.Data["preview_token"]), gconv.Bool(r.Data["skip_invalid"]), gconv.Bool(r.Data["keep_pending"])); e != nil {
			return nil, e
		}
		scan.Contacts = scan.ContactsForImport(gconv.Bool(r.Data["keep_pending"]))
	}
	return scan, nil
}
func importWorkbook(ctx context.Context, p *governance.Principal, r *Request, group, dept int64, data string) (interface{}, error) {
	scan, e := readImport(ctx, p, r, data)
	if e != nil {
		return nil, e
	}
	var inserted int64
	if r.Action == "import" {
		e = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
			var e error
			inserted, e = insertWorkbookRows(ctx, tx, workbookRows(scan, group, dept, p.ID, gconv.Bool(r.Data["confirmed"])))
			return e
		})
	}
	invalid := []contactimport.Row{}
	for _, row := range scan.Rows {
		if row.Status == "invalid" {
			invalid = append(invalid, row)
		}
	}
	return g.Map{"valid": scan.Valid, "invalid": invalid, "pending": scan.Pending, "filtered_count": scan.Filtered, "duplicate_count": scan.Duplicate, "inserted": inserted}, e
}
func createGroupImport(ctx context.Context, p *governance.Principal, r *Request) (interface{}, error) {
	if !p.Has("contact:create") || !p.Has("contact:import") {
		return nil, errors.New("Import access denied")
	}
	if e := p.CheckOwnership(ctx, "contact:create", p.DeptID, p.ID); e != nil {
		return nil, e
	}
	name := text(r.Data, "name")
	if name == "" {
		return nil, errors.New("Group name is required")
	}
	scan, e := readImport(ctx, p, r, gconv.String(r.Data["file_data"]))
	if e != nil {
		return nil, e
	}
	token := make([]byte, 12)
	if _, e = rand.Read(token); e != nil {
		return nil, e
	}
	var id, inserted int64
	e = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		count, e := tx.Model("bm_contact_groups").Where("dept_id", p.DeptID).Where("name", name).Count()
		if e != nil {
			return e
		}
		if count > 0 {
			return errors.New("Group name already exists")
		}
		id, e = tx.Model("bm_contact_groups").Data(g.Map{"name": name, "description": text(r.Data, "description"), "dept_id": p.DeptID, "owner_id": p.ID, "token": hex.EncodeToString(token)}).InsertAndGetId()
		if e != nil {
			return e
		}
		inserted, e = insertWorkbookRows(ctx, tx, workbookRows(scan, id, p.DeptID, p.ID, true))
		return e
	})
	return g.Map{"id": id, "imported_count": inserted, "filtered_count": scan.Filtered, "invalid_count": scan.Invalid}, e
}
