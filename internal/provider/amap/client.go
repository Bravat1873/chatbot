package amap

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"chatbot/internal/core"
)

type Config struct {
	APIKey  string
	BaseURL string
	City    string
	Timeout time.Duration
}

type Client struct {
	apiKey     string
	baseURL    string
	city       string
	httpClient *http.Client
}

func New(config Config) *Client {
	baseURL := strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	if baseURL == "" {
		baseURL = "https://restapi.amap.com/v3"
	}
	timeout := config.Timeout
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	return &Client{
		apiKey:  strings.TrimSpace(config.APIKey),
		baseURL: baseURL,
		city:    strings.TrimSpace(config.City),
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

func (c *Client) ResolvePlace(ctx context.Context, keywords string) (core.GeocodeResult, error) {
	if c.apiKey == "" {
		return core.GeocodeResult{Error: "未配置 AMAP_KEY"}, nil
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/place/text", nil)
	if err != nil {
		return core.GeocodeResult{}, err
	}
	query := httpReq.URL.Query()
	query.Set("key", c.apiKey)
	query.Set("keywords", strings.TrimSpace(keywords))
	query.Set("output", "json")
	if c.city != "" {
		query.Set("city", c.city)
	}
	httpReq.URL.RawQuery = query.Encode()

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return core.GeocodeResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return core.GeocodeResult{}, fmt.Errorf("amap status %d", resp.StatusCode)
	}

	var decoded placeTextResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return core.GeocodeResult{}, err
	}
	if decoded.Status != "1" {
		return core.GeocodeResult{Error: decoded.Info}, nil
	}
	if len(decoded.POIs) == 0 {
		return core.GeocodeResult{Found: false, Error: "未匹配到有效地址"}, nil
	}
	poi := decoded.POIs[0]
	candidate := &core.PlaceCandidate{
		Name:        poi.Name,
		Address:     poi.Address,
		District:    poi.District,
		DisplayText: buildDisplayText(poi),
		Raw: map[string]any{
			"name":     poi.Name,
			"address":  poi.Address,
			"district": poi.District,
		},
	}
	return core.GeocodeResult{Found: true, Best: candidate}, nil
}

type placeTextResponse struct {
	Status string `json:"status"`
	Info   string `json:"info"`
	POIs   []poi  `json:"pois"`
}

type poi struct {
	Name     string `json:"name"`
	Address  string `json:"address"`
	District string `json:"adname"`
}

func buildDisplayText(p poi) string {
	parts := []string{p.District, p.Address, p.Name}
	result := ""
	for _, part := range parts {
		if part == "" || strings.Contains(result, part) {
			continue
		}
		result += part
	}
	return result
}
