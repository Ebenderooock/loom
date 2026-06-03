-- +goose Up

-- Mode controls list behaviour: 'auto' auto-adds fetched items to the library
-- (existing behaviour), 'discover' only lists them for manual adding via the
-- Discover section.
ALTER TABLE import_lists ADD COLUMN mode TEXT NOT NULL DEFAULT 'auto';

-- Cache poster/overview on items so the Discover grid can render without
-- hitting TMDB on every page load.
ALTER TABLE import_list_items ADD COLUMN poster_path TEXT DEFAULT '';
ALTER TABLE import_list_items ADD COLUMN overview TEXT DEFAULT '';

-- +goose Down

-- SQLite does not support DROP COLUMN in older versions; this is a best-effort rollback.
