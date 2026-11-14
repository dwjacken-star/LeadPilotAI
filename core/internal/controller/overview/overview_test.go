package overview

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"billionmail-core/internal/service/middlewares"
	"billionmail-core/internal/service/public"
	"billionmail-core/internal/testsupport"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
)

func TestGovernedOverviewHTTP(t *testing.T) {
	testsupport.Database(t)
	ctx := context.Background()
	now := time.Now().Unix()
	for _, query := range []string{
		`INSERT INTO bm_departments(id,name) VALUES(10,'Own'),(20,'Other')`,
		`INSERT INTO account(account_id,username,password,email,status,dept_id) VALUES(101,'overview-only','fixture','',1,10),(102,'no-overview','fixture','',1,10),(201,'foreign-owner','fixture','',1,20)`,
		`INSERT INTO role(role_id,role_name,status,data_scope,permissions) VALUES(101,'overview-only',1,'self','["overview:view"]')`,
		`INSERT INTO account_role(account_id,role_id) VALUES(101,101)`,
		`INSERT INTO email_templates(id,temp_name,content,dept_id,owner_id) VALUES(101,'overview-own','<p>Own fixture</p>',10,101),(201,'overview-foreign','<p>Foreign fixture</p>',20,201)`,
		`INSERT INTO email_tasks(id,task_name,addresser,recipient_count,template_id,dept_id,owner_id) VALUES(101,'Own','sender@own.example',1,101,10,101),(201,'Other','sender@secret.example',1,201,20,201)`,
		`INSERT INTO recipient_info(task_id,recipient,message_id) VALUES(101,'own@client.test','<own-mid>'),(201,'secret@client.test','<secret-mid>')`,
		`INSERT INTO domain(domain) VALUES('own.example'),('secret.example')`,
	} {
		if _, err := g.DB().Exec(ctx, query); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []string{"own", "secret"} {
		for _, query := range []string{
			`INSERT INTO mailstat_message_ids(postfix_message_id,message_id,log_time_millis) VALUES($1,$1||'-mid',$2)`,
			`INSERT INTO mailstat_senders(postfix_message_id,sender,log_time_millis) VALUES($1,'sender@'||$1||'.example',$2)`,
			`INSERT INTO mailstat_send_mails(postfix_message_id,recipient,status,dsn,description,log_time_millis) VALUES($1,$1||'@client.test','bounced','5.1.1',$1||'-failure',$2)`,
		} {
			if _, err := g.DB().Exec(ctx, query, id, now*1000); err != nil {
				t.Fatal(err)
			}
		}
	}
	public.SetCache("postfix_deferred_queue_count", 987, 300)
	server := g.Server("governed-overview-http")
	server.SetAddr("127.0.0.1:0")
	server.SetDumpRouterMap(false)
	server.Group("/api", func(group *ghttp.RouterGroup) {
		group.Middleware(func(r *ghttp.Request) {
			r.SetCtxVar("accountId", r.Header.Get("X-Test-Account"))
			r.Middleware.Next()
		})
		group.Middleware(middlewares.HandleApiResponse)
		group.Bind(NewV1())
	})
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	defer server.Shutdown()
	call := func(path, account string) g.Map {
		t.Helper()
		url := fmt.Sprintf("http://127.0.0.1:%d/api%s", server.GetListenedPort(), path)
		request, _ := http.NewRequest(http.MethodGet, url, nil)
		request.Header.Set("X-Test-Account", account)
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		var result g.Map
		if err = json.NewDecoder(response.Body).Decode(&result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	params := fmt.Sprintf("?start_time=%d&end_time=%d", now-3600, now+1)
	for _, account := range []string{"101", "1"} {
		result := call("/overview"+params, account)
		if result["success"] != true {
			t.Fatal("overview HTTP failed", result)
		}
		data := result["data"].(map[string]interface{})
		dashboard := data["dashboard"].(map[string]interface{})
		want := float64(1)
		if account == "1" {
			want = 2
		}
		if dashboard["sends"] != want || data["queue_access"] != (account == "1") {
			t.Fatal("HTTP scope or queue capability incorrect", data)
		}
		result = call("/overview/failed"+params, account)
		if result["success"] != true || len(result["data"].([]interface{})) != int(want) {
			t.Fatal("failure HTTP scope incorrect", result)
		}
		result = call("/overview/domains", account)
		if result["success"] != true || len(result["data"].([]interface{})) != int(want) {
			t.Fatal("domains HTTP scope incorrect", result)
		}
		if account == "101" && result["data"].([]interface{})[0] != "own.example" {
			t.Fatal("unrelated domain exposed", result)
		}
	}
	for _, path := range []string{"/overview" + params, "/overview/failed" + params, "/overview/domains"} {
		result := call(path, "102")
		if result["success"] == true || result["code"] != float64(403) {
			t.Fatal("Overview permission not enforced", path, result)
		}
	}
}
