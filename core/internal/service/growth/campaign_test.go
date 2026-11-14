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
)

func exerciseCampaignAuthor(t *testing.T, ctx context.Context, p *governance.Principal) {
	t.Helper()
	previous, _ := g.DB().Model("bm_options").Where("name", "growth_ai_config").One()
	defer func() {
		if len(previous) == 0 {
			g.DB().Model("bm_options").Where("name", "growth_ai_config").Delete()
		} else {
			g.DB().Model("bm_options").Where("name", "growth_ai_config").Data("value", previous["value"].String()).Update()
		}
	}()
	oldProvider, oldClient := IntelligenceProvider, providerClient
	defer func() { IntelligenceProvider = oldProvider; providerClient = oldClient }()
	IntelligenceProvider = &responsesProvider{}
	must(t, SaveSettings(ctx, p, AIConfig{Enabled: true, BaseURL: "https://fixture.example/v1", Model: "configured", APIKey: "secret-fixture", DailyCalls: 1000, DailyTokens: 1000000, MaxOutput: 8000}))
	calls := 0
	providerClient = &http.Client{Transport: modelTransport(func(req *http.Request) (*http.Response, error) {
		calls++
		if req.URL.Host != "fixture.example" || req.Header.Get("Authorization") != "Bearer secret-fixture" {
			t.Fatal("saved credential routing changed")
		}
		if req.Method == "GET" {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"data":[{"id":"configured"},{"id":"alternative"}]}`))}, nil
		}
		var body g.Map
		raw, _ := io.ReadAll(req.Body)
		must(t, json.Unmarshal(raw, &body))
		if body["model"] != "alternative" || body["max_output_tokens"] != float64(4000) {
			t.Fatal("selected model or bounded output ignored", body["model"], body["max_output_tokens"])
		}
		text := jsonText(g.Map{"full_name": "BESTMAPLE Sales", "subject": "A/C parts for importers", "paragraphs": []string{"Dear buyer,", "Literal <script> is text.\nSecond line."}, "language": "en"})
		response := jsonText(g.Map{"status": "completed", "output": []g.Map{{"content": []g.Map{{"type": "output_text", "text": text}}}}, "usage": g.Map{"input_tokens": 120, "output_tokens": 70}})
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(response))}, nil
	})}
	denied := &governance.Principal{ID: 999}
	if _, e := CampaignOptions(ctx, denied); e == nil {
		t.Fatal("unauthorized options allowed")
	}
	in := CampaignBrief{Requirement: "Introduce BESTMAPLE A/C parts to importers", Country: "US", Language: "en", Model: "alternative", Target: "all"}
	if _, e := ComposeCampaign(ctx, denied, in); e == nil {
		t.Fatal("unauthorized authoring allowed")
	}
	if calls != 0 {
		t.Fatal("authorization rejection invoked provider")
	}
	options, e := CampaignOptions(ctx, p)
	must(t, e)
	if _, exists := options["api_key"]; exists || len(options["models"].([]ModelOption)) != 2 {
		t.Fatal("options leaked credential or missing models")
	}
	before, _ := g.DB().Model("email_tasks").Count()
	templatesBefore, _ := g.DB().Model("email_templates").Count()
	result, e := ComposeCampaign(ctx, p, in)
	must(t, e)
	content := result["html_content"].(string)
	if result["model"] != "alternative" || !strings.Contains(content, "&lt;script&gt;") || !strings.Contains(content, "<br>Second line.") || strings.Contains(content, "<script>") {
		t.Fatal("draft formatting or selected model incorrect", result)
	}
	after, _ := g.DB().Model("email_tasks").Count()
	templatesAfter, _ := g.DB().Model("email_templates").Count()
	if before != after || templatesBefore != templatesAfter {
		t.Fatal("generation created a campaign or template")
	}
	usage, e := g.DB().Model("bm_growth_usage").Where("purpose", "campaign_compose").OrderDesc("id").One()
	must(t, e)
	if usage["model"].String() != "alternative" || usage["input_tokens"].Int() != 120 || usage["output_tokens"].Int() != 70 {
		t.Fatal("model usage not recorded", usage)
	}
	for _, bad := range []CampaignBrief{{Requirement: "", Country: "US", Language: "en", Target: "all"}, {Requirement: "test", Country: "USA", Language: "en", Target: "all"}, {Requirement: "test", Country: "US", Language: "<script>", Target: "all"}, {Requirement: "test", Country: "US", Language: "en", Target: "send"}} {
		n := calls
		if _, e = ComposeCampaign(ctx, p, bad); e == nil || calls != n {
			t.Fatal("invalid brief reached AI", bad, e)
		}
	}
	in.Model = "not-available"
	if _, e = ComposeCampaign(ctx, p, in); e == nil {
		t.Fatal("unsupported arbitrary model accepted")
	}
	current, e := config(ctx)
	must(t, e)
	current.DailyTokens = 100
	current.APIKey = ""
	must(t, SaveSettings(ctx, p, current))
	in.Model = "configured"
	n := calls
	if _, e = ComposeCampaign(ctx, p, in); e == nil || calls != n {
		t.Fatal("AI budget bypassed", e)
	}
}
