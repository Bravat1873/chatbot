package service

import (
	"testing"
	"time"

	"chatbot/internal/model"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseFlexibleTimeRFC3339(t *testing.T) {
	parsed, err := parseFlexibleTime("2026-04-23T12:34:56Z")
	require.NoError(t, err)
	assert.Equal(t, "2026-04-23T12:34:56Z", parsed.UTC().Format(time.RFC3339))
}

func TestParseFlexibleTimeUnixMilli(t *testing.T) {
	parsed, err := parseFlexibleTime("1713873600000")
	require.NoError(t, err)
	assert.False(t, parsed.IsZero(), "expected non-zero time")
}

func TestFindStringRecursive(t *testing.T) {
	payload := map[string]any{
		"outer": map[string]any{
			"CallId": "abc-123",
		},
	}
	assert.Equal(t, "abc-123", findString(payload, "call_id", "CallId"))
}

func TestValidateCreateCallTaskRequestRejectsUnsupportedBizType(t *testing.T) {
	err := validateCreateCallTaskRequest(model.CreateCallTaskRequest{
		CalledNumber: "13800138000",
		BizType:      "unknown",
	})

	require.NotNil(t, err)
	assert.Equal(t, "unsupported biz_type", err.Message)
}

func TestBuildProviderBizParamsInjectsBizTypeAndTaskID(t *testing.T) {
	taskID := uuid.MustParse("11111111-1111-1111-1111-111111111111")

	params := buildProviderBizParams(map[string]any{
		"biz_type":      "wrong",
		"customer_name": "张三",
	}, "address_verify", taskID)

	assert.Equal(t, "address_verify", params["biz_type"])
	assert.Equal(t, taskID.String(), params["task_id"])
	assert.Equal(t, "张三", params["customer_name"])
}
