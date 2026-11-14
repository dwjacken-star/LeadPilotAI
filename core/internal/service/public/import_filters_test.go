package public

import (
	"strings"
	"testing"
)

func TestImportKeywords(t *testing.T) {
	words, err := NormalizeImportKeywords([]string{" Spam ,NOREPLY\n", "spam", "@example.com； sales"})
	if err != nil || len(words) != 4 {
		t.Fatalf("normalization: %v %v", words, err)
	}
	for _, email := range []string{"SPAM123@domain.com", "a@EXAMPLE.COM", "noreply@domain.com", "SALES@domain.com"} {
		if !MatchesImportKeywords(email, words) {
			t.Fatalf("not excluded: %s", email)
		}
	}
	if MatchesImportKeywords("valid@domain.com", words) || MatchesImportKeywords("valid@domain.com", nil) {
		t.Fatal("unexpected exclusion")
	}
	if _, err := NormalizeImportKeywords([]string{strings.Repeat("x", 101)}); err == nil {
		t.Fatal("missing length limit")
	}
	if _, err := NormalizeImportKeywords(strings.Fields(strings.Repeat("x ", 51))); err != nil {
		t.Fatal("duplicate words should be deduplicated")
	}
}
