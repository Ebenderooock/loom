-- +goose Up

ALTER TABLE import_list_items ADD COLUMN media_type TEXT DEFAULT '';

-- +goose Down

-- SQLite does not support DROP COLUMN in older versions; this is a best-effort rollback.
