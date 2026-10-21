-- +goose Up
-- The Postgres series schema has not landed on all supported installations.
ALTER TABLE IF EXISTS episodes ADD COLUMN air_date_utc TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE IF EXISTS episodes DROP COLUMN air_date_utc;
