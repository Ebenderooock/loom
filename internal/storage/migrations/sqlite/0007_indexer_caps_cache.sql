-- +goose Up
-- Phase 2c: cache the most recent caps document for each indexer so a
-- restart doesn't blank-state every kind that does network discovery
-- (Newznab/Torznab `t=caps`). The column is nullable so kinds that
-- don't fetch caps (e.g. builtin/null) leave it untouched.
ALTER TABLE indexer_health ADD COLUMN last_caps_json TEXT;

-- +goose Down
-- SQLite cannot DROP COLUMN before 3.35; rebuild the table without it.
CREATE TABLE indexer_health_new (
    indexer_id      TEXT    PRIMARY KEY REFERENCES indexers(id) ON DELETE CASCADE,
    status          TEXT    NOT NULL DEFAULT 'unknown',
    last_checked_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_success_at DATETIME,
    latency_ms      INTEGER,
    last_error      TEXT    NOT NULL DEFAULT ''
);
INSERT INTO indexer_health_new
SELECT indexer_id, status, last_checked_at, last_success_at, latency_ms, last_error
FROM indexer_health;
DROP TABLE indexer_health;
ALTER TABLE indexer_health_new RENAME TO indexer_health;
