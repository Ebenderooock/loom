-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS play_history (
    id                TEXT PRIMARY KEY,
    connection_id     TEXT NOT NULL,
    provider          TEXT NOT NULL,
    session_key       TEXT NOT NULL,
    media_id          TEXT NOT NULL DEFAULT '',
    user              TEXT NOT NULL DEFAULT '',
    media_type        TEXT NOT NULL DEFAULT '',
    title             TEXT NOT NULL DEFAULT '',
    grandparent_title TEXT NOT NULL DEFAULT '',
    full_title        TEXT NOT NULL DEFAULT '',
    device            TEXT NOT NULL DEFAULT '',
    transcode         INTEGER NOT NULL DEFAULT 0,
    started_at        TEXT NOT NULL,
    last_seen_at      TEXT NOT NULL,
    last_position_ms  INTEGER NOT NULL DEFAULT 0,
    duration_ms       INTEGER NOT NULL DEFAULT 0,
    watched_ms        INTEGER NOT NULL DEFAULT 0,
    ended_at          TEXT,
    end_reason        TEXT NOT NULL DEFAULT ''
);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX IF NOT EXISTS idx_play_history_ended ON play_history (ended_at);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX IF NOT EXISTS idx_play_history_user ON play_history (user);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX IF NOT EXISTS idx_play_history_started ON play_history (started_at);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE UNIQUE INDEX IF NOT EXISTS idx_play_history_open
    ON play_history (connection_id, session_key, media_id)
    WHERE ended_at IS NULL;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS play_history;
-- +goose StatementEnd
