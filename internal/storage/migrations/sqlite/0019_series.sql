-- +goose Up

CREATE TABLE IF NOT EXISTS series (
    id TEXT PRIMARY KEY,
    title TEXT NOT NULL,
    year INTEGER,
    imdb_id TEXT,
    tmdb_id TEXT,
    tvdb_id TEXT,
    overview TEXT,
    genres TEXT DEFAULT '[]',
    runtime INTEGER DEFAULT 0,
    rating REAL DEFAULT 0,
    backdrop_path TEXT DEFAULT '',
    poster_path TEXT DEFAULT '',
    network TEXT DEFAULT '',
    status TEXT DEFAULT 'continuing',
    series_type TEXT DEFAULT 'standard',
    metadata_provider TEXT DEFAULT 'tmdb',
    quality_profile_id TEXT DEFAULT '',
    root_folder_id TEXT DEFAULT '',
    monitoring_status TEXT DEFAULT 'monitored',
    season_folder BOOLEAN DEFAULT true,
    release_date TEXT DEFAULT '',
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
);

CREATE TABLE IF NOT EXISTS seasons (
    id TEXT PRIMARY KEY,
    series_id TEXT NOT NULL REFERENCES series(id) ON DELETE CASCADE,
    season_number INTEGER NOT NULL,
    title TEXT DEFAULT '',
    overview TEXT DEFAULT '',
    poster_path TEXT DEFAULT '',
    monitored BOOLEAN DEFAULT true,
    episode_count INTEGER DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
    UNIQUE(series_id, season_number)
);

CREATE TABLE IF NOT EXISTS episodes (
    id TEXT PRIMARY KEY,
    series_id TEXT NOT NULL REFERENCES series(id) ON DELETE CASCADE,
    season_id TEXT NOT NULL REFERENCES seasons(id) ON DELETE CASCADE,
    episode_number INTEGER NOT NULL,
    title TEXT DEFAULT '',
    overview TEXT DEFAULT '',
    air_date TEXT DEFAULT '',
    runtime INTEGER DEFAULT 0,
    still_path TEXT DEFAULT '',
    monitored BOOLEAN DEFAULT true,
    has_file BOOLEAN DEFAULT false,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
    UNIQUE(series_id, season_id, episode_number)
);

CREATE TABLE IF NOT EXISTS episode_files (
    id TEXT PRIMARY KEY,
    episode_id TEXT NOT NULL REFERENCES episodes(id) ON DELETE CASCADE,
    series_id TEXT NOT NULL REFERENCES series(id) ON DELETE CASCADE,
    file_path TEXT NOT NULL,
    file_size INTEGER DEFAULT 0,
    quality TEXT DEFAULT '',
    source TEXT DEFAULT '',
    resolution TEXT DEFAULT '',
    codec TEXT DEFAULT '',
    media_info TEXT DEFAULT '{}',
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
);

CREATE TABLE IF NOT EXISTS series_credits (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    series_id TEXT NOT NULL REFERENCES series(id) ON DELETE CASCADE,
    person_name TEXT NOT NULL,
    character_name TEXT DEFAULT '',
    role TEXT NOT NULL DEFAULT 'actor',
    profile_path TEXT DEFAULT '',
    tmdb_person_id INTEGER DEFAULT 0,
    display_order INTEGER DEFAULT 0
);

-- +goose Down

DROP TABLE IF EXISTS series_credits;
DROP TABLE IF EXISTS episode_files;
DROP TABLE IF EXISTS episodes;
DROP TABLE IF EXISTS seasons;
DROP TABLE IF EXISTS series;
