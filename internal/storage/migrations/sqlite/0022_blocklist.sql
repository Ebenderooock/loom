-- 0022_blocklist: Stalled/failed download blocklist
CREATE TABLE IF NOT EXISTS blocklist (
    id           TEXT PRIMARY KEY,
    title        TEXT NOT NULL,
    indexer_id   TEXT NOT NULL DEFAULT '',
    release_hash TEXT NOT NULL DEFAULT '',
    reason       TEXT NOT NULL DEFAULT '',
    created_at   DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX idx_blocklist_release_hash ON blocklist(release_hash);
