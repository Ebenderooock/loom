-- +goose Up

-- request_quota_config holds the global, per-user media-request quota. A single
-- row (id=1) stores the limits applied to each non-admin user within a rolling
-- window. A limit of 0 means unlimited (the default, preserving prior
-- behaviour). Admins are exempt from quotas.
CREATE TABLE IF NOT EXISTS request_quota_config (
    id           INTEGER PRIMARY KEY CHECK (id = 1),
    movie_limit  INTEGER NOT NULL DEFAULT 0,
    series_limit INTEGER NOT NULL DEFAULT 0,
    window_days  INTEGER NOT NULL DEFAULT 7,
    updated_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT OR IGNORE INTO request_quota_config (id, movie_limit, series_limit, window_days)
VALUES (1, 0, 0, 7);

-- Supports the per-user quota count (user_id + media_type + created_at), scoped
-- to the request statuses that consume a quota slot.
CREATE INDEX IF NOT EXISTS idx_media_requests_quota
    ON media_requests (user_id, media_type, created_at)
    WHERE status IN ('pending', 'approving', 'approved', 'available');

-- +goose Down

DROP INDEX IF EXISTS idx_media_requests_quota;
DROP TABLE IF EXISTS request_quota_config;
