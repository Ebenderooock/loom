-- +goose Up
ALTER TABLE episodes ADD COLUMN air_date_utc TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE episodes DROP COLUMN air_date_utc;
