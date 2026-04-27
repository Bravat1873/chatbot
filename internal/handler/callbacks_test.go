package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"chatbot/internal/model"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeCallService struct{}

func (f *fakeCallService) CreateCallTask(context.Context, model.CreateCallTaskRequest) (*model.CallTask, error) {
	return nil, nil
}

type fakeCallbackService struct {
	called bool
}

func (f *fakeCallbackService) HandleCallReport(ctx context.Context, payload map[string]any, rawPayload json.RawMessage, sourceIP, authMode string) (bool, bool, error) {
	f.called = true
	return false, true, nil
}

func TestCallbackRouteRequiresToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	callbackSvc := &fakeCallbackService{}
	router := NewRouter(RouterDeps{
		CallService:      &fakeCallService{},
		CallbackService:  callbackSvc,
		CallbackAPIToken: "secret",
	})

	rawBody, _ := json.Marshal(map[string]any{"CallId": "abc"})
	req := httptest.NewRequest(http.MethodPost, "/callbacks/aiccs/report", bytes.NewReader(rawBody))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusUnauthorized, resp.Code)
	var responseBody map[string]any
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &responseBody))
	assert.Equal(t, false, responseBody["ok"])
	assert.Equal(t, "unauthorized", responseBody["message"])
	assert.False(t, callbackSvc.called, "callback service should not be called when unauthorized")
}

func TestCallbackRouteAcceptsJSONPayload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	callbackSvc := &fakeCallbackService{}
	router := NewRouter(RouterDeps{
		CallService:      &fakeCallService{},
		CallbackService:  callbackSvc,
		CallbackAPIToken: "secret",
	})

	rawBody, _ := json.Marshal(map[string]any{"CallId": "abc"})
	req := httptest.NewRequest(http.MethodPost, "/callbacks/aiccs/report", bytes.NewReader(rawBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Callback-Token", "secret")
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusOK, resp.Code)
	var responseBody map[string]any
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &responseBody))
	assert.Equal(t, true, responseBody["ok"])
	assert.True(t, callbackSvc.called, "expected callback service to be called")
}
