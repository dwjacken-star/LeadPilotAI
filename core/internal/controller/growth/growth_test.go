package growth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"

	"billionmail-core/internal/controller/enterprise"
	"billionmail-core/internal/service/governance"
	service "billionmail-core/internal/service/growth"
	"billionmail-core/internal/service/middlewares"
	"billionmail-core/internal/testsupport"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
)

func TestGrowthAuthenticatedHTTP(t *testing.T) {
	testsupport.Database(t)
	t.Setenv("MAPLEMAIL_GROWTH_DATA", t.TempDir())
	ctx := context.Background()
	p, e := governance.Load(ctx, 1)
	if e != nil {
		t.Fatal(e)
	}
	id, e := service.SaveCompany(ctx, p, 0, g.Map{"name": "HTTP company", "profile": g.Map{}})
	if e != nil {
		t.Fatal(e)
	}
	_, e = g.DB().Exec(ctx, `INSERT INTO bm_departments(id,name,active) VALUES(22,'Other department',true);INSERT INTO account(username,password,email,status,dept_id) VALUES('outside','test','',1,22);INSERT INTO role(role_name,status,data_scope,permissions) VALUES('growth_reader',1,'department','["company:list","material:list","material:download"]');INSERT INTO account_role(account_id,role_id) SELECT 2,role_id FROM role WHERE role_name='growth_reader'`)
	if e != nil {
		t.Fatal(e)
	}
	s := g.Server("growth-http-test")
	s.SetAddr("127.0.0.1:0")
	s.SetDumpRouterMap(false)
	s.Group("/api", func(group *ghttp.RouterGroup) {
		group.Middleware(func(r *ghttp.Request) {
			account := int64(1)
			if r.Header.Get("X-Test-Account") == "outside" {
				account = 2
			}
			r.SetCtxVar("accountId", account)
			r.Middleware.Next()
		})
		group.Middleware(enterprise.LegacyBoundary)
		group.Middleware(middlewares.HandleApiResponse)
		group.Bind(New())
		group.GET("/legacy-secret", func(r *ghttp.Request) { r.Response.Write("secret") })
	})
	if e = s.Start(); e != nil {
		t.Fatal(e)
	}
	defer s.Shutdown()
	base := fmt.Sprintf("http://127.0.0.1:%d/api", s.GetListenedPort())
	post := func(path, body, account string) *http.Response {
		t.Helper()
		req, _ := http.NewRequest("POST", base+path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if account != "" {
			req.Header.Set("X-Test-Account", account)
		}
		r, e := http.DefaultClient.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		return r
	}
	var form bytes.Buffer
	writer := multipart.NewWriter(&form)
	writer.WriteField("company_id", fmt.Sprint(id))
	writer.WriteField("purpose", "company")
	writer.WriteField("ai_allowed", "false")
	file, _ := writer.CreateFormFile("file", "company-facts.txt")
	file.Write([]byte("Automotive parts evidence"))
	writer.Close()
	request, _ := http.NewRequest("POST", base+"/growth/upload", &form)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response, e := http.DefaultClient.Do(request)
	if e != nil {
		t.Fatal(e)
	}
	var result struct {
		Success bool   `json:"success"`
		Msg     string `json:"msg"`
		Data    g.Map  `json:"data"`
	}
	e = json.NewDecoder(response.Body).Decode(&result)
	response.Body.Close()
	if e != nil || !result.Success {
		t.Fatal("multipart upload", result, e)
	}
	assetID := fmt.Sprint(result.Data["id"])
	response = post("/growth/file", fmt.Sprintf(`{"asset_id":%q}`, assetID), "")
	raw, e := io.ReadAll(response.Body)
	response.Body.Close()
	if e != nil || string(raw) != "Automotive parts evidence" || !strings.Contains(response.Header.Get("Content-Disposition"), "company-facts.txt") {
		t.Fatal("authorized original download failed", string(raw), e)
	}
	for _, body := range []string{fmt.Sprintf(`{"action":"asset","asset_id":%q}`, assetID), fmt.Sprintf(`{"action":"list","kind":"assets","company_id":%d}`, id)} {
		response = post("/growth/action", body, "outside")
		raw, _ = io.ReadAll(response.Body)
		response.Body.Close()
		if strings.Contains(string(raw), `"success":true`) || strings.Contains(string(raw), "Automotive parts evidence") {
			t.Fatal("private materials leaked", string(raw))
		}
	}
	response = post("/growth/file", fmt.Sprintf(`{"asset_id":%q}`, assetID), "outside")
	raw, _ = io.ReadAll(response.Body)
	response.Body.Close()
	if !strings.Contains(string(raw), `"success":false`) {
		t.Fatal("unauthorized file was downloaded", string(raw))
	}
	response = post("/growth/action", `{"action":"settings"}`, "outside")
	raw, _ = io.ReadAll(response.Body)
	response.Body.Close()
	if strings.Contains(string(raw), `"success":true`) {
		t.Fatal("nonadmin read provider settings")
	}
	for _, action := range []string{"serper_settings", "save_serper_settings", "test_serper"} {
		response = post("/growth/action", fmt.Sprintf(`{"action":%q,"data":{"api_key":"unauthorized-test-key"}}`, action), "outside")
		raw, _ = io.ReadAll(response.Body)
		response.Body.Close()
		if !strings.Contains(string(raw), `"success":false`) || strings.Contains(string(raw), "unauthorized-test-key") {
			t.Fatal("Serper configuration boundary failed", action, string(raw))
		}
	}
	req, _ := http.NewRequest("GET", base+"/legacy-secret", nil)
	req.Header.Set("X-Test-Account", "outside")
	response, e = http.DefaultClient.Do(req)
	if e != nil {
		t.Fatal(e)
	}
	raw, _ = io.ReadAll(response.Body)
	response.Body.Close()
	if !strings.Contains(string(raw), `"code":403`) {
		t.Fatal("legacy boundary bypassed", string(raw))
	}
	for _, kind := range []string{"products", "company", "images"} {
		response = post("/growth/template", fmt.Sprintf(`{"kind":%q,"company_id":%d,"language":"zh"}`, kind, id), "")
		raw, _ = io.ReadAll(response.Body)
		response.Body.Close()
		if len(raw) < 2 || string(raw[:2]) != "PK" || !strings.Contains(response.Header.Get("Content-Disposition"), "template.") {
			t.Fatal("template download failed", kind, response.Header, string(raw))
		}
	}
	response = post("/growth/template", fmt.Sprintf(`{"kind":"products","company_id":%d}`, id), "outside")
	raw, _ = io.ReadAll(response.Body)
	response.Body.Close()
	if !strings.Contains(string(raw), `"success":false`) || response.Header.Get("Content-Disposition") != "" {
		t.Fatal("unauthorized template download allowed")
	}
}
