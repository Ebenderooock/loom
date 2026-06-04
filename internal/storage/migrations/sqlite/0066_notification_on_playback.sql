-- +goose Up
-- +goose StatementBegin
ALTER TABLE notification_connections ADD COLUMN on_playback INTEGER NOT NULL DEFAULT 0;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE notification_connections DROP COLUMN on_playback;
-- +goose StatementEnd
