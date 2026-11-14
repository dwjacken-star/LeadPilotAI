package growth

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"billionmail-core/internal/service/governance"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/gconv"
)

type textProviderTransport func(*http.Request) (*http.Response, error)

func (f textProviderTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func exerciseTextProviders(t *testing.T) {
	ctx := context.Background()
	p, e := governance.Load(ctx, 1)
	must(t, e)
	base, e := config(ctx)
	must(t, e)
	base.DailyCalls = 100000
	base.DailyTokens = 100000000
	base.MaxOutput = 4000
	_, e = g.DB().Model("bm_options").Data(g.Map{"name": "growth_ai_config", "value": jsonText(base)}).OnConflict("name").Save()
	must(t, e)
	oldClient := providerClient
	oldAI := IntelligenceProvider
	t.Cleanup(func() { providerClient = oldClient; IntelligenceProvider = oldAI })
	c := TextProviders{DefaultModel: "qwen::qwen-turbo", OpenAI: TextProvider{Enabled: true, BaseURL: "https://api.openai.com/v1", APIKey: "openai-test-secret", Models: []string{"gpt-4o-mini"}}, Qwen: TextProvider{Enabled: true, BaseURL: "https://dashscope-intl.aliyuncs.com/compatible-mode/v1", APIKey: "qwen-test-secret", Models: []string{"qwen-turbo"}}}
	must(t, SaveTextProviders(ctx, p, c))
	safe, e := TextProviderSettings(ctx, p)
	must(t, e)
	if strings.Contains(jsonText(safe), "test-secret") {
		t.Fatal("provider key leaked")
	}
	c.OpenAI.APIKey = ""
	c.Qwen.APIKey = ""
	must(t, SaveTextProviders(ctx, p, c))
	invalid := c
	invalid.OpenAI.BaseURL = "https://wrong.example/v1"
	if SaveTextProviders(ctx, p, invalid) == nil {
		t.Fatal("saved credential could move to another host")
	}
	if SaveTextProviders(ctx, &governance.Principal{ID: p.ID, DeptID: p.DeptID}, c) == nil {
		t.Fatal("non-administrator changed provider keys")
	}
	calls := map[string]int{}
	providerClient = &http.Client{Transport: textProviderTransport(func(r *http.Request) (*http.Response, error) {
		raw, _ := io.ReadAll(r.Body)
		body := g.Map{}
		must(t, json.Unmarshal(raw, &body))
		response := ""
		switch r.URL.Host {
		case "api.openai.com":
			if r.URL.Path != "/v1/responses" || r.Header.Get("Authorization") != "Bearer openai-test-secret" || body["store"] != false || body["model"] != "gpt-4o-mini" {
				t.Fatal("incorrect OpenAI request", r.URL.Path)
			}
			calls["openai"]++
			response = `{"status":"completed","output":[{"content":[{"type":"output_text","text":"{\"ok\":true}"}]}],"usage":{"input_tokens":12,"output_tokens":5}}`
		case "dashscope-intl.aliyuncs.com":
			if !strings.HasSuffix(r.URL.Path, "/chat/completions") || r.Header.Get("Authorization") != "Bearer qwen-test-secret" || body["enable_thinking"] != false || gconv.Map(body["response_format"])["type"] != "json_object" || body["model"] != "qwen-turbo" {
				t.Fatal("incorrect Qwen request")
			}
			calls["qwen"]++
			text := `{"ok":true}`
			if strings.Contains(string(raw), "Expand public B2B") {
				text = `{"keywords":["buyer wanted","Buyer Wanted","purchase request"]}`
			}
			response = jsonText(g.Map{"choices": []g.Map{{"finish_reason": "stop", "message": g.Map{"content": text}}}, "usage": g.Map{"prompt_tokens": 15, "completion_tokens": 7}})
		default:
			t.Fatal("credential sent to unexpected host", r.URL.Host)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(response))}, nil
	})}
	IntelligenceProvider = &responsesProvider{}
	for _, model := range []string{"qwen::qwen-turbo", "openai::gpt-4o-mini"} {
		var out struct {
			OK bool `json:"ok"`
		}
		must(t, IntelligenceProvider.Extract(ctx, p, "provider_test", AIInput{Model: model, MaxOutput: 256, Instruction: "Return JSON with ok true", Data: g.Map{}, Schema: schema(g.Map{"ok": g.Map{"type": "boolean"}})}, &out))
		if !out.OK {
			t.Fatal("model result not decoded")
		}
	}
	if calls["qwen"] != 1 || calls["openai"] != 1 {
		t.Fatal("provider selection was ignored")
	}
	keywords, e := ExpandIntentKeywords(ctx, p, g.Map{"seed": "procurement", "model": "qwen::qwen-turbo", "language": "en"})
	must(t, e)
	if len(gconv.Strings(keywords["keywords"])) != 2 {
		t.Fatal("keyword suggestions were not deduplicated")
	}
	c.Qwen.Enabled = false
	c.DefaultModel = ""
	must(t, SaveTextProviders(ctx, p, c))
	if _, _, e = effectiveTextConfig(ctx, "qwen::qwen-turbo"); e == nil {
		t.Fatal("disabled provider silently fell back")
	}
	company, e := SaveCompany(ctx, p, 0, g.Map{"name": "Deletion test company"})
	must(t, e)
	plan, e := g.DB().Model("bm_growth_plans").Data(g.Map{"company_id": company, "dept_id": p.DeptID, "owner_id": p.ID, "name": "Deleted plan", "config": "{}", "active": true}).InsertAndGetId()
	must(t, e)
	reader := &governance.Principal{ID: p.ID, DeptID: p.DeptID, Roles: []governance.Role{{Scope: "all", Permissions: []string{"outreach:list"}}}}
	if DeleteRecord(ctx, reader, "growth", "plans", plan) == nil {
		t.Fatal("read-only principal deleted task")
	}
	must(t, DeleteRecord(ctx, p, "growth", "plans", plan))
	list, e := List(ctx, p, "plans", 0, 1, "Deleted plan", "")
	must(t, e)
	if gconv.Int(list["total"]) != 0 {
		t.Fatal("deleted plan is visible")
	}
	if _, e = scoped(ctx, p, "bm_growth_plans", plan, "outreach:run"); e == nil {
		t.Fatal("deleted plan may run again")
	}
}
