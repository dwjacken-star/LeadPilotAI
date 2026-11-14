package social

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"billionmail-core/internal/controller/enterprise"
	"billionmail-core/internal/service/governance"
	"billionmail-core/internal/service/growth"
	"billionmail-core/internal/service/middlewares"
	service "billionmail-core/internal/service/social"
	"billionmail-core/internal/testsupport"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/xuri/excelize/v2"
)

func TestSocialAuthenticatedHTTP(t *testing.T) {
	testsupport.Database(t)
	ctx := context.Background()
	t.Setenv("MAPLEMAIL_GROWTH_DATA", t.TempDir())
	p, e := governance.Load(ctx, 1)
	if e != nil {
		t.Fatal(e)
	}
	company, e := growth.SaveCompany(ctx, p, 0, g.Map{"name": "MapleMail QA", "brand": "BESTMAPLE", "profile": g.Map{"business": "Automotive A/C parts"}})
	if e != nil {
		t.Fatal(e)
	}
	if e = growth.PublishCompany(ctx, p, company); e != nil {
		t.Fatal(e)
	}
	config := service.PlanConfig{Platforms: []string{"facebook", "linkedin"}, Markets: []service.Market{{Country: "US", Language: "en", Timezone: "America/New_York", LocalTime: "09:00"}}, StartDate: time.Now().AddDate(0, 0, 3).Format("2006-01-02"), Cadence: "once", DailyPosts: 3, MinIntervalMinutes: 30, Requirement: "Introduce our automotive A/C parts"}
	plan, e := service.SavePlan(ctx, p, 0, "Local social task", company, config)
	if e != nil {
		t.Fatal(e)
	}
	_, e = g.DB().Exec(ctx, `INSERT INTO bm_departments(id,name,active) VALUES(22,'Other department',true);INSERT INTO account(username,password,email,status,dept_id) VALUES('outside','test','',1,22);INSERT INTO role(role_name,status,data_scope,permissions) VALUES('social_reader',1,'department','["social:plan:view","social:account:view","social:interaction:view","social:report:view"]');INSERT INTO account_role(account_id,role_id) SELECT 2,role_id FROM role WHERE role_name='social_reader'`)
	if e != nil {
		t.Fatal(e)
	}
	s := g.Server("social-http-test")
	s.SetAddr("127.0.0.1:0")
	s.SetDumpRouterMap(false)
	s.BindHandler("/social/callback", Callback)
	if os.Getenv("SOCIAL_LOCAL_BROWSER_TEST") == "1" {
		s.SetAddr("0.0.0.0:19896")
		s.SetServerRoot("/workspace/core/frontend/dist")
		s.BindHandler("/social", func(r *ghttp.Request) { r.Response.ServeFile("/workspace/core/frontend/dist/index.html") })
	}
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
		if os.Getenv("SOCIAL_LOCAL_BROWSER_TEST") == "1" {
			group.Bind(enterprise.New())
			group.POST("/languages/get", func(r *ghttp.Request) {
				r.Response.WriteJson(g.Map{"code": 0, "success": true, "data": g.Map{"language": "zh"}})
			})
		}
	})
	if e = s.Start(); e != nil {
		t.Fatal(e)
	}
	defer s.Shutdown()
	base := fmt.Sprintf("http://127.0.0.1:%d/api", s.GetListenedPort())
	post := func(path, body, account string) ([]byte, *http.Response) {
		t.Helper()
		req, _ := http.NewRequest("POST", base+path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Test-Account", account)
		res, e := http.DefaultClient.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		raw, e := io.ReadAll(res.Body)
		res.Body.Close()
		if e != nil {
			t.Fatal(e)
		}
		return raw, res
	}
	t.Run("copied OAuth callbacks do not require or transfer management login", func(t *testing.T) {
		callbackURL := fmt.Sprintf("http://127.0.0.1:%d/social/callback?state=invalid&code=fixture-code", s.GetListenedPort())
		response, e := http.Get(callbackURL)
		if e != nil {
			t.Fatal(e)
		}
		defer response.Body.Close()
		raw, e := io.ReadAll(response.Body)
		if e != nil || response.StatusCode != http.StatusOK || response.Header.Get("Cache-Control") != "no-store" || response.Header.Get("Referrer-Policy") != "no-referrer" || response.Header.Get("Content-Security-Policy") == "" || !strings.Contains(string(raw), "original management window") || strings.Contains(string(raw), "fixture-code") {
			t.Fatal("unsafe or misleading authorization callback", response.StatusCode, e)
		}
	})
	t.Run("manual account creation and authorization actions enforce permissions", func(t *testing.T) {
		for _, action := range []string{
			`{"action":"create_manual_account","data":{"name":"Manual personal Facebook"}}`,
			`{"action":"authorization_status","id":123}`,
			`{"action":"authorization_confirm","id":123,"data":{"external_ids":["fixture"]}}`,
			`{"action":"authorization_cancel","id":123}`,
			`{"action":"manual_reminders"}`,
		} {
			raw, _ := post("/social/action", action, "outside")
			if !strings.Contains(string(raw), `"success":false`) {
				t.Fatal("restricted social action accepted", action, string(raw))
			}
		}
		raw, _ := post("/social/action", `{"action":"create_manual_account","data":{"name":"Manual personal Facebook"}}`, "")
		if !strings.Contains(string(raw), `"success":true`) {
			t.Fatal("manual account creation failed", string(raw))
		}
		raw, _ = post("/social/action", `{"action":"manual_reminders"}`, "")
		if !strings.Contains(string(raw), `"success":true`) || !strings.Contains(string(raw), `"total":0`) {
			t.Fatal("manual reminder endpoint failed", string(raw))
		}
	})
	var form bytes.Buffer
	writer := multipart.NewWriter(&form)
	writer.WriteField("plan_id", fmt.Sprint(plan))
	writer.WriteField("ai_allowed", "false")
	file, _ := writer.CreateFormFile("file", "facts.txt")
	file.Write([]byte("Private automotive parts facts"))
	writer.Close()
	req, _ := http.NewRequest("POST", base+"/social/upload", &form)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	res, e := http.DefaultClient.Do(req)
	if e != nil {
		t.Fatal(e)
	}
	var result struct {
		Success bool `json:"success"`
		Data    struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	e = json.NewDecoder(res.Body).Decode(&result)
	res.Body.Close()
	if e != nil || !result.Success {
		t.Fatal("multipart", e, result)
	}
	raw, res := post("/social/file", fmt.Sprintf(`{"id":%q}`, result.Data.ID), "")
	if string(raw) != "Private automotive parts facts" || res.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("file download", string(raw))
	}
	raw, _ = post("/social/file", fmt.Sprintf(`{"id":%q}`, result.Data.ID), "outside")
	if !strings.Contains(string(raw), `"success":false`) {
		t.Fatal("private file exposed")
	}
	for _, body := range []string{fmt.Sprintf(`{"action":"detail","id":%d}`, plan), `{"action":"settings"}`, fmt.Sprintf(`{"action":"save_plan","id":%d}`, plan)} {
		raw, _ = post("/social/action", body, "outside")
		if !strings.Contains(string(raw), `"success":false`) {
			t.Fatal("access bypass", string(raw))
		}
	}
	for _, kind := range []string{"plans", "accounts", "deliveries", "interactions", "jobs"} {
		raw, _ = post("/social/action", fmt.Sprintf(`{"action":"list","kind":%q}`, kind), "")
		if !strings.Contains(string(raw), `"success":true`) {
			t.Fatal(kind, string(raw))
		}
	}
	raw, res = post("/social/export", `{}`, "")
	if len(raw) < 2 || string(raw[:2]) != "PK" || !strings.Contains(res.Header.Get("Content-Disposition"), ".xlsx") {
		t.Fatal("report download")
	}
	if e = service.ManualVariants(ctx, p, plan); e != nil {
		t.Fatal(e)
	}
	variant, e := g.DB().Model("bm_social_variants").Where("plan_id", plan).Value("id")
	if e != nil {
		t.Fatal(e)
	}
	account, e := g.DB().Model("bm_social_accounts").Data(g.Map{"platform": "facebook", "external_id": "fixture-page", "name": "QA Page", "dept_id": p.DeptID, "owner_id": p.ID}).InsertAndGetId()
	if e != nil {
		t.Fatal(e)
	}
	delivery, e := g.DB().Model("bm_social_deliveries").Data(g.Map{"plan_id": plan, "variant_id": variant, "account_id": account, "publisher_id": p.ID, "variant_version": 1, "cycle_at": time.Now(), "due_at": time.Now(), "status": "published", "remote_id": "fixture-post", "snapshot": "{}"}).InsertAndGetId()
	if e != nil {
		t.Fatal(e)
	}
	_, e = g.DB().Model("bm_social_metrics").Data(g.Map{"delivery_id": delivery, "account_id": account, "counts": `{"reactions":5,"comments":0,"views":17}`, "availability": `{"reactions":"available","comments":"no_permission","views":"available"}`, "expires_at": time.Now().Add(time.Hour)}).Insert()
	if e != nil {
		t.Fatal(e)
	}
	raw, _ = post("/social/export", `{}`, "")
	book, e := excelize.OpenReader(bytes.NewReader(raw))
	if e != nil {
		t.Fatal(e)
	}
	reaction, e := book.GetCellValue("Sheet1", "F2")
	if e != nil || reaction != "5" {
		t.Fatal("export lost real metric", reaction, e)
	}
	missing, e := book.GetCellValue("Sheet1", "G2")
	views, viewErr := book.GetCellValue("Sheet1", "N2")
	if viewErr != nil || views != "17" {
		t.Fatal("export lost Facebook views", views, viewErr)
	}
	book.Close()
	if e != nil || missing != "" {
		t.Fatal("unavailable metric exported as zero", missing, e)
	}
	raw, _ = post("/social/action", `{"action":"calendar","data":{"from":"2026-11-01T00:00:00Z","to":"2026-12-01T00:00:00Z"}}`, "")
	if !strings.Contains(string(raw), `"success":true`) {
		t.Fatal("calendar", string(raw))
	}
	if os.Getenv("SOCIAL_LOCAL_BROWSER_TEST") == "1" {
		t.Log("LOCAL_BROWSER_READY", plan, base)
		deadline := time.Now().Add(30 * time.Minute)
		for time.Now().Before(deadline) {
			_, _ = service.ProcessJob(ctx)
			if _, e = os.Stat("/workspace/.tools/social-browser-done"); e == nil {
				break
			}
			time.Sleep(time.Second)
		}
	}
}
