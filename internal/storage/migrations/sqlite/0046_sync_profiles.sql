CREATE TABLE IF NOT EXISTS sync_profiles (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    app_type    TEXT NOT NULL DEFAULT '',
    enabled     BOOLEAN NOT NULL DEFAULT 1,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS sync_profile_indexers (
    profile_id  TEXT NOT NULL REFERENCES sync_profiles(id) ON DELETE CASCADE,
    indexer_id  TEXT NOT NULL,
    enabled     BOOLEAN NOT NULL DEFAULT 1,
    PRIMARY KEY (profile_id, indexer_id)
);

CREATE TABLE IF NOT EXISTS sync_profile_categories (
    profile_id  TEXT NOT NULL REFERENCES sync_profiles(id) ON DELETE CASCADE,
    category    TEXT NOT NULL,
    mapped_to   TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (profile_id, category)
);
