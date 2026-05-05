-- +goose Up

CREATE TABLE IF NOT EXISTS import_lists (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    list_type TEXT NOT NULL,
    enabled INTEGER NOT NULL DEFAULT 1,
    url TEXT,
    api_key TEXT,
    access_token TEXT,
    sync_interval_minutes INTEGER NOT NULL DEFAULT 360,
    root_folder_path TEXT,
    quality_profile_id TEXT DEFAULT 'default',
    media_type TEXT NOT NULL DEFAULT 'movie',
    monitor_type TEXT NOT NULL DEFAULT 'all',
    search_on_add INTEGER NOT NULL DEFAULT 1,
    last_sync DATETIME,
    settings TEXT DEFAULT '{}',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS import_list_exclusions (
    id TEXT PRIMARY KEY,
    tmdb_id TEXT,
    tvdb_id TEXT,
    imdb_id TEXT,
    title TEXT NOT NULL,
    year INTEGER,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS import_list_items (
    id TEXT PRIMARY KEY,
    list_id TEXT NOT NULL REFERENCES import_lists(id) ON DELETE CASCADE,
    external_id TEXT NOT NULL,
    title TEXT NOT NULL,
    year INTEGER,
    imdb_id TEXT,
    tmdb_id TEXT,
    tvdb_id TEXT,
    status TEXT NOT NULL DEFAULT 'pending',
    last_seen DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_import_list_items_list_id ON import_list_items(list_id);
CREATE INDEX idx_import_list_exclusions_imdb ON import_list_exclusions(imdb_id);
CREATE INDEX idx_import_list_exclusions_tmdb ON import_list_exclusions(tmdb_id);

-- +goose Down

DROP TABLE IF EXISTS import_list_items;
DROP TABLE IF EXISTS import_list_exclusions;
DROP TABLE IF EXISTS import_lists;
