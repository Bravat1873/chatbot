package dashscope

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"chatbot/internal/core"
)

func TestGenerateJSONCallsOpenAICompatibleEndpoint(t *testing.T) {
	var authHeader string
	var path string
	var payload chatCompletionRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		path = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"{\"intent\":\"yes\"}"}}]}`))
	}))
	defer server.Close()

	client := New(Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Model:   "qwen-test",
		Timeout: time.Second,
	})
	content, err := client.GenerateJSON(context.Background(), core.LLMRequest{
		SystemPrompt: "system",
		UserPrompt:   "user",
		MaxTokens:    20,
		Temperature:  0,
	})
	if err != nil {
		t.Fatalf("generate json: %v", err)
	}

	if path != "/chat/completions" || authHeader != "Bearer test-key" {
		t.Fatalf("unexpected request path/auth: path=%s auth=%s", path, authHeader)
	}
	if payload.Model != "qwen-test" || payload.Messages[0].Content != "system" || payload.Messages[1].Content != "user" {
		t.Fatalf("unexpected payload: %#v", payload)
	}
	if content != `{"intent":"yes"}` {
		t.Fatalf("unexpected content: %q", content)
	}
}
