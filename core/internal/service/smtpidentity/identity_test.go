package smtpidentity

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDualDomainDefaultsAndRelayPrecedence(t *testing.T) {
	main := "myhostname = mail.autoretsworld.com\nmyorigin = $myhostname\nsmtp_helo_name = old.example.com\nsender_dependent_default_transport_maps = pgsql:/etc/postfix/sql/routes.cf,\n  hash:/etc/postfix/dedicated_ip\n# keep custom settings\nsmtpd_tls_security_level = may\n"
	master := "smtp unix - - n - - smtp\n# existing relay transport\nsmtp_relay unix - - n - - smtp\n"
	domains := []string{"autoretsworld.com", "best-maple.com"}
	a, b, routes, err := render(main, master, "best-maple.com", domains)
	if err != nil {
		t.Fatal(err)
	}
	if setting(a, "myhostname") != "mail.autoretsworld.com" || setting(a, "myorigin") != "best-maple.com" || setting(a, "smtp_helo_name") != "mail.best-maple.com" {
		t.Fatal(a)
	}
	if setting(a, "sender_dependent_default_transport_maps") != "pgsql:/etc/postfix/sql/routes.cf, hash:/etc/postfix/dedicated_ip, "+transportMap {
		t.Fatal("existing mappings lost precedence", a)
	}
	for _, domain := range domains {
		if !strings.Contains(routes, "@"+domain+" "+serviceName(domain)+":\n") || !strings.Contains(b, "-o smtp_helo_name=mail."+domain) {
			t.Fatal("missing domain route", domain, b, routes)
		}
	}
	if !strings.Contains(b, "smtp_relay unix") || strings.Contains(a, "sender_canonical_maps") {
		t.Fatal("relay or From identity changed")
	}
	a2, b2, r2, err := render(a, b, "best-maple.com", domains)
	if err != nil || a2 != a || b2 != b || r2 != routes {
		t.Fatal("reapplying configuration is not stable", err)
	}
	a, b, _, err = render(a, b, "autoretsworld.com", domains)
	if err != nil || setting(a, "myorigin") != "autoretsworld.com" || setting(a, "smtp_helo_name") != "mail.autoretsworld.com" || strings.Count(b, beginMarker) != 1 {
		t.Fatal("switching default failed", err, a, b)
	}
}

func TestRejectInvalidIdentityAndInactiveDefault(t *testing.T) {
	for _, domain := range []string{"bad.com\nrelayhost=attacker", "best-maple.com;whoami", "-bad.com", "bad..com", "bad com", "localhost"} {
		if _, _, _, err := render("", "", domain, []string{domain}); err == nil {
			t.Fatal("unsafe domain accepted", domain)
		}
	}
	if _, _, _, err := render("", "", "inactive.com", []string{"active.com"}); err == nil {
		t.Fatal("inactive default accepted")
	}
	if _, _, _, err := render("", beginMarker+"\n", "active.com", []string{"active.com"}); err == nil {
		t.Fatal("broken managed block accepted")
	}
}

func TestSingleDomainRemovesManagedRotation(t *testing.T) {
	main, master, _, e := render("myhostname = mail.autoretsworld.com\nsender_dependent_default_transport_maps = pgsql:/etc/postfix/sql/routes.cf\n", "smtp unix - - n - - smtp\n", "best-maple.com", []string{"autoretsworld.com", "best-maple.com"})
	if e != nil {
		t.Fatal(e)
	}
	main, master, routes, e := render(main, master, "best-maple.com", []string{"best-maple.com"})
	if e != nil {
		t.Fatal(e)
	}
	if setting(main, "myhostname") != "mail.best-maple.com" || setting(main, "myorigin") != "best-maple.com" || setting(main, "smtp_helo_name") != "mail.best-maple.com" || strings.Contains(main, transportMap) || strings.Contains(master, beginMarker) || routes != "" || strings.Contains(main+master, "autoretsworld.com") {
		t.Fatal("dual-domain configuration remained", main, master, routes)
	}
	if setting(main, "sender_dependent_default_transport_maps") != "pgsql:/etc/postfix/sql/routes.cf" {
		t.Fatal("external SMTP relay precedence changed", main)
	}
	a, b, c, e := render(main, master, "best-maple.com", []string{"best-maple.com"})
	if e != nil || a != main || b != master || c != routes {
		t.Fatal("single-domain rendering is not idempotent")
	}
}

func TestFailedActivationRestoresFilesAndReloadsOldConfiguration(t *testing.T) {
	dir := t.TempDir()
	main := "myhostname = mail.example.com\nmyorigin = old.example.com\n"
	master := "smtp unix - - n - - smtp\n"
	for name, contents := range map[string]string{"main.cf": main, "master.cf": master} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(contents), 0640); err != nil {
			t.Fatal(err)
		}
	}
	calls := 0
	err := apply(context.Background(), dir, "best-maple.com", []string{"best-maple.com", "autoretsworld.com"}, func(ctx context.Context) error {
		calls++
		if calls == 1 {
			return errors.New("controlled postfix check failure")
		}
		data, _ := os.ReadFile(filepath.Join(dir, "main.cf"))
		if string(data) != main {
			t.Fatal("old config not restored before reload")
		}
		return nil
	})
	if err == nil || calls != 2 {
		t.Fatal("failed configuration was not rolled back", err, calls)
	}
	for name, contents := range map[string]string{"main.cf": main, "master.cf": master} {
		data, _ := os.ReadFile(filepath.Join(dir, name))
		info, _ := os.Stat(filepath.Join(dir, name))
		if string(data) != contents || info.Mode().Perm() != 0640 {
			t.Fatal("original files or permissions changed", name)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "conf/maplemail_sender_domains")); !os.IsNotExist(err) {
		t.Fatal("new map retained after rollback")
	}
}

func TestFailedOptionPublicationRestoresSMTPConfiguration(t *testing.T) {
	dir := t.TempDir()
	for name, contents := range map[string]string{"main.cf": "myorigin = old.example.com\n", "master.cf": "smtp unix - - n - - smtp\n"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(contents), 0644); err != nil {
			t.Fatal(err)
		}
	}
	calls := 0
	err := apply(context.Background(), dir, "best-maple.com", []string{"best-maple.com"}, func(context.Context) error { calls++; return nil }, func() error { return errors.New("controlled option write failure") })
	data, _ := os.ReadFile(filepath.Join(dir, "main.cf"))
	if err == nil || calls != 2 || string(data) != "myorigin = old.example.com\n" {
		t.Fatal("failed option publication did not restore SMTP config", err, calls, string(data))
	}
}
