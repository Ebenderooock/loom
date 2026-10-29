-- +goose Up
-- Retain the server-resolved folder after the last episode file is removed,
-- so remaining extras in custom-named show folders can still be managed.
CREATE TABLE title_file_folders (
    media_type TEXT NOT NULL,
    title_id TEXT NOT NULL,
    library_id TEXT NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
    folder TEXT NOT NULL,
    PRIMARY KEY (media_type, title_id)
);

-- +goose Down
DROP TABLE title_file_folders;
