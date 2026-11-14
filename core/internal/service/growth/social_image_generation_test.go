package growth

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"billionmail-core/internal/service/governance"
	"billionmail-core/internal/testsupport"
	"github.com/gogf/gf/v2/frame/g"
)

func TestSocialImagePromptProvider(t *testing.T) {
	testsupport.Database(t)
	ctx := context.Background()
	p, e := governance.Load(ctx, 1)
	must(t, e)
	must(t, SaveSettings(ctx, p, AIConfig{Enabled: true, BaseURL: "https://image-provider.example/v1", Model: "fixture-text", APIKey: "fixture-generation-secret", DailyCalls: 1000, DailyTokens: 10000000, MaxOutput: 256}))
	old := providerClient
	t.Cleanup(func() { providerClient = old })
	imageData := []byte("fixture-generation-output")
	posts, reject := 0, false
	providerClient = &http.Client{Transport: modelTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "image-provider.example" || r.Header.Get("Authorization") != "Bearer fixture-generation-secret" {
			t.Fatal("image generation credential destination")
		}
		if r.Method == http.MethodGet {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"data":[{"id":"gpt-image-2.5-flare"}]}`))}, nil
		}
		posts++
		if r.URL.Path != "/v1/images/generations" || r.Header.Get("Content-Type") != "application/json" {
			t.Fatal("prompt generation did not use JSON Images API", r.URL)
		}
		var payload map[string]interface{}
		must(t, json.NewDecoder(r.Body).Decode(&payload))
		if payload["model"] != "gpt-image-2.5-flare" || payload["prompt"] != "Illustrated campaign background" || payload["n"] != float64(1) || payload["size"] != "1024x1024" || payload["image"] != nil {
			t.Fatal("prompt generation payload lost instructions or used a source image", payload)
		}
		if reject {
			return &http.Response{StatusCode: 403, Body: io.NopCloser(strings.NewReader("fixture-generation-secret private provider details"))}, nil
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(fmt.Sprintf(`{"data":[{"b64_json":%q}],"usage":{"input_tokens":70,"output_tokens":110}}`, base64.StdEncoding.EncodeToString(imageData))))}, nil
	})}
	generator, ok := SocialImageEditor.(ImageGenerator)
	if !ok {
		t.Fatal("configured Images provider does not implement prompt generation")
	}
	input := ImageGenerateInput{Model: "gpt-image-2.5-flare", Prompt: "Illustrated campaign background", Size: "1024x1024"}
	output, e := generator.Generate(ctx, p, input)
	must(t, e)
	if posts != 1 || !bytes.Equal(output, imageData) {
		t.Fatal("prompt generation response lost")
	}
	usage, e := g.DB().Model("bm_growth_usage").Where("purpose", "social_image_generate").Order("id DESC").One()
	must(t, e)
	if usage["input_tokens"].Int() != 70 || usage["output_tokens"].Int() != 110 {
		t.Fatal("generation usage was not recorded")
	}
	empty := input
	empty.Prompt = ""
	if _, e = generator.Generate(ctx, p, empty); e == nil || posts != 1 {
		t.Fatal("empty generation prompt called the provider")
	}
	reject = true
	if _, e = generator.Generate(ctx, p, input); e == nil || strings.Contains(e.Error(), "fixture-generation-secret") || strings.Contains(e.Error(), "private provider details") {
		t.Fatal("generation provider errors leaked private data", e)
	}
}
