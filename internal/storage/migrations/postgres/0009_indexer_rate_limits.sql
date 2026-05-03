-- +goose Up
-- Phase 2f: per-indexer rate limiting + retry policy.
ALTER TABLE indexers ADD COLUMN rate_limit_per_min INTEGER;
ALTER TABLE indexers ADD COLUMN rate_limit_burst INTEGER;
ALTER TABLE indexers ADD COLUMN retry_max_attempts INTEGER;

-- +goose Down
ALTER TABLE indexers DROP COLUMN IF EXISTS retry_max_attempts;
ALTER TABLE indexers DROP COLUMN IF EXISTS rate_limit_burst;
ALTER TABLE indexers DROP COLUMN IF EXISTS rate_limit_per_min;
