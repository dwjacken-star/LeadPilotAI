package database_initialization

import (
	"billionmail-core/internal/testsupport"
	"context"
	"github.com/gogf/gf/v2/frame/g"
	"testing"
)

func TestRelayRenameIsIdempotent(t *testing.T) {
	testsupport.Database(t)
	ctx := context.Background()
	if err := RenameRelayTable(ctx); err != nil {
		t.Fatal(err)
	}
	if !checkTableExists(ctx, "bm_relay_old") || checkTableExists(ctx, "bm_relay") {
		t.Fatal("legacy table was not renamed")
	}
	if err := RenameRelayTable(ctx); err != nil {
		t.Fatal("repeated migration must be a no-op:", err)
	}
	if _, err := g.DB().Exec(ctx, `CREATE TABLE bm_relay(id serial PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	if err := RenameRelayTable(ctx); err != nil {
		t.Fatal("existing archive must be preserved:", err)
	}
	if !checkTableExists(ctx, "bm_relay_old") || !checkTableExists(ctx, "bm_relay") {
		t.Fatal("existing table unexpectedly changed")
	}
}
