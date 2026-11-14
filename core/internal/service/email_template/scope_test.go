package email_template

import (
	"billionmail-core/internal/service/governance"
	"billionmail-core/internal/testsupport"
	"context"
	"github.com/gogf/gf/v2/frame/g"
	"testing"
)

func TestTemplateCRUDUsesOperationBoundScopes(t *testing.T) {
	testsupport.Database(t)
	ctx := context.Background()
	_, e := g.DB().Exec(ctx, `INSERT INTO bm_departments(id,name) VALUES(20,'Template A'),(21,'Template B');INSERT INTO account(account_id,username,password,email,status,dept_id) VALUES(20,'template_a','test','',1,20),(21,'template_b','test','',1,21)`)
	if e != nil {
		t.Fatal(e)
	}
	principal := func(id, dept int64, codes ...string) *governance.Principal {
		return &governance.Principal{ID: id, DeptID: dept, Roles: []governance.Role{{Scope: "self", Permissions: codes}}}
	}
	full := []string{"template:list", "template:create", "template:update", "template:delete"}
	a := governance.WithPrincipal(ctx, principal(20, 20, full...))
	b := governance.WithPrincipal(ctx, principal(21, 21, full...))
	first, e := CreateTemplate(a, "Shared name", 0, "<p>A</p>", "", "")
	if e != nil {
		t.Fatal(e)
	}
	other, e := CreateTemplate(b, "Shared name", 0, "<p>B</p>", "", "")
	if e != nil {
		t.Fatal("name uniqueness leaked across departments", e)
	}
	total, rows, e := GetTemplatesWithPage(a, 1, 20, "", -1)
	if e != nil || total != 1 || len(rows) != 1 || rows[0].Id != first {
		t.Fatal("list crossed department", total, rows, e)
	}
	if row, e := GetTemplate(a, other); e != nil || row != nil {
		t.Fatal("foreign template readable", row, e)
	}
	if UpdateTemplate(a, other, "forged", "<p>forged</p>", "") == nil {
		t.Fatal("foreign template update accepted")
	}
	if DeleteTemplate(a, other) == nil {
		t.Fatal("foreign template deletion accepted")
	}
	reader := governance.WithPrincipal(ctx, principal(20, 20, "template:list"))
	if _, e = CreateTemplate(reader, "forged", 0, "<p>forged</p>", "", ""); e == nil {
		t.Fatal("reader created a template")
	}
	if UpdateTemplate(reader, first, "forged", "<p>forged</p>", "") == nil || DeleteTemplate(reader, first) == nil {
		t.Fatal("reader mutated a template")
	}
	if e = UpdateTemplate(a, first, "Edited", "<p>new</p>", ""); e != nil {
		t.Fatal(e)
	}
	row, e := GetTemplate(a, first)
	if e != nil || row.Content != "<p>new</p>" {
		t.Fatal("own update failed", row, e)
	}
	if e = DeleteTemplate(a, first); e != nil {
		t.Fatal(e)
	}
}
func TestChatScopeRejectsSharedOrForeignConversations(t *testing.T) {
	testsupport.Database(t)
	ctx := context.Background()
	_, e := g.DB().Exec(ctx, `INSERT INTO bm_departments(id,name) VALUES(30,'Chat A'),(31,'Chat B');INSERT INTO email_templates(temp_name,chat_id,dept_id,owner_id,content) VALUES('Own','shared-chat',30,30,'<p>A</p>'),('Foreign','shared-chat',31,31,'<p>B</p>')`)
	if e != nil {
		t.Fatal(e)
	}
	p := &governance.Principal{ID: 30, DeptID: 30, Roles: []governance.Role{{Scope: "self", Permissions: []string{"template:list", "template:update"}}}}
	request := governance.WithPrincipal(ctx, p)
	if CheckChat(request, "shared-chat", "template:update") == nil {
		t.Fatal("shared cross-department conversation writable")
	}
	if CheckChat(request, "../../private", "template:list") == nil {
		t.Fatal("path traversal accepted")
	}
}
