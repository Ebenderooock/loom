-- +goose Up
-- Phase 5a: movies library foundation. The tables below establish the
-- foundational data model for the Movies module (equivalent to Radarr).
--
-- `movies` stores the library of managed movies. Each row represents a unique
-- movie tracked in the system. The `monitoring_status` field indicates whether
-- the movie is active (monitored), unmonitored, or soft-deleted. External IDs
-- (TMDB, IMDB, TVDB) enable lookup and deduplication across metadata providers.
-- `genres` is stored as JSON for flexibility (SQL/Postgres both support JSON).
-- `deleted_at` enables soft deletes; queries filter out soft-deleted rows by
-- default using a WHERE clause in the service layer.
--
-- `root_folders` maintains a list of paths where movies are organized on disk.
-- Each folder tracks free space (updated by scheduler) and a count of unmapped
-- files that don't belong to any movie in the library.
--
-- `movie_files` links each physical file to a movie, storing metadata (quality,
-- format, media info). One movie may have multiple files (e.g., remux + web-dl).

CREATE TABLE movies (
    id                  TEXT     PRIMARY KEY,
    title               TEXT     NOT NULL,
    year                INTEGER  NOT NULL DEFAULT 0,
    imdb_id             TEXT,
    tmdb_id             TEXT,
    tvdb_id             TEXT,
    overview            TEXT     NOT NULL DEFAULT '',
    genres              TEXT     NOT NULL DEFAULT '[]',
    runtime             INTEGER  NOT NULL DEFAULT 0,
    rating              REAL     NOT NULL DEFAULT 0.0,
    backdrop_path       TEXT     NOT NULL DEFAULT '',
    poster_path         TEXT     NOT NULL DEFAULT '',
    metadata_provider   TEXT     NOT NULL DEFAULT '',
    last_search_at      DATETIME,
    monitoring_status   TEXT     NOT NULL DEFAULT 'monitored',
    created_at          DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at          DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at          DATETIME
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
    free_space          INTEGER  NOT NULL DEFAULT 0,
    unmapped_count      INTEGER  NOT NULL DEFAULT 0,
    created_at          DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at          DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at          DATETIME
);

CREATE INDEX idx_root_folders_deleted_at ON root_folders(deleted_at);

CREATE TABLE movie_files (
    id                  TEXT     PRIMARY KEY,
    movie_id            TEXT     NOT NULL REFERENCES movies(id) ON DELETE CASCADE,
    file_path           TEXT     NOT NULL,
    size                INTEGER  NOT NULL DEFAULT 0,
    quality             TEXT     NOT NULL DEFAULT '',
    format              TEXT     NOT NULL DEFAULT '',
    media_info          TEXT     NOT NULL DEFAULT '{}',
    file_date           DATETIME,
    date_added          DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_at          DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at          DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at          DATETIME
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
