package gateway

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChatCompletionRequestSupportsAliyunInputEnvelope(t *testing.T) {
	raw := []byte(`{
		"call_id": "call-123",
		"input": {
			"messages": [
				{"role": "assistant", "content": "您好"},
				{"role": "user", "content": "广州海珠区仑头村仑头路82号"}
			],
			"biz_params": {
				"biz_type": "address_verify",
				"order_id": "ADDR-001"
			}
		}
	}`)

	var req ChatCompletionRequest
	require.NoError(t, json.Unmarshal(raw, &req))

	assert.Equal(t, "call-123", req.NormalizedSessionID())
	assert.Equal(t, "广州海珠区仑头村仑头路82号", req.UserText())
	assert.Len(t, req.NormalizedMessages(), 2)
	bizParams := req.CoerceBizParams()
	assert.Equal(t, "address_verify", bizParams["biz_type"])
	assert.Equal(t, "ADDR-001", bizParams["order_id"])
}

func TestChatCompletionRequestPrefersTopLevelFields(t *testing.T) {
	raw := []byte(`{
		"session_id": "session-1",
		"call_id": "call-123",
		"messages": [{"role": "user", "content": "顶层消息"}],
		"biz_params": {"biz_type": "workorder_appointment"},
		"input": {
			"messages": [{"role": "user", "content": "input 消息"}],
			"biz_params": {"biz_type": "address_verify"}
		}
	}`)

	var req ChatCompletionRequest
	require.NoError(t, json.Unmarshal(raw, &req))

	assert.Equal(t, "session-1", req.NormalizedSessionID())
	assert.Equal(t, "顶层消息", req.UserText())
	assert.Equal(t, "workorder_appointment", req.CoerceBizParams()["biz_type"])
}
