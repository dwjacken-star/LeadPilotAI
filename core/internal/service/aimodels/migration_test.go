package aimodels_test

import (
	"billionmail-core/internal/service/aicredentials"
	"billionmail-core/internal/service/aimodels"
	"billionmail-core/internal/service/governance"
	"billionmail-core/internal/testsupport"
	"context"
	"encoding/json"
	"github.com/gogf/gf/v2/frame/g"
	"testing"
)

func TestRegistryDefaultUpgradePersistsOnce(t *testing.T) {
	testsupport.Database(t)
	ctx := context.Background()
	t.Setenv("MAPLEMAIL_GROWTH_DATA", t.TempDir())
	key, e := aicredentials.Protect("fixture-secret", false)
	if e != nil {
		t.Fatal(e)
	}
	r := aimodels.Registry{
		Version: 4, Defaults: map[string]string{"text": aimodels.Reference("openai", "gpt-4o")},
		Providers: []aimodels.Provider{
			{ID: "openai", Name: "OpenAI", Enabled: true, BaseURL: "https://openai.example/v1", APIKey: key, Format: "responses",
				Models: []aimodels.Model{{ID: "gpt-4o", Enabled: true, Capabilities: []string{"text", "chat"}}}},
			{ID: "gateway", Name: "Qwen", Enabled: true, BaseURL: "https://gateway.example/v1", APIKey: key, Format: "chat_completions",
				Models: []aimodels.Model{
					{ID: "qwen-disabled", Enabled: false, Capabilities: []string{"text", "chat"}},
					{ID: "qwen3", Enabled: true, Capabilities: []string{"text", "chat"}},
				}},
		},
	}
	raw, _ := json.Marshal(r)
	if _, e = g.DB().Exec(ctx, `INSERT INTO bm_options(name,value) VALUES('ai_model_registry',$1)`, string(raw)); e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 2; i++ {
		if e = aimodels.ImportLegacy(ctx); e != nil {
			t.Fatal(e)
		}
	}
	stored, e := g.DB().Model("bm_options").Where("name", "ai_model_registry").Value("value")
	if e != nil {
		t.Fatal(e)
	}
	if e = json.Unmarshal(stored.Bytes(), &r); e != nil {
		t.Fatal(e)
	}
	want := aimodels.Reference("gateway", "qwen3")
	if r.Version != 5 || !r.QwenDefaults || r.Defaults["text"] != want || r.Defaults["chat"] != want {
		t.Fatal("default upgrade was not persisted idempotently", r.Version, r.Defaults)
	}
	catalog, e := aimodels.Catalog(ctx, "text")
	if e != nil || len(catalog["models"].([]aimodels.Option)) != 2 || catalog["model"] != want {
		t.Fatal("catalog differs from active registry", catalog, e)
	}
	p, e := governance.Load(ctx, 1)
	if e != nil {
		t.Fatal(e)
	}
	settings, e := aimodels.Settings(ctx, p)
	if e != nil {
		t.Fatal(e)
	}
	settings.Defaults["text"] = aimodels.Reference("openai", "gpt-4o")
	if e = aimodels.Save(ctx, p, settings); e != nil {
		t.Fatal(e)
	}
	if e = aimodels.ImportLegacy(ctx); e != nil {
		t.Fatal(e)
	}
	current, _, e := aimodels.Read(ctx)
	if e != nil || current.Version != 6 || current.Defaults["text"] != settings.Defaults["text"] {
		t.Fatal("manual default was not preserved", current.Version, current.Defaults, e)
	}
}
