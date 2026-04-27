package handler

import (
	"context"
	"log/slog"
	"net/http"

	"chatbot/internal/gateway"

	"github.com/gin-gonic/gin"
)

const safeFallbackReply = "不好意思，请您再说一遍？"

type DialogueService interface {
	ProcessTurn(ctx context.Context, req gateway.TurnRequest) (gateway.TurnResponse, error)
}

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

		reply := safeFallbackReply
		if svc != nil {
			resp, err := svc.ProcessTurn(c.Request.Context(), gateway.TurnRequest{
				SessionID: sessionID,
				UserText:  req.UserText(),
				BizParams: req.CoerceBizParams(),
				Messages:  req.Messages,
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
