-- +goose Up

CREATE TABLE IF NOT EXISTS manual_review (
    id TEXT PRIMARY KEY,
    media_type TEXT NOT NULL DEFAULT '',
    media_id TEXT NOT NULL DEFAULT '',
    download_path TEXT NOT NULL DEFAULT '',
    reason TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'pending',
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
    resolved_at TEXT
);

CREATE INDEX idx_manual_review_status ON manual_review(status);

-- +goose Down

DROP INDEX IF EXISTS idx_manual_review_status;
DROP TABLE IF EXISTS manual_review;
