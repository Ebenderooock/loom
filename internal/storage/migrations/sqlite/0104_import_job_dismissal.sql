-- +goose Up
ALTER TABLE import_jobs ADD COLUMN dismissed_at DATETIME;

-- +goose Down
ALTER TABLE import_jobs DROP COLUMN dismissed_at;
