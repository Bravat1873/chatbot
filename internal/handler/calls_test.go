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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

	assert.Equal(t, http.StatusOK, resp.Code)
	var responseBody map[string]any
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &responseBody))
	assert.Equal(t, "OK", responseBody["code"])
	assert.Equal(t, "success", responseBody["message"])
	data := responseBody["data"].(map[string]any)
	assert.Equal(t, taskID.String(), data["task_id"])
	assert.Equal(t, callID, data["call_id"])
	assert.Equal(t, string(model.CallStatusAccepted), data["status"])
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

	assert.Equal(t, http.StatusBadGateway, resp.Code)
	var responseBody map[string]any
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &responseBody))
	assert.Equal(t, "UPSTREAM_ERROR", responseBody["code"])
	assert.Equal(t, "call submit failed", responseBody["message"])
	data := responseBody["data"].(map[string]any)
	assert.Equal(t, taskID.String(), data["task_id"])
}
