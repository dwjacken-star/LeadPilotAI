package publiclegal

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
)

func TestPublicPoliciesWithoutAdminAccess(t *testing.T) {
	s := g.Server(fmt.Sprintf("legal-test-%d", time.Now().UnixNano()))
	s.SetAddr("127.0.0.1:0")
	s.SetDumpRouterMap(false)
	s.BindHookHandler("/*", ghttp.HookBeforeServe, func(r *ghttp.Request) {
		if r.Host != "mail.best-maple.com" {
			r.Response.WriteHeader(421)
			r.ExitAll()
			return
		}
		Serve(r)
	})
	// Represents the private management application: every other request is denied.
	s.Use(func(r *ghttp.Request) { r.Response.WriteHeader(401); r.ExitAll() })
	s.BindHandler("/*any", func(r *ghttp.Request) { r.Response.Write("PRIVATE") })
	if e := s.Start(); e != nil {
		t.Fatal(e)
	}
	defer s.Shutdown()
	base := fmt.Sprintf("http://127.0.0.1:%d", s.GetListenedPort())
	request := func(method, path, host string) (*http.Response, string) {
		t.Helper()
		req, e := http.NewRequest(method, base+path, nil)
		if e != nil {
			t.Fatal(e)
		}
		req.Host = host
		req.Header.Set("User-Agent", "facebookexternalhit/1.1")
		res, e := http.DefaultClient.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		body, e := io.ReadAll(res.Body)
		res.Body.Close()
		if e != nil {
			t.Fatal(e)
		}
		return res, string(body)
	}
	for _, p := range []string{"/legal", "/legal/", "/legal/privacy", "/legal/privacy/", "/legal/terms", "/legal/data-deletion", "/legal/privacy.html?lang=zh"} {
		res, body := request("GET", p, "mail.best-maple.com")
		if res.StatusCode != 200 {
			t.Fatal("public policy blocked", p, res.StatusCode)
		}
		for _, value := range []string{"Hangzhou Bestmaple Auto Parts Co., Ltd.", "杭州佳枫汽车配件有限公司", "autobestmaple@gmail.com", "/legal/privacy", "/legal/terms", "/legal/data-deletion", "name=\"viewport\""} {
			if !strings.Contains(body, value) {
				t.Fatal(p, "missing disclosure/link", value)
			}
		}
		for _, bad := range []string{"sales@best-maple.com", "TODO", "[Company Name]", "PRIVATE", "<iframe"} {
			if strings.Contains(body, bad) {
				t.Fatal(p, "unintended content", bad)
			}
		}
		if res.Header.Get("Set-Cookie") != "" {
			t.Fatal("public policy creates a login cookie")
		}
		if !strings.Contains(res.Header.Get("Content-Security-Policy"), "connect-src 'none'") {
			t.Fatal("unexpected third-party connections allowed")
		}
	}
	for _, p := range []string{"/legal/style.css", "/legal/app.js", "/legal/mark.svg"} {
		res, body := request("GET", p, "mail.best-maple.com")
		if res.StatusCode != 200 || len(body) == 0 || res.Header.Get("X-Content-Type-Options") != "nosniff" {
			t.Fatal("asset unavailable", p)
		}
	}
	res, body := request("HEAD", "/legal/privacy", "mail.best-maple.com")
	if res.StatusCode != 200 || len(body) != 0 || res.Header.Get("Content-Length") == "0" {
		t.Fatal("HEAD invalid")
	}
	res, _ = request("POST", "/legal/data-deletion", "mail.best-maple.com")
	if res.StatusCode != 405 {
		t.Fatal("instructions page accepts a submission")
	}
	for _, p := range []string{"/api/enterprise/action", "/workspace", "/legal/content-manifest.json", "/legal/../private", "/legal/not-found"} {
		res, body = request("GET", p, "mail.best-maple.com")
		if res.StatusCode == 200 || strings.Contains(body, "PRIVATE") {
			t.Fatal("private route exposed", p)
		}
	}
	res, _ = request("GET", "/legal/privacy", "retired.example")
	if res.StatusCode != 421 {
		t.Fatal("host restriction bypassed")
	}
}

func TestDeletionTextIsHonestAndNoNetworkScripts(t *testing.T) {
	b, e := pages.ReadFile("pages/data-deletion.html")
	if e != nil {
		t.Fatal(e)
	}
	s := string(b)
	for _, fragment := range []string{"not an automated callback", "does not submit a deletion request", "do not send a password", "mailto:autobestmaple@gmail.com"} {
		if !strings.Contains(s, fragment) {
			t.Fatal("missing deletion safeguard", fragment)
		}
	}
	b, e = pages.ReadFile("pages/app.js")
	if e != nil {
		t.Fatal(e)
	}
	s = string(b)
	for _, bad := range []string{"fetch(", "XMLHttpRequest", "sendBeacon", "localStorage", "sessionStorage"} {
		if strings.Contains(s, bad) {
			t.Fatal("unexpected collection", bad)
		}
	}
}
