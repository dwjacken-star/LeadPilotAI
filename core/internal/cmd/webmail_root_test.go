package cmd

import "testing"

func TestMailHomepageAndAdminRoutes(t *testing.T) {
	active := func(domain string) bool { return domain == "best-maple.com" || domain == "autoretsworld.com" }
	for _, tc := range []struct{ path, host, want string }{
		{"/", "mail.best-maple.com", "/roundcube/"},
		{"/", "MAIL.BEST-MAPLE.COM:443", "/roundcube/"},
		{"/", "mail.autoretsworld.com", "/roundcube/"},
		{"/", "mail.best-maple.com.", "/roundcube/"},
		{"/", "mail.unconfigured.example", ""},
		{"/", "mai-admin.autoretsworld.com", ""},
		{"/", "mail.best-maple.com.attacker.example", ""},
		{"/", "mail.best-maple.com@attacker.example", ""},
		{"/", "mail.best-maple.com/attacker", ""},
		{"/bm", "mail.best-maple.com", ""},
		{"/workspace", "mail.best-maple.com", ""},
		{"/login", "mail.best-maple.com", ""},
		{"/api/login", "mail.best-maple.com", ""},
		{"/roundcube/", "mail.best-maple.com", ""},
	} {
		if got := webmailRootDestination(tc.path, tc.host, "https://mail.autoretsworld.com", active); got != tc.want {
			t.Errorf("%s%s: got %q, want %q", tc.host, tc.path, got, tc.want)
		}
	}
}
