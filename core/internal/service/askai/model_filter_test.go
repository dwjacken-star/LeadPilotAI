package askai

import "testing"

func TestEmailModelFilter(t *testing.T) {
	for _, model := range []string{"gpt-4.1-mini", "deepseek-pro", "gemini-2.5-pro"} {
		if !IsEmailModel(model) {
			t.Errorf("valid chat model excluded: %s", model)
		}
	}
	for _, model := range []string{"gpt-5-pro", "text-embedding-3-small", "gpt-image-1", "gpt-4o-realtime-preview", "davinci-002"} {
		if IsEmailModel(model) {
			t.Errorf("unsupported model included: %s", model)
		}
	}
}
