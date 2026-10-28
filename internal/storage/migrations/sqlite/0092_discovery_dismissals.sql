-- +goose Up
CREATE TABLE discovery_dismissals (
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    media_type TEXT NOT NULL CHECK (media_type IN ('movie', 'series')),
    tmdb_id INTEGER NOT NULL CHECK (tmdb_id > 0),
    PRIMARY KEY (user_id, media_type, tmdb_id)
);

-- +goose Down
DROP TABLE discovery_dismissals;
