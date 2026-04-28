// 阿里云 AICCS Provider：封装 LlmSmartCall API，实现外呼提交与响应解析。
package aliyun

import (
	"context"
	"encoding/json"
	"fmt"

	"chatbot/internal/service"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk"
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/auth/credentials"
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/requests"
)

// Config 阿里云 SDK 所需配置。
type Config struct {
	AccessKeyID     string
	AccessKeySecret string
	RegionID        string
	Endpoint        string
}

// Provider 封装阿里云 SDK Client。
type Provider struct {
	client *sdk.Client
	config Config
}

// New 使用 AK/SK 创建阿里云 SDK Client。
func New(config Config) (*Provider, error) {
	credential := credentials.NewAccessKeyCredential(config.AccessKeyID, config.AccessKeySecret)
	client, err := sdk.NewClientWithOptions(config.RegionID, sdk.NewConfig(), credential)
	if err != nil {
		return nil, fmt.Errorf("create aliyun sdk client: %w", err)
	}
	return &Provider{client: client, config: config}, nil
}

// SubmitCall 调用 LlmSmartCall API 发起外呼。
func (p *Provider) SubmitCall(ctx context.Context, req service.SubmitCallRequest) (*service.SubmitCallResult, error) {
	commonRequest, err := buildLlmSmartCallRequest(p.config, req)
	if err != nil {
		return nil, err
	}
	_ = ctx
	response, err := p.client.ProcessCommonRequest(commonRequest)
	if err != nil {
		return nil, fmt.Errorf("invoke aliyun llm smart call: %w", err)
	}

	raw := json.RawMessage(response.GetHttpContentBytes())
	callID, code, message, err := extractCallResponse(raw)
	if err != nil {
		return nil, err
	}
	if code != "" && code != "OK" && code != "200" {
		if message == "" {
			message = "aliyun returned non-success code"
		}
		return nil, fmt.Errorf("aliyun llm smart call failed: code=%s message=%s", code, message)
	}
	if callID == "" {
		return nil, fmt.Errorf("aliyun llm smart call returned empty call id")
	}
	return &service.SubmitCallResult{CallID: callID, RawResponse: raw}, nil
}

func buildLlmSmartCallRequest(config Config, req service.SubmitCallRequest) (*requests.CommonRequest, error) {
	commonRequest := requests.NewCommonRequest()
	commonRequest.Method = requests.POST
	commonRequest.Scheme = "https"
	commonRequest.Domain = config.Endpoint
	commonRequest.Version = "2019-10-15"
	commonRequest.ApiName = "LlmSmartCall"
	commonRequest.Product = "aiccs"
	commonRequest.QueryParams["CalledNumber"] = req.CalledNumber
	commonRequest.QueryParams["CallerNumber"] = req.CallerNumber
	commonRequest.QueryParams["ApplicationCode"] = req.ApplicationCode
	commonRequest.QueryParams["SessionTimeout"] = fmt.Sprintf("%d", req.SessionTimeoutSecond)
	if len(req.BizParams) > 0 {
		encodedBizParams, err := json.Marshal(req.BizParams)
		if err != nil {
			return nil, fmt.Errorf("marshal biz params: %w", err)
		}
		commonRequest.QueryParams["BizParam"] = string(encodedBizParams)
	}
	if len(req.StartWordParams) > 0 {
		encodedStartWordParams, err := json.Marshal(req.StartWordParams)
		if err != nil {
			return nil, fmt.Errorf("marshal start word params: %w", err)
		}
		commonRequest.QueryParams["StartWordParam"] = string(encodedStartWordParams)
	}
	return commonRequest, nil
}

// llmSmartCallResponse 阿里云 LlmSmartCall 返回结构。
type llmSmartCallResponse struct {
	RequestID string `json:"RequestId"`
	CallID    string `json:"CallId"`
	Code      string `json:"Code"`
	Message   string `json:"Message"`
	Data      string `json:"Data"`
}

// extractCallResponse 解析阿里云响应，优先取 CallId 字段，兼容部分 API 将 CallId 放在 Data 字段的情况。
func extractCallResponse(raw json.RawMessage) (callID, code, message string, err error) {
	var response llmSmartCallResponse
	if err := json.Unmarshal(raw, &response); err != nil {
		return "", "", "", fmt.Errorf("decode aliyun response: %w", err)
	}
	callID = response.CallID
	if callID == "" {
		callID = response.Data
	}
	return callID, response.Code, response.Message, nil
}
