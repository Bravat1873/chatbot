// 认证中间件：Token 校验，支持内部 API 和回调两种模式的鉴权。
package middleware

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"chatbot/internal/utils/httpresponse"

	"github.com/gin-gonic/gin"
)

// InternalToken 校验内部 API 的 Authorization: Bearer <token>。
func InternalToken(token string) gin.HandlerFunc {
	return func(c *gin.Context) {
		expected := strings.TrimSpace(token)
		if expected == "" {
			c.Next()
			return
		}
		provided := parseBearerToken(c.GetHeader("Authorization"))
		if !tokenMatches(provided, expected) {
			httpresponse.AbortError(c, http.StatusUnauthorized, httpresponse.CodeUnauthorized, "unauthorized", nil)
			return
		}
		c.Next()
	}
}

// CallbackToken 同时校验 Header 和 Query 中的 token，标记认证模式供 handler 使用。
func CallbackToken(token string) gin.HandlerFunc {
	return func(c *gin.Context) {
		expected := strings.TrimSpace(token)
		if expected == "" {
			c.Next()
			return
		}
		// 支持 Header（X-Callback-Token）和 Query（?token=）两种传参
		headerToken := c.GetHeader("X-Callback-Token")
		queryToken := c.Query("token")
		headerMatched := tokenMatches(headerToken, expected)
		queryMatched := tokenMatches(queryToken, expected)
		if !headerMatched && !queryMatched {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"ok": false, "message": "unauthorized"})
			return
		}
		// 记录认证来源，后续 handler 可用
		if headerMatched {
			c.Set("auth_mode", "header")
		} else if queryMatched {
			c.Set("auth_mode", "query")
		}
		c.Next()
	}
}

// GatewayBearer 校验 Authorization: Bearer <token>，用于保护 /v1 网关路由。
func GatewayBearer(token string) gin.HandlerFunc {
	return func(c *gin.Context) {
		expected := strings.TrimSpace(token)
		if expected == "" {
			c.Next()
			return
		}
		provided := parseBearerToken(c.GetHeader("Authorization"))
		if !tokenMatches(provided, expected) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"detail": "Unauthorized"})
			return
		}
		c.Next()
	}
}

func parseBearerToken(authHeader string) string {
	fields := strings.Fields(authHeader)
	if len(fields) != 2 || !strings.EqualFold(fields[0], "Bearer") {
		return ""
	}
	return fields[1]
}

func tokenMatches(provided, expected string) bool {
	provided = strings.TrimSpace(provided)
	expected = strings.TrimSpace(expected)
	if provided == "" || expected == "" || len(provided) != len(expected) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) == 1
}
