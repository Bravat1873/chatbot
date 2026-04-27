package core

import (
	"context"
	"strings"
	"testing"
)

func TestDialogueEngineHappyPath(t *testing.T) {
	engine := NewDialogueEngine(NewHeuristicIntentClassifier(), DefaultFlowSteps())

	firstReply, _, err := engine.ProcessTurn(context.Background(), "session-1", "", nil)
	if err != nil {
		t.Fatalf("first turn: %v", err)
	}
	secondReply, _, err := engine.ProcessTurn(context.Background(), "session-1", "有的，已经预约了", nil)
	if err != nil {
		t.Fatalf("second turn: %v", err)
	}
	finalReply, status, err := engine.ProcessTurn(context.Background(), "session-1", "满意，已经解决了", nil)
	if err != nil {
		t.Fatalf("final turn: %v", err)
	}

	state := engine.Snapshot("session-1")
	if firstReply != DefaultSteps[0].Question {
		t.Fatalf("unexpected first reply: %q", firstReply)
	}
	if !strings.Contains(secondReply, "好的，已经为您记录。") || !strings.Contains(secondReply, DefaultSteps[1].Question) {
		t.Fatalf("unexpected second reply: %q", secondReply)
	}
	if !strings.Contains(finalReply, "好的，记录到您比较满意。") || !strings.Contains(finalReply, EndMessage) {
		t.Fatalf("unexpected final reply: %q", finalReply)
	}
	if status != "completed" || state.Status != "completed" {
		t.Fatalf("expected completed status, got response=%s state=%s", status, state.Status)
	}
	if state.Results["appointment_confirmed"]["intent"] != "yes" || state.Results["service_satisfied"]["intent"] != "yes" {
		t.Fatalf("unexpected results: %#v", state.Results)
	}
}

func TestDialogueEngineStoresBizParams(t *testing.T) {
	engine := NewDialogueEngine(NewHeuristicIntentClassifier(), DefaultFlowSteps())

	_, _, err := engine.ProcessTurn(context.Background(), "session-biz", "", map[string]any{
		"customer_name": "张三",
		"order_id":      "BL-001",
	})
	if err != nil {
		t.Fatalf("process turn: %v", err)
	}

	state := engine.Snapshot("session-biz")
	if state.BizParams["customer_name"] != "张三" || state.BizParams["order_id"] != "BL-001" {
		t.Fatalf("unexpected biz params: %#v", state.BizParams)
	}
}

func TestDialogueEngineHandlesSilenceThenTermination(t *testing.T) {
	engine := NewDialogueEngine(NewHeuristicIntentClassifier(), DefaultFlowSteps())

	_, _, _ = engine.ProcessTurn(context.Background(), "session-2", "", nil)
	retryReply, _, err := engine.ProcessTurn(context.Background(), "session-2", "用户没有说话", nil)
	if err != nil {
		t.Fatalf("retry turn: %v", err)
	}
	finalReply, status, err := engine.ProcessTurn(context.Background(), "session-2", "用户没有说话", nil)
	if err != nil {
		t.Fatalf("final turn: %v", err)
	}

	state := engine.Snapshot("session-2")
	if retryReply != TimeoutRetryPrompt {
		t.Fatalf("expected retry prompt, got %q", retryReply)
	}
	if finalReply != TimeoutEndMessage || status != "terminated" {
		t.Fatalf("expected termination, got reply=%q status=%s", finalReply, status)
	}
	if !state.Finished || state.Results["appointment_confirmed"]["status"] != "timeout" {
		t.Fatalf("unexpected state: %#v", state)
	}
}

func TestDialogueEngineReturnsEndAfterFinish(t *testing.T) {
	engine := NewDialogueEngine(NewHeuristicIntentClassifier(), []DialogueStep{DefaultSteps[0]})

	_, _, _ = engine.ProcessTurn(context.Background(), "session-3", "", nil)
	_, _, _ = engine.ProcessTurn(context.Background(), "session-3", "有的，已经预约了", nil)
	reply, _, err := engine.ProcessTurn(context.Background(), "session-3", "继续", nil)
	if err != nil {
		t.Fatalf("after finish: %v", err)
	}

	if reply != EndMessage {
		t.Fatalf("expected end message, got %q", reply)
	}
}
