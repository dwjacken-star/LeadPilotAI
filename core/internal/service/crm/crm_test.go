package crm

import (
	"billionmail-core/internal/service/governance"
	"billionmail-core/internal/testsupport"
	"context"
	"github.com/gogf/gf/v2/frame/g"
	"net/mail"
	"strings"
	"testing"
)

func TestCustomerScopeAndMIME(t *testing.T) {
	testsupport.Database(t)
	ctx := context.Background()
	admin, err := governance.Load(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	id, err := Save(ctx, admin, 0, map[string]interface{}{"email": "buyer@client.test", "name": "Alex", "stage": "inquiry"})
	if err != nil {
		t.Fatal(err)
	}
	limited := &governance.Principal{ID: 2, DeptID: 2, Roles: []governance.Role{{Scope: "department", Permissions: []string{"crm:list", "crm:manage", "crm:reply"}}}}
	if _, err = Customer(ctx, limited, id, "crm:list"); err == nil {
		t.Fatal("cross-department customer visible")
	}
	if _, err = Customer(ctx, limited, id, "crm:reply"); err == nil {
		t.Fatal("cross-department reply accepted")
	}
	if result, e := List(ctx, limited, 1, "buyer", ""); e != nil || result["total"].(int) != 0 {
		t.Fatal("CRM keyword filter bypassed scope", e)
	}
	if err = Followup(ctx, admin, id, "Asked about delivery date"); err != nil {
		t.Fatal(err)
	}
	_, err = g.DB().Model("bm_crm_messages").Data(g.Map{"customer_id": id, "dept_id": 1, "owner_id": 1, "mailbox": "sender@example.com", "direction": "incoming", "subject": "Inquiry", "message_id": "<inquiry@client.test>", "from_address": "buyer@client.test", "body_html": "<p>Hello</p>", "body_loaded": true}).Insert()
	if err != nil {
		t.Fatal(err)
	}
	detail, err := Detail(ctx, admin, id)
	if err != nil || detail["customer"] == nil {
		t.Fatal(err)
	}
	mime := "Content-Type: multipart/alternative; boundary=part\r\n\r\n--part\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\nHello\r\n--part\r\nContent-Type: text/html; charset=UTF-8\r\nContent-Transfer-Encoding: quoted-printable\r\n\r\n<p style=3D\"color:green\">Hello</p>\r\n--part--\r\n"
	parsed, err := mail.ReadMessage(strings.NewReader(mime))
	if err != nil {
		t.Fatal(err)
	}
	html, plain := parseBody(parsed.Header, parsed.Body, 0)
	if !strings.Contains(html, `style="color:green"`) || !strings.Contains(plain, "Hello") {
		t.Fatalf("MIME layout lost: %s %s", html, plain)
	}
	exerciseReply(t, ctx, admin, id)
	exerciseCustomerDeletion(t, ctx, admin, id)
}

func exerciseCustomerDeletion(t *testing.T, ctx context.Context, admin *governance.Principal, id int64) {
	t.Helper()
	limited := &governance.Principal{ID: 2, DeptID: 2, Roles: []governance.Role{{Scope: "department", Permissions: []string{"crm:list", "crm:delete"}}}}
	if err := Delete(ctx, limited, id); err == nil {
		t.Fatal("cross-department customer deletion allowed")
	}
	withoutDelete := &governance.Principal{ID: 1, DeptID: 1, Roles: []governance.Role{{Scope: "all", Permissions: []string{"crm:list", "crm:manage"}}}}
	if err := Delete(ctx, withoutDelete, id); err == nil {
		t.Fatal("delete permission was not enforced")
	}
	if err := Delete(ctx, admin, id); err != nil {
		t.Fatal(err)
	}
	if _, err := Detail(ctx, admin, id); err == nil {
		t.Fatal("deleted customer still visible")
	}
	result, err := List(ctx, admin, 1, "buyer@client.test", "")
	if err != nil || result["total"].(int) != 0 {
		t.Fatal("deleted customer still listed", result, err)
	}
	messages, err := g.DB().Model("bm_crm_messages").Where("customer_id", id).Count()
	if err != nil || messages == 0 {
		t.Fatal("deletion destroyed correspondence")
	}
	messageID, _ := g.DB().Model("bm_crm_messages").Where("customer_id", id).Value("id")
	if _, err := Message(ctx, admin, messageID.Int64()); err == nil {
		t.Fatal("deleted customer's messages accessible")
	}
	restored, err := Save(ctx, admin, 0, map[string]interface{}{"email": "buyer@client.test", "stage": "inquiry"})
	if err != nil || restored != id {
		t.Fatal("same-email restore failed", restored, err)
	}
	audit, _ := g.DB().Model("bm_security_audit").Where("action", "crm:delete").Where("resource_id", id).Count()
	if audit != 1 {
		t.Fatal("deletion not audited")
	}
}
