package growth

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"billionmail-core/internal/service/governance"
)

type modelTransport func(*http.Request) (*http.Response, error)

func (f modelTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestModelDiscoveryIsBoundedReadOnlyAndRedactsErrors(t *testing.T) {
	old := providerClient
	defer func() { providerClient = old }()
	providerClient = &http.Client{Transport: modelTransport(func(r *http.Request) (*http.Response, error) {
		if r.Method != "GET" || r.URL.String() != "https://provider.example/v1/models" || r.Header.Get("Authorization") != "Bearer fixture-secret" {
			t.Fatal("discovery request", r.Method, r.URL)
		}

		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"data":[{"id":"text-generation-custom"},{"id":"gpt-6-luna"},{"id":"text-generation-custom"},{"id":"text-embedding-3-small"},{"id":"gpt-image-2"},{"id":"whisper"},{"id":"bad\r\nheader"}]}`))}, nil
	})}
	options, e := fetchModels(context.Background(), "https://provider.example/v1/", "fixture-secret")
	if e != nil || len(options) != 2 || options[0].Value != "gpt-6-luna" || options[1].Value != "text-generation-custom" {
		t.Fatal(options, e)
	}
	if _, e = Models(context.Background(), &governance.Principal{Super: false}, "https://provider.example/v1", "secret"); e == nil {
		t.Fatal("non-admin model discovery allowed")
	}
	for _, base := range []string{"http://provider.example", "https://user:password@provider.example", "https://provider.example?key=secret", "https://provider.example#fragment"} {
		if _, e = fetchModels(context.Background(), base, "secret"); e == nil {
			t.Fatal("invalid provider URL accepted", base)
		}
	}
	providerClient = &http.Client{Transport: modelTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 401, Body: io.NopCloser(strings.NewReader("fixture-secret provider confidential detail"))}, nil
	})}
	if _, e = fetchModels(context.Background(), "https://provider.example/v1", "fixture-secret"); e == nil || strings.Contains(e.Error(), "secret") || strings.Contains(e.Error(), "confidential") {
		t.Fatal("provider failure or secret disclosure", e)
	}
	providerClient = &http.Client{Transport: modelTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(strings.Repeat(" ", 2*1024*1024+1)))}, nil
	})}
	if _, e = fetchModels(context.Background(), "https://provider.example/v1", "secret"); e == nil {
		t.Fatal("oversized response accepted")
	}
}

func TestStructuredModelText(t *testing.T) {
	for _, tc := range []struct {
		name string
		text string
		ok   bool
	}{
		{"json", `{"title":"Greeting","body":"Hello","language":"en"}`, true},
		{"json fence", " \n```json\n{\"title\":\"Greeting\",\"body\":\"Hello\",\"language\":\"en\"}\n```\n", true},
		{"plain fence", "```\n{\"title\":\"Greeting\",\"body\":\"Hello\",\"language\":\"en\"}\n```", true},
		{"unknown fields", `{"title":"Greeting","body":"Hello","language":"en","secret":"not allowed"}`, false},
		{"wrong field type", `{"title":"Greeting","body":["Hello"],"language":"en"}`, false},
		{"trailing object", `{"body":"Hello"} {"body":"different"}`, false},
		{"trailing text", `{"body":"Hello"} extra`, false},
		{"unclosed fence", "```json\n{\"body\":\"Hello\"}", false},
		{"commentary", "Here is the result:\n{\"body\":\"Hello\"}", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var result struct {
				Title    string `json:"title"`
				Body     string `json:"body"`
				Language string `json:"language"`
			}
			err := decodeStructuredText(tc.text, &result)
			if (err == nil) != tc.ok {
				t.Fatalf("structured text accepted=%v, expected=%v: %v", err == nil, tc.ok, err)
			}
			if tc.ok && (result.Title != "Greeting" || result.Body != "Hello" || result.Language != "en") {
				t.Fatalf("structured content changed: %+v", result)
			}
			if err != nil && err.Error() != "AI returned invalid structured fields" {
				t.Fatal("provider content leaked in error", err)
			}
		})
	}
}
