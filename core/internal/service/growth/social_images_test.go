package growth

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"billionmail-core/internal/service/governance"
	"github.com/gogf/gf/v2/frame/g"
)

func exerciseSocialImageProvider(t *testing.T, ctx context.Context, p *governance.Principal) {
	t.Helper()
	old := providerClient
	defer func() { providerClient = old }()
	must(t, SaveSettings(ctx, p, AIConfig{Enabled: true, BaseURL: "https://image-provider.example/v1", Model: "fixture-text", APIKey: "fixture-image-secret", DailyCalls: 1000, DailyTokens: 10000000, MaxOutput: 256}))
	imageData := []byte("fixture-output-data")
	posts := 0
	providerClient = &http.Client{Transport: modelTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "image-provider.example" || r.Header.Get("Authorization") != "Bearer fixture-image-secret" {
			t.Fatal("image provider credential destination")
		}
		if r.Method == http.MethodGet {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"data":[{"id":"fixture-text"},{"id":"gpt-image-2.5-flare"},{"id":"gpt-image-1"}]}`))}, nil
		}
		posts++
		if r.URL.Path != "/v1/images/edits" {
			t.Fatal("wrong image endpoint", r.URL.Path)
		}
		must(t, r.ParseMultipartForm(1024*1024))
		defer r.MultipartForm.RemoveAll()
		if r.FormValue("model") != "gpt-image-2.5-flare" || r.FormValue("n") != "1" || r.FormValue("size") != "1024x1024" || r.FormValue("output_format") != "png" {
			t.Fatal("incorrect image edit settings")
		}
		file, header, e := r.FormFile("image[]")
		must(t, e)
		if header.Header.Get("Content-Type") != "image/png" {
			t.Fatal("image MIME type lost")
		}
		data, e := io.ReadAll(file)
		must(t, e)
		file.Close()
		if string(data) != "fixture-original" {
			t.Fatal("original image bytes lost")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(fmt.Sprintf(`{"data":[{"b64_json":%q}],"usage":{"input_tokens":90,"output_tokens":120}}`, base64.StdEncoding.EncodeToString(imageData))))}, nil
	})}
	options, e := SocialImageModels(ctx, p)
	must(t, e)
	if len(options["models"].([]ModelOption)) != 1 {
		t.Fatal("text or retired model offered for editing", options)
	}
	if e = ValidateSocialImageModel(ctx, p, "fixture-text"); e == nil {
		t.Fatal("text model accepted for image editing")
	}
	input := ImageEditInput{Model: "gpt-image-2.5-flare", Prompt: "Keep product unchanged", Size: "1024x1024", Filename: "product.png", MIME: "image/png", Image: []byte("fixture-original")}
	output, e := SocialImageEditor.Edit(ctx, p, input)
	must(t, e)
	if posts != 1 || !bytes.Equal(output, imageData) {
		t.Fatal("image edit response lost")
	}
	usage, e := g.DB().Model("bm_growth_usage").Where("purpose", "social_image_edit").Order("id DESC").One()
	must(t, e)
	if usage["input_tokens"].Int() != 90 || usage["output_tokens"].Int() != 120 {
		t.Fatal("image token usage was not recorded", usage)
	}
	providerClient = &http.Client{Transport: modelTransport(func(r *http.Request) (*http.Response, error) {
		if r.Method == http.MethodGet {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"data":[{"id":"gpt-image-2.5-flare"}]}`))}, nil
		}
		return &http.Response{StatusCode: 401, Body: io.NopCloser(strings.NewReader("fixture-image-secret private provider details"))}, nil
	})}
	_, e = SocialImageEditor.Edit(ctx, p, input)
	if e == nil || strings.Contains(e.Error(), "fixture-image-secret") {
		t.Fatal("provider errors leaked credentials", e)
	}
}
