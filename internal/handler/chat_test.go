package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"chatbot/internal/gateway"

	"github.com/gin-gonic/gin"
)

type stubDialogueService struct {
	req        gateway.TurnRequest
	reply      string
	shouldFail bool
}

func (s *stubDialogueService) ProcessTurn(ctx context.Context, req gateway.TurnRequest) (gateway.TurnResponse, error) {
	_ = ctx
	s.req = req
	if s.shouldFail {
		return gateway.TurnResponse{}, errStubDialogue
	}
	return gateway.TurnResponse{Reply: s.reply, Status: "in_progress"}, nil
}

type stubDialogueError string

func (e stubDialogueError) Error() string {
	return string(e)
}

const errStubDialogue = stubDialogueError("boom")

func TestChatCompletionsStreamsSSEReply(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dialogueSvc := &stubDialogueService{reply: "第一句，第二句。"}
	router := NewRouter(RouterDeps{
		CallService:      &fakeCallService{},
		CallbackService:  &fakeCallbackService{},
		DialogueService:  dialogueSvc,
		GatewayAuthToken: "secret",
		DefaultLLMModel:  "default-model",
	})

	body := `{"model":"qwen-plus","session_id":"session-1","biz_params":{"customer_name":"张三"},"messages":[{"role":"assistant","content":"您好"},{"role":"user","content":"有的"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer secret")
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	if dialogueSvc.req.SessionID != "session-1" || dialogueSvc.req.UserText != "有的" {
		t.Fatalf("unexpected dialogue request: %#v", dialogueSvc.req)
	}
	if dialogueSvc.req.BizParams["customer_name"] != "张三" {
		t.Fatalf("unexpected biz params: %#v", dialogueSvc.req.BizParams)
	}
	dataLines := sseDataLines(resp.Body.String())
	if dataLines[len(dataLines)-1] != "[DONE]" {
		t.Fatalf("expected DONE, got %q", dataLines[len(dataLines)-1])
	}
	var first gateway.SSEChunk
	if err := json.Unmarshal([]byte(dataLines[0]), &first); err != nil {
		t.Fatalf("decode first chunk: %v", err)
	}
	if first.Choices[0].Delta.Content != "第一句，" || first.Model != "qwen-plus" || !strings.HasPrefix(first.ID, "chatcmpl-") {
		t.Fatalf("unexpected first chunk: %#v", first)
	}
}

func TestChatCompletionsRequiresAuthWhenTokenConfigured(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := NewRouter(RouterDeps{
		CallService:      &fakeCallService{},
		CallbackService:  &fakeCallbackService{},
		DialogueService:  &stubDialogueService{reply: "ok"},
		GatewayAuthToken: "secret",
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"messages":[{"role":"user","content":"有的"}]}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.Code)
	}
}

func TestChatCompletionsReturnsSafeReplyWhenDialogueFails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := NewRouter(RouterDeps{
		CallService:      &fakeCallService{},
		CallbackService:  &fakeCallbackService{},
		DialogueService:  &stubDialogueService{shouldFail: true},
		GatewayAuthToken: "secret",
		DefaultLLMModel:  "default-model",
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"session_id":"session-1","messages":[{"role":"user","content":"有的"}]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer secret")
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	dataLines := sseDataLines(resp.Body.String())
	var first gateway.SSEChunk
	if err := json.Unmarshal([]byte(dataLines[0]), &first); err != nil {
		t.Fatalf("decode first chunk: %v", err)
	}
	if first.Choices[0].Delta.Content != "不好意思，" || first.Model != "default-model" {
		t.Fatalf("unexpected fallback chunk: %#v", first)
	}
}

func sseDataLines(body string) []string {
	lines := strings.Split(body, "\n")
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.HasPrefix(line, "data: ") {
			result = append(result, strings.TrimPrefix(line, "data: "))
		}
	}
	return result
}
