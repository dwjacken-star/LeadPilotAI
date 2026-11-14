package askai

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestChunkIndependentHTML(t *testing.T) {
	want := "<!doctype html><html><head><style>p{margin:0}</style></head><body><p>Hi</p></body></html>"
	for _, content := range []string{"Here is the email:\n```html\n" + want + "\n```", want, "<think>internal</think>\n" + want} {
		if got := extractEmailHTML(content); got != want {
			t.Fatalf("HTML changed: %s", got)
		}
	}
	if got := extractEmailHTML("No HTML returned"); got != "" {
		t.Fatal("plain text saved as template")
	}
}
func TestGenerationStopAndErrors(t *testing.T) {
	ctx, finish, err := startGeneration(context.Background(), "one")
	if err != nil {
		t.Fatal(err)
	}
	defer finish()
	if _, _, err := startGeneration(context.Background(), "one"); err == nil {
		t.Fatal("concurrent generation accepted")
	}
	if err = stopGeneration("one"); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(ctx.Err(), context.Canceled) {
		t.Fatal("stop did not cancel request")
	}
	for raw, want := range map[string]string{"429 no credits remaining": "ai_balance", "422 missing content": "ai_parameters", "401 invalid api key": "ai_credentials"} {
		if got := publicAIError(errors.New(raw)); got != want {
			t.Fatal(got)
		}
	}
	client := (&OpenAI{BaseUrl: "http://localhost:9999/v1", ApiKey: "test"}).GetClient()
	if client == nil {
		t.Fatal("custom client unavailable")
	}
	if !strings.Contains(publicAIError(context.DeadlineExceeded), "timeout") {
		t.Fatal("timeout classification")
	}
}
