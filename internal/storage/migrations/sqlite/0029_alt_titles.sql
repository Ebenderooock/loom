-- +goose Up

CREATE TABLE IF NOT EXISTS alternate_titles (
    id TEXT PRIMARY KEY,
    media_id TEXT NOT NULL,
    media_type TEXT NOT NULL,
    title TEXT NOT NULL,
    language TEXT DEFAULT 'en',
    source TEXT DEFAULT 'manual',
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
    UNIQUE(media_id, title)
);

CREATE INDEX idx_alt_titles_media ON alternate_titles(media_id, media_type);

-- +goose Down

DROP TABLE IF EXISTS alternate_titles;
