package aimodels

import "testing"

func defaultFixture() Registry {
	return Registry{
		Defaults: map[string]string{"text": Reference("openai", "gpt-4o")},
		Providers: []Provider{
			{ID: "openai", Enabled: true, BaseURL: "https://openai.example/v1", APIKey: "fixture",
				Models: []Model{{ID: "gpt-4o", Enabled: true, Capabilities: []string{"text", "chat", "vision"}}}},
			{ID: "gateway", Enabled: true, BaseURL: "https://gateway.example/v1", APIKey: "fixture",
				Models: []Model{
					{ID: "qwen-disabled", Enabled: false, Capabilities: []string{"text", "chat"}},
					{ID: "Qwen3", Enabled: true, Capabilities: []string{"text", "chat"}},
					{ID: "qwen-vl", Enabled: true, Capabilities: []string{"vision"}},
					{ID: "qwen-image", Enabled: true, Capabilities: []string{"image"}},
				}},
		},
	}
}

func TestQwenDefaultsMatchCapabilitiesAndResolution(t *testing.T) {
	r := defaultFixture()
	initializeDefaults(&r)
	for kind, want := range map[string]string{
		"text": "Qwen3", "chat": "Qwen3", "vision": "qwen-vl", "image": "qwen-image",
	} {
		resolved, e := resolve(r, "", kind)
		if e != nil || resolved.Reference != Reference("gateway", want) || r.Defaults[kind] != resolved.Reference {
			t.Fatalf("%s: defaults=%q resolved=%+v error=%v", kind, r.Defaults[kind], resolved, e)
		}
	}
	if r.Defaults["embedding"] != "" {
		t.Fatal("selected a model without embedding capability")
	}
	explicit := Reference("openai", "gpt-4o")
	if resolved, e := resolve(r, explicit, "text"); e != nil || resolved.Reference != explicit {
		t.Fatal("explicit task model was overridden", resolved, e)
	}
	r.Defaults["text"] = explicit
	initializeDefaults(&r)
	if r.Defaults["text"] != explicit {
		t.Fatal("subsequent administrator choice was overridden")
	}
}

func TestDefaultFallbackExcludesUnavailableModels(t *testing.T) {
	for _, unavailable := range []string{"provider", "key", "endpoint", "models"} {
		t.Run(unavailable, func(t *testing.T) {
			r := defaultFixture()
			switch unavailable {
			case "provider":
				r.Providers[1].Enabled = false
			case "key":
				r.Providers[1].APIKey = ""
			case "endpoint":
				r.Providers[1].BaseURL = ""
			case "models":
				for i := range r.Providers[1].Models {
					r.Providers[1].Models[i].Enabled = false
				}
			}
			r.Defaults["text"] = Reference("gateway", "Qwen3")
			initializeDefaults(&r)
			if got := defaultModel(r, "text"); got != Reference("openai", "gpt-4o") {
				t.Fatal("unavailable default did not fall back", got)
			}
			if _, e := resolve(r, Reference("gateway", "Qwen3"), "text"); e == nil {
				t.Fatal("explicit unavailable model was accepted")
			}
			r.Providers[0].Enabled = false
			if got := defaultModel(r, "text"); got != "" {
				t.Fatal("empty catalog had a default", got)
			}
			if _, e := resolve(r, "", "text"); e == nil {
				t.Fatal("empty catalog resolved a default")
			}
		})
	}
}

func TestDefaultAliasesAreCanonical(t *testing.T) {
	r := defaultFixture()
	r.QwenDefaults = true
	r.Providers[1].ID = "text_qwen"
	r.Defaults["text"] = "qwen::Qwen3"
	initializeDefaults(&r)
	if got := r.Defaults["text"]; got != Reference("text_qwen", "Qwen3") {
		t.Fatal("legacy default was not canonicalized", got)
	}
}
