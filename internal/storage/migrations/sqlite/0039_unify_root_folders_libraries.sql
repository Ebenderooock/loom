-- +goose Up
ALTER TABLE movies RENAME COLUMN root_folder_id TO library_id;
ALTER TABLE series RENAME COLUMN root_folder_id TO library_id;
INSERT OR IGNORE INTO libraries (id, name, path, media_type, created_at, updated_at)
  SELECT id, path, path, 'movie', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP FROM root_folders WHERE deleted_at IS NULL;

-- +goose Down
ALTER TABLE movies RENAME COLUMN library_id TO root_folder_id;
ALTER TABLE series RENAME COLUMN library_id TO root_folder_id;
