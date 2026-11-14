package overview

import (
	"context"
	"errors"

	v1 "billionmail-core/api/overview/v1"
	"billionmail-core/internal/service/governance"
	"billionmail-core/internal/service/maillog_stat"
	"billionmail-core/internal/service/public"
	"github.com/gogf/gf/v2/frame/g"
)

func overviewPrincipal(ctx context.Context) (*governance.Principal, error) {
	request := g.RequestFromCtx(ctx)
	if request == nil {
		return nil, errors.New("authentication required")
	}
	p, err := governance.Load(ctx, request.GetCtxVar("accountId").Int64())
	if err != nil {
		return nil, err
	}
	if !p.Has(maillog_stat.OverviewPermission) {
		return nil, errors.New("无权查看")
	}
	return p, nil
}

func (c *ControllerV1) Domains(ctx context.Context, req *v1.DomainsReq) (res *v1.DomainsRes, err error) {
	res = &v1.DomainsRes{}
	p, err := overviewPrincipal(ctx)
	if err != nil {
		res.Code = 403
		res.SetError(err)
		return res, nil
	}
	res.Data, err = maillog_stat.NewOverview().DomainsForPrincipal(ctx, p)
	if err != nil {
		return nil, err
	}
	res.SetSuccess(public.LangCtx(ctx, "Success"))
	return res, nil
}
