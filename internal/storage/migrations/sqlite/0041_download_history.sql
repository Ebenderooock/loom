-- +goose Up
CREATE TABLE IF NOT EXISTS download_history (
  id TEXT PRIMARY KEY,
  download_id TEXT NOT NULL,
  client_id TEXT NOT NULL DEFAULT '',
  title TEXT NOT NULL,
  category TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'completed',
  grabbed_at TEXT,
  completed_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX idx_download_history_completed_at ON download_history(completed_at DESC);

-- +goose Down
DROP TABLE IF EXISTS download_history;
