package maillog_stat

import (
	"context"
	"fmt"
	"github.com/gogf/gf/v2/net/ghttp"
	"image/png"
	"net/http"
	"strings"
	"testing"
	"time"

	"billionmail-core/internal/service/public"
	"billionmail-core/internal/testsupport"
	"github.com/gogf/gf/v2/frame/g"
)

func TestOpenAttributionAndCoverage(t *testing.T) {
	testsupport.Database(t)
	ctx := context.Background()
	public.SetCache("postfix_deferred_queue_count", 0, 300)
	now := time.Now().UnixMilli()
	for _, q := range []string{"queue-a", "queue-b", "queue-c"} {
		_, err := g.DB().Exec(ctx, `INSERT INTO mailstat_message_ids(postfix_message_id,message_id,log_time_millis) VALUES($1,$2,$3)`, q, "message-"+q, now)
		if err != nil {
			t.Fatal(err)
		}
		_, err = g.DB().Exec(ctx, `INSERT INTO mailstat_senders(postfix_message_id,sender,log_time_millis) VALUES($1,'sender@example.com',$2)`, q, now)
		if err != nil {
			t.Fatal(err)
		}
	}
	_, err := g.DB().Exec(ctx, `INSERT INTO mailstat_send_mails(postfix_message_id,recipient,mail_provider,status,dsn,log_time_millis) VALUES('queue-a','a@example.com','google','sent','2.0.0',$1),('queue-b','b@example.com','google','sent','2.0.0',$1),('queue-c','c@example.com','other','sent','2.0.0',$1)`, now)
	if err != nil {
		t.Fatal(err)
	}
	tokenA, tokenB := strings.Repeat("a", 64), strings.Repeat("b", 64)
	_, err = g.DB().Exec(ctx, `INSERT INTO bm_webmail_tracking(token,message_id,recipient,event_type) VALUES($1,'message-queue-a','a@example.com','open'),($2,'message-queue-b','b@example.com','open')`, tokenA, tokenB)
	if err != nil {
		t.Fatal(err)
	}
	// Record before Postfix's Message-ID mapping exists; report joins must recover it later.
	_, err = g.DB().Exec(ctx, `DELETE FROM mailstat_message_ids WHERE postfix_message_id='queue-a'`)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if _, err := recordWebmailEvent(ctx, tokenA); err != nil {
			t.Fatal(err)
		}
	}
	if n, e := g.DB().Model("mailstat_opened").Count(); e != nil || n != 1 {
		t.Fatalf("duplicate image requests inflated opens: %d %v", n, e)
	}
	_, err = g.DB().Exec(ctx, `INSERT INTO mailstat_message_ids(postfix_message_id,message_id,log_time_millis) VALUES('queue-a','message-queue-a',$1)`, now)
	if err != nil {
		t.Fatal(err)
	}
	// An event for another recipient on the same queue must not count for b.
	_, err = g.DB().Exec(ctx, `INSERT INTO mailstat_opened(message_id,postfix_message_id,recipient,campaign_id,log_time_millis) VALUES('message-queue-b','queue-b','other@example.com',0,$1)`, now)
	if err != nil {
		t.Fatal(err)
	}
	o := NewOverview()
	stats := o.overviewDashboard(0, "", 0, 0)
	if stats["opened"] != 1 || stats["open_tracked"] != 2 || stats["open_rate"] != 50.0 {
		t.Fatalf("incorrect attribution or measured denominator: %+v", stats)
	}
	providers := o.overviewProviders(0, "", 0, 0)
	for _, provider := range providers {
		if provider["mail_provider"] == "google" && provider["open_rate"] != 50.0 {
			t.Fatal(provider)
		}
		if provider["mail_provider"] == "other" && provider["open_rate"] != -1.0 {
			t.Fatal("untracked historical mail presented as measured zero", provider)
		}
	}
	if _, err = recordWebmailEvent(ctx, strings.Repeat("c", 64)); err != nil {
		t.Fatal(err)
	}
	if n, e := g.DB().Model("mailstat_opened").Count(); e != nil || n != 2 {
		t.Fatal("unknown token recorded an event")
	}
	for _, chart := range []map[string]interface{}{o.chartOpenRate(0, "", time.Now().Add(-time.Hour).Unix(), time.Now().Add(time.Hour).Unix()), o.chartClickRate(0, "", time.Now().Add(-time.Hour).Unix(), time.Now().Add(time.Hour).Unix())} {
		if chart == nil || chart["data"] == nil {
			t.Fatal("tracking chart query failed")
		}
	}
	server := g.Server("tracking-public-test")
	server.SetAddr("127.0.0.1:0")
	server.SetDumpRouterMap(false)
	server.BindHandler("/pmta/*any", func(r *ghttp.Request) { CampaignEventHandler(r, r.Get("any").String()) })
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	defer server.Shutdown()
	base := fmt.Sprintf("http://127.0.0.1:%d/pmta/w/", server.GetListenedPort())
	response, err := http.Get(base + tokenA)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = png.Decode(response.Body); err != nil {
		t.Fatal("public endpoint did not return a tracking image", err)
	}
	response.Body.Close()
	response, err = http.Head(base + tokenB)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if n, _ := g.DB().Model("mailstat_opened").Count(); n != 2 {
		t.Fatal("HEAD probe counted as a read")
	}
	clickToken := strings.Repeat("d", 64)
	_, err = g.DB().Exec(ctx, `INSERT INTO bm_webmail_tracking(token,message_id,recipient,event_type,target_url) VALUES($1,'message-queue-b','b@example.com','click',$2)`, clickToken, "https://example.com/catalog?a=1&b=2")
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }}
	for i := 0; i < 2; i++ {
		r, e := client.Get(base + clickToken)
		if e != nil {
			t.Fatal(e)
		}
		r.Body.Close()
		if r.StatusCode != 302 || r.Header.Get("Location") != "https://example.com/catalog?a=1&b=2" {
			t.Fatalf("tracked link redirect failed: status=%d location=%q", r.StatusCode, r.Header.Get("Location"))
		}
	}
	if n, _ := g.DB().Model("mailstat_clicked").Count(); n != 1 {
		t.Fatal("repeated clicks inflated unique clicks")
	}
	t.Run("snapshot parity, normalized campaign scope and 3000 deliveries", exerciseOverviewSnapshotParityAndCampaignScope)
}
