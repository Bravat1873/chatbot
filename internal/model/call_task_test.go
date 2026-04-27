package model

import (
	"testing"
	"time"
)

func TestDeriveNextStatusFromAnsweredHangupToCompleted(t *testing.T) {
	now := time.Now().UTC()
	status := DeriveNextStatus(CallStatusAnswered, CallReportPayload{HangupTime: &now})
	if status != CallStatusCompleted {
		t.Fatalf("expected completed, got %s", status)
	}
}

func TestDeriveNextStatusFromAcceptedRingToRinging(t *testing.T) {
	now := time.Now().UTC()
	status := DeriveNextStatus(CallStatusAccepted, CallReportPayload{RingTime: &now})
	if status != CallStatusRinging {
		t.Fatalf("expected ringing, got %s", status)
	}
}

func TestNormalizeStatusKeepsTerminalFailed(t *testing.T) {
	status := NormalizeStatus(CallStatusFailed, CallStatusAnswered)
	if status != CallStatusFailed {
		t.Fatalf("expected failed to remain terminal, got %s", status)
	}
}
