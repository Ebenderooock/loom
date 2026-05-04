-- +goose Up
-- RSS items table for Phase 5e-a: RSS feed monitoring
CREATE TABLE rss_items (
    id TEXT PRIMARY KEY,
    title TEXT NOT NULL,
    link TEXT,
    published_at TIMESTAMP,
    source_id TEXT NOT NULL,
    guid TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    raw TEXT,
    UNIQUE(guid, source_id)
);

CREATE INDEX idx_rss_items_source_created ON rss_items(source_id, created_at DESC);

-- User sources table for Phase 5e-c: Custom sources CRUD
CREATE TABLE user_sources (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    type TEXT NOT NULL, -- 'rss' or 'scraper'
    enabled BOOLEAN DEFAULT true,
    config TEXT NOT NULL, -- JSON: RSSSourceConfig or ScraperConfig
    last_sync_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_user_sources_enabled ON user_sources(enabled);

-- +goose Down
DROP TABLE IF EXISTS user_sources;
DROP TABLE IF EXISTS rss_items;
