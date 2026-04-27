package dashscope

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"chatbot/internal/core"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateJSONCallsOpenAICompatibleEndpoint(t *testing.T) {
	var authHeader string
	var path string
	var payload chatCompletionRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		path = r.URL.Path
		require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
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
	require.NoError(t, err)

	assert.Equal(t, "/chat/completions", path)
	assert.Equal(t, "Bearer test-key", authHeader)
	assert.Equal(t, "qwen-test", payload.Model)
	assert.Equal(t, "system", payload.Messages[0].Content)
	assert.Equal(t, "user", payload.Messages[1].Content)
	assert.Equal(t, `{"intent":"yes"}`, content)
}
