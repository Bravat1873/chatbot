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

func NewDialogueService() *DialogueService {
	return &DialogueService{
		engine: core.NewDialogueEngine(core.NewHeuristicIntentClassifier(), core.DefaultFlowSteps()),
	}
}

func (s *DialogueService) ProcessTurn(ctx context.Context, req gateway.TurnRequest) (gateway.TurnResponse, error) {
	reply, status, err := s.engine.ProcessTurn(ctx, req.SessionID, req.UserText, req.BizParams)
	return gateway.TurnResponse{Reply: reply, Status: status}, err
}
