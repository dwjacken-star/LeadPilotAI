package emailvalidation

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestRecipientImportQueueContinuesAndDeduplicates(t *testing.T) {
	e := New(fakeResolver{})
	emails := make([]string, 3000)
	for i := range emails {
		emails[i] = fmt.Sprintf("buyer-%d@client.test", i)
	}
	var active, peak, calls atomic.Int32
	check := func(ctx context.Context, email, sender string, deep bool) Result {
		if !deep {
			t.Error("import skipped SMTP check")
		}
		n := active.Add(1)
		defer active.Add(-1)
		for old := peak.Load(); n > old; old = peak.Load() {
			if peak.CompareAndSwap(old, n) {
				break
			}
		}
		calls.Add(1)
		time.Sleep(time.Millisecond)
		return result(Valid, "smtp_accepted", "smtp")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	initial := e.recipients(ctx, "sender@example.com", emails, check)
	if len(initial) != 3000 {
		t.Fatal("initial preview lost addresses")
	}
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); e.recipients(ctx, "sender@example.com", emails, check) }()
	}
	wg.Wait()
	final := e.recipients(context.Background(), "sender@example.com", emails, check)
	for email, r := range final {
		if r.Status != Valid {
			t.Fatalf("verification never completed for %s: %+v", email, r)
		}
	}
	if calls.Load() != 3000 || peak.Load() > 8 {
		t.Fatal("duplicate probes or unbounded verification concurrency", calls.Load(), peak.Load())
	}
}
