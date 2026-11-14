package public

import (
	"fmt"
	"strings"
)

// NormalizeImportKeywords limits input and treats each keyword as a literal substring.
func NormalizeImportKeywords(values []string) ([]string, error) {
	result := []string{}
	seen := map[string]bool{}
	for _, value := range values {
		for _, word := range strings.FieldsFunc(value, func(r rune) bool { return r == '\n' || r == '\r' || r == ',' || r == '，' || r == ';' || r == '；' }) {
			word = strings.ToLower(strings.TrimSpace(word))
			if word == "" || seen[word] {
				continue
			}
			if len([]rune(word)) > 100 || len(result) >= 50 {
				return nil, fmt.Errorf("import_filter_limit")
			}
			seen[word] = true
			result = append(result, word)
		}
	}
	return result, nil
}

func MatchesImportKeywords(email string, keywords []string) bool {
	email = strings.ToLower(email)
	for _, keyword := range keywords {
		if strings.Contains(email, keyword) {
			return true
		}
	}
	return false
}
