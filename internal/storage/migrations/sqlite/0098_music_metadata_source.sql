-- +goose Up
CREATE TABLE music_metadata_source (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    mode TEXT NOT NULL DEFAULT 'musicbrainz',
    base_url TEXT NOT NULL DEFAULT '',
    rate_limit_ms INTEGER NOT NULL DEFAULT 0,
    auth_header_name TEXT NOT NULL DEFAULT '',
    auth_header_value TEXT NOT NULL DEFAULT '',
    search_on_public BOOLEAN NOT NULL DEFAULT TRUE
);
INSERT INTO music_metadata_source (id) VALUES (1) ON CONFLICT(id) DO NOTHING;

-- +goose Down
DROP TABLE music_metadata_source;
