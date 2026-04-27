// 认证中间件：Token 校验，支持内部 API 和回调两种模式的鉴权。
package middleware

import (
	"net/http"
	"strings"

	"chatbot/internal/utils/httpresponse"

	"github.com/gin-gonic/gin"
)

// InternalToken 校验 X-Internal-Token header，用于保护内部 API。
func InternalToken(token string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if strings.TrimSpace(token) == "" {
			c.Next()
			return
		}
		provided := strings.TrimSpace(c.GetHeader("X-Internal-Token"))
		if provided != token {
			httpresponse.AbortError(c, http.StatusUnauthorized, httpresponse.CodeUnauthorized, "unauthorized", nil)
			return
		}
		c.Next()
	}
}

// CallbackToken 同时校验 Header 和 Query 中的 token，标记认证模式供 handler 使用。
func CallbackToken(token string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if strings.TrimSpace(token) == "" {
			c.Next()
			return
		}
		// 支持 Header（X-Callback-Token）和 Query（?token=）两种传参
		headerToken := strings.TrimSpace(c.GetHeader("X-Callback-Token"))
		queryToken := strings.TrimSpace(c.Query("token"))
		if headerToken != token && queryToken != token {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"ok": false, "message": "unauthorized"})
			return
		}
		// 记录认证来源，后续 handler 可用
		if headerToken == token {
			c.Set("auth_mode", "header")
		} else if queryToken == token {
			c.Set("auth_mode", "query")
		}
		c.Next()
	}
}

func GatewayBearer(token string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if strings.TrimSpace(token) == "" {
			c.Next()
			return
		}
		authHeader := strings.TrimSpace(c.GetHeader("Authorization"))
		provided := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
		if provided != token {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"detail": "Unauthorized"})
			return
		}
		c.Next()
	}
}
