package gateway

import (
	"encoding/json"

	"github.com/google/uuid"
)

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatCompletionRequest struct {
	Model     string          `json:"model"`
	Stream    bool            `json:"stream"`
	SessionID string          `json:"session_id"`
	BizParams json.RawMessage `json:"biz_params"`
	Messages  []ChatMessage   `json:"messages"`
}

type TurnRequest struct {
	SessionID string
	UserText  string
	BizParams map[string]any
	Messages  []ChatMessage
}

type TurnResponse struct {
	Reply  string
	Status string
}

func (r ChatCompletionRequest) NormalizedSessionID() string {
	if r.SessionID != "" {
		return r.SessionID
	}
	return uuid.NewString()
}

func (r ChatCompletionRequest) UserText() string {
	for i := len(r.Messages) - 1; i >= 0; i-- {
		if r.Messages[i].Role == "user" {
			return r.Messages[i].Content
		}
	}
	return ""
}

func (r ChatCompletionRequest) CoerceBizParams() map[string]any {
	if len(r.BizParams) == 0 || string(r.BizParams) == "null" {
		return nil
	}
	var object map[string]any
	if err := json.Unmarshal(r.BizParams, &object); err == nil && object != nil {
		return object
	}
	var raw any
	if err := json.Unmarshal(r.BizParams, &raw); err != nil {
		return map[string]any{"raw": string(r.BizParams)}
	}
	return map[string]any{"raw": raw}
}
