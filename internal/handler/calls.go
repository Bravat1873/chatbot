// 外呼任务 HTTP 处理器：接收创建外呼请求，参数校验后委托给 CallService。
package handler

import (
	"errors"
	"log/slog"
	"net/http"

	"chatbot/internal/model"
	"chatbot/internal/service"
	"chatbot/internal/utils/httpresponse"

	"github.com/gin-gonic/gin"
)

// CreateCallTask 创建外呼任务：解析 JSON 请求 -> 调用 service -> 返回 task_id。
func CreateCallTask(logger *slog.Logger, svc CallService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req model.CreateCallTaskRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			httpresponse.Error(c, http.StatusBadRequest, httpresponse.CodeInvalidRequest, "invalid request", nil)
			return
		}

		task, err := svc.CreateCallTask(c.Request.Context(), req)
		if err != nil {
			// 捕获 APIError，根据状态码区分 4xx 客户端错误和 5xx 上游错误
			var apiErr *service.APIError
			if errors.As(err, &apiErr) {
				code := httpresponse.CodeInvalidRequest
				if apiErr.StatusCode >= http.StatusInternalServerError {
					code = httpresponse.CodeUpstreamError
				}
				var data any
				if apiErr.TaskID != nil {
					data = gin.H{"task_id": apiErr.TaskID.String()}
				}
				httpresponse.Error(c, apiErr.StatusCode, code, apiErr.Message, data)
				return
			}
			if logger != nil {
				logger.Error("create_call_task_handler_failed", "error", err.Error())
			}
			httpresponse.Error(c, http.StatusInternalServerError, httpresponse.CodeInternalError, "internal server error", nil)
			return
		}

		response := gin.H{
			"task_id": task.TaskID.String(),
			"status":  task.Status,
		}
		if task.CallID != nil {
			response["call_id"] = *task.CallID
		}
		httpresponse.OK(c, response)
	}
}
