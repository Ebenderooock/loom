-- +goose Up

-- cleanup_orphans tracks files/folders found in download save folders that are
-- no longer associated with any active download or in-progress import. They are
-- surfaced for review and, when auto-delete is enabled, removed after a
-- configurable retention period. Status transitions:
--   pending       -> discovered, awaiting review or retention
--   ignored       -> user kept it; never auto-deleted, never re-flagged
--   deleted       -> removed (recycled) from disk
--   delete_failed -> removal attempt errored; error captured for the user
CREATE TABLE IF NOT EXISTS cleanup_orphans (
    id            TEXT PRIMARY KEY,
    path          TEXT NOT NULL UNIQUE,
    client_id     TEXT NOT NULL DEFAULT '',
    root          TEXT NOT NULL DEFAULT '',
    size_bytes    INTEGER NOT NULL DEFAULT 0,
    status        TEXT NOT NULL DEFAULT 'pending',
    error         TEXT NOT NULL DEFAULT '',
    first_seen_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_seen_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at    TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_cleanup_orphans_status ON cleanup_orphans(status);

-- cleanup_settings is a single-row table holding the global cleanup config.
CREATE TABLE IF NOT EXISTS cleanup_settings (
    id                  INTEGER PRIMARY KEY CHECK (id = 1),
    auto_delete_enabled INTEGER NOT NULL DEFAULT 1,
    retention_days      INTEGER NOT NULL DEFAULT 7,
    updated_at          TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO cleanup_settings (id, auto_delete_enabled, retention_days)
VALUES (1, 1, 7)
ON CONFLICT(id) DO NOTHING;

-- +goose Down

DROP TABLE IF EXISTS cleanup_orphans;
DROP TABLE IF EXISTS cleanup_settings;
