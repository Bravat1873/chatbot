// 路由层：使用 gin 框架注册 HTTP 路由及中间件，将请求委托给 service 层。
package handler

import (
	"context"
	"encoding/json"
	"log/slog"

	"chatbot/internal/middleware"
	"chatbot/internal/model"

	"github.com/gin-gonic/gin"
)

// CallService 定义创建外呼任务的接口，供 handler 依赖注入。
type CallService interface {
	CreateCallTask(ctx context.Context, req model.CreateCallTaskRequest) (*model.CallTask, error)
}

// CallbackService 定义处理运营商回调报告的接口。
type CallbackService interface {
	HandleCallReport(ctx context.Context, payload map[string]any, rawPayload json.RawMessage, sourceIP, authMode string) (duplicate bool, matched bool, err error)
}

// RouterDeps 聚合所有路由所需的依赖，便于测试时替换。
type RouterDeps struct {
	Logger           *slog.Logger
	CallService      CallService
	CallbackService  CallbackService
	InternalAPIToken string
	CallbackAPIToken string
	HealthCheck      func(context.Context) error
}

// NewRouter 创建 gin Engine，注册全局中间件和业务路由分组。
func NewRouter(deps RouterDeps) *gin.Engine {
	router := gin.New()
	router.Use(gin.Recovery())                           // panic 恢复
	router.Use(middleware.RequestLogger(deps.Logger))    // 请求日志

	// 健康检查路由
	router.GET("/healthz", Healthz(deps.HealthCheck))

	// 内部 API：创建外呼任务（需要 X-Internal-Token 鉴权）
	internalGroup := router.Group("/internal")
	internalGroup.Use(middleware.InternalToken(deps.InternalAPIToken))
	internalGroup.POST("/calls", CreateCallTask(deps.Logger, deps.CallService))

	// 回调路由：接收运营商通话报告（支持 Header 和 Query 两种鉴权）
	callbackGroup := router.Group("/callbacks")
	callbackGroup.Use(middleware.CallbackToken(deps.CallbackAPIToken))
	callbackGroup.POST("/aiccs/report", HandleAICCSReport(deps.Logger, deps.CallbackService))

	return router
}
