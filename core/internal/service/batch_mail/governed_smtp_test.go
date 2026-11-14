package batch_mail

import (
	"billionmail-core/internal/service/emailvalidation"
	"billionmail-core/internal/service/governance"
	"billionmail-core/internal/service/mail_service"
	"billionmail-core/internal/service/public"
	"bufio"
	"context"
	"fmt"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"io"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/textproto"
	"strings"
	"testing"
	"time"
)

func exerciseSMTP(t *testing.T, ctx context.Context, p *governance.Principal) {
	t.Helper()
	oldChecks := emailvalidation.Default
	checks := emailvalidation.New(oldChecks.Resolver)
	checks.Probe = func(context.Context, emailvalidation.Result, string, string) emailvalidation.Result {
		t.Error("sending repeated recipient verification")
		return emailvalidation.Result{Status: emailvalidation.Invalid, Code: "recipient_not_found"}
	}
	emailvalidation.Default = checks
	defer func() { emailvalidation.Default = oldChecks }()
	_, _ = g.DB().Exec(ctx, `UPDATE email_tasks SET pause=1`)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	received := make(chan string, 4)
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				defer conn.Close()
				reader := textproto.NewReader(bufio.NewReader(conn))
				_, _ = fmt.Fprint(conn, "220 localhost ESMTP test\r\n")
				for {
					line, err := reader.ReadLine()
					if err != nil {
						return
					}
					words := strings.Fields(line)
					if len(words) == 0 {
						return
					}
					verb := strings.ToUpper(words[0])
					switch verb {
					case "EHLO":
						_, _ = fmt.Fprint(conn, "250-localhost\r\n250 AUTH PLAIN\r\n")
					case "AUTH":
						_, _ = fmt.Fprint(conn, "235 authenticated\r\n")
					case "MAIL", "RCPT", "RSET":
						_, _ = fmt.Fprint(conn, "250 OK\r\n")
					case "DATA":
						_, _ = fmt.Fprint(conn, "354 continue\r\n")
						data, err := reader.ReadDotBytes()
						if err != nil {
							return
						}
						received <- string(data)
						_, _ = fmt.Fprint(conn, "250 queued\r\n")
					case "QUIT":
						_, _ = fmt.Fprint(conn, "221 bye\r\n")
						return
					default:
						_, _ = fmt.Fprint(conn, "250 OK\r\n")
					}
				}
			}(conn)
		}
	}()
	old := newGovernedSender
	defer func() { newGovernedSender = old }()
	_, port, _ := net.SplitHostPort(listener.Addr().String())
	newGovernedSender = func(email string) (*mail_service.EmailSender, error) {
		return &mail_service.EmailSender{Email: email, UserName: email, Password: "test", Host: "127.0.0.1", Port: port}, nil
	}
	_ = public.Cache.Set(ctx, "ServerIP", "127.0.0.1", time.Hour)
	_ = public.OptionsMgrInstance.SetOption(ctx, "reverse_proxy_domain", "http://localhost")
	config := CampaignConfig{CCEmail: "copy@example.com", TrackOpen: true, GroupID: 1, TemplateID: 1, Sender: "sender@example.com", Subject: "SMTP test", Policy: governance.Policy{PerMinute: 10}}
	oldContent, _ := g.DB().Model("email_templates").Where("id", 1).Value("content")
	_, err = g.DB().Model("email_templates").Where("id", 1).Data("content", "Dear Purchasing Manager,\nGreetings from BESTMAPLE!\n\nProducts:\n✓ Compressors\n✓ Condensers").Update()
	if err != nil {
		t.Fatal(err)
	}
	defer g.DB().Model("email_templates").Where("id", 1).Data("content", oldContent.String()).Update()
	var task int64
	err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		var e error
		task, e = CreateGovernedTask(ctx, tx, p, config, true, 1)
		return e
	})
	if err != nil {
		t.Fatal(err)
	}
	job, err := claimGoverned(ctx)
	if err != nil || job == nil {
		t.Fatalf("claim smtp: %v", err)
	}
	processGoverned(ctx, job)
	row, err := g.DB().Model("recipient_info").Where("task_id", task).One()
	if err != nil || row["delivery_state"].String() != "accepted" {
		t.Fatalf("SMTP not accepted: %v %v", row, err)
	}
	select {
	case payload := <-received:
		parsed, e := mail.ReadMessage(strings.NewReader(payload))
		if e != nil {
			t.Fatal(e)
		}
		decoded, e := io.ReadAll(quotedprintable.NewReader(parsed.Body))
		if e != nil {
			t.Fatal(e)
		}
		if !strings.Contains(string(decoded), "Dear Purchasing Manager,<br>Greetings") || !strings.Contains(string(decoded), "✓ Compressors<br>✓ Condensers") || strings.Contains(parsed.Header.Get("Subject"), "<p") {
			t.Fatal("SMTP lost rich-text layout or altered the subject")
		}
		if !strings.Contains(payload, "Cc: copy@example.com") || strings.Contains(payload, "To: a@example.com,copy@example.com") || strings.Contains(payload, "/pmta/") {
			t.Fatal("CC headers or attribution are wrong")
		}
		if !strings.Contains(payload, "Unsubscribe") || !strings.Contains(strings.ToLower(payload), "message-id:") {
			t.Fatalf("missing unsubscribe/message identifier: %s", payload)
		}
	case <-time.After(time.Second):
		t.Fatal("SMTP server received no message")
	}
	DispatchGoverned(ctx)
	value, err := g.DB().Model("email_tasks").Where("id", task).Value("task_process")
	if err != nil || value.Int() != 2 {
		t.Fatalf("completed SMTP task not finalized: %v %v", value, err)
	}
	inline := `<p style="margin:0 0 16px">AI standalone body.</p><p>Second <strong>formatted</strong> paragraph.</p>`
	err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		_, e := CreateGovernedTask(ctx, tx, p, CampaignConfig{GroupID: 1, Sender: "sender@example.com", FullName: "BESTMAPLE AI", Subject: "Standalone AI draft", HTMLContent: &inline}, true, 1)
		return e
	})
	if err != nil {
		t.Fatal(err)
	}
	// Allow the real default global interval between both SMTP deliveries.
	time.Sleep(1100 * time.Millisecond)
	inlineJob, e := claimGoverned(ctx)
	if e != nil || inlineJob == nil {
		t.Fatal("inline task claim", e)
	}
	processGoverned(ctx, inlineJob)
	select {
	case payload := <-received:
		parsed, e := mail.ReadMessage(strings.NewReader(payload))
		if e != nil {
			t.Fatal(e)
		}
		body, e := io.ReadAll(quotedprintable.NewReader(parsed.Body))
		if e != nil || !strings.Contains(string(body), inline) || !strings.Contains(parsed.Header.Get("From"), "BESTMAPLE AI") {
			t.Fatal("standalone AI body or name lost in delivery", e, payload)
		}
	case <-time.After(2 * time.Second):
		state, _ := g.DB().Model("recipient_info").Where("id", inlineJob.Record["delivery_id"]).Fields("task_id,delivery_state,last_error").One()
		t.Fatal("inline campaign SMTP missing", state)
	}
	exercisePersonalizedOutreach(t, ctx, p, received)
	exercisePreviewDeliveries(t, ctx, p, received)
}
