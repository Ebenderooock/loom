-- +goose Up
ALTER TABLE album_artists ADD COLUMN catalog_included BOOLEAN NOT NULL DEFAULT TRUE;

-- +goose Down
ALTER TABLE album_artists DROP COLUMN catalog_included;
