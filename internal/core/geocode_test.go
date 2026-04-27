package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsPreciseEnough(t *testing.T) {
	assert.True(t, IsPreciseEnough("门牌号", "北京市朝阳区建国路88号"), "expected precise level to pass")
	assert.False(t, IsPreciseEnough("区县", "北京市朝阳区"), "expected broad level to fail")
}

func TestBuildDisplayTextDeduplicatesAdjacentParts(t *testing.T) {
	got := BuildDisplayText("小家公寓", "仑头村仑头路82号", "海珠区")
	assert.Equal(t, "海珠区仑头村仑头路82号小家公寓", got)
}

func TestMergePlaceCandidatesRanksInputTipsWithVerification(t *testing.T) {
	tips := []PlaceCandidate{{
		Name:        "小家公寓",
		Address:     "仑头村仑头路82号",
		District:    "海珠区",
		DisplayText: "海珠区仑头村仑头路82号小家公寓",
		Source:      "input_tips",
	}}
	pois := []PlaceCandidate{{
		Name:        "天河体育中心",
		Address:     "体育西路",
		District:    "天河区",
		DisplayText: "天河区体育西路天河体育中心",
		Source:      "poi_search",
	}}

	candidates := MergePlaceCandidates("广州海珠区轮头村八二路小家公寓", tips, pois, "广州", func(text string, city string) AddressVerifyResult {
		if text == "海珠区仑头村仑头路82号小家公寓" {
			return AddressVerifyResult{
				Success:     true,
				Formatted:   "广东省广州市海珠区仑头村仑头路82号小家公寓",
				Level:       "兴趣点",
				PrecisionOK: true,
			}
		}
		return AddressVerifyResult{Success: true, Formatted: text, Level: "兴趣点", PrecisionOK: true}
	})
	assert.Len(t, candidates, 2)
	assert.Equal(t, "小家公寓", candidates[0].Name)
	assert.True(t, candidates[0].PrecisionOK)
}

func TestAddressNeedsConfirmation(t *testing.T) {
	exact := PlaceCandidate{DisplayText: "海珠区仑头村仑头路82号小家公寓"}
	assert.False(t, addressNeedsConfirmation("海珠区仑头村仑头路82号小家公寓", exact), "exact address should not need confirmation")
	fuzzy := PlaceCandidate{DisplayText: "海珠区仑头村仑头路82号小家公寓"}
	assert.True(t, addressNeedsConfirmation("广州海珠区轮头村八二路小家公寓", fuzzy), "fuzzy ASR address should need confirmation")
}
