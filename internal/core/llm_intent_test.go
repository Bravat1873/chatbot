package core

import (
	"context"
	"testing"
)

type fakeLLMClient struct {
	content string
	err     error
	req     LLMRequest
}

func (f *fakeLLMClient) GenerateJSON(ctx context.Context, req LLMRequest) (string, error) {
	_ = ctx
	f.req = req
	return f.content, f.err
}

func TestLLMIntentClassifierUsesLLMWhenHeuristicIsUnclear(t *testing.T) {
	llm := &fakeLLMClient{content: `{"intent":"yes"}`}
	classifier := NewLLMIntentClassifier(llm)

	result, err := classifier.Classify(context.Background(), "当然", IntentContext{
		Stage:          "appointment_confirmed",
		ExpectedIntent: "yes_no",
		Question:       DefaultSteps[0].Question,
	})
	if err != nil {
		t.Fatalf("classify: %v", err)
	}

	if result.Intent != "yes" || result.Source != "llm" {
		t.Fatalf("expected llm yes, got %#v", result)
	}
	if llm.req.MaxTokens != 20 || llm.req.SystemPrompt == "" || llm.req.UserPrompt == "" {
		t.Fatalf("unexpected llm request: %#v", llm.req)
	}
}

func TestLLMIntentClassifierKeepsHeuristicWhenCertain(t *testing.T) {
	llm := &fakeLLMClient{content: `{"intent":"no"}`}
	classifier := NewLLMIntentClassifier(llm)

	result, err := classifier.Classify(context.Background(), "有的", IntentContext{ExpectedIntent: "yes_no"})
	if err != nil {
		t.Fatalf("classify: %v", err)
	}

	if result.Intent != "yes" || result.Source != "heuristic" {
		t.Fatalf("expected heuristic yes, got %#v", result)
	}
	if llm.req.UserPrompt != "" {
		t.Fatalf("llm should not be called, got request %#v", llm.req)
	}
}
