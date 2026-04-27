package core

import (
	"context"
	"strings"
	"testing"
)

type fakeLLMClient struct {
	content string
	err     error
	req     LLMRequest
	calls   int
}

func (f *fakeLLMClient) GenerateJSON(ctx context.Context, req LLMRequest) (string, error) {
	_ = ctx
	f.req = req
	f.calls++
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

func TestLLMIntentClassifierUsesLLMWhenConfiguredEvenIfHeuristicIsCertain(t *testing.T) {
	llm := &fakeLLMClient{content: `{"intent":"no"}`}
	classifier := NewLLMIntentClassifier(llm)

	result, err := classifier.Classify(context.Background(), "有的", IntentContext{ExpectedIntent: "yes_no"})
	if err != nil {
		t.Fatalf("classify: %v", err)
	}

	if result.Intent != "no" || result.Source != "llm" {
		t.Fatalf("expected llm no, got %#v", result)
	}
	if llm.calls != 1 {
		t.Fatalf("llm should be called once, got %d", llm.calls)
	}
}

func TestLLMIntentClassifierGeneratesAddressConfirmationPrompt(t *testing.T) {
	llm := &fakeLLMClient{content: "您说的是龙吟大街8号对吗？"}
	classifier := NewLLMIntentClassifier(llm)

	prompt, err := classifier.GenerateAddressConfirmation(context.Background(), AddressConfirmationInput{
		OriginalText:   "广州市海珠区官洲街道龙影大到8节9好",
		MatchedText:    "广东省广州市海珠区龙吟大街8",
		FocusText:      "龙吟大街8",
		FallbackPrompt: "回退话术",
	})
	if err != nil {
		t.Fatalf("generate prompt: %v", err)
	}

	if prompt != "您说的是龙吟大街8号对吗？" {
		t.Fatalf("unexpected prompt: %q", prompt)
	}
	if llm.calls != 1 || llm.req.MaxTokens != 64 || llm.req.Temperature != 0.2 {
		t.Fatalf("unexpected llm request: calls=%d req=%#v", llm.calls, llm.req)
	}
	if !strings.Contains(llm.req.UserPrompt, "龙吟大街8") {
		t.Fatalf("prompt should include focus text: %q", llm.req.UserPrompt)
	}
}

func TestLLMIntentClassifierAddsNamedPlaceWhenLLMOmitsIt(t *testing.T) {
	llm := &fakeLLMClient{content: "您是指琶洲大道东1号保利国际广场南塔吗？"}
	classifier := NewLLMIntentClassifier(llm)

	prompt, err := classifier.GenerateAddressConfirmation(context.Background(), AddressConfirmationInput{
		OriginalText: "保利国际南塔",
		MatchedText:  "琶洲大道东1号保利国际广场南塔贝朗公司",
		MatchedName:  "贝朗公司",
		FocusText:    "琶洲大道东1号保利国际广场南塔贝朗公司",
	})
	if err != nil {
		t.Fatalf("generate prompt: %v", err)
	}

	if !strings.Contains(prompt, "贝朗公司") {
		t.Fatalf("named place should be preserved, got %q", prompt)
	}
}
