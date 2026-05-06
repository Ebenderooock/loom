-- +goose Up
CREATE TABLE IF NOT EXISTS remote_path_mappings (
  id TEXT PRIMARY KEY,
  client_id TEXT NOT NULL,
  remote_path TEXT NOT NULL,
  local_path TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX idx_rpm_client ON remote_path_mappings(client_id);

-- +goose Down
DROP TABLE IF EXISTS remote_path_mappings;
