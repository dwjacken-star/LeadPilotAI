package emailvalidation

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"golang.org/x/net/dns/dnsmessage"
)

type fallbackResolver struct {
	primary        Resolver
	client         *http.Client
	endpoint       string
	primaryTimeout time.Duration
}

func newFallbackResolver(primary Resolver) *fallbackResolver {
	return &fallbackResolver{primary: primary, client: &http.Client{Timeout: 5 * time.Second}, endpoint: "https://cloudflare-dns.com/dns-query", primaryTimeout: time.Second}
}
func dnsNotFound(err error) bool {
	var dns *net.DNSError
	return errors.As(err, &dns) && dns.IsNotFound
}
func dnsError(host string, notFound bool) error {
	return &net.DNSError{Name: host, Err: "DNS response unavailable", IsNotFound: notFound, IsTemporary: !notFound}
}

// Only a failed system lookup uses HTTPS DNS. Negative answers remain negative;
// failure of both resolvers remains unknown and never permits an unchecked send.
func (r *fallbackResolver) LookupMX(ctx context.Context, host string) ([]*net.MX, error) {
	primary, cancel := context.WithTimeout(ctx, r.primaryTimeout)
	mx, err := r.primary.LookupMX(primary, host)
	cancel()
	if err == nil || dnsNotFound(err) {
		return mx, err
	}
	answers, err := r.query(ctx, host, dnsmessage.TypeMX)
	if err != nil {
		return nil, err
	}
	for _, answer := range answers {
		if value, ok := answer.Body.(*dnsmessage.MXResource); ok {
			mx = append(mx, &net.MX{Host: value.MX.String(), Pref: value.Pref})
		}
	}
	if len(mx) == 0 {
		return nil, dnsError(host, true)
	}
	return mx, nil
}
func (r *fallbackResolver) LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error) {
	primary, cancel := context.WithTimeout(ctx, r.primaryTimeout)
	ips, err := r.primary.LookupIPAddr(primary, host)
	cancel()
	if err == nil || dnsNotFound(err) {
		return ips, err
	}
	type answer struct {
		records []dnsmessage.Resource
		err     error
	}
	results := make(chan answer, 2)
	for _, kind := range []dnsmessage.Type{dnsmessage.TypeA, dnsmessage.TypeAAAA} {
		go func(kind dnsmessage.Type) { records, err := r.query(ctx, host, kind); results <- answer{records, err} }(kind)
	}
	var temporary bool
	for i := 0; i < 2; i++ {
		value := <-results
		if value.err != nil && !dnsNotFound(value.err) {
			temporary = true
		}
		for _, record := range value.records {
			switch body := record.Body.(type) {
			case *dnsmessage.AResource:
				ips = append(ips, net.IPAddr{IP: net.IP(body.A[:])})
			case *dnsmessage.AAAAResource:
				ips = append(ips, net.IPAddr{IP: net.IP(body.AAAA[:])})
			}
		}
	}
	if len(ips) > 0 {
		return ips, nil
	}
	return nil, dnsError(host, !temporary)
}

func (r *fallbackResolver) query(ctx context.Context, host string, kind dnsmessage.Type) ([]dnsmessage.Resource, error) {
	name, err := dnsmessage.NewName(strings.TrimSuffix(host, ".") + ".")
	if err != nil {
		return nil, dnsError(host, true)
	}
	request := dnsmessage.Message{Header: dnsmessage.Header{RecursionDesired: true}, Questions: []dnsmessage.Question{{Name: name, Type: kind, Class: dnsmessage.ClassINET}}}
	packed, err := request.Pack()
	if err != nil {
		return nil, dnsError(host, false)
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, r.endpoint, bytes.NewReader(packed))
	if err != nil {
		return nil, dnsError(host, false)
	}
	httpRequest.Header.Set("Content-Type", "application/dns-message")
	httpRequest.Header.Set("Accept", "application/dns-message")
	response, err := r.client.Do(httpRequest)
	if err != nil {
		return nil, dnsError(host, false)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || !strings.HasPrefix(response.Header.Get("Content-Type"), "application/dns-message") {
		return nil, dnsError(host, false)
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, 65537))
	if err != nil || len(raw) > 65536 {
		return nil, dnsError(host, false)
	}
	var message dnsmessage.Message
	if message.Unpack(raw) != nil || !message.Response || message.Truncated || len(message.Questions) != 1 || message.Questions[0] != request.Questions[0] {
		return nil, dnsError(host, false)
	}
	if message.RCode == dnsmessage.RCodeNameError {
		return nil, dnsError(host, true)
	}
	if message.RCode != dnsmessage.RCodeSuccess {
		return nil, dnsError(host, false)
	}
	return message.Answers, nil
}
