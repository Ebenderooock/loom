-- +goose NO TRANSACTION
-- +goose Up
-- Album artist credits are independent from a primary managed artist. Rebuild
-- albums so artist_id can be NULL while preserving its existing child rows.
PRAGMA foreign_keys = OFF;
BEGIN;

CREATE TABLE albums_phase3 (
    id                  TEXT     PRIMARY KEY,
    mbid                TEXT     UNIQUE,
    artist_id           TEXT     REFERENCES artists(id) ON DELETE CASCADE,
    title               TEXT     NOT NULL,
    album_type          TEXT     NOT NULL DEFAULT '',
    secondary_types     TEXT     NOT NULL DEFAULT '[]',
    release_date        TEXT     NOT NULL DEFAULT '',
    genres              TEXT     NOT NULL DEFAULT '[]',
    cover_art_url       TEXT     NOT NULL DEFAULT '',
    overview            TEXT     NOT NULL DEFAULT '',
    monitored           INTEGER  NOT NULL DEFAULT 1,
    selected_release_id TEXT,
    last_search_at      DATETIME,
    releases_fetched_at DATETIME,
    tracks_fetched_at   DATETIME,
    created_at          DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at          DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at          DATETIME,
    rating              REAL     NOT NULL DEFAULT 0,
    rating_votes        INTEGER  NOT NULL DEFAULT 0,
    links               TEXT     NOT NULL DEFAULT '[]',
    details_fetched_at  TIMESTAMP,
    artist_credits      TEXT     NOT NULL DEFAULT '[]',
    library_id          TEXT,
    quality_profile_id  TEXT,
    metadata_profile_id TEXT
);

INSERT INTO albums_phase3 (
    id, mbid, artist_id, title, album_type, secondary_types, release_date,
    genres, cover_art_url, overview, monitored, selected_release_id,
    last_search_at, releases_fetched_at, tracks_fetched_at, created_at,
    updated_at, deleted_at, rating, rating_votes, links, details_fetched_at
)
SELECT
    id, mbid, artist_id, title, album_type, secondary_types, release_date,
    genres, cover_art_url, overview, monitored, selected_release_id,
    last_search_at, releases_fetched_at, tracks_fetched_at, created_at,
    updated_at, deleted_at, rating, rating_votes, links, details_fetched_at
FROM albums;

DROP TABLE albums;
ALTER TABLE albums_phase3 RENAME TO albums;

CREATE INDEX idx_albums_mbid       ON albums(mbid);
CREATE INDEX idx_albums_artist_id  ON albums(artist_id);
CREATE INDEX idx_albums_monitored  ON albums(monitored);
CREATE INDEX idx_albums_deleted_at ON albums(deleted_at);

COMMIT;
PRAGMA foreign_keys = ON;

-- +goose Down
-- Refuse a downgrade that would discard standalone albums.
CREATE TEMP TABLE IF NOT EXISTS album_downgrade_guard (
    standalone_count INTEGER CONSTRAINT standalone_albums_prevent_downgrade CHECK (standalone_count = 0)
);
INSERT INTO album_downgrade_guard
SELECT COUNT(*) FROM albums WHERE artist_id IS NULL;
DROP TABLE album_downgrade_guard;

PRAGMA foreign_keys = OFF;
BEGIN;

CREATE TABLE albums_before_0100 (
    id                  TEXT     PRIMARY KEY,
    mbid                TEXT     UNIQUE,
    artist_id           TEXT     NOT NULL REFERENCES artists(id) ON DELETE CASCADE,
    title               TEXT     NOT NULL,
    album_type          TEXT     NOT NULL DEFAULT '',
    secondary_types     TEXT     NOT NULL DEFAULT '[]',
    release_date        TEXT     NOT NULL DEFAULT '',
    genres              TEXT     NOT NULL DEFAULT '[]',
    cover_art_url       TEXT     NOT NULL DEFAULT '',
    overview            TEXT     NOT NULL DEFAULT '',
    monitored           INTEGER  NOT NULL DEFAULT 1,
    selected_release_id TEXT,
    last_search_at      DATETIME,
    releases_fetched_at DATETIME,
    tracks_fetched_at   DATETIME,
    created_at          DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at          DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at          DATETIME,
    rating              REAL     NOT NULL DEFAULT 0,
    rating_votes        INTEGER  NOT NULL DEFAULT 0,
    links               TEXT     NOT NULL DEFAULT '[]',
    details_fetched_at  TIMESTAMP
);

INSERT INTO albums_before_0100 (
    id, mbid, artist_id, title, album_type, secondary_types, release_date,
    genres, cover_art_url, overview, monitored, selected_release_id,
    last_search_at, releases_fetched_at, tracks_fetched_at, created_at,
    updated_at, deleted_at, rating, rating_votes, links, details_fetched_at
)
SELECT
    id, mbid, artist_id, title, album_type, secondary_types, release_date,
    genres, cover_art_url, overview, monitored, selected_release_id,
    last_search_at, releases_fetched_at, tracks_fetched_at, created_at,
    updated_at, deleted_at, rating, rating_votes, links, details_fetched_at
FROM albums;

DROP TABLE albums;
ALTER TABLE albums_before_0100 RENAME TO albums;

CREATE INDEX idx_albums_mbid       ON albums(mbid);
CREATE INDEX idx_albums_artist_id  ON albums(artist_id);
CREATE INDEX idx_albums_monitored  ON albums(monitored);
CREATE INDEX idx_albums_deleted_at ON albums(deleted_at);

COMMIT;
PRAGMA foreign_keys = ON;
