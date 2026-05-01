-- name: UpsertScheduledJob :one
INSERT INTO scheduled_jobs (name, schedule, payload, enabled, next_run_at)
VALUES ($1, $2, $3, TRUE, $4)
ON CONFLICT(name) DO UPDATE SET
    schedule   = EXCLUDED.schedule,
    payload    = EXCLUDED.payload,
    updated_at = NOW()
RETURNING *;

-- name: GetScheduledJob :one
SELECT * FROM scheduled_jobs WHERE name = $1 LIMIT 1;

-- name: ListScheduledJobs :many
SELECT * FROM scheduled_jobs ORDER BY name;

-- name: SetScheduledJobNextRun :exec
UPDATE scheduled_jobs
SET next_run_at = $2, updated_at = NOW()
WHERE name = $1;

-- name: RecordScheduledJobRun :exec
UPDATE scheduled_jobs
SET last_run_at = $2,
    next_run_at = $3,
    last_status = $4,
    last_error  = $5,
    updated_at  = NOW()
WHERE name = $1;

-- name: SetScheduledJobEnabled :exec
UPDATE scheduled_jobs
SET enabled = $2, updated_at = NOW()
WHERE name = $1;
