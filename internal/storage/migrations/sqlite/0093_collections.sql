-- +goose Up
CREATE TABLE collections (
    id TEXT PRIMARY KEY,
    body TEXT NOT NULL,
    revision INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE collection_sources (
    provider TEXT NOT NULL,
    kind TEXT NOT NULL,
    external_id TEXT NOT NULL,
    collection_id TEXT NOT NULL REFERENCES collections(id) ON DELETE CASCADE,
    PRIMARY KEY (provider, kind, external_id)
);
CREATE TABLE collection_redirects (
    id TEXT PRIMARY KEY,
    collection_id TEXT NOT NULL REFERENCES collections(id) ON DELETE CASCADE
);

-- +goose Down
DROP TABLE collection_redirects;
DROP TABLE collection_sources;
DROP TABLE collections;
