-- +goose Up
-- Phase 4a: metadata-service foundation. The three tables below provide
-- a shared abstraction layer for metadata (TMDB, TVDB, MusicBrainz).
--
-- Mirrors the SQLite schema with Postgres-native types: JSONB for
-- cached_json, TIMESTAMPTZ for timezone-aware timestamps, TEXT[] for
-- genres (still stored as JSON for parity with SQLite and sqlc).

CREATE TABLE metadata_movies (
    id              TEXT     PRIMARY KEY,
    tmdb_id         TEXT,
    imdb_id         TEXT,
    tvdb_id         TEXT,
    title           TEXT     NOT NULL,
    year            INTEGER,
    overview        TEXT     NOT NULL DEFAULT '',
    poster_path     TEXT     NOT NULL DEFAULT '',
    release_date    TEXT     NOT NULL DEFAULT '',
    runtime         INTEGER  NOT NULL DEFAULT 0,
    genres          JSONB    NOT NULL DEFAULT '[]'::jsonb,
    rating          FLOAT8   NOT NULL DEFAULT 0.0,
    cached_json     JSONB    NOT NULL DEFAULT '{}'::jsonb,
    cached_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at      TIMESTAMPTZ NOT NULL DEFAULT (NOW() + INTERVAL '7 days')
);

CREATE INDEX idx_metadata_movies_tmdb_id  ON metadata_movies(tmdb_id);
CREATE INDEX idx_metadata_movies_imdb_id  ON metadata_movies(imdb_id);
CREATE INDEX idx_metadata_movies_tvdb_id  ON metadata_movies(tvdb_id);
CREATE INDEX idx_metadata_movies_expires  ON metadata_movies(expires_at);

CREATE TABLE metadata_series (
    id              TEXT     PRIMARY KEY,
    tmdb_id         TEXT,
    imdb_id         TEXT,
    tvdb_id         TEXT,
    title           TEXT     NOT NULL,
    overview        TEXT     NOT NULL DEFAULT '',
    poster_path     TEXT     NOT NULL DEFAULT '',
    first_air_date  TEXT     NOT NULL DEFAULT '',
    genres          JSONB    NOT NULL DEFAULT '[]'::jsonb,
    rating          FLOAT8   NOT NULL DEFAULT 0.0,
    seasons         INTEGER  NOT NULL DEFAULT 0,
    cached_json     JSONB    NOT NULL DEFAULT '{}'::jsonb,
    cached_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at      TIMESTAMPTZ NOT NULL DEFAULT (NOW() + INTERVAL '7 days')
);

CREATE INDEX idx_metadata_series_tmdb_id  ON metadata_series(tmdb_id);
CREATE INDEX idx_metadata_series_imdb_id  ON metadata_series(imdb_id);
CREATE INDEX idx_metadata_series_tvdb_id  ON metadata_series(tvdb_id);
CREATE INDEX idx_metadata_series_expires  ON metadata_series(expires_at);

CREATE TABLE metadata_episodes (
    id              TEXT     PRIMARY KEY,
    series_id       TEXT     NOT NULL,
    season          INTEGER  NOT NULL,
    episode         INTEGER  NOT NULL,
    tvdb_id         TEXT,
    tmdb_id         TEXT,
    title           TEXT     NOT NULL,
    overview        TEXT     NOT NULL DEFAULT '',
    air_date        TEXT     NOT NULL DEFAULT '',
    runtime         INTEGER  NOT NULL DEFAULT 0,
    rating          FLOAT8   NOT NULL DEFAULT 0.0,
    cached_json     JSONB    NOT NULL DEFAULT '{}'::jsonb,
    cached_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at      TIMESTAMPTZ NOT NULL DEFAULT (NOW() + INTERVAL '7 days'),
    UNIQUE(series_id, season, episode)
);

CREATE INDEX idx_metadata_episodes_series_id  ON metadata_episodes(series_id);
CREATE INDEX idx_metadata_episodes_tvdb_id    ON metadata_episodes(tvdb_id);
CREATE INDEX idx_metadata_episodes_tmdb_id    ON metadata_episodes(tmdb_id);
CREATE INDEX idx_metadata_episodes_expires    ON metadata_episodes(expires_at);

-- +goose Down
DROP INDEX IF EXISTS idx_metadata_episodes_expires;
DROP INDEX IF EXISTS idx_metadata_episodes_tmdb_id;
DROP INDEX IF EXISTS idx_metadata_episodes_tvdb_id;
DROP INDEX IF EXISTS idx_metadata_episodes_series_id;
DROP TABLE IF EXISTS metadata_episodes;

DROP INDEX IF EXISTS idx_metadata_series_expires;
DROP INDEX IF EXISTS idx_metadata_series_tvdb_id;
DROP INDEX IF EXISTS idx_metadata_series_imdb_id;
DROP INDEX IF EXISTS idx_metadata_series_tmdb_id;
DROP TABLE IF EXISTS metadata_series;

DROP INDEX IF EXISTS idx_metadata_movies_expires;
DROP INDEX IF EXISTS idx_metadata_movies_tvdb_id;
DROP INDEX IF EXISTS idx_metadata_movies_imdb_id;
DROP INDEX IF EXISTS idx_metadata_movies_tmdb_id;
DROP TABLE IF EXISTS metadata_movies;
