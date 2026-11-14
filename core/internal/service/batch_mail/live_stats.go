package batch_mail

import (
	"context"
	"encoding/json"
	"github.com/gogf/gf/v2/frame/g"
)

type LiveTaskStats struct {
	Total     int `json:"total"`
	Submitted int `json:"submitted"`
	Failed    int `json:"failed"`
	Skipped   int `json:"skipped"`
	Waiting   int `json:"waiting"`
	Unknown   int `json:"unknown"`
	Held      int `json:"held"`
	Delivered int `json:"delivered"`
	Deferred  int `json:"deferred"`
	Processed int `json:"processed"`
	Progress  int `json:"progress"`
}

// One row per snapshotted recipient. Retry attempts and CC log entries never
// inflate totals. SMTP submission and downstream delivery are separate facts.
func GetLiveTaskStats(ctx context.Context, ids []int) (map[int]LiveTaskStats, error) {
	result := map[int]LiveTaskStats{}
	if len(ids) == 0 {
		return result, nil
	}
	rows, err := g.DB().Model("recipient_info r").Ctx(ctx).WhereIn("r.task_id", ids).Fields(`r.task_id,count(*) total,
 count(*) FILTER(WHERE r.delivery_state='accepted' OR (r.delivery_state='pending' AND r.is_sent=1)) submitted,
 count(*) FILTER(WHERE r.delivery_state='failed' OR (r.delivery_state='pending' AND r.is_sent=3)) failed,
 count(*) FILTER(WHERE r.delivery_state='skipped') skipped,
 count(*) FILTER(WHERE r.delivery_state='unknown') unknown,
 count(*) FILTER(WHERE r.delivery_state='validation_hold') held,
 count(*) FILTER(WHERE r.delivery_state IN('pending','retry','claimed','sending','validation_hold') AND r.is_sent<>1 AND r.is_sent<>3) waiting`).Group("r.task_id").All()
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		s := LiveTaskStats{Total: row["total"].Int(), Submitted: row["submitted"].Int(), Failed: row["failed"].Int(), Skipped: row["skipped"].Int(), Unknown: row["unknown"].Int(), Held: row["held"].Int(), Waiting: row["waiting"].Int()}
		s.Processed = s.Submitted + s.Failed + s.Skipped
		if s.Total > 0 {
			s.Progress = s.Processed * 100 / s.Total
		}
		result[row["task_id"].Int()] = s
	}
	// Mail logs can arrive after submission. Match both message and recipient so a
	// CC copy or an older retry cannot be counted as delivery for the main address.
	encoded, _ := json.Marshal(ids)
	logs, err := g.DB().GetAll(ctx, `SELECT task_id,count(*) FILTER(WHERE remote_status='sent') delivered,
 count(*) FILTER(WHERE remote_status='bounced' AND (delivery_state='accepted' OR(delivery_state='pending' AND is_sent=1))) bounced,
 count(*) FILTER(WHERE remote_status='deferred') deferred FROM (
 SELECT r.task_id,r.delivery_state,r.is_sent,(SELECT m.status FROM mailstat_message_ids mid JOIN mailstat_send_mails m ON m.postfix_message_id=mid.postfix_message_id
 WHERE mid.message_id IN(r.message_id,btrim(r.message_id,'<>'),'<'||btrim(r.message_id,'<>')||'>') AND lower(trim(m.recipient))=lower(trim(r.recipient)) ORDER BY m.log_time_millis DESC LIMIT 1) remote_status
 FROM recipient_info r WHERE task_id IN(SELECT value::int FROM jsonb_array_elements_text($1::jsonb))) facts GROUP BY task_id`, string(encoded))
	if err != nil {
		return nil, err
	}
	for _, row := range logs {
		id := row["task_id"].Int()
		s := result[id]
		bounced := row["bounced"].Int()
		s.Delivered = row["delivered"].Int()
		s.Deferred = row["deferred"].Int()
		// Downstream bounces replace a submitted success, without advancing progress twice.
		s.Submitted -= bounced
		if s.Submitted < 0 {
			s.Submitted = 0
		}
		s.Failed += bounced
		result[id] = s
	}
	return result, nil
}
