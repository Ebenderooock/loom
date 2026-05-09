-- +goose Up
-- Add separate release date columns for theatrical (cinema) and digital releases.
-- The existing release_date column remains as the general/primary release date.
ALTER TABLE movies ADD COLUMN theatrical_date TEXT NOT NULL DEFAULT '';
ALTER TABLE movies ADD COLUMN digital_date TEXT NOT NULL DEFAULT '';

CREATE INDEX idx_movies_theatrical_date ON movies(theatrical_date);
CREATE INDEX idx_movies_digital_date ON movies(digital_date);

-- +goose Down
DROP INDEX IF EXISTS idx_movies_digital_date;
DROP INDEX IF EXISTS idx_movies_theatrical_date;
-- SQLite doesn't support DROP COLUMN before 3.35.0, so we leave the columns
