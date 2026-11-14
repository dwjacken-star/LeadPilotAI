package askai

import (
	"billionmail-core/internal/testsupport"
	"encoding/json"
	"fmt"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/google/uuid"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestStreamingRequestPersistsCompleteHTML(t *testing.T) {
	testsupport.Database(t)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&request)
		if request["model"] != "mock-chat" {
			t.Error("model was changed")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		// Usage-only frames and arbitrary HTML/code-fence token boundaries.
		fmt.Fprint(w, "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":2,\"completion_tokens\":3,\"total_tokens\":5}}\n\n")
		for _, content := range []string{"```", "html\n<!doctype html><html><head><style>p{margin:0}</style></head><body>", "<p>你好</p></body></html>\n", "```"} {
			payload, _ := json.Marshal(map[string]interface{}{"choices": []map[string]interface{}{{"index": 0, "delta": map[string]string{"content": content}}}})
			fmt.Fprintf(w, "data: %s\n\n", payload)
			w.(http.Flusher).Flush()
		}
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer upstream.Close()
	id := "stream-test-" + uuid.NewString()
	directory := CHAT_CONFIG_PATH + "/" + id
	if err := os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(directory)
	server := g.Server("ai-stream-" + id)
	server.SetAddr("127.0.0.1:0")
	server.SetDumpRouterMap(false)
	server.BindHandler("/generate", func(r *ghttp.Request) {
		o := &OpenAI{Ctx: r.Context(), BaseUrl: upstream.URL, ApiKey: "local-test", ModelId: "mock-chat", ChatId: id, TempInfo: &TempChat{AddType: 99}, AddedSystemPrompt: "Use the supplied prompt", MaxTokens: 1000}
		if err := o.Chat("Write an inquiry email in Chinese", false); err != nil {
			r.Response.WriteJson(map[string]string{"error": err.Error()})
		}
	})
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	defer server.Shutdown()
	response, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/generate", server.GetListenedPort()))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if !strings.Contains(string(body), `"is_end":true`) || !strings.Contains(response.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("stream did not complete: %s", body)
	}
	html, err := GetHtml(id)
	if err != nil || !strings.Contains(html, "<style>p{margin:0}</style>") || !strings.Contains(html, "你好") || strings.Contains(html, "```") {
		t.Fatalf("HTML lost or changed: %s %v", html, err)
	}
}
