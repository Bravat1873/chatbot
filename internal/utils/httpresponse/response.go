// 统一 HTTP 响应格式：提供 OK/Error/AbortError 辅助方法。
package httpresponse

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// 业务错误码常量
const (
	CodeOK                 = "OK"
	CodeInvalidRequest     = "INVALID_REQUEST"
	CodeUnauthorized       = "UNAUTHORIZED"
	CodeInternalError      = "INTERNAL_ERROR"
	CodeServiceUnavailable = "SERVICE_UNAVAILABLE"
	CodeUpstreamError      = "UPSTREAM_ERROR"
)

// Body 统一响应体结构。
type Body struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data"`
}

// OK 返回成功响应（200）。
func OK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, Body{
		Code:    CodeOK,
		Message: "success",
		Data:    data,
	})
}

// Error 返回业务错误，调用方自行决定 HTTP 状态码。
func Error(c *gin.Context, status int, code, message string, data any) {
	c.JSON(status, Body{
		Code:    code,
		Message: message,
		Data:    data,
	})
}

// AbortError 返回错误并终止后续 handler 链（用于中间件）。
func AbortError(c *gin.Context, status int, code, message string, data any) {
	c.AbortWithStatusJSON(status, Body{
		Code:    code,
		Message: message,
		Data:    data,
	})
}
