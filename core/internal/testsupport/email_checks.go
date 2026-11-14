package testsupport

import (
	"billionmail-core/internal/service/emailvalidation"
	"context"
	"net"
	"testing"
	"time"
)

type mailResolver struct{}

func (mailResolver) LookupMX(context.Context, string) ([]*net.MX, error) {
	return []*net.MX{{Host: "mx.fixture.test."}}, nil
}
func (mailResolver) LookupIPAddr(context.Context, string) ([]net.IPAddr, error) {
	return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}, nil
}

// Network-free dependency injection, used only by tests with synthetic recipients.
func ValidEmailChecks(t *testing.T) {
	old := emailvalidation.Default
	engine := emailvalidation.New(mailResolver{})
	engine.Probe = func(context.Context, emailvalidation.Result, string, string) emailvalidation.Result {
		return emailvalidation.Result{Status: emailvalidation.Valid, Code: "smtp_accepted", Stage: "smtp", CheckedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour)}
	}
	emailvalidation.Default = engine
	t.Cleanup(func() { emailvalidation.Default = old })
}
