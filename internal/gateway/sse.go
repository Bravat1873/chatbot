package gateway

import (
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

var sentenceDelimiter = regexp.MustCompile(`([，。？！；,\.!\?;])`)

type SSEChunk struct {
	Choices []SSEChoice `json:"choices"`
	Object  string      `json:"object"`
	Model   string      `json:"model"`
	Created int64       `json:"created"`
	ID      string      `json:"id"`
}

type SSEChoice struct {
	Delta        SSEDelta `json:"delta"`
	FinishReason *string  `json:"finish_reason"`
	Index        int      `json:"index"`
}

type SSEDelta struct {
	Content string `json:"content"`
}

func SplitSentences(text string) []string {
	parts := sentenceDelimiter.Split(text, -1)
	matches := sentenceDelimiter.FindAllString(text, -1)
	sentences := make([]string, 0, len(parts))
	current := ""
	for i, part := range parts {
		current += part
		if i < len(matches) {
			current += matches[i]
			if current != "" {
				sentences = append(sentences, current)
			}
			current = ""
		}
	}
	if current != "" {
		sentences = append(sentences, current)
	}
	if len(sentences) == 0 {
		return []string{text}
	}
	return sentences
}

func NewCompletionID() string {
	return fmt.Sprintf("chatcmpl-%s", strings.ReplaceAll(uuid.NewString(), "-", "")[:12])
}

func WriteChatCompletionSSE(w io.Writer, reply string, model string, created int64, completionID string) error {
	if created == 0 {
		created = time.Now().Unix()
	}
	if completionID == "" {
		completionID = NewCompletionID()
	}
	for _, sentence := range SplitSentences(reply) {
		chunk := SSEChunk{
			Choices: []SSEChoice{{Delta: SSEDelta{Content: sentence}, FinishReason: nil, Index: 0}},
			Object:  "chat.completion.chunk",
			Model:   model,
			Created: created,
			ID:      completionID,
		}
		if err := writeSSEData(w, chunk); err != nil {
			return err
		}
	}
	stop := "stop"
	final := SSEChunk{
		Choices: []SSEChoice{{Delta: SSEDelta{Content: ""}, FinishReason: &stop, Index: 0}},
		Object:  "chat.completion.chunk",
		Model:   model,
		Created: created,
		ID:      completionID,
	}
	if err := writeSSEData(w, final); err != nil {
		return err
	}
	_, err := io.WriteString(w, "data: [DONE]\n\n")
	return err
}

func writeSSEData(w io.Writer, payload any) error {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "data: %s\n\n", encoded)
	return err
}
