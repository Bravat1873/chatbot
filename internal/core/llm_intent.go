package core

import (
	"context"
	"encoding/json"
	"regexp"
)

type LLMRequest struct {
	SystemPrompt string
	UserPrompt   string
	MaxTokens    int
	Temperature  float64
}

type LLMClient interface {
	GenerateJSON(ctx context.Context, req LLMRequest) (string, error)
}

type LLMIntentClassifier struct {
	heuristic *HeuristicIntentClassifier
	llm       LLMClient
}

func NewLLMIntentClassifier(llm LLMClient) *LLMIntentClassifier {
	return &LLMIntentClassifier{
		heuristic: NewHeuristicIntentClassifier(),
		llm:       llm,
	}
}

func (c *LLMIntentClassifier) Classify(ctx context.Context, text string, intentContext IntentContext) (IntentResult, error) {
	heuristic, err := c.heuristic.Classify(ctx, text, intentContext)
	if err != nil {
		return heuristic, err
	}
	if heuristic.Intent != "unclear" || c.llm == nil {
		return heuristic, nil
	}

	systemPrompt, userPrompt := buildLLMPrompts(text, intentContext)
	content, err := c.llm.GenerateJSON(ctx, LLMRequest{
		SystemPrompt: systemPrompt,
		UserPrompt:   userPrompt,
		MaxTokens:    maxTokensForExpectedIntent(intentContext.ExpectedIntent),
		Temperature:  0,
	})
	if err != nil {
		return heuristic, nil
	}
	parsed := parseIntentJSON(content)
	if parsed.Intent == "" {
		return heuristic, nil
	}
	parsed.Confidence = "high"
	parsed.Source = "llm"
	parsed.RawText = content
	return parsed, nil
}

func buildLLMPrompts(text string, context IntentContext) (string, string) {
	switch context.ExpectedIntent {
	case "yes_no":
		systemPrompt := "你是严格的意图分类器。禁止解释、禁止补充、禁止输出 markdown、禁止输出思考过程。你只能输出且必须输出一个最短 JSON：{\"intent\":\"yes\"}、{\"intent\":\"no\"} 或 {\"intent\":\"unclear\"}。"
		questionLine := ""
		if context.Question != "" {
			questionLine = "机器人问题: " + context.Question + "\n"
		}
		userPrompt := "当前阶段: " + context.Stage + "\n" +
			questionLine +
			"任务: 仅判断用户对上述问题的真实意图是肯定、否定还是无法判断。\n" +
			"注意: 反问句('满意？你觉得我满意吗？')、反讽、质问语气通常表达的是相反意图，请识别真实态度而非字面关键词。\n" +
			"用户文本: " + text + "\n" +
			"只返回一个最短 JSON，然后立刻结束。"
		return systemPrompt, userPrompt
	case "address":
		systemPrompt := "你是严格的地址提取与意图分类器。禁止解释、禁止补充、禁止输出 markdown、禁止输出思考过程。你只能输出且必须输出一个最短 JSON：{\"intent\":\"address\",\"address\":\"详细地址\"} 或 {\"intent\":\"unclear\",\"address\":\"\"}。"
		userPrompt := "当前阶段: " + context.Stage + "\n" +
			"任务: 如果用户在提供地址，则提取尽可能完整的地址；否则返回 unclear。\n" +
			"用户文本: " + text + "\n" +
			"只返回一个最短 JSON，然后立刻结束。"
		return systemPrompt, userPrompt
	default:
		systemPrompt := "你是严格的意图分类器。禁止解释、禁止补充、禁止输出 markdown、禁止输出思考过程。你只能输出且必须输出一个 JSON：{\"intent\":\"yes|no|address|unclear\",\"address\":\"提取出的地址或空字符串\"}。"
		userPrompt := "当前阶段: " + context.Stage + "\n" +
			"期望意图: " + context.ExpectedIntent + "\n" +
			"用户文本: " + text + "\n" +
			"只返回一个 JSON，然后立刻结束。"
		return systemPrompt, userPrompt
	}
}

func maxTokensForExpectedIntent(expected string) int {
	if expected == "yes_no" {
		return 20
	}
	return 64
}

func parseIntentJSON(content string) IntentResult {
	var payload struct {
		Intent  string `json:"intent"`
		Address string `json:"address"`
	}
	if err := json.Unmarshal([]byte(content), &payload); err != nil {
		match := regexp.MustCompile(`(?s)\{.*\}`).FindString(content)
		if match == "" {
			return IntentResult{Intent: "unclear", Address: ""}
		}
		_ = json.Unmarshal([]byte(match), &payload)
	}
	if payload.Intent == "" {
		payload.Intent = "unclear"
	}
	return IntentResult{Intent: payload.Intent, Address: payload.Address}
}
