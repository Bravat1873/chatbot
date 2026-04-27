// handler/callbacks.go: 处理阿里云 AICCS 回调，解析 JSON/Form 格式的报告并写入数据库。
package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// HandleAICCSReport 接收并处理阿里云 AICCS 通话报告回调，返回是否重复/匹配。
func HandleAICCSReport(logger *slog.Logger, svc CallbackService) gin.HandlerFunc {
	return func(c *gin.Context) {
		payload, rawPayload, err := parsePayload(c)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "invalid callback payload"})
			return
		}

		authMode, _ := c.Get("auth_mode")
		duplicate, matched, err := svc.HandleCallReport(
			c.Request.Context(),
			payload,
			rawPayload,
			c.ClientIP(),
			stringValue(authMode),
		)
		if err != nil {
			if logger != nil {
				logger.Error("handle_aiccs_report_failed", "error", err.Error())
			}
			c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": "internal server error"})
			return
		}

		response := gin.H{"ok": true}
		if duplicate {
			response["duplicate"] = true
		}
		if !matched {
			response["matched"] = false
		}
		c.JSON(http.StatusOK, response)
	}
}

// parsePayload 根据 Content-Type 解析回调 payload，兼容 JSON 和 form-urlencoded 两种格式。
func parsePayload(c *gin.Context) (map[string]any, []byte, error) {
	contentType := c.ContentType()
	switch {
	case strings.Contains(contentType, "application/json"):
		var payload map[string]any
		raw, err := c.GetRawData()
		if err != nil {
			return nil, nil, err
		}
		if len(raw) == 0 {
			payload = map[string]any{}
			return payload, []byte("{}"), nil
		}
		if err := json.Unmarshal(raw, &payload); err != nil {
			return nil, nil, err
		}
		return payload, raw, nil
	default:
		if err := c.Request.ParseForm(); err != nil {
			return nil, nil, err
		}
		payload := make(map[string]any, len(c.Request.PostForm))
		for key, values := range c.Request.PostForm {
			if len(values) == 1 {
				payload[key] = values[0]
				continue
			}
			list := make([]any, 0, len(values))
			for _, value := range values {
				list = append(list, value)
			}
			payload[key] = list
		}
		raw, err := json.Marshal(payload)
		if err != nil {
			return nil, nil, err
		}
		return payload, raw, nil
	}
}

// stringValue 安全地将 any 转为 string，非字符串时返回空。
func stringValue(value any) string {
	text, _ := value.(string)
	return text
}
