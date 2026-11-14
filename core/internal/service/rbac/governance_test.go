package rbac_test

import (
	"billionmail-core/internal/service/rbac"
	"billionmail-core/internal/testsupport"
	"context"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/golang-jwt/jwt/v5"
	"testing"
	"time"
)

func TestRevocationAndAlgorithmRestrictions(t *testing.T) {
	testsupport.Database(t)
	ctx := context.Background()
	service := &rbac.JWTService{Secret: "local-test-signing-key", AccessExpiry: time.Hour, RefreshExpiry: 24 * time.Hour}
	token, _, err := service.GenerateApiToken(1, "test_admin", []string{"admin"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.ParseToken(token); err != nil {
		t.Fatal(err)
	}
	if _, err = g.DB().Exec(ctx, `UPDATE account SET token_version=token_version+1 WHERE account_id=1`); err != nil {
		t.Fatal(err)
	}
	if _, err = service.ParseToken(token); err == nil {
		t.Fatal("revoked long-lived API token still accepted")
	}
	token, _, err = service.GenerateApiToken(1, "test_admin", []string{"admin"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.ParseToken(token); err != nil {
		t.Fatal("fresh token rejected:", err)
	}
	if _, err = g.DB().Exec(ctx, `UPDATE account SET status=0 WHERE account_id=1`); err != nil {
		t.Fatal(err)
	}
	if _, err = service.ParseToken(token); err == nil {
		t.Fatal("disabled account token still accepted")
	}
	forged := jwt.NewWithClaims(jwt.SigningMethodHS512, rbac.JWTCustomClaims{AccountId: 1})
	signed, err := forged.SignedString([]byte(service.Secret))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.ParseToken(signed); err == nil {
		t.Fatal("unexpected signing algorithm accepted")
	}
}
