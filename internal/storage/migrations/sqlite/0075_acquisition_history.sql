-- +goose Up
ALTER TABLE download_history ADD COLUMN acquisition_key TEXT NOT NULL DEFAULT '';
ALTER TABLE download_history ADD COLUMN media_type TEXT NOT NULL DEFAULT '';
ALTER TABLE download_history ADD COLUMN media_id TEXT NOT NULL DEFAULT '';
ALTER TABLE download_history ADD COLUMN indexer_id TEXT NOT NULL DEFAULT '';
ALTER TABLE download_history ADD COLUMN release_hash TEXT NOT NULL DEFAULT '';
ALTER TABLE download_history ADD COLUMN stage TEXT NOT NULL DEFAULT '';
ALTER TABLE download_history ADD COLUMN reason TEXT NOT NULL DEFAULT '';
ALTER TABLE download_history ADD COLUMN attempt INTEGER NOT NULL DEFAULT 0;
ALTER TABLE download_history ADD COLUMN retry_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE download_history ADD COLUMN retry_eligible INTEGER NOT NULL DEFAULT 0;
ALTER TABLE download_history ADD COLUMN next_retry_at TEXT NOT NULL DEFAULT '';
CREATE UNIQUE INDEX idx_acquisition_attempt ON download_history(acquisition_key, attempt)
  WHERE acquisition_key <> '';

-- +goose Down
DROP INDEX idx_acquisition_attempt;
ALTER TABLE download_history DROP COLUMN next_retry_at;
ALTER TABLE download_history DROP COLUMN retry_eligible;
ALTER TABLE download_history DROP COLUMN retry_count;
ALTER TABLE download_history DROP COLUMN attempt;
ALTER TABLE download_history DROP COLUMN reason;
ALTER TABLE download_history DROP COLUMN stage;
ALTER TABLE download_history DROP COLUMN release_hash;
ALTER TABLE download_history DROP COLUMN indexer_id;
ALTER TABLE download_history DROP COLUMN media_id;
ALTER TABLE download_history DROP COLUMN media_type;
ALTER TABLE download_history DROP COLUMN acquisition_key;
