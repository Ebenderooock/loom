-- +goose Up
CREATE TABLE IF NOT EXISTS media_preferences (
    id TEXT PRIMARY KEY DEFAULT 'default',
    preferred_audio TEXT NOT NULL DEFAULT '[]',
    preferred_sub_languages TEXT NOT NULL DEFAULT '[]',
    require_subtitles INTEGER NOT NULL DEFAULT 0,
    prefer_hdr INTEGER NOT NULL DEFAULT 1,
    prefer_atmos INTEGER NOT NULL DEFAULT 1,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- +goose Down
DROP TABLE IF EXISTS media_preferences;
