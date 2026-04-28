// Chat 对话处理器：接收 OpenAI-compatible 请求，经 DialogueService 处理后以 SSE 流式返回。
package handler

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"chatbot/internal/gateway"

	"github.com/gin-gonic/gin"
)

const safeFallbackReply = "不好意思，请您再说一遍？"

// DialogueService 对话服务接口，handler 通过此接口解耦 service 层。
type DialogueService interface {
	ProcessTurn(ctx context.Context, req gateway.TurnRequest) (gateway.TurnResponse, error)
}

// ChatCompletions 处理 /v1/chat/completions 请求：解析 -> 调 service -> 写 SSE 流。
func ChatCompletions(logger *slog.Logger, svc DialogueService, defaultModel string, resolver CallContextResolver) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req gateway.ChatCompletionRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"detail": "invalid request"})
			return
		}

		sessionID := req.NormalizedSessionID()
		model := req.Model
		if model == "" {
			model = defaultModel
		}

		messages := req.NormalizedMessages()
		bizParams := req.CoerceBizParams()
		userText := req.UserText()
		contextResolved := false
		if bizTypeFromParams(bizParams) == "" && resolver != nil {
			lookupCallID := firstNonEmpty(req.CallID, sessionID)
			resolved, matched, err := resolver.ResolveCallBizParams(c.Request.Context(), lookupCallID)
			if err != nil {
				if logger != nil {
					logger.Warn("chat_gateway_context_lookup_failed", "session_id", sessionID, "lookup_call_id", lookupCallID, "error", err.Error())
				}
			} else if matched {
				bizParams = mergeBizParams(bizParams, resolved)
				contextResolved = true
			}
		}
		if logger != nil {
			logger.Info("chat_gateway_request_received",
				"session_id", sessionID,
				"call_id_present", req.CallID != "",
				"biz_params_source", req.BizParamsSource(),
				"biz_type", bizTypeFromParams(bizParams),
				"context_resolved", contextResolved,
				"messages_source", req.MessagesSource(),
				"messages_count", len(messages),
				"user_text_present", userText != "",
			)
		}

		reply := safeFallbackReply
		if svc != nil {
			resp, err := svc.ProcessTurn(c.Request.Context(), gateway.TurnRequest{
				SessionID: sessionID,
				UserText:  userText,
				BizParams: bizParams,
				Messages:  messages,
			})
			if err != nil {
				if logger != nil {
					logger.Error("chat_process_turn_failed", "session_id", sessionID, "error", err.Error())
				}
			} else if resp.Reply != "" {
				reply = resp.Reply
			}
		}

		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		c.Status(http.StatusOK)
		if err := gateway.WriteChatCompletionSSE(c.Writer, reply, model, 0, gateway.NewCompletionID()); err != nil && logger != nil {
			logger.Error("chat_sse_write_failed", "session_id", sessionID, "error", err.Error())
		}
	}
}

func bizTypeFromParams(params map[string]any) string {
	for _, key := range []string{"biz_type", "scene", "scenario"} {
		if value, ok := params[key]; ok {
			if text, ok := value.(string); ok {
				return strings.TrimSpace(text)
			}
		}
	}
	return ""
}

func mergeBizParams(requestParams map[string]any, resolvedParams map[string]any) map[string]any {
	if len(requestParams) == 0 {
		return copyBizParams(resolvedParams)
	}
	merged := copyBizParams(requestParams)
	for key, value := range resolvedParams {
		merged[key] = value
	}
	return merged
}

func copyBizParams(params map[string]any) map[string]any {
	output := make(map[string]any, len(params))
	for key, value := range params {
		output[key] = value
	}
	return output
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
