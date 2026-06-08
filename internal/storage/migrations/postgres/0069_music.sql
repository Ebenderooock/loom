-- +goose Up
-- Phase 16: Music library foundation (Lidarr-equivalent), PostgreSQL.
-- Uses JSONB for JSON columns and BOOLEAN for flags. Mirrors the SQLite
-- migration 0069 (artist→album→album_release→track→track_file plus audio
-- quality definitions/profiles and metadata profiles).

CREATE TABLE artists (
    id                  TEXT      PRIMARY KEY,
    mbid                TEXT      UNIQUE,
    name                TEXT      NOT NULL,
    sort_name           TEXT      NOT NULL DEFAULT '',
    disambiguation      TEXT      NOT NULL DEFAULT '',
    artist_type         TEXT      NOT NULL DEFAULT '',
    country             TEXT      NOT NULL DEFAULT '',
    overview            TEXT      NOT NULL DEFAULT '',
    genres              JSONB     NOT NULL DEFAULT '[]'::jsonb,
    image_url           TEXT      NOT NULL DEFAULT '',
    path                TEXT      NOT NULL DEFAULT '',
    library_id          TEXT,
    quality_profile_id  TEXT,
    metadata_profile_id TEXT,
    monitoring_status   TEXT      NOT NULL DEFAULT 'monitored',
    metadata_provider   TEXT      NOT NULL DEFAULT 'musicbrainz',
    last_search_at      TIMESTAMP,
    created_at          TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at          TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at          TIMESTAMP
);

CREATE INDEX idx_artists_mbid              ON artists(mbid);
CREATE INDEX idx_artists_name              ON artists(name);
CREATE INDEX idx_artists_monitoring_status ON artists(monitoring_status);
CREATE INDEX idx_artists_deleted_at        ON artists(deleted_at);

CREATE TABLE albums (
    id                  TEXT      PRIMARY KEY,
    mbid                TEXT      UNIQUE,
    artist_id           TEXT      NOT NULL REFERENCES artists(id) ON DELETE CASCADE,
    title               TEXT      NOT NULL,
    album_type          TEXT      NOT NULL DEFAULT '',
    secondary_types     JSONB     NOT NULL DEFAULT '[]'::jsonb,
    release_date        TEXT      NOT NULL DEFAULT '',
    genres              JSONB     NOT NULL DEFAULT '[]'::jsonb,
    cover_art_url       TEXT      NOT NULL DEFAULT '',
    overview            TEXT      NOT NULL DEFAULT '',
    monitored           BOOLEAN   NOT NULL DEFAULT TRUE,
    selected_release_id TEXT,
    last_search_at      TIMESTAMP,
    releases_fetched_at TIMESTAMP,
    tracks_fetched_at   TIMESTAMP,
    created_at          TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at          TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at          TIMESTAMP
);

CREATE INDEX idx_albums_mbid       ON albums(mbid);
CREATE INDEX idx_albums_artist_id  ON albums(artist_id);
CREATE INDEX idx_albums_monitored  ON albums(monitored);
CREATE INDEX idx_albums_deleted_at ON albums(deleted_at);

CREATE TABLE album_releases (
    id                  TEXT      PRIMARY KEY,
    mbid                TEXT      UNIQUE,
    album_id            TEXT      NOT NULL REFERENCES albums(id) ON DELETE CASCADE,
    title               TEXT      NOT NULL DEFAULT '',
    disambiguation      TEXT      NOT NULL DEFAULT '',
    status              TEXT      NOT NULL DEFAULT '',
    release_date        TEXT      NOT NULL DEFAULT '',
    country             TEXT      NOT NULL DEFAULT '',
    label               TEXT      NOT NULL DEFAULT '',
    format              TEXT      NOT NULL DEFAULT '',
    media_count         INTEGER   NOT NULL DEFAULT 0,
    track_count         INTEGER   NOT NULL DEFAULT 0,
    created_at          TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at          TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_album_releases_album_id ON album_releases(album_id);
CREATE INDEX idx_album_releases_mbid     ON album_releases(mbid);

CREATE TABLE tracks (
    id                  TEXT      PRIMARY KEY,
    recording_mbid      TEXT,
    track_mbid          TEXT,
    album_id            TEXT      NOT NULL REFERENCES albums(id) ON DELETE CASCADE,
    release_id          TEXT      REFERENCES album_releases(id) ON DELETE CASCADE,
    title               TEXT      NOT NULL DEFAULT '',
    track_number        INTEGER   NOT NULL DEFAULT 0,
    disc_number         INTEGER   NOT NULL DEFAULT 1,
    duration_ms         INTEGER   NOT NULL DEFAULT 0,
    artist_name         TEXT      NOT NULL DEFAULT '',
    monitored           BOOLEAN   NOT NULL DEFAULT TRUE,
    has_file            BOOLEAN   NOT NULL DEFAULT FALSE,
    created_at          TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at          TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_tracks_album_id        ON tracks(album_id);
CREATE INDEX idx_tracks_release_id      ON tracks(release_id);
CREATE INDEX idx_tracks_recording_mbid  ON tracks(recording_mbid);
CREATE INDEX idx_tracks_track_mbid      ON tracks(track_mbid);
CREATE INDEX idx_tracks_has_file        ON tracks(has_file);
CREATE UNIQUE INDEX idx_tracks_release_pos ON tracks(release_id, disc_number, track_number);

CREATE TABLE track_files (
    id                  TEXT      PRIMARY KEY,
    track_id            TEXT      REFERENCES tracks(id) ON DELETE CASCADE,
    album_id            TEXT,
    artist_id           TEXT,
    file_path           TEXT      NOT NULL,
    size                BIGINT    NOT NULL DEFAULT 0,
    quality             TEXT      NOT NULL DEFAULT '',
    format              TEXT      NOT NULL DEFAULT '',
    bitrate             INTEGER   NOT NULL DEFAULT 0,
    media_info          JSONB     NOT NULL DEFAULT '{}'::jsonb,
    file_date           TIMESTAMP,
    date_added          TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_at          TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at          TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at          TIMESTAMP
);

CREATE INDEX idx_track_files_track_id   ON track_files(track_id);
CREATE INDEX idx_track_files_album_id   ON track_files(album_id);
CREATE INDEX idx_track_files_file_path  ON track_files(file_path);
CREATE INDEX idx_track_files_deleted_at ON track_files(deleted_at);

CREATE TABLE audio_quality_definitions (
    id          TEXT      PRIMARY KEY,
    name        TEXT      NOT NULL UNIQUE,
    format      TEXT      NOT NULL DEFAULT '',
    bitrate     INTEGER   NOT NULL DEFAULT 0,
    vbr         BOOLEAN   NOT NULL DEFAULT FALSE,
    lossless    BOOLEAN   NOT NULL DEFAULT FALSE,
    tier_order  INTEGER   NOT NULL DEFAULT 0,
    created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE audio_quality_profiles (
    id              TEXT      PRIMARY KEY,
    name            TEXT      NOT NULL,
    items           JSONB     NOT NULL DEFAULT '[]'::jsonb,
    cutoff          TEXT      NOT NULL DEFAULT '',
    upgrade_allowed BOOLEAN   NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at      TIMESTAMP
);

CREATE TABLE metadata_profiles (
    id               TEXT      PRIMARY KEY,
    name             TEXT      NOT NULL,
    primary_types    JSONB     NOT NULL DEFAULT '["Album"]'::jsonb,
    secondary_types  JSONB     NOT NULL DEFAULT '[]'::jsonb,
    release_statuses JSONB     NOT NULL DEFAULT '["official"]'::jsonb,
    created_at       TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at       TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at       TIMESTAMP
);

INSERT INTO audio_quality_definitions (id, name, format, bitrate, vbr, lossless, tier_order) VALUES
    ('aq_unknown',  'Unknown',  '',     0,   FALSE, FALSE, 0),
    ('aq_mp3_128',  'MP3-128',  'mp3',  128, FALSE, FALSE, 1),
    ('aq_mp3_v2',   'MP3-V2',   'mp3',  190, TRUE,  FALSE, 2),
    ('aq_mp3_256',  'MP3-256',  'mp3',  256, FALSE, FALSE, 3),
    ('aq_mp3_v0',   'MP3-V0',   'mp3',  245, TRUE,  FALSE, 4),
    ('aq_mp3_320',  'MP3-320',  'mp3',  320, FALSE, FALSE, 5),
    ('aq_aac_256',  'AAC-256',  'aac',  256, FALSE, FALSE, 6),
    ('aq_aac_320',  'AAC-320',  'aac',  320, FALSE, FALSE, 7),
    ('aq_flac',     'FLAC',     'flac', 0,   FALSE, TRUE,  8),
    ('aq_flac_24',  'FLAC-24',  'flac', 0,   FALSE, TRUE,  9);

-- +goose Down
DROP TABLE IF EXISTS metadata_profiles;
DROP TABLE IF EXISTS audio_quality_profiles;
DROP TABLE IF EXISTS audio_quality_definitions;
DROP TABLE IF EXISTS track_files;
DROP TABLE IF EXISTS tracks;
DROP TABLE IF EXISTS album_releases;
DROP TABLE IF EXISTS albums;
DROP TABLE IF EXISTS artists;
