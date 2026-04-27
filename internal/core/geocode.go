package core

import (
	"context"
	"math"
	"regexp"
	"sort"
	"strings"
)

type PlaceCandidate struct {
	Name        string
	Address     string
	District    string
	Location    string
	DisplayText string
	Source      string
	Formatted   string
	CompareText string
	PrecisionOK bool
	Distance    int
	Similarity  float64
	NumberScore float64
	OrgScore    float64
	RoadScore   float64
	Score       int
	Verify      *AddressVerifyResult
	Raw         map[string]any
}

type GeocodeResult struct {
	Found      bool
	Best       *PlaceCandidate
	Candidates []PlaceCandidate
	Tips       []PlaceCandidate
	POIs       []PlaceCandidate
	Error      string
}

type AddressVerifyResult struct {
	Success     bool
	Formatted   string
	Level       string
	Location    string
	PrecisionOK bool
	Error       string
	Raw         map[string]any
}

type Geocoder interface {
	ResolvePlace(ctx context.Context, keywords string) (GeocodeResult, error)
}

type AddressConfirmationInput struct {
	OriginalText   string
	MatchedText    string
	MatchedName    string
	FocusText      string
	FallbackPrompt string
}

var (
	normalizePattern      = regexp.MustCompile(`[\s，。、“”‘’,.!！？；;:：\-（）()\[\]【】/\\]+`)
	prefixPattern         = regexp.MustCompile(`^.*?(?:省|市|区|县|镇|乡|街道)`)
	candidatePrefixRegexp = regexp.MustCompile(`.+?(?:省|市|区|县|镇|乡|街道)`)
	numberPattern         = regexp.MustCompile(`\d+`)
	orgSuffixPattern      = regexp.MustCompile(`(有限责任公司|有限公司|公司|集团|店|中心|分店|分公司|营业部)$`)
	orgNoisePattern       = regexp.MustCompile(`[^\p{Han}A-Za-z0-9]`)
	roadTokenPattern      = regexp.MustCompile(`[\p{Han}A-Za-z0-9]+(?:大道|大街|路|街|巷|道|弄|村)`)
	preciseLevelKeywords  = []string{"门牌号", "兴趣点", "楼栋", "单元", "房间号", "道路交叉口", "道路", "村庄"}
	broadLevelKeywords    = []string{"国家", "省", "市", "区县", "开发区", "乡镇"}
	namedPlaceTokens      = []string{"公司", "店", "中心", "广场", "大厦", "公寓", "小区", "苑", "城", "园"}
)

func buildAddressConfirmationPrompt(originalText string, candidate PlaceCandidate) string {
	name := strings.TrimSpace(candidate.Name)
	compareText := meaningfulCandidateText(candidate)
	differingText := extractAddressDifference(originalText, compareText)
	minLen := max(4, runeLen(compareText)-2)
	if isNamedPlace(name) {
		basis := compareText
		if isGoodAddressFragment(differingText, minLen) {
			basis = differingText
		}
		location := addressWithoutName(basis, name)
		if location != "" {
			return "请问是" + name + "，地址在" + location + "吗？"
		}
		return "请问是" + name + "吗？"
	}
	if isGoodAddressFragment(differingText, minLen) {
		return "我核对到的详细位置像是“" + differingText + "”，请问对吗？"
	}
	if compareText != "" {
		return "我核对到的地址是“" + compareText + "”，请问对吗？"
	}
	return "我核对到的地址和您说的“" + originalText + "”接近，请问对吗？"
}

func meaningfulCandidateText(candidate PlaceCandidate) string {
	text := firstNonEmpty(candidate.DisplayText, candidate.Formatted, candidate.CompareText)
	if text == "" {
		parts := make([]string, 0, 3)
		if candidate.District != "" {
			parts = append(parts, candidate.District)
		}
		if candidate.Address != "" {
			parts = append(parts, candidate.Address)
		}
		if candidate.Name != "" {
			parts = append(parts, candidate.Name)
		}
		text = strings.Join(parts, "")
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	prefixEnd := 0
	for _, match := range candidatePrefixRegexp.FindAllStringIndex(text, -1) {
		prefixEnd = match[1]
	}
	if prefixEnd > 0 && prefixEnd < len(text) {
		if trimmed := strings.TrimSpace(text[prefixEnd:]); trimmed != "" {
			return trimmed
		}
	}
	return text
}

func addressNeedsConfirmation(originalText string, candidate PlaceCandidate) bool {
	original := normalizeText(originalText)
	compareText := normalizeText(meaningfulCandidateText(candidate))
	if original == "" || compareText == "" {
		return false
	}
	if strings.Contains(original, compareText) {
		return false
	}
	return original != compareText
}

func MergePlaceCandidates(query string, tips []PlaceCandidate, pois []PlaceCandidate, city string, verify func(string, string) AddressVerifyResult) []PlaceCandidate {
	_ = city
	deduped := make(map[string]PlaceCandidate)
	all := append(append([]PlaceCandidate{}, tips...), pois...)
	for _, candidate := range all {
		displayText := strings.TrimSpace(candidate.DisplayText)
		if displayText == "" {
			displayText = buildDisplayText(candidate.Name, candidate.Address, candidate.District)
			candidate.DisplayText = displayText
		}
		if displayText == "" {
			continue
		}

		localQuery := meaningfulQueryText(query)
		localDisplay := meaningfulQueryText(displayText)
		verifyResult := AddressVerifyResult{}
		if verify != nil {
			verifyResult = verify(displayText, city)
			candidate.Verify = &verifyResult
		}
		compareText := firstNonEmpty(verifyResult.Formatted, displayText)
		localCompare := meaningfulQueryText(compareText)

		candidate.Distance = levenshteinDistance([]rune(normalizeText(localQuery)), []rune(normalizeText(localDisplay)))
		candidate.Similarity = sequenceSimilarity(localQuery, firstNonEmpty(localCompare, localDisplay))
		candidate.NumberScore = numberMatchScore(localQuery, firstNonEmpty(localCompare, localDisplay))
		candidate.OrgScore = organizationMatchScore(localQuery, firstNonEmpty(localCompare, localDisplay))
		candidate.RoadScore = roadMatchScore(localQuery, firstNonEmpty(localCompare, localDisplay))
		candidate.PrecisionOK = verifyResult.PrecisionOK
		candidate.Formatted = verifyResult.Formatted
		candidate.CompareText = compareText
		candidate.Score = candidateScore(candidate)

		key := normalizeText(displayText)
		if existing, ok := deduped[key]; !ok || candidateRankLess(candidate, existing) {
			deduped[key] = candidate
		}
	}

	candidates := make([]PlaceCandidate, 0, len(deduped))
	for _, candidate := range deduped {
		candidates = append(candidates, candidate)
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return candidateRankLess(candidates[i], candidates[j])
	})
	return candidates
}

func BuildDisplayText(name string, address string, district string) string {
	return buildDisplayText(name, address, district)
}

func IsPreciseEnough(level string, formattedAddress string) bool {
	for _, keyword := range preciseLevelKeywords {
		if strings.Contains(level, keyword) {
			return true
		}
	}
	for _, keyword := range broadLevelKeywords {
		if strings.Contains(level, keyword) {
			return false
		}
	}
	return strings.Contains(formattedAddress, "号") ||
		strings.Contains(formattedAddress, "栋") ||
		strings.Contains(formattedAddress, "单元") ||
		strings.Contains(formattedAddress, "室")
}

func buildDisplayText(name string, address string, district string) string {
	parts := []string{strings.TrimSpace(district), strings.TrimSpace(address), strings.TrimSpace(name)}
	deduped := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			continue
		}
		if len(deduped) > 0 && deduped[len(deduped)-1] == part {
			continue
		}
		deduped = append(deduped, part)
	}
	return strings.Join(deduped, "")
}

func fallbackAddressPrompt(focusText string, matchedName string) string {
	cleaned := strings.TrimSpace(focusText)
	if cleaned == "" {
		cleaned = "这个地址"
	}
	if isNamedPlace(matchedName) {
		location := addressWithoutName(cleaned, matchedName)
		if location != "" {
			return "请问是" + strings.TrimSpace(matchedName) + "，地址在" + location + "吗？"
		}
		return "请问是" + strings.TrimSpace(matchedName) + "吗？"
	}
	return "我核对到的详细位置像是“" + cleaned + "”，请问对吗？"
}

func cleanConfirmationPrompt(prompt string) string {
	cleaned := strings.TrimSpace(prompt)
	cleaned = strings.Trim(cleaned, "\"'“”")
	cleaned = regexp.MustCompile(`^\d+[.)、]\s*`).ReplaceAllString(cleaned, "")
	if idx := strings.IndexAny(cleaned, "\r\n"); idx >= 0 {
		cleaned = cleaned[:idx]
	}
	return strings.TrimSpace(cleaned)
}

func ensureNamedPlacePrompt(prompt string, matchedText string, matchedName string, focusText string) string {
	if !isNamedPlace(matchedName) {
		return prompt
	}
	if strings.Contains(prompt, strings.TrimSpace(matchedName)) {
		return prompt
	}
	location := addressWithoutName(firstNonEmpty(strings.TrimSpace(focusText), strings.TrimSpace(matchedText)), matchedName)
	if location != "" {
		return "请问是" + strings.TrimSpace(matchedName) + "，地址在" + location + "吗？"
	}
	return "请问是" + strings.TrimSpace(matchedName) + "吗？"
}

func extractAddressDifference(originalText string, candidateText string) string {
	original := []rune(normalizeText(originalText))
	candidate := []rune(normalizeText(candidateText))
	if len(original) == 0 || len(candidate) == 0 || string(original) == string(candidate) {
		return ""
	}
	longest := ""
	start := 0
	for start < len(candidate) {
		for start < len(candidate) && containsRune(original, candidate[start]) {
			start++
		}
		end := start
		for end < len(candidate) && !containsRune(original, candidate[end]) {
			end++
		}
		if end > start {
			fragment := string(candidate[start:end])
			if runeLen(fragment) > runeLen(longest) {
				longest = fragment
			}
		}
		start = end + 1
	}
	if longest != "" {
		return longest
	}
	if runeLen(candidateText) <= runeLen(originalText) || runeLen(candidateText) <= 12 {
		return candidateText
	}
	return string([]rune(candidateText)[runeLen(candidateText)-12:])
}

func isGoodAddressFragment(text string, minLen int) bool {
	if text == "" || runeLen(text) < minLen {
		return false
	}
	for _, token := range []string{"路", "街", "巷", "号", "村", "苑", "厦", "城", "园"} {
		if strings.Contains(text, token) {
			return true
		}
	}
	return false
}

func isNamedPlace(name string) bool {
	stripped := strings.TrimSpace(name)
	if stripped == "" {
		return false
	}
	for _, token := range namedPlaceTokens {
		if strings.Contains(stripped, token) {
			return true
		}
	}
	return false
}

func addressWithoutName(text string, name string) string {
	cleaned := strings.TrimSpace(text)
	if cleaned == "" {
		return ""
	}
	if strippedName := strings.TrimSpace(name); strippedName != "" {
		cleaned = strings.Replace(cleaned, strippedName, "", 1)
	}
	return strings.Trim(cleaned, " ，,。；;：:")
}

func meaningfulQueryText(text string) string {
	raw := strings.TrimSpace(text)
	if raw == "" {
		return ""
	}
	if match := prefixPattern.FindStringIndex(raw); match != nil && match[1] < len(raw) {
		if trimmed := strings.TrimSpace(raw[match[1]:]); trimmed != "" {
			raw = trimmed
		}
	}
	return raw
}

func sequenceSimilarity(textA string, textB string) float64 {
	a := []rune(normalizeText(textA))
	b := []rune(normalizeText(textB))
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	distance := levenshteinDistance(a, b)
	maxLen := math.Max(float64(len(a)), float64(len(b)))
	return math.Max(0, 1-float64(distance)/maxLen)
}

func numberMatchScore(query string, candidate string) float64 {
	queryNumbers := numberPattern.FindAllString(query, -1)
	candidateNumbers := numberPattern.FindAllString(candidate, -1)
	if len(queryNumbers) == 0 {
		return 0
	}
	if len(candidateNumbers) == 0 {
		return -0.5
	}
	matched := 0
	for _, number := range queryNumbers {
		for _, candidateNumber := range candidateNumbers {
			if number == candidateNumber {
				matched++
				break
			}
		}
	}
	return float64(matched) / float64(len(queryNumbers))
}

func organizationMatchScore(query string, candidate string) float64 {
	if !strings.Contains(query, "公司") && !strings.Contains(query, "店") && !strings.Contains(query, "中心") {
		return 0
	}
	queryCore := organizationCore(query)
	candidateCore := organizationCore(candidate)
	if queryCore == "" || candidateCore == "" {
		return 0
	}
	if strings.Contains(candidateCore, queryCore) {
		return 1
	}
	queryRunes := []rune(queryCore)
	if len(queryRunes) >= 2 && strings.Contains(candidateCore, string(queryRunes[:2])) {
		return 0.6
	}
	return 0
}

func organizationCore(text string) string {
	cleaned := strings.NewReplacer("(", "", ")", "", "（", "", "）", "").Replace(text)
	cleaned = orgSuffixPattern.ReplaceAllString(cleaned, "")
	return orgNoisePattern.ReplaceAllString(cleaned, "")
}

func roadMatchScore(query string, candidate string) float64 {
	queryTokens := roadTokenPattern.FindAllString(query, -1)
	candidateTokens := roadTokenPattern.FindAllString(candidate, -1)
	if len(queryTokens) == 0 || len(candidateTokens) == 0 {
		return 0
	}
	matched := 0
	for _, token := range queryTokens {
		for _, candidateToken := range candidateTokens {
			if strings.Contains(candidateToken, token) ||
				strings.Contains(token, candidateToken) ||
				sequenceSimilarity(token, candidateToken) >= 0.55 {
				matched++
				break
			}
		}
	}
	return float64(matched) / float64(len(queryTokens))
}

func candidateScore(candidate PlaceCandidate) int {
	score := 0
	if candidate.PrecisionOK {
		score += 30
	}
	score += int(candidate.Similarity * 100)
	score += int(candidate.NumberScore * 18)
	score += int(candidate.OrgScore * 35)
	score += int(candidate.RoadScore * 25)
	score -= int(float64(candidate.Distance) * 0.6)
	if candidate.Source == "input_tips" {
		score += 4
	}
	return score
}

func candidateRankLess(left PlaceCandidate, right PlaceCandidate) bool {
	if left.Score != right.Score {
		return left.Score > right.Score
	}
	if left.PrecisionOK != right.PrecisionOK {
		return left.PrecisionOK
	}
	if left.Distance != right.Distance {
		return left.Distance < right.Distance
	}
	if left.Source != right.Source {
		return left.Source == "input_tips"
	}
	return runeLen(left.DisplayText) > runeLen(right.DisplayText)
}

func normalizeText(text string) string {
	return strings.ToLower(normalizePattern.ReplaceAllString(text, ""))
}

func levenshteinDistance(a []rune, b []rune) int {
	if string(a) == string(b) {
		return 0
	}
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}
	prev := make([]int, len(b)+1)
	for i := range prev {
		prev[i] = i
	}
	for i, charA := range a {
		curr := make([]int, len(b)+1)
		curr[0] = i + 1
		for j, charB := range b {
			insertCost := curr[j] + 1
			deleteCost := prev[j+1] + 1
			replaceCost := prev[j]
			if charA != charB {
				replaceCost++
			}
			curr[j+1] = min(insertCost, min(deleteCost, replaceCost))
		}
		prev = curr
	}
	return prev[len(b)]
}

func containsRune(values []rune, target rune) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func runeLen(text string) int {
	return len([]rune(text))
}
