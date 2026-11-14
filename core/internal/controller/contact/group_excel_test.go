package contact

import (
	v1 "billionmail-core/api/contact/v1"
	"billionmail-core/internal/service/contactimport"
	"billionmail-core/internal/service/emailvalidation"
	"billionmail-core/internal/service/governance"
	"billionmail-core/internal/service/public"
	"billionmail-core/internal/testsupport"
	"bytes"
	"context"
	"encoding/base64"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/gconv"
	"github.com/xuri/excelize/v2"
	"net"
	"strings"
	"testing"
)

type previewDNSResolver struct{}

func (previewDNSResolver) LookupMX(_ context.Context, host string) ([]*net.MX, error) {
	if host == "temporary.test" {
		return nil, &net.DNSError{Err: "SERVFAIL", IsTemporary: true}
	}
	return []*net.MX{{Host: "mx.fixture.test."}}, nil
}

func (previewDNSResolver) LookupIPAddr(context.Context, string) ([]net.IPAddr, error) {
	return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}, nil
}

func exercisePreviewImportFiltersPendingWithoutImporting(t *testing.T, ctx context.Context) {
	t.Helper()
	previous := emailvalidation.Default
	engine := emailvalidation.New(previewDNSResolver{})
	engine.Probe = func(_ context.Context, _ emailvalidation.Result, email, sender string) emailvalidation.Result {
		status, code := emailvalidation.Valid, "smtp_accepted"
		if strings.HasPrefix(email, "missing@") {
			status, code = emailvalidation.Invalid, "recipient_not_found"
		}
		if strings.HasPrefix(email, "catch@") {
			status, code = emailvalidation.Unknown, "catch_all"
		}
		return emailvalidation.Result{Status: status, Code: code, Stage: "smtp"}
	}
	emailvalidation.Default = engine
	defer func() { emailvalidation.Default = previous }()
	book := excelize.NewFile()
	defer book.Close()
	for i, email := range []string{"email", "buyer@route.test", "later@temporary.test", "missing@route.test", "catch@catch.test", "invalid-address", "spam@route.test", "buyer@route.test"} {
		cell, _ := excelize.CoordinatesToCellName(1, i+1)
		values := []interface{}{email}
		if e := book.SetSheetRow("Sheet1", cell, &values); e != nil {
			t.Fatal(e)
		}
	}
	raw, e := book.WriteToBuffer()
	if e != nil {
		t.Fatal(e)
	}
	file := base64.StdEncoding.EncodeToString(raw.Bytes())
	controller := &ControllerV1{}
	preview, e := controller.PreviewImport(ctx, &v1.PreviewImportReq{FileData: file, FilterKeywords: []string{"spam"}, Status: "pending"})
	if e != nil || !preview.Success {
		t.Fatal("pending preview failed", e, preview)
	}
	data := gconv.Map(preview.Data)
	rows := gconv.Maps(data["rows"])
	if gconv.Int(data["valid"]) != 1 || gconv.Int(data["invalid"]) != 2 || gconv.Int(data["pending"]) != 2 || gconv.Int(data["row_total"]) != 2 || len(rows) != 2 {
		t.Fatal("deep import checks lost classifications", data)
	}
	token := gconv.String(data["token"])
	created, e := controller.CreateGroup(ctx, &v1.CreateGroupReq{Name: "verified-only", CreateType: 2, FileType: "xlsx", FileData: file, FilterKeywords: []string{"spam"}, PreviewToken: token})
	if e != nil || !created.Success || gconv.Int(gconv.Map(created.Data)["imported_count"]) != 1 {
		t.Fatal("unchecked imports only verified contacts", e, created)
	}
	group := gconv.Int(gconv.Map(created.Data)["group_id"])
	emails, e := g.DB().Model("bm_contacts").Where("group_id", group).Array("email")
	if e != nil || len(emails) != 1 || emails[0].String() != "buyer@route.test" {
		t.Fatal("invalid or pending contact reached group", emails, e)
	}
	withPending, e := controller.CreateGroup(ctx, &v1.CreateGroupReq{Name: "explicit-pending", CreateType: 2, FileType: "xlsx", FileData: file, FilterKeywords: []string{"spam"}, PreviewToken: token, KeepPending: true})
	if e != nil || !withPending.Success || gconv.Int(gconv.Map(withPending.Data)["imported_count"]) != 3 {
		t.Fatal("checked pending import failed", withPending, e)
	}
	selectedGroup := gconv.Int(gconv.Map(withPending.Data)["group_id"])
	selected, e := g.DB().Model("bm_contacts").Where("group_id", selectedGroup).Order("email").Array("email")
	if e != nil || len(selected) != 3 || selected[0].String() != "buyer@route.test" || selected[1].String() != "catch@catch.test" || selected[2].String() != "later@temporary.test" {
		t.Fatal("pending choice admitted invalid, filtered or duplicate rows", selected, e)
	}
	scan, e := contactimport.ReadForPrincipal(ctx, mustImportPrincipal(t, ctx), file, []string{"spam"}, "")
	if e != nil {
		t.Fatal(e)
	}
	pending, e := scan.StatusWorkbook("pending")
	if e != nil {
		t.Fatal(e)
	}
	exported, e := excelize.OpenReader(bytes.NewReader(pending))
	if e != nil {
		t.Fatal(e)
	}
	defer exported.Close()
	downloaded, _ := exported.GetRows(exported.GetSheetName(0))
	if len(downloaded) != 3 {
		t.Fatal("pending download must preserve both addresses", downloaded)
	}
	// A later poll may validate more rows. Confirmation must still import exactly
	// the previously reviewed set instead of expanding it or rejecting its proof.
	emailvalidation.Default = previous
	newPreview, e := controller.PreviewImport(ctx, &v1.PreviewImportReq{FileData: file, FilterKeywords: []string{"spam"}})
	if e != nil || !newPreview.Success || gconv.Int(gconv.Map(newPreview.Data)["valid"]) <= 1 {
		t.Fatal("fixture did not advance verification", newPreview, e)
	}
	approved, e := contactimport.Confirmed(ctx, mustImportPrincipal(t, ctx), file, []string{"spam"}, "", token)
	if e != nil || len(approved.Contacts) != 1 {
		t.Fatal("background checks invalidated or expanded approved preview", approved, e)
	}
}
func mustImportPrincipal(t *testing.T, ctx context.Context) *governance.Principal {
	t.Helper()
	p, e := governance.Load(ctx, 1)
	if e != nil {
		t.Fatal(e)
	}
	return p
}

func TestCreateGroupImportsExcelAndFilters(t *testing.T) {
	testsupport.Database(t)
	ctx := context.WithValue(context.Background(), "accountId", int64(1))
	data, e := public.BuildImportWorkbook(false, "zh")
	if e != nil {
		t.Fatal(e)
	}
	encoded := base64.StdEncoding.EncodeToString(data)
	controller := &ControllerV1{}
	for _, tc := range []struct {
		name, fileType, fileData string
		keywords                 []string
		success                  bool
		imported, filtered       int
	}{
		{"xlsx-list", "xlsx", encoded, nil, true, 1, 0},
		{"filtered-list", "xlsx", encoded, []string{"BUYER", "spam"}, true, 0, 1},
		{"bad-xlsx", "xlsx", "invalid-workbook", nil, false, 0, 0},
		{"csv-rejected", "csv", "email\nbuyer@example.com", nil, false, 0, 0},
	} {
		res, e := controller.CreateGroup(ctx, &v1.CreateGroupReq{Name: tc.name, CreateType: 2, FileType: tc.fileType, FileData: tc.fileData, FilterKeywords: tc.keywords})
		if e != nil || res.Success != tc.success {
			t.Fatalf("%s: %v %+v", tc.name, e, res)
		}
		count, e := g.DB().Model("bm_contact_groups").Ctx(ctx).Where("name", tc.name).Count()
		if e != nil {
			t.Fatal(e)
		}
		if !tc.success {
			if count != 0 {
				t.Fatal("invalid import created a group", tc.name)
			}
			continue
		}
		result := gconv.Map(res.Data)
		if gconv.Int(result["imported_count"]) != tc.imported || gconv.Int(result["filtered_count"]) != tc.filtered {
			t.Fatal("incorrect import counts", result)
		}
		group := gconv.Int(result["group_id"])
		count, e = g.DB().Model("bm_contacts").Where("group_id", group).Count()
		if e != nil || count != tc.imported {
			t.Fatal("group contains unexpected contacts", count, e)
		}
	}
	book, e := excelize.OpenReader(bytes.NewReader(data))
	if e != nil {
		t.Fatal(e)
	}
	defer book.Close()
	for i, values := range [][]interface{}{{"invalid-address", "Bad", "", "", "{}"}, {"spam@example.com", "Excluded", "", "", "bad-json"}, {"BUYER@example.com", "Duplicate", "", "", "{}"}} {
		cell, _ := excelize.CoordinatesToCellName(1, i+3)
		_ = book.SetSheetRow(book.GetSheetName(0), cell, &values)
	}
	raw, _ := book.WriteToBuffer()
	file := base64.StdEncoding.EncodeToString(raw.Bytes())
	before, _ := g.DB().Model("bm_contact_groups").Count()
	preview, e := controller.PreviewImport(ctx, &v1.PreviewImportReq{FileData: file, FilterKeywords: []string{"spam"}})
	if e != nil || !preview.Success {
		t.Fatal("preview failed", e, preview)
	}
	counts := gconv.Map(preview.Data)
	after, _ := g.DB().Model("bm_contact_groups").Count()
	if before != after || gconv.Int(counts["valid"]) != 1 || gconv.Int(counts["invalid"]) != 1 || gconv.Int(counts["filtered"]) != 1 || gconv.Int(counts["duplicate"]) != 1 {
		t.Fatal("preview wrote data or counts are wrong", counts)
	}
	token := gconv.String(counts["token"])
	request := &v1.CreateGroupReq{Name: "preview-confirmed", CreateType: 2, FileType: "xlsx", FileData: file, FilterKeywords: []string{"spam"}, PreviewToken: token}
	rejected, _ := controller.CreateGroup(ctx, request)
	if !rejected.Success || gconv.Int(gconv.Map(rejected.Data)["imported_count"]) != 1 {
		t.Fatal("invalid rows should be automatically excluded after preview", rejected)
	}
	request.Name = "preview-final"
	request.SkipInvalid = true
	request.PreviewToken = "stale"
	rejected, _ = controller.CreateGroup(ctx, request)
	if rejected.Success {
		t.Fatal("changed preview accepted")
	}
	request.PreviewToken = token
	accepted, e := controller.CreateGroup(ctx, request)
	if e != nil || !accepted.Success || gconv.Int(gconv.Map(accepted.Data)["imported_count"]) != 1 {
		t.Fatal("valid-only confirmed import failed", e, accepted)
	}
	exercisePreviewImportFiltersPendingWithoutImporting(t, ctx)
}
