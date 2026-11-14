package contactimport

import (
	"billionmail-core/internal/service/emailvalidation"
	"context"
	"encoding/base64"
	"github.com/xuri/excelize/v2"
	"net"
	"testing"
)

type importResolver struct{}

func (importResolver) LookupMX(ctx context.Context, host string) ([]*net.MX, error) {
	switch host {
	case "null.test":
		return []*net.MX{{Host: "."}}, nil
	case "temporary.test":
		return nil, &net.DNSError{Err: "SERVFAIL", IsTemporary: true}
	}
	return []*net.MX{{Host: "mx.fixture.net."}}, nil
}
func (importResolver) LookupIPAddr(context.Context, string) ([]net.IPAddr, error) {
	return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}, nil
}
func TestImportNeverConfirmsTemporaryDNS(t *testing.T) {
	old := emailvalidation.Default
	emailvalidation.Default = emailvalidation.New(importResolver{})
	defer func() { emailvalidation.Default = old }()
	book := excelize.NewFile()
	defer book.Close()
	for i, email := range []string{"email", "buyer@route.test", "nobody@null.test", "later@temporary.test"} {
		row := []interface{}{email}
		cell, _ := excelize.CoordinatesToCellName(1, i+1)
		_ = book.SetSheetRow("Sheet1", cell, &row)
	}
	data, _ := book.WriteToBuffer()
	scan, err := Read(context.Background(), base64.StdEncoding.EncodeToString(data.Bytes()), nil)
	if err != nil {
		t.Fatal(err)
	}
	if scan.Valid != 1 || scan.Invalid != 1 || scan.Pending != 1 || len(scan.Contacts) != 1 {
		t.Fatal(scan)
	}
	if scan.Rows[1].Reason != "null_mx" || scan.Rows[2].Status != "pending" || scan.Rows[2].Reason != "dns_temporary" {
		t.Fatal(scan.Rows)
	}
	if scan.ValidateConfirmation(scan.Token, true) != nil || len(scan.ContactsForImport(false)) != 1 {
		t.Fatal("import verified rows and retain pending only in the download")
	}
	if scan.ValidateConfirmation("", true, true) == nil {
		t.Fatal("keeping pending contacts requires an explicit preview proof")
	}
	if scan.ValidateConfirmation(scan.Token, true, true) != nil || len(scan.ContactsForImport(true)) != 2 || len(scan.ContactsForImport(false)) != 1 {
		t.Fatal("explicit pending import must retain pending contacts while excluding invalid ones")
	}
}
