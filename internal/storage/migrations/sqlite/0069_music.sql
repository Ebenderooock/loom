-- +goose Up
-- Phase 16: Music library foundation (Lidarr-equivalent), SQLite.
--
-- Hierarchy mirrors series→season→episode as artist→album→track:
--   artists        — managed music artists (MusicBrainz artist MBID)
--   albums         — release-groups for an artist (the abstract "album")
--   album_releases — concrete MusicBrainz releases (editions) of an album
--   tracks         — per-release track listing, with has_file completeness
--   track_files    — physical audio files linked to a track
--
-- Audio quality + metadata profiles are music-specific (format/bitrate shaped,
-- not resolution-shaped). library_id / quality_profile_id / metadata_profile_id
-- are loose TEXT references (cross-domain coupling is avoided on purpose).

CREATE TABLE artists (
    id                  TEXT     PRIMARY KEY,
    mbid                TEXT     UNIQUE,
    name                TEXT     NOT NULL,
    sort_name           TEXT     NOT NULL DEFAULT '',
    disambiguation      TEXT     NOT NULL DEFAULT '',
    artist_type         TEXT     NOT NULL DEFAULT '',
    country             TEXT     NOT NULL DEFAULT '',
    overview            TEXT     NOT NULL DEFAULT '',
    genres              TEXT     NOT NULL DEFAULT '[]',
    image_url           TEXT     NOT NULL DEFAULT '',
    path                TEXT     NOT NULL DEFAULT '',
    library_id          TEXT,
    quality_profile_id  TEXT,
    metadata_profile_id TEXT,
    monitoring_status   TEXT     NOT NULL DEFAULT 'monitored',
    metadata_provider   TEXT     NOT NULL DEFAULT 'musicbrainz',
    last_search_at      DATETIME,
    created_at          DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at          DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at          DATETIME
);

CREATE INDEX idx_artists_mbid              ON artists(mbid);
CREATE INDEX idx_artists_name              ON artists(name);
CREATE INDEX idx_artists_monitoring_status ON artists(monitoring_status);
CREATE INDEX idx_artists_deleted_at        ON artists(deleted_at);

CREATE TABLE albums (
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
    deleted_at          DATETIME
);

CREATE INDEX idx_albums_mbid       ON albums(mbid);
CREATE INDEX idx_albums_artist_id  ON albums(artist_id);
CREATE INDEX idx_albums_monitored  ON albums(monitored);
CREATE INDEX idx_albums_deleted_at ON albums(deleted_at);

CREATE TABLE album_releases (
    id                  TEXT     PRIMARY KEY,
    mbid                TEXT     UNIQUE,
    album_id            TEXT     NOT NULL REFERENCES albums(id) ON DELETE CASCADE,
    title               TEXT     NOT NULL DEFAULT '',
    disambiguation      TEXT     NOT NULL DEFAULT '',
    status              TEXT     NOT NULL DEFAULT '',
    release_date        TEXT     NOT NULL DEFAULT '',
    country             TEXT     NOT NULL DEFAULT '',
    label               TEXT     NOT NULL DEFAULT '',
    format              TEXT     NOT NULL DEFAULT '',
    media_count         INTEGER  NOT NULL DEFAULT 0,
    track_count         INTEGER  NOT NULL DEFAULT 0,
    created_at          DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at          DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_album_releases_album_id ON album_releases(album_id);
CREATE INDEX idx_album_releases_mbid     ON album_releases(mbid);

CREATE TABLE tracks (
    id                  TEXT     PRIMARY KEY,
    recording_mbid      TEXT,
    track_mbid          TEXT,
    album_id            TEXT     NOT NULL REFERENCES albums(id) ON DELETE CASCADE,
    release_id          TEXT     REFERENCES album_releases(id) ON DELETE CASCADE,
    title               TEXT     NOT NULL DEFAULT '',
    track_number        INTEGER  NOT NULL DEFAULT 0,
    disc_number         INTEGER  NOT NULL DEFAULT 1,
    duration_ms         INTEGER  NOT NULL DEFAULT 0,
    artist_name         TEXT     NOT NULL DEFAULT '',
    monitored           INTEGER  NOT NULL DEFAULT 1,
    has_file            INTEGER  NOT NULL DEFAULT 0,
    created_at          DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at          DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_tracks_album_id        ON tracks(album_id);
CREATE INDEX idx_tracks_release_id      ON tracks(release_id);
CREATE INDEX idx_tracks_recording_mbid  ON tracks(recording_mbid);
CREATE INDEX idx_tracks_track_mbid      ON tracks(track_mbid);
CREATE INDEX idx_tracks_has_file        ON tracks(has_file);
CREATE UNIQUE INDEX idx_tracks_release_pos ON tracks(release_id, disc_number, track_number);

CREATE TABLE track_files (
    id                  TEXT     PRIMARY KEY,
    track_id            TEXT     REFERENCES tracks(id) ON DELETE CASCADE,
    album_id            TEXT,
    artist_id           TEXT,
    file_path           TEXT     NOT NULL,
    size                INTEGER  NOT NULL DEFAULT 0,
    quality             TEXT     NOT NULL DEFAULT '',
    format              TEXT     NOT NULL DEFAULT '',
    bitrate             INTEGER  NOT NULL DEFAULT 0,
    media_info          TEXT     NOT NULL DEFAULT '{}',
    file_date           DATETIME,
    date_added          DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_at          DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at          DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at          DATETIME
);

CREATE INDEX idx_track_files_track_id   ON track_files(track_id);
CREATE INDEX idx_track_files_album_id   ON track_files(album_id);
CREATE INDEX idx_track_files_file_path  ON track_files(file_path);
CREATE INDEX idx_track_files_deleted_at ON track_files(deleted_at);

CREATE TABLE audio_quality_definitions (
    id          TEXT     PRIMARY KEY,
    name        TEXT     NOT NULL UNIQUE,
    format      TEXT     NOT NULL DEFAULT '',
    bitrate     INTEGER  NOT NULL DEFAULT 0,
    vbr         INTEGER  NOT NULL DEFAULT 0,
    lossless    INTEGER  NOT NULL DEFAULT 0,
    tier_order  INTEGER  NOT NULL DEFAULT 0,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE audio_quality_profiles (
    id              TEXT     PRIMARY KEY,
    name            TEXT     NOT NULL,
    items           TEXT     NOT NULL DEFAULT '[]',
    cutoff          TEXT     NOT NULL DEFAULT '',
    upgrade_allowed INTEGER  NOT NULL DEFAULT 1,
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at      DATETIME
);

CREATE TABLE metadata_profiles (
    id               TEXT     PRIMARY KEY,
    name             TEXT     NOT NULL,
    primary_types    TEXT     NOT NULL DEFAULT '["Album"]',
    secondary_types  TEXT     NOT NULL DEFAULT '[]',
    release_statuses TEXT     NOT NULL DEFAULT '["official"]',
    created_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at       DATETIME
);

-- Seed the canonical audio quality tiers (ascending = better).
INSERT INTO audio_quality_definitions (id, name, format, bitrate, vbr, lossless, tier_order) VALUES
    ('aq_unknown',  'Unknown',  '',    0,   0, 0, 0),
    ('aq_mp3_128',  'MP3-128',  'mp3', 128, 0, 0, 1),
    ('aq_mp3_v2',   'MP3-V2',   'mp3', 190, 1, 0, 2),
    ('aq_mp3_256',  'MP3-256',  'mp3', 256, 0, 0, 3),
    ('aq_mp3_v0',   'MP3-V0',   'mp3', 245, 1, 0, 4),
    ('aq_mp3_320',  'MP3-320',  'mp3', 320, 0, 0, 5),
    ('aq_aac_256',  'AAC-256',  'aac', 256, 0, 0, 6),
    ('aq_aac_320',  'AAC-320',  'aac', 320, 0, 0, 7),
    ('aq_flac',     'FLAC',     'flac', 0,  0, 1, 8),
    ('aq_flac_24',  'FLAC-24',  'flac', 0,  0, 1, 9);

-- +goose Down
DROP TABLE IF EXISTS metadata_profiles;
DROP TABLE IF EXISTS audio_quality_profiles;
DROP TABLE IF EXISTS audio_quality_definitions;
DROP TABLE IF EXISTS track_files;
DROP TABLE IF EXISTS tracks;
DROP TABLE IF EXISTS album_releases;
DROP TABLE IF EXISTS albums;
DROP TABLE IF EXISTS artists;
