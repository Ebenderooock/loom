-- +goose Up
CREATE TABLE collection_scan (
    media_type TEXT NOT NULL,
    media_id TEXT NOT NULL,
    identity TEXT NOT NULL,
    position INTEGER NOT NULL,
    checked_at BIGINT NOT NULL DEFAULT 0,
    next_at BIGINT NOT NULL DEFAULT 0,
    error TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (media_type, media_id)
);
CREATE TABLE collection_scan_lock (
    id INTEGER PRIMARY KEY,
    lease_until BIGINT NOT NULL DEFAULT 0
);
INSERT INTO collection_scan_lock (id) VALUES (1);

-- +goose Down
DROP TABLE collection_scan;
DROP TABLE collection_scan_lock;
