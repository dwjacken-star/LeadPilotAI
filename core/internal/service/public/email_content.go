package public

import (
	"fmt"
	"html"
	"regexp"
	"strings"
)

var emailHTMLTag = regexp.MustCompile(`(?is)<(?:!doctype\s+html\b[^>]*|/?(?:html|head|body|style|table|tbody|thead|tfoot|tr|td|th|p|div|br|ul|ol|li|h[1-6]|span|a|img|strong|b|em|i|u|s|blockquote|pre|hr|font|center|section|article|meta|title|link)(?:\s[^<>]*|/?)?)>`)
var emailTemplateAction = regexp.MustCompile(`(?s){{.*?}}`)
var emailParagraphBreak = regexp.MustCompile(`\n[\t ]*\n+`)
var emailBrandFooter = regexp.MustCompile(`(?is)(Powered\s+by\s*(?:</span>\s*)?<a\b[^>]*>)(?:BillionMail|MapleMail)(</a>)`)
var emailPlainBrandFooter = regexp.MustCompile(`(?i)(Powered\s+by\s+)(?:BillionMail|MapleMail)\b`)

// NormalizeEmailHTML preserves authored HTML verbatim. Plain text needs explicit
// paragraphs and breaks because HTML rendering otherwise collapses every newline.
// Template actions remain intact when a legacy caller adds its footer before rendering.
func NormalizeEmailHTML(content string) string {
	content = emailBrandFooter.ReplaceAllStringFunc(content, func(footer string) string {
		footer = strings.ReplaceAll(footer, "https://www.billionmail.com/", "https://github.com/dwjacken-star/LeadPilotAI")
		footer = strings.ReplaceAll(footer, "https://www.billionmail.com", "https://github.com/dwjacken-star/LeadPilotAI")
		return emailBrandFooter.ReplaceAllString(footer, "${1}leadpilotmail${2}")
	})
	content = emailPlainBrandFooter.ReplaceAllString(content, "${1}leadpilotmail")
	if strings.TrimSpace(content) == "" {
		return content
	}
	actions := []string{}
	masked := emailTemplateAction.ReplaceAllStringFunc(content, func(value string) string {
		actions = append(actions, value)
		return fmt.Sprintf("MAPLEMAILACTIONTOKEN%dEND", len(actions)-1)
	})
	if emailHTMLTag.MatchString(masked) {
		return content
	}
	masked = strings.ReplaceAll(strings.ReplaceAll(masked, "\r\n", "\n"), "\r", "\n")
	parts := emailParagraphBreak.Split(masked, -1)
	var output strings.Builder
	output.WriteString(`<div style="font-family:Arial,'Microsoft YaHei',sans-serif;font-size:14px;line-height:1.6;color:#222;">`)
	for _, part := range parts {
		output.WriteString(`<p style="margin:0 0 16px;white-space:pre-wrap;">`)
		output.WriteString(strings.ReplaceAll(html.EscapeString(part), "\n", "<br>"))
		output.WriteString("</p>")
	}
	output.WriteString("</div>")
	result := output.String()
	for i, action := range actions {
		result = strings.ReplaceAll(result, fmt.Sprintf("MAPLEMAILACTIONTOKEN%dEND", i), action)
	}
	return result
}

func AppendEmailHTML(content, footer string) string {
	content = NormalizeEmailHTML(content)
	lower := strings.ToLower(content)
	if index := strings.LastIndex(lower, "</body>"); index >= 0 {
		return content[:index] + footer + content[index:]
	}
	if index := strings.LastIndex(lower, "</html>"); index >= 0 {
		return content[:index] + footer + content[index:]
	}
	return content + footer
}
