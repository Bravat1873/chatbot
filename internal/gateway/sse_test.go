package gateway

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSplitSentencesKeepsPunctuation(t *testing.T) {
	got := SplitSentences("你好，今天怎么样？挺好。")
	want := []string{"你好，", "今天怎么样？", "挺好。"}
	assert.Equal(t, want, got)
}

func TestWriteChatCompletionSSEUsesOpenAIChunkFormat(t *testing.T) {
	var buf bytes.Buffer

	require.NoError(t, WriteChatCompletionSSE(&buf, "你好，世界。", "qwen-plus", 1734523000, "chatcmpl-fixed"))

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	assert.Equal(t, "data: [DONE]", lines[len(lines)-1])

	var first SSEChunk
	require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(lines[0], "data: ")), &first))
	assert.Equal(t, "chat.completion.chunk", first.Object)
	assert.Equal(t, "qwen-plus", first.Model)
	assert.Equal(t, int64(1734523000), first.Created)
	assert.Equal(t, "chatcmpl-fixed", first.ID)
	assert.Equal(t, "你好，", first.Choices[0].Delta.Content)
	assert.Nil(t, first.Choices[0].FinishReason)

	var final SSEChunk
	require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(lines[len(lines)-3], "data: ")), &final))
	require.NotNil(t, final.Choices[0].FinishReason)
	assert.Equal(t, "stop", *final.Choices[0].FinishReason)
}
