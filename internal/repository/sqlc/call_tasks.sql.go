// sqlc 自动生成：call_tasks 表的数据操作代码（参数结构体 + 查询方法 + 行扫描）。
package sqlc

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"chatbot/internal/model"

	"github.com/google/uuid"
)

type CreateCallTaskParams struct {
	TaskID       uuid.UUID
	Provider     string
	BizType      string
	BizParams    json.RawMessage
	RequestedBy  string
	CalledNumber string
	CallerNumber string
	Status       model.CallStatus
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type MarkCallTaskAcceptedParams struct {
	TaskID            uuid.UUID
	CallID            string
	AcceptedAt        time.Time
	RawSubmitResponse json.RawMessage
	UpdatedAt         time.Time
}

type MarkCallTaskSubmitFailedParams struct {
	TaskID      uuid.UUID
	SubmitError string
	UpdatedAt   time.Time
}

type ApplyCallReportUpdateParams struct {
	TaskID                  uuid.UUID
	Status                  model.CallStatus
	ProviderStatusCode      *string
	ProviderSmartStatusCode *string
	OriginateTime           *time.Time
	RingTime                *time.Time
	StartTime               *time.Time
	HangupTime              *time.Time
	LatestReportAt          time.Time
	UpdatedAt               time.Time
}

func (q *Queries) CreateCallTask(ctx context.Context, arg CreateCallTaskParams) (model.CallTask, error) {
	const stmt = `
INSERT INTO call_tasks (
    task_id, provider, biz_type, biz_params, requested_by,
    called_number, caller_number, status, created_at, updated_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
RETURNING
    task_id, provider, biz_type, biz_params, requested_by,
    called_number, caller_number, status, call_id, submit_error,
    provider_status_code, provider_smart_status_code,
    originate_time, ring_time, start_time, hangup_time,
    accepted_at, latest_report_at, raw_submit_response,
    created_at, updated_at`
	row := q.db.QueryRow(ctx, stmt,
		arg.TaskID,
		arg.Provider,
		arg.BizType,
		arg.BizParams,
		arg.RequestedBy,
		arg.CalledNumber,
		arg.CallerNumber,
		arg.Status,
		arg.CreatedAt,
		arg.UpdatedAt,
	)
	return scanCallTask(row)
}

func (q *Queries) GetCallTaskByCallID(ctx context.Context, callID string) (model.CallTask, error) {
	const stmt = `
SELECT
    task_id, provider, biz_type, biz_params, requested_by,
    called_number, caller_number, status, call_id, submit_error,
    provider_status_code, provider_smart_status_code,
    originate_time, ring_time, start_time, hangup_time,
    accepted_at, latest_report_at, raw_submit_response,
    created_at, updated_at
FROM call_tasks
WHERE call_id = $1`
	return scanCallTask(q.db.QueryRow(ctx, stmt, callID))
}

func (q *Queries) MarkCallTaskAccepted(ctx context.Context, arg MarkCallTaskAcceptedParams) (model.CallTask, error) {
	const stmt = `
UPDATE call_tasks
SET call_id = $2,
    status = 'accepted',
    accepted_at = $3,
    raw_submit_response = $4,
    updated_at = $5
WHERE task_id = $1
RETURNING
    task_id, provider, biz_type, biz_params, requested_by,
    called_number, caller_number, status, call_id, submit_error,
    provider_status_code, provider_smart_status_code,
    originate_time, ring_time, start_time, hangup_time,
    accepted_at, latest_report_at, raw_submit_response,
    created_at, updated_at`
	return scanCallTask(q.db.QueryRow(ctx, stmt,
		arg.TaskID,
		arg.CallID,
		arg.AcceptedAt,
		arg.RawSubmitResponse,
		arg.UpdatedAt,
	))
}

func (q *Queries) MarkCallTaskSubmitFailed(ctx context.Context, arg MarkCallTaskSubmitFailedParams) (model.CallTask, error) {
	const stmt = `
UPDATE call_tasks
SET status = 'submit_failed',
    submit_error = $2,
    updated_at = $3
WHERE task_id = $1
RETURNING
    task_id, provider, biz_type, biz_params, requested_by,
    called_number, caller_number, status, call_id, submit_error,
    provider_status_code, provider_smart_status_code,
    originate_time, ring_time, start_time, hangup_time,
    accepted_at, latest_report_at, raw_submit_response,
    created_at, updated_at`
	return scanCallTask(q.db.QueryRow(ctx, stmt,
		arg.TaskID,
		arg.SubmitError,
		arg.UpdatedAt,
	))
}

func (q *Queries) ApplyCallReportUpdate(ctx context.Context, arg ApplyCallReportUpdateParams) (model.CallTask, error) {
	const stmt = `
UPDATE call_tasks
SET status = $2,
    provider_status_code = COALESCE($3, provider_status_code),
    provider_smart_status_code = COALESCE($4, provider_smart_status_code),
    originate_time = COALESCE($5, originate_time),
    ring_time = COALESCE($6, ring_time),
    start_time = COALESCE($7, start_time),
    hangup_time = COALESCE($8, hangup_time),
    latest_report_at = $9,
    updated_at = $10
WHERE task_id = $1
RETURNING
    task_id, provider, biz_type, biz_params, requested_by,
    called_number, caller_number, status, call_id, submit_error,
    provider_status_code, provider_smart_status_code,
    originate_time, ring_time, start_time, hangup_time,
    accepted_at, latest_report_at, raw_submit_response,
    created_at, updated_at`
	return scanCallTask(q.db.QueryRow(ctx, stmt,
		arg.TaskID,
		arg.Status,
		arg.ProviderStatusCode,
		arg.ProviderSmartStatusCode,
		arg.OriginateTime,
		arg.RingTime,
		arg.StartTime,
		arg.HangupTime,
		arg.LatestReportAt,
		arg.UpdatedAt,
	))
}

// scanCallTask 将数据库行扫描为模型 struct，所有查询方法共用。
func scanCallTask(row interface{ Scan(dest ...any) error }) (model.CallTask, error) {
	var task model.CallTask
	err := row.Scan(
		&task.TaskID,
		&task.Provider,
		&task.BizType,
		&task.BizParams,
		&task.RequestedBy,
		&task.CalledNumber,
		&task.CallerNumber,
		&task.Status,
		&task.CallID,
		&task.SubmitError,
		&task.ProviderStatusCode,
		&task.ProviderSmartStatusCode,
		&task.OriginateTime,
		&task.RingTime,
		&task.StartTime,
		&task.HangupTime,
		&task.AcceptedAt,
		&task.LatestReportAt,
		&task.RawSubmitResponse,
		&task.CreatedAt,
		&task.UpdatedAt,
	)
	if err != nil {
		return model.CallTask{}, fmt.Errorf("scan call task: %w", err)
	}
	return task, nil
}
