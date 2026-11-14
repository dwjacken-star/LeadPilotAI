package social

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"billionmail-core/internal/service/governance"
	"billionmail-core/internal/service/growth"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/gconv"
)

func exerciseFacebookSyncIsolation(t *testing.T, ctx context.Context, p *governance.Principal, company int64) {
	old := httpClient
	t.Cleanup(func() { httpClient = old })
	token, e := growth.SocialCredential("sync-fixture-page-token", false)
	must(t, e)
	aid, e := g.DB().Model("bm_social_accounts").Data(g.Map{"platform": "facebook", "external_id": "sync-page", "name": "Sync isolation", "dept_id": p.DeptID, "owner_id": p.ID, "token": token, "scopes": `["pages_read_engagement"]`, "capabilities": `{"publish":true,"reactions":true,"metrics":true}`, "next_sync_at": time.Now().Add(time.Hour)}).InsertAndGetId()
	must(t, e)
	c := PlanConfig{Platforms: []string{"facebook"}, Accounts: []int64{aid}, Markets: []Market{{Country: "US", Language: "en", Timezone: "UTC", LocalTime: "10:00:00"}}, StartDate: time.Now().AddDate(0, 0, 10).Format("2006-01-02"), Cadence: "once", DailyPosts: 3, MinIntervalMinutes: 5}
	plan, e := SavePlan(ctx, p, 0, "Sync object failure", company, c)
	must(t, e)
	must(t, ManualVariants(ctx, p, plan))
	v, e := g.DB().Model("bm_social_variants").Where("plan_id", plan).Value("id")
	must(t, e)
	ids := []int64{}
	for _, remote := range []string{"sync-page_bad", "sync-page_good"} {
		id, e := g.DB().Model("bm_social_deliveries").Data(g.Map{"plan_id": plan, "variant_id": v.Int64(), "account_id": aid, "snapshot": "{}", "variant_version": 1, "cycle_at": time.Now(), "due_at": time.Now(), "status": "published", "publisher_id": p.ID, "remote_id": remote}).InsertAndGetId()
		must(t, e)
		ids = append(ids, id)
	}
	_, e = g.DB().Model("bm_social_metrics").Data(g.Map{"delivery_id": ids[0], "account_id": aid, "counts": `{"comments":7}`, "availability": `{"comments":"available"}`, "expires_at": time.Now().AddDate(1, 0, 0)}).Insert()
	must(t, e)
	bad, expired, missingPageScope := true, false, false
	goodCalls, pageChecks := 0, 0
	httpClient = &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != "GET" {
			t.Fatal("sync attempted to post")
		}
		if strings.HasSuffix(r.URL.Path, "/sync-page") {
			pageChecks++
			return response(200, `{"id":"sync-page","name":"Authorized Page"}`), nil
		}
		if strings.HasSuffix(r.URL.Path, "/sync-page/posts") {
			if missingPageScope {
				return response(400, `{"error":{"code":10,"message":"Page posts permission missing"}}`), nil
			}
			return response(200, `{"data":[{"id":"sync-page_good"}]}`), nil
		}
		if strings.HasSuffix(r.URL.Path, "/sync-page_bad") && expired {
			return response(400, `{"error":{"code":190,"message":"Expired access token"}}`), nil
		}
		if strings.HasSuffix(r.URL.Path, "/sync-page_bad") && bad {
			return response(400, `{"error":{"code":10,"message":"Object does not exist or needs pages_read_engagement"}}`), nil
		}

		if strings.HasSuffix(r.URL.Path, "/insights") {
			switch r.URL.Query().Get("metric") {
			case "post_media_view":
				return response(200, `{"data":[{"name":"post_media_view","values":[{"value":17}]}]}`), nil
			case "post_clicks":
				return response(400, `{"error":{"code":100,"message":"The value must be a valid insights metric"}}`), nil
			default:
				t.Fatal("deprecated or combined metrics requested", r.URL.Query().Get("metric"))
			}
		}
		goodCalls++
		return response(200, `{"id":"sync-page_good","reactions":{"summary":{"total_count":0}},"comments":{"summary":{"total_count":2}},"shares":{"count":1}}`), nil
	})}
	must(t, SyncAccount(ctx, p, aid))
	account, e := g.DB().Model("bm_social_accounts").Where("id", aid).One()
	must(t, e)
	if account["status"].String() != "connected" || account["error"].String() != "POST_SYNC_UNAVAILABLE:1" || goodCalls != 1 || pageChecks != 1 {
		t.Fatal("one bad post blocked a valid Page or subsequent posts", account["status"], account["error"], goodCalls, pageChecks)
	}
	for index, id := range ids {
		d, e := g.DB().Model("bm_social_deliveries").Where("id", id).One()
		must(t, e)
		expected := "unavailable"
		if index == 1 {
			expected = "synced"
		}
		if d["sync_state"].String() != expected || d["status"].String() != "published" {
			t.Fatal("sync overwrote publication status or lost per-post result")
		}
	}
	m, e := g.DB().Model("bm_social_metrics").Where("delivery_id", ids[0]).One()
	must(t, e)
	if object(m["counts"])["comments"] == nil || object(m["availability"])["comments"] != "object_unavailable" {
		t.Fatal("old counts were overwritten or exposed as current")
	}

	good, e := g.DB().Model("bm_social_metrics").Where("delivery_id", ids[1]).One()
	must(t, e)
	if good["counts"].String() == "" || object(good["availability"])["views"] != "available" || object(good["availability"])["clicks"] != "not_supported" {
		t.Fatal("unrelated metric failure discarded real Page views")
	}
	if gconv.Int(object(good["counts"])["views"]) != 17 || object(good["counts"])["impressions"] != nil {
		t.Fatal("views were lost or relabeled as impressions")
	}
	bad = false
	must(t, SyncAccount(ctx, p, aid))
	account, e = g.DB().Model("bm_social_accounts").Where("id", aid).One()
	must(t, e)
	if account["error"].String() != "" {
		t.Fatal("successful resync did not clear warning")
	}
	bad, missingPageScope = true, true
	if SyncAccount(ctx, p, aid) == nil {
		t.Fatal("Page-wide permission failure was treated as a single missing post")
	}
	bad, missingPageScope = false, false
	expired = true
	if SyncAccount(ctx, p, aid) == nil {
		t.Fatal("actual authorization failure was swallowed")
	}
	account, e = g.DB().Model("bm_social_accounts").Where("id", aid).One()
	must(t, e)
	if account["status"].String() != "reconnect" {
		t.Fatal("expired token was not marked for reconnect")
	}
}

func TestFacebookObjectErrorClassification(t *testing.T) {
	for _, e := range []*RemoteError{{Code: 10}, {Code: 100, Subcode: 33}} {
		if !facebookObjectUnavailable(e) {
			t.Fatal("object error not recognized")
		}
	}
	for _, e := range []*RemoteError{{Code: 190}, {Code: 4}, {Code: 100}, {Status: 403, Kind: "permission_denied"}} {
		if facebookObjectUnavailable(e) {
			t.Fatal("account/network error treated as a single missing post")
		}
	}
}
