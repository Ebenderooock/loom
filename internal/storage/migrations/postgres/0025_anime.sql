-- +goose Up

CREATE TABLE IF NOT EXISTS anime_preferences (
    series_id           TEXT PRIMARY KEY,
    numbering_scheme    TEXT NOT NULL DEFAULT 'absolute',
    preferred_groups    TEXT NOT NULL DEFAULT '[]',
    dual_audio_required BOOLEAN NOT NULL DEFAULT FALSE,
    release_group_scoring TEXT NOT NULL DEFAULT '{}'
);

CREATE TABLE IF NOT EXISTS anime_mappings (
    series_id       TEXT NOT NULL,
    absolute_number INTEGER NOT NULL,
    season_number   INTEGER NOT NULL,
    episode_number  INTEGER NOT NULL,
    PRIMARY KEY (series_id, absolute_number)
);

CREATE INDEX IF NOT EXISTS idx_anime_mappings_series ON anime_mappings(series_id);

-- +goose Down

DROP TABLE IF EXISTS anime_mappings;
DROP TABLE IF EXISTS anime_preferences;
