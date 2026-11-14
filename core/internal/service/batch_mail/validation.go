package batch_mail

import (
	"context"
	"github.com/gogf/gf/v2/frame/g"
)

// Releases only historical holds after an explicit operator confirmation.
// Ambiguous DATA outcomes remain fenced; no recipient probing occurs here.
func RecheckTaskRecipients(ctx context.Context, id int64) error {
	_, err := g.DB().Exec(ctx, `UPDATE recipient_info SET delivery_state='retry',next_attempt_at=now(),last_error='',verification_status='',verification_code='' WHERE task_id=$1 AND delivery_state='validation_hold'`, id)
	if err == nil {
		_, err = g.DB().Exec(ctx, `UPDATE email_tasks SET next_attempt_at=now(),wait_reason='' WHERE id=$1`, id)
	}
	return err
}
