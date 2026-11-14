package emailvalidation

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeResolver struct {
	mx    []*net.MX
	mxErr error
	ips   []net.IPAddr
	ipErr error
}

func (f fakeResolver) LookupMX(context.Context, string) ([]*net.MX, error) { return f.mx, f.mxErr }
func (f fakeResolver) LookupIPAddr(context.Context, string) ([]net.IPAddr, error) {
	return f.ips, f.ipErr
}
func TestDNSRoutes(t *testing.T) {
	public := []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}
	notFound := &net.DNSError{IsNotFound: true, Err: "not found"}
	temporary := &net.DNSError{IsTemporary: true, Err: "SERVFAIL"}
	for _, tc := range []struct {
		name         string
		resolver     fakeResolver
		status, code string
	}{
		{"MX", fakeResolver{mx: []*net.MX{{Host: "mx.example.net."}}, ips: public}, Valid, "mx_valid"},
		{"implicit MX", fakeResolver{mxErr: notFound, ips: public}, Valid, "implicit_mx"},
		{"null MX", fakeResolver{mx: []*net.MX{{Host: "."}}, ips: public}, Invalid, "null_mx"},
		{"NXDOMAIN", fakeResolver{mxErr: notFound, ipErr: notFound}, Invalid, "no_mail_route"},
		{"temporary DNS", fakeResolver{mxErr: temporary, ips: public}, Unknown, "dns_temporary"},
		{"private MX", fakeResolver{mx: []*net.MX{{Host: "internal.net."}}, ips: []net.IPAddr{{IP: net.ParseIP("10.0.0.1")}}}, Invalid, "no_mail_route"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := New(tc.resolver).Domain(context.Background(), "example.net")
			if r.Status != tc.status || r.Code != tc.code {
				t.Fatal(r)
			}
		})
	}
	for _, value := range []string{"127.0.0.1", "169.254.169.254", "100.64.0.1", "::1", "fc00::1"} {
		if publicIP(net.ParseIP(value)) {
			t.Fatal("private/metadata route accepted", value)
		}
	}
}
func TestSMTPAndCatchAllWithoutDATA(t *testing.T) {
	old := dialSMTP
	defer func() { dialSMTP = old }()
	for _, tc := range []struct{ name, target, random, status, code string }{
		{"specific recipient", "250 2.1.5 Recipient OK", "550 5.1.1 No such user", Valid, "smtp_accepted"},
		{"missing mailbox", "550 5.1.1 No such user", "", Invalid, "recipient_not_found"},
		{"policy rejection", "550 5.7.1 Access denied", "", Unknown, "smtp_unconfirmed"},
		{"greylisting", "451 4.7.1 Try later", "", Unknown, "smtp_unconfirmed"},
		{"cannot verify", "252 Cannot verify", "", Unknown, "smtp_unconfirmed"},
		{"full mailbox", "552 5.2.2 Full mailbox", "", Unknown, "smtp_unconfirmed"},
		{"catch all", "250 2.1.5 OK", "250 2.1.5 OK", Unknown, "catch_all"},
		{"random probe inconclusive", "250 2.1.5 OK", "450 4.7.1 Try later", Unknown, "smtp_unconfirmed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var mu sync.Mutex
			commands := []string{}
			done := make(chan struct{})
			dialSMTP = func(context.Context, string) (net.Conn, error) {
				client, server := net.Pipe()
				go func() {
					defer close(done)
					defer server.Close()
					reader := bufio.NewReader(server)
					fmt.Fprint(server, "220 fixture ESMTP\r\n")
					for {
						line, err := reader.ReadString('\n')
						if err != nil {
							return
						}
						line = strings.TrimSpace(line)
						mu.Lock()
						commands = append(commands, line)
						mu.Unlock()
						reply := "250 OK"
						switch {
						case strings.HasPrefix(line, "RCPT TO:<maplemail-probe-"):
							reply = tc.random
						case strings.HasPrefix(line, "RCPT TO:"):
							reply = tc.target
						case line == "QUIT":
							fmt.Fprint(server, "221 Bye\r\n")
							return
						case line == "DATA":
							reply = "554 Unexpected DATA"
						}
						fmt.Fprint(server, reply+"\r\n")
					}
				}()
				return client, nil
			}
			probeMu.Lock()
			nextProbe = map[string]time.Time{}
			probeMu.Unlock()
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			r := probeSMTP(ctx, Result{Routes: []Route{{Host: "mx.fixture.net", IPs: []string{"8.8.8.8"}}}}, "buyer@example.net", "sender@example.org")
			if r.Status != tc.status || r.Code != tc.code {
				t.Fatal(r)
			}
			<-done
			mu.Lock()
			defer mu.Unlock()
			for _, command := range commands {
				if command == "DATA" {
					t.Fatal("verification sent DATA")
				}
			}
		})
	}
}
func TestCatchAllCacheIsNotPositiveValidation(t *testing.T) {
	engine := New(fakeResolver{mx: []*net.MX{{Host: "mx.example.net."}}, ips: []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}})
	calls := 0
	engine.Probe = func(context.Context, Result, string, string) Result {
		calls++
		return result(Unknown, "catch_all", "smtp")
	}
	a := engine.Check(context.Background(), "one@example.net", "sender@example.org", true)
	b := engine.Check(context.Background(), "two@example.net", "sender@example.org", true)
	if a.Status != Unknown || b.Status != Unknown || b.Code != "catch_all" || calls != 1 {
		t.Fatal(a, b, calls)
	}
}
func TestQueueWaitDoesNotConsumeSMTPProbeTimeout(t *testing.T) {
	engine := New(fakeResolver{mx: []*net.MX{{Host: "mx.example.net."}}, ips: []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}})
	engine.Timeout = 10 * time.Millisecond
	engine.slots = make(chan struct{}, 1)
	engine.slots <- struct{}{}
	engine.Probe = func(context.Context, Result, string, string) Result { return result(Valid, "smtp_accepted", "smtp") }
	go func() { time.Sleep(30 * time.Millisecond); <-engine.slots }()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if r := engine.Check(ctx, "buyer@example.net", "sender@example.org", true); r.Status != Valid {
		t.Fatal("local queue delay was misclassified as a recipient failure", r)
	}
}
