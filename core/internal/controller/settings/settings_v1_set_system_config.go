package settings

import (
	"billionmail-core/internal/consts"
	"billionmail-core/internal/service/public"
	"billionmail-core/internal/service/rbac"
	"context"
	"github.com/gogf/gf/os/gtimer"
	"github.com/gogf/gf/v2/os/gfile"
	"os"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"

	"billionmail-core/api/settings/v1"
)

func (c *ControllerV1) SetSystemConfig(ctx context.Context, req *v1.SetSystemConfigReq) (res *v1.SetSystemConfigRes, err error) {
	res = &v1.SetSystemConfigRes{}
	if req.AdminUsername != "" || req.AdminPassword != "" {
		if req.SafePath != "" || req.Hostname != "" {
			res.SetError(gerror.New("请分别保存账号设置与网络设置"))
			return res, nil
		}
		if err := changeAdminCredentials(ctx, rbac.GetCurrentAccountId(ctx), req.AdminUsername, req.AdminPassword, req.CurrentPassword); err != nil {
			res.SetError(err)
			return res, nil
		}
		_ = public.WriteLog(ctx, public.LogParams{Type: consts.LOGTYPE.Settings, Log: "Updated administrator credentials; previous sessions revoked"})
		res.SetSuccess(public.LangCtx(ctx, "账号设置已更新，请使用新凭据重新登录"))
		return res, nil
	}

	// Parameter validation
	if req.AdminUsername != "" {
		if err := validateConfigValue("ADMIN_USERNAME", req.AdminUsername); err != nil {
			res.SetError(gerror.New(public.LangCtx(ctx, "Parameter validation failed: {}", err)))
			return res, nil
		}
	}
	if req.AdminPassword != "" {
		if err := validateConfigValue("ADMIN_PASSWORD", req.AdminPassword); err != nil {
			res.SetError(gerror.New(public.LangCtx(ctx, "Parameter validation failed: {}", err)))
			return res, nil
		}
	}
	if req.SafePath != "" {
		if err := validateConfigValue("SafePath", req.SafePath); err != nil {
			res.SetError(gerror.New(public.LangCtx(ctx, "Parameter validation failed: {}", err)))
			return res, nil
		}
	}
	if req.Hostname != "" {
		if err := validateConfigValue("BILLIONMAIL_HOSTNAME", req.Hostname); err != nil {
			res.SetError(gerror.New(public.LangCtx(ctx, "Parameter validation failed: {}", err)))
			return res, nil
		}
	}

	envMap, err := public.LoadEnvFile()
	if err != nil {
		res.SetError(gerror.New(public.LangCtx(ctx, "Failed to read environment variable file: {}", err)))
		return res, nil
	}

	// 2. Update the configuration items that need to be modified
	modified := updateEnvMap(envMap, req)

	// 3. If there is a modification, write to the file
	if modified {

		if err := saveEnvFile(envMap); err != nil {
			res.SetError(gerror.New(public.LangCtx(ctx, "Failed to update environment variable file: {}", err)))
			return res, nil
		}

		gtimer.AddOnce(500*time.Millisecond, func() {
			err = public.DockerApiFromCtx(ctx).RestartContainerByName(context.Background(), consts.SERVICES.Core)
			if err != nil {
				return
			}
		})

		_ = public.WriteLog(ctx, public.LogParams{
			Type: consts.LOGTYPE.Settings,
			Log:  "Updated system configuration",
			Data: map[string]interface{}{"safe_path": req.SafePath, "hostname": req.Hostname},
		})

	}
	res.SetSuccess(public.LangCtx(ctx, "Configuration updated successfully"))
	return res, nil
}

// updateEnvMap Update the environment variable map
func updateEnvMap(envMap map[string]string, req *v1.SetSystemConfigReq) bool {
	modified := false

	// Basic configuration
	if req.SafePath != "" && req.SafePath != envMap["SafePath"] {
		envMap["SafePath"] = req.SafePath
		modified = true

	}
	if req.Hostname != "" && req.Hostname != envMap["BILLIONMAIL_HOSTNAME"] {
		envMap["BILLIONMAIL_HOSTNAME"] = req.Hostname
		modified = true

	}

	return modified
}

// saveEnvFile Save the environment variable to the file
func saveEnvFile(envMap map[string]string) error {
	// 1. Read the original file content
	content, err := os.ReadFile(envPath)
	if err != nil {
		return err
	}

	// 2. Process by line
	var newLines []string
	lines := strings.Split(string(content), "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Keep comment lines and empty lines
		if line == "" || strings.HasPrefix(line, "#") {
			newLines = append(newLines, line)
			continue
		}

		// Parse key-value pairs
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			newLines = append(newLines, line)
			continue
		}

		key := strings.TrimSpace(parts[0])
		// If envMap has this key, update the value
		if val, ok := envMap[key]; ok {

			newLines = append(newLines, key+"="+val)
			delete(envMap, key) // Mark as processed
		} else {
			newLines = append(newLines, line)
		}
	}

	// 3. Add new key-value pairs in envMap
	for k, v := range envMap {
		newLines = append(newLines, k+"="+v)
	}

	// 4. Write to file
	return gfile.PutContents(envPath, strings.Join(newLines, "\n"))
}
