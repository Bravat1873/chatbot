// DashScope (阿里云百炼) Provider：封装兼容 OpenAI 协议的 Chat Completion API，实现 core.LLMClient 接口。
package dashscope

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"chatbot/internal/core"
)

const defaultBaseURL = "https://dashscope.aliyuncs.com/compatible-mode/v1"

// Client DashScope API 客户端，实现 core.LLMClient 接口。
type Client struct {
	apiKey     string
	baseURL    string
	model      string
	httpClient *http.Client
}

// Config DashScope 客户端配置。
type Config struct {
	APIKey  string
	BaseURL string
	Model   string
	Timeout time.Duration
}

// New 创建 DashScope 客户端实例。
func New(config Config) *Client {
	baseURL := strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	timeout := config.Timeout
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	return &Client{
		apiKey:  strings.TrimSpace(config.APIKey),
		baseURL: baseURL,
		model:   strings.TrimSpace(config.Model),
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// GenerateJSON 调用 LLM 生成结构化 JSON 响应，关闭思考链以加快推理速度。
func (c *Client) GenerateJSON(ctx context.Context, req core.LLMRequest) (string, error) {
	if c.apiKey == "" {
		return "", fmt.Errorf("dashscope api key is empty")
	}
	model := c.model
	if model == "" {
		model = "qwen3.5-flash"
	}
	payload := chatCompletionRequest{
		Model: model,
		Messages: []chatMessage{
			{Role: "system", Content: req.SystemPrompt},
			{Role: "user", Content: req.UserPrompt},
		},
		Temperature:    req.Temperature,
		MaxTokens:      req.MaxTokens,
		EnableThinking: false,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("dashscope status %d", resp.StatusCode)
	}

	var decoded chatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return "", err
	}
	if len(decoded.Choices) == 0 {
		return "", fmt.Errorf("dashscope returned no choices")
	}
	return decoded.Choices[0].Message.Content, nil
}

type chatCompletionRequest struct {
	Model          string        `json:"model"`
	Messages       []chatMessage `json:"messages"`
	Temperature    float64       `json:"temperature"`
	MaxTokens      int           `json:"max_tokens"`
	EnableThinking bool          `json:"enable_thinking"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatCompletionResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
}
