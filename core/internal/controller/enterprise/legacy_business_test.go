package enterprise

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	legacybatch "billionmail-core/internal/controller/batch_mail"
	legacytemplate "billionmail-core/internal/controller/email_template"
	"billionmail-core/internal/service/middlewares"
	"billionmail-core/internal/service/public"
	"billionmail-core/internal/testsupport"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
)

func TestLegacyMarketingIsolation(t *testing.T) {
	testsupport.Database(t)
	ctx := context.Background()
	_ = public.Cache.Set(ctx, "ServerIP", "127.0.0.1", time.Hour)
	_, err := g.DB().Exec(ctx, `
		INSERT INTO bm_departments(id,name,parent_id) VALUES(2,'Marketing A',1),(3,'Marketing B',1);
		INSERT INTO account(account_id,username,password,email,status,dept_id) VALUES
		  (2,'marketing-a','test','',1,2),(3,'marketing-b','test','',1,3),(4,'same-department','test','',1,2);
		INSERT INTO role(role_id,role_name,status,data_scope,permissions) VALUES
		  (2,'marketing-self',1,'self','["campaign:list","campaign:create","campaign:update","campaign:delete","campaign:pause","template:list"]'),
		  (3,'unrelated-wide-read',1,'all','["contact:list"]');
		INSERT INTO account_role(account_id,role_id) VALUES(2,2),(2,3);
		INSERT INTO mailbox(username,password,password_encode,full_name,maildir,local_part,domain,active)
		  VALUES('private@example.com','test','','Private','test/','private','example.com',1);
		INSERT INTO email_tasks(id,task_name,subject,addresser,dept_id,owner_id,pause,task_process,create_time,recipient_count,template_id)
		  VALUES(101,'match-own','match own','sender@example.com',2,2,1,0,1,0,0),
		  (102,'match-other','match other department','sender@example.com',3,3,1,0,2,0,0),
		  (103,'match-peer','match same department','sender@example.com',2,4,1,0,3,0,0);
		INSERT INTO email_templates(id,temp_name,content,dept_id,owner_id)
		  VALUES(101,'own-template','<p>own</p>',2,2),(102,'other-template','<p>private</p>',3,3),
		  (103,'peer-template','<p>peer</p>',2,4);
		INSERT INTO recipient_info(task_id,recipient,message_id) VALUES
		  (101,'own@client.test','shared'),(102,'private@foreign.test','shared'),
		  (101,'blank@client.test',''),(101,'ambiguous@client.test','ambiguous'),(102,'ambiguous@client.test','ambiguous');
		INSERT INTO mailstat_message_ids(postfix_message_id,message_id) VALUES
		  ('OWN','shared'),('FOREIGN','shared'),('BLANK',''),('AMBIGUOUS','ambiguous');
		INSERT INTO mailstat_send_mails(postfix_message_id,recipient,status,dsn,log_time_millis) VALUES
		  ('OWN','own@client.test','sent','2.0.0',extract(epoch FROM now())::bigint*1000-60000),
		  ('FOREIGN','private@foreign.test','sent','2.0.0',extract(epoch FROM now())::bigint*1000-60000),
		  ('BLANK','blank@client.test','sent','2.0.0',extract(epoch FROM now())::bigint*1000-60000),
		  ('AMBIGUOUS','ambiguous@client.test','sent','2.0.0',extract(epoch FROM now())::bigint*1000-60000);`)
	if err != nil {
		t.Fatal(err)
	}
	server := g.Server("legacy-marketing-isolation")
	server.SetAddr("127.0.0.1:0")
	server.SetDumpRouterMap(false)
	server.Group("/api", func(group *ghttp.RouterGroup) {
		group.Middleware(func(r *ghttp.Request) {
			account := int64(2)
			if r.Header.Get("X-Test-Administrator") == "yes" {
				account = 1
			}
			r.SetCtxVar("accountId", account)
			r.Middleware.Next()
		})
		group.Middleware(LegacyBoundary)
		group.Middleware(middlewares.HandleApiResponse)
		group.Bind(legacybatch.NewV1())
		group.Bind(legacytemplate.NewV1())
	})
	if err = server.Start(); err != nil {
		t.Fatal(err)
	}
	defer server.Shutdown()
	call := func(method, path string, body interface{}, admin bool) Response {
		t.Helper()
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		request, err := http.NewRequest(method, fmt.Sprintf("http://127.0.0.1:%d/api%s", server.GetListenedPort(), path), bytes.NewReader(encoded))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Content-Type", "application/json")
		if admin {
			request.Header.Set("X-Test-Administrator", "yes")
		}
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		var result Response
		if err = json.NewDecoder(response.Body).Decode(&result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	const taskList = "/batch_mail/task/list?page=1&page_size=20&status=-1"
	for _, path := range []string{taskList, taskList + "&keyword=match", taskList + "&keyword=other"} {
		result := call("GET", path, nil, false)
		if !result.Success {
			t.Fatalf("ordinary legacy marketing list denied: %+v", result)
		}
		data := result.Data.(map[string]interface{})
		want := float64(1)
		if path == taskList+"&keyword=other" {
			want = 0
		}
		if data["total"] != want {
			t.Fatalf("scope/OR keyword bypass: %+v", data)
		}
		for _, raw := range data["list"].([]interface{}) {
			if raw.(map[string]interface{})["id"] != float64(101) {
				t.Fatal("foreign task returned", raw)
			}
		}
	}
	admin := call("GET", taskList, nil, true)
	if !admin.Success || admin.Data.(map[string]interface{})["total"] != float64(3) {
		t.Fatalf("administrator list changed: %+v", admin)
	}
	templates := call("GET", "/email_template/all", nil, false)
	if !templates.Success || len(templates.Data.([]interface{})) != 1 {
		t.Fatalf("template selector is not self-scoped: %+v", templates)
	}
	logs := call("GET", "/batch_mail/tracking/logs?task_id=101&status=1", nil, false)
	if !logs.Success {
		t.Fatalf("own-task log read failed: %+v", logs)
	}
	logData := logs.Data.(map[string]interface{})
	if logData["total"] != float64(1) || logData["list"].([]interface{})[0].(map[string]interface{})["recipient"] != "own@client.test" {
		t.Fatal("foreign, blank or ambiguous message identity leaked into task logs", logData)
	}
	providers := call("GET", "/batch_mail/tracking/mail_provider?task_id=101&status=1", nil, false)
	if !providers.Success || len(providers.Data.([]interface{})) != 1 || providers.Data.([]interface{})[0].(map[string]interface{})["count"] != float64(1) {
		t.Fatal("provider statistics attributed foreign deliveries to an owned task", providers)
	}
	for _, endpoint := range []string{
		"/batch_mail/task/find?id=102", "/batch_mail/task/find?id=103",
		"/batch_mail/task/stat_chart?task_id=102&start_time=1&end_time=2",
		"/batch_mail/tracking/logs?task_id=102", "/batch_mail/tracking/mail_provider?task_id=102",
		"/batch_mail/task/send_count?task_id=102", "/email_template/get?id=102",
		"/batch_mail/task/overview?id=101", "/batch_mail/task/stat_chart?task_id=0",
	} {
		result := call("GET", endpoint, nil, false)
		if result.Success || result.Code != 403 {
			t.Fatalf("unscoped legacy access allowed: %s %+v", endpoint, result)
		}
	}
	for _, operation := range []struct {
		path string
		body g.Map
	}{
		{"/batch_mail/task/update", g.Map{"task_id": 102, "subject": "stolen"}},
		{"/batch_mail/task/delete", g.Map{"id": 102}},
		{"/batch_mail/task/resume", g.Map{"task_id": 101}},
		{"/batch_mail/task/send_test", g.Map{"recipient": "nobody@example.com"}},
	} {
		result := call("POST", operation.path, operation.body, false)
		if result.Success || result.Code != 403 {
			t.Fatalf("forbidden legacy mutation reached controller: %s %+v", operation.path, result)
		}
	}
	update := call("POST", "/batch_mail/task/update", g.Map{"task_id": 101, "subject": "safe edit", "warmup": 0}, false)
	if !update.Success {
		t.Fatalf("authorized own-task edit failed: %+v", update)
	}
	_ = call("POST", "/batch_mail/task/update?task_id=101", g.Map{"task_id": 102, "subject": "ambiguous"}, false)
	foreign, err := g.DB().Model("email_tasks").Where("id", 102).Value("subject")
	if err != nil || foreign.String() != "match other department" {
		t.Fatal("conflicting query/body ID bypassed scope", foreign, err)
	}
	count := call("GET", "/batch_mail/task/send_count?task_id=102", g.Map{"task_id": 101}, false)
	if count.Success || count.Code != 403 {
		t.Fatal("query-only task ID was authorized using the body", count)
	}
	sender := call("POST", "/batch_mail/task/update", g.Map{"task_id": 101, "subject": "ungranted sender", "senders": []string{"private@example.com"}}, false)
	if sender.Success {
		t.Fatal("ungranted sender update allowed")
	}
	if _, err = g.DB().Exec(ctx, `UPDATE email_tasks SET task_process=2 WHERE id=101`); err != nil {
		t.Fatal(err)
	}
	terminal := call("POST", "/batch_mail/task/update", g.Map{"task_id": 101, "subject": "terminal edit"}, false)
	if terminal.Success {
		t.Fatal("completed task update allowed")
	}
	if _, err = g.DB().Exec(ctx, `INSERT INTO bm_contact_groups(id,name,dept_id,owner_id) VALUES(201,'Draft audience',2,2);
		INSERT INTO bm_resource_grants(dept_id,sender) VALUES(2,'sender@example.com')`); err != nil {
		t.Fatal(err)
	}
	draft := call("POST", "/batch_mail/task/create", g.Map{
		"subject": "draft only", "addresser": "sender@example.com", "group_id": 201,
		"html_content": "<p>draft</p>", "start_time": time.Now().Unix(), "warmup": 0,
	}, false)
	if !draft.Success {
		t.Fatalf("create-only role cannot save a draft: %+v", draft)
	}
	saved, err := g.DB().Model("email_tasks").Where("subject", "draft only").One()
	if err != nil || len(saved) == 0 || saved["pause"].Int() != 1 || saved["owner_id"].Int() != 2 {
		t.Fatal("create-only role auto-published or lost ownership", saved, err)
	}
	if _, err = g.DB().Exec(ctx, `UPDATE role SET permissions='["campaign:list","template:list"]' WHERE role_id=2`); err != nil {
		t.Fatal(err)
	}
	for _, operation := range []struct {
		path string
		body g.Map
	}{
		{"/batch_mail/task/update", g.Map{"task_id": 101, "subject": "read-only"}},
		{"/batch_mail/task/delete", g.Map{"id": 101}},
		{"/batch_mail/task/create", g.Map{}},
		{"/batch_mail/task/pause", g.Map{"task_id": 101}},
	} {
		result := call("POST", operation.path, operation.body, false)
		if result.Success || result.Code != 403 {
			t.Fatalf("read-only role can mutate via legacy API: %s %+v", operation.path, result)
		}
	}
}
