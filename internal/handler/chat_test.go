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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

	assert.Equal(t, http.StatusOK, resp.Code)
	assert.Equal(t, "session-1", dialogueSvc.req.SessionID)
	assert.Equal(t, "有的", dialogueSvc.req.UserText)
	assert.Equal(t, "张三", dialogueSvc.req.BizParams["customer_name"])
	dataLines := sseDataLines(resp.Body.String())
	assert.Equal(t, "[DONE]", dataLines[len(dataLines)-1])
	var first gateway.SSEChunk
	require.NoError(t, json.Unmarshal([]byte(dataLines[0]), &first))
	assert.Equal(t, "第一句，", first.Choices[0].Delta.Content)
	assert.Equal(t, "qwen-plus", first.Model)
	assert.True(t, strings.HasPrefix(first.ID, "chatcmpl-"))
}

func TestChatCompletionsAcceptsAliyunInputEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dialogueSvc := &stubDialogueService{reply: "收到。"}
	router := NewRouter(RouterDeps{
		CallService:      &fakeCallService{},
		CallbackService:  &fakeCallbackService{},
		DialogueService:  dialogueSvc,
		GatewayAuthToken: "secret",
		DefaultLLMModel:  "default-model",
	})

	body := `{"call_id":"call-123","input":{"biz_params":{"biz_type":"address_verify","order_id":"ADDR-001"},"messages":[{"role":"assistant","content":"您好"},{"role":"user","content":"广州海珠区仑头村仑头路82号"}]}}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer secret")
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusOK, resp.Code)
	assert.Equal(t, "call-123", dialogueSvc.req.SessionID)
	assert.Equal(t, "广州海珠区仑头村仑头路82号", dialogueSvc.req.UserText)
	assert.Equal(t, "address_verify", dialogueSvc.req.BizParams["biz_type"])
	assert.Equal(t, "ADDR-001", dialogueSvc.req.BizParams["order_id"])
	assert.Len(t, dialogueSvc.req.Messages, 2)
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

	assert.Equal(t, http.StatusUnauthorized, resp.Code)
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

	assert.Equal(t, http.StatusOK, resp.Code)
	dataLines := sseDataLines(resp.Body.String())
	var first gateway.SSEChunk
	require.NoError(t, json.Unmarshal([]byte(dataLines[0]), &first))
	assert.Equal(t, "不好意思，", first.Choices[0].Delta.Content)
	assert.Equal(t, "default-model", first.Model)
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
