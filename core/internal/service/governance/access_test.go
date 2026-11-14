package governance_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"billionmail-core/internal/service/governance"
	"billionmail-core/internal/testsupport"
	"github.com/gogf/gf/v2/frame/g"
)

func TestScopeDoesNotCombineUnrelatedRoleAuthority(t *testing.T) {
	p := &governance.Principal{ID: 7, DeptID: 2, Roles: []governance.Role{{Scope: "all", Permissions: []string{"contact:list"}}, {Scope: "self", Permissions: []string{"contact:update"}}}}
	predicate, err := p.Predicate(context.Background(), "contact:update", "")
	if err != nil {
		t.Fatal(err)
	}
	if predicate != "((dept_id=2 AND owner_id=7))" {
		t.Fatalf("write authority escaped self scope: %s", predicate)
	}
	no, _ := p.Predicate(context.Background(), "campaign:publish", "")
	if no != "FALSE" {
		t.Fatal("unknown permission must deny")
	}
}
func TestPolicyWindow(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Shanghai")
	p := governance.Policy{Timezone: "Asia/Shanghai", WindowStart: "09:00", WindowEnd: "18:00", Weekdays: []int{1, 2, 3, 4, 5}}
	friday := time.Date(2026, 11, 6, 18, 0, 0, 0, loc)
	want := time.Date(2026, 11, 9, 9, 0, 0, 0, loc)
	if got := p.Window(friday); !got.Equal(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	if err := (governance.Policy{WindowStart: "18:00", WindowEnd: "09:00"}).Validate(); err == nil {
		t.Fatal("invalid window accepted")
	}
}
func TestSharedAtomicQuotas(t *testing.T) {
	testsupport.Database(t)
	ctx := context.Background()
	limits := []governance.Limit{{Key: "global:test", Policy: governance.Policy{PerMinute: 3}}, {Key: "sender:test", Policy: governance.Policy{PerMinute: 2}}}
	var accepted atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			release, err := governance.Reserve(ctx, limits, 1, fmt.Sprintf("r%d@example.com", i), fmt.Sprint(i))
			if err == nil {
				accepted.Add(1)
				release()
			} else {
				var d *governance.Deferred
				if !errors.As(err, &d) {
					t.Error(err)
				}
			}
		}(i)
	}
	wg.Wait()
	if accepted.Load() != 2 {
		t.Fatalf("shared sender cap violated: %d", accepted.Load())
	}
	n, err := g.DB().Model("bm_rate_events").Where("scope_key", "global:test").Count()
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("rejected reservations leaked global quota: %d", n)
	}
	cooldown := []governance.Limit{{Key: "task:touch", Policy: governance.Policy{RecipientCooldownHours: 24}}}
	release, err := governance.Reserve(ctx, cooldown, 1, "USER@example.com", "touch1")
	if err != nil {
		t.Fatal(err)
	}
	release()
	if _, err = governance.Reserve(ctx, cooldown, 1, "user@example.com", "touch2"); err == nil {
		t.Fatal("contact cooldown did not span tasks / normalized address")
	}
	batch := []governance.Limit{{Key: "batch:test", Policy: governance.Policy{BatchSize: 2, BatchRestSeconds: 60}}}
	for i := 0; i < 2; i++ {
		release, e := governance.Reserve(ctx, batch, 1, "batch@example.com", fmt.Sprintf("batch%d", i))
		if e != nil {
			t.Fatal(e)
		}
		release()
	}
	if _, e := governance.Reserve(ctx, batch, 1, "batch@example.com", "batch3"); e == nil {
		t.Fatal("batch rest ignored")
	}
	concurrent := []governance.Limit{{Key: "concurrent:test", Policy: governance.Policy{Concurrency: 1}}}
	release, e := governance.Reserve(ctx, concurrent, 1, "one@example.com", "slot1")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = governance.Reserve(ctx, concurrent, 1, "two@example.com", "slot2"); e == nil {
		t.Fatal("concurrency cap ignored")
	}
	release()
	release, e = governance.Reserve(ctx, concurrent, 1, "two@example.com", "slot3")
	if e != nil {
		t.Fatal("released slot was not reusable:", e)
	}
	release()
	_, e = g.DB().Model("bm_send_policies").Data(g.Map{"scope": "sender", "scope_key": "*", "policy": `{"per_minute":7}`}).Insert()
	if e != nil {
		t.Fatal(e)
	}
	_, e = g.DB().Model("bm_send_policies").Data(g.Map{"scope": "sender", "scope_key": "specific@example.com", "policy": `{"per_minute":3}`}).Insert()
	if e != nil {
		t.Fatal(e)
	}
	for sender, want := range map[string]int{"other@example.com": 7, "specific@example.com": 3} {
		limits, e := governance.Limits(ctx, 1, sender, "1.2.3.4", "other", 1, governance.Policy{})
		if e != nil {
			t.Fatal(e)
		}
		found := false
		for _, limit := range limits {
			if limit.Key == "sender:"+sender {
				found = true
				if limit.Policy.PerMinute != want {
					t.Fatal("specific policy did not override sender default", limit)
				}
			}
		}
		if !found {
			t.Fatal("sender wildcard policy did not apply", sender)
		}
	}
}
