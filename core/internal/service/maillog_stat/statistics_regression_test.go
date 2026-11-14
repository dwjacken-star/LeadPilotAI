package maillog_stat

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"billionmail-core/internal/service/public"
	"billionmail-core/internal/testsupport"
	"github.com/gogf/gf/v2/frame/g"
)

// Requires the disposable PostgreSQL fixture; never points at production.
func TestOverviewClickReceiptNormalizesMessageIDAndPreservesHistoricalCoverage(t *testing.T) {
	testsupport.Database(t)
	ctx := context.Background()
	public.SetCache("postfix_deferred_queue_count", 4, 300)
	now := time.Now().UnixMilli()
	queries := []string{
		`INSERT INTO mailstat_senders(postfix_message_id,sender,log_time_millis) VALUES('demo-sent','sales@example.org',$1),('demo-bounced','sales@example.org',$1),('demo-untracked','sales@example.org',$1)`,
		`INSERT INTO mailstat_message_ids(postfix_message_id,message_id,log_time_millis) VALUES('demo-sent','message-demo',$1),('demo-bounced','message-bounced',$1),('demo-untracked','message-untracked',$1)`,
		`INSERT INTO mailstat_send_mails(postfix_message_id,recipient,status,dsn,mail_provider,log_time_millis) VALUES('demo-sent','buyer@example.org','sent','2.0.0','other',$1),('demo-bounced','invalid@example.org','bounced','5.1.1','other',$1),('demo-untracked','untracked@example.org','sent','2.0.0','other',$1)`,
		// No mutable template/task flag exists. Actual receipts prove coverage;
		// an early receipt has no queue id and includes legacy angle brackets.
		`INSERT INTO mailstat_clicked(postfix_message_id,message_id,recipient,campaign_id,log_time_millis) VALUES('','<message-demo>','buyer@example.org',0,$1),('demo-untracked','message-untracked','wrong-recipient@example.org',0,$1)`,
	}
	for _, query := range queries {
		if _, err := g.DB().Exec(ctx, query, now); err != nil {
			t.Fatal(err)
		}
	}
	result, err := NewOverview().OverviewContext(ctx, 0, "example.org", now/1000-60, now/1000+60)
	if err != nil {
		t.Fatal(err)
	}
	dashboard := result["dashboard"].(map[string]interface{})
	if dashboard["clicked"] != 1 || dashboard["click_tracked"] != 1 || dashboard["click_rate"] != 100.0 {
		t.Fatal("click evidence was lost or another recipient was counted", dashboard)
	}
	if dashboard["bounced"] != 1 || dashboard["bounce_rate"] != 33.33 || dashboard["delayed_queue"] != 4 {
		t.Fatal("nonzero bounce/queue metrics were lost", dashboard)
	}
}

func TestMaillogBatchPersistsFinalBounceAtSameTimestamp(t *testing.T) {
	testsupport.Database(t)
	ctx := context.Background()
	stamp := time.Now().UTC().Format("2006-01-02T15:04:05Z")
	log := stamp + " postfix/qmgr[1]: DEMOQUEUE: from=<sales@example.org>, size=123, nrcpt=1 (queue active)\n" +
		stamp + " postfix/smtp[2]: DEMOQUEUE: to=<buyer@example.org>, relay=none, delay=1, delays=0/0/1/0, dsn=4.0.0, status=deferred (temporary failure)\n" +
		stamp + " postfix/smtp[2]: DEMOQUEUE: to=<buyer@example.org>, relay=none, delay=2, delays=0/0/2/0, dsn=5.1.1, status=bounced (invalid recipient)\n"
	file := filepath.Join(t.TempDir(), "mail.log")
	if err := os.WriteFile(file, []byte(log), 0600); err != nil {
		t.Fatal(err)
	}
	if err := NewMaillogStat(file, 0, 0, false).AnalysisAndSaveToDatabase(ctx); err != nil {
		t.Fatal(err)
	}
	row, err := g.DB().Model("mailstat_send_mails").Where("postfix_message_id", "DEMOQUEUE").One()
	if err != nil || row.IsEmpty() || row["status"].String() != "bounced" || row["dsn"].String() != "5.1.1" {
		t.Fatal("batch kept the first deferred state instead of the final bounce", row, err)
	}
	// Replay is safe for final send state even when timestamps tie.
	if err := NewMaillogStat(file, 0, 0, false).AnalysisAndSaveToDatabase(ctx); err != nil {
		t.Fatal(err)
	}
	row, err = g.DB().Model("mailstat_send_mails").Where("postfix_message_id", "DEMOQUEUE").One()
	if err != nil || row["status"].String() != "bounced" {
		t.Fatal("replay regressed the terminal state", row, err)
	}
}
