package core

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDialogueEngineHappyPath(t *testing.T) {
	engine := NewDialogueEngine(NewHeuristicIntentClassifier(), DefaultFlowSteps())

	firstReply, _, err := engine.ProcessTurn(context.Background(), "session-1", "", nil)
	require.NoError(t, err)
	secondReply, _, err := engine.ProcessTurn(context.Background(), "session-1", "有的，已经预约了", nil)
	require.NoError(t, err)
	finalReply, status, err := engine.ProcessTurn(context.Background(), "session-1", "满意，已经解决了", nil)
	require.NoError(t, err)

	state := engine.Snapshot("session-1")
	assert.Equal(t, DefaultSteps[0].Question, firstReply)
	assert.Contains(t, secondReply, "好的，已经为您记录。")
	assert.Contains(t, secondReply, DefaultSteps[1].Question)
	assert.Contains(t, finalReply, "好的，记录到您比较满意。")
	assert.Contains(t, finalReply, EndMessage)
	assert.Equal(t, "completed", status)
	assert.Equal(t, "completed", state.Status)
	assert.Equal(t, "yes", state.Results["appointment_confirmed"]["intent"])
	assert.Equal(t, "yes", state.Results["service_satisfied"]["intent"])
}

func TestDialogueEngineStoresBizParams(t *testing.T) {
	engine := NewDialogueEngine(NewHeuristicIntentClassifier(), DefaultFlowSteps())

	_, _, err := engine.ProcessTurn(context.Background(), "session-biz", "", map[string]any{
		"customer_name": "张三",
		"order_id":      "BL-001",
	})
	require.NoError(t, err)

	state := engine.Snapshot("session-biz")
	assert.Equal(t, "张三", state.BizParams["customer_name"])
	assert.Equal(t, "BL-001", state.BizParams["order_id"])
}

func TestDialogueEngineHandlesSilenceThenTermination(t *testing.T) {
	engine := NewDialogueEngine(NewHeuristicIntentClassifier(), DefaultFlowSteps())

	_, _, _ = engine.ProcessTurn(context.Background(), "session-2", "", nil)
	retryReply, _, err := engine.ProcessTurn(context.Background(), "session-2", "用户没有说话", nil)
	require.NoError(t, err)
	finalReply, status, err := engine.ProcessTurn(context.Background(), "session-2", "用户没有说话", nil)
	require.NoError(t, err)

	state := engine.Snapshot("session-2")
	assert.Equal(t, TimeoutRetryPrompt, retryReply)
	assert.Equal(t, TimeoutEndMessage, finalReply)
	assert.Equal(t, "terminated", status)
	assert.True(t, state.Finished)
	assert.Equal(t, "timeout", state.Results["appointment_confirmed"]["status"])
}

func TestDialogueEngineReturnsEndAfterFinish(t *testing.T) {
	engine := NewDialogueEngine(NewHeuristicIntentClassifier(), []DialogueStep{DefaultSteps[0]})

	_, _, _ = engine.ProcessTurn(context.Background(), "session-3", "", nil)
	_, _, _ = engine.ProcessTurn(context.Background(), "session-3", "有的，已经预约了", nil)
	reply, _, err := engine.ProcessTurn(context.Background(), "session-3", "继续", nil)
	require.NoError(t, err)

	assert.Equal(t, EndMessage, reply)
}

type fakeGeocoder struct{}

func (f *fakeGeocoder) ResolvePlace(ctx context.Context, keywords string) (GeocodeResult, error) {
	_ = ctx
	_ = keywords
	return GeocodeResult{
		Found: true,
		Best: &PlaceCandidate{
			Name:        "小家公寓",
			Address:     "仑头村仑头路82号",
			District:    "海珠区",
			DisplayText: "海珠区仑头村仑头路82号小家公寓",
		},
	}, nil
}

func TestDialogueEngineAddressConfirmation(t *testing.T) {
	engine := NewDialogueEngine(NewHeuristicIntentClassifier(), []DialogueStep{DefaultSteps[2]}, &fakeGeocoder{})

	firstReply, _, _ := engine.ProcessTurn(context.Background(), "session-4", "", nil)
	confirmReply, _, err := engine.ProcessTurn(context.Background(), "session-4", "广州海珠区轮头村八二路小家公寓", nil)
	require.NoError(t, err)
	finalReply, _, err := engine.ProcessTurn(context.Background(), "session-4", "是的", nil)
	require.NoError(t, err)

	state := engine.Snapshot("session-4")
	assert.Equal(t, DefaultSteps[2].Question, firstReply)
	assert.Contains(t, confirmReply, "小家公寓")
	assert.Contains(t, confirmReply, "仑头村仑头路82号")
	assert.Contains(t, finalReply, EndMessage)
	assert.Equal(t, "ok", state.Results["address"]["status"])
}

type blockingClassifier struct {
	started chan struct{}
	release chan struct{}
}

func (c *blockingClassifier) Classify(ctx context.Context, text string, intentContext IntentContext) (IntentResult, error) {
	_ = ctx
	_ = text
	_ = intentContext
	c.started <- struct{}{}
	<-c.release
	return IntentResult{Intent: "yes", Source: "test"}, nil
}

func (c *blockingClassifier) GenerateAddressConfirmation(ctx context.Context, input AddressConfirmationInput) (string, error) {
	_ = ctx
	return input.FallbackPrompt, nil
}

func TestDialogueEngineDoesNotSerializeDifferentSessions(t *testing.T) {
	classifier := &blockingClassifier{
		started: make(chan struct{}, 2),
		release: make(chan struct{}),
	}
	engine := NewDialogueEngine(classifier, []DialogueStep{DefaultSteps[0]})

	var wg sync.WaitGroup
	wg.Add(2)
	for _, sessionID := range []string{"session-a", "session-b"} {
		go func(sessionID string) {
			defer wg.Done()
			_, _, _ = engine.ProcessTurn(context.Background(), sessionID, "当然", nil)
		}(sessionID)
	}

	for i := 0; i < 2; i++ {
		select {
		case <-classifier.started:
		case <-time.After(200 * time.Millisecond):
			close(classifier.release)
			t.Fatal("different sessions should not wait on one global dialogue lock")
		}
	}
	close(classifier.release)
	wg.Wait()
}
