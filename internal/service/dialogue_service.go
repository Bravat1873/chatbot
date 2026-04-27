// 对话服务层：封装 core.DialogueEngine，提供 Turn 级别的请求处理。
package service

import (
	"context"

	"chatbot/internal/core"
	"chatbot/internal/gateway"
)

// DefaultInitialPrompt 对话启动时的问候语。
const DefaultInitialPrompt = "您好，请问已经有师傅和您预约上门时间了吗？"

// DialogueService 对话服务，持有 core.DialogueEngine 实例。
type DialogueService struct {
	engine *core.DialogueEngine
}

// DialogueOption 函数选项模式，用于注入 IntentClassifier / Geocoder 等可选依赖。
type DialogueOption func(*dialogueDeps)

// dialogueDeps 对话服务的可选依赖集合。
type dialogueDeps struct {
	classifier core.IntentClassifier
	geocoder   core.Geocoder
}

// WithIntentClassifier 注入自定义意图分类器（如 LLM 分类器），未注入时使用启发式默认。
func WithIntentClassifier(classifier core.IntentClassifier) DialogueOption {
	return func(deps *dialogueDeps) {
		deps.classifier = classifier
	}
}

// WithGeocoder 注入地址解析服务，未注入时跳过地址验证。
func WithGeocoder(geocoder core.Geocoder) DialogueOption {
	return func(deps *dialogueDeps) {
		deps.geocoder = geocoder
	}
}

// NewDialogueService 创建对话服务，通过 Option 注入 LLM 分类器和 Geocoder。
func NewDialogueService(options ...DialogueOption) *DialogueService {
	deps := dialogueDeps{classifier: core.NewHeuristicIntentClassifier()}
	for _, option := range options {
		option(&deps)
	}
	return &DialogueService{
		engine: core.NewDialogueEngine(deps.classifier, core.DefaultFlowSteps(), deps.geocoder),
	}
}

// ProcessTurn 处理一轮对话，委托给底层 DialogueEngine 并适配返回格式。
func (s *DialogueService) ProcessTurn(ctx context.Context, req gateway.TurnRequest) (gateway.TurnResponse, error) {
	reply, status, err := s.engine.ProcessTurn(ctx, req.SessionID, req.UserText, req.BizParams)
	return gateway.TurnResponse{Reply: reply, Status: status}, err
}
