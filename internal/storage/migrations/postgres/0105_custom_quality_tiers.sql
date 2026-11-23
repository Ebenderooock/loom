-- +goose Up
CREATE TABLE custom_quality_tiers (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    specifications TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

-- +goose Down
DROP TABLE custom_quality_tiers;
