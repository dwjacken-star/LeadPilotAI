package maillog_stat

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"billionmail-core/internal/service/governance"
	"billionmail-core/internal/service/public"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// eventExists splits the queue and Message-ID alternatives into indexable probes.
// Recipient matching is essential: a queue can contain several recipients.
func eventExists(table string) string {
	return fmt.Sprintf(`(EXISTS(SELECT 1 FROM %[1]s ev WHERE ev.postfix_message_id=sm.postfix_message_id AND lower(ev.recipient)=lower(sm.recipient)) OR
 EXISTS(SELECT 1 FROM mailstat_message_ids mid JOIN %[1]s ev ON ev.message_id IN(mid.message_id,btrim(mid.message_id,'<>'),'<'||btrim(mid.message_id,'<>')||'>')
 WHERE mid.postfix_message_id=sm.postfix_message_id AND btrim(mid.message_id,'<>')<>'' AND lower(ev.recipient)=lower(sm.recipient)))`, table)
}

func (o *Overview) snapshotQuery(campaignID int64, domain string, startTime, endTime int64) (string, []interface{}) {
	return o.snapshotQueryScoped(campaignID, domain, startTime, endTime, "")
}

func (o *Overview) snapshotQueryScoped(campaignID int64, domain string, startTime, endTime int64, scope string) (string, []interface{}) {
	_, axis := o.prepareChartData(startTime, endTime)
	axis = strings.TrimSuffix(axis, " as x")
	where := []string{"EXISTS(SELECT 1 FROM mailstat_senders s WHERE s.postfix_message_id=sm.postfix_message_id)"}
	args := []interface{}{}
	if scope != "" {
		where = append(where, scope)
	}
	if startTime > 0 {
		where = append(where, "sm.log_time_millis>?")
		args = append(args, startTime*1000)
	}
	if endTime > 0 {
		where = append(where, "sm.log_time_millis<?")
		args = append(args, endTime*1000)
	}
	if domain != "" {
		if scope != "" {
			where = append(where, "EXISTS(SELECT 1 FROM mailstat_senders s WHERE s.postfix_message_id=sm.postfix_message_id AND lower(split_part(s.sender,'@',2))=lower(?))")
			args = append(args, domain)
		} else {
			where = append(where, "EXISTS(SELECT 1 FROM mailstat_senders s WHERE s.postfix_message_id=sm.postfix_message_id AND s.sender LIKE ?)")
			args = append(args, "%@"+domain)
		}
	}
	if campaignID > 0 {
		where = append(where, `EXISTS(SELECT 1 FROM mailstat_message_ids mid JOIN recipient_info ri ON btrim(ri.message_id,'<>')=mid.message_id
 WHERE mid.postfix_message_id=sm.postfix_message_id AND ri.task_id=? AND lower(ri.recipient)=lower(sm.recipient))`)
		args = append(args, campaignID)
	}
	query := fmt.Sprintf(`WITH metrics AS MATERIALIZED (
 SELECT sm.mail_provider,%s AS bucket, sm.status='sent' AND sm.dsn LIKE '2.%%' AS delivered,
 sm.status='bounced' AS bounced,%s AS opened,%s AS clicked,
 sm.status='sent' AND sm.dsn LIKE '2.%%' AND %s AS open_tracked,
 sm.status='sent' AND sm.dsn LIKE '2.%%' AND %s AS click_tracked
 FROM mailstat_send_mails sm WHERE %s)
 SELECT grouping(mail_provider) AS gp,grouping(bucket) AS gb,mail_provider,bucket,
 count(*) AS sends,count(*) FILTER(WHERE delivered) AS delivered,count(*) FILTER(WHERE bounced) AS bounced,
 count(*) FILTER(WHERE opened) AS opened,count(*) FILTER(WHERE clicked) AS clicked,
 count(*) FILTER(WHERE open_tracked) AS open_tracked,count(*) FILTER(WHERE click_tracked) AS click_tracked
 FROM metrics GROUP BY GROUPING SETS((),(mail_provider),(bucket))`, axis, eventExists("mailstat_opened"), eventExists("mailstat_clicked"), trackingCoverage("open"), trackingCoverage("click"), strings.Join(where, " AND "))
	return query, args
}

func snapshotCounts(row gdb.Record) map[string]interface{} {
	m := map[string]interface{}{}
	for _, key := range []string{"sends", "delivered", "bounced", "opened", "clicked", "open_tracked", "click_tracked"} {
		m[key] = 0
		if row[key] != nil {
			m[key] = row[key].Int()
		}
	}
	return m
}
func snapshotRates(m map[string]interface{}) {
	percent := func(n, d int) float64 {
		if d == 0 {
			return 0
		}
		return public.Round(float64(n)*100/float64(d), 2)
	}
	sends := m["sends"].(int)
	m["delivery_rate"] = percent(m["delivered"].(int), sends)
	m["bounce_rate"] = percent(m["bounced"].(int), sends)
	for _, kind := range []string{"open", "click"} {
		numerator := "opened"
		if kind == "click" {
			numerator = "clicked"
		}
		denominator := m[kind+"_tracked"].(int)
		if denominator == 0 {
			m[kind+"_rate"] = -1.0
		} else {
			m[kind+"_rate"] = percent(m[numerator].(int), denominator)
		}
	}
}

// OverviewContext computes every panel from one database snapshot, with no stale
// result cache. A successful empty result is distinct from a failed query.
func (o *Overview) OverviewContext(ctx context.Context, campaignID int64, domain string, startTime, endTime int64) (map[string]interface{}, error) {
	queueAccess := true
	if campaignID > 0 {
		// Task authorization belongs to the campaign caller, not overview:view.
		// Its ordinary-user response must still conceal the global queue.
		p, err := governance.RequestPrincipal(ctx)
		if err != nil {
			return nil, err
		}
		queueAccess = p == nil || p.Super
	}
	return o.overviewContext(ctx, campaignID, domain, startTime, endTime, "", queueAccess)
}

func (o *Overview) overviewContext(ctx context.Context, campaignID int64, domain string, startTime, endTime int64, scope string, queueAccess bool) (map[string]interface{}, error) {
	startTime, endTime = o.filterAndPrepareTimeSection(startTime, endTime)
	query, args := o.snapshotQueryScoped(campaignID, domain, startTime, endTime, scope)
	var rows gdb.Result
	err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		// These index-probe aggregates can receive very high planner estimates.
		// JIT compilation then dominates a short dashboard read. Scope the
		// setting to this transaction; do not alter the server/global setting.
		previous, e := tx.GetValue("SHOW jit")
		if e != nil {
			return e
		}
		if _, e = tx.Exec("SET LOCAL jit=off"); e != nil {
			return e
		}
		rows, e = tx.GetAll(query, args...)
		if e != nil {
			return e
		}
		// Also preserve an enclosing caller transaction's setting.
		if previous.String() == "on" {
			_, e = tx.Exec("SET LOCAL jit=on")
		}
		return e
	})
	if err != nil {
		return nil, err
	}
	dashboard := snapshotCounts(gdb.Record{})
	snapshotRates(dashboard)
	providers := []map[string]interface{}{}
	sendData, bounceData, openData, clickData := []map[string]interface{}{}, []map[string]interface{}{}, []map[string]interface{}{}, []map[string]interface{}{}
	for _, row := range rows {
		counts := snapshotCounts(row)
		snapshotRates(counts)
		switch {
		case row["gp"].Int() == 1 && row["gb"].Int() == 1:
			dashboard = counts
		case row["gp"].Int() == 0:
			counts["mail_provider"] = row["mail_provider"].String()
			providers = append(providers, counts)
		default:
			x := row["bucket"].Val()
			sends, delivered := counts["sends"].(int), counts["delivered"].(int)
			sendData = append(sendData, map[string]interface{}{"x": x, "sends": sends, "delivered": delivered, "failed": sends - delivered})
			bounceData = append(bounceData, map[string]interface{}{"x": x, "bounce_rate": counts["bounce_rate"]})
			var opened, clicked interface{}
			if counts["open_tracked"].(int) > 0 {
				opened = counts["open_rate"]
			}
			if counts["click_tracked"].(int) > 0 {
				clicked = counts["click_rate"]
			}
			openData = append(openData, map[string]interface{}{"x": x, "open_rate": opened})
			clickData = append(clickData, map[string]interface{}{"x": x, "click_rate": clicked})
		}
	}
	sort.Slice(providers, func(i, j int) bool {
		a, b := providers[i]["sends"].(int), providers[j]["sends"].(int)
		if a == b {
			return providers[i]["mail_provider"].(string) < providers[j]["mail_provider"].(string)
		}
		return a > b
	})
	dashboard["delayed_queue"] = 0
	if queueAccess {
		// Unknown Docker/postqueue results must never masquerade as an empty queue.
		dashboard["delayed_queue"] = -1
		if count, e := o.getPostfixDeferredQueueCount(ctx); e == nil {
			dashboard["delayed_queue"] = count
		}
	}
	sends, delivered := dashboard["sends"].(int), dashboard["delivered"].(int)
	sendDashboard := map[string]interface{}{"sends": sends, "delivered": delivered, "failed": sends - delivered, "delivery_rate": dashboard["delivery_rate"], "failure_rate": 0.0}
	if sends > 0 {
		sendDashboard["failure_rate"] = public.Round(float64(sends-delivered)*100/float64(sends), 2)
	}
	column, _ := o.prepareChartData(startTime, endTime)
	chart := func(data []map[string]interface{}, fill map[string]interface{}) map[string]interface{} {
		return map[string]interface{}{"column_type": column, "data": o.fillChartData(data, fill, column, "x", startTime, endTime)}
	}
	sendChart := chart(sendData, map[string]interface{}{"sends": 0, "delivered": 0, "failed": 0})
	sendChart["dashboard"] = sendDashboard
	return map[string]interface{}{"dashboard": dashboard, "mail_providers": providers, "send_mail_chart": sendChart,
		"bounce_rate_chart": chart(bounceData, map[string]interface{}{"bounce_rate": 0.0}),
		"open_rate_chart":   chart(openData, map[string]interface{}{"open_rate": 0.0}), "click_rate_chart": chart(clickData, map[string]interface{}{"click_rate": 0.0})}, nil
}
