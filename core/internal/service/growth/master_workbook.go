package growth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"billionmail-core/internal/service/governance"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/google/uuid"
	"github.com/xuri/excelize/v2"
)

func MasterWorkbookTemplate(ctx context.Context, p *governance.Principal, cid, packID int64) ([]byte, error) {
	if _, e := company(ctx, p, cid, "material:upload", false); e != nil {
		return nil, e
	}
	_, pack, e := masterPack(ctx, p, packID)
	if e != nil {
		return nil, e
	}
	b := excelize.NewFile()
	defer b.Close()
	headers := []interface{}{"SKU", "Name", "Category"}
	for _, f := range pack.Fields {
		headers = append(headers, f.Key)
	}
	if e = b.SetSheetRow("Sheet1", "A1", &headers); e != nil {
		return nil, e
	}
	style, e := b.NewStyle(&excelize.Style{NumFmt: 49})
	if e != nil {
		return nil, e
	}
	end, _ := excelize.ColumnNumberToName(len(headers))
	_ = b.SetColStyle("Sheet1", "A:"+end, style)
	_ = b.SetColWidth("Sheet1", "A", end, 24)
	_, _ = b.NewSheet("Instructions")
	notes := [][]interface{}{{"Field", "Label", "Type", "Required", "Unit", "Options"}}
	for _, f := range pack.Fields {
		notes = append(notes, []interface{}{f.Key, f.Label, f.Type, f.Required, f.Unit, strings.Join(f.Options, " | ")})
	}
	for i, row := range notes {
		if e = b.SetSheetRow("Instructions", fmt.Sprintf("A%d", i+1), &row); e != nil {
			return nil, e
		}
	}
	buf, e := b.WriteToBuffer()
	if e != nil {
		return nil, e
	}
	return buf.Bytes(), nil
}

type masterImportRow struct {
	Row      int      `json:"row"`
	SKU      string   `json:"sku"`
	Name     string   `json:"name"`
	Category string   `json:"category"`
	Fields   g.Map    `json:"fields"`
	Errors   []string `json:"errors"`
	State    string   `json:"state"`
}
type masterImportPreview struct {
	Token       string            `json:"token"`
	CompanyID   int64             `json:"company_id"`
	PackID      int64             `json:"pack_id"`
	PackVersion int               `json:"pack_version"`
	Rows        []masterImportRow `json:"rows"`
	Valid       int               `json:"valid"`
	Invalid     int               `json:"invalid"`
	Duplicate   int               `json:"duplicate"`
}

func MasterPreviewWorkbook(ctx context.Context, p *governance.Principal, cid, packID int64, name string, reader io.Reader) (g.Map, error) {
	if _, e := company(ctx, p, cid, "product:import", false); e != nil {
		return nil, e
	}
	if !strings.HasSuffix(strings.ToLower(name), ".xlsx") {
		return nil, errors.New("only XLSX workbooks are supported")
	}
	record, pack, e := masterPack(ctx, p, packID)
	if e != nil {
		return nil, e
	}
	raw, e := io.ReadAll(io.LimitReader(reader, 10*1024*1024+1))
	if e != nil || len(raw) > 10*1024*1024 {
		return nil, errors.New("XLSX import exceeds 10 MB")
	}
	book, e := excelize.OpenReader(strings.NewReader(string(raw)), excelize.Options{UnzipSizeLimit: 64 * 1024 * 1024, UnzipXMLSizeLimit: 16 * 1024 * 1024})
	if e != nil {
		return nil, errors.New("invalid XLSX workbook")
	}
	defer book.Close()
	preview := masterImportPreview{Token: uuid.NewString(), CompanyID: cid, PackID: packID, PackVersion: record["version"].Int(), Rows: []masterImportRow{}}
	rows, e := book.GetRows(book.GetSheetName(0))
	if e != nil {
		return nil, e
	}
	if len(rows) > 10001 {
		return nil, errors.New("at most 10000 rows per import")
	}
	if len(rows) < 1 {
		return nil, errors.New("workbook has no header")
	}
	headers := map[string]int{}
	for i, key := range rows[0] {
		key = strings.TrimSpace(key)
		if _, exists := headers[key]; exists {
			return nil, errors.New("duplicate column header")
		}
		headers[key] = i
	}
	for _, key := range []string{"SKU", "Name", "Category"} {
		if _, ok := headers[key]; !ok {
			return nil, fmt.Errorf("missing %s column", key)
		}
	}
	for _, f := range pack.Fields {
		if f.Required {
			if _, ok := headers[f.Key]; !ok {
				return nil, fmt.Errorf("missing required %s column", f.Key)
			}
		}
	}
	seen := map[string]bool{}
	existing, e := g.DB().Model("bm_growth_products").Ctx(ctx).Where("company_id", cid).Fields("sku").All()
	if e != nil {
		return nil, e
	}
	for _, r := range existing {
		seen[r["sku"].String()] = true
	}
	for i, values := range rows[1:] {
		get := func(key string) string {
			n, ok := headers[key]
			if !ok || n >= len(values) {
				return ""
			}
			return strings.TrimSpace(values[n])
		}
		r := masterImportRow{Row: i + 2, SKU: get("SKU"), Name: get("Name"), Category: get("Category"), Fields: g.Map{}, Errors: []string{}, State: "valid"}
		if r.SKU == "" && r.Name == "" {
			continue
		}
		if r.SKU == "" || r.Name == "" {
			r.Errors = append(r.Errors, "SKU and Name required")
		}
		if len(r.SKU) > 128 || len(r.Name) > 300 {
			r.Errors = append(r.Errors, "SKU or Name too long")
		}
		for _, f := range pack.Fields {
			value := get(f.Key)
			if value == "" {
				continue
			}
			var v interface{} = value
			switch f.Type {
			case "number":
				n, e := strconv.ParseFloat(value, 64)
				if e != nil {
					r.Errors = append(r.Errors, f.Key+": invalid number")
				} else {
					v = n
				}
			case "boolean":
				low := strings.ToLower(value)
				if low == "true" || low == "1" || low == "yes" {
					v = true
				} else if low == "false" || low == "0" || low == "no" {
					v = false
				} else {
					r.Errors = append(r.Errors, f.Key+": invalid boolean")
				}
			case "list":
				parts := strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == '，' || r == ';' || r == '；' || r == '\n' })
				list := []string{}
				for _, part := range parts {
					if s := strings.TrimSpace(part); s != "" {
						list = append(list, s)
					}
				}
				v = list
			}
			r.Fields[f.Key] = v
		}
		r.Errors = append(r.Errors, masterFieldErrors(pack, r.Fields)...)
		if seen[r.SKU] {
			r.State = "duplicate"
			preview.Duplicate++
		} else if len(r.Errors) > 0 {
			r.State = "invalid"
			preview.Invalid++
		} else {
			preview.Valid++
		}
		seen[r.SKU] = true
		preview.Rows = append(preview.Rows, r)
	}
	_, e = g.DB().Model("bm_gm_import_previews").Ctx(ctx).Data(g.Map{"token": preview.Token, "company_id": cid, "dept_id": p.DeptID, "owner_id": p.ID, "preview": jsonText(preview), "expires_at": time.Now().Add(time.Hour)}).Insert()
	if e != nil {
		return nil, e
	}
	return g.Map{"token": preview.Token, "valid": preview.Valid, "invalid": preview.Invalid, "duplicate": preview.Duplicate, "total": len(preview.Rows), "rows": preview.Rows[:min(len(preview.Rows), 100)]}, nil
}
func masterConfirmImport(ctx context.Context, p *governance.Principal, token string) (g.Map, error) {
	if e := require(p, "product:import"); e != nil {
		return nil, e
	}
	count := 0
	e := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		r, e := tx.GetOne(`SELECT * FROM bm_gm_import_previews WHERE token=$1 AND dept_id=$2 AND owner_id=$3 AND expires_at>now() FOR UPDATE`, token, p.DeptID, p.ID)
		if e != nil {
			return e
		}
		if len(r) == 0 {
			return errors.New("preview expired or access denied")
		}
		if !r["imported_at"].IsNil() {
			count = r["imported_count"].Int()
			return nil
		}
		var preview masterImportPreview
		if e = json.Unmarshal(r["preview"].Bytes(), &preview); e != nil {
			return e
		}
		if _, e = company(ctx, p, preview.CompanyID, "product:manage", false); e != nil {
			return e
		}
		record, pack, e := masterPack(ctx, p, preview.PackID)
		if e != nil {
			return e
		}
		if record["version"].Int() != preview.PackVersion {
			return errors.New("industry configuration changed; preview the workbook again")
		}
		for _, row := range preview.Rows {
			if row.State != "valid" {
				continue
			}
			if len(masterFieldErrors(pack, row.Fields)) > 0 {
				return errors.New("import field validation failed")
			}
			exists, e := tx.Model("bm_growth_products").Where("company_id", preview.CompanyID).Where("sku", row.SKU).Count()
			if e != nil {
				return e
			}
			if exists > 0 {
				continue
			}
			_, e = SaveProduct(ctx, p, 0, g.Map{"company_id": preview.CompanyID, "industry_pack_id": preview.PackID, "sku": row.SKU, "name": row.Name, "category": row.Category, "fields": row.Fields})
			if e != nil {
				return e
			}
			count++
		}
		_, e = tx.Model("bm_gm_import_previews").Where("token", token).Data(g.Map{"imported_at": time.Now(), "imported_count": count}).Update()
		return e
	})
	return g.Map{"imported": count, "published": false}, e
}
func masterImportRows(ctx context.Context, p *governance.Principal, token string, page int) (g.Map, error) {
	if e := require(p, "product:import"); e != nil {
		return nil, e
	}
	r, e := g.DB().Model("bm_gm_import_previews").Ctx(ctx).Where("token", token).Where("owner_id", p.ID).Where("dept_id", p.DeptID).Where("expires_at>now()").One()
	if e != nil {
		return nil, e
	}
	if len(r) == 0 {
		return nil, errors.New("preview unavailable")
	}
	var preview masterImportPreview
	if e = json.Unmarshal(r["preview"].Bytes(), &preview); e != nil {
		return nil, e
	}
	if page < 1 {
		page = 1
	}
	start := min((page-1)*100, len(preview.Rows))
	end := min(start+100, len(preview.Rows))
	return g.Map{"rows": preview.Rows[start:end], "total": len(preview.Rows)}, nil
}
func masterImportInvalid(ctx context.Context, p *governance.Principal, token string) ([]byte, error) {
	r, e := g.DB().Model("bm_gm_import_previews").Ctx(ctx).Where("token", token).Where("owner_id", p.ID).Where("dept_id", p.DeptID).Where("expires_at>now()").One()
	if e != nil {
		return nil, e
	}
	if len(r) == 0 {
		return nil, errors.New("preview unavailable")
	}
	var preview masterImportPreview
	if e = json.Unmarshal(r["preview"].Bytes(), &preview); e != nil {
		return nil, e
	}
	b := excelize.NewFile()
	defer b.Close()
	header := []interface{}{"Row", "SKU", "Name", "State", "Reason", "Fields"}
	_ = b.SetSheetRow("Sheet1", "A1", &header)
	n := 2
	for _, r := range preview.Rows {
		if r.State == "valid" {
			continue
		}
		row := []interface{}{r.Row, r.SKU, r.Name, r.State, strings.Join(r.Errors, "; "), jsonText(r.Fields)}
		_ = b.SetSheetRow("Sheet1", fmt.Sprintf("A%d", n), &row)
		n++
	}
	buf, e := b.WriteToBuffer()
	if e != nil {
		return nil, e
	}
	return buf.Bytes(), nil
}
