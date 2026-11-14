package maillog_stat

import (
	"html"
	"net/url"
	"regexp"
	"strings"
)

const mailLinkAttributeExpression = "(?i)(?:^|\\s)href\\s*=\\s*(?:\"([^\"]*)\"|'([^']*)'|([^\\s\"'<>`]+))"

var mailAnchorTag = regexp.MustCompile(`(?is)<a\b[^>]*>`)
var mailLinkAttribute = regexp.MustCompile(mailLinkAttributeExpression)

// Only instrument navigable anchors, never stylesheet/resource hrefs. HTML
// allows single/double quotes, unquoted values and case-insensitive attributes.
func rewriteTrackedLinks(content string, trackingURL func(string) string) string {
	return mailAnchorTag.ReplaceAllStringFunc(content, func(anchor string) string {
		return mailLinkAttribute.ReplaceAllStringFunc(anchor, func(attribute string) string {
			matches := mailLinkAttribute.FindStringSubmatch(attribute)
			target := ""
			for _, value := range matches[1:] {
				if value != "" {
					target = html.UnescapeString(value)
					break
				}
			}
			parsed, err := url.Parse(target)
			if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
				return attribute
			}
			path := strings.ToLower(parsed.Path)
			if strings.HasPrefix(path, "/pmta/") || strings.HasSuffix(path, "unsubscribe_new.html") || strings.HasSuffix(path, "unsubscribe.html") || strings.HasSuffix(path, "subscribe_form.html") {
				return attribute
			}
			return ` href="` + html.EscapeString(trackingURL(target)) + `"`
		})
	})
}
