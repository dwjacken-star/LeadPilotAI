package growth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"billionmail-core/internal/service/governance"
)

type ModelOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

func providerURL(base string) (*url.URL, error) {
	u, e := url.Parse(strings.TrimSpace(base))
	if e != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("HTTPS provider base URL required")
	}
	return u, nil
}

// Models uses the provider's discovery endpoint; it neither enables AI nor runs inference.
func Models(ctx context.Context, p *governance.Principal, base, key string) ([]ModelOption, error) {
	if p == nil || !p.Super {
		return nil, errors.New("administrator required")
	}
	c, e := config(ctx)
	if e != nil {
		return nil, e
	}
	if base == "" {
		base = c.BaseURL
	}
	if _, e = providerURL(base); e != nil {
		return nil, e
	}
	if key == "" && c.APIKey != "" {
		// A saved secret must never be forwarded to a newly entered provider URL.
		if strings.TrimRight(strings.TrimSpace(base), "/") != strings.TrimRight(c.BaseURL, "/") {
			return nil, errors.New("enter an API key for the new provider")
		}
		key, e = protect(c.APIKey, true)
		if e != nil {
			return nil, errors.New("cannot read saved API credential")
		}
	}
	if strings.TrimSpace(key) == "" {
		return []ModelOption{}, nil
	}
	return fetchModels(ctx, base, key)
}

func fetchModels(ctx context.Context, base, key string) ([]ModelOption, error) {
	return fetchModelsByKind(ctx, base, key, false)
}
func fetchModelsByKind(ctx context.Context, base, key string, images bool) ([]ModelOption, error) {
	if _, e := providerURL(base); e != nil {
		return nil, e
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, e := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(strings.TrimSpace(base), "/")+"/models", nil)
	if e != nil {
		return nil, errors.New("invalid model discovery request")
	}
	req.Header.Set("Authorization", "Bearer "+key)
	response, e := providerClient.Do(req)
	if e != nil {
		return nil, errors.New("model discovery failed; check provider URL and connectivity")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("model discovery returned HTTP %d; check API access", response.StatusCode)
	}
	body, e := io.ReadAll(io.LimitReader(response.Body, 2*1024*1024+1))
	if e != nil || len(body) > 2*1024*1024 {
		return nil, errors.New("invalid model discovery response")
	}
	var result struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &result) != nil {
		return nil, errors.New("invalid model discovery response")
	}
	options := []ModelOption{}
	seen := map[string]bool{}
	for _, model := range result.Data {
		id := strings.TrimSpace(model.ID)
		lower := strings.ToLower(id)
		if id == "" || len(id) > 200 || strings.ContainsAny(id, "\r\n\x00") || seen[id] {
			continue
		}
		unsupported := false
		if images {
			if !strings.Contains(lower, "image") || lower == "gpt-image-1" {
				continue
			}
			seen[id] = true
			options = append(options, ModelOption{Value: id, Label: id})
			continue
		}
		for _, kind := range []string{"embedding", "moderation", "image", "audio", "realtime", "transcrib", "whisper", "tts", "dall-e", "live"} {
			if strings.Contains(lower, kind) {
				unsupported = true
				break
			}
		}
		if unsupported {
			continue
		}
		seen[id] = true
		options = append(options, ModelOption{Value: id, Label: id})
	}
	sort.Slice(options, func(i, j int) bool { return options[i].Value < options[j].Value })
	return options, nil
}
