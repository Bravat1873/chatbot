// 数据模型层：定义外呼任务、回调报告的状态机和核心结构体。
package model

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// CallStatus 表示外呼任务的生命周期状态。
type CallStatus string

const (
	CallStatusCreated      CallStatus = "created"
	CallStatusSubmitFailed CallStatus = "submit_failed"
	CallStatusAccepted     CallStatus = "accepted"
	CallStatusRinging      CallStatus = "ringing"
	CallStatusAnswered     CallStatus = "answered"
	CallStatusCompleted    CallStatus = "completed"
	CallStatusFailed       CallStatus = "failed"
)

// statusPriority 定义状态优先级，用于判断状态升级方向。
var statusPriority = map[CallStatus]int{
	CallStatusCreated:      1,
	CallStatusSubmitFailed: 2,
	CallStatusAccepted:     3,
	CallStatusRinging:      4,
	CallStatusAnswered:     5,
	CallStatusCompleted:    6,
}

// CallTask 外呼任务完整模型，映射 call_tasks 表。
type CallTask struct {
	TaskID                  uuid.UUID       `json:"task_id"`
	Provider                string          `json:"provider"`
	BizType                 string          `json:"biz_type"`
	BizParams               json.RawMessage `json:"biz_params"`
	RequestedBy             string          `json:"requested_by"`
	CalledNumber            string          `json:"called_number"`
	CallerNumber            string          `json:"caller_number"`
	Status                  CallStatus      `json:"status"`
	CallID                  *string         `json:"call_id,omitempty"`
	SubmitError             *string         `json:"submit_error,omitempty"`
	ProviderStatusCode      *string         `json:"provider_status_code,omitempty"`
	ProviderSmartStatusCode *string         `json:"provider_smart_status_code,omitempty"`
	OriginateTime           *time.Time      `json:"originate_time,omitempty"`
	RingTime                *time.Time      `json:"ring_time,omitempty"`
	StartTime               *time.Time      `json:"start_time,omitempty"`
	HangupTime              *time.Time      `json:"hangup_time,omitempty"`
	AcceptedAt              *time.Time      `json:"accepted_at,omitempty"`
	LatestReportAt          *time.Time      `json:"latest_report_at,omitempty"`
	RawSubmitResponse       json.RawMessage `json:"raw_submit_response,omitempty"`
	CreatedAt               time.Time       `json:"created_at"`
	UpdatedAt               time.Time       `json:"updated_at"`
}

// CallTaskReport 回调报告模型，映射 call_task_reports 表。
type CallTaskReport struct {
	ID         int64           `json:"id"`
	EventKey   string          `json:"event_key"`
	TaskID     *uuid.UUID      `json:"task_id,omitempty"`
	CallID     *string         `json:"call_id,omitempty"`
	Payload    json.RawMessage `json:"payload"`
	ReceivedAt time.Time       `json:"received_at"`
	SourceIP   *string         `json:"source_ip,omitempty"`
	AuthMode   *string         `json:"auth_mode,omitempty"`
	CreatedAt  time.Time       `json:"created_at"`
}

// CreateCallTaskRequest 创建外呼任务的 HTTP 请求体。
type CreateCallTaskRequest struct {
	CalledNumber string         `json:"called_number"`
	BizType      string         `json:"biz_type"`
	BizParams    map[string]any `json:"biz_params"`
	RequestedBy  string         `json:"requested_by"`
}

// CallReportPayload 归一化后的回调载荷，供 handler -> service -> repo 流转。
type CallReportPayload struct {
	RawPayload              json.RawMessage
	NormalizedPayload       map[string]any
	CallID                  string
	ProviderStatusCode      *string
	ProviderSmartStatusCode *string
	OriginateTime           *time.Time
	RingTime                *time.Time
	StartTime               *time.Time
	HangupTime              *time.Time
	ReceivedAt              time.Time
	SourceIP                string
	AuthMode                string
	EventKey                string
}

// CallReportUpdate 回调更新参数，用于批量更新任务状态和时间。
type CallReportUpdate struct {
	TaskID                  uuid.UUID
	Status                  CallStatus
	ProviderStatusCode      *string
	ProviderSmartStatusCode *string
	OriginateTime           *time.Time
	RingTime                *time.Time
	StartTime               *time.Time
	HangupTime              *time.Time
	LatestReportAt          time.Time
}

// NormalizeStatus 按优先级合并状态：终态不可逆，其余取优先级更高的。
func NormalizeStatus(current, candidate CallStatus) CallStatus {
	if candidate == "" {
		return current
	}
	if current == CallStatusCompleted || current == CallStatusFailed {
		return current
	}
	if candidate == CallStatusFailed {
		return CallStatusFailed
	}
	if statusPriority[candidate] > statusPriority[current] {
		return candidate
	}
	return current
}

// DeriveNextStatus 根据回调载荷推导任务的最新状态。
func DeriveNextStatus(current CallStatus, payload CallReportPayload) CallStatus {
	status := current
	if payload.RingTime != nil {
		status = NormalizeStatus(status, CallStatusRinging)
	}
	if payload.StartTime != nil {
		status = NormalizeStatus(status, CallStatusAnswered)
	}
	if payload.HangupTime != nil {
		if status == CallStatusAnswered || current == CallStatusAnswered || payload.StartTime != nil {
			return NormalizeStatus(status, CallStatusCompleted)
		}
		return NormalizeStatus(status, CallStatusFailed)
	}
	if isFailureCode(payload.ProviderStatusCode, payload.ProviderSmartStatusCode) {
		return NormalizeStatus(status, CallStatusFailed)
	}
	return status
}

// BuildEventKey 使用 SHA256 生成事件去重键，防止重复处理同一回调。
func BuildEventKey(normalized json.RawMessage) string {
	sum := sha256.Sum256(normalized)
	return hex.EncodeToString(sum[:])
}

// isFailureCode 判断状态码是否表示失败（包括挂断等终止态）。
func isFailureCode(statusCode, smartStatusCode *string) bool {
	for _, value := range []*string{statusCode, smartStatusCode} {
		if value == nil {
			continue
		}
		switch *value {
		case "failed", "fail", "error", "hangup_before_answer", "not_connected", "no_answer", "busy":
			return true
		}
	}
	return false
}
