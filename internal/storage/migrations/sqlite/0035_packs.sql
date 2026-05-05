-- +goose Up

CREATE TABLE IF NOT EXISTS season_pack_history (
    id TEXT PRIMARY KEY,
    series_id TEXT NOT NULL,
    season INTEGER NOT NULL,
    pack_title TEXT NOT NULL,
    episodes_included TEXT NOT NULL DEFAULT '[]',
    quality TEXT,
    grabbed_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_pack_history_series ON season_pack_history(series_id);

-- +goose Down

DROP TABLE IF EXISTS season_pack_history;
