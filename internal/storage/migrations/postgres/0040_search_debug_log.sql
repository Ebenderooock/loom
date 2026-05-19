-- +goose Up
CREATE TABLE IF NOT EXISTS search_debug_log (
    id              TEXT PRIMARY KEY,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    media_type      TEXT NOT NULL DEFAULT '',
    media_id        TEXT NOT NULL DEFAULT '',
    title           TEXT NOT NULL DEFAULT '',
    year            INTEGER NOT NULL DEFAULT 0,
    season          INTEGER NOT NULL DEFAULT 0,
    episode         INTEGER NOT NULL DEFAULT 0,
    imdb_id         TEXT NOT NULL DEFAULT '',
    tvdb_id         TEXT NOT NULL DEFAULT '',
    tmdb_id         TEXT NOT NULL DEFAULT '',
    quality_profile_id TEXT NOT NULL DEFAULT '',
    request_json    TEXT NOT NULL DEFAULT '{}',
    tiers_json      TEXT NOT NULL DEFAULT '[]',
    indexer_results_json TEXT NOT NULL DEFAULT '[]',
    evaluation_json TEXT NOT NULL DEFAULT '[]',
    total_results   INTEGER NOT NULL DEFAULT 0,
    total_rejected  INTEGER NOT NULL DEFAULT 0,
    grabbed_title   TEXT NOT NULL DEFAULT '',
    outcome         TEXT NOT NULL DEFAULT '',
    duration_ms     INTEGER NOT NULL DEFAULT 0,
    error_message   TEXT NOT NULL DEFAULT ''
);

CREATE INDEX idx_search_debug_log_created ON search_debug_log(created_at);
CREATE INDEX idx_search_debug_log_media ON search_debug_log(media_type, media_id);
CREATE INDEX idx_search_debug_log_outcome ON search_debug_log(outcome);

-- +goose Down
DROP TABLE IF EXISTS search_debug_log;
