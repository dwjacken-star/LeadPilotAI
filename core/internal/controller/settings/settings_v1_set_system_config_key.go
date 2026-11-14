package settings

import (
	"billionmail-core/internal/consts"
	"billionmail-core/internal/service/governance"
	"billionmail-core/internal/service/mail_service"
	"billionmail-core/internal/service/rbac"
	"context"
	"encoding/json"
	"time"

	"github.com/gogf/gf/os/gtimer"

	"billionmail-core/api/settings/v1"
	"billionmail-core/internal/service/public"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
)

func (c *ControllerV1) SetSystemConfigKey(ctx context.Context, req *v1.SetSystemConfigKeyReq) (res *v1.SetSystemConfigKeyRes, err error) {
	res = &v1.SetSystemConfigKeyRes{}
	if req.Key == "outbound_identity" {
		p, e := governance.Load(ctx, rbac.GetCurrentAccountId(ctx))
		if e != nil || p == nil || !p.Super {
			res.SetError(gerror.New("administrator required"))
			return res, nil
		}
		var identity mail_service.DeliveryIdentity
		if e = json.Unmarshal([]byte(req.Value), &identity); e == nil {
			e = mail_service.SaveOutboundIdentity(ctx, identity)
		}
		if e != nil {
			res.SetError(e)
		} else {
			res.SetSuccess("Saved")
		}
		return res, nil
	}
	if credentialKey(req.Key) {
		username, password := "", ""
		if strings.EqualFold(req.Key, "admin_username") {
			username = req.Value
		} else {
			password = req.Value
		}
		if err := changeAdminCredentials(ctx, rbac.GetCurrentAccountId(ctx), username, password, req.CurrentPassword); err != nil {
			res.SetError(err)
			return res, nil
		}
		// Never include credentials or their request body in operational logs.
		_ = public.WriteLog(ctx, public.LogParams{Type: consts.LOGTYPE.Settings, Log: "Updated administrator credentials; previous sessions revoked"})
		res.SetSuccess(public.LangCtx(ctx, "账号设置已更新，请使用新凭据重新登录"))
		return res, nil
	}

	// Parameter validation
	if err := validateConfigValue(req.Key, req.Value); err != nil {
		res.SetError(gerror.New(public.LangCtx(ctx, "Parameter validation failed: {}", err)))
		return res, nil
	}

	// ip_whitelist_enable The whitelist cannot be empty when the switch is on
	if strings.ToLower(req.Key) == "ip_whitelist_enable" && req.Value == "true" {
		whitelist, err := GetIPWhitelist()
		if err != nil {
			res.SetError(gerror.New(public.LangCtx(ctx, "Failed to get IP whitelist: {}", err)))
			return res, nil
		}
		if len(whitelist) == 0 {
			res.SetError(gerror.New(public.LangCtx(ctx, "IP whitelist list is empty. Whitelist function cannot be enabled")))
			return res, nil
		}
	}

	// 1. Read the current .env file content
	envMap, err := public.LoadEnvFile()
	if err != nil {
		res.SetError(gerror.New(public.LangCtx(ctx, "Failed to read environment variable file: {}", err)))
		return res, nil
	}

	// 2. Find the corresponding environment variable name based on the key
	envKey, modified := convertJsonKeyToEnvKey(req.Key, req.Value, envMap)
	if !modified {
		res.SetError(gerror.New(public.LangCtx(ctx, "Invalid configuration item or value not changed")))
		return res, nil
	}

	// 3. If there is a modification, write to the file
	if err := saveEnvFile(envMap); err != nil {
		res.SetError(gerror.New(public.LangCtx(ctx, "Failed to update environment variable file: {}", err)))
		return res, nil
	}

	// 4. Restart the container
	gtimer.AddOnce(500*time.Millisecond, func() {
		err = public.DockerApiFromCtx(ctx).RestartContainerByName(context.Background(), consts.SERVICES.Core)
		if err != nil {
			g.Log().Error(ctx, "Failed to restart container: {}", err)
			return
		}

	})

	_ = public.WriteLog(ctx, public.LogParams{
		Type: consts.LOGTYPE.Settings,
		Log:  "Changed configuration item: " + envKey,
		Data: g.Map{"key": envKey},
	})

	res.SetSuccess(public.LangCtx(ctx, "Configuration updated successfully"))
	return res, nil
}

// convertJsonKeyToEnvKey Convert JSON configuration key to environment variable key and update value
func convertJsonKeyToEnvKey(jsonKey string, value string, envMap map[string]string) (string, bool) {
	// JSON key to environment variable key mapping
	keyMapping := map[string]string{
		"safe_path":            "SafePath",
		"billionmail_hostname": "BILLIONMAIL_HOSTNAME",
		"db_name":              "DBNAME",
		"db_user":              "DBUSER",
		"db_pass":              "DBPASS",
		"redis_pass":           "REDISPASS",
		"redis_port":           "REDIS_PORT",
		"smtp":                 "SMTP_PORT",
		"smtps":                "SMTPS_PORT",
		"submission":           "SUBMISSION_PORT",
		"imap":                 "IMAP_PORT",
		"imaps":                "IMAPS_PORT",
		"pop":                  "POP_PORT",
		"pops":                 "POPS_PORT",
		"http":                 "HTTP_PORT",
		"https":                "HTTPS_PORT",
		"timezone":             "TZ",
		"ipv4_network":         "IPV4_NETWORK",
		"fail2ban":             "FAIL2BAN_INIT",
		"ip_whitelist_enable":  "IP_WHITELIST_ENABLE",
		"retention_days":       "RETENTION_DAYS",
	}

	// Get the environment variable key name
	envKey, exists := keyMapping[strings.ToLower(jsonKey)]
	if !exists {
		return "", false
	}

	// Special handling for fail2ban values
	if envKey == "FAIL2BAN_INIT" {
		if value == "1" {
			value = "y"
		} else if value == "0" {
			value = "n"
		}
	}

	// Check if the value has changed
	if currentValue, exists := envMap[envKey]; exists && currentValue == value {
		return envKey, false
	}

	// Update the environment variable map
	envMap[envKey] = value
	g.Log().Debug(nil, "Update configuration item: %s", envKey)

	return envKey, true
}
