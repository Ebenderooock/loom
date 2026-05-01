-- +goose Up
ALTER TABLE scheduled_jobs ADD COLUMN enabled     BOOLEAN NOT NULL DEFAULT TRUE;
ALTER TABLE scheduled_jobs ADD COLUMN last_status TEXT    NOT NULL DEFAULT '';
ALTER TABLE scheduled_jobs ADD COLUMN last_error  TEXT    NOT NULL DEFAULT '';

UPDATE scheduled_jobs SET enabled = NOT paused;

CREATE INDEX IF NOT EXISTS idx_scheduled_jobs_enabled_next
    ON scheduled_jobs(enabled, next_run_at);

-- +goose Down
DROP INDEX IF EXISTS idx_scheduled_jobs_enabled_next;
ALTER TABLE scheduled_jobs DROP COLUMN last_error;
ALTER TABLE scheduled_jobs DROP COLUMN last_status;
ALTER TABLE scheduled_jobs DROP COLUMN enabled;
