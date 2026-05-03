-- +goose Up
-- Phase 2f: per-indexer rate limiting + retry policy. Each indexer
-- gets three optional dials so operators can be a polite client per
-- tracker / Usenet provider:
--
--   rate_limit_per_min   token-bucket fill rate (requests/minute)
--   rate_limit_burst     bucket capacity (instantaneous burst)
--   retry_max_attempts   how many times to retry on 429/503/transient
--
-- All three are nullable; NULL means "use the package default" so
-- existing rows keep their pre-2f behaviour when the column is added.
ALTER TABLE indexers ADD COLUMN rate_limit_per_min INTEGER;
ALTER TABLE indexers ADD COLUMN rate_limit_burst INTEGER;
ALTER TABLE indexers ADD COLUMN retry_max_attempts INTEGER;

-- +goose Down
-- SQLite cannot DROP COLUMN before 3.35; rebuild the table without
-- the rate-limit columns to mirror the 0008 down-migration style.
CREATE TABLE indexers_new (
    id              TEXT     PRIMARY KEY,
    kind            TEXT     NOT NULL,
    name            TEXT     NOT NULL,
    enabled         INTEGER  NOT NULL DEFAULT 1,
    priority        INTEGER  NOT NULL DEFAULT 25,
    config_json     TEXT     NOT NULL DEFAULT '{}',
    categories_json TEXT     NOT NULL DEFAULT '[]',
    tags_json       TEXT     NOT NULL DEFAULT '[]',
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    proxy_id        TEXT     REFERENCES proxies(id) ON DELETE SET NULL
);
INSERT INTO indexers_new
SELECT id, kind, name, enabled, priority, config_json, categories_json, tags_json, created_at, updated_at, proxy_id
FROM indexers;
DROP TABLE indexers;
ALTER TABLE indexers_new RENAME TO indexers;
CREATE INDEX idx_indexers_enabled ON indexers(enabled);
CREATE INDEX idx_indexers_proxy_id ON indexers(proxy_id);
