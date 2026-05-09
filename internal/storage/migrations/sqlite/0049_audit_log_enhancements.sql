-- +goose Up
-- 0049_audit_log_enhancements: Add occurred_at, entity_type, and extra indexes
-- for centralized audit log.

ALTER TABLE audit_log ADD COLUMN occurred_at TEXT;
ALTER TABLE audit_log ADD COLUMN entity_type TEXT;

-- Composite indexes for the Events page filters and drill-down.
CREATE INDEX IF NOT EXISTS idx_audit_log_event_type ON audit_log(event_type);
CREATE INDEX IF NOT EXISTS idx_audit_log_entity ON audit_log(entity_id);
CREATE INDEX IF NOT EXISTS idx_audit_log_category_ts ON audit_log(category, timestamp);
CREATE INDEX IF NOT EXISTS idx_audit_log_level_ts ON audit_log(level, timestamp);

-- +goose Down
-- SQLite doesn't support DROP COLUMN in older versions, so this is best-effort.
DROP INDEX IF EXISTS idx_audit_log_level_ts;
DROP INDEX IF EXISTS idx_audit_log_category_ts;
DROP INDEX IF EXISTS idx_audit_log_entity;
DROP INDEX IF EXISTS idx_audit_log_event_type;
