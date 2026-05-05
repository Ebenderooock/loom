-- +goose Up

CREATE TABLE IF NOT EXISTS libraries (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    path TEXT NOT NULL UNIQUE,
    media_type TEXT NOT NULL DEFAULT 'movie',
    monitor_on_add INTEGER NOT NULL DEFAULT 1,
    quality_profile_id TEXT NOT NULL DEFAULT 'default',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS library_files (
    id TEXT PRIMARY KEY,
    library_id TEXT NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
    path TEXT NOT NULL UNIQUE,
    size_bytes INTEGER NOT NULL DEFAULT 0,
    media_id TEXT,
    last_scanned DATETIME,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_library_files_library ON library_files(library_id);
CREATE INDEX IF NOT EXISTS idx_library_files_media ON library_files(media_id);

-- +goose Down

DROP TABLE IF EXISTS library_files;
DROP TABLE IF EXISTS libraries;
