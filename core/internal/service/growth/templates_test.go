package growth

import (
	"archive/zip"
	"bytes"
	"context"
	"testing"

	"billionmail-core/internal/service/governance"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/xuri/excelize/v2"
)

func exerciseUploadTemplateRoundTrip(t *testing.T) {
	ctx := context.Background()
	t.Setenv("MAPLEMAIL_GROWTH_DATA", t.TempDir())
	p, e := governance.Load(ctx, 1)
	must(t, e)
	id, e := SaveCompany(ctx, p, 0, g.Map{"name": "Template test", "profile": g.Map{}})
	must(t, e)
	old := IntelligenceProvider
	IntelligenceProvider = fakeIntelligence{}
	defer func() { IntelligenceProvider = old }()
	body, mime, name, e := UploadTemplate(ctx, p, id, "products", "zh")
	must(t, e)
	if mime != "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" || name != "MapleMail-products-upload-template.xlsx" {
		t.Fatal("invalid template response", mime, name)
	}
	book, e := excelize.OpenReader(bytes.NewReader(body))
	must(t, e)
	defer book.Close()
	headers, e := book.GetRows("Sheet1")
	must(t, e)
	if len(headers) != 1 || headers[0][0] != "SKU" || headers[0][1] != "Name" {
		t.Fatal("unexpected data in blank template", headers)
	}
	asset, e := saveBlob(ctx, p, id, "blank.xlsx", "", "products", bytes.NewReader(body), false)
	must(t, e)
	must(t, ingestSheet(ctx, p, asset))
	if n, e := g.DB().Model("bm_growth_products").Where("company_id", id).Count(); e != nil || n != 0 {
		t.Fatal("instructions or examples became products", n, e)
	}
	row := []interface{}{"000001", "Verified compressor", "A/C Parts", "0012345", "Verified model", "12V", "R134a", "10", "images/000001-main.jpg", "USD", "100.00", "Verified information only"}
	must(t, book.SetSheetRow("Sheet1", "A2", &row))
	buffer, e := book.WriteToBuffer()
	must(t, e)
	asset, e = saveBlob(ctx, p, id, "filled.xlsx", "", "products", bytes.NewReader(buffer.Bytes()), false)
	must(t, e)
	must(t, ingestSheet(ctx, p, asset))
	products, e := g.DB().Model("bm_growth_products").Where("company_id", id).All()
	must(t, e)
	if len(products) != 1 || products[0]["sku"].String() != "000001" || jsonMap(products[0]["fields"])["oe"] != "0012345" {
		t.Fatal("template did not round trip leading zeros", products)
	}
	if n, e := g.DB().Model("bm_growth_import_rows").Where("asset_id", asset["id"]).Count(); e != nil || n != 1 {
		t.Fatal("instructions were imported as rows", n, e)
	}
	for _, language := range []string{"zh", "en", "ja"} {
		data, _, _, e := UploadTemplate(ctx, p, id, "company", language)
		must(t, e)
		company, e := excelize.OpenReader(bytes.NewReader(data))
		must(t, e)
		rows, e := company.GetRows("Sheet1")
		must(t, e)
		company.Close()
		if len(rows) != 15 {
			t.Fatal("missing company fields", language, len(rows))
		}
		data, mime, _, e = UploadTemplate(ctx, p, id, "images", language)
		must(t, e)
		if mime != "application/zip" {
			t.Fatal(mime)
		}
		archive, e := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		must(t, e)
		if len(archive.File) != 2 || archive.File[0].Name != "images/" || archive.File[1].Name != "README.txt" {
			t.Fatal("unexpected image template archive")
		}
	}
	outside := &governance.Principal{ID: 2, DeptID: 88, Roles: []governance.Role{{Scope: "department", Permissions: []string{"material:upload", "product:import"}}}}
	if _, _, _, e := UploadTemplate(ctx, outside, id, "products", "zh"); e == nil {
		t.Fatal("cross-department template access allowed")
	}
	if _, _, _, e := UploadTemplate(ctx, nil, id, "products", "zh"); e == nil {
		t.Fatal("unauthenticated template access allowed")
	}
}
