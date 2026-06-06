-- +goose Up
-- +goose StatementBegin
ALTER TABLE plugins ADD COLUMN source TEXT NOT NULL DEFAULT '';
-- +goose StatementEnd
-- Plugins are now JavaScript-only. Any pre-existing (command-era) rows have no
-- JS source, so disable them rather than letting them dispatch as no-op
-- "successful" runs. Admins can re-author them as JavaScript and re-enable.
-- +goose StatementBegin
UPDATE plugins SET enabled = 0 WHERE source = '';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE plugins DROP COLUMN source;
-- +goose StatementEnd
