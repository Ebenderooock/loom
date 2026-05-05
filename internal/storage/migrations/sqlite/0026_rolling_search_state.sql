-- +goose Up
CREATE TABLE IF NOT EXISTS search_state (
    media_type     TEXT NOT NULL,
    media_id       TEXT NOT NULL,
    last_searched_at TEXT NOT NULL,
    search_count   INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (media_type, media_id)
);

-- +goose Down
DROP TABLE IF EXISTS search_state;
