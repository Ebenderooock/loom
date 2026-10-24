-- +goose Up
ALTER TABLE media_requests ADD COLUMN fulfilled_at TEXT;
ALTER TABLE media_requests ADD COLUMN availability TEXT NOT NULL DEFAULT 'unknown';
ALTER TABLE media_requests ADD COLUMN availability_reason TEXT NOT NULL DEFAULT 'Not checked';
ALTER TABLE media_requests ADD COLUMN availability_checked_at TEXT;
ALTER TABLE media_requests ADD COLUMN availability_library_id TEXT NOT NULL DEFAULT '';
ALTER TABLE media_requests ADD COLUMN re_request_of TEXT NOT NULL DEFAULT '';
ALTER TABLE media_requests ADD COLUMN availability_files TEXT NOT NULL DEFAULT '{}';
UPDATE media_requests SET fulfilled_at=updated_at::text WHERE status='available';

DROP INDEX idx_media_requests_open;
CREATE UNIQUE INDEX idx_media_requests_open ON media_requests(media_type,tmdb_id)
    WHERE status IN ('pending','approving','approved');

CREATE TABLE request_storage_anchors (
    library_id TEXT PRIMARY KEY,
    root_path TEXT NOT NULL,
    token TEXT NOT NULL
);
CREATE TABLE request_reconciliations (
    id TEXT PRIMARY KEY,
    actor TEXT NOT NULL,
    dry_run BOOLEAN NOT NULL,
    created_at TEXT NOT NULL,
    outcomes TEXT NOT NULL
);

-- +goose Down
DROP TABLE request_reconciliations;
DROP TABLE request_storage_anchors;
DROP INDEX idx_media_requests_open;
CREATE UNIQUE INDEX idx_media_requests_open ON media_requests(media_type,tmdb_id)
    WHERE status IN ('pending','approving','approved');
ALTER TABLE media_requests DROP COLUMN re_request_of;
ALTER TABLE media_requests DROP COLUMN availability_files;
ALTER TABLE media_requests DROP COLUMN availability_library_id;
ALTER TABLE media_requests DROP COLUMN availability_checked_at;
ALTER TABLE media_requests DROP COLUMN availability_reason;
ALTER TABLE media_requests DROP COLUMN availability;
ALTER TABLE media_requests DROP COLUMN fulfilled_at;
