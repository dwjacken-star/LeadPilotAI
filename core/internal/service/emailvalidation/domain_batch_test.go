package emailvalidation

import (
	"context"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"
)

type queuedResolver struct {
	mu           sync.Mutex
	calls        map[string]int
	active, peak int
}

func (r *queuedResolver) LookupMX(ctx context.Context, domain string) ([]*net.MX, error) {
	r.mu.Lock()
	r.calls[domain]++
	r.active++
	if r.active > r.peak {
		r.peak = r.active
	}
	r.mu.Unlock()
	defer func() { r.mu.Lock(); r.active--; r.mu.Unlock() }()
	select {
	case <-time.After(2 * time.Millisecond):
		return []*net.MX{{Host: "mx.fixture.test."}}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
func (*queuedResolver) LookupIPAddr(context.Context, string) ([]net.IPAddr, error) {
	return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}, nil
}

func TestLargeImportDNSContinuesAfterPreviewCancellation(t *testing.T) {
	resolver := &queuedResolver{calls: map[string]int{}}
	engine := New(resolver)
	domains := make([]string, 3000)
	for i := range domains {
		domains[i] = fmt.Sprintf("domain-%d.test", i)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	initial := engine.Domains(ctx, domains)
	if len(initial) != 3000 {
		t.Fatal("cancelled preview lost rows")
	}
	for _, r := range initial {
		if r.Status != Unknown && r.Status != Valid {
			t.Fatal("unfinished DNS classified invalid", r)
		}
	}
	var polls sync.WaitGroup
	for i := 0; i < 8; i++ {
		polls.Add(1)
		go func() { defer polls.Done(); engine.Domains(ctx, domains) }()
	}
	polls.Wait()
	completed := engine.Domains(context.Background(), domains)
	for domain, r := range completed {
		if r.Status != Valid {
			t.Fatalf("queued DNS never completed for %s: %+v", domain, r)
		}
	}
	resolver.mu.Lock()
	defer resolver.mu.Unlock()
	if resolver.peak > 16 {
		t.Fatalf("unbounded DNS workers: %d", resolver.peak)
	}
	if len(resolver.calls) != 3000 {
		t.Fatalf("missing domains: %d", len(resolver.calls))
	}
	for domain, count := range resolver.calls {
		if count != 1 {
			t.Fatalf("polling repeated DNS for %s: %d", domain, count)
		}
	}
}
