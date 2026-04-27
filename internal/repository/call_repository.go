// 数据访问层：封装 call_tasks 和 call_task_reports 的 CRUD 操作，回调写入使用事务保证原子性。
package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"chatbot/internal/model"
	"chatbot/internal/repository/sqlc"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// CallTaskRepository 持有一组 sqlc 生成的查询方法，提供面向业务的方法签名。
type CallTaskRepository struct {
	pool    *pgxpool.Pool
	queries *sqlc.Queries
}

// NewCallTaskRepository 创建仓库实例。
func NewCallTaskRepository(pool *pgxpool.Pool) *CallTaskRepository {
	return &CallTaskRepository{
		pool:    pool,
		queries: sqlc.New(pool),
	}
}

// Create 创建一条外呼任务记录。
func (r *CallTaskRepository) Create(ctx context.Context, task *model.CallTask) (*model.CallTask, error) {
	created, err := r.queries.CreateCallTask(ctx, sqlc.CreateCallTaskParams{
		TaskID:       task.TaskID,
		Provider:     task.Provider,
		BizType:      task.BizType,
		BizParams:    task.BizParams,
		RequestedBy:  task.RequestedBy,
		CalledNumber: task.CalledNumber,
		CallerNumber: task.CallerNumber,
		Status:       task.Status,
		CreatedAt:    task.CreatedAt,
		UpdatedAt:    task.UpdatedAt,
	})
	if err != nil {
		return nil, fmt.Errorf("create call task: %w", err)
	}
	return &created, nil
}

// GetByCallID 根据运营商返回的 CallID 查询关联的任务，可能返回 nil（不存在）。
func (r *CallTaskRepository) GetByCallID(ctx context.Context, callID string) (*model.CallTask, error) {
	task, err := r.queries.GetCallTaskByCallID(ctx, callID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get call task by call id: %w", err)
	}
	return &task, nil
}

// MarkAccepted 将任务标记为已接入，记录 CallID 和原始响应。
func (r *CallTaskRepository) MarkAccepted(ctx context.Context, taskID uuid.UUID, callID string, raw json.RawMessage, acceptedAt time.Time) (*model.CallTask, error) {
	task, err := r.queries.MarkCallTaskAccepted(ctx, sqlc.MarkCallTaskAcceptedParams{
		TaskID:            taskID,
		CallID:            callID,
		AcceptedAt:        acceptedAt,
		RawSubmitResponse: raw,
		UpdatedAt:         acceptedAt,
	})
	if err != nil {
		return nil, fmt.Errorf("mark call task accepted: %w", err)
	}
	return &task, nil
}

// MarkSubmitFailed 将任务标记为提交失败，记录错误信息。
func (r *CallTaskRepository) MarkSubmitFailed(ctx context.Context, taskID uuid.UUID, submitError string, updatedAt time.Time) (*model.CallTask, error) {
	task, err := r.queries.MarkCallTaskSubmitFailed(ctx, sqlc.MarkCallTaskSubmitFailedParams{
		TaskID:      taskID,
		SubmitError: submitError,
		UpdatedAt:   updatedAt,
	})
	if err != nil {
		return nil, fmt.Errorf("mark call task submit failed: %w", err)
	}
	return &task, nil
}

// ApplyReport 在事务中处理回调：先写入报告（去重），再更新任务状态。
// 返回值：(更新后的task, 是否重复, 错误)
func (r *CallTaskRepository) ApplyReport(ctx context.Context, report model.CallReportPayload) (*model.CallTask, bool, error) {
	// 开启事务，确保报告写入与状态更新原子执行
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, false, fmt.Errorf("begin report tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }() // 确保异常时回滚

	queries := r.queries.WithTx(tx)

	// 尝试根据 CallID 匹配已有任务
	var taskID *uuid.UUID
	task, err := queries.GetCallTaskByCallID(ctx, report.CallID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, false, fmt.Errorf("get task in report tx: %w", err)
	}
	if err == nil {
		taskID = &task.TaskID
	}

	var sourceIP *string
	if report.SourceIP != "" {
		sourceIP = &report.SourceIP
	}
	var authMode *string
	if report.AuthMode != "" {
		authMode = &report.AuthMode
	}
	var callID *string
	if report.CallID != "" {
		callID = &report.CallID
	}

	// 写入报告（利用 event_key 唯一约束实现幂等）
	_, inserted, err := queries.InsertCallTaskReport(ctx, sqlc.InsertCallTaskReportParams{
		EventKey:   report.EventKey,
		TaskID:     taskID,
		CallID:     callID,
		Payload:    report.RawPayload,
		ReceivedAt: report.ReceivedAt,
		SourceIP:   sourceIP,
		AuthMode:   authMode,
		CreatedAt:  report.ReceivedAt,
	})
	if err != nil {
		return nil, false, fmt.Errorf("insert call task report: %w", err)
	}
	// 报告已存在（重复回调），直接提交返回
	if !inserted {
		if err := tx.Commit(ctx); err != nil {
			return nil, false, fmt.Errorf("commit duplicate report tx: %w", err)
		}
		return nil, true, nil
	}

	// 存在报告但 CallID 无法匹配任何任务（孤立回调）
	if taskID == nil {
		if err := tx.Commit(ctx); err != nil {
			return nil, false, fmt.Errorf("commit unmatched report tx: %w", err)
		}
		return nil, false, nil
	}

	nextStatus := model.DeriveNextStatus(task.Status, report)
	updatedTask, err := queries.ApplyCallReportUpdate(ctx, sqlc.ApplyCallReportUpdateParams{
		TaskID:                  *taskID,
		Status:                  nextStatus,
		ProviderStatusCode:      report.ProviderStatusCode,
		ProviderSmartStatusCode: report.ProviderSmartStatusCode,
		OriginateTime:           report.OriginateTime,
		RingTime:                report.RingTime,
		StartTime:               report.StartTime,
		HangupTime:              report.HangupTime,
		LatestReportAt:          report.ReceivedAt,
		UpdatedAt:               report.ReceivedAt,
	})
	if err != nil {
		return nil, false, fmt.Errorf("apply call report update: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, false, fmt.Errorf("commit report tx: %w", err)
	}
	return &updatedTask, false, nil
}
