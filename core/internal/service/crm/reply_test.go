package crm

import (
	"billionmail-core/internal/service/governance"
	"billionmail-core/internal/service/mail_service"
	"billionmail-core/internal/service/public"
	"bufio"
	"context"
	"fmt"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/google/uuid"
	"net"
	"net/textproto"
	"strings"
	"testing"
	"time"
)

func exerciseReply(t *testing.T, ctx context.Context, p *governance.Principal, customer int64) {
	t.Helper()
	t.Setenv("MAPLEMAIL_FROM_ADDRESS", "sales@best-maple.com")
	t.Setenv("MAPLEMAIL_REPLY_TO_ADDRESS", "sales@best-maple.com")
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	received := make(chan string, 2)
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				defer conn.Close()
				reader := textproto.NewReader(bufio.NewReader(conn))
				fmt.Fprint(conn, "220 localhost ESMTP test\r\n")
				for {
					line, err := reader.ReadLine()
					if err != nil {
						return
					}
					parts := strings.Fields(line)
					if len(parts) == 0 {
						return
					}
					switch strings.ToUpper(parts[0]) {
					case "EHLO":
						fmt.Fprint(conn, "250-localhost\r\n250 AUTH PLAIN\r\n")
					case "AUTH":
						fmt.Fprint(conn, "235 authenticated\r\n")
					case "DATA":
						fmt.Fprint(conn, "354 continue\r\n")
						body, e := reader.ReadDotBytes()
						if e != nil {
							return
						}
						received <- string(body)
						fmt.Fprint(conn, "250 accepted\r\n")
					case "QUIT":
						fmt.Fprint(conn, "221 bye\r\n")
						return
					default:
						fmt.Fprint(conn, "250 OK\r\n")
					}
				}
			}(conn)
		}
	}()
	_, port, _ := net.SplitHostPort(listener.Addr().String())
	previous := newSender
	defer func() { newSender = previous }()
	newSender = func(email string) (*mail_service.EmailSender, error) {
		return &mail_service.EmailSender{Email: email, UserName: email, Password: "test", Host: "127.0.0.1", Port: port}, nil
	}
	_ = public.Cache.Set(ctx, "ServerIP", "127.0.0.1", time.Hour)
	value, err := g.DB().Model("bm_crm_messages").Where("customer_id", customer).Where("direction", "incoming").Value("id")
	if err != nil {
		t.Fatal(err)
	}
	key := uuid.NewString()
	id, err := Reply(ctx, p, value.Int64(), "<p>Quotation attached in follow-up.</p>", key)
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := Reply(ctx, p, value.Int64(), "<p>Duplicate click</p>", key)
	if err != nil || duplicate != id {
		t.Fatal("idempotent reply failed", err)
	}
	select {
	case payload := <-received:
		if !strings.Contains(payload, "From: sales@best-maple.com") || !strings.Contains(payload, "Reply-To: sales@best-maple.com") {
			t.Fatal("CRM business identity missing")
		}
		if !strings.Contains(payload, "In-Reply-To: <inquiry@client.test>") || !strings.Contains(payload, "Subject: Re: Inquiry") {
			t.Fatalf("thread headers missing: %s", payload)
		}
	case <-time.After(time.Second):
		t.Fatal("no SMTP reply received")
	}
	select {
	case <-received:
		t.Fatal("duplicate request sent twice")
	default:
	}
	row, _ := g.DB().Model("bm_crm_messages").Where("id", id).One()
	if row["status"].String() != "sent" || row["from_address"].String() != "sales@best-maple.com" || row["mailbox"].String() != "sender@example.com" {
		t.Fatal("reply status not persisted")
	}
}
