-- call_tasks 查询集：外呼任务的创建、查询与状态更新。

-- name: CreateCallTask :one
INSERT INTO call_tasks (
    task_id, provider, biz_type, biz_params, requested_by,
    called_number, caller_number, status, created_at, updated_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
RETURNING *;

-- name: GetCallTaskByCallID :one
SELECT * FROM call_tasks WHERE call_id = $1;

-- name: MarkCallTaskAccepted :one
UPDATE call_tasks
SET call_id = $2,
    status = 'accepted',
    accepted_at = $3,
    raw_submit_response = $4,
    updated_at = $5
WHERE task_id = $1
RETURNING *;

-- name: MarkCallTaskSubmitFailed :one
UPDATE call_tasks
SET status = 'submit_failed',
    submit_error = $2,
    updated_at = $3
WHERE task_id = $1
RETURNING *;

-- name: ApplyCallReportUpdate :one
-- 使用 COALESCE 只更新非空字段，避免覆盖已有时间戳。
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
RETURNING *;
