package batch_mail

import (
	"billionmail-core/internal/service/public"
	"billionmail-core/internal/service/warmup"
	"context"
	"fmt"
	"github.com/gogf/gf/v2/frame/g"
)

// Warmup is a relation, not a column on email_tasks. Use the transaction context
// so task/recipient creation is rolled back if this association cannot be saved.
func SetTaskWarmup(ctx context.Context, taskID int64, enabled int) error {
	if enabled != 0 && enabled != 1 {
		return fmt.Errorf("Warmup must be 0 or 1")
	}
	if enabled == 0 {
		_, err := g.DB().Model("bm_campaign_warmup").Ctx(ctx).Where("task_id", taskID).Delete()
		return err
	}
	ip, err := public.GetServerIP()
	if err != nil {
		return err
	}
	if ip == "" {
		return fmt.Errorf("Cannot determine sender IP for warmup")
	}
	_, err = warmup.WarmupCampaign().AssociateCampaignWithWarmup(ctx, taskID, ip)
	return err
}
func TaskWarmupEnabled(ctx context.Context, taskID int64) (int, error) {
	count, err := g.DB().Model("bm_campaign_warmup").Ctx(ctx).Where("task_id", taskID).Count()
	if err != nil {
		return 0, err
	}
	if count > 0 {
		return 1, nil
	}
	return 0, nil
}
