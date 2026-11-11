-- +goose Up
-- Durable manual import queue with per-file outcomes, and source identity on
-- import history so a verified prior import can be recognised per file.
ALTER TABLE import_history ADD COLUMN source_size INTEGER NOT NULL DEFAULT 0;
ALTER TABLE import_history ADD COLUMN source_mtime INTEGER NOT NULL DEFAULT 0; -- unix nanoseconds

CREATE INDEX IF NOT EXISTS idx_import_history_source ON import_history(source_path, status);

CREATE TABLE IF NOT EXISTS import_jobs (
    id              TEXT PRIMARY KEY,
    kind            TEXT NOT NULL,              -- manual, manual_match, reimport
    source_path     TEXT NOT NULL,
    media_type      TEXT NOT NULL DEFAULT '',
    media_id        TEXT NOT NULL DEFAULT '',
    conflict_policy TEXT NOT NULL DEFAULT '',
    workflow_id     TEXT NOT NULL DEFAULT '',
    review_id       TEXT NOT NULL DEFAULT '',  -- manual_review entry blocking this job, if any
    status          TEXT NOT NULL,              -- queued, importing, retry_pending, imported, failed, pending_review
    attempts        INTEGER NOT NULL DEFAULT 0,
    max_attempts    INTEGER NOT NULL DEFAULT 5,
    error           TEXT NOT NULL DEFAULT '',
    next_retry_at   DATETIME,
    created_at      DATETIME NOT NULL,
    updated_at      DATETIME NOT NULL,
    completed_at    DATETIME
);

CREATE INDEX IF NOT EXISTS idx_import_jobs_status ON import_jobs(status, next_retry_at);
CREATE INDEX IF NOT EXISTS idx_import_jobs_created ON import_jobs(created_at);

CREATE TABLE IF NOT EXISTS import_job_files (
    id          TEXT PRIMARY KEY,
    job_id      TEXT NOT NULL REFERENCES import_jobs(id) ON DELETE CASCADE,
    source_path TEXT NOT NULL,
    dest_path   TEXT NOT NULL DEFAULT '',
    media_type  TEXT NOT NULL DEFAULT '',
    media_id    TEXT NOT NULL DEFAULT '',
    status      TEXT NOT NULL,                  -- queued, importing, imported, failed, skipped, pending_review
    attempts    INTEGER NOT NULL DEFAULT 0,
    retryable   INTEGER NOT NULL DEFAULT 1,
    error       TEXT NOT NULL DEFAULT '',
    updated_at  DATETIME NOT NULL,
    UNIQUE (job_id, source_path)
);

CREATE INDEX IF NOT EXISTS idx_import_job_files_source ON import_job_files(source_path);

-- +goose Down
DROP TABLE IF EXISTS import_job_files;
DROP TABLE IF EXISTS import_jobs;
DROP INDEX IF EXISTS idx_import_history_source;
ALTER TABLE import_history DROP COLUMN source_mtime;
ALTER TABLE import_history DROP COLUMN source_size;
