-- 数据库迁移：创建外呼任务表和回调报告表，包含索引和降级脚本。

-- +goose Up
-- call_tasks：外呼任务主表，记录从创建到完成的完整生命周期。
CREATE TABLE IF NOT EXISTS call_tasks (
    task_id UUID PRIMARY KEY,
    provider TEXT NOT NULL,
    biz_type TEXT NOT NULL,
    biz_params JSONB NOT NULL DEFAULT '{}'::jsonb,
    requested_by TEXT NOT NULL,
    called_number TEXT NOT NULL,
    caller_number TEXT NOT NULL,
    status TEXT NOT NULL,
    call_id TEXT NULL,
    submit_error TEXT NULL,
    provider_status_code TEXT NULL,
    provider_smart_status_code TEXT NULL,
    originate_time TIMESTAMPTZ NULL,
    ring_time TIMESTAMPTZ NULL,
    start_time TIMESTAMPTZ NULL,
    hangup_time TIMESTAMPTZ NULL,
    accepted_at TIMESTAMPTZ NULL,
    latest_report_at TIMESTAMPTZ NULL,
    raw_submit_response JSONB NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

-- 部分唯一索引：允许多个 NULL call_id，非空时保证唯一。
CREATE UNIQUE INDEX IF NOT EXISTS idx_call_tasks_call_id
    ON call_tasks(call_id)
    WHERE call_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_call_tasks_status_created_at
    ON call_tasks(status, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_call_tasks_called_number
    ON call_tasks(called_number);

CREATE INDEX IF NOT EXISTS idx_call_tasks_latest_report_at
    ON call_tasks(latest_report_at DESC);

-- call_task_reports：运营商回调报告表，event_key 作为幂等去重键。
CREATE TABLE IF NOT EXISTS call_task_reports (
    id BIGSERIAL PRIMARY KEY,
    event_key TEXT NOT NULL,
    task_id UUID NULL,
    call_id TEXT NULL,
    payload JSONB NOT NULL,
    received_at TIMESTAMPTZ NOT NULL,
    source_ip TEXT NULL,
    auth_mode TEXT NULL,
    created_at TIMESTAMPTZ NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_call_task_reports_event_key
    ON call_task_reports(event_key);

CREATE INDEX IF NOT EXISTS idx_call_task_reports_call_id
    ON call_task_reports(call_id);

CREATE INDEX IF NOT EXISTS idx_call_task_reports_received_at
    ON call_task_reports(received_at DESC);

-- +goose Down
DROP TABLE IF EXISTS call_task_reports;
DROP TABLE IF EXISTS call_tasks;
