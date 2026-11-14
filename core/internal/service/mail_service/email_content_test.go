package mail_service

import (
	"billionmail-core/internal/service/public"
	"io"
	"mime/quotedprintable"
	"strings"
	"testing"
)

func TestMIMEKeepsEmailLayout(t *testing.T) {
	for _, content := range []string{"Dear Buyer,\nGreetings!\n\nProducts:\n✓ Compressors\n✓ Condensers", `<!doctype html><html><head><style>td{padding:24px}</style></head><body><table><tr><td><b>报价</b></td></tr></table></body></html>`} {
		message := NewMessage("Quote", content)
		decoded, err := io.ReadAll(quotedprintable.NewReader(strings.NewReader(message.MailText())))
		if err != nil || string(decoded) != public.NormalizeEmailHTML(content) {
			t.Fatalf("MIME changed layout: %s %v", decoded, err)
		}
	}
	message := NewMessage("Plain", "Keep\ntext")
	message.SetHeader("Content-Type", "text/plain; charset=utf-8")
	decoded, _ := io.ReadAll(quotedprintable.NewReader(strings.NewReader(message.MailText())))
	if string(decoded) != "Keep\ntext" && string(decoded) != "Keep\r\ntext" {
		t.Fatal("explicit plain-text message changed", string(decoded))
	}
}
