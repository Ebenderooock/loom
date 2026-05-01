-- +goose Up
CREATE TABLE scheduled_jobs (
    id           BIGSERIAL PRIMARY KEY,
    name         TEXT NOT NULL UNIQUE,
    schedule     TEXT NOT NULL,
    last_run_at  TIMESTAMPTZ,
    next_run_at  TIMESTAMPTZ,
    paused       BOOLEAN NOT NULL DEFAULT FALSE,
    payload      JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_scheduled_jobs_next_run ON scheduled_jobs(next_run_at) WHERE paused = FALSE;

-- +goose Down
DROP INDEX IF EXISTS idx_scheduled_jobs_next_run;
DROP TABLE scheduled_jobs;
