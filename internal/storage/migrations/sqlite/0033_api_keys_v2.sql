-- +goose Up
CREATE TABLE IF NOT EXISTS api_keys_v2 (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    key TEXT NOT NULL UNIQUE,
    scopes TEXT NOT NULL DEFAULT '*',
    expires_at DATETIME,
    last_used DATETIME,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS command_queue (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    body TEXT DEFAULT '{}',
    status TEXT NOT NULL DEFAULT 'queued',
    priority INTEGER NOT NULL DEFAULT 0,
    started_at DATETIME,
    completed_at DATETIME,
    result TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- +goose Down
DROP TABLE IF EXISTS command_queue;
DROP TABLE IF EXISTS api_keys_v2;
