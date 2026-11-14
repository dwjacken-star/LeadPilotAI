package social

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"billionmail-core/internal/service/governance"
	"billionmail-core/internal/service/growth"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/gconv"
)

type masterBridgeAI struct{}

func (masterBridgeAI) Extract(ctx context.Context, p *governance.Principal, purpose string, input growth.AIInput, out interface{}) error {
	if purpose == "master_MASTER" {
		return json.Unmarshal([]byte(`{"goal":"Approved business awareness","markets":[{"country":"US","language":"en","focus":"B2B services","buyer_types":["business buyer"],"search_queries":1,"priority":"normal","reason":"Approved requirement"}],"risks":[],"human_checkpoints":["Publication approval"]}`), out)
	}
	if purpose == "master_S10" {
		return json.Unmarshal([]byte(`{"subject":"Business service introduction","paragraphs":["Our approved company information.","Contact our team for details."],"language":"en"}`), out)
	}
	return errors.New("unexpected master bridge purpose")
}
func exerciseMasterNativeSocialHandoff(t *testing.T) {
	ctx := context.Background()
	t.Setenv("MAPLEMAIL_GROWTH_DATA", t.TempDir())
	p, e := governance.Load(ctx, 1)
	must(t, e)
	must(t, growth.MasterSeed(ctx))
	// Previous lifecycle subtests have completed; isolate this native publishing lane.
	_, e = g.DB().Model("bm_social_plans").Data("status", "paused").Where("id>0").Update()
	must(t, e)
	cid, e := growth.SaveCompany(ctx, p, 0, g.Map{"name": "Master social fixture", "profile": g.Map{"business": "Professional business services"}})
	must(t, e)
	must(t, growth.PublishCompany(ctx, p, cid))
	cfg, e := config(ctx)
	must(t, e)
	cfg.Facebook.Enabled, cfg.Facebook.Publishing = true, true
	cfg.Facebook.ClientID = "fixture-app"
	cfg.Facebook.ClientSecret = "fixture-secret"
	cfg.Facebook.RedirectURL = "https://example.com/social/callback"
	must(t, SaveSettings(ctx, p, cfg))
	token, e := growth.SocialCredential("fixture-page-token", false)
	must(t, e)
	account, e := g.DB().Model("bm_social_accounts").Data(g.Map{"platform": "facebook", "external_id": "fixture-page", "name": "Fixture authorized page", "dept_id": p.DeptID, "owner_id": p.ID, "token": token, "capabilities": encode(g.Map{"publish": true})}).InsertAndGetId()
	must(t, e)
	pack, e := g.DB().Model("bm_gm_packs").Where("slug", "general").Value("id")
	must(t, e)
	c := growth.MasterDefaultConfig("WF04")
	c.Requirement = "Company introduction"
	c.Cadence = "manual"
	c.Channels = []string{"facebook"}
	c.SocialAccounts = []int64{account}
	c.Budget.Tokens = 500000
	result, e := growth.MasterAction(ctx, p, "save_workflow", 0, 0, 1, "", "", g.Map{"company_id": cid, "pack_id": pack.Int64(), "name": "Master native handoff", "code": "WF04", "config": c})
	must(t, e)
	w := gconv.Int64(result)
	oldAI, oldHTTP := growth.IntelligenceProvider, httpClient
	growth.IntelligenceProvider = masterBridgeAI{}
	calls := 0
	httpClient = &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Host != "graph.facebook.com" || !strings.HasSuffix(r.URL.Path, "/fixture-page/feed") {
			t.Fatal("unexpected provider call", r.URL)
		}
		return response(200, `{"id":"fixture-page_fixture-post"}`), nil
	})}
	t.Cleanup(func() { growth.IntelligenceProvider, httpClient = oldAI, oldHTTP })
	run, e := growth.MasterWorkflowControl(ctx, p, w, "run")
	must(t, e)
	for i := 0; i < 4; i++ {
		_, e = growth.MasterRunOne(ctx)
		must(t, e)
	}
	r, e := g.DB().Model("bm_gm_runs").Where("id", run).One()
	must(t, e)
	if r["status"].String() != "waiting_approval" {
		t.Fatal("master did not wait for review", r["error"].String())
	}
	content, e := g.DB().Model("bm_gm_contents").Where("run_id", run).One()
	must(t, e)
	_, e = growth.MasterAction(ctx, p, "content_control", content["id"].Int64(), cid, 1, "", "", g.Map{"operation": "approve"})
	must(t, e)
	for i := 0; i < 4; i++ {
		_, e = growth.MasterRunOne(ctx)
		must(t, e)
	}
	native, e := g.DB().Model("bm_social_plans").Where("master_content_id", content["id"]).One()
	must(t, e)
	if len(native) == 0 {
		r, _ = g.DB().Model("bm_gm_runs").Where("id", run).One()
		t.Fatal("native plan was not created", r["status"].String(), r["error"].String())
	}
	if calls != 0 {
		t.Fatal("public post occurred during queue preparation")
	}
	_, e = ProcessDelivery(ctx)
	must(t, e)
	if calls != 1 {
		d, _ := g.DB().Model("bm_social_deliveries").Where("plan_id", native["id"]).One()
		t.Fatal("native worker did not publish exactly once", calls, d["status"].String(), d["error"].String())
	}
	_, e = ProcessDelivery(ctx)
	must(t, e)
	if calls != 1 {
		t.Fatal("duplicate publish")
	}
	must(t, growth.MasterTick(ctx))
	current, e := g.DB().Model("bm_gm_contents").Where("id", content["id"]).One()
	must(t, e)
	if current["status"].String() != "published" || current["external_id"].String() != "fixture-page_fixture-post" {
		t.Fatal("publication receipt not synchronized")
	}
}
