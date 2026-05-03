-- +goose Up
-- Phase 2e: per-indexer outbound proxies. Proxies are first-class
-- records (one row per upstream proxy / FlareSolverr endpoint) so
-- multiple indexers can share a configuration. `kind` selects the
-- transport builder (http, https, socks5, flaresolverr); `config_json`
-- carries the kind-specific opaque settings (URL, credentials,
-- session strategy, …).
CREATE TABLE proxies (
    id          TEXT     PRIMARY KEY,
    kind        TEXT     NOT NULL,
    name        TEXT     NOT NULL,
    enabled     INTEGER  NOT NULL DEFAULT 1,
    config_json TEXT     NOT NULL DEFAULT '{}',
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Attach a proxy to an indexer. Nullable: indexers without a
-- proxy_id keep using the default transport, which is the existing
-- Phase-2c behaviour.
ALTER TABLE indexers ADD COLUMN proxy_id TEXT REFERENCES proxies(id) ON DELETE SET NULL;
CREATE INDEX idx_indexers_proxy_id ON indexers(proxy_id);

-- +goose Down
DROP INDEX IF EXISTS idx_indexers_proxy_id;
-- SQLite cannot DROP COLUMN before 3.35; rebuild the table without it.
CREATE TABLE indexers_new (
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
INSERT INTO indexers_new
SELECT id, kind, name, enabled, priority, config_json, categories_json, tags_json, created_at, updated_at
FROM indexers;
DROP TABLE indexers;
ALTER TABLE indexers_new RENAME TO indexers;
CREATE INDEX idx_indexers_enabled ON indexers(enabled);
DROP TABLE proxies;
