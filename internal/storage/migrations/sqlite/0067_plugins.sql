-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS plugins (
    id            TEXT PRIMARY KEY,
    name          TEXT NOT NULL,
    enabled       INTEGER NOT NULL DEFAULT 0,
    command       TEXT NOT NULL DEFAULT '[]',
    events        TEXT NOT NULL DEFAULT '[]',
    env           TEXT NOT NULL DEFAULT '{}',
    timeout_secs  INTEGER NOT NULL DEFAULT 30,
    working_dir   TEXT NOT NULL DEFAULT '',
    created_at    TEXT NOT NULL,
    updated_at    TEXT NOT NULL
);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS plugin_runs (
    id           TEXT PRIMARY KEY,
    plugin_id    TEXT NOT NULL,
    plugin_name  TEXT NOT NULL DEFAULT '',
    topic        TEXT NOT NULL DEFAULT '',
    success      INTEGER NOT NULL DEFAULT 0,
    exit_code    INTEGER NOT NULL DEFAULT 0,
    duration_ms  INTEGER NOT NULL DEFAULT 0,
    stdout       TEXT NOT NULL DEFAULT '',
    stderr       TEXT NOT NULL DEFAULT '',
    error_msg    TEXT NOT NULL DEFAULT '',
    started_at   TEXT NOT NULL
);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX IF NOT EXISTS idx_plugin_runs_plugin ON plugin_runs (plugin_id, started_at DESC);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX IF NOT EXISTS idx_plugin_runs_started ON plugin_runs (started_at DESC);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS plugin_runs;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS plugins;
-- +goose StatementEnd
