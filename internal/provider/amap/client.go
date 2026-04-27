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
	cleaned := strings.TrimSpace(keywords)
	if cleaned == "" {
		return core.GeocodeResult{Found: false, Error: "地址关键词为空"}, nil
	}
	tipsResult, tipsErr := c.GetInputTips(ctx, cleaned)
	poisResult, poisErr := c.SearchPlace(ctx, cleaned)
	if sharedError := pickSharedError(tipsResult.Error, poisResult.Error); sharedError != "" {
		return core.GeocodeResult{Found: false, Error: sharedError}, nil
	}
	if tipsErr != nil && poisErr != nil {
		return core.GeocodeResult{}, fmt.Errorf("amap resolve failed: inputtips=%w; poi=%v", tipsErr, poisErr)
	}
	candidates := core.MergePlaceCandidates(cleaned, tipsResult.Tips, poisResult.POIs, c.city, func(text string, city string) core.AddressVerifyResult {
		result, err := c.VerifyAddress(ctx, text)
		if err != nil {
			return core.AddressVerifyResult{Success: false, Error: err.Error()}
		}
		return result
	})
	if len(candidates) == 0 {
		errText := firstNonEmpty(tipsResult.Error, poisResult.Error)
		return core.GeocodeResult{
			Found: false,
			Error: errText,
			Tips:  tipsResult.Tips,
			POIs:  poisResult.POIs,
		}, nil
	}
	return core.GeocodeResult{
		Found:      true,
		Best:       &candidates[0],
		Candidates: candidates,
		Tips:       tipsResult.Tips,
		POIs:       poisResult.POIs,
	}, nil
}

func (c *Client) VerifyAddress(ctx context.Context, addressText string) (core.AddressVerifyResult, error) {
	cleaned := strings.TrimSpace(addressText)
	if cleaned == "" {
		return core.AddressVerifyResult{Success: false, Error: "地址为空,无法复核。"}, nil
	}
	if c.apiKey == "" {
		return core.AddressVerifyResult{Success: false, Error: "未配置 AMAP_KEY,无法调用高德地址复核。"}, nil
	}
	var decoded geocodeResponse
	if err := c.getJSON(ctx, "/geocode/geo", map[string]string{
		"address": cleaned,
		"city":    c.city,
	}, &decoded); err != nil {
		return core.AddressVerifyResult{}, err
	}
	if decoded.Status != "1" {
		return core.AddressVerifyResult{Success: false, Error: firstNonEmpty(decoded.Info, "高德地址复核失败。")}, nil
	}
	if len(decoded.Geocodes) == 0 {
		return core.AddressVerifyResult{Success: false, Error: "未匹配到有效地址,请补充更详细的门牌信息。"}, nil
	}
	first := decoded.Geocodes[0]
	return core.AddressVerifyResult{
		Success:     true,
		Formatted:   first.FormattedAddress,
		Level:       first.Level,
		Location:    first.Location,
		PrecisionOK: core.IsPreciseEnough(first.Level, first.FormattedAddress),
	}, nil
}

func (c *Client) GetInputTips(ctx context.Context, keywords string) (core.GeocodeResult, error) {
	cleaned := strings.TrimSpace(keywords)
	if cleaned == "" {
		return core.GeocodeResult{Found: false, Error: "联想关键词为空"}, nil
	}
	if c.apiKey == "" {
		return core.GeocodeResult{Found: false, Error: "未配置 AMAP_KEY"}, nil
	}
	var decoded inputTipsResponse
	if err := c.getJSON(ctx, "/assistant/inputtips", map[string]string{
		"keywords":  cleaned,
		"city":      c.city,
		"citylimit": "true",
		"datatype":  "all",
	}, &decoded); err != nil {
		return core.GeocodeResult{}, err
	}
	if decoded.Status != "1" {
		return core.GeocodeResult{Found: false, Error: firstNonEmpty(decoded.Info, "InputTips 请求失败")}, nil
	}
	results := make([]core.PlaceCandidate, 0, min(len(decoded.Tips), 5))
	for _, tip := range decoded.Tips {
		if len(results) >= 5 {
			break
		}
		name := strings.TrimSpace(tip.Name)
		address := strings.TrimSpace(tip.Address.String())
		district := strings.TrimSpace(tip.District)
		displayText := core.BuildDisplayText(name, address, district)
		if displayText == "" {
			continue
		}
		results = append(results, core.PlaceCandidate{
			Name:        name,
			Address:     address,
			District:    district,
			Location:    strings.TrimSpace(tip.Location),
			DisplayText: displayText,
			Source:      "input_tips",
		})
	}
	return core.GeocodeResult{Found: len(results) > 0, Tips: results}, nil
}

func (c *Client) SearchPlace(ctx context.Context, keywords string) (core.GeocodeResult, error) {
	cleaned := strings.TrimSpace(keywords)
	if cleaned == "" {
		return core.GeocodeResult{Found: false, Error: "搜索关键词为空"}, nil
	}
	if c.apiKey == "" {
		return core.GeocodeResult{Found: false, Error: "未配置 AMAP_KEY"}, nil
	}
	var decoded placeTextResponse
	if err := c.getJSON(ctx, "/place/text", map[string]string{
		"keywords":  cleaned,
		"city":      c.city,
		"citylimit": "true",
	}, &decoded); err != nil {
		return core.GeocodeResult{}, err
	}
	if decoded.Status != "1" {
		return core.GeocodeResult{Found: false, Error: firstNonEmpty(decoded.Info, "搜索失败")}, nil
	}
	results := make([]core.PlaceCandidate, 0, min(len(decoded.POIs), 3))
	for _, poi := range decoded.POIs {
		if len(results) >= 3 {
			break
		}
		district := firstNonEmpty(poi.AdName, poi.CityName)
		displayText := core.BuildDisplayText(poi.Name, poi.Address.String(), district)
		results = append(results, core.PlaceCandidate{
			Name:        strings.TrimSpace(poi.Name),
			Address:     strings.TrimSpace(poi.Address.String()),
			District:    strings.TrimSpace(district),
			Location:    strings.TrimSpace(poi.Location),
			DisplayText: displayText,
			Source:      "poi_search",
			Raw: map[string]any{
				"cityname": poi.CityName,
				"adname":   poi.AdName,
			},
		})
	}
	return core.GeocodeResult{Found: len(results) > 0, POIs: results}, nil
}

func (c *Client) getJSON(ctx context.Context, path string, params map[string]string, target any) error {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	query := httpReq.URL.Query()
	query.Set("key", c.apiKey)
	query.Set("output", "json")
	for key, value := range params {
		if strings.TrimSpace(value) != "" {
			query.Set(key, value)
		}
	}
	httpReq.URL.RawQuery = query.Encode()

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("amap status %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(target)
}

type geocodeResponse struct {
	Status   string `json:"status"`
	Info     string `json:"info"`
	Geocodes []struct {
		FormattedAddress string `json:"formatted_address"`
		Level            string `json:"level"`
		Location         string `json:"location"`
	} `json:"geocodes"`
}

type inputTipsResponse struct {
	Status string `json:"status"`
	Info   string `json:"info"`
	Tips   []struct {
		Name     string     `json:"name"`
		Address  amapString `json:"address"`
		District string     `json:"district"`
		Location string     `json:"location"`
	} `json:"tips"`
}

type placeTextResponse struct {
	Status string `json:"status"`
	Info   string `json:"info"`
	POIs   []struct {
		Name     string     `json:"name"`
		Address  amapString `json:"address"`
		Location string     `json:"location"`
		CityName string     `json:"cityname"`
		AdName   string     `json:"adname"`
	} `json:"pois"`
}

type amapString string

func (s *amapString) UnmarshalJSON(data []byte) error {
	var text string
	if err := json.Unmarshal(data, &text); err == nil {
		*s = amapString(text)
		return nil
	}
	var values []string
	if err := json.Unmarshal(data, &values); err == nil {
		*s = amapString(strings.Join(values, ""))
		return nil
	}
	*s = ""
	return nil
}

func (s amapString) String() string {
	return string(s)
}

func pickSharedError(errors ...string) string {
	collected := make([]string, 0, len(errors))
	for _, errText := range errors {
		if strings.TrimSpace(errText) != "" {
			collected = append(collected, errText)
		}
	}
	if len(collected) == 0 {
		return ""
	}
	for _, errText := range collected {
		if !strings.HasPrefix(errText, "未配置 AMAP_KEY") {
			return ""
		}
	}
	return collected[0]
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
