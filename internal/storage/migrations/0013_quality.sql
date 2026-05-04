-- +migrate Up

-- Create quality_definitions table for storing quality settings
CREATE TABLE quality_definitions (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    title TEXT NOT NULL,
    source TEXT NOT NULL,          -- e.g., "BluRay", "HDTV", "WebRip"
    resolution TEXT NOT NULL,      -- e.g., "1080p", "720p", "2160p"
    modifier TEXT,                 -- e.g., "REMUX", "PROPER"
    min_file_size BIGINT DEFAULT 0,
    max_file_size BIGINT DEFAULT 0,
    preferred_at INTEGER DEFAULT 100,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP
);

CREATE INDEX idx_quality_definitions_name ON quality_definitions(name) WHERE deleted_at IS NULL;
CREATE INDEX idx_quality_definitions_deleted ON quality_definitions(deleted_at);

-- Create quality_profiles table for storing quality profile configurations
CREATE TABLE quality_profiles (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    upgrade_allowed BOOLEAN DEFAULT FALSE,
    cutoff TEXT NOT NULL,           -- quality definition ID
    language TEXT DEFAULT 'en',
    format_items TEXT,              -- JSON array of format score objects
    min_format_score INTEGER DEFAULT 0,
    cutoff_format_score INTEGER DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP,
    FOREIGN KEY (cutoff) REFERENCES quality_definitions(id)
);

CREATE INDEX idx_quality_profiles_name ON quality_profiles(name) WHERE deleted_at IS NULL;
CREATE INDEX idx_quality_profiles_cutoff ON quality_profiles(cutoff) WHERE deleted_at IS NULL;
CREATE INDEX idx_quality_profiles_deleted ON quality_profiles(deleted_at);

-- Create quality_profile_items junction table for quality items in profiles
CREATE TABLE quality_profile_items (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    profile_id TEXT NOT NULL,
    quality_definition_id TEXT NOT NULL,
    preferred BOOLEAN DEFAULT FALSE,
    allowed BOOLEAN DEFAULT TRUE,
    FOREIGN KEY (profile_id) REFERENCES quality_profiles(id) ON DELETE CASCADE,
    FOREIGN KEY (quality_definition_id) REFERENCES quality_definitions(id)
);

CREATE INDEX idx_quality_profile_items_profile ON quality_profile_items(profile_id);
CREATE INDEX idx_quality_profile_items_quality ON quality_profile_items(quality_definition_id);

-- +migrate Down

DROP INDEX IF EXISTS idx_quality_profile_items_quality;
DROP INDEX IF EXISTS idx_quality_profile_items_profile;
DROP TABLE IF EXISTS quality_profile_items;

DROP INDEX IF EXISTS idx_quality_profiles_deleted;
DROP INDEX IF EXISTS idx_quality_profiles_cutoff;
DROP INDEX IF EXISTS idx_quality_profiles_name;
DROP TABLE IF EXISTS quality_profiles;

DROP INDEX IF EXISTS idx_quality_definitions_deleted;
DROP INDEX IF EXISTS idx_quality_definitions_name;
DROP TABLE IF EXISTS quality_definitions;
