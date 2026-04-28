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

// CallContextResolver 定义按 call_id 查找外呼任务上下文的接口，供文本网关缺失 biz_params 时兜底。
type CallContextResolver interface {
	ResolveCallBizParams(ctx context.Context, callID string) (map[string]any, bool, error)
}

// RouterDeps 聚合所有路由所需的依赖，便于测试时替换。
type RouterDeps struct {
	Logger              *slog.Logger
	CallService         CallService
	CallbackService     CallbackService
	CallContextResolver CallContextResolver
	DialogueService     DialogueService
	InternalAPIToken    string
	CallbackAPIToken    string
	GatewayAuthToken    string
	DefaultLLMModel     string
	HealthCheck         func(context.Context) error
}

// NewRouter 创建 gin Engine，注册全局中间件和业务路由分组。
func NewRouter(deps RouterDeps) *gin.Engine {
	router := gin.New()
	router.Use(gin.Recovery())                        // panic 恢复
	router.Use(middleware.RequestLogger(deps.Logger)) // 请求日志

	// 健康检查路由
	router.GET("/healthz", Healthz(deps.HealthCheck))

	// 文本网关：兼容 OpenAI / 阿里云大模型网关 SSE 协议
	gatewayGroup := router.Group("/v1")
	gatewayGroup.Use(middleware.GatewayBearer(deps.GatewayAuthToken))
	gatewayGroup.POST("/chat/completions", ChatCompletions(deps.Logger, deps.DialogueService, deps.DefaultLLMModel, deps.CallContextResolver))

	// 内部 API：创建外呼任务（需要 Authorization: Bearer 鉴权）
	internalGroup := router.Group("/internal")
	internalGroup.Use(middleware.InternalToken(deps.InternalAPIToken))
	internalGroup.POST("/calls", CreateCallTask(deps.Logger, deps.CallService))

	// 回调路由：接收运营商通话报告（支持 Header 和 Query 两种鉴权）
	callbackGroup := router.Group("/callbacks")
	callbackGroup.Use(middleware.CallbackToken(deps.CallbackAPIToken))
	callbackGroup.POST("/aiccs/report", HandleAICCSReport(deps.Logger, deps.CallbackService))

	return router
}
