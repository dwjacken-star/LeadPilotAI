package aimodels

import (
	"billionmail-core/internal/service/aicredentials"
	"context"
	"encoding/json"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/gconv"
	"os"
	"path/filepath"
	"strings"
)

// ImportLegacy is idempotent and retains old configuration files/options for rollback.
func ImportLegacy(ctx context.Context) error {
	existing, ok, e := Read(ctx)
	if e != nil {
		return e
	}
	if ok {
		existing.Version++
		raw, e := json.Marshal(existing)
		if e != nil {
			return e
		}
		_, e = g.DB().Exec(ctx, `UPDATE bm_options SET value=$1 WHERE name=$2 AND (value::jsonb->>'version')::bigint=$3 AND COALESCE((value::jsonb->>'qwen_defaults')::boolean,false)=false`, string(raw), optionName, existing.Version-1)
		return e
	}
	r := Registry{Version: 1, Providers: []Provider{}, Defaults: map[string]string{}, TemplateAliases: map[string]string{}}
	raw, e := g.DB().Model("bm_options").Ctx(ctx).Where("name", "growth_ai_config").Value("value")
	if e != nil {
		return e
	}
	c := gconv.Map(raw)
	if raw != nil && !raw.IsEmpty() {
		_ = json.Unmarshal(raw.Bytes(), &c)
	}
	if base := gconv.String(c["base_url"]); base != "" {
		model := gconv.String(c["model"])
		models := []Model{}
		if model != "" {
			models = append(models, Model{model, model, true, InferCapabilities(model), func() int {
				v := gconv.Int(c["max_output"])
				if v < 256 {
					return 4000
				}
				return v
			}()})
		}
		p := Provider{ID: "growth_default", Name: "Legacy / " + providerLabel(base), BaseURL: base, APIKey: gconv.String(c["api_key"]), Enabled: gconv.Bool(c["enabled"]) && gconv.String(c["api_key"]) != "", Format: "responses", Models: models}
		r.Providers = append(r.Providers, p)
		if p.Enabled && len(models) > 0 {
			for _, kind := range []string{"text", "vision", "chat"} {
				if HasCapability(models[0], kind) {
					r.Defaults[kind] = Reference(p.ID, model)
				}
			}
		}
	}
	raw, e = g.DB().Model("bm_options").Ctx(ctx).Where("name", "growth_text_providers").Value("value")
	if e != nil {
		return e
	}
	var named map[string]json.RawMessage
	if !raw.IsEmpty() {
		_ = json.Unmarshal(raw.Bytes(), &named)
	}
	for _, name := range []string{"openai", "qwen"} {
		var x struct {
			Enabled bool     `json:"enabled"`
			BaseURL string   `json:"base_url"`
			APIKey  string   `json:"api_key"`
			Models  []string `json:"models"`
		}
		if v, ok := named[name]; ok {
			if e = json.Unmarshal(v, &x); e != nil {
				return e
			}
			models := []Model{}
			for _, m := range x.Models {
				models = append(models, Model{m, m, true, InferCapabilities(m), 4000})
			}
			format := "responses"
			label := "ChatGPT / OpenAI"
			if name == "qwen" {
				format = "chat_completions"
				label = "Qwen"
			}
			r.Providers = append(r.Providers, Provider{ID: "text_" + name, Name: label, BaseURL: x.BaseURL, APIKey: x.APIKey, Enabled: x.Enabled && x.APIKey != "", Format: format, Models: models})
		}
	}
	if v := named["default_model"]; v != nil {
		var selected string
		_ = json.Unmarshal(v, &selected)
		if parts := strings.SplitN(selected, "::", 2); len(parts) == 2 {
			r.Defaults["text"] = Reference("text_"+parts[0], parts[1])
		}
	}
	root := "../conf/supplier"
	if override := os.Getenv("LEADPILOT_SUPPLIER_ROOT"); override != "" {
		root = override
	}
	entries, _ := os.ReadDir(root)
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == "template" {
			continue
		}
		b, e := os.ReadFile(filepath.Join(root, entry.Name(), "config.json"))
		if e != nil {
			continue
		}
		var s struct {
			Title  string `json:"supplierTitle"`
			Name   string `json:"supplierName"`
			Base   string `json:"baseUrl"`
			Key    string `json:"apiKey"`
			Status bool   `json:"status"`
		}
		if json.Unmarshal(b, &s) != nil {
			continue
		}
		var old []struct {
			ID           string   `json:"modelId"`
			Name         string   `json:"title"`
			Enabled      bool     `json:"status"`
			Capabilities []string `json:"capability"`
			MaxOutput    int      `json:"max_tokens"`
		}
		b, _ = os.ReadFile(filepath.Join(root, entry.Name(), "models.json"))
		_ = json.Unmarshal(b, &old)
		models := []Model{}
		seen := map[string]bool{}
		for _, m := range old {
			if m.ID == "" || seen[m.ID] {
				continue
			}
			seen[m.ID] = true
			caps := InferCapabilities(m.ID)
			for _, cap := range m.Capabilities {
				if cap == "vision" && !HasCapability(Model{Capabilities: caps}, "vision") {
					caps = append(caps, "vision")
				}
			}
			name := m.Name
			if name == "" {
				name = m.ID
			}
			if m.MaxOutput < 256 {
				m.MaxOutput = 4000
			}
			models = append(models, Model{m.ID, name, m.Enabled, caps, m.MaxOutput})
		}
		id := "template_" + entry.Name()
		key := ""
		if s.Key != "" {
			key, e = aicredentials.Protect(s.Key, false)
			if e != nil {
				return e
			}
		}
		name := s.Title
		if name == "" {
			name = entry.Name()
		}
		r.Providers = append(r.Providers, Provider{ID: id, Name: name + " / Templates", BaseURL: strings.TrimRight(s.Base, "/"), APIKey: key, Enabled: s.Status && key != "" && s.Base != "", Format: "chat_completions", Models: models})
		r.TemplateAliases[entry.Name()] = id
	}
	initializeDefaults(&r)
	b, e := json.Marshal(r)
	if e != nil {
		return e
	}
	_, e = g.DB().Exec(ctx, `INSERT INTO bm_options(name,value) VALUES($1,$2) ON CONFLICT(name) DO NOTHING`, optionName, string(b))
	return e
}
func providerLabel(base string) string {
	lower := strings.ToLower(base)
	for _, name := range []string{"deepseek", "openai", "dashscope", "gemini", "anthropic"} {
		if strings.Contains(lower, name) {
			return name
		}
	}
	return "AI provider"
}
