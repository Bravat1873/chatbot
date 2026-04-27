package service

import (
	"context"

	"chatbot/internal/gateway"
)

const DefaultInitialPrompt = "您好，请问已经有师傅和您预约上门时间了吗？"

type DialogueService struct{}

func NewDialogueService() *DialogueService {
	return &DialogueService{}
}

func (s *DialogueService) ProcessTurn(ctx context.Context, req gateway.TurnRequest) (gateway.TurnResponse, error) {
	_ = ctx
	if req.UserText == "" {
		return gateway.TurnResponse{Reply: DefaultInitialPrompt, Status: "in_progress"}, nil
	}
	return gateway.TurnResponse{Reply: "好的，已经为您记录。请问本次服务您满意吗？", Status: "in_progress"}, nil
}
