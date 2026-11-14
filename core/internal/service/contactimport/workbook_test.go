package contactimport

import (
	"billionmail-core/internal/testsupport"
	"bytes"
	"context"
	"encoding/base64"
	"github.com/xuri/excelize/v2"
	"testing"
)

func TestPreviewClassifiesAndConfirms(t *testing.T) {
	testsupport.ValidEmailChecks(t)
	book := excelize.NewFile()
	defer book.Close()
	rows := [][]interface{}{{"email", "name", "attributes"}, {"BUYER@example.com", "Buyer", "{}"}, {"spam@example.com", "Skip", "invalid-json"}, {"bad-address", "Invalid", "{}"}, {"buyer@example.com", "Duplicate", "{}"}, {"other@example.com", "Bad attributes", "invalid-json"}}
	for i, row := range rows {
		cell, _ := excelize.CoordinatesToCellName(1, i+1)
		_ = book.SetSheetRow("Sheet1", cell, &row)
	}
	data, _ := book.WriteToBuffer()
	encoded := base64.StdEncoding.EncodeToString(data.Bytes())
	result, e := Read(context.Background(), encoded, []string{"SPAM", "no-reply"})
	if e != nil {
		t.Fatal(e)
	}
	if result.Total != 5 || result.Valid != 1 || result.Filtered != 1 || result.Invalid != 2 || result.Duplicate != 1 {
		t.Fatalf("wrong classification: %+v", result)
	}
	if result.Contacts[0].Email != "buyer@example.com" || result.Rows[1].Keyword != "spam" || result.Rows[2].Row != 4 {
		t.Fatal("row metadata or normalization was lost")
	}
	if result.ValidateConfirmation(result.Token, false) != nil {
		t.Fatal("invalid rows must be excluded automatically after preview")
	}
	if result.ValidateConfirmation("", true) == nil {
		t.Fatal("skip-invalid accepted without preview")
	}
	if result.ValidateConfirmation("stale-token", true) == nil {
		t.Fatal("stale preview accepted")
	}
	if result.ValidateConfirmation(result.Token, true) != nil {
		t.Fatal("matching preview rejected")
	}
	changed, _ := Read(context.Background(), encoded, []string{"buyer"})
	if changed.Token == result.Token {
		t.Fatal("keywords did not invalidate preview")
	}
	invalid, e := result.InvalidWorkbook()
	if e != nil {
		t.Fatal(e)
	}
	xlsx, e := excelize.OpenReader(bytes.NewReader(invalid))
	if e != nil {
		t.Fatal(e)
	}
	defer xlsx.Close()
	badRows, _ := xlsx.GetRows(xlsx.GetSheetName(0))
	if len(badRows) != 3 {
		t.Fatal("invalid rows workbook missing rows", len(badRows))
	}
}
