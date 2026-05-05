-- +goose Up
CREATE TABLE IF NOT EXISTS indexer_rules (
    id TEXT PRIMARY KEY,
    indexer_id TEXT NOT NULL,
    media_type TEXT,
    category_filter TEXT NOT NULL DEFAULT '[]',
    tag_filter TEXT NOT NULL DEFAULT '[]',
    priority INTEGER NOT NULL DEFAULT 0,
    enabled INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
);

CREATE INDEX IF NOT EXISTS idx_indexer_rules_indexer ON indexer_rules(indexer_id);
CREATE INDEX IF NOT EXISTS idx_indexer_rules_media ON indexer_rules(media_type);

CREATE TABLE IF NOT EXISTS replacement_history (
    id TEXT PRIMARY KEY,
    media_type TEXT NOT NULL,
    media_id TEXT NOT NULL,
    old_path TEXT NOT NULL,
    new_path TEXT NOT NULL,
    old_quality TEXT NOT NULL DEFAULT '',
    new_quality TEXT NOT NULL DEFAULT '',
    old_size INTEGER NOT NULL DEFAULT 0,
    new_size INTEGER NOT NULL DEFAULT 0,
    old_score INTEGER NOT NULL DEFAULT 0,
    new_score INTEGER NOT NULL DEFAULT 0,
    reason TEXT NOT NULL DEFAULT '',
    replaced_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
);

CREATE INDEX IF NOT EXISTS idx_replacement_history_media ON replacement_history(media_type, media_id);

-- +goose Down
DROP TABLE IF EXISTS replacement_history;
DROP INDEX IF EXISTS idx_indexer_rules_media;
DROP INDEX IF EXISTS idx_indexer_rules_indexer;
DROP TABLE IF EXISTS indexer_rules;
