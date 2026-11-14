package emailvalidation

import (
	"context"
	"time"
)

type batchCache interface {
	GetMany(context.Context, []string) map[string]Result
}

// Domains returns a bounded preview while queued work survives HTTP cancellation.
// All previews share 16 DNS workers; completed domains are reused on later polls.
func (e *Engine) Domains(ctx context.Context, domains []string) map[string]Result {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	missing := []string{}
	seen := map[string]bool{}
	e.mu.Lock()
	for _, domain := range domains {
		if !seen[domain] {
			seen[domain] = true
			if r, ok := e.cached["dns:"+domain]; !ok || !time.Now().Before(r.ExpiresAt) {
				missing = append(missing, "dns:"+domain)
			}
		}
	}
	e.mu.Unlock()
	if e.Store != nil && len(missing) > 0 {
		saved := map[string]Result{}
		if store, ok := e.Store.(batchCache); ok {
			saved = store.GetMany(ctx, missing)
		} else {
			for _, key := range missing {
				if ctx.Err() != nil {
					break
				}
				if r, ok := e.Store.Get(ctx, key); ok {
					saved[key] = r
				}
			}
		}
		e.mu.Lock()
		for key, r := range saved {
			if current, ok := e.cached[key]; (!ok || !time.Now().Before(current.ExpiresAt)) && time.Now().Before(r.ExpiresAt) {
				e.cached[key] = r
			}
		}
		e.mu.Unlock()
	}
	e.mu.Lock()
	if e.domainQueued == nil {
		e.domainQueued = map[string]bool{}
		e.domainChanged = make(chan struct{})
	}
	for domain := range seen {
		r, found := e.cached["dns:"+domain]
		if (!found || !time.Now().Before(r.ExpiresAt)) && !e.domainQueued[domain] && len(e.domainQueued) < 50000 {
			e.domainQueued[domain] = true
			e.domainQueue = append(e.domainQueue, domain)
		}
	}
	for e.domainWorkers < 16 && e.domainWorkers < len(e.domainQueue) {
		e.domainWorkers++
		go e.runDomainQueue()
	}
	e.mu.Unlock()
	for {
		values := map[string]Result{}
		ready := true
		e.mu.Lock()
		changed := e.domainChanged
		for domain := range seen {
			if r, ok := e.cached["dns:"+domain]; ok && time.Now().Before(r.ExpiresAt) {
				values[domain] = r
			} else {
				values[domain] = result(Unknown, "verification_pending", "dns")
				ready = false
			}
		}
		e.mu.Unlock()
		if ready || ctx.Err() != nil {
			return values
		}
		select {
		case <-changed:
		case <-ctx.Done():
		}
	}
}

func (e *Engine) runDomainQueue() {
	for {
		e.mu.Lock()
		if len(e.domainQueue) == 0 {
			e.domainWorkers--
			e.mu.Unlock()
			return
		}
		domain := e.domainQueue[0]
		e.domainQueue = e.domainQueue[1:]
		e.mu.Unlock()
		ctx, cancel := context.WithTimeout(context.Background(), 14*time.Second)
		r := e.Domain(ctx, domain)
		cancel()
		e.mu.Lock()
		// A timeout must remain retryable, with an explanation for the preview.
		if current, ok := e.cached["dns:"+domain]; !ok || !time.Now().Before(current.ExpiresAt) {
			e.cached["dns:"+domain] = r
		}
		delete(e.domainQueued, domain)
		close(e.domainChanged)
		e.domainChanged = make(chan struct{})
		e.mu.Unlock()
	}
}
