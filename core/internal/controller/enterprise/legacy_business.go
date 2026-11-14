package enterprise

import (
	"billionmail-core/internal/service/governance"
	"errors"

	"github.com/gogf/gf/v2/net/ghttp"
)

type businessRoute struct {
	method, permission, table, idField, operation string
}

var legacyBusinessRoutes = map[string]businessRoute{
	"/api/overview":                          {"GET", "overview:view", "", "", ""},
	"/api/overview/failed":                   {"GET", "overview:view", "", "", ""},
	"/api/overview/domains":                  {"GET", "overview:view", "", "", ""},
	"/api/batch_mail/task/list":              {"GET", "campaign:list", "", "", ""},
	"/api/batch_mail/task/find":              {"GET", "campaign:list", "email_tasks", "id", ""},
	"/api/batch_mail/task/stat_chart":        {"GET", "campaign:list", "email_tasks", "task_id", ""},
	"/api/batch_mail/task/send_count":        {"GET", "campaign:list", "email_tasks", "query_task_id", ""},
	"/api/batch_mail/tracking/mail_provider": {"GET", "campaign:list", "email_tasks", "task_id", ""},
	"/api/batch_mail/tracking/logs":          {"GET", "campaign:list", "email_tasks", "task_id", ""},
	"/api/batch_mail/task/create":            {"POST", "campaign:create", "", "", ""},
	"/api/batch_mail/task/update":            {"POST", "campaign:update", "email_tasks", "task_id", ""},
	"/api/batch_mail/task/delete":            {"POST", "campaign:delete", "email_tasks", "id", ""},
	"/api/batch_mail/task/pause":             {"POST", "campaign:pause", "email_tasks", "task_id", "pause"},
	"/api/batch_mail/task/resume":            {"POST", "campaign:publish", "email_tasks", "task_id", "publish"},
	"/api/batch_mail/task/send_test":         {"POST", "campaign:publish", "", "", ""},
	"/api/email_template/list":               {"GET", "template:list", "", "", ""},
	"/api/email_template/create":             {"POST", "template:create", "", "", ""},
	"/api/email_template/update":             {"POST", "template:update", "email_templates", "id", ""},
	"/api/email_template/delete":             {"POST", "template:delete", "email_templates", "id", ""},
	"/api/email_template/copy":               {"POST", "template:create", "", "", ""},
	"/api/email_template/options":            {"GET", "template:create", "", "", ""},
	"/api/email_template/all":                {"GET", "template:list", "", "", ""},
	"/api/email_template/get":                {"GET", "template:list", "email_templates", "id", ""},
}

func legacyBusinessAccess(r *ghttp.Request) bool {
	if templateAIAccess(r) {
		return true
	}
	rule, ok := legacyBusinessRoutes[r.URL.Path]
	if !ok {
		return false
	}
	p, err := governance.Load(r.GetCtx(), r.GetCtxVar("accountId").Int64())
	if err == nil && p.Super {
		r.Middleware.Next()
		return true
	}
	if err == nil && (r.Method != rule.method || !p.Has(rule.permission)) {
		err = errors.New("permission required: " + rule.permission)
	}
	var id int64
	if err == nil && rule.table != "" {
		var input struct {
			ID     int64 `json:"id"`
			TaskID int64 `json:"task_id"`
		}
		err = r.Parse(&input)
		id = input.ID
		if rule.idField == "task_id" {
			id = input.TaskID
		}
		if rule.idField == "query_task_id" {
			id = r.GetQuery("task_id").Int64()
		}
		if err == nil && id <= 0 {
			err = errors.New("resource ID required")
		}
		if err == nil {
			err = p.Check(r.GetCtx(), rule.table, id, rule.permission)
		}
	}
	if err != nil {
		r.Response.WriteJson(Response{Code: 403, Msg: err.Error()})
		r.ExitAll()
		return true
	}
	if rule.operation != "" {
		err = taskControl(r.GetCtx(), p, &Request{ID: id, Data: map[string]interface{}{"operation": rule.operation}})
		if err == nil {
			err = governance.Audit(r.GetCtx(), p, "task_control:tasks", id, map[string]string{"operation": rule.operation})
		}
		if err != nil {
			r.Response.WriteJson(Response{Code: 400, Msg: err.Error()})
		} else {
			r.Response.WriteJson(Response{Success: true, Msg: "操作成功"})
		}
		r.ExitAll()
		return true
	}
	r.Middleware.Next()
	return true
}
