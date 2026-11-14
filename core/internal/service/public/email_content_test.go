package public

import (
	_ "embed"
	"encoding/json"
	"strings"
	"testing"
)

//go:embed testdata/email_content.json
var emailFormattingFixtures []byte

func TestEmailFormattingContract(t *testing.T) {
	var fixtures []struct{ Name, Input, Expected string }
	if err := json.Unmarshal(emailFormattingFixtures, &fixtures); err != nil {
		t.Fatal(err)
	}
	for _, tc := range fixtures {
		t.Run(tc.Name, func(t *testing.T) {
			got := NormalizeEmailHTML(tc.Input)
			if got != tc.Expected {
				t.Fatalf("unexpected email layout:\n%s", got)
			}
			if NormalizeEmailHTML(got) != got {
				t.Fatal("email normalization is not idempotent")
			}
		})
	}
	html := `<HTML><HEAD><style>p{color:red}</style></HEAD><BODY><p>Quote</p></BODY></HTML>`
	result := AppendEmailHTML(html, `<p>Unsubscribe</p>`)
	if !strings.Contains(result, `<p>Unsubscribe</p></BODY>`) || !strings.Contains(result, `<style>p{color:red}</style>`) {
		t.Fatal("footer damaged the document", result)
	}
}
