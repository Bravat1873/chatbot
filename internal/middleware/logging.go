// HTTP 请求日志中间件：记录每个请求的方法、路径、状态码、延迟和客户端 IP。
package middleware

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
)

// RequestLogger 记录每个 HTTP 请求的方法、路径、状态码、耗时和客户端 IP。
func RequestLogger(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		startedAt := time.Now()
		c.Next()
		if logger == nil {
			return
		}
		logger.Info("http_request",
			"method", c.Request.Method,
			"path", c.FullPath(),
			"status", c.Writer.Status(),
			"latency_ms", time.Since(startedAt).Milliseconds(),
			"client_ip", c.ClientIP(),
		)
	}
}
