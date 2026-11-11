-- +goose Up
-- Historical matches are a metadata-only preview, never proof for source deletion.
ALTER TABLE import_history ADD COLUMN dest_size INTEGER NOT NULL DEFAULT 0;
ALTER TABLE import_history ADD COLUMN dest_mtime INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE import_history DROP COLUMN dest_mtime;
ALTER TABLE import_history DROP COLUMN dest_size;
