-- +goose Up
-- Retain the newest registration for each episode/path, not distinct versions
-- or multiple episodes sharing one media file.
DELETE FROM episode_files
WHERE rowid IN (
    SELECT file_rowid FROM (
        SELECT rowid AS file_rowid,
               ROW_NUMBER() OVER (
                   PARTITION BY episode_id, file_path
                   ORDER BY updated_at DESC, created_at DESC, rowid DESC
               ) AS duplicate_rank
        FROM episode_files
    )
    WHERE duplicate_rank > 1
);

CREATE UNIQUE INDEX idx_episode_files_episode_path ON episode_files(episode_id, file_path);

-- +goose Down
DROP INDEX idx_episode_files_episode_path;
