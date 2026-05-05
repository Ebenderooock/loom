-- +goose Up
CREATE TABLE IF NOT EXISTS quality_profiles_v2 (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    cutoff TEXT NOT NULL DEFAULT '1080p',
    min_format_score INTEGER NOT NULL DEFAULT 0,
    cutoff_format_score INTEGER NOT NULL DEFAULT 0,
    upgrade_allowed INTEGER NOT NULL DEFAULT 1,
    items TEXT NOT NULL DEFAULT '[]',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS quality_profile_format_items (
    profile_id TEXT NOT NULL REFERENCES quality_profiles_v2(id) ON DELETE CASCADE,
    format_id TEXT NOT NULL,
    score INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (profile_id, format_id)
);

-- +goose Down
DROP TABLE IF EXISTS quality_profile_format_items;
DROP TABLE IF EXISTS quality_profiles_v2;
