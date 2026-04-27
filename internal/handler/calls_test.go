package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"chatbot/internal/model"
	"chatbot/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type stubCallService struct {
	task *model.CallTask
	err  error
}

func (s *stubCallService) CreateCallTask(context.Context, model.CreateCallTaskRequest) (*model.CallTask, error) {
	return s.task, s.err
}

func TestCreateCallTaskReturnsEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	callID := "call-123"
	taskID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	svc := &stubCallService{
		task: &model.CallTask{
			TaskID: taskID,
			Status: model.CallStatusAccepted,
			CallID: &callID,
		},
	}
	r := gin.New()
	r.POST("/internal/calls", CreateCallTask(nil, svc))

	rawBody, _ := json.Marshal(map[string]any{
		"called_number": "13800138000",
		"biz_type":      "repair_followup",
		"biz_params":    map[string]any{"order_id": "A001"},
	})
	req := httptest.NewRequest(http.MethodPost, "/internal/calls", bytes.NewReader(rawBody))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()

	r.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	var responseBody map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &responseBody); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if responseBody["code"] != "OK" || responseBody["message"] != "success" {
		t.Fatalf("unexpected response body: %#v", responseBody)
	}
	data, ok := responseBody["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected response data object, got %#v", responseBody["data"])
	}
	if data["task_id"] != taskID.String() || data["call_id"] != callID || data["status"] != string(model.CallStatusAccepted) {
		t.Fatalf("unexpected response data: %#v", data)
	}
}

func TestCreateCallTaskMapsAPIErrorEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	taskID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	svc := &stubCallService{
		err: &service.APIError{
			StatusCode: http.StatusBadGateway,
			Message:    "call submit failed",
			TaskID:     &taskID,
		},
	}
	r := gin.New()
	r.POST("/internal/calls", CreateCallTask(nil, svc))

	rawBody, _ := json.Marshal(map[string]any{
		"called_number": "13800138000",
		"biz_type":      "repair_followup",
		"biz_params":    map[string]any{"order_id": "A001"},
	})
	req := httptest.NewRequest(http.MethodPost, "/internal/calls", bytes.NewReader(rawBody))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()

	r.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d", resp.Code)
	}
	var responseBody map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &responseBody); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if responseBody["code"] != "UPSTREAM_ERROR" || responseBody["message"] != "call submit failed" {
		t.Fatalf("unexpected response body: %#v", responseBody)
	}
	data, ok := responseBody["data"].(map[string]any)
	if !ok || data["task_id"] != taskID.String() {
		t.Fatalf("expected task_id data, got %#v", responseBody["data"])
	}
}
