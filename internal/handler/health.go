package handler

import (
	"context"
	"net/http"

	"chatbot/internal/utils/httpresponse"

	"github.com/gin-gonic/gin"
)

// Healthz 健康检查端点，若提供了 check 回调则检查底层依赖（如 DB 连接）。
func Healthz(check func(context.Context) error) gin.HandlerFunc {
	return func(c *gin.Context) {
		if check != nil {
			if err := check(c.Request.Context()); err != nil {
				httpresponse.Error(c, http.StatusServiceUnavailable, httpresponse.CodeServiceUnavailable, "service unavailable", gin.H{"detail": err.Error()})
				return
			}
		}
		httpresponse.OK(c, gin.H{"status": "ok"})
	}
}
