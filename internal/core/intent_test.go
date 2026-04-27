package core

import (
	"context"
	"strings"
	"testing"
)

func TestHeuristicIntentDetectsYesAndNo(t *testing.T) {
	classifier := NewHeuristicIntentClassifier()

	yes, err := classifier.Classify(context.Background(), "嗯对，已经约过了", IntentContext{ExpectedIntent: "yes_no"})
	if err != nil {
		t.Fatalf("classify yes: %v", err)
	}
	no, err := classifier.Classify(context.Background(), "还没呢，没有预约", IntentContext{ExpectedIntent: "yes_no"})
	if err != nil {
		t.Fatalf("classify no: %v", err)
	}

	if yes.Intent != "yes" {
		t.Fatalf("expected yes, got %#v", yes)
	}
	if no.Intent != "no" {
		t.Fatalf("expected no, got %#v", no)
	}
}

func TestHeuristicIntentDetectsAddress(t *testing.T) {
	classifier := NewHeuristicIntentClassifier()

	result, err := classifier.Classify(context.Background(), "北京市朝阳区建国路88号SOHO现代城", IntentContext{ExpectedIntent: "address"})
	if err != nil {
		t.Fatalf("classify address: %v", err)
	}

	if result.Intent != "address" || !strings.Contains(result.Address, "88号") {
		t.Fatalf("expected address with 88号, got %#v", result)
	}
}
