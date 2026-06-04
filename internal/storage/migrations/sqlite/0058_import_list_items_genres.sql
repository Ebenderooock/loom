-- +goose Up

-- Cache genres on discover items so the Discover grid can filter by genre
-- without re-querying TMDB. Stored as a pipe-separated list of genre names.
ALTER TABLE import_list_items ADD COLUMN genres TEXT NOT NULL DEFAULT '';

-- +goose Down

-- SQLite does not support DROP COLUMN in older versions; best-effort rollback.
