// 程序入口：启动 HTTP 服务，加载配置、连接数据库、组装依赖并监听优雅退出信号。
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"chatbot/internal/config"
	"chatbot/internal/core"
	"chatbot/internal/db"
	"chatbot/internal/handler"
	internallog "chatbot/internal/log"
	"chatbot/internal/provider/aliyun"
	"chatbot/internal/provider/amap"
	"chatbot/internal/provider/dashscope"
	"chatbot/internal/repository"
	"chatbot/internal/service"
)

func main() {
	// 监听 SIGINT/SIGTERM，支持优雅关闭
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// 从环境变量加载所有配置
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config failed: %v\n", err)
		os.Exit(1)
	}

	// 初始化结构化日志（JSON 格式输出到 stdout）
	logger := internallog.New(cfg.LogLevel)

	// 运行数据库迁移（goose + embed 方式）
	if err := db.RunMigrations(ctx, cfg.PostgresDSN()); err != nil {
		logger.Error("run migrations failed", "error", err.Error())
		os.Exit(1)
	}

	// 创建 pgx 连接池
	pool, err := db.NewPool(ctx, cfg.PostgresDSN())
	if err != nil {
		logger.Error("create postgres pool failed", "error", err.Error())
		os.Exit(1)
	}
	defer pool.Close()

	// 初始化阿里云 AICCS 外呼 Provider
	provider, err := aliyun.New(aliyun.Config{
		AccessKeyID:     cfg.AliyunAccessKeyID,
		AccessKeySecret: cfg.AliyunAccessKeySecret,
		RegionID:        cfg.AICCSRegionID,
		Endpoint:        cfg.AICCSEndpoint,
	})
	if err != nil {
		logger.Error("create aliyun provider failed", "error", err.Error())
		os.Exit(1)
	}

	// 组装依赖链：仓库 -> 服务 -> 路由
	repo := repository.NewCallTaskRepository(pool)
	callService := service.NewCallService(logger, repo, provider, cfg.CallerNumber, cfg.AICCSAppCode, cfg.SessionTimeoutSeconds)
	dialogueOptions := make([]service.DialogueOption, 0, 2)
	if cfg.DashScopeAPIKey != "" {
		llmClient := dashscope.New(dashscope.Config{
			APIKey:  cfg.DashScopeAPIKey,
			BaseURL: cfg.DashScopeBaseURL,
			Model:   cfg.LLMModel,
			Timeout: time.Duration(cfg.DashScopeTimeout) * time.Second,
		})
		dialogueOptions = append(dialogueOptions, service.WithIntentClassifier(core.NewLLMIntentClassifier(llmClient)))
	}
	if cfg.AMapKey != "" {
		dialogueOptions = append(dialogueOptions, service.WithGeocoder(amap.New(amap.Config{
			APIKey:  cfg.AMapKey,
			BaseURL: cfg.AMapBaseURL,
			City:    cfg.AMapCity,
			Timeout: time.Duration(cfg.AMapTimeout) * time.Second,
		})))
	}
	dialogueService := service.NewDialogueService(dialogueOptions...)
	router := handler.NewRouter(handler.RouterDeps{
		Logger:              logger,
		CallService:         callService,
		CallbackService:     callService,
		CallContextResolver: callService,
		DialogueService:     dialogueService,
		InternalAPIToken:    cfg.InternalAPIToken,
		CallbackAPIToken:    cfg.AICCSCallbackToken,
		GatewayAuthToken:    cfg.GatewayAuthToken,
		DefaultLLMModel:     cfg.LLMModel,
		HealthCheck: func(ctx context.Context) error {
			return pool.Ping(ctx) // 健康检查：ping 数据库
		},
	})

	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.AppPort),
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second, // 防止慢客户端攻击
	}

	// 在后台 goroutine 启动 HTTP 服务
	go func() {
		logger.Info("server_started", "port", cfg.AppPort, "env", cfg.AppEnv)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server_listen_failed", "error", err.Error())
			stop()
		}
	}()

	// 等待关闭信号
	<-ctx.Done()

	// 最多等待 10 秒让已接收的请求处理完毕
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("server_shutdown_failed", "error", err.Error())
		os.Exit(1)
	}
	logger.Info("server_stopped")
}
