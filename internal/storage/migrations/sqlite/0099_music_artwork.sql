-- +goose Up
ALTER TABLE artists ADD COLUMN background_url TEXT NOT NULL DEFAULT '';
ALTER TABLE artists ADD COLUMN logo_url TEXT NOT NULL DEFAULT '';
ALTER TABLE artists ADD COLUMN begin_area TEXT NOT NULL DEFAULT '';
ALTER TABLE artists ADD COLUMN begin_date TEXT NOT NULL DEFAULT '';
ALTER TABLE artists ADD COLUMN end_date TEXT NOT NULL DEFAULT '';
ALTER TABLE artists ADD COLUMN ended BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE artists ADD COLUMN rating REAL NOT NULL DEFAULT 0;
ALTER TABLE artists ADD COLUMN rating_votes INTEGER NOT NULL DEFAULT 0;
ALTER TABLE artists ADD COLUMN links TEXT NOT NULL DEFAULT '[]';
ALTER TABLE albums ADD COLUMN rating REAL NOT NULL DEFAULT 0;
ALTER TABLE albums ADD COLUMN rating_votes INTEGER NOT NULL DEFAULT 0;
ALTER TABLE albums ADD COLUMN links TEXT NOT NULL DEFAULT '[]';
ALTER TABLE albums ADD COLUMN details_fetched_at TIMESTAMP;

CREATE TABLE music_artwork_settings (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    fanart_api_key TEXT NOT NULL DEFAULT '',
    audiodb_api_key TEXT NOT NULL DEFAULT ''
);
INSERT INTO music_artwork_settings (id) VALUES (1) ON CONFLICT(id) DO NOTHING;

-- +goose Down
DROP TABLE music_artwork_settings;
ALTER TABLE albums DROP COLUMN details_fetched_at;
ALTER TABLE albums DROP COLUMN links;
ALTER TABLE albums DROP COLUMN rating_votes;
ALTER TABLE albums DROP COLUMN rating;
ALTER TABLE artists DROP COLUMN links;
ALTER TABLE artists DROP COLUMN rating_votes;
ALTER TABLE artists DROP COLUMN rating;
ALTER TABLE artists DROP COLUMN ended;
ALTER TABLE artists DROP COLUMN end_date;
ALTER TABLE artists DROP COLUMN begin_date;
ALTER TABLE artists DROP COLUMN begin_area;
ALTER TABLE artists DROP COLUMN logo_url;
ALTER TABLE artists DROP COLUMN background_url;
