-- +goose Up
-- +goose StatementBegin
ALTER TABLE notification_connections ADD COLUMN on_update_available INTEGER NOT NULL DEFAULT 0;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE notification_connections DROP COLUMN on_update_available;
-- +goose StatementEnd
