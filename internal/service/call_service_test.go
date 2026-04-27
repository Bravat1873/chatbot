package service

import (
	"testing"
	"time"
)

func TestParseFlexibleTimeRFC3339(t *testing.T) {
	parsed, err := parseFlexibleTime("2026-04-23T12:34:56Z")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if parsed.UTC().Format(time.RFC3339) != "2026-04-23T12:34:56Z" {
		t.Fatalf("unexpected parsed time: %s", parsed.UTC().Format(time.RFC3339))
	}
}

func TestParseFlexibleTimeUnixMilli(t *testing.T) {
	parsed, err := parseFlexibleTime("1713873600000")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if parsed.IsZero() {
		t.Fatal("expected non-zero time")
	}
}

func TestFindStringRecursive(t *testing.T) {
	payload := map[string]any{
		"outer": map[string]any{
			"CallId": "abc-123",
		},
	}
	if got := findString(payload, "call_id", "CallId"); got != "abc-123" {
		t.Fatalf("expected call id abc-123, got %s", got)
	}
}
