-- +goose Up
-- Phase 3a: download-clients core. See the SQLite migration for the
-- design notes that apply to both engines.
CREATE TABLE download_clients (
    id                TEXT        PRIMARY KEY,
    name              TEXT        NOT NULL,
    kind              TEXT        NOT NULL,
    protocol          TEXT        NOT NULL,
    enabled           BOOLEAN     NOT NULL DEFAULT TRUE,
    priority          INTEGER     NOT NULL DEFAULT 25,
    host              TEXT        NOT NULL DEFAULT '',
    port              INTEGER     NOT NULL DEFAULT 0,
    tls               BOOLEAN     NOT NULL DEFAULT FALSE,
    username          TEXT        NOT NULL DEFAULT '',
    password          TEXT        NOT NULL DEFAULT '',
    config_json       JSONB       NOT NULL DEFAULT '{}'::jsonb,
    category_default  TEXT        NOT NULL DEFAULT '',
    save_path_default TEXT        NOT NULL DEFAULT '',
    remove_completed  BOOLEAN     NOT NULL DEFAULT FALSE,
    remove_failed     BOOLEAN     NOT NULL DEFAULT FALSE,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_download_clients_kind    ON download_clients(kind);
CREATE INDEX idx_download_clients_enabled ON download_clients(enabled);

CREATE TABLE download_client_health (
    client_id             TEXT        PRIMARY KEY REFERENCES download_clients(id) ON DELETE CASCADE,
    status                TEXT        NOT NULL DEFAULT 'unknown',
    last_checked_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_success_at       TIMESTAMPTZ,
    last_failure_at       TIMESTAMPTZ,
    last_error            TEXT        NOT NULL DEFAULT '',
    consecutive_failures  INTEGER     NOT NULL DEFAULT 0,
    last_free_space_bytes BIGINT,
    last_categories_json  JSONB       NOT NULL DEFAULT '[]'::jsonb
);

-- +goose Down
DROP TABLE download_client_health;
DROP INDEX IF EXISTS idx_download_clients_enabled;
DROP INDEX IF EXISTS idx_download_clients_kind;
DROP TABLE download_clients;
