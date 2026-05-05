-- +goose Up
-- Add unique constraint on movie_files.file_path to support idempotent imports.
CREATE UNIQUE INDEX IF NOT EXISTS idx_movie_files_file_path_unique ON movie_files(file_path);

-- +goose Down
DROP INDEX IF EXISTS idx_movie_files_file_path_unique;
