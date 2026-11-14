package maillog_stat

import "fmt"

// A queue id can cover several recipients. Attribute an event only to the
// corresponding delivery and fall back to Message-ID for early/late log arrival.
func eventJoin(table, alias string) string {
	return fmt.Sprintf(`LATERAL(SELECT ev.id FROM %s ev
 WHERE lower(ev.recipient)=lower(sm.recipient) AND
 (ev.postfix_message_id=sm.postfix_message_id OR EXISTS(
 SELECT 1 FROM mailstat_message_ids mid WHERE mid.postfix_message_id=sm.postfix_message_id AND mid.message_id=ev.message_id)) LIMIT 1) AS %s`, table, alias)
}

func trackingCoverage(kind string) string {
	flag := "track_open"
	if kind == "click" {
		flag = "track_click"
	}
	return fmt.Sprintf(`EXISTS(SELECT 1 FROM mailstat_message_ids mid WHERE mid.postfix_message_id=sm.postfix_message_id AND (
 EXISTS(SELECT 1 FROM bm_webmail_tracking w WHERE w.message_id=mid.message_id AND lower(w.recipient)=lower(sm.recipient) AND w.event_type='%s') OR
 EXISTS(SELECT 1 FROM recipient_info ri JOIN email_tasks t ON t.id=ri.task_id WHERE btrim(ri.message_id,'<>')=mid.message_id AND lower(ri.recipient)=lower(sm.recipient) AND t.%s=1) OR
 EXISTS(SELECT 1 FROM api_mail_logs al JOIN api_templates ap ON ap.id=al.api_id WHERE btrim(al.message_id,'<>')=mid.message_id AND lower(al.recipient)=lower(sm.recipient) AND ap.%s=1)))`, kind, flag, flag)
}

func trackedCount(kind string) string {
	return `COALESCE(sum(CASE WHEN sm.status='sent' AND sm.dsn LIKE '2.%' AND ` + trackingCoverage(kind) + ` THEN 1 ELSE 0 END),0)`
}
