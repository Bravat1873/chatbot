package gateway

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestSplitSentencesKeepsPunctuation(t *testing.T) {
	got := SplitSentences("你好，今天怎么样？挺好。")
	want := []string{"你好，", "今天怎么样？", "挺好。"}
	if len(got) != len(want) {
		t.Fatalf("expected %d sentences, got %d: %#v", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("sentence %d expected %q, got %q", i, want[i], got[i])
		}
	}
}

func TestWriteChatCompletionSSEUsesOpenAIChunkFormat(t *testing.T) {
	var buf bytes.Buffer

	if err := WriteChatCompletionSSE(&buf, "你好，世界。", "qwen-plus", 1734523000, "chatcmpl-fixed"); err != nil {
		t.Fatalf("write sse: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if lines[len(lines)-1] != "data: [DONE]" {
		t.Fatalf("expected final DONE line, got %q", lines[len(lines)-1])
	}

	var first SSEChunk
	if err := json.Unmarshal([]byte(strings.TrimPrefix(lines[0], "data: ")), &first); err != nil {
		t.Fatalf("decode first chunk: %v", err)
	}
	if first.Object != "chat.completion.chunk" || first.Model != "qwen-plus" || first.Created != 1734523000 || first.ID != "chatcmpl-fixed" {
		t.Fatalf("unexpected first chunk metadata: %#v", first)
	}
	if first.Choices[0].Delta.Content != "你好，" || first.Choices[0].FinishReason != nil {
		t.Fatalf("unexpected first choice: %#v", first.Choices[0])
	}

	var final SSEChunk
	if err := json.Unmarshal([]byte(strings.TrimPrefix(lines[len(lines)-3], "data: ")), &final); err != nil {
		t.Fatalf("decode final chunk: %v", err)
	}
	if final.Choices[0].FinishReason == nil || *final.Choices[0].FinishReason != "stop" {
		t.Fatalf("expected stop finish reason, got %#v", final.Choices[0].FinishReason)
	}
}
