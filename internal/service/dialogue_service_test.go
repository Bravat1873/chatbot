package service

import (
	"context"
	"testing"

	"chatbot/internal/core"
	"chatbot/internal/gateway"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDialogueServiceDefaultsToAppointmentFlow(t *testing.T) {
	svc := NewDialogueService()

	resp, err := svc.ProcessTurn(context.Background(), gateway.TurnRequest{
		SessionID: "session-default",
	})

	require.NoError(t, err)
	assert.Equal(t, core.DefaultSteps[0].Question, resp.Reply)
}

func TestDialogueServiceSelectsAddressFlow(t *testing.T) {
	svc := NewDialogueService()

	resp, err := svc.ProcessTurn(context.Background(), gateway.TurnRequest{
		SessionID: "session-address",
		BizParams: map[string]any{
			"biz_type": "address_verify",
		},
	})

	require.NoError(t, err)
	assert.Equal(t, core.DefaultSteps[2].Question, resp.Reply)
}
