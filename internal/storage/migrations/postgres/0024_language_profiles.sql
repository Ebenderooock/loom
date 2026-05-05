-- +goose Up

CREATE TABLE IF NOT EXISTS language_profiles (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    languages JSONB NOT NULL DEFAULT '[]',
    cutoff_language TEXT NOT NULL DEFAULT '',
    upgrade_allowed BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_lp_name ON language_profiles(name);

-- +goose Down

DROP TABLE IF EXISTS language_profiles;
