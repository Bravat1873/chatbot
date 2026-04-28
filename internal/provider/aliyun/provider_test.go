package aliyun

import (
	"encoding/json"
	"testing"

	"chatbot/internal/service"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildLlmSmartCallRequestIncludesStartWordParam(t *testing.T) {
	req, err := buildLlmSmartCallRequest(Config{
		Endpoint: "aiccs.aliyuncs.com",
	}, service.SubmitCallRequest{
		CalledNumber:         "13800138000",
		CallerNumber:         "02000000000",
		ApplicationCode:      "app-code",
		SessionTimeoutSecond: 1200,
		BizParams: map[string]any{
			"biz_type": "address_verify",
			"task_id":  "task-001",
		},
		StartWordParams: map[string]any{
			"param": "此次致电是想跟您核实服务地址。",
		},
	})

	require.NoError(t, err)
	assert.Equal(t, "aiccs.aliyuncs.com", req.Domain)
	assert.Equal(t, "LlmSmartCall", req.ApiName)
	assert.JSONEq(t, `{"biz_type":"address_verify","task_id":"task-001"}`, req.QueryParams["BizParam"])

	var startWordParams map[string]string
	require.NoError(t, json.Unmarshal([]byte(req.QueryParams["StartWordParam"]), &startWordParams))
	assert.Equal(t, "此次致电是想跟您核实服务地址。", startWordParams["param"])
}
