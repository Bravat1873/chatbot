// 对话引擎：管理多轮对话状态机，驱动预约确认、满意度、地址采集三个阶段的流程。
package core

import (
	"context"
	"strings"
	"sync"
)

// 对话流程中的固定话术常量。
const (
	EndMessage         = "本次回访结束，感谢您的配合。"
	TimeoutEndMessage  = "长时间未收到回应，本次回访先结束。"
	TimeoutRetryPrompt = "您好，请问您还在吗？"
	AddressRetryPrompt = "我还没完全核对到这个地址，请再详细说一下，尽量包含小区、路名和门牌号。"
)

// DialogueStep 定义对话流程中的一个阶段：提问、期望意图与重试话术。
type DialogueStep struct {
	Key            string
	Question       string
	ExpectedIntent string
	RetryPrompt    string
}

// DefaultSteps 默认两阶段流程（预约确认 + 满意度），不含地址采集。
// 地址阶段只在同时配置了 LLM 和 Geocoder 时由 service 层动态追加。
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

// DefaultFlowSteps 返回默认流程步骤的副本，调用方可安全修改。
func DefaultFlowSteps() []DialogueStep {
	return append([]DialogueStep(nil), DefaultSteps[:2]...)
}

// SessionState 记录单个会话的运行态，包括当前步骤、重试计数、已采集结果和对话抄本。
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

// DialogueEngine 多轮对话状态机，按预设 DialogueStep 推进对话并采集用户意图与地址。
type DialogueEngine struct {
	mu                sync.Mutex
	sessions          map[string]*SessionState
	sessionLocks      map[string]*sync.Mutex
	classifier        IntentClassifier
	geocoder          Geocoder
	steps             []DialogueStep
	maxUnclearRetries int
	maxTimeoutRetries int
	maxAddressRetries int
}

// NewDialogueEngine 创建对话引擎实例，classifier 为 nil 时自动退化为启发式分类器。
func NewDialogueEngine(classifier IntentClassifier, steps []DialogueStep, geocoders ...Geocoder) *DialogueEngine {
	if classifier == nil {
		classifier = NewHeuristicIntentClassifier()
	}
	if len(steps) == 0 {
		steps = DefaultFlowSteps()
	}
	var geocoder Geocoder
	if len(geocoders) > 0 {
		geocoder = geocoders[0]
	}
	return &DialogueEngine{
		sessions:          make(map[string]*SessionState),
		sessionLocks:      make(map[string]*sync.Mutex),
		classifier:        classifier,
		geocoder:          geocoder,
		steps:             append([]DialogueStep(nil), steps...),
		maxUnclearRetries: 2,
		maxTimeoutRetries: 1,
		maxAddressRetries: 2,
	}
}

// ProcessTurn 处理一轮用户输入，返回 bot 回复文本和当前会话状态。
func (e *DialogueEngine) ProcessTurn(ctx context.Context, sessionID string, userText string, bizParams map[string]any) (string, string, error) {
	sessionLock := e.lockForSession(sessionID)
	sessionLock.Lock()
	defer sessionLock.Unlock()

	state := e.getOrCreate(sessionID)
	if bizParams != nil {
		state.BizParams = copyMap(bizParams)
	}
	if state.Finished {
		return EndMessage, state.Status, nil
	}

	if state.AwaitingAddressConfirm {
		return e.handleAddressConfirmation(ctx, state, userText), state.Status, nil
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
		return e.handleAddressStep(ctx, state, *step, userText, classified), state.Status, nil
	default:
		return e.completeUnclearStep(state, *step, userText, classified.Intent), state.Status, nil
	}
}

// Snapshot 返回指定会话状态的只读副本，用于外部监控或调试。
func (e *DialogueEngine) Snapshot(sessionID string) *SessionState {
	sessionLock := e.lockForSession(sessionID)
	sessionLock.Lock()
	defer sessionLock.Unlock()

	e.mu.Lock()
	defer e.mu.Unlock()
	state := e.sessions[sessionID]
	if state == nil {
		return nil
	}
	copied := *state
	return &copied
}

func (e *DialogueEngine) lockForSession(sessionID string) *sync.Mutex {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.sessionLocks == nil {
		e.sessionLocks = make(map[string]*sync.Mutex)
	}
	lock := e.sessionLocks[sessionID]
	if lock == nil {
		lock = &sync.Mutex{}
		e.sessionLocks[sessionID] = lock
	}
	return lock
}

func (e *DialogueEngine) getOrCreate(sessionID string) *SessionState {
	e.mu.Lock()
	defer e.mu.Unlock()
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
	if state.AwaitingAddressConfirm {
		if text := lastBotText(state); text != "" {
			return text
		}
	}
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

func (e *DialogueEngine) handleAddressStep(ctx context.Context, state *SessionState, step DialogueStep, userText string, classified IntentResult) string {
	if classified.Intent != "address" && classified.Address == "" {
		if state.AddressRetries < e.maxAddressRetries {
			state.AddressRetries++
			return recordBotReply(state, AddressRetryPrompt)
		}
		state.Results["address"] = map[string]any{"status": "not_found", "text": userText, "intent": classified.Intent}
		return e.advanceWithReply(state, "好的，地址已记录，后续会由人工进一步核实。")
	}
	if e.geocoder == nil {
		state.Results["address"] = map[string]any{"status": "unverified", "text": userText, "intent": classified.Intent}
		return e.advanceWithReply(state, "好的，地址已记录。当前环境暂时无法完成地点搜索。")
	}

	searchText := classified.Address
	if searchText == "" {
		searchText = userText
	}
	result, err := e.geocoder.ResolvePlace(ctx, searchText)
	if err == nil && strings.HasPrefix(result.Error, "未配置 AMAP_KEY") {
		state.Results["address"] = map[string]any{"status": "unverified", "text": userText, "intent": "address"}
		return e.advanceWithReply(state, "好的，地址已记录。当前环境暂时无法完成地点搜索。")
	}
	if err == nil && result.Found && result.Best != nil {
		candidate := result.Best
		if !addressNeedsConfirmation(searchText, *candidate) {
			state.Results["address"] = map[string]any{
				"status": "ok",
				"text":   userText,
				"intent": "address",
				"place":  placeCandidateMap(*candidate),
			}
			return e.advanceWithReply(state, "好的，地址已记录。")
		}
		state.AwaitingAddressConfirm = true
		state.PendingAddressCandidate = placeCandidateMap(*candidate)
		state.PendingAddressText = userText
		return recordBotReply(state, e.buildAddressConfirmationTurn(ctx, searchText, *candidate))
	}
	if state.AddressRetries < e.maxAddressRetries {
		state.AddressRetries++
		return recordBotReply(state, AddressRetryPrompt)
	}
	state.Results["address"] = map[string]any{"status": "not_found", "text": userText, "intent": "address"}
	return e.advanceWithReply(state, "好的，地址已记录，后续会由人工进一步核实。")
}

func (e *DialogueEngine) buildAddressConfirmationTurn(ctx context.Context, originalText string, candidate PlaceCandidate) string {
	matchedText := meaningfulCandidateText(candidate)
	focusText := extractAddressDifference(originalText, matchedText)
	fallback := buildAddressConfirmationPrompt(originalText, candidate)
	prompt, err := e.classifier.GenerateAddressConfirmation(ctx, AddressConfirmationInput{
		OriginalText:   originalText,
		MatchedText:    matchedText,
		MatchedName:    candidate.Name,
		FocusText:      focusText,
		FallbackPrompt: fallback,
	})
	if err != nil || strings.TrimSpace(prompt) == "" {
		return fallback
	}
	return prompt
}

func placeCandidateMap(candidate PlaceCandidate) map[string]any {
	value := map[string]any{
		"name":         candidate.Name,
		"address":      candidate.Address,
		"district":     candidate.District,
		"location":     candidate.Location,
		"display_text": candidate.DisplayText,
		"source":       candidate.Source,
		"formatted":    candidate.Formatted,
		"compare_text": candidate.CompareText,
		"precision_ok": candidate.PrecisionOK,
		"score":        candidate.Score,
	}
	if candidate.Verify != nil {
		value["verify"] = map[string]any{
			"success":      candidate.Verify.Success,
			"formatted":    candidate.Verify.Formatted,
			"level":        candidate.Verify.Level,
			"location":     candidate.Verify.Location,
			"precision_ok": candidate.Verify.PrecisionOK,
			"error":        candidate.Verify.Error,
		}
	}
	return value
}

func (e *DialogueEngine) handleAddressConfirmation(ctx context.Context, state *SessionState, userText string) string {
	prompt := e.currentPrompt(state)
	ensurePromptRecorded(state, prompt)
	recordUserTurn(state, userText)

	candidate := state.PendingAddressCandidate
	originalText := state.PendingAddressText
	if candidate == nil {
		state.AwaitingAddressConfirm = false
		state.PendingAddressText = ""
		return recordBotReply(state, AddressRetryPrompt)
	}
	if userText == "用户没有说话" {
		return e.handleAddressConfirmationRejected(state, originalText)
	}
	classified, err := e.classifier.Classify(ctx, userText, IntentContext{
		Stage:          "address_confirm",
		ExpectedIntent: "yes_no",
		Question:       prompt,
	})
	if err != nil {
		classified = IntentResult{Intent: "unclear"}
	}
	state.AwaitingAddressConfirm = false
	state.PendingAddressCandidate = nil
	state.PendingAddressText = ""
	if classified.Intent == "yes" {
		state.Results["address"] = map[string]any{
			"status": "ok",
			"text":   originalText,
			"intent": "address",
			"place":  candidate,
		}
		return e.advanceWithReply(state, "好的，地址已记录。")
	}
	return e.handleAddressConfirmationRejected(state, originalText)
}

func (e *DialogueEngine) handleAddressConfirmationRejected(state *SessionState, originalText string) string {
	state.AwaitingAddressConfirm = false
	state.PendingAddressCandidate = nil
	state.PendingAddressText = ""
	if state.AddressRetries < e.maxAddressRetries {
		state.AddressRetries++
		return recordBotReply(state, AddressRetryPrompt)
	}
	state.Results["address"] = map[string]any{"status": "not_found", "text": originalText, "intent": "address"}
	return e.advanceWithReply(state, "好的，地址已记录，后续会由人工进一步核实。")
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

func lastBotText(state *SessionState) string {
	for i := len(state.Transcript) - 1; i >= 0; i-- {
		if state.Transcript[i]["speaker"] == "bot" {
			return state.Transcript[i]["text"]
		}
	}
	return ""
}

func copyMap(input map[string]any) map[string]any {
	output := make(map[string]any, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}
