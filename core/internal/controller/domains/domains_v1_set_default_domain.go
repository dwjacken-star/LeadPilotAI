package domains

import (
	"billionmail-core/internal/consts"
	"billionmail-core/internal/service/public"
	"billionmail-core/internal/service/smtpidentity"
	"context"
	"github.com/gogf/gf/v2/frame/g"

	"github.com/gogf/gf/v2/errors/gcode"
	"github.com/gogf/gf/v2/errors/gerror"

	"billionmail-core/api/domains/v1"
)

func (c *ControllerV1) SetDefaultDomain(ctx context.Context, req *v1.SetDefaultDomainReq) (res *v1.SetDefaultDomainRes, err error) {
	res = &v1.SetDefaultDomainRes{}

	// 1. Validate if the domain exists and is active
	var count int
	count, err = g.DB().Model("domain").
		Where("domain", req.Domain).
		Where("active", 1).
		Count()

	if err != nil {
		res.SetError(gerror.New(public.LangCtx(ctx, "check domain failed: {}", err.Error())))
		return res, nil
	}

	if count == 0 {
		res.SetError(gerror.NewCode(gcode.CodeNotFound, "Domain does not exist or is not activated"))
		return res, nil
	}

	// Validate/reload Postfix as part of changing the default sending identity.
	err = smtpidentity.SetDefault(ctx, req.Domain)

	if err != nil {
		res.SetError(gerror.New(public.LangCtx(ctx, "Failed to set default sender domain: {}", err.Error())))
		return
	}

	_ = public.WriteLog(ctx, public.LogParams{
		Type: consts.LOGTYPE.Domain,
		Log:  "Set default sender domain :" + req.Domain + " successfully",
	})

	res.SetSuccess(public.LangCtx(ctx, "Default sender domain set successfully"))
	return
}
