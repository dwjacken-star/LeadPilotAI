package cmd

import "testing"

func TestPublicHostAllowlist(t *testing.T) {
	allowed := "mail.best-maple.com,autorets.best-maple.com,47.237.67.184"
	for _, host := range []string{"mail.best-maple.com", "MAIL.BEST-MAPLE.COM:443", "autorets.best-maple.com", "127.0.0.1:80", "[::1]:443", "47.237.67.184"} {
		if !publicHostAllowed(host, allowed) {
			t.Fatal("allowed host blocked", host)
		}
	}
	for _, host := range []string{"mail.autoretsworld.com", "mai-admin.autoretsworld.com", "mail.best-maple.com.attacker.test", "attacker.test"} {
		if publicHostAllowed(host, allowed) {
			t.Fatal("retired or unknown host accepted", host)
		}
	}
	if !publicHostAllowed("existing.example.com", "") {
		t.Fatal("unconfigured deployment behavior changed")
	}
}
