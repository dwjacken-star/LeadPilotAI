// Package publiclegal serves only the explicitly listed, embedded public policies.
// It does not accept data requests or change account authorization.
package publiclegal

import (
	"embed"
	"github.com/gogf/gf/v2/net/ghttp"
	"net/http"
	"strconv"
)

//go:embed pages/*.html pages/*.css pages/*.js pages/*.svg
var pages embed.FS

var paths = map[string]string{
	"/legal": "index.html", "/legal/": "index.html",
	"/legal/privacy": "privacy.html", "/legal/privacy/": "privacy.html", "/legal/privacy.html": "privacy.html",
	"/legal/terms": "terms.html", "/legal/terms/": "terms.html", "/legal/terms.html": "terms.html",
	"/legal/data-deletion": "data-deletion.html", "/legal/data-deletion/": "data-deletion.html", "/legal/data-deletion.html": "data-deletion.html",
	"/legal/style.css": "style.css", "/legal/app.js": "app.js", "/legal/mark.svg": "mark.svg",
}

// Serve returns true only for public policy URLs and their local assets.
// Call it after host validation and before administrator/IP/session middleware.
func Serve(r *ghttp.Request) bool {
	filename, ok := paths[r.URL.Path]
	if !ok {
		return false
	}
	r.Response.Header().Set("X-Content-Type-Options", "nosniff")
	r.Response.Header().Set("Referrer-Policy", "no-referrer")
	r.Response.Header().Set("X-Frame-Options", "DENY")
	r.Response.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
	r.Response.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'self'; script-src 'self'; img-src 'self'; connect-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'none'")
	r.Response.Header().Set("Cache-Control", "no-cache, max-age=0, must-revalidate")
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		r.Response.Header().Set("Allow", "GET, HEAD")
		r.Response.WriteHeader(http.StatusMethodNotAllowed)
		r.ExitAll()
		return true
	}
	data, err := pages.ReadFile("pages/" + filename)
	if err != nil {
		r.Response.WriteHeader(http.StatusInternalServerError)
		r.ExitAll()
		return true
	}
	contentType := "text/html; charset=utf-8"
	switch filename {
	case "style.css":
		contentType = "text/css; charset=utf-8"
	case "app.js":
		contentType = "text/javascript; charset=utf-8"
	case "mark.svg":
		contentType = "image/svg+xml"
	}
	r.Response.Header().Set("Content-Type", contentType)
	r.Response.Header().Set("Content-Length", strconv.Itoa(len(data)))
	r.Response.WriteHeader(http.StatusOK)
	if r.Method == http.MethodGet {
		r.Response.Write(data)
	}
	r.ExitAll()
	return true
}
