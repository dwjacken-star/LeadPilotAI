package overview

import (
	"billionmail-core/internal/service/maillog_stat"
	"billionmail-core/internal/service/public"
	"context"
	"fmt"
	"time"

	"github.com/gogf/gf/v2/util/gconv"

	"billionmail-core/api/overview/v1"
)

func (c *ControllerV1) Overview(ctx context.Context, req *v1.OverviewReq) (res *v1.OverviewRes, err error) {
	res = &v1.OverviewRes{}
	p, err := overviewPrincipal(ctx)
	if err != nil {
		res.Code = 403
		res.SetError(err)
		return res, nil
	}

	if req.EndTime == 0 {
		req.EndTime = time.Now().Unix()
	}

	if req.EndTime < req.StartTime {
		err = fmt.Errorf("start time must be greater or equal to end time")
		return
	}

	overview := maillog_stat.NewOverview()
	overviewMap, queryErr := overview.OverviewForPrincipal(ctx, p, req.CampaignId, req.Domain, req.StartTime, req.EndTime)
	if queryErr != nil {
		return nil, queryErr
	}

	err = gconv.Struct(overviewMap, &res.Data)

	if err != nil {
		err = fmt.Errorf("failed to convert overview data: %v", err)
		return
	}

	res.SetSuccess(public.LangCtx(ctx, "Success"))

	return
}
