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

	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.Code)
	}
	var responseBody map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &responseBody); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if responseBody["ok"] != false || responseBody["message"] != "unauthorized" {
		t.Fatalf("unexpected response body: %#v", responseBody)
	}
	if callbackSvc.called {
		t.Fatal("callback service should not be called when unauthorized")
	}
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

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	var responseBody map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &responseBody); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if responseBody["ok"] != true {
		t.Fatalf("unexpected response body: %#v", responseBody)
	}
	if !callbackSvc.called {
		t.Fatal("expected callback service to be called")
	}
}
