// Package testsupport creates an isolated disposable database for integration tests.
package testsupport

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"os"
	"testing"
	"time"

	"billionmail-core/internal/service/governance"
	_ "github.com/gogf/gf/contrib/drivers/pgsql/v2"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gcfg"
)

//go:embed base.sql
var base string

// GoFrame's frame singleton retains its initial database name after Close.
// Recreate one process-private name for each sequential, isolated fixture.
var testDatabaseName = fmt.Sprintf("bm_test_%d_%d", os.Getpid(), time.Now().UnixNano())

func Database(t *testing.T) {
	t.Helper()
	ValidEmailChecks(t)
	adapter, e := gcfg.NewAdapterContent(`{"server":{"address":"127.0.0.1:0"},"logger":{"level":"error"}}`)
	if e != nil {
		t.Fatal(e)
	}
	g.Cfg().SetAdapter(adapter)
	host := os.Getenv("BM_TEST_PG_HOST")
	if host == "" {
		t.Skip("set BM_TEST_PG_HOST to run isolated PostgreSQL integration tests")
	}
	port := os.Getenv("BM_TEST_PG_PORT")
	if port == "" {
		port = "5432"
	}
	password := os.Getenv("BM_TEST_PG_PASSWORD")
	admin, err := sql.Open("postgres", fmt.Sprintf("host=%s port=%s user=postgres password=%s dbname=postgres sslmode=disable", host, port, password))
	if err != nil {
		t.Fatal(err)
	}
	name := testDatabaseName
	if _, err = admin.Exec(`CREATE DATABASE ` + name); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		g.DB().Close(context.Background())
		_, _ = admin.Exec(`DROP DATABASE ` + name + ` WITH (FORCE)`)
		_ = admin.Close()
	})
	err = gdb.SetConfig(gdb.Config{"default": gdb.ConfigGroup{{Host: host, Port: port, User: "postgres", Pass: password, Name: name, Type: "pgsql", MaxOpenConnCount: 30}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = g.DB().Exec(context.Background(), base); err != nil {
		t.Fatal("base schema:", err)
	}
	if _, err = g.DB().Exec(context.Background(), `ALTER TABLE api_templates ADD COLUMN IF NOT EXISTS group_id integer NOT NULL DEFAULT 0`); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"account", "role", "bm_contact_groups", "bm_contacts", "email_templates", "email_tasks"} {
		if _, err = g.DB().TableFields(context.Background(), table); err != nil {
			t.Fatal(err)
		}
	}
	if err = governance.Migrate(context.Background()); err != nil {
		t.Fatal("migration:", err)
	}
	// Bootstrap only explicit test accounts. No production environment is read.
	_, err = g.DB().Exec(context.Background(), `INSERT INTO account(username,password,email,status) VALUES('test_admin','test','',1); INSERT INTO role(role_name,status) VALUES('admin',1); INSERT INTO account_role(account_id,role_id) VALUES(1,1); INSERT INTO mailbox(username,password,password_encode,full_name,maildir,local_part,domain,active) VALUES('sender@example.com','test','6447567a64413d3d','Test','test/','sender','example.com',1)`)
	if err != nil {
		t.Fatal(err)
	}
	if err = governance.Migrate(context.Background()); err != nil {
		t.Fatal("repeat migration:", err)
	}
}
