-- +goose Up
CREATE TABLE proxies (
    id          TEXT        PRIMARY KEY,
    kind        TEXT        NOT NULL,
    name        TEXT        NOT NULL,
    enabled     BOOLEAN     NOT NULL DEFAULT TRUE,
    config_json JSONB       NOT NULL DEFAULT '{}'::jsonb,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE indexers ADD COLUMN proxy_id TEXT REFERENCES proxies(id) ON DELETE SET NULL;
CREATE INDEX idx_indexers_proxy_id ON indexers(proxy_id);

-- +goose Down
DROP INDEX IF EXISTS idx_indexers_proxy_id;
ALTER TABLE indexers DROP COLUMN IF EXISTS proxy_id;
DROP TABLE proxies;
