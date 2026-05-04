-- +goose Up
-- +goose StatementBegin

-- Movies table: core movie metadata and monitoring state
CREATE TABLE IF NOT EXISTS movies (
    id TEXT PRIMARY KEY,
    title TEXT NOT NULL,
    year INTEGER,
    imdb_id TEXT UNIQUE,
    tmdb_id TEXT UNIQUE,
    tvdb_id TEXT UNIQUE,
    overview TEXT,
    genres TEXT, -- JSON array
    runtime INTEGER,
    rating REAL,
    backdrop_path TEXT,
    poster_path TEXT,
    metadata_provider TEXT,
    last_search_at TIMESTAMP,
    monitoring_status TEXT NOT NULL DEFAULT 'monitored' CHECK(monitoring_status IN ('monitored', 'unmonitored', 'deleted')),
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP
);

-- Root folders: filesystem paths where movies are stored
CREATE TABLE IF NOT EXISTS root_folders (
    id TEXT PRIMARY KEY,
    path TEXT NOT NULL UNIQUE,
    free_space INTEGER DEFAULT 0,
    unmapped_count INTEGER DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP
);

-- Movie files: individual files on disk for movies
CREATE TABLE IF NOT EXISTS movie_files (
    id TEXT PRIMARY KEY,
    movie_id TEXT NOT NULL,
    file_path TEXT NOT NULL UNIQUE,
    size INTEGER,
    quality TEXT,
    format TEXT,
    media_info TEXT, -- JSON object
    file_date TIMESTAMP,
    date_added TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP,
    FOREIGN KEY (movie_id) REFERENCES movies(id) ON DELETE CASCADE
);

-- Indexes for common queries
CREATE INDEX IF NOT EXISTS idx_movies_tmdb_id ON movies(tmdb_id);
CREATE INDEX IF NOT EXISTS idx_movies_imdb_id ON movies(imdb_id);
CREATE INDEX IF NOT EXISTS idx_movies_monitoring_status ON movies(monitoring_status);
CREATE INDEX IF NOT EXISTS idx_movies_deleted_at ON movies(deleted_at);
CREATE INDEX IF NOT EXISTS idx_movie_files_movie_id ON movie_files(movie_id);
CREATE INDEX IF NOT EXISTS idx_movie_files_file_path ON movie_files(file_path);
CREATE INDEX IF NOT EXISTS idx_movie_files_deleted_at ON movie_files(deleted_at);
CREATE INDEX IF NOT EXISTS idx_root_folders_deleted_at ON root_folders(deleted_at);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP INDEX IF EXISTS idx_root_folders_deleted_at;
DROP INDEX IF EXISTS idx_movie_files_deleted_at;
DROP INDEX IF EXISTS idx_movie_files_file_path;
DROP INDEX IF EXISTS idx_movie_files_movie_id;
DROP INDEX IF EXISTS idx_movies_deleted_at;
DROP INDEX IF EXISTS idx_movies_monitoring_status;
DROP INDEX IF EXISTS idx_movies_imdb_id;
DROP INDEX IF EXISTS idx_movies_tmdb_id;

DROP TABLE IF EXISTS movie_files;
DROP TABLE IF EXISTS root_folders;
DROP TABLE IF EXISTS movies;

-- +goose StatementEnd
