-- +goose Up
-- Adds run-status columns for the persistent scheduler. The original
-- 0004 migration carried `paused`; we keep it for back-compat and
-- introduce `enabled` as the positive form the Go API uses, defaulted
-- from the inverse of `paused`.
ALTER TABLE scheduled_jobs ADD COLUMN enabled     INTEGER NOT NULL DEFAULT 1;
ALTER TABLE scheduled_jobs ADD COLUMN last_status TEXT    NOT NULL DEFAULT '';
ALTER TABLE scheduled_jobs ADD COLUMN last_error  TEXT    NOT NULL DEFAULT '';

UPDATE scheduled_jobs SET enabled = CASE WHEN paused = 0 THEN 1 ELSE 0 END;

CREATE INDEX IF NOT EXISTS idx_scheduled_jobs_enabled_next
    ON scheduled_jobs(enabled, next_run_at);

-- +goose Down
DROP INDEX IF EXISTS idx_scheduled_jobs_enabled_next;
ALTER TABLE scheduled_jobs DROP COLUMN last_error;
ALTER TABLE scheduled_jobs DROP COLUMN last_status;
ALTER TABLE scheduled_jobs DROP COLUMN enabled;
