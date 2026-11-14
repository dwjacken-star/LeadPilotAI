package mail_service

import (
	"encoding/base64"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"strings"
	"testing"
)

func TestOriginalMessageForwardingMIME(t *testing.T) {
	original := []byte("From: buyer@example.test\r\nSubject: Quotation\r\nContent-Type: multipart/mixed; boundary=original\r\n\r\n--original\r\nOriginal attachment bytes\r\n--original--\r\n")
	m := NewMessage("Inquiry", "<p>Please review this inquiry.</p>")
	m.AddAttachment("原始邮件.eml", "message/rfc822", original)
	headers, body, e := m.mimePayload()
	if e != nil {
		t.Fatal(e)
	}
	media, params, e := mime.ParseMediaType(strings.TrimSpace(strings.TrimPrefix(headers, "Content-Type:")))
	if e != nil || media != "multipart/mixed" {
		t.Fatal(headers, e)
	}
	reader := multipart.NewReader(strings.NewReader(body), params["boundary"])
	part, e := reader.NextPart()
	if e != nil {
		t.Fatal(e)
	}
	decoded, e := io.ReadAll(quotedprintable.NewReader(part))
	if e != nil || !strings.Contains(string(decoded), "Please review") {
		t.Fatal("summary content lost", e)
	}
	part, e = reader.NextPart()
	if e != nil {
		t.Fatal(e)
	}
	decoded, e = io.ReadAll(base64.NewDecoder(base64.StdEncoding, part))
	if e != nil || string(decoded) != string(original) {
		t.Fatal("original MIME changed", e)
	}
	if part.Header.Get("Content-Transfer-Encoding") != "base64" {
		t.Fatal("attachment encoding missing")
	}
	_, disposition, e := mime.ParseMediaType(part.Header.Get("Content-Disposition"))
	if e != nil || disposition["filename"] != "原始邮件.eml" {
		t.Fatal("Unicode attachment filename lost", e)
	}
	if _, e = reader.NextPart(); e != io.EOF {
		t.Fatal("unexpected extra MIME part", e)
	}
}
