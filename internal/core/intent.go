package core

import (
	"context"
	"regexp"
	"strings"
)

type IntentContext struct {
	Stage          string
	ExpectedIntent string
	Question       string
}

type IntentResult struct {
	Intent     string
	Address    string
	Confidence string
	Source     string
	RawText    string
}

type IntentClassifier interface {
	Classify(ctx context.Context, text string, context IntentContext) (IntentResult, error)
}

type HeuristicIntentClassifier struct{}

var (
	yesHints = map[string]struct{}{
		"有": {}, "有的": {}, "好的": {}, "好": {}, "嗯": {}, "嗯嗯": {}, "是": {}, "是的": {},
		"对": {}, "对的": {}, "没错": {}, "收到": {}, "收到了": {}, "满意": {}, "解决了": {},
		"还行": {}, "还可以": {}, "不错": {}, "挺好": {}, "可以": {},
	}
	noHints = map[string]struct{}{
		"没": {}, "没有": {}, "还没": {}, "不是": {}, "不满意": {}, "没收到": {}, "不行": {},
		"不对": {}, "未解决": {},
	}
	addressHints = []string{"路", "街", "巷", "弄", "号", "栋", "单元", "室", "小区", "广场", "大厦", "村", "镇", "乡", "省", "市", "区", "县"}
	unclearHints = []string{"啊", "哈", "喂", "你说什么", "什么意思", "没听清", "再说一遍"}
	spacePattern = regexp.MustCompile(`\s+`)
	digitPattern = regexp.MustCompile(`\d`)
)

func NewHeuristicIntentClassifier() *HeuristicIntentClassifier {
	return &HeuristicIntentClassifier{}
}

func (c *HeuristicIntentClassifier) Classify(ctx context.Context, text string, context IntentContext) (IntentResult, error) {
	_ = ctx
	cleaned := strings.TrimSpace(text)
	if cleaned == "" {
		return IntentResult{Intent: "unclear", Confidence: "low", Source: "heuristic", RawText: text}, nil
	}
	normalized := spacePattern.ReplaceAllString(cleaned, "")
	if looksUnclear(normalized) {
		return IntentResult{Intent: "unclear", Confidence: "low", Source: "heuristic", RawText: text}, nil
	}
	if looksLikeAddress(normalized) {
		return IntentResult{Intent: "address", Address: extractAddress(normalized), Confidence: "medium", Source: "heuristic", RawText: text}, nil
	}

	noScore := 0
	for token := range noHints {
		if strings.Contains(normalized, token) {
			noScore++
		}
	}
	yesScore := 0
	for token := range yesHints {
		if idx := strings.Index(normalized, token); idx >= 0 {
			if isNegated(normalized, idx) {
				noScore++
			} else {
				yesScore++
			}
		}
	}
	if yesScore > noScore && yesScore > 0 {
		return IntentResult{Intent: "yes", Confidence: "medium", Source: "heuristic", RawText: text}, nil
	}
	if noScore > 0 {
		return IntentResult{Intent: "no", Confidence: "medium", Source: "heuristic", RawText: text}, nil
	}
	if looksLikeAddress(normalized) {
		return IntentResult{Intent: "address", Address: extractAddress(normalized), Confidence: "medium", Source: "heuristic", RawText: text}, nil
	}
	return IntentResult{Intent: "unclear", Confidence: "low", Source: "heuristic", RawText: text}, nil
}

func isNegated(text string, idx int) bool {
	for _, prefix := range []string{"不太", "不", "没", "未", "别"} {
		if idx >= len([]rune(prefix)) && strings.HasSuffix(text[:idx], prefix) {
			return true
		}
	}
	return false
}

func looksUnclear(text string) bool {
	for _, hint := range unclearHints {
		if strings.Contains(text, hint) {
			return true
		}
	}
	_, isYes := yesHints[text]
	_, isNo := noHints[text]
	return len([]rune(text)) <= 1 && !isYes && !isNo
}

func looksLikeAddress(text string) bool {
	if digitPattern.MatchString(text) {
		return true
	}
	count := 0
	for _, hint := range addressHints {
		if strings.Contains(text, hint) {
			count++
		}
	}
	return count >= 2
}

func extractAddress(text string) string {
	return strings.Trim(text, "，。,. ")
}
