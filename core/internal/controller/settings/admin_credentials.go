package settings

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"golang.org/x/crypto/bcrypt"
)

// Credentials belong to account, not to the bootstrap values in .env.
// Locking the row makes reauthentication and session revocation one transaction.
func changeAdminCredentials(ctx context.Context, accountID int64, username, password, currentPassword string) error {
	if accountID <= 0 || currentPassword == "" {
		return errors.New("请输入当前账号的密码")
	}
	if username == "" && password == "" {
		return errors.New("请填写新的账号或密码")
	}
	if username != "" {
		if err := validateConfigValue("admin_username", username); err != nil {
			return err
		}
	}
	if password != "" {
		if utf8.RuneCountInString(password) < 8 || len(password) > 72 {
			return errors.New("新密码至少8位，过长时请使用更短的密码")
		}
	}
	return g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		row, err := tx.Model("account").Where("account_id", accountID).LockUpdate().One()
		if err != nil {
			return err
		}
		if row.IsEmpty() || row["status"].Int() != 1 {
			return errors.New("当前账号不可用")
		}
		allowed, err := tx.Model("account_role ar").InnerJoin("role r", "r.role_id=ar.role_id").Where("ar.account_id", accountID).Where("r.role_name", "admin").Where("r.status", 1).Count()
		if err != nil {
			return err
		}
		if allowed == 0 {
			return errors.New("仅系统管理员可修改此设置")
		}
		if bcrypt.CompareHashAndPassword([]byte(row["password"].String()), []byte(currentPassword)) != nil {
			return errors.New("当前密码不正确，账号设置未修改")
		}
		data := g.Map{"token_version": gdb.Raw("token_version+1"), "update_time": time.Now().Unix()}
		if username != "" {
			if username == row["username"].String() {
				return errors.New("新账号与当前账号相同")
			}
			n, err := tx.Model("account").Where("username", username).WhereNot("account_id", accountID).Count()
			if err != nil {
				return err
			}
			if n > 0 {
				return errors.New("该账号名称已被使用")
			}
			data["username"] = username
		}
		if password != "" {
			if bcrypt.CompareHashAndPassword([]byte(row["password"].String()), []byte(password)) == nil {
				return errors.New("新密码不能与当前密码相同")
			}
			hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
			if err != nil {
				return err
			}
			data["password"] = string(hash)
		}
		_, err = tx.Model("account").Where("account_id", accountID).Data(data).Update()
		return err
	})
}

func credentialKey(key string) bool {
	key = strings.ToLower(key)
	return key == "admin_username" || key == "admin_password"
}
