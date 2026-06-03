-- +goose Up

-- Feature flags store per-feature enable/disable overrides. Rows only exist
-- for features an admin has explicitly toggled; the default for any feature
-- not present here comes from the in-code registry (internal/featureflags).
CREATE TABLE IF NOT EXISTS feature_flags (
    key        TEXT PRIMARY KEY,
    enabled    INTEGER NOT NULL,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- +goose Down

DROP TABLE IF EXISTS feature_flags;
