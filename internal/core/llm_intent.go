// LLM 意图分类器：通过大模型（DashScope/qwen）提升意图和地址提取的准确率，启发式作为兜底。
package core

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
)

// LLMRequest LLM 调用请求参数。
type LLMRequest struct {
	SystemPrompt string
	UserPrompt   string
	MaxTokens    int
	Temperature  float64
}

// LLMClient LLM 客户端接口，解耦具体的大模型服务实现。
type LLMClient interface {
	GenerateJSON(ctx context.Context, req LLMRequest) (string, error)
}

// LLMIntentClassifier LLM + 启发式双通道分类器：LLM 优先，启发式兜底。
type LLMIntentClassifier struct {
	heuristic *HeuristicIntentClassifier
	llm       LLMClient
}

// NewLLMIntentClassifier 创建双通道分类器，llm 为 nil 时退化为纯启发式。
func NewLLMIntentClassifier(llm LLMClient) *LLMIntentClassifier {
	return &LLMIntentClassifier{
		heuristic: NewHeuristicIntentClassifier(),
		llm:       llm,
	}
}

// Classify 先跑启发式，若 LLM 可用则用 LLM 结果覆盖（置信度标记为 high）。
func (c *LLMIntentClassifier) Classify(ctx context.Context, text string, intentContext IntentContext) (IntentResult, error) {
	heuristic, err := c.heuristic.Classify(ctx, text, intentContext)
	if err != nil {
		return heuristic, err
	}
	if text == "" || c.llm == nil {
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
	parsed.Confidence = "high"
	parsed.Source = "llm"
	parsed.RawText = content
	return parsed, nil
}

// GenerateAddressConfirmation 生成地址确认话术，LLM 不可用时回退到模板生成。
func (c *LLMIntentClassifier) GenerateAddressConfirmation(ctx context.Context, input AddressConfirmationInput) (string, error) {
	fallback := input.FallbackPrompt
	if fallback == "" {
		fallback = fallbackAddressPrompt(firstNonEmpty(input.FocusText, input.MatchedText), input.MatchedName)
	}
	if c.llm == nil {
		return fallback, nil
	}
	systemPrompt, userPrompt := buildAddressConfirmationLLMPrompts(input)
	content, err := c.llm.GenerateJSON(ctx, LLMRequest{
		SystemPrompt: systemPrompt,
		UserPrompt:   userPrompt,
		MaxTokens:    64,
		Temperature:  0.2,
	})
	if err != nil {
		return fallback, nil
	}
	prompt := cleanConfirmationPrompt(content)
	prompt = ensureNamedPlacePrompt(prompt, input.MatchedText, input.MatchedName, input.FocusText)
	if prompt == "" {
		return fallback, nil
	}
	return prompt, nil
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

func buildAddressConfirmationLLMPrompts(input AddressConfirmationInput) (string, string) {
	systemPrompt := "你是电话地址确认助手。" +
		"你的任务是生成一句简短、自然、口语化的中文确认话术。" +
		"只确认真正不确定的局部，不要确认省、市、区这类常识信息。" +
		"禁止解释、禁止输出多句、禁止输出引号、禁止输出编号。" +
		"只输出一句可以直接念给用户听的话。"
	matchedName := strings.TrimSpace(input.MatchedName)
	if matchedName == "" {
		matchedName = "无"
	}
	focusText := strings.TrimSpace(input.FocusText)
	if focusText == "" {
		focusText = "无"
	}
	userPrompt := "用户原话: " + input.OriginalText + "\n" +
		"系统匹配地址: " + input.MatchedText + "\n" +
		"系统匹配名称: " + matchedName + "\n" +
		"建议重点确认的局部: " + focusText + "\n" +
		"要求:\n" +
		"1. 如果存在村名、路名、门牌号、小区名差异，只确认这些局部。\n" +
		"2. 不要问“广东省/广州市/海珠区这类大范围信息对不对”。\n" +
		"3. 话术尽量像真人客服，控制在 25 个字以内。\n" +
		"4. 如果 focus_text 已经足够具体，优先围绕它来问。\n" +
		"5. 不要照抄用户原话里的错别字，优先使用系统匹配地址中的名称。\n" +
		"6. 如果系统匹配名称里有公司名、店名、小区名、楼盘名，优先把这个名称说出来再确认。\n" +
		"7. 对公司类地点，优先生成“您说的是XX公司，地址在XX，对吗？”这种话术。\n" +
		"只输出一句最终话术。"
	return systemPrompt, userPrompt
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
