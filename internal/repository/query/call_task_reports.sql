-- call_task_reports 查询集：回调报告的幂等写入（通过 event_key 唯一约束实现去重）。

-- name: InsertCallTaskReport :one
-- ON CONFLICT DO NOTHING 保证重复回调被静默忽略。
INSERT INTO call_task_reports (
    event_key, task_id, call_id, payload, received_at, source_ip, auth_mode, created_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
ON CONFLICT (event_key) DO NOTHING
RETURNING *;
