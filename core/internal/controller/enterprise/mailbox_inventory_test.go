package enterprise

import (
	"context"
	"testing"

	"billionmail-core/internal/service/governance"
	"billionmail-core/internal/testsupport"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

func TestGrantEditorUsesNewestActiveInventoryWithoutWideningMemberAccess(t *testing.T) {
	testsupport.Database(t)
	ctx := context.Background()
	_, e := g.DB().Exec(ctx, `INSERT INTO mailbox(username,password,password_encode,full_name,maildir,local_part,domain,active,create_time) VALUES
 ('old-inventory@example.com','test','','Old','test/','old-inventory','example.com',1,100),
 ('new-inventory@example.com','test','','New','test/','new-inventory','example.com',1,200),
 ('inactive-inventory@example.com','test','','Inactive','test/','inactive-inventory','example.com',0,300)`)
	if e != nil {
		t.Fatal(e)
	}
	admin, e := governance.Load(ctx, 1)
	if e != nil {
		t.Fatal(e)
	}
	result, e := list(ctx, admin, &Request{Type: "senders", Page: 1, PageSize: 1, Data: g.Map{"purpose": "grant_editor"}})
	if e != nil {
		t.Fatal(e)
	}
	rows := result.(g.Map)["list"].(gdb.Result)
	if len(rows) != 1 || rows[0]["username"].String() != "new-inventory@example.com" {
		t.Fatal("newest active mailbox was not first", rows)
	}
	member := &governance.Principal{ID: 2, DeptID: 99}
	if _, e = list(ctx, member, &Request{Type: "senders", Data: g.Map{"purpose": "grant_editor"}}); e == nil {
		t.Fatal("ordinary member obtained global mailbox inventory")
	}
	scoped, e := list(ctx, member, &Request{Type: "senders", Data: g.Map{}})
	if e != nil || g.NewVar(scoped.(g.Map)["total"]).Int() != 0 {
		t.Fatal("regular sender list bypassed department grants", scoped, e)
	}
	if _, e = save(ctx, admin, &Request{Type: "grants", Data: g.Map{"dept_id": 99999, "sender": "new-inventory@example.com", "active": true}}); e == nil {
		t.Fatal("invalid department grant accepted")
	}
	if _, e = save(ctx, admin, &Request{Type: "grants", Data: g.Map{"dept_id": 1, "sender": "inactive-inventory@example.com", "active": true}}); e == nil {
		t.Fatal("inactive mailbox grant accepted")
	}
}
