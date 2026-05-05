-- +goose Up

CREATE TABLE IF NOT EXISTS language_profiles (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    languages TEXT NOT NULL DEFAULT '[]',
    cutoff_language TEXT NOT NULL DEFAULT '',
    upgrade_allowed BOOLEAN NOT NULL DEFAULT true,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
);

CREATE INDEX idx_lp_name ON language_profiles(name);

-- +goose Down

DROP TABLE IF EXISTS language_profiles;
