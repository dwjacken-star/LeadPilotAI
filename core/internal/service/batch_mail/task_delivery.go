package batch_mail

import (
	"context"
	"github.com/gogf/gf/v2/frame/g"
)

type TaskDeliveryOptions struct {
	Warmup      bool
	Unsubscribe bool
	TrackOpen   bool
	TrackClick  bool
}

// SetTaskDeliveryOptions joins the caller's transaction so a new task cannot be
// claimed until all flags and its IP warmup association have been persisted.
func SetTaskDeliveryOptions(ctx context.Context, taskID int64, options TaskDeliveryOptions) error {
	_, err := g.DB().Model("email_tasks").Ctx(ctx).Where("id", taskID).Data(g.Map{"unsubscribe": boolInt(options.Unsubscribe), "track_open": boolInt(options.TrackOpen), "track_click": boolInt(options.TrackClick)}).Update()
	if err != nil {
		return err
	}
	return SetTaskWarmup(ctx, taskID, boolInt(options.Warmup))
}
