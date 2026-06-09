-- +goose Up
-- Add a music (artist) request limit to the global per-user quota. 0 = unlimited.
ALTER TABLE request_quota_config ADD COLUMN music_limit INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE request_quota_config DROP COLUMN music_limit;
