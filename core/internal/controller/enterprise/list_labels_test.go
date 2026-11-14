package enterprise

import (
	"context"
	"testing"

	"billionmail-core/internal/service/governance"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

func testListLabelsPreserveScopeAndPagination(t *testing.T) {
	ctx := context.Background()
	t.Cleanup(func() {
		_, e := g.DB().Exec(ctx, `DELETE FROM bm_security_audit WHERE dept_id IN(2,3); DELETE FROM bm_campaign_runs; DELETE FROM bm_automation_enrollments; DELETE FROM bm_automation_rules; DELETE FROM bm_campaign_schedules; DELETE FROM email_tasks; DELETE FROM email_templates; DELETE FROM bm_contacts; DELETE FROM bm_tags; DELETE FROM bm_contact_groups; DELETE FROM bm_send_policies WHERE scope='department' AND scope_key='2'; DELETE FROM bm_resource_grants WHERE dept_id IN(2,3); DELETE FROM account_role WHERE account_id=2; DELETE FROM account WHERE account_id=2; DELETE FROM role WHERE role_id=2; DELETE FROM bm_departments WHERE id IN(2,3);`)
		if e != nil {
			t.Error("isolated label fixtures cleanup", e)
		}
	})
	_, err := g.DB().Exec(ctx, `
INSERT INTO bm_departments(id,name,parent_id) VALUES(2,'外贸一部',1),(3,'保密部门',1);
INSERT INTO role(role_id,role_name,status,data_scope,permissions,custom_depts) VALUES(2,'销售经理',1,'custom','["contact:list"]','[2]');
INSERT INTO account(account_id,username,password,email,status,dept_id) VALUES(2,'销售成员','hash','',1,2);
INSERT INTO account_role(account_id,role_id) VALUES(2,2);
INSERT INTO bm_resource_grants(dept_id,sender) VALUES(2,'sender@example.com');
INSERT INTO bm_contact_groups(id,name,token,dept_id,owner_id) VALUES(501,'欧洲客户','eu',2,1),(502,'保密客户','hidden',3,1);
INSERT INTO bm_contact_groups(name,token,dept_id,owner_id) SELECT '分页分组'||i,'page-'||i,2,1 FROM generate_series(1,220) i;
INSERT INTO bm_contact_groups(id,name,token,dept_id,owner_id) VALUES(999,'分页之外的分组','last',2,1);
INSERT INTO bm_tags(id,name,group_id,dept_id,owner_id) VALUES(1,'重点客户',501,2,1),(2,'跨页标签',999,2,1),(3,'损坏关联',502,2,1),(4,'其他部门标签',502,3,1);
INSERT INTO bm_contacts(id,email,group_id,active,status,dept_id,owner_id) VALUES(1,'buyer@example.com',501,1,1,2,1);
INSERT INTO email_templates(id,temp_name,content,dept_id,owner_id) VALUES(1,'中文报价模板','Hello',2,1);
INSERT INTO email_tasks(id,task_name,addresser,recipient_count,group_id,group_ids,template_id,dept_id,owner_id) VALUES(1,'欧洲促销任务','sender@example.com',1,501,'[501,999]',1,2,1);
INSERT INTO bm_campaign_schedules(id,name,config,cadence,next_run_at,dept_id,owner_id,active) VALUES(1,'每周计划','{"group_ids":[999,501],"template_id":1}','weekly',now()+interval '10 days',2,1,false);
INSERT INTO bm_campaign_runs(schedule_id,due_at,task_id) VALUES(1,now(),1);
INSERT INTO bm_automation_rules(id,name,group_id,config,dept_id,owner_id,active) VALUES(1,'跟进规则',501,'{}',2,1,false);
INSERT INTO bm_automation_enrollments(rule_id,contact_id,version,config) VALUES(1,1,1,'{}');
INSERT INTO bm_send_policies(scope,scope_key,policy) VALUES('department','2','{}');
INSERT INTO bm_security_audit(account_id,dept_id,action,resource_id,detail) VALUES(1,2,'save:groups',501,'{}');`)
	if err != nil {
		t.Fatal(err)
	}
	admin := &governance.Principal{ID: 1, DeptID: 1, Super: true}
	member := &governance.Principal{ID: 1, DeptID: 2, Roles: []governance.Role{{Scope: "department", Permissions: []string{"contact:list", "template:list", "campaign:list", "schedule:list", "automation:list", "report:list"}}}}
	listingRows := func(p *governance.Principal, kind string, id int64, page int) (gdb.Result, int) {
		t.Helper()
		data, e := list(ctx, p, &Request{Type: kind, ID: id, Page: page, PageSize: 1, Data: g.Map{}})
		if e != nil {
			t.Fatal(kind, e)
		}
		m := data.(g.Map)
		return m["list"].(gdb.Result), m["total"].(int)
	}
	for _, tc := range []struct {
		kind, key, want string
		id              int64
	}{
		{"contacts", "group_name", "欧洲客户", 0}, {"templates", "dept_name", "外贸一部", 0}, {"reports", "dept_name", "外贸一部", 0},
		{"automations", "group_name", "欧洲客户", 0}, {"runs", "task_name", "欧洲促销任务", 1}, {"enrollments", "contact_email", "buyer@example.com", 1},
		{"policies", "scope_name", "外贸一部", 0}, {"audit", "resource_name", "欧洲客户", 0},
		{"grants", "dept_name", "外贸一部", 0}, {"users", "dept_name", "外贸一部", 0},
	} {
		p := member
		if tc.kind == "policies" || tc.kind == "audit" || tc.kind == "grants" || tc.kind == "users" {
			p = admin
		}
		rows, _ := listingRows(p, tc.kind, tc.id, 1)
		if len(rows) != 1 || rows[0][tc.key].String() != tc.want {
			t.Fatalf("%s: expected %s, got %+v", tc.kind, tc.want, rows)
		}
	}
	for _, kind := range []string{"tasks", "schedules"} {
		rows, _ := listingRows(member, kind, 0, 1)
		want := "欧洲客户"
		if kind == "schedules" {
			want = "分页之外的分组"
		}
		if rows[0]["group_names"].Strings()[0] != want || rows[0]["template_name"].String() != "中文报价模板" {
			t.Fatal(kind, rows)
		}
	}
	rows, total := listingRows(member, "tags", 0, 1)
	if total != 3 || rows[0]["group_name"].String() != "" || rows[0]["group_id"].Int() != 502 {
		t.Fatal("cross-department reference leaked or ID changed", rows, total)
	}
	rows, total = listingRows(member, "tags", 0, 2)
	if total != 3 || rows[0]["group_name"].String() != "分页之外的分组" || rows[0]["group_id"].Int() != 999 {
		t.Fatal("name outside option first page was not resolved", rows, total)
	}
	_, err = g.DB().Model("bm_contact_groups").Where("id", 999).Data("name", "已修改分组名称").Update()
	if err != nil {
		t.Fatal(err)
	}
	rows, _ = listingRows(member, "tags", 0, 2)
	if rows[0]["group_name"].String() != "已修改分组名称" {
		t.Fatal("stale relation label", rows)
	}
	rows, _ = listingRows(admin, "audit", 0, 1)
	if rows[0]["account_name"].String() != "test_admin" {
		t.Fatal(rows)
	}
	rows, _ = listingRows(admin, "users", 0, 1)
	if rows[0]["role_names"].Strings()[0] != "销售经理" {
		t.Fatal("user role names", rows)
	}
	rows, _ = listingRows(admin, "roles", 0, 1)
	if rows[0]["custom_dept_names"].Strings()[0] != "外贸一部" {
		t.Fatal("custom department names", rows)
	}
	rows, _ = listingRows(admin, "departments", 0, 1)
	if rows[0]["parent_name"].String() == "" {
		t.Fatal("parent department name", rows)
	}
}
