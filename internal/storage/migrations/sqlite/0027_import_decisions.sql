-- 0027_import_decisions: Track every import decision for auditability.
CREATE TABLE IF NOT EXISTS import_decisions (
    id              TEXT PRIMARY KEY,
    timestamp       TEXT NOT NULL DEFAULT '',
    source_path     TEXT NOT NULL DEFAULT '',
    dest_path       TEXT NOT NULL DEFAULT '',
    media_type      TEXT NOT NULL DEFAULT '',
    media_id        TEXT NOT NULL DEFAULT '',
    action          TEXT NOT NULL DEFAULT '',
    reason          TEXT NOT NULL DEFAULT '',
    conflict_policy TEXT NOT NULL DEFAULT '',
    file_size       INTEGER NOT NULL DEFAULT 0,
    file_quality    TEXT NOT NULL DEFAULT '',
    created_at      TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX idx_import_decisions_media ON import_decisions(media_id);
CREATE INDEX idx_import_decisions_created ON import_decisions(created_at);
