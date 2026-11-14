package social

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"billionmail-core/internal/service/governance"
	"billionmail-core/internal/testsupport"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/gconv"
)

func TestAuthorizationTasks(t *testing.T) {
	testsupport.Database(t)
	ctx := context.Background()
	p, e := governance.Load(ctx, 1)
	must(t, e)
	cfg, e := config(ctx)
	must(t, e)
	cfg.LinkedIn.Enabled, cfg.LinkedIn.Publishing = true, true
	cfg.LinkedIn.ClientID, cfg.LinkedIn.ClientSecret = "fixture-app", "fixture-secret"
	cfg.LinkedIn.RedirectURL = "https://admin.example.com/social/callback"
	cfg.Facebook.Enabled = true
	cfg.Facebook.ClientID, cfg.Facebook.ClientSecret = "fixture-meta", "fixture-secret"
	cfg.Facebook.RedirectURL = cfg.LinkedIn.RedirectURL
	must(t, SaveSettings(ctx, p, cfg))
	original := httpClient
	t.Cleanup(func() { httpClient = original })
	calls := 0
	memberImage := false
	httpClient = &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		switch {
		case r.URL.Host == "www.linkedin.com" && r.URL.Path == "/oauth/v2/accessToken":
			body, _ := io.ReadAll(r.Body)
			form, _ := url.ParseQuery(string(body))
			return response(200, `{"access_token":"fixture-token-`+form.Get("code")+`","scope":"openid profile w_member_social","expires_in":3600}`), nil
		case r.URL.Host == "api.linkedin.com" && r.URL.Path == "/v2/userinfo":
			if r.Header.Get("Authorization") == "Bearer fixture-token-second" {
				return response(200, `{"sub":"member-two","name":"Second member"}`), nil
			}
			return response(200, `{"sub":"member-one","name":"First member"}`), nil
		case r.URL.Host == "api.linkedin.com" && r.URL.Path == "/rest/posts":
			body, _ := io.ReadAll(r.Body)
			var payload g.Map
			must(t, json.Unmarshal(body, &payload))
			if payload["author"] != "urn:li:person:member-one" {
				t.Fatal("personal post author was not the authorized member", payload["author"])
			}
			if memberImage && gconv.String(gconv.Map(gconv.Map(payload["content"])["media"])["id"]) != "urn:li:image:fixture-image" {
				t.Fatal("member image missing from post")
			}
			res := response(201, `{}`)
			res.Header.Set("x-restli-id", "urn:li:share:fixture")
			return res, nil
		case r.URL.Host == "api.linkedin.com" && r.URL.Path == "/rest/images" && r.Method == "POST":
			return response(200, `{"value":{"uploadUrl":"https://www.linkedin.com/dms-uploads/fixture","image":"urn:li:image:fixture-image"}}`), nil
		case r.URL.Host == "www.linkedin.com" && r.URL.Path == "/dms-uploads/fixture" && r.Method == "PUT":
			return response(201, ``), nil
		default:
			t.Fatalf("unexpected provider request: %s %s", r.Method, r.URL)
			return nil, nil
		}
	})}
	begin := func(target string, account int64) (g.Map, string) {
		auth, e := BeginAuthorization(ctx, p, "linkedin", g.Map{"target_type": target, "account_id": account})
		must(t, e)
		u, e := url.Parse(gconv.String(auth["url"]))
		must(t, e)
		if target == "person" && u.Query().Get("scope") != "openid profile w_member_social" {
			t.Fatal("member OAuth included enterprise permissions")
		}
		return auth, u.Query().Get("state")
	}
	first, state := begin("person", 0)
	second, secondState := begin("person", 0)
	if state == secondState || first["id"] == second["id"] {
		t.Fatal("parallel authorization tasks share state")
	}
	_, e = CompleteAuthorization(ctx, secondState, "second", "")
	must(t, e)
	_, e = CompleteAuthorization(ctx, state, "first", "")
	must(t, e)
	n, e := g.DB().Model("bm_social_accounts").Count()
	must(t, e)
	if n != 0 {
		t.Fatal("callback connected accounts without confirmation")
	}
	status, e := AuthorizationStatus(ctx, p, gconv.Int64(first["id"]))
	must(t, e)
	if status["status"] != "ready" || strings.Contains(encode(status), "fixture-token") {
		t.Fatal("pending status incorrect or exposes credentials")
	}
	stored, e := g.DB().Model("bm_social_oauth_states").Where("id", first["id"]).One()
	must(t, e)
	if strings.Contains(stored["encrypted_payload"].String(), "fixture-token") {
		t.Fatal("pending credentials stored unencrypted")
	}
	other := *p
	other.ID++
	if _, e = AuthorizationStatus(ctx, &other, gconv.Int64(first["id"])); e == nil {
		t.Fatal("another operator can read an authorization task")
	}
	if _, e = ConfirmAuthorization(ctx, &other, gconv.Int64(first["id"]), []string{"urn:li:person:member-one"}, false); e == nil {
		t.Fatal("another operator can claim an authorization")
	}
	if _, e = ConfirmAuthorization(ctx, p, gconv.Int64(first["id"]), []string{"urn:li:person:member-two"}, false); e == nil {
		t.Fatal("target from another task accepted")
	}
	_, e = ConfirmAuthorization(ctx, p, gconv.Int64(first["id"]), []string{"urn:li:person:member-one"}, false)
	must(t, e)
	_, e = ConfirmAuthorization(ctx, p, gconv.Int64(second["id"]), []string{"urn:li:person:member-two"}, false)
	must(t, e)
	account, e := g.DB().Model("bm_social_accounts").Where("external_id", "urn:li:person:member-one").One()
	must(t, e)
	if account["target_type"].String() != "person" || account["token"].String() == "fixture-token-first" || !gconv.Bool(object(account["capabilities"])["publish"]) || gconv.Bool(object(account["capabilities"])["metrics"]) {
		t.Fatal("member identity, token or capabilities incorrect")
	}
	stored, e = g.DB().Model("bm_social_oauth_states").Where("id", first["id"]).One()
	must(t, e)
	if stored["encrypted_payload"].String() != "" {
		t.Fatal("confirmed credentials left in temporary storage")
	}
	before := calls
	if _, e = CompleteAuthorization(ctx, state, "first", ""); e == nil || calls != before {
		t.Fatal("callback replay reached provider")
	}
	if _, e = ConfirmAuthorization(ctx, p, gconv.Int64(first["id"]), []string{"urn:li:person:member-one"}, false); e == nil {
		t.Fatal("confirmation replay succeeded")
	}
	reconnect, reconnectState := begin("person", account["id"].Int64())
	_, e = CompleteAuthorization(ctx, reconnectState, "second", "")
	must(t, e)
	if _, e = ConfirmAuthorization(ctx, p, gconv.Int64(reconnect["id"]), []string{"urn:li:person:member-two"}, true); e == nil {
		t.Fatal("reconnect overwrote a different member identity")
	}
	cancelled, cancelledState := begin("person", 0)
	must(t, CancelAuthorization(ctx, p, gconv.Int64(cancelled["id"])))
	before = calls
	if _, e = CompleteAuthorization(ctx, cancelledState, "first", ""); e == nil || calls != before {
		t.Fatal("cancelled OAuth reached provider")
	}
	expired, expiredState := begin("person", 0)
	_, e = CompleteAuthorization(ctx, expiredState, "first", "")
	must(t, e)
	_, e = g.DB().Model("bm_social_oauth_states").Where("id", expired["id"]).Data("expires_at", time.Now().Add(-time.Minute)).Update()
	must(t, e)
	if _, e = ConfirmAuthorization(ctx, p, gconv.Int64(expired["id"]), []string{"urn:li:person:member-one"}, false); e == nil {
		t.Fatal("expired task connected an account")
	}
	must(t, PurgeAuthorizationTasks(ctx))
	stored, e = g.DB().Model("bm_social_oauth_states").Where("id", expired["id"]).One()
	must(t, e)
	if stored["encrypted_payload"].String() != "" {
		t.Fatal("expired credentials retained")
	}
	if _, e = BeginAuthorization(ctx, p, "facebook", g.Map{"target_type": "person"}); e == nil {
		t.Fatal("Facebook personal OAuth enabled")
	}
	token, e := tokenFor(account)
	must(t, e)
	result, e := publish(ctx, "linkedin", token, account["external_id"].String(), PublishInput{Body: "Authorized member post"})
	must(t, e)
	if result.State != "submitted" {
		t.Fatal("member post fabricated a confirmed receipt")
	}
	memberImage = true
	_, e = publish(ctx, "linkedin", token, account["external_id"].String(), PublishInput{Body: "Authorized member image post", Images: []MediaFile{{Filename: "fixture.png", MIME: "image/png", Data: []byte("fixture")}}})
	must(t, e)
	t.Run("confirmation fences ownership and authorizer changes", func(t *testing.T) {
		auth, state := begin("person", 0)
		_, e := CompleteAuthorization(ctx, state, "first", "")
		must(t, e)
		_, e = g.DB().Model("bm_social_accounts").Where("id", account["id"]).Data("dept_id", p.DeptID+100).Update()
		must(t, e)
		if _, e = ConfirmAuthorization(ctx, p, gconv.Int64(auth["id"]), []string{"urn:li:person:member-one"}, true); e == nil {
			t.Fatal("confirmation took over another department's target")
		}
		_, e = g.DB().Model("bm_social_accounts").Where("id", account["id"]).Data(g.Map{"dept_id": p.DeptID, "provider_user": "prior-fixture-authorizer"}).Update()
		must(t, e)
		if _, e = ConfirmAuthorization(ctx, p, gconv.Int64(auth["id"]), []string{"urn:li:person:member-one"}, false); e == nil {
			t.Fatal("authorizing identity replaced without explicit confirmation")
		}
		_, e = ConfirmAuthorization(ctx, p, gconv.Int64(auth["id"]), []string{"urn:li:person:member-one"}, true)
		must(t, e)
	})
	t.Run("parallel confirmations keep one target record", func(t *testing.T) {
		ids := []int64{}
		for i := 0; i < 2; i++ {
			auth, state := begin("person", 0)
			_, e := CompleteAuthorization(ctx, state, "first", "")
			must(t, e)
			ids = append(ids, gconv.Int64(auth["id"]))
		}
		errs := make(chan error, len(ids))
		var wg sync.WaitGroup
		for _, id := range ids {
			wg.Add(1)
			go func(taskID int64) {
				defer wg.Done()
				_, e := ConfirmAuthorization(ctx, p, taskID, []string{"urn:li:person:member-one"}, false)
				errs <- e
			}(id)
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			must(t, err)
		}
		n, e := g.DB().Model("bm_social_accounts").Where("external_id", "urn:li:person:member-one").Count()
		must(t, e)
		if n != 1 {
			t.Fatal("parallel confirmations duplicated a publishing target")
		}
	})
	t.Run("application changes invalidate pending credentials", func(t *testing.T) {
		auth, state := begin("person", 0)
		_, e := CompleteAuthorization(ctx, state, "first", "")
		must(t, e)
		cfg.LinkedIn.Version = "202609"
		must(t, SaveSettings(ctx, p, cfg))
		if _, e = ConfirmAuthorization(ctx, p, gconv.Int64(auth["id"]), []string{"urn:li:person:member-one"}, false); e == nil {
			t.Fatal("pending credentials survived a changed application configuration")
		}
		must(t, CancelAuthorization(ctx, p, gconv.Int64(auth["id"])))
	})
}

func TestLinkedInMemberGrantedScopes(t *testing.T) {
	if _, e := discoverLinkedInMember(context.Background(), "fixture", []string{"openid", "profile", "w_organization_social"}); e == nil {
		t.Fatal("organization scope accepted for member publishing")
	}
	if accountCapabilities("linkedin", "page", []string{"w_member_social"})["publish"] != false {
		t.Fatal("member scope treated as organization authority")
	}
}
