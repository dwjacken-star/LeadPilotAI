package cmd

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
)

func TestMailboxRedirectPrecedesStaticAdminIndex(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("ADMIN STATIC INDEX"), 0600); err != nil {
		t.Fatal(err)
	}
	s := g.Server(fmt.Sprintf("root-test-%d", time.Now().UnixNano()))
	s.SetAddr("127.0.0.1:0")
	s.SetServerRoot(dir)
	s.SetIndexFiles([]string{"index.html"})
	s.SetDumpRouterMap(false)
	s.BindHookHandler("/*", ghttp.HookBeforeServe, func(r *ghttp.Request) {
		serveWebmailRoot(r, "https://mail.autoretsworld.com", func(domain string) bool { return domain == "best-maple.com" })
	})
	s.BindHandler("/roundcube/*any", func(r *ghttp.Request) { r.Response.Write("MAILBOX LOGIN") })
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	defer s.Shutdown()
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		request, _ := http.NewRequest(method, fmt.Sprintf("http://127.0.0.1:%d/", s.GetListenedPort()), nil)
		request.Host = "mail.best-maple.com"
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(response.Body)
		response.Body.Close()
		if response.StatusCode != 302 || response.Header.Get("Location") != "/roundcube/" || strings.Contains(string(body), "ADMIN STATIC INDEX") {
			t.Fatalf("static index overrode redirect: %d %s %s", response.StatusCode, response.Header.Get("Location"), body)
		}
	}
	request, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/roundcube/", s.GetListenedPort()), nil)
	request.Host = "mail.best-maple.com"
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != 200 || string(body) != "MAILBOX LOGIN" {
		t.Fatal("mailbox handler changed", response.StatusCode, string(body))
	}
}
