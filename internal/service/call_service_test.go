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
		"param":         "开场白变量",
	}, "address_verify", taskID)

	assert.Equal(t, "address_verify", params["biz_type"])
	assert.Equal(t, taskID.String(), params["task_id"])
	assert.Equal(t, "张三", params["customer_name"])
	assert.NotContains(t, params, "param")
}

func TestBuildStartWordParamsInjectsAddressOpeningParam(t *testing.T) {
	params := buildStartWordParams(nil, "address_verify")

	assert.Equal(t, "此次致电是想跟您核实服务地址，请您说一下详细地址，尽量具体到门牌号。", params["param"])
}

func TestBuildStartWordParamsInjectsAppointmentOpeningParam(t *testing.T) {
	params := buildStartWordParams(nil, "workorder_appointment")

	assert.Equal(t, "此次致电是想询问您是否已有师傅跟您预约上门时间。", params["param"])
}

func TestBuildStartWordParamsKeepsExplicitOpeningParam(t *testing.T) {
	params := buildStartWordParams(map[string]any{
		"param": "自定义开场参数",
	}, "address_verify")

	assert.Equal(t, "自定义开场参数", params["param"])
}

func TestBuildCallTaskBizParamsInjectsStoredContext(t *testing.T) {
	taskID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	callID := "call-123"
	params, err := buildCallTaskBizParams(&model.CallTask{
		TaskID:    taskID,
		BizType:   "address_verify",
		BizParams: []byte(`{"order_id":"ADDR-001","biz_type":"wrong"}`),
		CallID:    &callID,
	})

	require.NoError(t, err)
	assert.Equal(t, "address_verify", params["biz_type"])
	assert.Equal(t, "ADDR-001", params["order_id"])
	assert.Equal(t, taskID.String(), params["task_id"])
	assert.Equal(t, callID, params["call_id"])
}
