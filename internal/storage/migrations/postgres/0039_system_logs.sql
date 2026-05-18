-- +goose Up
CREATE TABLE IF NOT EXISTS system_logs (
    id          TEXT PRIMARY KEY,
    timestamp   TEXT NOT NULL,
    level       TEXT NOT NULL,
    message     TEXT NOT NULL,
    source      TEXT,
    attrs       TEXT,
    workflow_id TEXT,
    created_at  TEXT NOT NULL DEFAULT (NOW() AT TIME ZONE 'utc')
);

CREATE INDEX idx_system_logs_timestamp ON system_logs(timestamp);
CREATE INDEX idx_system_logs_level ON system_logs(level);
CREATE INDEX idx_system_logs_workflow ON system_logs(workflow_id);

-- +goose Down
DROP TABLE IF EXISTS system_logs;
