package maillog_stat

import (
	"context"
	"encoding/json"
	"reflect"
	"strconv"
	"testing"
	"time"

	"billionmail-core/internal/service/public"
	"github.com/gogf/gf/v2/frame/g"
)

func exerciseOverviewSnapshotParityAndCampaignScope(t *testing.T) {
	ctx := context.Background()
	public.SetCache("postfix_deferred_queue_count", 0, 300)
	now := time.Now().Unix()
	exec := func(query string, args ...interface{}) {
		t.Helper()
		if _, e := g.DB().Exec(ctx, query, args...); e != nil {
			t.Fatal(e)
		}
	}
	exec(`INSERT INTO mailstat_senders(postfix_message_id,sender,log_time_millis) SELECT 'q-'||n,CASE WHEN n%2=0 THEN 'sender@example.com' ELSE 'other@else.test' END,$1::bigint-n*1000 FROM generate_series(1,3000)n`, now*1000)
	exec(`INSERT INTO mailstat_message_ids(postfix_message_id,message_id,log_time_millis) SELECT 'q-'||n,'mid-'||n,$1::bigint-n*1000 FROM generate_series(1,3000)n`, now*1000)
	exec(`INSERT INTO mailstat_send_mails(postfix_message_id,recipient,status,dsn,mail_provider,log_time_millis) SELECT 'q-'||n,'buyer'||n||'@client.test',CASE WHEN n%5=0 THEN 'bounced' ELSE 'sent' END,CASE WHEN n%5=0 THEN '5.1.1' ELSE '2.0.0' END,CASE WHEN n%3=0 THEN 'google' ELSE 'other' END,$1::bigint-n*1000 FROM generate_series(1,3000)n`, now*1000)
	exec(`INSERT INTO bm_webmail_tracking(token,message_id,recipient,event_type) SELECT lpad(n::text,64,'0'),'mid-'||n,'buyer'||n||'@client.test','open' FROM generate_series(1,3000)n WHERE n%5<>0`)
	exec(`INSERT INTO mailstat_opened(postfix_message_id,message_id,recipient,campaign_id,log_time_millis) SELECT CASE WHEN n%2=0 THEN '' ELSE 'q-'||n END,'mid-'||n,CASE WHEN n%7=0 THEN 'wrong@client.test' ELSE 'buyer'||n||'@client.test' END,0,$1 FROM generate_series(1,3000)n WHERE n%11=0`, now*1000)
	o := NewOverview()
	// Legacy PostgreSQL NUMERIC/chart fields serialize as strings. Compare the
	// measurements rather than that driver-specific JSON representation.
	var numbers func(interface{}) interface{}
	numbers = func(value interface{}) interface{} {
		switch typed := value.(type) {
		case string:
			if n, e := strconv.ParseFloat(typed, 64); e == nil {
				return n
			}
		case map[string]interface{}:
			for key, v := range typed {
				typed[key] = numbers(v)
			}
		case []interface{}:
			for i, v := range typed {
				typed[i] = numbers(v)
			}
		}
		return value
	}
	clean := func(v interface{}) interface{} {
		t.Helper()
		data, e := json.Marshal(v)
		if e != nil {
			t.Fatal(e)
		}
		var result interface{}
		if e = json.Unmarshal(data, &result); e != nil {
			t.Fatal(e)
		}
		return numbers(result)
	}
	for _, domain := range []string{"", "example.com", "no-records.test"} {
		for _, span := range []int64{3600, 86400 * 7, 86400 * 35} {
			start, end := now-span, now+1
			legacy := map[string]interface{}{"dashboard": o.overviewDashboard(0, domain, start, end), "mail_providers": o.overviewProviders(0, domain, start, end), "send_mail_chart": o.chartSendMail(0, domain, start, end), "bounce_rate_chart": o.chartBounceRate(0, domain, start, end), "open_rate_chart": o.chartOpenRate(0, domain, start, end), "click_rate_chart": o.chartClickRate(0, domain, start, end)}
			got, e := o.OverviewContext(ctx, 0, domain, start, end)
			if e != nil {
				t.Fatal(e)
			}
			for key, want := range legacy {
				if !reflect.DeepEqual(clean(want), clean(got[key])) {
					t.Fatalf("snapshot parity failed domain=%s span=%d panel=%s\nwant=%v\ngot=%v", domain, span, key, clean(want), clean(got[key]))
				}
			}
		}
	}
	// Real stored recipient Message-IDs include angle brackets; mail logs strip them.
	exec(`INSERT INTO email_tasks(id,task_name,addresser,subject,recipient_count,template_id,track_open,track_click) VALUES(77,'campaign','sender@example.com','campaign',1,1,1,0); INSERT INTO recipient_info(task_id,recipient,message_id) VALUES(77,'buyer2@client.test','<mid-2>')`)
	got, e := o.OverviewContext(ctx, 77, "example.com", now-3600, now+1)
	if e != nil {
		t.Fatal(e)
	}
	dashboard := got["dashboard"].(map[string]interface{})
	if dashboard["sends"] != 1 || dashboard["open_tracked"] != 1 {
		t.Fatal("campaign scope lost normalized Message-ID", dashboard)
	}
	exec(`UPDATE recipient_info SET recipient='another@client.test' WHERE task_id=77`)
	got, e = o.OverviewContext(ctx, 77, "example.com", now-3600, now+1)
	if e != nil {
		t.Fatal(e)
	}
	if got["dashboard"].(map[string]interface{})["sends"] != 0 {
		t.Fatal("campaign leaked another recipient's delivery")
	}
}
