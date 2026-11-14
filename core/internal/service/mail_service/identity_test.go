package mail_service

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/mail"
	"net/textproto"
	"strings"
	"testing"
	"time"
)

func TestOutboundIdentityHeadersAndEnvelope(t *testing.T) {
	previous := SuppressionCheck
	SuppressionCheck = func(context.Context, string, string) (bool, error) { return false, nil }
	defer func() { SuppressionCheck = previous }()
	t.Setenv("MAPLEMAIL_FROM_ADDRESS", "sales@best-maple.com")
	t.Setenv("MAPLEMAIL_REPLY_TO_ADDRESS", "sales@best-maple.com")
	listener, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	defer listener.Close()
	commands := make(chan string, 30)
	messages := make(chan string, 2)
	go func() {
		conn, e := listener.Accept()
		if e != nil {
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
		reader := textproto.NewReader(bufio.NewReader(conn))
		fmt.Fprint(conn, "220 localhost ESMTP test\r\n")
		for {
			line, e := reader.ReadLine()
			if e != nil {
				return
			}
			commands <- line
			switch strings.ToUpper(strings.Fields(line)[0]) {
			case "EHLO":
				fmt.Fprint(conn, "250-localhost\r\n250 AUTH PLAIN\r\n")
			case "AUTH":
				fmt.Fprint(conn, "235 authenticated\r\n")
			case "DATA":
				fmt.Fprint(conn, "354 continue\r\n")
				data, e := reader.ReadDotBytes()
				if e != nil {
					return
				}
				messages <- string(data)
				fmt.Fprint(conn, "250 accepted\r\n")
			case "QUIT":
				fmt.Fprint(conn, "221 bye\r\n")
				return
			default:
				fmt.Fprint(conn, "250 OK\r\n")
			}
		}
	}()
	_, port, _ := net.SplitHostPort(listener.Addr().String())
	sender := &EmailSender{Email: "support@best-maple.com", UserName: "support@best-maple.com", Password: "fake-only", Host: "127.0.0.1", Port: port}
	defer sender.Close()
	message := NewMessage("Offer", "<p>Offer</p>")
	message.SetHeader("from", "original@example.test")
	message.SetHeader("reply-to", "other@example.test")
	message.SetRealName("BESTMAPLE Sales")
	for i := 0; i < 2; i++ {
		if e = sender.SendOnce(message, []string{"buyer@example.test"}); e != nil {
			t.Fatal(e)
		}
		select {
		case raw := <-messages:
			parsed, e := mail.ReadMessage(strings.NewReader(raw))
			if e != nil {
				t.Fatal(e)
			}
			from, e := mail.ParseAddress(parsed.Header.Get("From"))
			if e != nil || from.Address != "sales@best-maple.com" || parsed.Header.Get("Reply-To") != "sales@best-maple.com" || len(parsed.Header["From"]) != 1 || len(parsed.Header["Reply-To"]) != 1 {
				t.Fatal("identity headers incorrect", parsed.Header, e)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("SMTP payload missing")
		}
	}
	if message.Headers["from"] != "original@example.test" || message.Headers["RealName"] == "" {
		t.Fatal("caller headers mutated")
	}
	mailCommands := 0
	for len(commands) > 0 {
		line := <-commands
		if strings.HasPrefix(line, "MAIL FROM:") {
			mailCommands++
			if !strings.HasPrefix(line, "MAIL FROM:<support@best-maple.com>") {
				t.Fatal("SMTP envelope rotation changed", line)
			}
		}
	}
	if mailCommands != 2 {
		t.Fatal("SMTP envelope commands missing", mailCommands)
	}
}

func TestIdentityRejectsInjectionAndPreservesUnconfiguredHeaders(t *testing.T) {
	t.Setenv("MAPLEMAIL_FROM_ADDRESS", "")
	t.Setenv("MAPLEMAIL_REPLY_TO_ADDRESS", "")
	message := NewMessage("Offer", "<p>Test</p>")
	message.SetHeader("From", "Original <original@example.test>")
	message.SetHeader("Reply-To", "reply@example.test")
	prepared, from, e := prepareDeliveryIdentity(message, "sender@example.test")
	if e != nil || from != "Original <original@example.test>" || prepared.Headers["Reply-To"] != "reply@example.test" {
		t.Fatal("unconfigured behavior lost", e)
	}
	message.SetHeader("reply-to", "duplicate@example.test")
	if _, _, e = prepareDeliveryIdentity(message, "sender@example.test"); e == nil {
		t.Fatal("duplicate header accepted")
	}
	delete(message.Headers, "reply-to")
	message.SetHeader("Reply-To", "reply@example.test\r\nBcc: hidden@example.test")
	if _, _, e = prepareDeliveryIdentity(message, "sender@example.test"); e == nil {
		t.Fatal("header injection accepted")
	}
	t.Setenv("MAPLEMAIL_FROM_ADDRESS", "sales@best-maple.com\r\nBcc: hidden@example.test")
	if _, e = OutboundIdentity("sender@example.test"); e == nil {
		t.Fatal("configured injection accepted")
	}
}
