package social

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// Meta uses these errors for an inaccessible object as well as missing access.
// They do not by themselves establish that the entire Page token is invalid.
func facebookObjectUnavailable(err error) bool {
	var remote *RemoteError
	return errors.As(err, &remote) && (remote.Code == 10 || (remote.Code == 100 && remote.Subcode == 33))
}

func recordPostSyncFailure(ctx context.Context, accountID, deliveryID int64, cause error) error {
	var remote *RemoteError
	if !errors.As(cause, &remote) {
		return cause
	}
	reason := fmt.Sprintf("FACEBOOK_POST_UNAVAILABLE:%d/%d", remote.Code, remote.Subcode)
	return g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if _, e := tx.Model("bm_social_deliveries").Where("id", deliveryID).Data(g.Map{"sync_state": "unavailable", "sync_error": reason, "last_sync_at": time.Now()}).Update(); e != nil {
			return e
		}
		availability := g.Map{"sync": "object_unavailable"}
		for _, key := range []string{"reactions", "comments", "shares", "views", "impressions", "reach", "clicks"} {
			availability[key] = "object_unavailable"
		}
		prior, e := tx.Model("bm_social_metrics").Where("delivery_id", deliveryID).One()
		if e != nil {
			return e
		}
		if len(prior) > 0 {
			// Retain the previous counts and their observation time for history,
			// but never present them as fresh, currently available measurements.
			_, e = tx.Model("bm_social_metrics").Where("delivery_id", deliveryID).Data("availability", encode(availability)).Update()
			return e
		}
		_, e = tx.Model("bm_social_metrics").Data(g.Map{"delivery_id": deliveryID, "account_id": accountID, "counts": "{}", "availability": encode(availability), "as_of": time.Now(), "expires_at": time.Now().AddDate(1, 0, 0)}).OnConflict("delivery_id").InsertIgnore()
		return e
	})
}

func recordPostSyncSuccess(ctx context.Context, deliveryID int64) error {
	_, e := g.DB().Model("bm_social_deliveries").Ctx(ctx).Where("id", deliveryID).Data(g.Map{"sync_state": "synced", "sync_error": "", "last_sync_at": time.Now()}).Update()
	return e
}
