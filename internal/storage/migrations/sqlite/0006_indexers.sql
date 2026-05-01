-- +goose Up
-- Phase 2a indexer core. `indexers` is one row per configured search
-- source; `config_json` carries the per-kind opaque settings (URL,
-- API key, Cardigann definition name, etc.) so we don't grow this
-- table when new kinds land. `indexer_health` is split out so health
-- updates from the scheduler don't churn the main row.
CREATE TABLE indexers (
    id              TEXT    PRIMARY KEY,
    kind            TEXT    NOT NULL,
    name            TEXT    NOT NULL,
    enabled         INTEGER NOT NULL DEFAULT 1,
    priority        INTEGER NOT NULL DEFAULT 25,
    config_json     TEXT    NOT NULL DEFAULT '{}',
    categories_json TEXT    NOT NULL DEFAULT '[]',
    tags_json       TEXT    NOT NULL DEFAULT '[]',
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_indexers_enabled ON indexers(enabled);

CREATE TABLE indexer_health (
    indexer_id      TEXT    PRIMARY KEY REFERENCES indexers(id) ON DELETE CASCADE,
    status          TEXT    NOT NULL DEFAULT 'unknown',
    last_checked_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_success_at DATETIME,
    latency_ms      INTEGER,
    last_error      TEXT    NOT NULL DEFAULT ''
);

-- +goose Down
DROP TABLE indexer_health;
DROP INDEX IF EXISTS idx_indexers_enabled;
DROP TABLE indexers;
