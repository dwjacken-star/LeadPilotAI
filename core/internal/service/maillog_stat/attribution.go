package maillog_stat

import "fmt"

// A queue id can cover several recipients. Attribute an event only to the
// corresponding delivery and fall back to Message-ID for early/late log arrival.
func eventJoin(table, alias string) string {
	return fmt.Sprintf(`LATERAL(SELECT ev.id FROM %s ev
 WHERE lower(ev.recipient)=lower(sm.recipient) AND
 (ev.postfix_message_id=sm.postfix_message_id OR EXISTS(
 SELECT 1 FROM mailstat_message_ids mid WHERE mid.postfix_message_id=sm.postfix_message_id AND btrim(mid.message_id,'<>')<>'' AND ev.message_id IN(mid.message_id,btrim(mid.message_id,'<>'),'<'||btrim(mid.message_id,'<>')||'>'))) LIMIT 1) AS %s`, table, alias)
}

func trackingCoverage(kind string) string {
	flag := "track_open"
	if kind == "click" {
		flag = "track_click"
	}
	configured := fmt.Sprintf(`EXISTS(SELECT 1 FROM mailstat_message_ids mid WHERE mid.postfix_message_id=sm.postfix_message_id AND btrim(mid.message_id,'<>')<>'' AND (
 EXISTS(SELECT 1 FROM bm_webmail_tracking w WHERE w.message_id IN(mid.message_id,btrim(mid.message_id,'<>'),'<'||btrim(mid.message_id,'<>')||'>') AND lower(w.recipient)=lower(sm.recipient) AND w.event_type='%s') OR
 EXISTS(SELECT 1 FROM recipient_info ri JOIN email_tasks t ON t.id=ri.task_id WHERE ri.message_id IN(mid.message_id,btrim(mid.message_id,'<>'),'<'||btrim(mid.message_id,'<>')||'>') AND lower(ri.recipient)=lower(sm.recipient) AND t.%s=1) OR
 EXISTS(SELECT 1 FROM api_mail_logs al JOIN api_templates ap ON ap.id=al.api_id WHERE al.message_id IN(mid.message_id,btrim(mid.message_id,'<>'),'<'||btrim(mid.message_id,'<>')||'>') AND lower(al.recipient)=lower(sm.recipient) AND ap.%s=1)))`, kind, flag, flag)
	table := "mailstat_opened"
	if kind == "click" {
		table = "mailstat_clicked"
	}
	// A recorded receipt proves this delivery was instrumented, even if a
	// mutable task/template flag has since been disabled or its row removed.
	return "(" + configured + " OR " + eventExists(table) + ")"
}

func trackedCount(kind string) string {
	return `COALESCE(sum(CASE WHEN sm.status='sent' AND sm.dsn LIKE '2.%' AND ` + trackingCoverage(kind) + ` THEN 1 ELSE 0 END),0)`
}
