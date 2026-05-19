-- +goose Up
ALTER TABLE search_debug_log ADD COLUMN status TEXT NOT NULL DEFAULT 'completed';
ALTER TABLE search_debug_log ADD COLUMN updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();
ALTER TABLE search_debug_log ADD COLUMN search_run_id TEXT NOT NULL DEFAULT '';

CREATE INDEX idx_search_debug_log_status ON search_debug_log(status);
CREATE INDEX idx_search_debug_log_updated ON search_debug_log(updated_at);
CREATE INDEX idx_search_debug_log_run ON search_debug_log(search_run_id);

-- +goose Down
DROP INDEX IF EXISTS idx_search_debug_log_run;
DROP INDEX IF EXISTS idx_search_debug_log_updated;
DROP INDEX IF EXISTS idx_search_debug_log_status;

ALTER TABLE search_debug_log DROP COLUMN search_run_id;
ALTER TABLE search_debug_log DROP COLUMN updated_at;
ALTER TABLE search_debug_log DROP COLUMN status;
