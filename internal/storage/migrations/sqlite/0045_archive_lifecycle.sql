-- +goose Up

-- Add archive/lifecycle settings columns to libraries.
ALTER TABLE libraries ADD COLUMN unmonitor_on_delete BOOLEAN NOT NULL DEFAULT 0;
ALTER TABLE libraries ADD COLUMN auto_archive_watched BOOLEAN NOT NULL DEFAULT 0;
ALTER TABLE libraries ADD COLUMN auto_archive_days_after_watch INTEGER NOT NULL DEFAULT 0;

-- +goose Down

-- SQLite does not support DROP COLUMN before 3.35; recreate table if needed.
-- For simplicity these columns are left in place on rollback.
