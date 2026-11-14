package contact

import (
	"billionmail-core/internal/service/public"
	"context"
	"encoding/base64"
	"testing"
)

func TestContactExcelTemplate(t *testing.T) {
	data, err := public.BuildImportWorkbook(false, "en")
	if err != nil {
		t.Fatal(err)
	}
	rows, err := parseContactWorkbook(context.Background(), base64.StdEncoding.EncodeToString(data))
	if err != nil || len(rows) != 1 {
		t.Fatalf("parse: %v %v", rows, err)
	}
	if rows[0].Attribs["name"] != "Alex" || rows[0].Attribs["company"] != "Example Trading" {
		t.Fatal("attributes were lost")
	}
	filtered := 0
	rows, err = parseContactWorkbook(context.Background(), base64.StdEncoding.EncodeToString(data), func(email string) bool { filtered++; return true })
	if err != nil || len(rows) != 0 || filtered != 1 {
		t.Fatal("all-excluded workbook must be valid", err)
	}
	_, err = parseCSVContent(context.Background(), "name,email\nmissing\n")
	if err != nil {
		t.Fatal(err)
	}
}
