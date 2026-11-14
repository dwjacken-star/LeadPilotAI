package crm

import (
	service "billionmail-core/internal/service/crm"
	"billionmail-core/internal/service/governance"
	"billionmail-core/internal/service/rbac"
	"billionmail-core/utility/types/api_v1"
	"context"
	"errors"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/gconv"
)

type Controller struct{}

func New() *Controller { return &Controller{} }

type Request struct {
	g.Meta  `path:"/crm/action" method:"post" tags:"CRM" summary:"Customer inquiries and replies"`
	Action  string                 `json:"action" v:"required"`
	ID      int64                  `json:"id"`
	Page    int                    `json:"page"`
	Keyword string                 `json:"keyword"`
	Stage   string                 `json:"stage"`
	Data    map[string]interface{} `json:"data"`
}
type Response struct{ api_v1.StandardRes }

func (c *Controller) Action(ctx context.Context, req *Request) (res *Response, err error) {
	res = &Response{}
	p, err := governance.Load(ctx, rbac.GetCurrentAccountId(ctx))
	if err != nil {
		res.SetError(err)
		return res, nil
	}
	if !p.Has("crm:list") {
		res.SetError(errors.New("CRM access denied"))
		return res, nil
	}
	switch req.Action {
	case "mailboxes":
		res.Data, err = service.Mailboxes(ctx, p)
	case "list":
		res.Data, err = service.List(ctx, p, req.Page, req.Keyword, req.Stage)
	case "detail":
		res.Data, err = service.Detail(ctx, p, req.ID)
	case "message":
		res.Data, err = service.Message(ctx, p, req.ID)
	case "save":
		res.Data, err = service.Save(ctx, p, req.ID, req.Data)
	case "delete":
		err = service.Delete(ctx, p, req.ID)
	case "sync":
		res.Data, err = service.Sync(ctx, p, gconv.String(req.Data["mailbox"]))
	case "followup":
		err = service.Followup(ctx, p, req.ID, gconv.String(req.Data["content"]))
	case "reply":
		res.Data, err = service.Reply(ctx, p, req.ID, gconv.String(req.Data["html"]), gconv.String(req.Data["request_key"]))
	default:
		err = errors.New("Unknown CRM action")
	}
	if err != nil {
		res.SetError(err)
	} else {
		res.SetSuccess("OK")
	}
	return res, nil
}
