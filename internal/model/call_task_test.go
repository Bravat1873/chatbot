package model

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDeriveNextStatusFromAnsweredHangupToCompleted(t *testing.T) {
	now := time.Now().UTC()
	status := DeriveNextStatus(CallStatusAnswered, CallReportPayload{HangupTime: &now})
	assert.Equal(t, CallStatusCompleted, status)
}

func TestDeriveNextStatusFromAcceptedRingToRinging(t *testing.T) {
	now := time.Now().UTC()
	status := DeriveNextStatus(CallStatusAccepted, CallReportPayload{RingTime: &now})
	assert.Equal(t, CallStatusRinging, status)
}

func TestNormalizeStatusKeepsTerminalFailed(t *testing.T) {
	status := NormalizeStatus(CallStatusFailed, CallStatusAnswered)
	assert.Equal(t, CallStatusFailed, status)
}
