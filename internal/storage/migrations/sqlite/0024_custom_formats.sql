-- +goose Up
CREATE TABLE custom_formats (
    id                   TEXT PRIMARY KEY,
    name                 TEXT NOT NULL,
    include_when_renaming BOOLEAN NOT NULL DEFAULT 0,
    specifications       TEXT NOT NULL DEFAULT '[]',
    score                INTEGER NOT NULL DEFAULT 0,
    created_at           TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    updated_at           TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);

-- +goose Down
DROP TABLE IF EXISTS custom_formats;
