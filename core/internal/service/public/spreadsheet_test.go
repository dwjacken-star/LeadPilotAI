package public

import (
	"encoding/base64"
	"testing"
)

func TestImportWorkbookRoundTrip(t *testing.T) {
	for _, mailbox := range []bool{false, true} {
		data, err := BuildImportWorkbook(mailbox, "zh")
		if err != nil {
			t.Fatal(err)
		}
		rows, err := ReadSpreadsheet("data:" + SpreadsheetMIME + ";base64," + base64.StdEncoding.EncodeToString(data))
		if err != nil || len(rows) != 2 {
			t.Fatalf("workbook: %v %v", rows, err)
		}
		expected := "email"
		if mailbox {
			expected = "username"
		}
		if rows[0][0] != expected {
			t.Fatal("wrong import columns")
		}
	}
	if _, err := ReadSpreadsheet("not-a-workbook"); err == nil {
		t.Fatal("invalid workbook accepted")
	}
}
