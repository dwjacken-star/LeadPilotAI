package maillog_stat

import (
	"context"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"billionmail-core/internal/service/governance"
	"billionmail-core/internal/service/public"
	"billionmail-core/internal/testsupport"
	"github.com/gogf/gf/v2/frame/g"
)

func TestGovernedOverviewScope(t *testing.T) {
	testsupport.Database(t)
	ctx := context.Background()
	now := time.Now().Unix()
	exec := func(query string, args ...interface{}) {
		t.Helper()
		if _, err := g.DB().Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO bm_departments(id,parent_id,name) VALUES(10,1,'Sales'),(11,10,'Child'),(20,1,'Other')`)
	exec(`INSERT INTO account(account_id,username,password,email,status,dept_id) VALUES
 (101,'overview-owner','fixture','',1,10),(102,'overview-peer','fixture','',1,10),
 (103,'overview-child','fixture','',1,11),(201,'overview-foreign','fixture','',1,20)`)
	exec(`INSERT INTO email_templates(id,temp_name,content,dept_id,owner_id) VALUES
 (101,'overview-own','<p>Own fixture</p>',10,101),
 (103,'overview-child','<p>Child fixture</p>',11,103),
 (201,'overview-foreign','<p>Foreign fixture</p>',20,201)`)
	exec(`INSERT INTO domain(domain) VALUES('shared.example'),('secret.example'),('unused.example')`)
	exec(`INSERT INTO email_tasks(id,task_name,addresser,recipient_count,template_id,dept_id,owner_id)
 VALUES(101,'Own','sender@shared.example',1,101,10,101),
 (102,'Peer','sender@shared.example',1,101,10,102),
 (103,'Child','sender@child.example',1,103,11,103),
 (201,'Foreign','sender@secret.example',1,201,20,201)`)
	exec(`INSERT INTO recipient_info(task_id,recipient,message_id) VALUES
 (101,'own@client.test','<mid-own>'),(102,'peer@client.test','<mid-peer>'),
 (103,'child@client.test','<mid-child>'),(201,'foreign@client.test','<mid-foreign>'),
 (101,'own-fail@client.test','<mid-mixed>'),(201,'foreign-fail@client.test','<mid-mixed>')`)
	exec(`INSERT INTO api_templates(id,api_key,api_name,template_id,subject,addresser,dept_id,owner_id)
 VALUES(101,'test-overview','API',101,'Subject','sender@api.example',10,101)`)
	exec(`INSERT INTO api_mail_logs(api_id,recipient,message_id,addresser)
 VALUES(101,'api@client.test','<mid-api>','sender@api.example')`)
	exec(`INSERT INTO bm_crm_customers(id,dept_id,owner_id,email) VALUES(101,10,101,'crm@client.test')`)
	exec(`INSERT INTO bm_crm_messages(customer_id,dept_id,owner_id,mailbox,direction,message_id,to_address)
 VALUES(101,10,101,'sender@crm.example','outgoing','<mid-crm>','crm@client.test')`)
	for _, delivery := range []struct {
		queue, recipient, domain, provider, status string
	}{
		{"own", "own", "shared.example", "visible", "sent"},
		{"peer", "peer", "shared.example", "visible", "bounced"},
		{"child", "child", "child.example", "child-provider", "sent"},
		{"foreign", "foreign", "secret.example", "secret-provider", "bounced"},
		{"mixed", "own-fail", "shared.example", "visible", "bounced"},
		{"mixed-foreign", "foreign-fail", "shared.example", "secret-provider", "bounced"},
		{"unowned", "unowned", "shared.example", "unowned-provider", "sent"},
		{"api", "api", "api.example", "visible", "sent"},
		{"crm", "crm", "crm.example", "visible", "sent"},
	} {
		exec(`INSERT INTO mailstat_send_mails(postfix_message_id,recipient,mail_provider,status,dsn,description,log_time_millis)
 VALUES($1,$2,$3,$4,$5,$6,$7)`, "queue-"+delivery.queue, delivery.recipient+"@client.test", delivery.provider, delivery.status,
			map[bool]string{true: "2.0.0", false: "5.1.1"}[delivery.status == "sent"], "failure-"+delivery.recipient, now*1000)
		exec(`INSERT INTO mailstat_senders(postfix_message_id,sender,log_time_millis) VALUES($1,$2,$3)`,
			"queue-"+delivery.queue, "sender@"+delivery.domain, now*1000)
		messageID := "mid-" + delivery.queue
		// Send rows are queue-keyed. Separate queues may retain one Message-ID;
		// recipient matching must still isolate their departments and owners.
		if delivery.queue == "mixed-foreign" {
			messageID = "mid-mixed"
		}
		exec(`INSERT INTO mailstat_message_ids(postfix_message_id,message_id,log_time_millis) VALUES($1,$2,$3)`,
			"queue-"+delivery.queue, messageID, now*1000)
	}
	// Historical events and queue-only diagnostics can concern another recipient
	// even though the queue's retained send row now identifies own-fail.
	exec(`INSERT INTO mailstat_opened(postfix_message_id,message_id,recipient,log_time_millis)
 VALUES('queue-own','mid-own','OWN@client.test',$1),('queue-mixed','mid-mixed','foreign-fail@client.test',$1)`, now*1000)
	exec(`INSERT INTO mailstat_clicked(postfix_message_id,message_id,recipient,log_time_millis)
 VALUES('queue-own','mid-own','own@client.test',$1)`, now*1000)
	exec(`INSERT INTO mailstat_deferred_mails(postfix_message_id,description,relay,log_time_millis)
 VALUES('queue-mixed','private queue-level recipient error','private-relay',$1)`, now*1000)
	public.SetCache("postfix_deferred_queue_count", 987, 300)
	member := func(scope string, custom ...int64) *governance.Principal {
		return &governance.Principal{ID: 101, DeptID: 10, Roles: []governance.Role{
			{Scope: scope, Custom: custom, Permissions: []string{OverviewPermission}},
			{Scope: "all", Permissions: []string{"report:list", "campaign:list"}},
		}}
	}
	o := NewOverview()
	stats := func(p *governance.Principal, campaign int64, domain string) map[string]interface{} {
		t.Helper()
		result, err := o.OverviewForPrincipal(ctx, p, campaign, domain, now-3600, now+1)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	for _, tc := range []struct {
		name, scope string
		custom      []int64
		sends       int
	}{
		{"department ignores unrelated all-scope role", "department", nil, 5},
		{"self", "self", nil, 4},
		{"descendants", "descendants", nil, 6},
		{"custom", "custom", []int64{20}, 2},
		{"all still requires attributed ownership", "all", nil, 8},
		{"empty custom", "custom", nil, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := stats(member(tc.scope, tc.custom...), 0, "")
			dashboard := result["dashboard"].(map[string]interface{})
			if dashboard["sends"] != tc.sends || dashboard["delayed_queue"] != 0 || result["queue_access"] != false {
				t.Fatal("incorrect scope or global queue leak", result)
			}
			total := 0
			for _, row := range result["send_mail_chart"].(map[string]interface{})["data"].([]map[string]interface{}) {
				total += row["sends"].(int)
			}
			if total != tc.sends {
				t.Fatal("chart scope differs from dashboard", total, tc.sends)
			}
		})
	}
	department := member("department")
	dashboard := stats(department, 0, "")["dashboard"].(map[string]interface{})
	if dashboard["delivered"] != 3 || dashboard["bounced"] != 2 || dashboard["opened"] != 1 || dashboard["clicked"] != 1 || dashboard["open_tracked"] != 2 || dashboard["open_rate"] != 50.0 {
		t.Fatal("scoped measurements incorrect", dashboard)
	}
	for _, provider := range stats(department, 0, "")["mail_providers"].([]map[string]interface{}) {
		if provider["mail_provider"] != "visible" {
			t.Fatal("unrelated provider exposed", provider)
		}
	}
	for _, tc := range []struct {
		campaign int64
		domain   string
		sends    int
	}{{0, "shared.example", 3}, {0, "SHARED.EXAMPLE", 3}, {0, "%.example", 0}, {0, "secret.example", 0}, {201, "", 0}, {101, "", 2}, {0, "api.example", 1}} {
		if got := stats(department, tc.campaign, tc.domain)["dashboard"].(map[string]interface{})["sends"]; got != tc.sends {
			t.Fatal("domain/campaign filter escaped scope", tc, got)
		}
	}
	for _, tc := range []struct {
		p    *governance.Principal
		want []string
	}{
		{department, []string{"api.example", "crm.example", "shared.example"}},
		{member("descendants"), []string{"api.example", "child.example", "crm.example", "shared.example"}},
		{member("custom", 20), []string{"secret.example", "shared.example"}},
		{member("custom"), []string{}},
	} {
		got, err := o.DomainsForPrincipal(ctx, tc.p)
		if err != nil || !reflect.DeepEqual(got, tc.want) {
			t.Fatal("domain options escaped scope", got, tc.want, err)
		}
	}
	for _, tc := range []struct {
		p        *governance.Principal
		campaign int64
		domain   string
		count    int
	}{
		{department, 0, "", 2}, {member("self"), 0, "", 1},
		{department, 201, "", 0}, {department, 0, "secret.example", 0},
		{member("custom", 20), 0, "", 2},
	} {
		rows, err := o.FailedListForPrincipal(ctx, tc.p, tc.campaign, tc.domain, now-3600, now+1)
		if err != nil || len(rows) != tc.count {
			t.Fatal("failure scope incorrect", rows, tc.count, err)
		}
		for _, row := range rows {
			if strings.Contains(g.NewVar(row["description"]).String(), "private queue-level") || row["relay"] == "private-relay" {
				t.Fatal("queue-only deferred details exposed", row)
			}
		}
	}
	t.Run("task analytics stays independent and task plus domain is conjunctive", func(t *testing.T) {
		campaignOnly := &governance.Principal{ID: 101, DeptID: 10, Roles: []governance.Role{
			{Scope: "self", Permissions: []string{"campaign:list"}},
		}}
		taskCtx := governance.WithPrincipal(ctx, campaignOnly)
		if campaignOnly.Has(OverviewPermission) {
			t.Fatal("fixture unexpectedly grants Overview")
		}
		if err := campaignOnly.Check(taskCtx, "email_tasks", 101, "campaign:list"); err != nil {
			t.Fatal("campaign-only fixture cannot view its task", err)
		}
		for _, tc := range []struct {
			domain string
			sends  int
		}{{"", 2}, {"shared.example", 2}, {"secret.example", 0}} {
			result, err := o.OverviewContext(taskCtx, 101, tc.domain, now-3600, now+1)
			if err != nil {
				t.Fatal("task analytics requires independent Overview permission", err)
			}
			dashboard := result["dashboard"].(map[string]interface{})
			if dashboard["sends"] != tc.sends || dashboard["delayed_queue"] != 0 {
				t.Fatal("task/domain restriction or queue concealment failed", tc, result)
			}
			providers := result["mail_providers"].([]map[string]interface{})
			if tc.sends == 0 && len(providers) != 0 {
				t.Fatal("unmatched task domain exposes providers", providers)
			}
			for _, provider := range providers {
				if provider["mail_provider"] != "visible" {
					t.Fatal("task analytics exposed another task provider", provider)
				}
			}
		}
		adminCtx := governance.WithPrincipal(ctx, &governance.Principal{Super: true})
		adminResult, err := o.OverviewContext(adminCtx, 101, "shared.example", now-3600, now+1)
		if err != nil || adminResult["dashboard"].(map[string]interface{})["delayed_queue"] != 987 {
			t.Fatal("administrator task queue behavior changed", adminResult, err)
		}
	})
	t.Run("administrator parity", func(t *testing.T) {
		admin := &governance.Principal{Super: true}
		want, err := o.OverviewContext(ctx, 0, "", now-3600, now+1)
		if err != nil {
			t.Fatal(err)
		}
		got := stats(admin, 0, "")
		if got["queue_access"] != true {
			t.Fatal("admin queue drilldown missing")
		}
		delete(got, "queue_access")
		if !reflect.DeepEqual(got, want) || got["dashboard"].(map[string]interface{})["delayed_queue"] != 987 {
			t.Fatal("administrator global overview changed", got, want)
		}
		failures, err := o.FailedListForPrincipal(ctx, admin, 0, "", now-3600, now+1)
		if err != nil || !reflect.DeepEqual(failures, o.FailedList(0, "", now-3600, now+1)) {
			t.Fatal("administrator failure list changed", err)
		}
		domains, err := o.DomainsForPrincipal(ctx, admin)
		sort.Strings(domains)
		if err != nil || !reflect.DeepEqual(domains, []string{"secret.example", "shared.example", "unused.example"}) {
			t.Fatal("administrator global domains changed", domains, err)
		}
	})
	t.Run("permission required for every read", func(t *testing.T) {
		denied := &governance.Principal{ID: 101, DeptID: 10, Roles: []governance.Role{{Scope: "all", Permissions: []string{"report:list"}}}}
		for _, p := range []*governance.Principal{nil, denied} {
			if _, err := o.OverviewForPrincipal(ctx, p, 0, "", now-3600, now+1); err == nil {
				t.Fatal("overview allowed without permission")
			}
			if _, err := o.FailedListForPrincipal(ctx, p, 0, "", now-3600, now+1); err == nil {
				t.Fatal("failures allowed without permission")
			}
			if _, err := o.DomainsForPrincipal(ctx, p); err == nil {
				t.Fatal("domains allowed without permission")
			}
		}
	})
}
