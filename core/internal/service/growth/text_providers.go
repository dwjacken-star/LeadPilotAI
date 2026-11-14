package growth

import (
	"billionmail-core/internal/service/aimodels"
	"context"
	"encoding/json"
	"errors"
	"strings"

	"billionmail-core/internal/service/governance"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/gconv"
)

type TextProvider struct {
	Enabled bool     `json:"enabled"`
	BaseURL string   `json:"base_url"`
	APIKey  string   `json:"api_key,omitempty"`
	Models  []string `json:"models"`
}
type TextProviders struct {
	DefaultModel string       `json:"default_model"`
	OpenAI       TextProvider `json:"openai"`
	Qwen         TextProvider `json:"qwen"`
}

func textProviders(ctx context.Context) (TextProviders, error) {
	c := TextProviders{OpenAI: TextProvider{BaseURL: "https://api.openai.com/v1", Models: []string{}}, Qwen: TextProvider{BaseURL: "https://dashscope-intl.aliyuncs.com/compatible-mode/v1", Models: []string{}}}
	v, e := g.DB().Model("bm_options").Ctx(ctx).Where("name", "growth_text_providers").Value("value")
	if e == nil && !v.IsEmpty() {
		e = json.Unmarshal(v.Bytes(), &c)
	}
	return c, e
}
func (c *TextProviders) provider(name string) *TextProvider {
	if name == "openai" {
		return &c.OpenAI
	}
	if name == "qwen" {
		return &c.Qwen
	}
	return nil
}
func validateTextEndpoint(name, base string) error {
	u, e := providerURL(base)
	if e != nil {
		return e
	}
	if name == "openai" && (u.Hostname() != "api.openai.com" || u.Path != "/v1") {
		return errors.New("use the official OpenAI API endpoint")
	}
	if name == "qwen" && (!strings.HasSuffix(u.Hostname(), ".aliyuncs.com") || !strings.HasSuffix(u.Path, "/compatible-mode/v1")) {
		return errors.New("use your official Model Studio compatible-mode endpoint")
	}
	if name != "openai" && name != "qwen" {
		return errors.New("unknown text provider")
	}
	return nil
}
func TextProviderSettings(ctx context.Context, p *governance.Principal) (g.Map, error) {
	if p == nil || !p.Super {
		return nil, errors.New("administrator required")
	}
	c, e := textProviders(ctx)
	if e != nil {
		return nil, e
	}
	keys := g.Map{"openai": c.OpenAI.APIKey != "", "qwen": c.Qwen.APIKey != ""}
	c.OpenAI.APIKey = ""
	c.Qwen.APIKey = ""
	return g.Map{"config": c, "saved": keys}, nil
}
func SaveTextProviders(ctx context.Context, p *governance.Principal, c TextProviders) error {
	if aimodels.Active(ctx) {
		return errors.New("ai_registry_manage_in_settings")
	}
	if p == nil || !p.Super {
		return errors.New("administrator required")
	}
	old, e := textProviders(ctx)
	if e != nil {
		return e
	}
	for _, name := range []string{"openai", "qwen"} {
		x, prior := c.provider(name), old.provider(name)
		x.BaseURL = strings.TrimRight(strings.TrimSpace(x.BaseURL), "/")
		if e = validateTextEndpoint(name, x.BaseURL); e != nil {
			return e
		}
		if x.APIKey == "" {
			if prior.APIKey != "" && x.BaseURL != prior.BaseURL {
				return errors.New("enter an API key for the new endpoint")
			}
			x.APIKey = prior.APIKey
		} else {
			x.APIKey, e = protect(strings.TrimSpace(x.APIKey), false)
			if e != nil {
				return e
			}
		}
		if len(x.Models) > 50 {
			return errors.New("select at most 50 text models per provider")
		}
		seen := map[string]bool{}
		for _, model := range x.Models {
			if model == "" || len(model) > 200 || strings.ContainsAny(model, "\r\n\x00:") || seen[model] {
				return errors.New("invalid or duplicate text model")
			}
			seen[model] = true
		}
		if x.Enabled && (x.APIKey == "" || len(x.Models) == 0) {
			return errors.New("provider key and at least one model required")
		}
	}
	if c.DefaultModel != "" {
		parts := strings.SplitN(c.DefaultModel, "::", 2)
		if len(parts) != 2 {
			return errors.New("invalid default text model")
		}
		x := c.provider(parts[0])
		if x == nil || !x.Enabled || !containsTextModel(x.Models, parts[1]) {
			return errors.New("default model must belong to an enabled provider")
		}
	}
	_, e = g.DB().Model("bm_options").Ctx(ctx).Data(g.Map{"name": "growth_text_providers", "value": jsonText(c)}).OnConflict("name").Save()
	return e
}
func containsTextModel(models []string, model string) bool {
	for _, m := range models {
		if m == model {
			return true
		}
	}
	return false
}

func validateSelectedTextModel(ctx context.Context, model string) error {
	if aimodels.Active(ctx) {
		_, e := aimodels.Resolve(ctx, model, "text")
		return e
	}
	if strings.Contains(model, "::") {
		_, _, e := effectiveTextConfig(ctx, model)
		return e
	}
	c, e := config(ctx)
	if e != nil {
		return e
	}
	if model == "" || model == c.Model {
		return nil
	}
	key, e := protect(c.APIKey, true)
	if e != nil {
		return errors.New("AI credential unavailable")
	}
	models, e := fetchModels(ctx, c.BaseURL, key)
	if e != nil {
		return e
	}
	for _, m := range models {
		if m.Value == model {
			return nil
		}
	}
	return errors.New("select an available configured provider model")
}
func TextProviderModels(ctx context.Context, p *governance.Principal, name, base, key string) ([]ModelOption, error) {
	if p == nil || !p.Super {
		return nil, errors.New("administrator required")
	}
	if e := validateTextEndpoint(name, base); e != nil {
		return nil, e
	}
	c, e := textProviders(ctx)
	if e != nil {
		return nil, e
	}
	x := c.provider(name)
	if key == "" {
		if x.BaseURL != base {
			return nil, errors.New("enter an API key for the new endpoint")
		}
		if x.APIKey == "" {
			return nil, errors.New("API key required")
		}
		key, e = protect(x.APIKey, true)
		if e != nil {
			return nil, errors.New("AI credential unavailable")
		}
	}
	return fetchModels(ctx, base, key)
}
func effectiveTextConfig(ctx context.Context, model string) (AIConfig, string, error) {
	c, e := config(ctx)
	if e != nil {
		return c, "", e
	}
	if aimodels.Active(ctx) {
		resolved, err := aimodels.Resolve(ctx, model, "text")
		if err != nil {
			return c, "", err
		}
		c.Enabled = true
		c.BaseURL = resolved.Provider.BaseURL
		c.APIKey = resolved.Provider.APIKey
		c.Model = resolved.Model.ID
		c.ModelRef = resolved.Reference
		if resolved.Model.MaxOutput >= 256 && (c.MaxOutput < 256 || resolved.Model.MaxOutput < c.MaxOutput) {
			c.MaxOutput = resolved.Model.MaxOutput
		}
		return c, resolved.Provider.Format, nil
	}

	providers, e := textProviders(ctx)
	if e != nil {
		return c, "", e
	}
	if model == "" {
		model = providers.DefaultModel
		if model == "" {
			model = c.Model
		}
	}
	parts := strings.SplitN(model, "::", 2)
	if len(parts) == 1 {
		c.Model = model
		if !c.Enabled || c.APIKey == "" || c.Model == "" {
			return c, "", errors.New("AI configuration required")
		}
		return c, "", nil
	}
	x := providers.provider(parts[0])
	if x == nil || !x.Enabled || x.APIKey == "" || !containsTextModel(x.Models, parts[1]) {
		return c, "", errors.New("selected text provider or model is disabled or not configured")
	}
	if e = validateTextEndpoint(parts[0], x.BaseURL); e != nil {
		return c, "", e
	}
	c.Enabled, c.BaseURL, c.APIKey, c.Model = true, x.BaseURL, x.APIKey, parts[1]
	return c, parts[0], nil
}
func textModelOptions(ctx context.Context, c AIConfig) ([]ModelOption, string, string) {
	if aimodels.Active(ctx) {
		catalog, e := aimodels.Catalog(ctx, "text")
		if e != nil {
			return []ModelOption{}, "", e.Error()
		}
		options := []ModelOption{}
		for _, m := range catalog["models"].([]aimodels.Option) {
			options = append(options, ModelOption{m.Value, m.Label})
		}
		return options, gconv.String(catalog["model"]), ""
	}

	options := []ModelOption{}
	warning := ""
	defaultModel := c.Model
	if c.Model != "" {
		options = append(options, ModelOption{c.Model, c.Model})
	}
	if c.Enabled && c.APIKey != "" {
		if key, e := protect(c.APIKey, true); e == nil {
			if models, e := fetchModels(ctx, c.BaseURL, key); e == nil {
				for _, m := range models {
					if m.Value != c.Model {
						options = append(options, m)
					}
				}
			} else {
				warning = e.Error()
			}
		}
	}
	providers, e := textProviders(ctx)
	if e != nil {
		return options, defaultModel, e.Error()
	}
	if providers.DefaultModel != "" {
		defaultModel = providers.DefaultModel
	}
	for _, name := range []string{"openai", "qwen"} {
		x := providers.provider(name)
		if x.Enabled && x.APIKey != "" {
			for _, m := range x.Models {
				label := "ChatGPT / OpenAI"
				if name == "qwen" {
					label = "Qwen"
				}
				options = append(options, ModelOption{name + "::" + m, label + " · " + m})
			}
		}
	}
	return options, defaultModel, warning
}

// Adapt the Responses request to Model Studio's official Chat Completions API.
func qwenRequest(body g.Map, maxOutput int) (g.Map, error) {
	messages := []g.Map{{"role": "system", "content": gconv.String(body["instructions"]) + " Return only a JSON object conforming to this JSON Schema: " + jsonText(gconv.Map(body["text"])["format"])}}
	for _, input := range gconv.Maps(body["input"]) {
		content := []g.Map{}
		for _, item := range gconv.Maps(input["content"]) {
			switch item["type"] {
			case "input_text":
				content = append(content, g.Map{"type": "text", "text": item["text"]})
			case "input_image":
				content = append(content, g.Map{"type": "image_url", "image_url": g.Map{"url": item["image_url"]}})
			default:
				return nil, errors.New("use text or images with Qwen; select OpenAI or the existing provider for PDF file inputs")
			}
		}
		messages = append(messages, g.Map{"role": "user", "content": content})
	}
	return g.Map{"messages": messages, "response_format": g.Map{"type": "json_object"}, "max_tokens": maxOutput, "enable_thinking": false}, nil
}

func KeywordModelOptions(ctx context.Context, p *governance.Principal) (g.Map, error) {
	if !p.Has("intelligence:manage") && !p.Has("outreach:manage") {
		return nil, errors.New("AI permission required")
	}
	c, e := config(ctx)
	if e != nil {
		return nil, e
	}
	options, model, warning := textModelOptions(ctx, c)
	return g.Map{"models": options, "model": model, "warning": warning}, nil
}
