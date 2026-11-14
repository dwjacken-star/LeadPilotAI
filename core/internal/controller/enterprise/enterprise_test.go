package enterprise

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"billionmail-core/internal/service/contactimport"
	"billionmail-core/internal/service/governance"
	"billionmail-core/internal/service/middlewares"
	"billionmail-core/internal/testsupport"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
)

func testSenderOptionsCountAndScope(t *testing.T) {
	ctx := context.Background()
	_, err := g.DB().Exec(ctx, `
		INSERT INTO bm_departments(id,name,parent_id) VALUES(20,'Sender department',1),(21,'Other sender department',1),(22,'No grants',1);
		INSERT INTO mailbox(username,password,password_encode,full_name,maildir,local_part,domain,active) VALUES
		('zeta@example.com','test','','Zeta','test/','zeta','example.com',1),
		('disabled@example.com','test','','Disabled','test/','disabled','example.com',0);
		INSERT INTO bm_resource_grants(dept_id,sender,active) VALUES
		(20,'sender@example.com',true),(20,'zeta@example.com',false),(20,'disabled@example.com',true),
		(21,'zeta@example.com',true)`)
	if err != nil {
		t.Fatal(err)
	}
	admin := &governance.Principal{Super: true, DeptID: 1}
	member := &governance.Principal{DeptID: 20}
	for _, tc := range []struct {
		name      string
		principal *governance.Principal
		page      int
		keyword   string
		ids       []string
		total     int
		username  string
	}{
		{"admin first page", admin, 1, "", nil, 2, "sender@example.com"},
		{"admin second page", admin, 2, "", nil, 2, "zeta@example.com"},
		{"admin empty page", admin, 3, "", nil, 2, ""},
		{"case insensitive search", admin, 1, "ZETA", nil, 1, "zeta@example.com"},
		{"selected mailbox hydration", admin, 1, "", []string{"zeta@example.com"}, 1, "zeta@example.com"},
		{"unknown mailbox", admin, 1, "", []string{"missing@example.com"}, 0, ""},
		{"department active grants only", member, 1, "", nil, 1, "sender@example.com"},
		{"ungranted mailbox selection", member, 1, "", []string{"zeta@example.com"}, 0, ""},
		{"other department", &governance.Principal{DeptID: 21}, 1, "", nil, 1, "zeta@example.com"},
		{"no grants", &governance.Principal{DeptID: 22}, 1, "", nil, 0, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := perform(ctx, tc.principal, &Request{Action: "list", Type: "senders", Page: tc.page, PageSize: 1, Keyword: tc.keyword, Data: g.Map{"ids": tc.ids}})
			if err != nil {
				t.Fatal(err)
			}
			encoded, err := json.Marshal(result)
			if err != nil {
				t.Fatal(err)
			}
			var response struct {
				List []struct {
					Username string `json:"username"`
					Domain   string `json:"domain"`
					Active   int    `json:"active"`
				} `json:"list"`
				Total int `json:"total"`
			}
			if err = json.Unmarshal(encoded, &response); err != nil {
				t.Fatal(err)
			}
			if response.Total != tc.total || response.List == nil {
				t.Fatalf("incorrect count or null collection: %s", encoded)
			}
			if tc.username == "" {
				if len(response.List) != 0 {
					t.Fatalf("expected empty options: %s", encoded)
				}
			} else if len(response.List) != 1 || response.List[0].Username != tc.username || response.List[0].Domain != "example.com" || response.List[0].Active != 1 {
				t.Fatalf("incorrect mailbox options: %s", encoded)
			}
		})
	}
	t.Run("DefaultDomainPreferencePreservesGrantsAndExplicitSender", func(t *testing.T) {
		old, err := g.DB().Model("bm_options").Where("name", "default_sender_domain").Value("value")
		if err != nil {
			t.Fatal(err)
		}
		_, err = g.DB().Exec(ctx, `INSERT INTO mailbox(username,password,password_encode,full_name,maildir,local_part,domain,active) VALUES('zz-preferred@z-preferred.example','test','','Preferred','test/','zz-preferred','z-preferred.example',1); INSERT INTO bm_options(name,value) VALUES('default_sender_domain','z-preferred.example') ON CONFLICT(name) DO UPDATE SET value=excluded.value`)
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			_, _ = g.DB().Model("mailbox").Where("username", "zz-preferred@z-preferred.example").Delete()
			if old.IsNil() {
				_, _ = g.DB().Model("bm_options").Where("name", "default_sender_domain").Delete()
			} else {
				_, _ = g.DB().Model("bm_options").Where("name", "default_sender_domain").Data(g.Map{"value": old.String()}).Update()
			}
		}()
		for _, tc := range []struct {
			p    *governance.Principal
			want string
		}{{admin, "zz-preferred@z-preferred.example"}, {member, "sender@example.com"}, {&governance.Principal{DeptID: 22}, ""}} {
			result, err := perform(ctx, tc.p, &Request{Action: "list", Type: "senders", Page: 1, PageSize: 1, Data: g.Map{}})
			if err != nil {
				t.Fatal(err)
			}
			if got := result.(g.Map)["default_sender"]; got != tc.want {
				t.Fatalf("default sender = %v, want %s", got, tc.want)
			}
			if tc.want != "" {
				sender, err := contactimport.ResolveSender(ctx, tc.p, "")
				if err != nil || sender != tc.want {
					t.Fatal("import default sender", sender, err)
				}
			}
		}
		sender, err := contactimport.ResolveSender(ctx, member, "sender@example.com")
		if err != nil || sender != "sender@example.com" {
			t.Fatal("explicit sender overwritten", sender, err)
		}
		if _, err = contactimport.ResolveSender(ctx, member, "zz-preferred@z-preferred.example"); err == nil {
			t.Fatal("default preference bypassed department authorization")
		}
	})
}

func TestDepartmentBusinessIsolation(t *testing.T) {
	testsupport.Database(t)
	t.Run("ListLabelsPreserveScopeAndPagination", testListLabelsPreserveScopeAndPagination)
	ctx := context.Background()
	_, e := g.DB().Exec(ctx, `INSERT INTO bm_departments(name,parent_id) VALUES('A',1),('B',1); INSERT INTO account(username,password,email,status,dept_id) VALUES('a','hash','',1,2),('b','hash','',1,3); INSERT INTO role(role_name,status,data_scope,permissions) VALUES('department-editor',1,'department','["contact:list","contact:create","contact:update","contact:delete","contact:import","contact:export","template:list","template:create","campaign:list","campaign:create","campaign:publish"]');INSERT INTO account_role(account_id,role_id) VALUES(2,2),(3,2)`)
	if e != nil {
		t.Fatal(e)
	}
	a, e := governance.Load(ctx, 2)
	if e != nil {
		t.Fatal(e)
	}
	b, e := governance.Load(ctx, 3)
	if e != nil {
		t.Fatal(e)
	}
	admin, e := governance.Load(ctx, 1)
	if e != nil {
		t.Fatal(e)
	}
	// Empty collections must remain JSON arrays for table and remote-select consumers.
	for _, kind := range []string{"tasks", "schedules", "automations", "contacts", "tags", "reports"} {
		empty, err := perform(ctx, admin, &Request{Action: "list", Type: kind, Data: g.Map{}, Page: 1, PageSize: 20})
		if err != nil {
			t.Fatalf("empty %s list: %v", kind, err)
		}
		encoded, err := json.Marshal(empty)
		if err != nil || !bytes.Contains(encoded, []byte(`"list":[]`)) {
			t.Fatalf("empty %s must serialize as []: %s (%v)", kind, encoded, err)
		}
	}
	newRole, e := perform(ctx, admin, &Request{Action: "save", Type: "roles", Data: g.Map{"role_name": "orm-created", "status": 1, "data_scope": "department", "permissions": []string{"template:list"}}})
	if e != nil {
		t.Fatal(e)
	}
	newUser, e := perform(ctx, admin, &Request{Action: "save", Type: "users", Data: g.Map{"username": "orm-created", "password": "test-password-123", "status": 1, "dept_id": 3, "role_ids": []int64{newRole.(g.Map)["id"].(int64)}}})
	if e != nil {
		t.Fatal(e)
	}
	created, e := governance.Load(ctx, newUser.(g.Map)["id"].(int64))
	if e != nil || created.DeptID != 3 || !created.Has("template:list") {
		t.Fatalf("migration left stale ORM field metadata: %+v %v", created, e)
	}
	t.Run("AssignableBusinessPermissions", func(t *testing.T) {
		_, err := perform(ctx, admin, &Request{Action: "save", Type: "roles", ID: newRole.(g.Map)["id"].(int64), Data: g.Map{
			"role_name": "orm-created", "status": 1, "data_scope": "department",
			"permissions": []string{"campaign:list", "template:list", "contact:list"},
		}})
		if err != nil {
			t.Fatal(err)
		}
		businessUser, err := governance.Load(ctx, created.ID)
		if err != nil {
			t.Fatal(err)
		}
		for _, kind := range []string{"tasks", "templates", "groups", "contacts", "tags"} {
			if _, err = perform(ctx, businessUser, &Request{Action: "list", Type: kind, Data: g.Map{}}); err != nil {
				t.Fatalf("assigned business list %s: %v", kind, err)
			}
		}
		if _, err = perform(ctx, businessUser, &Request{Action: "save", Type: "templates", Data: g.Map{"temp_name": "forbidden"}}); err == nil {
			t.Fatal("view-only template role can create")
		}
		if _, err = perform(ctx, businessUser, &Request{Action: "list", Type: "roles", Data: g.Map{}}); err == nil {
			t.Fatal("business grants expose role administration")
		}
	})
	restricted := *a
	restricted.Roles = []governance.Role{{Scope: "custom", Custom: []int64{3}, Permissions: []string{"contact:create"}}}
	if _, e = perform(ctx, &restricted, &Request{Action: "save", Type: "groups", Data: g.Map{"name": "out-of-scope-create"}}); e == nil {
		t.Fatal("create ignored operation data scope")
	}
	result, e := perform(ctx, a, &Request{Action: "save", Type: "groups", Data: g.Map{"name": "same-name"}})
	if e != nil {
		t.Fatal(e)
	}
	group := result.(g.Map)["id"].(int64)
	if _, e = perform(ctx, b, &Request{Action: "save", Type: "groups", ID: group, Data: g.Map{"name": "stolen"}}); e == nil {
		t.Fatal("cross-department update allowed")
	}
	if _, e = perform(ctx, b, &Request{Action: "delete", Type: "groups", ID: group}); e == nil {
		t.Fatal("cross-department delete allowed")
	}
	if _, e = perform(ctx, b, &Request{Action: "save", Type: "groups", Data: g.Map{"name": "same-name"}}); e != nil {
		t.Fatal("name should only be unique inside department:", e)
	}
	if _, e = perform(ctx, a, &Request{Action: "save", Type: "contacts", Data: g.Map{"group_id": group, "email": "user@example.com", "status": 1, "active": 1}}); e != nil {
		t.Fatal(e)
	}
	listed, e := perform(ctx, b, &Request{Action: "list", Type: "contacts", Data: g.Map{}})
	if e != nil {
		t.Fatal(e)
	}
	if listed.(g.Map)["total"].(int) != 0 {
		t.Fatal("contact list leaked another department")
	}
	exported, e := perform(ctx, b, &Request{Action: "export", Data: g.Map{}})
	if e != nil {
		t.Fatal(e)
	}
	if len(exported.(g.Map)["csv"].(string)) > 50 {
		t.Fatal("export leaked contacts")
	}
	if _, e = perform(ctx, b, &Request{Action: "save", Type: "users", Data: g.Map{"username": "evil"}}); e == nil {
		t.Fatal("department user can administer accounts")
	}
	if e = governance.SenderAllowed(ctx, a, "sender@example.com"); e == nil {
		t.Fatal("ungranted sender allowed")
	}
	_, e = g.DB().Exec(ctx, `INSERT INTO bm_resource_grants(dept_id,sender) VALUES(2,'sender@example.com')`)
	if e != nil {
		t.Fatal(e)
	}
	if e = governance.SenderAllowed(ctx, a, "sender@example.com"); e != nil {
		t.Fatal(e)
	}
	preview, e := perform(ctx, a, &Request{Action: "preview_import", Data: g.Map{"group_id": group, "csv": "email,name\nvalid@example.com,A\ninvalid,B\nSPAM@example.com,C\n", "confirmed": true, "filter_keywords": []string{"spam"}}})
	if e != nil {
		t.Fatal(e)
	}
	if preview.(g.Map)["valid"].(int) != 1 || preview.(g.Map)["filtered_count"].(int) != 1 {
		t.Fatal(preview)
	}
	if _, e = perform(ctx, a, &Request{Action: "import", Data: g.Map{"group_id": group, "csv": "email,name\nvalid@example.com,A\ninvalid,B\n", "confirmed": true}}); e != nil {
		t.Fatal(e)
	}
	n, e := g.DB().Model("bm_contacts").Where("group_id", group).Count()
	if e != nil || n != 2 {
		t.Fatalf("valid-only import failed: %d %v", n, e)
	}
	// Exercise actual GoFrame binding and the legacy boundary over HTTP as well.
	s := g.Server("department-http-test")
	s.SetAddr("127.0.0.1:0")
	s.SetDumpRouterMap(false)
	s.Group("/api", func(group *ghttp.RouterGroup) {
		group.Middleware(func(r *ghttp.Request) { r.SetCtxVar("accountId", 2); r.Middleware.Next() })
		group.Middleware(LegacyBoundary)
		group.Middleware(middlewares.HandleApiResponse)
		group.Bind(New())
		group.GET("/legacy-secret", func(r *ghttp.Request) { r.Response.Write("secret") })
	})
	if e = s.Start(); e != nil {
		t.Fatal(e)
	}
	defer s.Shutdown()
	endpoint := fmt.Sprintf("http://127.0.0.1:%d/api/enterprise/action", s.GetListenedPort())
	body := bytes.NewBufferString(`{"action":"list","type":"groups","data":{}}`)
	response, e := http.Post(endpoint, "application/json", body)
	if e != nil {
		t.Fatal(e)
	}
	var actual Response
	if e = json.NewDecoder(response.Body).Decode(&actual); e != nil {
		t.Fatal(e)
	}
	response.Body.Close()
	if !actual.Success {
		t.Fatalf("HTTP action failed: %+v", actual)
	}
	response, e = http.Get(fmt.Sprintf("http://127.0.0.1:%d/api/legacy-secret", s.GetListenedPort()))
	if e != nil {
		t.Fatal(e)
	}
	_ = json.NewDecoder(response.Body).Decode(&actual)
	response.Body.Close()
	if actual.Code != 403 {
		t.Fatalf("legacy route exposed: %+v", actual)
	}
	_, _ = g.DB().Exec(ctx, `UPDATE bm_departments SET active=false WHERE id=2`)
	if _, e = governance.Load(ctx, 2); e == nil {
		t.Fatal("disabled department can authenticate")
	}
	t.Run("SenderOptionsCountAndScope", testSenderOptionsCountAndScope)
}
