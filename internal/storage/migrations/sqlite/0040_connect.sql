-- +goose Up
CREATE TABLE IF NOT EXISTS connections (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    provider TEXT NOT NULL,
    enabled BOOLEAN DEFAULT true,
    settings TEXT DEFAULT '{}',
    notify_on_import BOOLEAN DEFAULT true,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
);

-- +goose Down
DROP TABLE IF EXISTS connections;
