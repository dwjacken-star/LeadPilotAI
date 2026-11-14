package batch_mail

import (
	"billionmail-core/internal/service/governance"
	"context"
	"errors"
)

var MasterDeliveryGuard func(context.Context, *governance.Principal, int64) error

func checkMasterDelivery(ctx context.Context, p *governance.Principal, id int64) error {
	if id == 0 {
		return nil
	}
	if MasterDeliveryGuard == nil {
		return errors.New("Growth Master delivery guard unavailable")
	}
	return MasterDeliveryGuard(ctx, p, id)
}
