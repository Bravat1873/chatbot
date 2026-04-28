// 请求/响应模型：定义 ChatCompletion 协议的请求体和内部 Turn 流转结构。
package gateway

import (
	"encoding/json"

	"github.com/google/uuid"
)

// ChatMessage OpenAI-compatible 消息结构。
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatCompletionRequest OpenAI-compatible /v1/chat/completions 请求体。
type ChatCompletionRequest struct {
	Model     string          `json:"model"`
	Stream    bool            `json:"stream"`
	SessionID string          `json:"session_id"`
	CallID    string          `json:"call_id"`
	BizParams json.RawMessage `json:"biz_params"`
	Messages  []ChatMessage   `json:"messages"`
	Input     GatewayInput    `json:"input"`
}

// GatewayInput 兼容阿里云百炼应用模型网关的 input 包装结构。
type GatewayInput struct {
	Messages  []ChatMessage   `json:"messages"`
	BizParams json.RawMessage `json:"biz_params"`
}

// TurnRequest 内部对话轮次请求，由 handler 从 ChatCompletionRequest 转换而来。
type TurnRequest struct {
	SessionID string
	UserText  string
	BizParams map[string]any
	Messages  []ChatMessage
}

// TurnResponse 内部对话轮次响应，包含 bot 回复文本和会话状态。
type TurnResponse struct {
	Reply  string
	Status string
}

// NormalizedSessionID 返回规范化后的会话 ID，为空时自动生成 UUID。
func (r ChatCompletionRequest) NormalizedSessionID() string {
	if r.SessionID != "" {
		return r.SessionID
	}
	if r.CallID != "" {
		return r.CallID
	}
	return uuid.NewString()
}

// UserText 从消息列表中找到最后一条 user 角色消息作为当前输入。
func (r ChatCompletionRequest) UserText() string {
	messages := r.NormalizedMessages()
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			return messages[i].Content
		}
	}
	return ""
}

// NormalizedMessages 返回顶层 messages 或阿里云 input.messages。
func (r ChatCompletionRequest) NormalizedMessages() []ChatMessage {
	if len(r.Messages) > 0 {
		return r.Messages
	}
	return r.Input.Messages
}

// CoerceBizParams 将 biz_params JSON 转换为 map，兼容对象和各类原始值格式。
func (r ChatCompletionRequest) CoerceBizParams() map[string]any {
	rawParams := r.normalizedBizParams()
	if len(rawParams) == 0 || string(rawParams) == "null" {
		return nil
	}
	var object map[string]any
	if err := json.Unmarshal(rawParams, &object); err == nil && object != nil {
		return object
	}
	var raw any
	if err := json.Unmarshal(rawParams, &raw); err != nil {
		return map[string]any{"raw": string(rawParams)}
	}
	return map[string]any{"raw": raw}
}

func (r ChatCompletionRequest) normalizedBizParams() json.RawMessage {
	if len(r.BizParams) > 0 {
		return r.BizParams
	}
	return r.Input.BizParams
}
