package enterprise

import (
	"billionmail-core/internal/service/email_template"
	"billionmail-core/internal/service/governance"
	"errors"
	"github.com/gogf/gf/v2/net/ghttp"
)

// Only the authoring subset is delegated. Provider secrets, global settings,
// domain mutations, chat enumeration/removal and arbitrary file paths stay closed.
func templateAIAccess(r *ghttp.Request) bool {
	permissions := map[string]string{
		"/api/askai/supplier/models":  "template:list",
		"/api/askai/chat/create_chat": "template:create",
		"/api/askai/chat/info":        "template:list",
		"/api/askai/chat/get_html":    "template:list",
		"/api/askai/chat/chat":        "template:update",
		"/api/askai/chat/modify_html": "template:update",
		"/api/askai/chat/stop":        "template:update",
	}
	permission, ok := permissions[r.URL.Path]
	if !ok {
		return false
	}
	p, e := governance.Load(r.GetCtx(), r.GetCtxVar("accountId").Int64())
	if e == nil && p.Super {
		r.Middleware.Next()
		return true
	}
	if e == nil && (r.Method != "POST" || !p.Has(permission)) {
		e = errors.New("permission required: " + permission)
	}
	var input struct {
		ChatID  string `json:"chatId"`
		Domain  string `json:"domain"`
		AddType int    `json:"add_type"`
	}
	if e == nil {
		e = r.Parse(&input)
	}
	if e == nil {
		switch r.URL.Path {
		case "/api/askai/supplier/models":
		case "/api/askai/chat/create_chat":
			if !p.Has("template:update") || input.AddType != 99 {
				e = errors.New("template create and update permissions required")
			} else if e = p.CheckOwnership(r.GetCtx(), "template:create", p.DeptID, p.ID); e == nil {
				e = email_template.ValidateDomain(r.GetCtx(), input.Domain)
			}
		default:
			e = email_template.CheckChat(r.GetCtx(), input.ChatID, permission)
		}
	}
	if e != nil {
		r.Response.WriteJson(Response{Code: 403, Msg: e.Error()})
		r.ExitAll()
		return true
	}
	r.Middleware.Next()
	return true
}
