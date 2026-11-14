package aimodels_test

import (
	"billionmail-core/internal/service/aicredentials"
	"billionmail-core/internal/service/aimodels"
	"billionmail-core/internal/service/governance"
	"billionmail-core/internal/testsupport"
	"context"
	"encoding/json"
	"github.com/gogf/gf/v2/frame/g"
	"strings"
	"testing"
)

func TestRegistryLifecycle(t *testing.T) {
	testsupport.Database(t)
	ctx := context.Background()
	t.Setenv("MAPLEMAIL_GROWTH_DATA", t.TempDir())
	t.Setenv("LEADPILOT_SUPPLIER_ROOT", t.TempDir())
	key, e := aicredentials.Protect("fixture-secret", false)
	if e != nil {
		t.Fatal(e)
	}
	raw, _ := json.Marshal(g.Map{"enabled": true, "base_url": "https://ai.example/v1", "model": "fixture-model", "api_key": key, "max_output": 1024})
	if _, e = g.DB().Exec(ctx, `INSERT INTO bm_options(name,value) VALUES('growth_ai_config',$1)`, string(raw)); e != nil {
		t.Fatal(e)
	}
	if e = aimodels.ImportLegacy(ctx); e != nil {
		t.Fatal(e)
	}
	first, _, _ := aimodels.Read(ctx)
	if e = aimodels.ImportLegacy(ctx); e != nil {
		t.Fatal(e)
	}
	second, _, _ := aimodels.Read(ctx)
	if first.Version != second.Version || len(second.Providers) != 1 {
		t.Fatal("nonidempotent migration")
	}
	p, e := governance.Load(ctx, 1)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = aimodels.Settings(ctx, &governance.Principal{}); e == nil {
		t.Fatal("nonadmin read")
	}
	safe, e := aimodels.Settings(ctx, p)
	if e != nil {
		t.Fatal(e)
	}
	b, _ := json.Marshal(safe)
	if strings.Contains(string(b), key) || strings.Contains(string(b), "fixture-secret") || !safe.Providers[0].HasKey {
		t.Fatal("credential leak")
	}
	r, e := aimodels.Resolve(ctx, "fixture-model", "text")
	if e != nil || r.Reference != "ai::growth_default::fixture-model" {
		t.Fatal("legacy resolution", e)
	}
	if _, e = aimodels.Resolve(ctx, r.Reference, "image"); e == nil {
		t.Fatal("incompatible model accepted")
	}
	if e = aimodels.Save(ctx, p, safe); e != nil {
		t.Fatal(e)
	}
	saved, _, _ := aimodels.Read(ctx)
	if saved.Providers[0].APIKey != key {
		t.Fatal("blank credential erased key")
	}
	if e = aimodels.Save(ctx, p, safe); e == nil {
		t.Fatal("stale update accepted")
	}
	current, _ := aimodels.Settings(ctx, p)
	current.Providers[0].BaseURL = "https://other.example/v1"
	if e = aimodels.Save(ctx, p, current); e == nil {
		t.Fatal("moved secret to other endpoint")
	}
	current, _ = aimodels.Settings(ctx, p)
	current.Providers[0].Enabled = false
	current.Defaults = map[string]string{}
	if e = aimodels.Save(ctx, p, current); e != nil {
		t.Fatal(e)
	}
	if _, e = aimodels.Resolve(ctx, r.Reference, "text"); e == nil {
		t.Fatal("disabled provider used")
	}
	catalog, e := aimodels.Catalog(ctx, "text")
	if e != nil {
		t.Fatal(e)
	}
	b, _ = json.Marshal(catalog)
	if strings.Contains(string(b), key) || strings.Contains(string(b), "api_key") {
		t.Fatal("catalog leaked credential")
	}
}
