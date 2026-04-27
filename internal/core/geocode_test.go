package core

import "testing"

func TestIsPreciseEnough(t *testing.T) {
	if !IsPreciseEnough("门牌号", "北京市朝阳区建国路88号") {
		t.Fatal("expected precise level to pass")
	}
	if IsPreciseEnough("区县", "北京市朝阳区") {
		t.Fatal("expected broad level to fail")
	}
}

func TestBuildDisplayTextDeduplicatesAdjacentParts(t *testing.T) {
	got := BuildDisplayText("小家公寓", "仑头村仑头路82号", "海珠区")
	if got != "海珠区仑头村仑头路82号小家公寓" {
		t.Fatalf("unexpected display text: %q", got)
	}
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
	if len(candidates) != 2 {
		t.Fatalf("expected candidates, got %#v", candidates)
	}
	if candidates[0].Name != "小家公寓" || !candidates[0].PrecisionOK {
		t.Fatalf("unexpected best candidate: %#v", candidates[0])
	}
}

func TestAddressNeedsConfirmation(t *testing.T) {
	exact := PlaceCandidate{DisplayText: "海珠区仑头村仑头路82号小家公寓"}
	if addressNeedsConfirmation("海珠区仑头村仑头路82号小家公寓", exact) {
		t.Fatal("exact address should not need confirmation")
	}
	fuzzy := PlaceCandidate{DisplayText: "海珠区仑头村仑头路82号小家公寓"}
	if !addressNeedsConfirmation("广州海珠区轮头村八二路小家公寓", fuzzy) {
		t.Fatal("fuzzy ASR address should need confirmation")
	}
}
