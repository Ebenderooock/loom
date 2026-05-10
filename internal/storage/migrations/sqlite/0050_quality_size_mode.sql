-- +goose Up
ALTER TABLE quality_definitions ADD COLUMN size_mode TEXT NOT NULL DEFAULT 'per_minute';

-- +goose Down
-- SQLite does not support DROP COLUMN; recreating would be destructive.
-- The column is harmless when unused.
