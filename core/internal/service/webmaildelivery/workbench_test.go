package webmaildelivery

import (
	"billionmail-core/internal/service/governance"
	"context"
	"github.com/gogf/gf/v2/frame/g"
	"strings"
	"testing"
)

func exerciseWorkbench(t *testing.T, ctx context.Context) {
	t.Helper()
	must := func(e error) {
		t.Helper()
		if e != nil {
			t.Fatal(e)
		}
	}
	admin, e := governance.Load(ctx, 1)
	must(e)
	_, e = SaveDraftOptions(ctx, "sender@example.com", g.Map{"draft_key": "c:first", "draft_uid": "42", "options": g.Map{"track_open": true, "warmup": false}})
	must(e)
	_, e = SaveDraftOptions(ctx, "sender@example.com", g.Map{"draft_key": "c:second", "options": g.Map{"track_open": false, "warmup": true}})
	must(e)
	first, e := DraftOptions(ctx, "sender@example.com", "d:42")
	must(e)
	second, e := DraftOptions(ctx, "sender@example.com", "c:second")
	must(e)
	if first["track_open"] != true || second["track_open"] != false || second["warmup"] != true {
		t.Fatal("draft delivery options leaked across messages")
	}
	other, e := DraftOptions(ctx, "other@example.com", "d:42")
	must(e)
	if len(other) != 0 {
		t.Fatal("draft options leaked across mailboxes")
	}
	if _, e = SaveDraftOptions(ctx, "sender@example.com", g.Map{"draft_key": "../../invalid"}); e == nil {
		t.Fatal("invalid draft key accepted")
	}
	unlinked, err := Workbench(ctx, "sender@example.com", Request{Action:"workbench_options"})
 must(err)
 if unlinked["ai"]!=false || unlinked["crm"]!=false {t.Fatal("unlinked mailbox inherited administrative privileges")}
 if _,err=Workbench(ctx,"sender@example.com",Request{Action:"workbench_ai"});err==nil{t.Fatal("unlinked mailbox obtained AI execution")}

	must(SaveBinding(ctx, admin, "sender@example.com", 1))
	must(SaveBinding(ctx, admin, "sender@example.com", 1))
	_, e = g.DB().Model("email_templates").Data(g.Map{"temp_name": "Workbench template", "html_content": "<p>Original <b>format</b></p>", "dept_id": 1, "owner_id": 1}).Insert()
	must(e)
	templates, e := Workbench(ctx, "sender@example.com", Request{Action: "workbench_templates", Data: g.Map{}})
	must(e)
	if !strings.Contains(g.NewVar(templates).String(), "Workbench template") {
		t.Fatal("authorized template list missing")
	}
	_, e = g.DB().Exec(ctx, `INSERT INTO bm_departments(id,name,active) VALUES(88,'Workbench restricted',true);INSERT INTO account(username,password,email,status,dept_id) VALUES('workbench_reader','test','',1,88);INSERT INTO role(role_name,status,data_scope,permissions) VALUES('workbench_reader',1,'department','["crm:list"]');INSERT INTO account_role(account_id,role_id) SELECT a.account_id,r.role_id FROM account a,role r WHERE a.username='workbench_reader' AND r.role_name='workbench_reader';INSERT INTO bm_resource_grants(dept_id,sender,active) VALUES(88,'sender@example.com',true);`)
	must(e)
	id, e := g.DB().Model("account").Where("username", "workbench_reader").Value("account_id")
	must(e)
	reader, e := governance.Load(ctx, id.Int64())
	must(e)
	if SaveBinding(ctx, reader, "sender@example.com", 1) == nil {
		t.Fatal("mailbox owner could grant administrator access")
	}
	must(SaveBinding(ctx, admin, "sender@example.com", id.Int64()))
	if _, e = Workbench(ctx, "sender@example.com", Request{Action: "workbench_ai"}); e == nil {
		t.Fatal("CRM reader could call AI authoring")
	}
	if _, e = Workbench(ctx, "sender@example.com", Request{Action: "workbench_templates"}); e == nil {
		t.Fatal("CRM reader could read campaign templates")
	}
	_, e = g.DB().Model("bm_crm_customers").Data(g.Map{"dept_id": 1, "owner_id": 1, "email": "private@client.test", "name": "Other department"}).Insert()
	must(e)
	customer, e := Workbench(ctx, "sender@example.com", Request{Action: "workbench_customer", Data: g.Map{"email": "private@client.test"}})
	must(e)
	if !g.NewVar(customer["customer"]).IsEmpty() {
		t.Fatal("cross department customer exposed")
	}
	_, e = g.DB().Model("account").Where("account_id", id.Int64()).Data("status", 0).Update()
	must(e)
	if _, e = Workbench(ctx, "sender@example.com", Request{Action: "workbench_customer", Data: g.Map{"email": "private@client.test"}}); e == nil {
		t.Fatal("disabled linked account retained access")
	}
	must(SaveBinding(ctx, admin, "sender@example.com", 0))
	if _, e = mailboxPrincipal(ctx, "sender@example.com"); e == nil {
		t.Fatal("unbound account retained access")
	}
}
