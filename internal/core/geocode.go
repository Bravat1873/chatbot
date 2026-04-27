package core

import (
	"context"
	"strings"
)

type PlaceCandidate struct {
	Name        string
	Address     string
	District    string
	DisplayText string
	Raw         map[string]any
}

type GeocodeResult struct {
	Found bool
	Best  *PlaceCandidate
	Error string
}

type Geocoder interface {
	ResolvePlace(ctx context.Context, keywords string) (GeocodeResult, error)
}

func buildAddressConfirmationPrompt(originalText string, candidate PlaceCandidate) string {
	name := strings.TrimSpace(candidate.Name)
	compareText := meaningfulCandidateText(candidate)
	if name != "" {
		location := strings.TrimSpace(strings.Replace(compareText, name, "", 1))
		location = strings.Trim(location, " ，,。；;：:")
		if location != "" {
			return "请问是" + name + "，地址在" + location + "吗？"
		}
		return "请问是" + name + "吗？"
	}
	if compareText != "" {
		return "我核对到的地址是“" + compareText + "”，请问对吗？"
	}
	return "我核对到的地址和您说的“" + originalText + "”接近，请问对吗？"
}

func meaningfulCandidateText(candidate PlaceCandidate) string {
	if candidate.DisplayText != "" {
		return candidate.DisplayText
	}
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
	return strings.Join(parts, "")
}
