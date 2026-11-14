package batch_mail

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"billionmail-core/internal/controller/enterprise"
	"billionmail-core/internal/service/middlewares"
	"billionmail-core/internal/testsupport"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
)

func TestCampaignAuthoringHTTP(t *testing.T) {
	testsupport.Database(t)
	ctx := context.Background()
	_, e := g.DB().Exec(ctx, `INSERT INTO bm_contact_groups(name,token) VALUES('authoring','authoring');INSERT INTO email_templates(temp_name,content) VALUES('library','<p>Original library</p>');INSERT INTO bm_contacts(email,group_id,active,status) VALUES('a@example.com',1,1,1);INSERT INTO account(username,password,email,status) VALUES('outside','fixture','',1)`)
	if e != nil {
		t.Fatal(e)
	}
	server := g.Server("campaign-authoring-http")
	server.SetAddr("127.0.0.1:0")
	server.SetDumpRouterMap(false)
	server.Group("/api", func(group *ghttp.RouterGroup) {
		group.Middleware(func(r *ghttp.Request) {
			id := int64(1)
			if r.Header.Get("X-Test-Outside") == "1" {
				id = 2
			}
			r.SetCtxVar("accountId", id)
			r.Middleware.Next()
		})
		group.Middleware(enterprise.LegacyBoundary)
		group.Middleware(middlewares.HandleApiResponse)
		group.Bind(NewV1())
	})
	if e = server.Start(); e != nil {
		t.Fatal(e)
	}
	defer server.Shutdown()
	base := fmt.Sprintf("http://127.0.0.1:%d/api", server.GetListenedPort())
	call := func(method, path string, body g.Map, outside bool) g.Map {
		t.Helper()
		encoded, _ := json.Marshal(body)
		req, _ := http.NewRequest(method, base+path, strings.NewReader(string(encoded)))
		req.Header.Set("Content-Type", "application/json")
		if outside {
			req.Header.Set("X-Test-Outside", "1")
		}
		res, e := http.DefaultClient.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		defer res.Body.Close()
		var result g.Map
		if e = json.NewDecoder(res.Body).Decode(&result); e != nil {
			t.Fatal(e)
		}
		return result
	}
	brief := g.Map{"mode": "ai", "country": "TH", "language": "th", "model": "selected-fixture", "requirement": "Introduce automotive compressors"}
	html := `<p style="margin:0 0 16px">First draft.</p><p><strong>Second paragraph.</strong></p>`
	result := call("POST", "/batch_mail/task/create", g.Map{"subject": "Draft subject", "full_name": "BESTMAPLE Sales", "senders": []string{"sender@example.com"}, "group_ids": []int{1}, "template_id": 0, "html_content": html, "authoring_config": brief, "start_time": time.Now().Add(72 * time.Hour).Unix(), "warmup": 0, "is_record": 1}, false)
	if result["success"] != true {
		t.Fatal("inline create HTTP", result)
	}
	id := int64(result["data"].(map[string]interface{})["id"].(float64))
	detail := call("GET", fmt.Sprintf("/batch_mail/task/find?id=%d", id), nil, false)
	data := detail["data"].(map[string]interface{})
	if data["snapshot_content"] != html || !strings.Contains(data["authoring_config"].(string), "selected-fixture") {
		t.Fatal("content and authoring preferences did not round trip", detail)
	}
	changed := `<p>Revised content.</p><p>Keep <em>formatting</em>.</p>`
	result = call("POST", "/batch_mail/task/update", g.Map{"task_id": id, "subject": "Revised subject", "full_name": "BESTMAPLE Team", "template_id": 1, "html_content": changed, "authoring_config": brief, "track_open": 1, "track_click": 0, "unsubscribe": 1, "warmup": 0}, false)
	if result["success"] != true {
		t.Fatal("inline update HTTP", result)
	}
	row, e := g.DB().Model("email_tasks").Where("id", id).One()
	if e != nil || row["snapshot_content"].String() != changed || row["template_id"].Int() != 1 || row["track_open"].Int() != 1 || row["track_click"].Int() != 0 {
		t.Fatal("edit snapshot or switches not saved", row, e)
	}
	value, _ := g.DB().Model("email_templates").Where("id", 1).Value("content")
	if value.String() != "<p>Original library</p>" {
		t.Fatal("library modified")
	}
	result = call("POST", "/batch_mail/task/update", g.Map{"task_id": id, "subject": "unauthorized", "html_content": "<p>attack</p>"}, true)
	if result["success"] == true {
		t.Fatal("unscoped legacy edit allowed")
	}
	_, e = g.DB().Model("email_tasks").Where("id", id).Data(g.Map{"task_process": 1, "pause": 0}).Update()
	if e != nil {
		t.Fatal(e)
	}
	result = call("POST", "/batch_mail/task/update", g.Map{"task_id": id, "subject": "must pause first", "html_content": "<p>changed while running</p>"}, false)
	if result["success"] == true {
		t.Fatal("running task changed")
	}
	value, _ = g.DB().Model("email_tasks").Where("id", id).Value("snapshot_content")
	if value.String() != changed {
		t.Fatal("failed edit overwrote snapshot")
	}
}
