package growth

import (
	"net/url"
	"regexp"
	"sort"
	"strings"
)

var markdownLink = regexp.MustCompile(`\[[^\]]*\]\(<?([^\s)>]+)>?[^)]*\)`)

func contactPage(value string) bool {
	path, _ := url.PathUnescape(strings.ToLower(value))
	for _, word := range []string{"contact", "about", "kontakt", "contacto", "contato", "lien-he", "lien_he", "ติดต่อ", "เกี่ยวกับ", "联系我们", "聯絡", "hubungi", "tungkol"} {
		if strings.Contains(path, word) {
			return true
		}
	}
	return false
}
func parseSerperPage(actual *url.URL, text, markdown string) WebPage {
	// Markdown retains mailto links and footer addresses which a summary could omit.
	page := WebPage{URL: actual.String(), Text: cleanText(strings.Join(strings.Fields(text+"\n"+markdown), " "), 60000)}
	emails := map[string]bool{}
	links := map[string]bool{}
	for _, email := range emailPattern.FindAllString(page.Text, -1) {
		if normalized, ok := collectedEmail(email); ok {
			emails[normalized] = true
		}
	}
	for _, match := range markdownLink.FindAllStringSubmatch(markdown, -1) {
		ref, e := url.Parse(strings.TrimSpace(match[1]))
		if e != nil {
			continue
		}
		if strings.EqualFold(ref.Scheme, "mailto") {
			for _, email := range emailPattern.FindAllString(ref.Opaque, -1) {
				if normalized, ok := collectedEmail(email); ok {
					emails[normalized] = true
				}
			}
			continue
		}
		target := actual.ResolveReference(ref)
		target.Fragment = ""
		if strings.EqualFold(target.Hostname(), actual.Hostname()) && target.String() != actual.String() && contactPage(target.Path) {
			if _, e = safeURL(target.String()); e == nil {
				links[target.String()] = true
			}
		}
	}
	for email := range emails {
		page.Emails = append(page.Emails, email)
	}
	for link := range links {
		page.Links = append(page.Links, link)
	}
	sort.Strings(page.Emails)
	sort.Slice(page.Links, func(i, j int) bool {
		a, b := strings.Contains(strings.ToLower(page.Links[i]), "contact"), strings.Contains(strings.ToLower(page.Links[j]), "contact")
		if a != b {
			return a
		}
		return page.Links[i] < page.Links[j]
	})
	return page
}
