-- +goose Up
-- Exact provider references from deleted collections. Background discovery
-- skips them; an explicit import of the same reference clears the row.
CREATE TABLE collection_source_suppressions (
    provider TEXT NOT NULL,
    kind TEXT NOT NULL,
    external_id TEXT NOT NULL,
    suppressed_at BIGINT NOT NULL DEFAULT 0,
    PRIMARY KEY (provider, kind, external_id)
);

-- +goose Down
DROP TABLE collection_source_suppressions;
