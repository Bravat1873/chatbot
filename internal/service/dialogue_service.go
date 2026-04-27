package service

import (
	"context"

	"chatbot/internal/core"
	"chatbot/internal/gateway"
)

const DefaultInitialPrompt = "您好，请问已经有师傅和您预约上门时间了吗？"

type DialogueService struct {
	engine *core.DialogueEngine
}

type DialogueOption func(*dialogueDeps)

type dialogueDeps struct {
	classifier core.IntentClassifier
	geocoder   core.Geocoder
}

func WithIntentClassifier(classifier core.IntentClassifier) DialogueOption {
	return func(deps *dialogueDeps) {
		deps.classifier = classifier
	}
}

func WithGeocoder(geocoder core.Geocoder) DialogueOption {
	return func(deps *dialogueDeps) {
		deps.geocoder = geocoder
	}
}

func NewDialogueService(options ...DialogueOption) *DialogueService {
	deps := dialogueDeps{classifier: core.NewHeuristicIntentClassifier()}
	for _, option := range options {
		option(&deps)
	}
	return &DialogueService{
		engine: core.NewDialogueEngine(deps.classifier, core.DefaultFlowSteps(), deps.geocoder),
	}
}

func (s *DialogueService) ProcessTurn(ctx context.Context, req gateway.TurnRequest) (gateway.TurnResponse, error) {
	reply, status, err := s.engine.ProcessTurn(ctx, req.SessionID, req.UserText, req.BizParams)
	return gateway.TurnResponse{Reply: reply, Status: status}, err
}
