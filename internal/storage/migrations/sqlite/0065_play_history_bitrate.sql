-- +goose Up
-- +goose StatementBegin
ALTER TABLE play_history ADD COLUMN bitrate_kbps INTEGER NOT NULL DEFAULT 0;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE play_history DROP COLUMN bitrate_kbps;
-- +goose StatementEnd
