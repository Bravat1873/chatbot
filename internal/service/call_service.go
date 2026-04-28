// 业务逻辑层：外呼任务创建（含提交到阿里云）、回调报告处理（归一化 + 状态推导）。
package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"chatbot/internal/model"
	"chatbot/internal/repository"

	"github.com/google/uuid"
)

// providerName 标识目前使用的运营商。
const providerName = "aliyun_aiccs"

// CallProvider 外呼厂商接口，便于未来扩展（如阿里云、腾讯云）。
type CallProvider interface {
	SubmitCall(ctx context.Context, req SubmitCallRequest) (*SubmitCallResult, error)
}

// SubmitCallRequest 外呼请求参数。
type SubmitCallRequest struct {
	CalledNumber         string
	CallerNumber         string
	ApplicationCode      string
	SessionTimeoutSecond int
	BizParams            map[string]any
}

// SubmitCallResult 外呼响应结果。
type SubmitCallResult struct {
	CallID      string
	RawResponse json.RawMessage
}

// CallService 外呼业务服务，组合 repo 和 provider。
type CallService struct {
	logger         *slog.Logger
	callTaskRepo   *repository.CallTaskRepository
	provider       CallProvider
	callerNumber   string
	appCode        string
	sessionTimeout int
}

// NewCallService 创建 CallService。
func NewCallService(logger *slog.Logger, repo *repository.CallTaskRepository, provider CallProvider, callerNumber, appCode string, sessionTimeout int) *CallService {
	return &CallService{
		logger:         logger,
		callTaskRepo:   repo,
		provider:       provider,
		callerNumber:   callerNumber,
		appCode:        appCode,
		sessionTimeout: sessionTimeout,
	}
}

// APIError 业务错误，包含 HTTP 状态码，便于 handler 区分客户端/服务端错误。
type APIError struct {
	StatusCode int
	Message    string
	TaskID     *uuid.UUID
	Err        error
}

// Error 实现 error 接口。
func (e *APIError) Error() string {
	if e.TaskID == nil {
		return e.Message
	}
	return fmt.Sprintf("%s: %s", e.Message, e.TaskID.String())
}

// Unwrap 支持 errors.As/Is 进行错误链诊断。
func (e *APIError) Unwrap() error {
	return e.Err
}

// CreateCallTask 创建外呼任务并提交到阿里云 AICCS。
// 流程：参数校验 -> 写入 DB（状态=created） -> 调用阿里云 API -> 更新状态（accepted/failed）。
func (s *CallService) CreateCallTask(ctx context.Context, req model.CreateCallTaskRequest) (*model.CallTask, error) {
	// 基础参数校验
	if apiErr := validateCreateCallTaskRequest(req); apiErr != nil {
		return nil, apiErr
	}
	bizType := string(model.NormalizeBizType(req.BizType))
	requestedBy := strings.TrimSpace(req.RequestedBy)
	if requestedBy == "" {
		requestedBy = "manual"
	}
	bizParams, err := json.Marshal(req.BizParams)
	if err != nil {
		return nil, &APIError{StatusCode: http.StatusBadRequest, Message: "invalid biz_params", Err: err}
	}
	if len(bizParams) == 0 {
		bizParams = json.RawMessage(`{}`)
	}

	// 构造任务模型并入库
	now := time.Now().UTC()
	task := &model.CallTask{
		TaskID:       uuid.New(),
		Provider:     providerName,
		BizType:      bizType,
		BizParams:    bizParams,
		RequestedBy:  requestedBy,
		CalledNumber: req.CalledNumber,
		CallerNumber: s.callerNumber,
		Status:       model.CallStatusCreated,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	// 先落库记录，再调用阿里云
	task, err = s.callTaskRepo.Create(ctx, task)
	if err != nil {
		return nil, fmt.Errorf("create task in repository: %w", err)
	}
	if s.logger != nil {
		s.logger.Info("call_task_created", "task_id", task.TaskID.String(), "called_number", task.CalledNumber, "biz_type", task.BizType)
	}

	if s.logger != nil {
		s.logger.Info("call_submit_started", "task_id", task.TaskID.String())
	}
	providerBizParams := buildProviderBizParams(req.BizParams, bizType, task.TaskID)
	// 提交到阿里云 AICCS
	result, err := s.provider.SubmitCall(ctx, SubmitCallRequest{
		CalledNumber:         req.CalledNumber,
		CallerNumber:         s.callerNumber,
		ApplicationCode:      s.appCode,
		SessionTimeoutSecond: s.sessionTimeout,
		BizParams:            providerBizParams,
	})
	// 提交失败：回写失败状态
	if err != nil {
		failedAt := time.Now().UTC()
		failedTask, markErr := s.callTaskRepo.MarkSubmitFailed(ctx, task.TaskID, err.Error(), failedAt)
		if markErr != nil {
			return nil, fmt.Errorf("submit call failed and mark failed errored: submit=%w mark=%v", err, markErr)
		}
		if s.logger != nil {
			s.logger.Error("call_submit_failed", "task_id", task.TaskID.String(), "error", err.Error())
		}
		return failedTask, &APIError{StatusCode: http.StatusBadGateway, Message: "call submit failed", TaskID: &task.TaskID, Err: err}
	}
	if strings.TrimSpace(result.CallID) == "" {
		err := errors.New("empty call_id in provider response")
		failedAt := time.Now().UTC()
		failedTask, markErr := s.callTaskRepo.MarkSubmitFailed(ctx, task.TaskID, err.Error(), failedAt)
		if markErr != nil {
			return nil, fmt.Errorf("empty call_id and mark failed errored: submit=%w mark=%v", err, markErr)
		}
		return failedTask, &APIError{StatusCode: http.StatusBadGateway, Message: "call submit failed", TaskID: &task.TaskID, Err: err}
	}

	// 提交成功：标记为已接入
	acceptedTask, err := s.callTaskRepo.MarkAccepted(ctx, task.TaskID, result.CallID, result.RawResponse, time.Now().UTC())
	if err != nil {
		return nil, fmt.Errorf("mark call accepted: %w", err)
	}
	if s.logger != nil {
		s.logger.Info("call_submit_succeeded", "task_id", task.TaskID.String(), "call_id", result.CallID)
	}
	return acceptedTask, nil
}

func validateCreateCallTaskRequest(req model.CreateCallTaskRequest) *APIError {
	if strings.TrimSpace(req.CalledNumber) == "" || strings.TrimSpace(req.BizType) == "" {
		return &APIError{StatusCode: http.StatusBadRequest, Message: "invalid request"}
	}
	if !model.IsSupportedBizType(req.BizType) {
		return &APIError{StatusCode: http.StatusBadRequest, Message: "unsupported biz_type"}
	}
	return nil
}

func buildProviderBizParams(input map[string]any, bizType string, taskID uuid.UUID) map[string]any {
	output := make(map[string]any, len(input)+2)
	for key, value := range input {
		output[key] = value
	}
	output["biz_type"] = string(model.NormalizeBizType(bizType))
	output["task_id"] = taskID.String()
	return output
}

// ResolveCallBizParams 按阿里云 call_id/session_id 查询本地任务上下文，供文本网关缺失 biz_params 时兜底。
func (s *CallService) ResolveCallBizParams(ctx context.Context, callID string) (map[string]any, bool, error) {
	callID = strings.TrimSpace(callID)
	if callID == "" || s == nil || s.callTaskRepo == nil {
		return nil, false, nil
	}
	task, err := s.callTaskRepo.GetByCallID(ctx, callID)
	if err != nil {
		return nil, false, err
	}
	if task == nil {
		return nil, false, nil
	}
	bizParams, err := buildCallTaskBizParams(task)
	if err != nil {
		return nil, false, err
	}
	return bizParams, true, nil
}

func buildCallTaskBizParams(task *model.CallTask) (map[string]any, error) {
	output := map[string]any{}
	if task == nil {
		return output, nil
	}
	if len(task.BizParams) > 0 && string(task.BizParams) != "null" {
		if err := json.Unmarshal(task.BizParams, &output); err != nil {
			return nil, fmt.Errorf("decode task biz params: %w", err)
		}
		if output == nil {
			output = map[string]any{}
		}
	}
	output["biz_type"] = string(model.NormalizeBizType(task.BizType))
	output["task_id"] = task.TaskID.String()
	if task.CallID != nil {
		output["call_id"] = *task.CallID
	}
	return output, nil
}

// HandleCallReport 处理运营商回调报告：归一化载荷 -> 状态推导 -> 仓库事务写入。
func (s *CallService) HandleCallReport(ctx context.Context, payload map[string]any, rawPayload json.RawMessage, sourceIP, authMode string) (duplicate bool, matched bool, err error) {
	normalized, err := normalizePayload(payload)
	if err != nil {
		return false, false, fmt.Errorf("normalize payload: %w", err)
	}

	receivedAt := time.Now().UTC()
	report := model.CallReportPayload{
		RawPayload:              normalized,
		NormalizedPayload:       payload,
		CallID:                  findString(payload, "call_id", "callid", "CallId"),
		ProviderStatusCode:      findOptionalString(payload, "status_code", "statuscode", "StatusCode"),
		ProviderSmartStatusCode: findOptionalString(payload, "smart_status_code", "smartstatuscode", "SmartStatusCode"),
		OriginateTime:           findOptionalTime(payload, "originate_time", "OriginateTime"),
		RingTime:                findOptionalTime(payload, "ring_time", "RingTime"),
		StartTime:               findOptionalTime(payload, "start_time", "StartTime"),
		HangupTime:              findOptionalTime(payload, "hangup_time", "HangupTime"),
		ReceivedAt:              receivedAt,
		SourceIP:                sourceIP,
		AuthMode:                authMode,
		EventKey:                model.BuildEventKey(normalized),
	}
	if len(rawPayload) > 0 {
		report.RawPayload = rawPayload
	}

	updatedTask, duplicate, err := s.callTaskRepo.ApplyReport(ctx, report)
	if err != nil {
		return false, false, err
	}
	if duplicate {
		if s.logger != nil {
			s.logger.Info("call_report_duplicated", "event_key", report.EventKey, "call_id", report.CallID)
		}
		return true, false, nil
	}
	if updatedTask == nil {
		if s.logger != nil {
			s.logger.Warn("call_report_unmatched", "event_key", report.EventKey, "call_id", report.CallID)
		}
		return false, false, nil
	}
	if s.logger != nil {
		s.logger.Info(
			"call_task_status_updated",
			"task_id", updatedTask.TaskID.String(),
			"call_id", report.CallID,
			"status", updatedTask.Status,
			"event_key", report.EventKey,
		)
	}
	return false, true, nil
}

// normalizePayload 将载荷重新 JSON 序列化作为标准格式。
func normalizePayload(payload map[string]any) (json.RawMessage, error) {
	normalized, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return normalized, nil
}

// findOptionalString 从嵌套结构中按候选 key 查找字符串值，未找到返回 nil。
func findOptionalString(value any, keys ...string) *string {
	result := findString(value, keys...)
	if result == "" {
		return nil
	}
	return &result
}

// findString 从嵌套结构中按候选 key 查找字符串值。
func findString(value any, keys ...string) string {
	lookup := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		lookup[strings.ToLower(key)] = struct{}{}
	}
	result, _ := walkString(value, lookup)
	return result
}

// walkString 递归遍历 map/slice 查找匹配 key 的值。
func walkString(value any, keys map[string]struct{}) (string, bool) {
	switch current := value.(type) {
	case map[string]any:
		for key, inner := range current {
			if _, ok := keys[strings.ToLower(key)]; ok {
				if text, ok := stringify(inner); ok {
					return text, true
				}
			}
			if found, ok := walkString(inner, keys); ok {
				return found, true
			}
		}
	case []any:
		for _, inner := range current {
			if found, ok := walkString(inner, keys); ok {
				return found, true
			}
		}
	}
	return "", false
}

// stringify 将常见类型转为字符串，用于回调参数解析。
func stringify(value any) (string, bool) {
	switch current := value.(type) {
	case string:
		trimmed := strings.TrimSpace(current)
		return trimmed, trimmed != ""
	case json.Number:
		return current.String(), true
	case float64:
		return strconv.FormatFloat(current, 'f', -1, 64), true
	case float32:
		return strconv.FormatFloat(float64(current), 'f', -1, 32), true
	case int:
		return strconv.Itoa(current), true
	case int64:
		return strconv.FormatInt(current, 10), true
	case int32:
		return strconv.FormatInt(int64(current), 10), true
	case uint64:
		return strconv.FormatUint(current, 10), true
	case uint32:
		return strconv.FormatUint(uint64(current), 10), true
	case bool:
		return strconv.FormatBool(current), true
	default:
		return "", false
	}
}

// findOptionalTime 从载荷中提取可选时间字段，支持多种格式。
func findOptionalTime(value any, keys ...string) *time.Time {
	raw := findString(value, keys...)
	if raw == "" {
		return nil
	}
	parsed, err := parseFlexibleTime(raw)
	if err != nil {
		return nil
	}
	return &parsed
}

// parseFlexibleTime 解析时间字符串：先尝试 Unix 时间戳（秒/毫秒），再尝试常见布局。
func parseFlexibleTime(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, errors.New("empty time")
	}
	if unixValue, err := strconv.ParseInt(value, 10, 64); err == nil {
		if len(value) >= 13 {
			return time.UnixMilli(unixValue).UTC(), nil
		}
		return time.Unix(unixValue, 0).UTC(), nil
	}
	layouts := []string{
		time.RFC3339,
		time.RFC3339Nano,
		"2006-01-02 15:04:05",
		"2006-01-02 15:04:05 -0700 MST",
		"2006-01-02 15:04:05Z07:00",
	}
	for _, layout := range layouts {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("unsupported time format: %s", value)
}
