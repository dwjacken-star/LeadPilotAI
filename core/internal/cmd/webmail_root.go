package cmd

import (
	"github.com/gogf/gf/v2/net/ghttp"
	"net/http"
	"net/url"
	"strings"
)

func serveWebmailRoot(r *ghttp.Request, publicBase string, activeDomain func(string) bool) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return
	}
	destination := webmailRootDestination(r.URL.Path, r.Host, publicBase, activeDomain)
	if destination == "" {
		return
	}
	r.Response.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	r.Response.Header().Set("Location", destination)
	r.Response.WriteHeader(http.StatusFound)
	// RedirectTo exits only the current hook. ExitAll is required here to stop
	// GoFrame's higher-priority static index handler from overwriting the 302.
	r.ExitAll()
}

// Mail domain homepages are public mailbox entrances. The administrator SPA
// keeps its separate safe-path entrance and its existing session/auth checks.
func webmailRootDestination(path, host, publicBase string, activeDomain func(string) bool) string {
	if path != "/" || strings.ContainsAny(host, "/\\@ \t\r\n") {
		return ""
	}
	u, err := url.Parse("https://" + host)
	if err != nil || u.Hostname() == "" {
		return ""
	}
	hostname := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	base, err := url.Parse(publicBase)
	if err == nil && base.Hostname() != "" && strings.EqualFold(hostname, strings.TrimSuffix(base.Hostname(), ".")) {
		return "/roundcube/"
	}
	if strings.HasPrefix(hostname, "mail.") && activeDomain != nil && activeDomain(strings.TrimPrefix(hostname, "mail.")) {
		return "/roundcube/"
	}
	return ""
}
