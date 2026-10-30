-- +goose Up
-- MusicBrainz release-groups can be credited to more than one artist.
CREATE TABLE album_artists (
    album_id  TEXT NOT NULL REFERENCES albums(id) ON DELETE CASCADE,
    artist_id TEXT NOT NULL REFERENCES artists(id) ON DELETE CASCADE,
    PRIMARY KEY (album_id, artist_id)
);

INSERT INTO album_artists (album_id, artist_id)
SELECT id, artist_id FROM albums;

CREATE INDEX idx_album_artists_artist_id ON album_artists(artist_id);

-- +goose Down
DROP TABLE IF EXISTS album_artists;
