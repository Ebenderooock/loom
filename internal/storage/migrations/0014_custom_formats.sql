-- Migration 0014: Custom formats and filters (Phase 5c)

-- Custom formats table: named sets of filters for scoring releases
CREATE TABLE custom_formats (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    description TEXT,
    tags TEXT, -- JSON array, e.g. ["hdr", "anime"]
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP
);

-- Indexes for common queries
CREATE INDEX idx_cf_name ON custom_formats(name);
CREATE INDEX idx_cf_created_at ON custom_formats(created_at);
CREATE INDEX idx_cf_deleted_at ON custom_formats(deleted_at);

-- Custom format filters: individual filter conditions
-- All filters within a format use AND logic (all must match for format to match)
CREATE TABLE custom_format_filters (
    id TEXT PRIMARY KEY,
    custom_format_id TEXT NOT NULL,
    field TEXT NOT NULL, -- codec, source, year, bitdepth, resolution, hdr, audio, language
    condition TEXT NOT NULL, -- equals, regex, range, in, gt, gte, lt, lte
    value TEXT NOT NULL, -- field-specific value or pattern
    "order" INTEGER NOT NULL DEFAULT 0, -- display order
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (custom_format_id) REFERENCES custom_formats(id) ON DELETE CASCADE
);

-- Indexes for filter lookups
CREATE INDEX idx_cff_format_id ON custom_format_filters(custom_format_id);
CREATE INDEX idx_cff_field ON custom_format_filters(field);
CREATE INDEX idx_cff_order ON custom_format_filters("order");
