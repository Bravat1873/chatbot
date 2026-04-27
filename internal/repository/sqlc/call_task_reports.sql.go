// sqlc 自动生成：call_task_reports 表的数据操作代码（幂等插入 + 行扫描）。
package sqlc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"chatbot/internal/model"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type InsertCallTaskReportParams struct {
	EventKey   string
	TaskID     *uuid.UUID
	CallID     *string
	Payload    json.RawMessage
	ReceivedAt time.Time
	SourceIP   *string
	AuthMode   *string
	CreatedAt  time.Time
}

func (q *Queries) InsertCallTaskReport(ctx context.Context, arg InsertCallTaskReportParams) (model.CallTaskReport, bool, error) {
	const stmt = `
INSERT INTO call_task_reports (
    event_key, task_id, call_id, payload, received_at, source_ip, auth_mode, created_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
ON CONFLICT (event_key) DO NOTHING
RETURNING id, event_key, task_id, call_id, payload, received_at, source_ip, auth_mode, created_at`
	row := q.db.QueryRow(ctx, stmt,
		arg.EventKey,
		arg.TaskID,
		arg.CallID,
		arg.Payload,
		arg.ReceivedAt,
		arg.SourceIP,
		arg.AuthMode,
		arg.CreatedAt,
	)
	report, err := scanCallTaskReport(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.CallTaskReport{}, false, nil
		}
		return model.CallTaskReport{}, false, err
	}
	return report, true, nil
}

// scanCallTaskReport 将数据库行扫描为 CallTaskReport 模型。
func scanCallTaskReport(row interface{ Scan(dest ...any) error }) (model.CallTaskReport, error) {
	var report model.CallTaskReport
	err := row.Scan(
		&report.ID,
		&report.EventKey,
		&report.TaskID,
		&report.CallID,
		&report.Payload,
		&report.ReceivedAt,
		&report.SourceIP,
		&report.AuthMode,
		&report.CreatedAt,
	)
	if err != nil {
		return model.CallTaskReport{}, fmt.Errorf("scan call task report: %w", err)
	}
	return report, nil
}
