package middleware

import "testing"

func TestPublicLanguageAssetsDoNotOpenAdministrativeRoutes(t *testing.T) {
	for _, path := range []string{"/leadpilot-public-layout.js", "/leadpilot-public-i18n.js"} {
		if !isExcludedPath(path) {
			t.Fatal("public language asset blocked", path)
		}
	}
	for _, path := range []string{"/api/enterprise/action", "/api/settings/get", "/static/js/admin.js"} {
		if isExcludedPath(path) {
			t.Fatal("administrative route was exposed", path)
		}
	}
}
