-- name: UpsertScheduledJob :one
-- Inserts a job row on first registration; on conflict (same name) only
-- the schedule and payload are refreshed so callers can change cron
-- expressions in code without manual intervention. Run-status fields
-- (last_run_at, last_status, last_error, next_run_at) are preserved.
INSERT INTO scheduled_jobs (name, schedule, payload, enabled, next_run_at)
VALUES (?, ?, ?, 1, ?)
ON CONFLICT(name) DO UPDATE SET
    schedule   = excluded.schedule,
    payload    = excluded.payload,
    updated_at = CURRENT_TIMESTAMP
RETURNING *;

-- name: GetScheduledJob :one
SELECT * FROM scheduled_jobs WHERE name = ? LIMIT 1;

-- name: ListScheduledJobs :many
SELECT * FROM scheduled_jobs ORDER BY name;

-- name: SetScheduledJobNextRun :exec
UPDATE scheduled_jobs
SET next_run_at = ?, updated_at = CURRENT_TIMESTAMP
WHERE name = ?;

-- name: RecordScheduledJobRun :exec
UPDATE scheduled_jobs
SET last_run_at = ?,
    next_run_at = ?,
    last_status = ?,
    last_error  = ?,
    updated_at  = CURRENT_TIMESTAMP
WHERE name = ?;

-- name: SetScheduledJobEnabled :exec
UPDATE scheduled_jobs
SET enabled = ?, updated_at = CURRENT_TIMESTAMP
WHERE name = ?;
