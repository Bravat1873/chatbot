// Chat 对话处理器：接收 OpenAI-compatible 请求，经 DialogueService 处理后以 SSE 流式返回。
package handler

import (
	"context"
	"log/slog"
	"net/http"

	"chatbot/internal/gateway"

	"github.com/gin-gonic/gin"
)

const safeFallbackReply = "不好意思，请您再说一遍？"

// DialogueService 对话服务接口，handler 通过此接口解耦 service 层。
type DialogueService interface {
	ProcessTurn(ctx context.Context, req gateway.TurnRequest) (gateway.TurnResponse, error)
}

// ChatCompletions 处理 /v1/chat/completions 请求：解析 -> 调 service -> 写 SSE 流。
func ChatCompletions(logger *slog.Logger, svc DialogueService, defaultModel string) gin.HandlerFunc {
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
		if logger != nil {
			logger.Info("chat_gateway_request_received",
				"session_id", sessionID,
				"call_id_present", req.CallID != "",
				"biz_params_source", req.BizParamsSource(),
				"biz_type", req.BizType(),
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
