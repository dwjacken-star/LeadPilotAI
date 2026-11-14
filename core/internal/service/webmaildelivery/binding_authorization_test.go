package webmaildelivery

import (
	"context"
	"testing"

	"billionmail-core/internal/service/governance"
	"billionmail-core/internal/testsupport"
	"github.com/gogf/gf/v2/frame/g"
)

func TestBindingAuthorizationIsExplicitAtomicAndScoped(t *testing.T) {
	testsupport.Database(t)
	ctx := context.Background()
	exec := func(q string, args ...interface{}) {
		t.Helper()
		if _, e := g.DB().Exec(ctx, q, args...); e != nil {
			t.Fatal(e)
		}
	}
	exec(`INSERT INTO bm_departments(id,name,active) VALUES(81,'Binding department',true),(82,'Disabled binding department',false);
 INSERT INTO account(username,password,email,status,dept_id) VALUES('binding_operator','test','',1,81),('disabled_binding_operator','test','',1,82);
 INSERT INTO role(role_name,status,data_scope,permissions) VALUES('binding_operator',1,'department','["crm:list"]');
 INSERT INTO account_role(account_id,role_id) SELECT a.account_id,r.role_id FROM account a,role r WHERE a.username='binding_operator' AND r.role_name='binding_operator';
 INSERT INTO mailbox(username,password,password_encode,full_name,maildir,local_part,domain,active) VALUES('binding@example.com','test','','Binding','test/','binding','example.com',1)`)
	admin, e := governance.Load(ctx, 1)
	if e != nil {
		t.Fatal(e)
	}
	id, e := g.DB().Model("account").Where("username", "binding_operator").Value("account_id")
	if e != nil {
		t.Fatal(e)
	}
	member, e := governance.Load(ctx, id.Int64())
	if e != nil {
		t.Fatal(e)
	}
	assertAbsent := func() {
		t.Helper()
		for _, table := range []string{"bm_webmail_bindings", "bm_resource_grants"} {
			field := "mailbox"
			if table == "bm_resource_grants" {
				field = "sender"
			}
			n, e := g.DB().Model(table).Where(field, "binding@example.com").Count()
			if e != nil || n != 0 {
				t.Fatalf("partial authorization persisted in %s: %d %v", table, n, e)
			}
		}
	}
	if SaveBinding(ctx, admin, "binding@example.com", member.ID) == nil {
		t.Fatal("missing department grant was silently bypassed")
	}
	if SaveBindingWithGrant(ctx, member, "binding@example.com", member.ID, true, 81) == nil {
		t.Fatal("member could create a department grant")
	}
	if SaveBindingWithGrant(ctx, admin, "binding@example.com", member.ID, true, 999) == nil {
		t.Fatal("stale department consent was accepted")
	}
	assertAbsent()
	metadata, e := Bindings(ctx, admin, "binding@example.com", "binding_operator", 1, member.ID)
	if e != nil {
		t.Fatal(e)
	}
	selected := g.NewVar(metadata["selected_account"]).Map()
	if g.NewVar(selected["eligible"]).Bool() != true || g.NewVar(selected["has_sender_grant"]).Bool() {
		t.Fatal("incorrect binding eligibility metadata", selected)
	}
	// Force an audit failure in this disposable database to prove the grant and
	// binding cannot survive a failed transaction.
	exec(`CREATE FUNCTION test_binding_audit_failure() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='webmail_binding' THEN RAISE EXCEPTION 'test audit failure'; END IF; RETURN NEW; END $$;
 CREATE TRIGGER test_binding_audit_failure BEFORE INSERT ON bm_security_audit FOR EACH ROW EXECUTE FUNCTION test_binding_audit_failure()`)
	if SaveBindingWithGrant(ctx, admin, "binding@example.com", member.ID, true, 81) == nil {
		t.Fatal("forced audit failure did not propagate")
	}
	assertAbsent()
	exec(`DROP TRIGGER test_binding_audit_failure ON bm_security_audit; DROP FUNCTION test_binding_audit_failure()`)
	if e = SaveBindingWithGrant(ctx, admin, " BINDING@example.com ", member.ID, true, 81); e != nil {
		t.Fatal(e)
	}
	if e = SaveBinding(ctx, admin, "binding@example.com", member.ID); e != nil {
		t.Fatal(e)
	}
	if e = governance.SenderAllowed(ctx, member, "binding@example.com"); e != nil {
		t.Fatal(e)
	}
	count, e := g.DB().Model("bm_resource_grants").Where("dept_id", 81).Where("sender", "binding@example.com").Count()
	if e != nil || count != 1 {
		t.Fatal("grant creation was not idempotent", count, e)
	}
	exec(`UPDATE bm_resource_grants SET active=false WHERE dept_id=81 AND sender='binding@example.com'`)
	if _, e = mailboxPrincipal(ctx, "binding@example.com"); e == nil {
		t.Fatal("revoked grant still allowed mailbox business access")
	}
	if e = SaveBindingWithGrant(ctx, admin, "binding@example.com", member.ID, true, 81); e != nil {
		t.Fatal(e)
	}
	exec(`UPDATE mailbox SET active=0 WHERE username='binding@example.com'`)
	if e = SaveBinding(ctx, admin, "binding@example.com", 0); e != nil {
		t.Fatal("stale inactive binding could not be revoked", e)
	}
	count, e = g.DB().Model("bm_resource_grants").Where("dept_id", 81).Where("sender", "binding@example.com").Count()
	if e != nil || count != 1 {
		t.Fatal("unlink unexpectedly deleted department authorization")
	}
}
