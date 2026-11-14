package webmaildelivery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	mailapi "billionmail-core/api/batch_mail/v1"
	batchcontroller "billionmail-core/internal/controller/batch_mail"
	subscribecontroller "billionmail-core/internal/controller/subscribe_list"
	"billionmail-core/internal/service/batch_mail"
	"billionmail-core/internal/service/deliveryprefs"
	"billionmail-core/internal/service/governance"
	"billionmail-core/internal/service/mail_service"
	"billionmail-core/internal/service/maillog_stat"
	"billionmail-core/internal/service/middlewares"
	"billionmail-core/internal/testsupport"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
)

func TestWebmailDeliveryAndPersistentOptOut(t *testing.T) {
	testsupport.Database(t)
	ctx := context.Background()
	must := func(e error) {
		t.Helper()
		if e != nil {
			t.Fatal(e)
		}
	}
	t.Setenv("MAPLEMAIL_FROM_ADDRESS", "sender@example.com")
	t.Setenv("MAPLEMAIL_REPLY_TO_ADDRESS", "sender@example.com")
	must(mail_service.InitializeOutboundIdentity(ctx))
	must(mail_service.SaveOutboundIdentity(ctx, mail_service.DeliveryIdentity{From: "sender@example.com", ReplyTo: "replies@example.com"}))
	t.Setenv("MAPLEMAIL_FROM_ADDRESS", "ignored@external.invalid")
	identity, e := mail_service.OutboundIdentity("other@example.com")
	must(e)
	if identity.From != "sender@example.com" || identity.ReplyTo != "replies@example.com" {
		t.Fatal("database identity did not override legacy environment", identity)
	}
	if e = mail_service.SaveOutboundIdentity(ctx, mail_service.DeliveryIdentity{From: "missing@example.com"}); e == nil {
		t.Fatal("nonexistent local sender accepted")
	}
	must(Authenticate(ctx, "sender@example.com", "test"))
	prior := WarmupCheck
	defer func() { WarmupCheck = prior }()
	warmupCalls := 0
	WarmupCheck = func(_ context.Context, recipients []string) error {
		warmupCalls++
		if recipients[0] != "buyer@client.test" {
			t.Fatal("warmup did not get actual address")
		}
		return nil
	}
	req := Request{Action: "prepare", MessageID: "<receipt-test@example.com>", Recipients: []string{"buyer@client.test"}, HTML: `<p style="color:green">Offer</p><a href="https://example.org/catalog">Catalog</a>`, Options: Options{Unsubscribe: true, TrackOpen: true, TrackClick: true, Warmup: true}}
	result, e := Prepare(ctx, "sender@example.com", req)
	must(e)
	body := result["html"].(string)
	if warmupCalls != 1 || !strings.Contains(body, `style="color:green"`) || !strings.Contains(body, "/subscribe_form.html?token=") || !strings.Contains(body, "/unsubscribe_new.html?jwt=") || !strings.Contains(body, "/pmta/w/") {
		t.Fatal("delivery controls or format missing", body)
	}
	header := result["headers"].(g.Map)["List-Unsubscribe"].(string)
	parsed, e := url.Parse(strings.Trim(header, "<>"))
	must(e)
	token := parsed.Query().Get("jwt")
	claims, e := batch_mail.ParseUnsubscribeJWT(token)
	must(e)
	if claims.Email != "buyer@client.test" || claims.GroupId < 1 {
		t.Fatal("unsubscribe link has no valid contact scope", claims)
	}
	record, e := g.DB().Model("bm_contact_groups").Where("id", claims.GroupId).One()
	must(e)
	// Tracking callbacks are exercised through HTTP without opening external sites.
	server := g.Server("webmail-delivery-integration")
	server.SetAddr("127.0.0.1:0")
	server.SetDumpRouterMap(false)
	server.BindHandler("/webmail/delivery", Handler)
	server.BindHandler("/pmta/*any", func(r *ghttp.Request) { maillog_stat.CampaignEventHandler(r, r.Get("any").String()) })
	server.Group("/api", func(group *ghttp.RouterGroup) {
		group.Middleware(middlewares.HandleApiResponse)
		group.Bind(subscribecontroller.NewV1())
	})
	must(server.Start())
	defer server.Shutdown()
	base := fmt.Sprintf("http://127.0.0.1:%d", server.GetListenedPort())
	client := &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	tokens, e := g.DB().Model("bm_webmail_tracking").Where("message_id", "receipt-test@example.com").All()
	must(e)
	if len(tokens) != 2 {
		t.Fatal("tracking registrations missing", tokens)
	}
	for _, tracking := range tokens {
		response, e := client.Get(base + "/pmta/w/" + tracking["token"].String())
		must(e)
		io.Copy(io.Discard, response.Body)
		response.Body.Close()
		if tracking["event_type"].String() == "click" && response.Header.Get("Location") != "https://example.org/catalog" {
			t.Fatal("click redirect lost")
		}
	}
	for _, table := range []string{"mailstat_opened", "mailstat_clicked"} {
		n, e := g.DB().Model(table).Where("message_id", "receipt-test@example.com").Count()
		must(e)
		if n != 1 {
			t.Fatal("tracking event not stored", table, n)
		}
	}
	request, _ := http.NewRequest("POST", base+"/webmail/delivery", strings.NewReader(`{"action":"config"}`))
	request.SetBasicAuth("sender@example.com", "test")
	response, e := client.Do(request)
	must(e)
	var output map[string]interface{}
	must(json.NewDecoder(response.Body).Decode(&output))
	response.Body.Close()
	if output["success"] != true {
		t.Fatal("authenticated webmail config unavailable", output)
	}
	// Real public unsubscribe handler: persistent record and contact state change.
	_, e = g.DB().Model("bm_contact_groups").Where("id", claims.GroupId).Data("send_unsubscribe_email", 0).Update()
	must(e)
	res, e := batchcontroller.NewV1().UnsubscribeNew(ctx, &mailapi.UnsubscribeNewReq{Jwt: token})
	must(e)
	if !res.Success {
		t.Fatal("unsubscribe handler failed", res)
	}
	blocked, e := deliveryprefs.IsSuppressed(ctx, "sender@example.com", "buyer@client.test")
	must(e)
	if !blocked {
		t.Fatal("unsubscribe record not persisted")
	}
	// Reactivating/importing contacts cannot override the durable suppression.
	_, e = g.DB().Model("bm_contacts").Where("email", "buyer@client.test").Data(g.Map{"active": 1, "status": 1}).Update()
	must(e)
	must(governance.Migrate(ctx))
	blocked, e = deliveryprefs.IsSuppressed(ctx, "sender@example.com", "buyer@client.test")
	must(e)
	if !blocked {
		t.Fatal("migration or contact import bypassed opt-out")
	}
	sender := &mail_service.EmailSender{Email: "sender@example.com", Host: "127.0.0.1", Port: "1"}
	if e = sender.SendOnce(mail_service.NewMessage("Should not send", "Offer"), []string{"buyer@client.test"}); e == nil || !strings.Contains(e.Error(), "unsubscribed") {
		t.Fatal("SMTP submission did not block an opted-out address", e)
	}
	off := req
	off.Options = Options{}
	if _, e = Prepare(ctx, "sender@example.com", off); e == nil {
		t.Fatal("turning off the footer bypassed opt-out protection")
	}
	// Resubscription always needs a confirmation, even when the list normally uses single opt-in.
	confirmations := make(chan string, 1)
	previousMailer := subscribecontroller.SendSubscriptionMail
	defer func() { subscribecontroller.SendSubscriptionMail = previousMailer }()
	subscribecontroller.SendSubscriptionMail = func(_ context.Context, _ string, _ string, _ string, confirm string) error {
		confirmations <- confirm
		return nil
	}
	response, e = client.PostForm(base+"/api/subscribe/submit", url.Values{"email": {"buyer@client.test"}, "token": {record["token"].String()}})
	must(e)
	response.Body.Close()
	var confirmURL string
	select {
	case confirmURL = <-confirmations:
	case <-time.After(3 * time.Second):
		t.Fatal("resubscription did not require confirmation")
	}
	blocked, e = deliveryprefs.IsSuppressed(ctx, "sender@example.com", "buyer@client.test")
	must(e)
	if !blocked {
		t.Fatal("unconfirmed form submission cleared opt-out")
	}
	confirmation, e := url.Parse(confirmURL)
	must(e)
	response, e = client.Get(base + "/api/subscribe/confirm?" + confirmation.RawQuery)
	must(e)
	response.Body.Close()
	blocked, e = deliveryprefs.IsSuppressed(ctx, "sender@example.com", "buyer@client.test")
	must(e)
	if blocked {
		t.Fatal("explicit confirmation did not restore subscription")
	}
	must(governance.Migrate(ctx))
	blocked, e = deliveryprefs.IsSuppressed(ctx, "sender@example.com", "buyer@client.test")
	must(e)
	if blocked {
		t.Fatal("historical backfill reactivated a confirmed opt-out record")
	}
	saved, e := g.DB().Model("bm_delivery_suppressions").Where("email", "buyer@client.test").One()
	must(e)
	if saved.IsEmpty() || saved["resubscribed_at"].IsEmpty() {
		t.Fatal("opt-out history was erased")
	}
	// Disabled options do not inject links or tracking, and a warmup rejection stops preparation.
	clean, e := Prepare(ctx, "sender@example.com", off)
	must(e)
	if strings.Contains(clean["html"].(string), "/pmta/") || strings.Contains(clean["html"].(string), "subscribe_form") {
		t.Fatal("disabled controls still injected content")
	}
	WarmupCheck = func(context.Context, []string) error { return errors.New("quota exhausted") }
	if _, e = Prepare(ctx, "sender@example.com", req); e == nil || !strings.Contains(e.Error(), "quota") {
		t.Fatal("IP warmup rejection ignored", e)
	}
	if _, e = Prepare(ctx, "sender@example.com", Request{Action: "prepare", Recipients: []string{"a@example.org", "b@example.org"}, Options: Options{TrackOpen: true}}); e == nil {
		t.Fatal("ambiguous multi-recipient tracking accepted")
	}
	exerciseWorkbench(t, ctx)
}
