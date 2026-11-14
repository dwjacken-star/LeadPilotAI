package growth

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"billionmail-core/internal/service/aimodels"
	"billionmail-core/internal/service/governance"
	"github.com/gogf/gf/v2/frame/g"
)

func exerciseRegistryRoutesSelectedPlatformAndModel(t *testing.T,ctx context.Context,p *governance.Principal) {
 t.Setenv("LEADPILOT_SUPPLIER_ROOT", t.TempDir())
	must(t, aimodels.ImportLegacy(ctx))
	r, e := aimodels.Settings(ctx, p)
	must(t, e)
	r.Providers = []aimodels.Provider{
		{ID: "alpha", Name: "Alpha", BaseURL: "https://alpha.example/v1", APIKey: "alpha-fixture", Enabled: true, Format: "responses", Models: []aimodels.Model{{ID: "same-model", Name: "Alpha model", Enabled: true, Capabilities: []string{"text"}, MaxOutput: 1000}}},
		{ID: "beta", Name: "Beta", BaseURL: "https://beta.example/v1", APIKey: "beta-fixture", Enabled: true, Format: "chat_completions", Models: []aimodels.Model{{ID: "same-model", Name: "Beta model", Enabled: true, Capabilities: []string{"text"}, MaxOutput: 600}}},
	}
	r.Defaults = map[string]string{"text": "ai::alpha::same-model"}
	must(t, aimodels.Save(ctx, p, r))
	old := providerClient
	defer func() { providerClient = old }()
	called := 0
	providerClient = &http.Client{Transport: modelTransport(func(req *http.Request) (*http.Response, error) {
		called++
		if req.URL.String() != "https://beta.example/v1/chat/completions" || req.Header.Get("Authorization") != "Bearer beta-fixture" {
			t.Fatal("selected platform ignored", req.URL)
		}
		var body g.Map
		must(t, json.NewDecoder(req.Body).Decode(&body))
		if body["model"] != "same-model" || body["max_tokens"] != float64(600) {
			t.Fatal("model/output bound ignored", body)
		}
		if _, exists := body["enable_thinking"]; exists {
			t.Fatal("provider-specific parameter leaked")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"choices":[{"message":{"content":"{\"answer\":\"ok\"}"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":4}}`)), Header: http.Header{}}, nil
	})}
	var out struct {
		Answer string `json:"answer"`
	}
	must(t, (&responsesProvider{}).Extract(ctx, p, "registry_fixture", AIInput{Model: "ai::beta::same-model", Instruction: "Return fixture", Data: g.Map{}, Schema: schema(g.Map{"answer": strSchema()})}, &out))
	if out.Answer != "ok" || called != 1 {
		t.Fatal("structured response not parsed")
	}
	usage, e := g.DB().Model("bm_growth_usage").Where("purpose", "registry_fixture").One()
	must(t, e)
	if usage["model"].String() != "ai::beta::same-model" {
		t.Fatal("usage lost selected platform")
	}
	if e = (&responsesProvider{}).Extract(ctx, p, "registry_fixture", AIInput{Model: "ai::missing::same-model"}, &out); e == nil || called != 1 {
		t.Fatal("unknown selection fell back silently")
	}
}
