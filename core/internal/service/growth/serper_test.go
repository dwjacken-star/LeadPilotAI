package growth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"billionmail-core/internal/service/governance"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

func exerciseSerperPipeline(t *testing.T, ctx context.Context, p *governance.Principal) {
	original, e := g.DB().Model("bm_options").Where("name", "growth_serper_config").Value("value")
	must(t, e)
	oldHTTP, oldSearchURL, oldScrapeURL, oldSearch, oldCollect := serperHTTP, serperSearchURL, serperScrapeURL, SearchBusinesses, CollectWebsite
	defer func() {
		serperHTTP = oldHTTP
		serperSearchURL = oldSearchURL
		serperScrapeURL = oldScrapeURL
		SearchBusinesses = oldSearch
		CollectWebsite = oldCollect
		_, _ = g.DB().Model("bm_options").Data(g.Map{"name": "growth_serper_config", "value": original.String()}).OnConflict("name").Save()
	}()
	var requests atomic.Int32
	api := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Header.Get("X-API-KEY") != "fixture-serper-secret" {
			t.Error("missing Serper credential")
		}
		var in g.Map
		must(t, json.NewDecoder(r.Body).Decode(&in))
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/search" {
			q := in["q"].(string)
			if strings.Contains(q, "unauthorized") {
				w.WriteHeader(401)
				io.WriteString(w, `{"message":"fixture-serper-secret"}`)
				return
			}
			if strings.Contains(q, "no-results") {
				io.WriteString(w, `{"organic":[],"credits":1}`)
				return
			}
			if in["gl"] != "us" || in["hl"] != "en" || in["num"].(float64) != 4 {
				t.Error("search locale/result limits lost", in)
			}
			io.WriteString(w, `{"credits":1,"organic":[{"title":"Fixture Importers","link":"https://serper-fixture.example/","snippet":"USA automotive distributor"},{"title":"No Email Company","link":"https://no-email-fixture.example/"},{"title":"Duplicate","link":"https://www.serper-fixture.example/contact"},{"title":"Blocked","link":"http://127.0.0.1/"}]}`)
			return
		}
		if r.URL.Path == "/places" {
			if strings.Contains(in["q"].(string), "no-results") {
				io.WriteString(w, `{"credits":1,"places":[]}`)
				return
			}
			io.WriteString(w, `{"credits":1,"places":[{"title":"No website business","address":"New York, USA","phoneNumber":"12345","cid":"12345"}]}`)
			return
		}
		if r.URL.Path == "/scrape" {
			if in["includeMarkdown"] != true {
				t.Error("scraping dropped links")
			}
			if strings.Contains(in["url"].(string), "no-email") {
				io.WriteString(w, `{"credits":2,"text":"No Email Company USA automotive distributor. Contact form only.","markdown":"# No Email Company"}`)
				return
			}
			io.WriteString(w, `{"credits":2,"text":"Fixture Importers USA automotive distributor. buyer@serper-fixture.example skip@serper-fixture.example","markdown":"[Contact](/contact) [About](/about) [Private](http://10.0.0.1/contact)"}`)
			return
		}
		t.Error("unexpected Serper path", r.URL.Path)
	}))
	defer api.Close()
	serperHTTP = api.Client()
	serperSearchURL = api.URL + "/"
	serperScrapeURL = api.URL + "/scrape"
	SearchBusinesses = searchSerper
	CollectWebsite = scrapeSerper
	settings, e := SerperSettings(ctx, p)
	must(t, e)
	if strings.Contains(jsonText(settings), "fixture-serper-secret") {
		t.Fatal("credential leaked")
	}
	reader := &governance.Principal{ID: p.ID, DeptID: p.DeptID, Roles: []governance.Role{{Scope: "department", Permissions: []string{"outreach:list"}}}}
	if _, e = SerperSettings(ctx, reader); e == nil {
		t.Fatal("non-admin read provider settings")
	}
	if e = SaveSerperSettings(ctx, reader, SerperConfig{}); e == nil {
		t.Fatal("non-admin modified provider")
	}
	companyID, e := SaveCompany(ctx, p, 0, g.Map{"name": "Serper fixture", "profile": g.Map{"business": "Automotive parts"}})
	must(t, e)
	must(t, PublishCompany(ctx, p, companyID))
	c := PlanConfig{Markets: []Market{{Country: "US", Language: "en", Timezone: "UTC", City: "New York"}}, Requirement: "Find automotive distributors", Senders: []string{"sender@example.com"}, DailyLeads: 10, DailySend: 1, SearchLimit: 1, SearchResults: 4, SearchChannel: "both", PageLimit: 2, Timezone: "UTC", LocalTime: "09:00", SendStart: "00:00", SendEnd: "23:59", CooldownDays: 30, ExcludeKeywords: []string{"skip"}}
	planID, e := SavePlan(ctx, p, 0, g.Map{"name": "Serper contract fixture", "company_id": companyID, "config": c})
	must(t, e)
	runID, e := PlanControl(ctx, p, planID, "run")
	must(t, e)
	queryID, e := g.DB().Model("bm_growth_queries").Data(g.Map{"run_id": runID, "country": "US", "language": "en", "query": "fixture automotive parts"}).InsertAndGetId()
	must(t, e)
	must(t, discover(ctx, p, queryID))
	count := requests.Load()
	must(t, discover(ctx, p, queryID))
	if requests.Load() != count {
		t.Fatal("retry repeated paid search")
	}
	candidates, e := g.DB().Model("bm_growth_candidates").Where("run_id", runID).All()
	must(t, e)
	if len(candidates) != 3 {
		t.Fatal("company/places dedupe failed", candidates)
	}
	for _, candidate := range candidates {
		if candidate["website"].String() == "" {
			if candidate["state"].String() != "no_website" || candidate["address"].String() == "" {
				t.Fatal("place lost", candidate)
			}
			continue
		}
		must(t, collect(ctx, p, g.Map{"run_id": runID, "candidate_id": candidate["id"].Int64(), "url": candidate["website"].String(), "country": "US", "language": "en"}))
	}
	empty, e := g.DB().Model("bm_growth_candidates").Where("run_id", runID).Where("website", "https://no-email-fixture.example/").One()
	must(t, e)
	if empty["state"].String() != "no_email" || len(jsonMap(empty["facts"])) == 0 {
		t.Fatal("company without email was lost", empty)
	}
	lead, e := g.DB().Model("bm_growth_leads").Where("plan_id", planID).One()
	must(t, e)
	if lead["email"].String() != "buyer@serper-fixture.example" {
		t.Fatal("evidence/keyword extraction failed", lead)
	}
	collected, e := g.DB().Model("bm_growth_candidates").Where("run_id", runID).Where("website", "https://serper-fixture.example/").One()
	must(t, e)
	beforeRetry := requests.Load()
	_, e = g.DB().Model("bm_growth_candidates").Where("id", collected["id"]).Data(g.Map{"state": "failed"}).Update()
	must(t, e)
	must(t, collect(ctx, p, g.Map{"run_id": runID, "candidate_id": collected["id"].Int64(), "country": "US", "language": "en"}))
	if requests.Load() != beforeRetry {
		t.Fatal("AI retry repurchased successful scrapes")
	}
	must(t, verifyLead(ctx, p, lead["id"].Int64(), runID))
	must(t, draftLead(ctx, p, lead["id"].Int64(), runID))
	readOnly := &governance.Principal{ID: p.ID, DeptID: p.DeptID, Roles: []governance.Role{{Scope: "department", Permissions: []string{"outreach:list"}}}}
	_, e = g.DB().Model("bm_growth_candidates").Where("id", collected["id"]).Data(g.Map{"emails": gdb.Raw(`emails||'["+inc/@14.5676321"]'::jsonb`)}).Update()
	must(t, e)
	candidateDetail, e := Detail(ctx, readOnly, "candidates", collected["id"].Int64())
	must(t, e)
	referenced := candidateDetail["record"].(gdb.Record)
	if referenced["company_name"].String() != "Serper fixture" || referenced["plan_name"].String() != "Serper contract fixture" || !strings.Contains(referenced["run_name"].String(), "Serper contract fixture") {
		t.Fatal("related names require a company selector permission", referenced)
	}
	emailRows := candidateDetail["email_details"].([]g.Map)
	states := map[string]string{}
	for _, row := range emailRows {
		states[row["email"].(string)] = row["status"].(string)
	}
	if states["buyer@serper-fixture.example"] != "verified" || states["skip@serper-fixture.example"] != "filtered" || states["+inc/@14.5676321"] != "invalid" {
		t.Fatal("collected addresses missing readable states", emailRows)
	}
	leadDetail, e := Detail(ctx, readOnly, "leads", lead["id"].Int64())
	must(t, e)
	if leadDetail["record"].(gdb.Record)["company_name"].String() != "Serper fixture" {
		t.Fatal("lead company name missing")
	}
	detail, e := Detail(ctx, p, "runs", runID)
	must(t, e)
	summary := detail["live_stats"].(map[string]interface{})
	if summary["candidate_count"].(float64) != 3 || summary["draft_count"].(float64) != 1 {
		t.Fatal("run outcome counters incorrect", summary)
	}
	listing, e := List(ctx, p, "candidates", companyID, 1, "", "")
	must(t, e)
	if listing["total"].(int) != 3 {
		t.Fatal("candidate pagination", listing)
	}
	withEmail, e := List(ctx, p, "candidates", companyID, 1, "", "with_email")
	must(t, e)
	if withEmail["total"].(int) != 1 {
		t.Fatal("email-only filter did not include collected addresses", withEmail)
	}
	byAddress, e := List(ctx, p, "candidates", companyID, 1, "buyer@serper-fixture.example", "")
	must(t, e)
	if byAddress["total"].(int) != 1 {
		t.Fatal("candidate email search did not match addresses", byAddress)
	}
	outsider := &governance.Principal{ID: p.ID, DeptID: 999999, Roles: []governance.Role{{Scope: "department", Permissions: []string{"outreach:list"}}}}
	if _, e = Detail(ctx, outsider, "candidates", empty["id"].Int64()); e == nil {
		t.Fatal("cross-department candidate exposed")
	}
	noResults, e := g.DB().Model("bm_growth_queries").Data(g.Map{"run_id": runID, "country": "US", "language": "en", "query": "no-results"}).InsertAndGetId()
	must(t, e)
	must(t, discover(ctx, p, noResults))
	v, e := g.DB().Model("bm_growth_queries").Where("id", noResults).Value("status")
	must(t, e)
	if v.String() != "no_results" {
		t.Fatal("empty search not distinguished")
	}
	if _, e = searchSerper(ctx, p, "unauthorized", c.Markets[0], 4, "search"); e == nil || !strings.Contains(e.Error(), "configuration") || strings.Contains(e.Error(), "fixture-serper-secret") {
		t.Fatal("authentication failure unsafe", e)
	}
	must(t, SaveSerperSettings(ctx, p, SerperConfig{Enabled: true, DailySearches: 1, DailyScrapes: 1000, DailyCredits: 100000}))
	before := requests.Load()
	if _, e = searchSerper(ctx, p, "fixture", c.Markets[0], 4, "search"); e == nil || !strings.Contains(e.Error(), "budget") {
		t.Fatal("daily budget not enforced", e)
	}
	if requests.Load() != before {
		t.Fatal("exhausted budget still made external call")
	}
	raw, e := g.DB().Model("bm_options").Where("name", "growth_serper_config").Value("value")
	must(t, e)
	if strings.Contains(raw.String(), "fixture-serper-secret") {
		t.Fatal("plaintext key persisted")
	}
	must(t, governance.Migrate(ctx))
	n, e := g.DB().Model("bm_growth_candidates").Where("run_id", runID).Count()
	must(t, e)
	if n != 3 {
		t.Fatal("repeat migration removed candidates")
	}
	active, e := PlanControl(ctx, p, planID, "run")
	must(t, e)
	if active != runID {
		t.Fatal("concurrent manual click created another active run")
	}
	_, e = g.DB().Model("bm_growth_runs").Where("id", runID).Data(g.Map{"status": "completed"}).Update()
	must(t, e)
	_, e = g.DB().Model("bm_growth_jobs").Where("payload->>'run_id'=?", fmt.Sprint(runID)).Data(g.Map{"status": "completed"}).Update()
	must(t, e)
	fresh, e := PlanControl(ctx, p, planID, "run")
	must(t, e)
	if fresh == runID {
		t.Fatal("manual retry reused a completed same-day run")
	}
	active, e = PlanControl(ctx, p, planID, "run")
	must(t, e)
	if active != fresh {
		t.Fatal("duplicate active manual run")
	}
}

func TestSerperPageEvidence(t *testing.T) {
	u, _ := url.Parse("https://business.example/")
	page := parseSerperPage(u, "Company", "[Email](mailto:SALES@business.example) [Contact](/contact) [Thai](/ติดต่อ) [Other](https://other.example/contact) [Local](http://127.0.0.1/contact)")
	if len(page.Emails) != 1 || page.Emails[0] != "sales@business.example" || len(page.Links) != 2 {
		t.Fatal("mailto/multilingual boundaries", page)
	}
	for _, value := range []string{"+inc/@14.5676321", "//www.google.com/maps/place/grand+arcade/@10.3305157", "//user@example.com"} {
		if _, ok := collectedEmail(value); ok {
			t.Fatal("map coordinates or URL accepted as email", value)
		}
	}
	markets := []Market{{Country: "TH", Language: "th"}, {Country: "PH", Language: "en"}}
	if validateQueryDistribution(markets, [][2]string{{"TH", "th"}, {"TH", "th"}}, 2) == nil {
		t.Fatal("a requested country was omitted")
	}
	if e := validateQueryDistribution(markets, [][2]string{{"TH", "th"}, {"PH", "en"}}, 2); e != nil {
		t.Fatal(e)
	}
}
