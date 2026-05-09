-- +goose Up
-- 0047_audit_log: Unified audit log for system events.
CREATE TABLE IF NOT EXISTS audit_log (
    id TEXT PRIMARY KEY,
    timestamp TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    category TEXT NOT NULL,       -- e.g. 'indexer', 'download', 'import', 'system', 'auth'
    event_type TEXT NOT NULL,     -- e.g. 'indexer.created', 'indexer.search.completed'
    message TEXT NOT NULL,        -- Human-readable summary
    detail TEXT,                  -- Optional longer detail / JSON blob
    entity_id TEXT,               -- Optional FK to the entity involved (indexer ID, etc.)
    entity_name TEXT,             -- Denormalized name for display
    level TEXT NOT NULL DEFAULT 'info',  -- 'info', 'warn', 'error'
    source TEXT                   -- Who triggered it: 'user', 'system', 'scheduler'
);
CREATE INDEX IF NOT EXISTS idx_audit_log_timestamp ON audit_log(timestamp);
CREATE INDEX IF NOT EXISTS idx_audit_log_category ON audit_log(category);
CREATE INDEX IF NOT EXISTS idx_audit_log_level ON audit_log(level);
