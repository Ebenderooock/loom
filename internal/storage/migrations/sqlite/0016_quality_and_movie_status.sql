-- +goose Up
-- Add quality definitions, quality profiles, and movie status tracking.

-- Quality definitions: individual quality tiers (e.g., BluRay-1080p, HDTV-720p)
CREATE TABLE IF NOT EXISTS quality_definitions (
    id              TEXT     PRIMARY KEY,
    name            TEXT     NOT NULL UNIQUE,
    title           TEXT     NOT NULL DEFAULT '',
    source          TEXT     NOT NULL,
    resolution      TEXT     NOT NULL,
    modifier        TEXT     NOT NULL DEFAULT '',
    min_file_size   INTEGER  NOT NULL DEFAULT 0,
    max_file_size   INTEGER  NOT NULL DEFAULT 0,
    preferred_at    INTEGER  NOT NULL DEFAULT 0,
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at      DATETIME
);

-- Quality profiles: named collections of quality tiers with upgrade preferences
CREATE TABLE IF NOT EXISTS quality_profiles (
    id                  TEXT     PRIMARY KEY,
    name                TEXT     NOT NULL UNIQUE,
    upgrade_allowed     BOOLEAN  NOT NULL DEFAULT 0,
    cutoff              TEXT     NOT NULL DEFAULT '',
    language            TEXT     NOT NULL DEFAULT 'en',
    items               TEXT     NOT NULL DEFAULT '[]',
    format_items        TEXT     NOT NULL DEFAULT '[]',
    min_format_score    INTEGER  NOT NULL DEFAULT 0,
    cutoff_format_score INTEGER  NOT NULL DEFAULT 0,
    created_at          DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at          DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at          DATETIME
);

-- Junction table: which quality definitions belong to which profiles
CREATE TABLE IF NOT EXISTS quality_profile_items (
    profile_id             TEXT    NOT NULL REFERENCES quality_profiles(id) ON DELETE CASCADE,
    quality_definition_id  TEXT    NOT NULL,
    preferred              BOOLEAN NOT NULL DEFAULT 0,
    allowed                BOOLEAN NOT NULL DEFAULT 1,
    PRIMARY KEY (profile_id, quality_definition_id)
);

-- Add quality_profile_id, root_folder_id, and status to movies
ALTER TABLE movies ADD COLUMN quality_profile_id TEXT NOT NULL DEFAULT '';
ALTER TABLE movies ADD COLUMN root_folder_id TEXT NOT NULL DEFAULT '';
ALTER TABLE movies ADD COLUMN status TEXT NOT NULL DEFAULT 'missing';
ALTER TABLE movies ADD COLUMN release_date TEXT NOT NULL DEFAULT '';

CREATE INDEX idx_movies_status ON movies(status);
CREATE INDEX idx_movies_quality_profile_id ON movies(quality_profile_id);
CREATE INDEX idx_movies_root_folder_id ON movies(root_folder_id);

-- +goose Down
DROP INDEX IF EXISTS idx_movies_root_folder_id;
DROP INDEX IF EXISTS idx_movies_quality_profile_id;
DROP INDEX IF EXISTS idx_movies_status;

DROP TABLE IF EXISTS quality_profile_items;
DROP TABLE IF EXISTS quality_profiles;
DROP TABLE IF EXISTS quality_definitions;
