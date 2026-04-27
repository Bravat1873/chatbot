package core

import (
	"context"
	"sync"
)

const (
	EndMessage         = "本次回访结束，感谢您的配合。"
	TimeoutEndMessage  = "长时间未收到回应，本次回访先结束。"
	TimeoutRetryPrompt = "您好，请问您还在吗？"
	AddressRetryPrompt = "我还没完全核对到这个地址，请再详细说一下，尽量包含小区、路名和门牌号。"
)

type DialogueStep struct {
	Key            string
	Question       string
	ExpectedIntent string
	RetryPrompt    string
}

var DefaultSteps = []DialogueStep{
	{
		Key:            "appointment_confirmed",
		Question:       "您好，请问已经有师傅和您预约上门时间了吗？",
		ExpectedIntent: "yes_no",
		RetryPrompt:    "抱歉，我没有听清。请问是否已经预约上门时间了？",
	},
	{
		Key:            "service_satisfied",
		Question:       "请问本次服务您满意吗？",
		ExpectedIntent: "yes_no",
		RetryPrompt:    "抱歉，我再确认一下，您对本次服务是否满意？",
	},
	{
		Key:            "address",
		Question:       "为了方便核对，请您说一下详细地址，尽量具体到门牌号。",
		ExpectedIntent: "address",
		RetryPrompt:    "地址还不够清楚，请您再说一遍，尽量包含路名和门牌号。",
	},
}

func DefaultFlowSteps() []DialogueStep {
	return append([]DialogueStep(nil), DefaultSteps[:2]...)
}

type SessionState struct {
	StepIndex               int
	UnclearRetries          int
	TimeoutRetries          int
	AddressRetries          int
	Results                 map[string]map[string]any
	BizParams               map[string]any
	Transcript              []map[string]string
	Finished                bool
	Status                  string
	AwaitingAddressConfirm  bool
	PendingAddressCandidate map[string]any
	PendingAddressText      string
}

type DialogueEngine struct {
	mu                sync.Mutex
	sessions          map[string]*SessionState
	classifier        IntentClassifier
	steps             []DialogueStep
	maxUnclearRetries int
	maxTimeoutRetries int
	maxAddressRetries int
}

func NewDialogueEngine(classifier IntentClassifier, steps []DialogueStep) *DialogueEngine {
	if classifier == nil {
		classifier = NewHeuristicIntentClassifier()
	}
	if len(steps) == 0 {
		steps = DefaultFlowSteps()
	}
	return &DialogueEngine{
		sessions:          make(map[string]*SessionState),
		classifier:        classifier,
		steps:             append([]DialogueStep(nil), steps...),
		maxUnclearRetries: 2,
		maxTimeoutRetries: 1,
		maxAddressRetries: 2,
	}
}

func (e *DialogueEngine) ProcessTurn(ctx context.Context, sessionID string, userText string, bizParams map[string]any) (string, string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	state := e.getOrCreate(sessionID)
	if bizParams != nil {
		state.BizParams = copyMap(bizParams)
	}
	if state.Finished {
		return EndMessage, state.Status, nil
	}

	step := e.currentStep(state)
	if step == nil {
		state.Finished = true
		state.Status = "completed"
		return EndMessage, state.Status, nil
	}

	if userText == "" {
		prompt := e.currentPrompt(state)
		return recordBotReply(state, prompt), state.Status, nil
	}

	prompt := e.currentPrompt(state)
	ensurePromptRecorded(state, prompt)
	recordUserTurn(state, userText)

	if userText == "用户没有说话" {
		return e.handleSilence(state, *step), state.Status, nil
	}

	classified, err := e.classifier.Classify(ctx, userText, IntentContext{
		Stage:          step.Key,
		ExpectedIntent: step.ExpectedIntent,
		Question:       prompt,
	})
	if err != nil {
		classified = IntentResult{Intent: "unclear", Confidence: "low", Source: "fallback", RawText: userText}
	}

	switch step.ExpectedIntent {
	case "yes_no":
		return e.handleYesNoStep(state, *step, userText, classified), state.Status, nil
	case "address":
		return e.handleAddressStep(state, *step, userText, classified), state.Status, nil
	default:
		return e.completeUnclearStep(state, *step, userText, classified.Intent), state.Status, nil
	}
}

func (e *DialogueEngine) Snapshot(sessionID string) *SessionState {
	e.mu.Lock()
	defer e.mu.Unlock()
	state := e.sessions[sessionID]
	if state == nil {
		return nil
	}
	copied := *state
	return &copied
}

func (e *DialogueEngine) getOrCreate(sessionID string) *SessionState {
	if state, ok := e.sessions[sessionID]; ok {
		return state
	}
	state := &SessionState{
		Results:   make(map[string]map[string]any),
		BizParams: make(map[string]any),
		Status:    "in_progress",
	}
	e.sessions[sessionID] = state
	return state
}

func (e *DialogueEngine) currentStep(state *SessionState) *DialogueStep {
	if state.StepIndex >= len(e.steps) {
		return nil
	}
	return &e.steps[state.StepIndex]
}

func (e *DialogueEngine) currentPrompt(state *SessionState) string {
	step := e.currentStep(state)
	if step == nil {
		return EndMessage
	}
	if state.TimeoutRetries > 0 {
		return TimeoutRetryPrompt
	}
	if step.ExpectedIntent == "address" && state.AddressRetries > 0 {
		return AddressRetryPrompt
	}
	if state.UnclearRetries > 0 {
		return step.RetryPrompt
	}
	return step.Question
}

func (e *DialogueEngine) handleYesNoStep(state *SessionState, step DialogueStep, userText string, classified IntentResult) string {
	if classified.Intent == "yes" || classified.Intent == "no" {
		state.Results[step.Key] = map[string]any{"status": "ok", "intent": classified.Intent, "text": userText}
		return e.advanceWithReply(state, buildYesNoReply(step.Key, classified.Intent))
	}
	if state.UnclearRetries < e.maxUnclearRetries {
		state.UnclearRetries++
		return recordBotReply(state, step.RetryPrompt)
	}
	return e.completeUnclearStep(state, step, userText, classified.Intent)
}

func (e *DialogueEngine) handleAddressStep(state *SessionState, step DialogueStep, userText string, classified IntentResult) string {
	state.Results["address"] = map[string]any{"status": "unverified", "text": userText, "intent": classified.Intent}
	return e.advanceWithReply(state, "好的，地址已记录。当前环境暂时无法完成地点搜索。")
}

func (e *DialogueEngine) handleSilence(state *SessionState, step DialogueStep) string {
	if state.TimeoutRetries < e.maxTimeoutRetries {
		state.TimeoutRetries++
		return recordBotReply(state, TimeoutRetryPrompt)
	}
	state.Results[step.Key] = map[string]any{"status": "timeout"}
	state.Finished = true
	state.Status = "terminated"
	return recordBotReply(state, TimeoutEndMessage)
}

func (e *DialogueEngine) completeUnclearStep(state *SessionState, step DialogueStep, userText string, intent string) string {
	state.Results[step.Key] = map[string]any{"status": "unclear", "text": userText, "intent": intent}
	return e.advanceWithReply(state, "好的，这一项我先为您标记为待确认。")
}

func (e *DialogueEngine) advanceWithReply(state *SessionState, prefix string) string {
	state.StepIndex++
	state.UnclearRetries = 0
	state.TimeoutRetries = 0
	state.AddressRetries = 0
	state.AwaitingAddressConfirm = false
	state.PendingAddressCandidate = nil
	state.PendingAddressText = ""

	var reply string
	if state.StepIndex >= len(e.steps) {
		state.Finished = true
		state.Status = "completed"
		reply = prefix + EndMessage
	} else {
		state.Status = "in_progress"
		reply = prefix + e.steps[state.StepIndex].Question
	}
	return recordBotReply(state, reply)
}

func buildYesNoReply(stepKey string, intent string) string {
	if stepKey == "service_satisfied" {
		if intent == "yes" {
			return "好的，记录到您比较满意。"
		}
		return "抱歉，给您造成了不愉快的体验。"
	}
	if intent == "yes" {
		return "好的，已经为您记录。"
	}
	return "好的，已经记录您这边还没有。"
}

func recordBotReply(state *SessionState, text string) string {
	state.Transcript = append(state.Transcript, map[string]string{"speaker": "bot", "text": text})
	return text
}

func recordUserTurn(state *SessionState, text string) {
	state.Transcript = append(state.Transcript, map[string]string{"speaker": "user", "text": text})
}

func ensurePromptRecorded(state *SessionState, prompt string) {
	if len(state.Transcript) == 0 {
		recordBotReply(state, prompt)
		return
	}
	last := state.Transcript[len(state.Transcript)-1]
	if last["speaker"] != "bot" || last["text"] != prompt {
		recordBotReply(state, prompt)
	}
}

func copyMap(input map[string]any) map[string]any {
	output := make(map[string]any, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}
