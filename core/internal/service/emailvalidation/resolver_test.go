package emailvalidation

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"golang.org/x/net/dns/dnsmessage"
)

func TestDNSFallbackPreservesValidationRules(t *testing.T) {
	for _, tc := range []struct {
		name         string
		rcode        dnsmessage.RCode
		mx           string
		ip           [4]byte
		status, code string
	}{
		{"recovered MX", dnsmessage.RCodeSuccess, "mx.fixture.test.", [4]byte{8, 8, 8, 8}, Valid, "mx_valid"},
		{"null MX", dnsmessage.RCodeSuccess, ".", [4]byte{8, 8, 8, 8}, Invalid, "null_mx"},
		{"no name", dnsmessage.RCodeNameError, "", [4]byte{}, Invalid, "no_mail_route"},
		{"both unavailable", dnsmessage.RCodeServerFailure, "", [4]byte{}, Unknown, "dns_temporary"},
		{"private MX", dnsmessage.RCodeSuccess, "mx.fixture.test.", [4]byte{10, 0, 0, 1}, Invalid, "no_mail_route"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/dns-message" {
					t.Error("invalid DoH request")
				}
				raw, _ := io.ReadAll(r.Body)
				var request dnsmessage.Message
				if request.Unpack(raw) != nil {
					t.Error("invalid DNS wire packet")
					return
				}
				response := dnsmessage.Message{Header: dnsmessage.Header{Response: true, RCode: tc.rcode}, Questions: request.Questions}
				q := request.Questions[0]
				if tc.rcode == dnsmessage.RCodeSuccess {
					switch q.Type {
					case dnsmessage.TypeMX:
						name, _ := dnsmessage.NewName(tc.mx)
						response.Answers = []dnsmessage.Resource{{Header: dnsmessage.ResourceHeader{Name: q.Name, Class: dnsmessage.ClassINET, TTL: 60}, Body: &dnsmessage.MXResource{MX: name}}}
					case dnsmessage.TypeA:
						response.Answers = []dnsmessage.Resource{{Header: dnsmessage.ResourceHeader{Name: q.Name, Class: dnsmessage.ClassINET, TTL: 60}, Body: &dnsmessage.AResource{A: tc.ip}}}
					}
				}
				body, _ := response.Pack()
				w.Header().Set("Content-Type", "application/dns-message")
				_, _ = w.Write(body)
			}))
			defer server.Close()
			resolver := newFallbackResolver(fakeResolver{mxErr: &net.DNSError{IsTemporary: true}, ipErr: &net.DNSError{IsTemporary: true}})
			resolver.endpoint = server.URL
			got := New(resolver).Domain(context.Background(), "client.test")
			if got.Status != tc.status || got.Code != tc.code {
				t.Fatalf("fallback result: %+v", got)
			}
		})
	}
}

func TestDNSFallbackDoesNotOverrideDefinitiveAnswers(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); http.Error(w, "unexpected fallback", 500) }))
	defer server.Close()
	resolver := newFallbackResolver(fakeResolver{mxErr: &net.DNSError{IsNotFound: true}, ipErr: &net.DNSError{IsNotFound: true}})
	resolver.endpoint = server.URL
	if got := New(resolver).Domain(context.Background(), "absent.test"); got.Status != Invalid || got.Code != "no_mail_route" {
		t.Fatal(got)
	}
	if calls.Load() != 0 {
		t.Fatal("negative system answer triggered fallback")
	}
}
