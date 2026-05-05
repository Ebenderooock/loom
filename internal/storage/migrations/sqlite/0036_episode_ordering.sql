-- +goose Up

CREATE TABLE IF NOT EXISTS episode_mappings (
    id TEXT PRIMARY KEY,
    series_id TEXT NOT NULL,
    ordering_type TEXT NOT NULL,
    season_from INTEGER,
    episode_from INTEGER,
    absolute_from INTEGER,
    season_to INTEGER,
    episode_to INTEGER,
    absolute_to INTEGER,
    source TEXT DEFAULT 'manual',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(series_id, ordering_type, season_from, episode_from)
);

CREATE INDEX IF NOT EXISTS idx_episode_mappings_series ON episode_mappings(series_id);

-- +goose Down

DROP TABLE IF EXISTS episode_mappings;
