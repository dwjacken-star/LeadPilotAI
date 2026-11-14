package batch_mail

import (
	"context"
	"io"
	"mime/quotedprintable"
	"net/mail"
	"strings"
	"testing"
	"time"

	v1 "billionmail-core/api/batch_mail/v1"
	"billionmail-core/internal/service/deliveryprefs"
	"billionmail-core/internal/service/governance"
	"billionmail-core/internal/service/mail_service"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

func exercisePreviewDeliveries(t *testing.T, ctx context.Context, p *governance.Principal, received <-chan string) {
	t.Helper()
	previous := previewSender
	previewSender = newGovernedSender
	defer func() { previewSender = previous }()
	old, e := g.DB().Model("email_templates").Where("id", 1).Value("content")
	if e != nil {
		t.Fatal(e)
	}
	defer g.DB().Model("email_templates").Where("id", 1).Data("content", old.String()).Update()
	_, e = g.DB().Model("email_templates").Where("id", 1).Data("content", `<p>Hello {{ .Subscriber.Email }}</p><a href="https://example.org/catalog">Catalog</a>`).Update()
	if e != nil {
		t.Fatal(e)
	}
	for _, enabled := range []int{1, 0} {
		request := &v1.SendTestEmailReq{Addresser: "sender@example.com", Subject: "Test {{ .Subscriber.Email }}", Recipient: "not-in-contacts@client.test", TemplateId: 1, GroupIDs: []int{1}, Unsubscribe: enabled, TrackOpen: enabled, TrackClick: enabled}
		if enabled == 0 {
			html := `<p style="margin:0 0 16px">Inline {{ .Subscriber.Email }}</p><p><strong>Second paragraph</strong></p>`
			request.HTMLContent = &html
			request.TemplateId = 0
			request.FullName = "BESTMAPLE Preview"
		}
		if e = SendPreview(ctx, p, request); e != nil {
			t.Fatal(e)
		}
		select {
		case raw := <-received:
			parsed, e := mail.ReadMessage(strings.NewReader(raw))
			if e != nil {
				t.Fatal(e)
			}
			body, e := io.ReadAll(quotedprintable.NewReader(parsed.Body))
			if e != nil {
				t.Fatal(e)
			}
			if !strings.Contains(string(body), "not-in-contacts@client.test") || strings.Contains(string(body), "{{") || strings.Contains(parsed.Header.Get("Subject"), "{{") {
				t.Fatal("unknown test recipient was not rendered", raw)
			}
			if enabled == 1 {
				if parsed.Header.Get("List-Unsubscribe") == "" || !strings.Contains(string(body), "unsubscribe_new.html?jwt=") || !strings.Contains(string(body), "subscribe_form.html?token=") || !strings.Contains(string(body), "/pmta/w/") {
					t.Fatal("test email did not honor enabled delivery controls", raw)
				}
			} else if parsed.Header.Get("List-Unsubscribe") != "" || strings.Contains(string(body), "/pmta/") || strings.Contains(string(body), "unsubscribe_new.html") {
				t.Fatal("disabled test email was instrumented", raw)
			}
			if enabled == 0 && (!strings.Contains(string(body), "<strong>Second paragraph</strong>") || !strings.Contains(parsed.Header.Get("From"), "BESTMAPLE Preview")) {
				t.Fatal("inline preview formatting or sender name lost", raw)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("preview SMTP message missing")
		}
	}
	// A new campaign and re-imported active contact cannot bypass durable opt-outs.
	_, e = g.DB().Model("email_tasks").Where("task_process", 0).Data("pause", 1).Update()
	if e != nil {
		t.Fatal(e)
	}
	contact, e := g.DB().Model("bm_contacts").Data(g.Map{"group_id": 1, "email": "opted-out@client.test", "active": 1, "status": 1, "dept_id": 1, "owner_id": 1}).InsertAndGetId()
	if e != nil {
		t.Fatal(e)
	}
	identity, e := mail_service.OutboundIdentity("sender@example.com")
	if e != nil {
		t.Fatal(e)
	}
	if e = deliveryprefs.Record(ctx, "opted-out@client.test", deliveryprefs.Domain(identity.From), 1, "integration fixture"); e != nil {
		t.Fatal(e)
	}
	var task int64
	e = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		var err error
		task, err = CreateGovernedTask(ctx, tx, p, CampaignConfig{GroupID: 1, TemplateID: 1, Sender: "sender@example.com", Subject: "Must skip opted-out recipient"}, true, contact)
		return err
	})
	if e != nil {
		t.Fatal(e)
	}
	job, e := claimGoverned(ctx)
	if e != nil || job == nil {
		t.Fatal("suppressed task not claimable", e)
	}
	processGoverned(ctx, job)
	row, e := g.DB().Model("recipient_info").Where("task_id", task).One()
	if e != nil || row["delivery_state"].String() != "skipped" {
		t.Fatal("opted-out address was not skipped", row, e)
	}
	select {
	case <-received:
		t.Fatal("opted-out address reached SMTP DATA")
	default:
	}
}
