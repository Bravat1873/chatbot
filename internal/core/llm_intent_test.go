package core

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	require.NoError(t, err)

	assert.Equal(t, "yes", result.Intent)
	assert.Equal(t, "llm", result.Source)
	assert.Equal(t, 20, llm.req.MaxTokens)
	assert.NotEmpty(t, llm.req.SystemPrompt)
	assert.NotEmpty(t, llm.req.UserPrompt)
}

func TestLLMIntentClassifierUsesLLMWhenConfiguredEvenIfHeuristicIsCertain(t *testing.T) {
	llm := &fakeLLMClient{content: `{"intent":"no"}`}
	classifier := NewLLMIntentClassifier(llm)

	result, err := classifier.Classify(context.Background(), "有的", IntentContext{ExpectedIntent: "yes_no"})
	require.NoError(t, err)

	assert.Equal(t, "no", result.Intent)
	assert.Equal(t, "llm", result.Source)
	assert.Equal(t, 1, llm.calls, "llm should be called once")
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
	require.NoError(t, err)

	assert.Equal(t, "您说的是龙吟大街8号对吗？", prompt)
	assert.Equal(t, 1, llm.calls)
	assert.Equal(t, 64, llm.req.MaxTokens)
	assert.Equal(t, float64(0.2), llm.req.Temperature)
	assert.Contains(t, llm.req.UserPrompt, "龙吟大街8")
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
	require.NoError(t, err)

	assert.Contains(t, prompt, "贝朗公司", "named place should be preserved")
}
