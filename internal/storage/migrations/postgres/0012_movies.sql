-- +goose Up
-- Phase 5a: movies library foundation (PostgreSQL).
-- Uses JSONB for genres and media_info for better performance.

CREATE TABLE movies (
    id                  TEXT     PRIMARY KEY,
    title               TEXT     NOT NULL,
    year                INTEGER  NOT NULL DEFAULT 0,
    imdb_id             TEXT,
    tmdb_id             TEXT,
    tvdb_id             TEXT,
    overview            TEXT     NOT NULL DEFAULT '',
    genres              JSONB    NOT NULL DEFAULT '[]'::jsonb,
    runtime             INTEGER  NOT NULL DEFAULT 0,
    rating              REAL     NOT NULL DEFAULT 0.0,
    backdrop_path       TEXT     NOT NULL DEFAULT '',
    poster_path         TEXT     NOT NULL DEFAULT '',
    metadata_provider   TEXT     NOT NULL DEFAULT '',
    last_search_at      TIMESTAMP,
    monitoring_status   TEXT     NOT NULL DEFAULT 'monitored',
    created_at          TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at          TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at          TIMESTAMP
);

CREATE INDEX idx_movies_tmdb_id         ON movies(tmdb_id);
CREATE INDEX idx_movies_imdb_id         ON movies(imdb_id);
CREATE INDEX idx_movies_tvdb_id         ON movies(tvdb_id);
CREATE INDEX idx_movies_monitoring_status ON movies(monitoring_status);
CREATE INDEX idx_movies_deleted_at      ON movies(deleted_at);
CREATE INDEX idx_movies_title           ON movies(title);

CREATE TABLE root_folders (
    id                  TEXT     PRIMARY KEY,
    path                TEXT     NOT NULL UNIQUE,
    free_space          BIGINT   NOT NULL DEFAULT 0,
    unmapped_count      INTEGER  NOT NULL DEFAULT 0,
    created_at          TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at          TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at          TIMESTAMP
);

CREATE INDEX idx_root_folders_deleted_at ON root_folders(deleted_at);

CREATE TABLE movie_files (
    id                  TEXT     PRIMARY KEY,
    movie_id            TEXT     NOT NULL REFERENCES movies(id) ON DELETE CASCADE,
    file_path           TEXT     NOT NULL,
    size                BIGINT   NOT NULL DEFAULT 0,
    quality             TEXT     NOT NULL DEFAULT '',
    format              TEXT     NOT NULL DEFAULT '',
    media_info          JSONB    NOT NULL DEFAULT '{}'::jsonb,
    file_date           TIMESTAMP,
    date_added          TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_at          TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at          TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at          TIMESTAMP
);

CREATE INDEX idx_movie_files_movie_id   ON movie_files(movie_id);
CREATE INDEX idx_movie_files_file_path  ON movie_files(file_path);
CREATE INDEX idx_movie_files_deleted_at ON movie_files(deleted_at);

-- +goose Down
DROP INDEX IF EXISTS idx_movie_files_deleted_at;
DROP INDEX IF EXISTS idx_movie_files_file_path;
DROP INDEX IF EXISTS idx_movie_files_movie_id;
DROP TABLE IF EXISTS movie_files;

DROP INDEX IF EXISTS idx_root_folders_deleted_at;
DROP TABLE IF EXISTS root_folders;

DROP INDEX IF EXISTS idx_movies_title;
DROP INDEX IF EXISTS idx_movies_deleted_at;
DROP INDEX IF EXISTS idx_movies_monitoring_status;
DROP INDEX IF EXISTS idx_movies_tvdb_id;
DROP INDEX IF EXISTS idx_movies_imdb_id;
DROP INDEX IF EXISTS idx_movies_tmdb_id;
DROP TABLE IF EXISTS movies;
