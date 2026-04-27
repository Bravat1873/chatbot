package core

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHeuristicIntentDetectsYesAndNo(t *testing.T) {
	classifier := NewHeuristicIntentClassifier()

	yes, err := classifier.Classify(context.Background(), "嗯对，已经约过了", IntentContext{ExpectedIntent: "yes_no"})
	require.NoError(t, err)
	no, err := classifier.Classify(context.Background(), "还没呢，没有预约", IntentContext{ExpectedIntent: "yes_no"})
	require.NoError(t, err)

	assert.Equal(t, "yes", yes.Intent)
	assert.Equal(t, "no", no.Intent)
}

func TestHeuristicIntentDetectsAddress(t *testing.T) {
	classifier := NewHeuristicIntentClassifier()

	result, err := classifier.Classify(context.Background(), "北京市朝阳区建国路88号SOHO现代城", IntentContext{ExpectedIntent: "address"})
	require.NoError(t, err)

	assert.Equal(t, "address", result.Intent)
	assert.Contains(t, result.Address, "88号")
}
