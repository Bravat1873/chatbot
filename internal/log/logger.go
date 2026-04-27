// 日志层：封装 slog，统一输出 JSON 格式到 stdout，支持 debug/warn/error 级别切换。
package log

import (
	"log/slog"
	"os"
	"strings"
)

// New 创建 slog.Logger，JSON 格式输出，日志级别通过环境变量控制。
func New(level string) *slog.Logger {
	logLevel := new(slog.LevelVar)
	logLevel.Set(parseLevel(level))
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel}))
}

// parseLevel 将字符串映射为 slog.Level，默认 Info。
func parseLevel(level string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
