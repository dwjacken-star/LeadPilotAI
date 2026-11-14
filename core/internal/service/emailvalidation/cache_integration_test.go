package emailvalidation_test

import (
	"context"
	"testing"
	"time"

	"billionmail-core/internal/service/emailvalidation"
	"billionmail-core/internal/testsupport"
)

func TestVerificationCachePersistence(t *testing.T) {
	store := emailvalidation.Default.Store
	testsupport.Database(t)
	ctx := context.Background()
	check := emailvalidation.Result{Status: emailvalidation.Unknown, Code: "dns_temporary", Stage: "dns", CheckedAt: time.Now().UTC(), ExpiresAt: time.Now().UTC().Add(time.Minute)}
	store.Put(ctx, "dns:cache-fixture.test", check)
	if got, ok := store.Get(ctx, "dns:cache-fixture.test"); !ok || got.Status != check.Status || got.Code != check.Code {
		t.Fatalf("DNS result not persisted: %+v found=%v", got, ok)
	}
	check.Status = emailvalidation.Valid
	check.Code = "mx_valid"
	check.Routes = []emailvalidation.Route{{Host: "mx.fixture.test", IPs: []string{"8.8.8.8"}}}
	store.Put(ctx, "dns:cache-fixture.test", check)
	if got, ok := store.Get(ctx, "dns:cache-fixture.test"); !ok || got.Status != check.Status || len(got.Routes) != 1 {
		t.Fatalf("updated DNS result not persisted: %+v found=%v", got, ok)
	}
	check.Stage = "smtp"
	check.Code = "catch_all"
	check.Status = emailvalidation.Unknown
	store.Put(ctx, "smtp:sender.test:buyer@client.test", check)
	if got, ok := store.Get(ctx, "smtp:sender.test:buyer@client.test"); !ok || got.Code != "catch_all" || got.Status != emailvalidation.Unknown {
		t.Fatalf("SMTP result not persisted: %+v found=%v", got, ok)
	}
	bulk := store.(interface {
		GetMany(context.Context, []string) map[string]emailvalidation.Result
	}).GetMany(ctx, []string{"dns:cache-fixture.test", "smtp:sender.test:buyer@client.test", "dns:absent.test"})
	if len(bulk) != 2 || bulk["dns:cache-fixture.test"].Status != emailvalidation.Valid || bulk["smtp:sender.test:buyer@client.test"].Code != "catch_all" {
		t.Fatal("bulk cache read lost verification data", bulk)
	}
}
