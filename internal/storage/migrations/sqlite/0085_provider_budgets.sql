-- +goose Up
CREATE TABLE provider_budgets (
    id TEXT PRIMARY KEY,
    allowance BIGINT NOT NULL CHECK (allowance > 0),
    window_seconds BIGINT NOT NULL CHECK (window_seconds > 0),
    consumed BIGINT NOT NULL DEFAULT 0,
    reset_at BIGINT NOT NULL,
    provider_remaining BIGINT,
    provider_reset_at BIGINT NOT NULL DEFAULT 0
);
CREATE TABLE provider_budget_members (
    indexer_id TEXT PRIMARY KEY REFERENCES indexers(id) ON DELETE CASCADE,
    budget_id TEXT NOT NULL REFERENCES provider_budgets(id) ON DELETE CASCADE
);
CREATE INDEX provider_budget_members_budget ON provider_budget_members(budget_id);

-- +goose Down
DROP TABLE provider_budget_members;
DROP TABLE provider_budgets;
