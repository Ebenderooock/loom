-- +goose Up

CREATE TABLE IF NOT EXISTS notification_connections (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    type TEXT NOT NULL,
    enabled BOOLEAN DEFAULT true,
    settings TEXT DEFAULT '{}',
    on_grab BOOLEAN DEFAULT false,
    on_download BOOLEAN DEFAULT false,
    on_upgrade BOOLEAN DEFAULT false,
    on_rename BOOLEAN DEFAULT false,
    on_delete BOOLEAN DEFAULT false,
    on_health_issue BOOLEAN DEFAULT false,
    on_application_update BOOLEAN DEFAULT false,
    tags TEXT DEFAULT '[]',
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
);

CREATE TABLE IF NOT EXISTS notification_history (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    connection_id TEXT REFERENCES notification_connections(id) ON DELETE SET NULL,
    event_type TEXT NOT NULL,
    title TEXT NOT NULL,
    message TEXT DEFAULT '',
    success BOOLEAN DEFAULT true,
    error_message TEXT DEFAULT '',
    sent_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
);

-- +goose Down

DROP TABLE IF EXISTS notification_history;
DROP TABLE IF EXISTS notification_connections;
