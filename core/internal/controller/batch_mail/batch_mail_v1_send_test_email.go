package batch_mail

import (
	v1 "billionmail-core/api/batch_mail/v1"
	"billionmail-core/internal/service/batch_mail"
	"billionmail-core/internal/service/governance"
	"billionmail-core/internal/service/rbac"
	"context"
)

func (c *ControllerV1) SendTestEmail(ctx context.Context, req *v1.SendTestEmailReq) (res *v1.SendTestEmailRes, err error) {
	res = &v1.SendTestEmailRes{}
	p, e := governance.Load(ctx, rbac.GetCurrentAccountId(ctx))
	if e == nil {
		e = batch_mail.SendPreview(ctx, p, req)
	}
	if e != nil {
		res.SetError(e)
	} else {
		res.SetSuccess("Test email sent")
	}
	return res, nil
}
