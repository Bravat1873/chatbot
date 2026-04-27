package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseFlexibleTimeRFC3339(t *testing.T) {
	parsed, err := parseFlexibleTime("2026-04-23T12:34:56Z")
	require.NoError(t, err)
	assert.Equal(t, "2026-04-23T12:34:56Z", parsed.UTC().Format(time.RFC3339))
}

func TestParseFlexibleTimeUnixMilli(t *testing.T) {
	parsed, err := parseFlexibleTime("1713873600000")
	require.NoError(t, err)
	assert.False(t, parsed.IsZero(), "expected non-zero time")
}

func TestFindStringRecursive(t *testing.T) {
	payload := map[string]any{
		"outer": map[string]any{
			"CallId": "abc-123",
		},
	}
	assert.Equal(t, "abc-123", findString(payload, "call_id", "CallId"))
}
