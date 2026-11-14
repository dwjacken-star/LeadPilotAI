package settings

import (
	"context"
	"testing"
	"time"

	v1 "billionmail-core/api/settings/v1"
	"billionmail-core/internal/service/rbac"
	"billionmail-core/internal/testsupport"
	"github.com/gogf/gf/v2/frame/g"
	"golang.org/x/crypto/bcrypt"
)

func TestAdminCredentialSettings(t *testing.T) {
	testsupport.Database(t)
	ctx := context.WithValue(context.Background(), "accountId", int64(1))
	const oldPassword = "original-password-123"
	const newPassword = "TestOnly-Admin-Password-123!"
	hash, _ := bcrypt.GenerateFromPassword([]byte(oldPassword), bcrypt.DefaultCost)
	if _, err := g.DB().Exec(ctx, `UPDATE account SET password=$1 WHERE account_id=1`, string(hash)); err != nil {
		t.Fatal(err)
	}
	if _, err := g.DB().Exec(ctx, `INSERT INTO account(username,password,email,status) VALUES('taken_admin',$1,'',1)`, string(hash)); err != nil {
		t.Fatal(err)
	}
	jwt := &rbac.JWTService{Secret: "settings-regression-secret", AccessExpiry: time.Hour, RefreshExpiry: time.Hour}
	oldToken, _, err := jwt.GenerateApiToken(1, "test_admin", []string{"admin"})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name                        string
		id                          int64
		username, password, current string
	}{
		{"missing current password", 1, "new_admin", "", ""},
		{"wrong current password", 1, "new_admin", "", "incorrect-password"},
		{"duplicate username", 1, "taken_admin", "", oldPassword},
		{"invalid username", 1, "bad name", "", oldPassword},
		{"too short password", 1, "", "short", oldPassword},
		{"seven characters", 1, "", "1234567", oldPassword},
		{"too long password", 1, "", "123456789012345678901234567890123456789012345678901234567890123456789012345", oldPassword},
		{"same password", 1, "", oldPassword, oldPassword},
		{"non administrator", 2, "another_admin", "", oldPassword},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := changeAdminCredentials(ctx, tc.id, tc.username, tc.password, tc.current); err == nil {
				t.Fatal("invalid credential change accepted")
			}
		})
	}
	row, err := g.DB().Model("account").Where("account_id", 1).One()
	if err != nil || row["username"].String() != "test_admin" || row["token_version"].Int64() != 0 {
		t.Fatal("failed requests modified account", err)
	}
	controller := &ControllerV1{}
	res, err := controller.SetSystemConfigKey(ctx, &v1.SetSystemConfigKeyReq{Key: "admin_username", Value: "new_admin", CurrentPassword: oldPassword})
	if err != nil || !res.Success {
		t.Fatalf("username endpoint failed: %+v %v", res, err)
	}
	if _, err = rbac.Account().Login(ctx, "new_admin", oldPassword); err != nil {
		t.Fatal("new username cannot log in", err)
	}
	if _, err = rbac.Account().Login(ctx, "test_admin", oldPassword); err == nil {
		t.Fatal("old username still logs in")
	}
	if _, err = jwt.ParseToken(oldToken); err == nil {
		t.Fatal("old access token still valid")
	}
	res, err = controller.SetSystemConfigKey(ctx, &v1.SetSystemConfigKeyReq{Key: "admin_password", Value: newPassword, CurrentPassword: oldPassword})
	if err != nil || !res.Success {
		t.Fatalf("password endpoint failed: %+v %v", res, err)
	}
	if _, err = rbac.Account().Login(ctx, "new_admin", newPassword); err != nil {
		t.Fatal("new password cannot log in", err)
	}
	if _, err = rbac.Account().Login(ctx, "new_admin", oldPassword); err == nil {
		t.Fatal("old password still logs in")
	}
	row, err = g.DB().Model("account").Where("account_id", 1).One()
	if err != nil || row["password"].String() == newPassword || row["token_version"].Int64() != 2 {
		t.Fatal("password storage or session version invalid", err)
	}
	// Bulk settings must use the same database-backed credential flow.
	bulk, err := controller.SetSystemConfig(ctx, &v1.SetSystemConfigReq{SystemConfig: v1.SystemConfig{AdminUsername: "final_admin", AdminPassword: oldPassword}, CurrentPassword: newPassword})
	if err != nil || !bulk.Success {
		t.Fatalf("bulk credential update failed: %+v %v", bulk, err)
	}
	if _, err = rbac.Account().Login(ctx, "final_admin", oldPassword); err != nil {
		t.Fatal(err)
	}
	config := convertEnvToConfig(map[string]string{"ADMIN_PASSWORD": "bootstrap-secret"})
	if config.AdminPassword != "" {
		t.Fatal("settings exposes bootstrap password")
	}
	if _, changed := convertJsonKeyToEnvKey("admin_password", newPassword, map[string]string{}); changed {
		t.Fatal("credentials still written to bootstrap environment")
	}
}
