-- +goose Up
CREATE TABLE scheduled_jobs (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    name         TEXT    NOT NULL UNIQUE,
    schedule     TEXT    NOT NULL,
    last_run_at  DATETIME,
    next_run_at  DATETIME,
    paused       INTEGER NOT NULL DEFAULT 0,
    payload      TEXT    NOT NULL DEFAULT '{}',
    created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_scheduled_jobs_next_run ON scheduled_jobs(next_run_at) WHERE paused = 0;

-- +goose Down
DROP INDEX IF EXISTS idx_scheduled_jobs_next_run;
DROP TABLE scheduled_jobs;
