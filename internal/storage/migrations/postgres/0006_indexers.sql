-- +goose Up
CREATE TABLE indexers (
    id              TEXT    PRIMARY KEY,
    kind            TEXT    NOT NULL,
    name            TEXT    NOT NULL,
    enabled         BOOLEAN NOT NULL DEFAULT TRUE,
    priority        INTEGER NOT NULL DEFAULT 25,
    config_json     JSONB   NOT NULL DEFAULT '{}'::jsonb,
    categories_json JSONB   NOT NULL DEFAULT '[]'::jsonb,
    tags_json       JSONB   NOT NULL DEFAULT '[]'::jsonb,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_indexers_enabled ON indexers(enabled);

CREATE TABLE indexer_health (
    indexer_id      TEXT    PRIMARY KEY REFERENCES indexers(id) ON DELETE CASCADE,
    status          TEXT    NOT NULL DEFAULT 'unknown',
    last_checked_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_success_at TIMESTAMPTZ,
    latency_ms      INTEGER,
    last_error      TEXT    NOT NULL DEFAULT ''
);

-- +goose Down
DROP TABLE indexer_health;
DROP INDEX IF EXISTS idx_indexers_enabled;
DROP TABLE indexers;
