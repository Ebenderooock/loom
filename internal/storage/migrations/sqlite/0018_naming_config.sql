-- +goose Up
CREATE TABLE IF NOT EXISTS naming_config (
    id                    TEXT PRIMARY KEY DEFAULT 'default',
    movie_folder_format   TEXT NOT NULL DEFAULT '{Movie Title} ({Release Year})',
    movie_file_format     TEXT NOT NULL DEFAULT '{Movie Title} ({Release Year}) [{Quality Full}]',
    colon_replacement     TEXT NOT NULL DEFAULT ' -',
    rename_movies         BOOLEAN NOT NULL DEFAULT 1,
    created_at            DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at            DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Seed default row
INSERT OR IGNORE INTO naming_config (id) VALUES ('default');

-- +goose Down
DROP TABLE IF EXISTS naming_config;
