package emailvalidation

import (
	"context"
	"time"
)

type recipientJob struct{ key, email, sender string }

// Recipients verifies import candidates in a shared background queue. Sending
// never calls it. Polling reuses results and does not restart network probes.
func Recipients(ctx context.Context, sender string, emails []string) map[string]Result {
	return Default.recipients(ctx, sender, emails, Check)
}

func (e *Engine) recipients(ctx context.Context, sender string, emails []string, check func(context.Context, string, string, bool) Result) map[string]Result {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	_, domain, valid := Normalize(sender)
	keys := map[string]string{}
	missing := []string{}
	e.mu.Lock()
	for _, email := range emails {
		key := "smtp:" + domain + ":" + email
		keys[email] = key
		if r, ok := e.cached[key]; !ok || !time.Now().Before(r.ExpiresAt) {
			missing = append(missing, key)
		}
	}
	e.mu.Unlock()
	if !valid {
		values := map[string]Result{}
		for email := range keys {
			values[email] = result(Unknown, "sender_required", "smtp")
		}
		return values
	}
	if store, ok := e.Store.(batchCache); ok && len(missing) > 0 {
		saved := store.GetMany(ctx, missing)
		e.mu.Lock()
		for key, r := range saved {
			if current, ok := e.cached[key]; (!ok || !time.Now().Before(current.ExpiresAt)) && time.Now().Before(r.ExpiresAt) {
				e.cached[key] = r
			}
		}
		e.mu.Unlock()
	}
	e.mu.Lock()
	if e.recipientQueued == nil {
		e.recipientQueued = map[string]bool{}
		e.recipientChanged = make(chan struct{})
	}
	for email, key := range keys {
		r, found := e.cached[key]
		if (!found || !time.Now().Before(r.ExpiresAt)) && !e.recipientQueued[key] && len(e.recipientQueued) < 50000 {
			e.recipientQueued[key] = true
			e.recipientQueue = append(e.recipientQueue, recipientJob{key, email, sender})
		}
	}
	for e.recipientWorkers < 8 && e.recipientWorkers < len(e.recipientQueue) {
		e.recipientWorkers++
		go e.runRecipientQueue(check)
	}
	e.mu.Unlock()
	for {
		values := map[string]Result{}
		ready := true
		e.mu.Lock()
		changed := e.recipientChanged
		for email, key := range keys {
			if r, ok := e.cached[key]; ok && time.Now().Before(r.ExpiresAt) {
				values[email] = r
			} else {
				values[email] = result(Unknown, "verification_pending", "smtp")
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

func (e *Engine) runRecipientQueue(check func(context.Context, string, string, bool) Result) {
	for {
		e.mu.Lock()
		if len(e.recipientQueue) == 0 {
			e.recipientWorkers--
			e.mu.Unlock()
			return
		}
		job := e.recipientQueue[0]
		e.recipientQueue = e.recipientQueue[1:]
		e.mu.Unlock()
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		r := check(ctx, job.email, job.sender, true)
		cancel()
		if r.CheckedAt.IsZero() {
			r.CheckedAt = time.Now()
		}
		if r.ExpiresAt.IsZero() {
			r.ExpiresAt = time.Now().Add(30 * time.Second)
		}
		e.mu.Lock()
		e.cached[job.key] = r
		delete(e.recipientQueued, job.key)
		close(e.recipientChanged)
		e.recipientChanged = make(chan struct{})
		e.mu.Unlock()
	}
}
