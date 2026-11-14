package growth

import (
	"billionmail-core/internal/service/aimodels"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strings"
	"time"

	"billionmail-core/internal/service/governance"
	"github.com/gogf/gf/v2/frame/g"
)

type ImageEditInput struct {
	Model, Prompt, Size, Filename, MIME string
	Image                               []byte
}
type ImageEditor interface {
	Models(context.Context, *governance.Principal) (g.Map, error)
	Edit(context.Context, *governance.Principal, ImageEditInput) ([]byte, error)
}
type ImageGenerateInput struct {
	Model, Prompt, Size string
}
type ImageGenerator interface {
	Generate(context.Context, *governance.Principal, ImageGenerateInput) ([]byte, error)
}

var SocialImageEditor ImageEditor = &socialImageProvider{}

type socialImageProvider struct{}

func SocialImageModels(ctx context.Context, p *governance.Principal) (g.Map, error) {
	if p == nil || !p.Has("social:content:edit") {
		return nil, errors.New("social AI permission required")
	}
	return SocialImageEditor.Models(ctx, p)
}
func (s *socialImageProvider) Models(ctx context.Context, p *governance.Principal) (g.Map, error) {
	if aimodels.Active(ctx) {
		r, e := aimodels.Catalog(ctx, "image")
		if e != nil {
			return nil, e
		}
		options := []ModelOption{}
		for _, m := range r["models"].([]aimodels.Option) {
			options = append(options, ModelOption{m.Value, m.Label})
		}
		r["models"] = options
		return r, nil
	}

	c, e := config(ctx)
	if e != nil {
		return nil, e
	}
	options := []ModelOption{}
	warning := ""
	if c.Enabled && c.APIKey != "" {
		key, e := protect(c.APIKey, true)
		if e != nil {
			return nil, errors.New("cannot read saved API credential")
		}
		options, e = fetchModelsByKind(ctx, c.BaseURL, key, true)
		if e != nil {
			warning = e.Error()
			options = []ModelOption{}
		}
	}
	return g.Map{"enabled": c.Enabled && c.APIKey != "", "models": options, "warning": warning}, nil
}

func ValidateSocialImageModel(ctx context.Context, p *governance.Principal, model string) error {
	result, e := SocialImageModels(ctx, p)
	if e != nil {
		return e
	}
	if !result["enabled"].(bool) {
		return errors.New("image AI is not configured or enabled")
	}
	for _, option := range result["models"].([]ModelOption) {
		if option.Value == model {
			return nil
		}
	}
	return errors.New("select an available image model from the configured AI provider")
}

func (s *socialImageProvider) Edit(ctx context.Context, p *governance.Principal, in ImageEditInput) ([]byte, error) {
	if len(in.Image) == 0 || len(in.Image) > 8*1024*1024 || strings.TrimSpace(in.Prompt) == "" {
		return nil, errors.New("a source image and editing instructions are required")
	}
	return s.process(ctx, p, in, false)
}

func (s *socialImageProvider) Generate(ctx context.Context, p *governance.Principal, in ImageGenerateInput) ([]byte, error) {
	if strings.TrimSpace(in.Prompt) == "" {
		return nil, errors.New("image generation instructions are required")
	}
	return s.process(ctx, p, ImageEditInput{Model: in.Model, Prompt: in.Prompt, Size: in.Size}, true)
}

func (s *socialImageProvider) process(ctx context.Context, p *governance.Principal, in ImageEditInput, generate bool) ([]byte, error) {
	if in.Size != "1024x1024" && in.Size != "1536x1024" && in.Size != "1024x1536" {
		return nil, errors.New("select a supported image size")
	}
	if e := ValidateSocialImageModel(ctx, p, in.Model); e != nil {
		return nil, e
	}
	c, e := config(ctx)
	if e != nil {
		return nil, e
	}
	if aimodels.Active(ctx) {
		resolved, err := aimodels.Resolve(ctx, in.Model, "image")
		if err != nil {
			return nil, err
		}
		c.Enabled = true
		c.BaseURL = resolved.Provider.BaseURL
		c.APIKey = resolved.Provider.APIKey
		c.Model = resolved.Model.ID
		c.ModelRef = resolved.Reference
	}
	if _, e = providerURL(c.BaseURL); e != nil {
		return nil, e
	}
	key, e := protect(c.APIKey, true)
	if e != nil {
		return nil, errors.New("cannot read saved API credential")
	}
	if !aimodels.Active(ctx) {
		c.Model = in.Model
	}
	c.MaxOutput = 8192
	purpose, endpoint := "social_image_edit", "/images/edits"
	if generate {
		purpose, endpoint = "social_image_generate", "/images/generations"
	}
	usageID, e := reserveAI(ctx, p, c, purpose, len(in.Prompt)*2+8192)
	if e != nil {
		return nil, e
	}
	var body bytes.Buffer
	contentType := "application/json"
	if generate {
		if e = json.NewEncoder(&body).Encode(g.Map{"model": c.Model, "prompt": in.Prompt, "size": in.Size, "n": 1, "output_format": "png"}); e != nil {
			return nil, e
		}
	} else {
		writer := multipart.NewWriter(&body)
		for k, v := range map[string]string{"model": c.Model, "prompt": in.Prompt, "size": in.Size, "n": "1", "output_format": "png"} {
			if e = writer.WriteField(k, v); e != nil {
				return nil, e
			}
		}
		if in.MIME == "" {
			in.MIME = http.DetectContentType(in.Image)
		}
		headers := textproto.MIMEHeader{}
		headers.Set("Content-Disposition", mime.FormatMediaType("form-data", map[string]string{"name": "image[]", "filename": in.Filename}))
		headers.Set("Content-Type", in.MIME)
		part, e := writer.CreatePart(headers)
		if e != nil {
			return nil, e
		}
		if _, e = part.Write(in.Image); e != nil {
			return nil, e
		}
		if e = writer.Close(); e != nil {
			return nil, e
		}
		contentType = writer.FormDataContentType()
	}
	ctx, cancel := context.WithTimeout(ctx, 180*time.Second)
	defer cancel()
	req, e := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.BaseURL, "/")+endpoint, &body)
	if e != nil {
		return nil, errors.New("invalid image AI request")
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", contentType)
	client := *providerClient
	client.Timeout = 180 * time.Second
	response, e := client.Do(req)
	if e != nil {
		return nil, errors.New("AI image request failed; check provider connectivity before retrying")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("AI image processing returned HTTP %d; verify model access and Images API support", response.StatusCode)
	}
	raw, e := io.ReadAll(io.LimitReader(response.Body, 32*1024*1024+1))
	if e != nil || len(raw) > 32*1024*1024 {
		return nil, errors.New("invalid or oversized AI image response")
	}
	var result struct {
		Data []struct {
			Base64 string `json:"b64_json"`
		} `json:"data"`
		Usage struct {
			Input  int64 `json:"input_tokens"`
			Output int64 `json:"output_tokens"`
		} `json:"usage"`
	}
	if json.Unmarshal(raw, &result) != nil || len(result.Data) != 1 || result.Data[0].Base64 == "" {
		return nil, errors.New("AI provider did not return one base64 image; use an Images API compatible model")
	}
	image, e := base64.StdEncoding.DecodeString(result.Data[0].Base64)
	if e != nil || len(image) == 0 || len(image) > 24*1024*1024 {
		return nil, errors.New("invalid AI image data")
	}
	if result.Usage.Input > 0 && result.Usage.Output > 0 {
		_, _ = g.DB().Model("bm_growth_usage").Ctx(ctx).Where("id", usageID).Data(g.Map{"input_tokens": result.Usage.Input, "output_tokens": result.Usage.Output}).Update()
	}
	return image, nil
}
