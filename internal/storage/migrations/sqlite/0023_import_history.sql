-- +goose Up
-- 0023_import_history: Track file imports from download clients into the media library.
CREATE TABLE IF NOT EXISTS import_history (
    id          TEXT PRIMARY KEY,
    media_type  TEXT NOT NULL DEFAULT '',   -- "movie" or "episode"
    media_id    TEXT NOT NULL DEFAULT '',   -- slug ID of the matched media item
    source_path TEXT NOT NULL,
    dest_path   TEXT NOT NULL DEFAULT '',
    import_mode TEXT NOT NULL DEFAULT 'move',
    status      TEXT NOT NULL DEFAULT 'pending', -- pending, imported, failed, pending_review
    error       TEXT NOT NULL DEFAULT '',
    imported_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX idx_import_history_status ON import_history(status);
CREATE INDEX idx_import_history_media ON import_history(media_type, media_id);
