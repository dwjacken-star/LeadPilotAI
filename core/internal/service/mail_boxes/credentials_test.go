package mail_boxes

import (
	"context"
	"testing"

	v1 "billionmail-core/api/mail_boxes/v1"
	"billionmail-core/internal/testsupport"
	"github.com/GehirnInc/crypt/md5_crypt"
	"github.com/gogf/gf/v2/frame/g"
)

func TestResolveMailboxPassword(t *testing.T) {
	for _, tc := range []struct {
		name, configured, supplied, want string
		wantErr                          bool
	}{
		{"missing", "", "", "", true},
		{"too short", "short", "", "", true},
		{"configured", "TestOnly-Mailbox-123!", "", "TestOnly-Mailbox-123!", false},
		{"explicit overrides configured", "TestOnly-Mailbox-123!", "TestOnly-Explicit-456!", "TestOnly-Explicit-456!", false},
		{"explicit without default", "", "TestOnly-Explicit-456!", "TestOnly-Explicit-456!", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("DEFAULT_NEW_MAILBOX_PASSWORD", tc.configured)
			got, err := resolveMailboxPassword(tc.supplied)
			if (err != nil) != tc.wantErr || got != tc.want {
				t.Fatal("unexpected mailbox password resolution result")
			}
		})
	}
}

func TestMailboxMissingPasswordFailsBeforeDatabase(t *testing.T) {
	t.Setenv("DEFAULT_NEW_MAILBOX_PASSWORD", "")
	ctx := context.Background()
	if err := Add(ctx, &v1.Mailbox{}); err == nil {
		t.Fatal("mailbox creation accepted a missing password")
	}
	if err := AddImport(ctx, &v1.Mailbox{}); err == nil {
		t.Fatal("mailbox import accepted a missing password")
	}
	if _, err := BatchAdd(ctx, "example.com", 0, 2, "test", 0); err == nil {
		t.Fatal("batch creation accepted a missing password")
	}
}

func TestMailboxCredentialDefaults(t *testing.T) {
	const defaultPassword = "TestOnly-Mailbox-123!"
	t.Setenv("DEFAULT_NEW_MAILBOX_PASSWORD", defaultPassword)
	testsupport.Database(t)
	ctx := context.Background()
	verify := func(email, password string) {
		t.Helper()
		row, err := g.DB().Model("mailbox").Where("username", email).One()
		if err != nil || row.IsEmpty() {
			t.Fatal("mailbox missing", err)
		}
		decoded, err := PasswdDecode(ctx, row["password_encode"].String())
		if err != nil || decoded != password {
			t.Fatal("submission password not synchronized", err)
		}
		if err := md5_crypt.New().Verify(row["password"].String(), []byte(password)); err != nil {
			t.Fatal("IMAP password not synchronized", err)
		}
	}
	mailbox := &v1.Mailbox{Username: "default@example.com", LocalPart: "default", Domain: "example.com", FullName: "Default", QuotaActive: 1}
	if err := Add(ctx, mailbox); err != nil {
		t.Fatal(err)
	}
	verify(mailbox.Username, defaultPassword)
	if err := Update(ctx, &v1.Mailbox{Username: mailbox.Username, LocalPart: "default", Domain: "example.com", FullName: "Edited", Active: 1, QuotaActive: 1}); err != nil {
		t.Fatal(err)
	}
	verify(mailbox.Username, defaultPassword)
	if err := Update(ctx, &v1.Mailbox{Username: mailbox.Username, LocalPart: "default", Domain: "example.com", FullName: "Edited", Password: "custom-mail-password", Active: 1, QuotaActive: 1}); err != nil {
		t.Fatal(err)
	}
	verify(mailbox.Username, "custom-mail-password")
	if err := AddImport(ctx, &v1.Mailbox{Username: "import-default@example.com", LocalPart: "import-default", Domain: "example.com", FullName: "Imported"}); err != nil {
		t.Fatal(err)
	}
	verify("import-default@example.com", defaultPassword)
	addresses, err := BatchAdd(ctx, "example.com", 0, 2, "qa-default", 1)
	if err != nil || len(addresses) != 2 {
		t.Fatal("batch creation failed", err)
	}
	for _, email := range addresses {
		verify(email, defaultPassword)
	}
	addresses, err = BatchAdd(ctx, "example.com", 0, 2, "qa-custom", 1, "custom-batch-password")
	if err != nil {
		t.Fatal(err)
	}
	for _, email := range addresses {
		verify(email, "custom-batch-password")
	}
}
