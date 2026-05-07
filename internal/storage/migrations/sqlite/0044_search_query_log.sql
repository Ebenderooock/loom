-- +goose Up
CREATE TABLE IF NOT EXISTS search_query_log (
    id          TEXT PRIMARY KEY,
    query       TEXT NOT NULL,
    query_type  TEXT NOT NULL DEFAULT 'search',
    media_type  TEXT NOT NULL DEFAULT '',
    media_id    TEXT NOT NULL DEFAULT '',
    started_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    finished_at DATETIME,
    total_results INTEGER NOT NULL DEFAULT 0,
    status      TEXT NOT NULL DEFAULT 'running'
);

CREATE TABLE IF NOT EXISTS search_query_indexer_log (
    id          TEXT PRIMARY KEY,
    query_id    TEXT NOT NULL REFERENCES search_query_log(id) ON DELETE CASCADE,
    indexer_id  TEXT NOT NULL,
    indexer_name TEXT NOT NULL,
    started_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    finished_at DATETIME,
    latency_ms  INTEGER NOT NULL DEFAULT 0,
    result_count INTEGER NOT NULL DEFAULT 0,
    error       TEXT NOT NULL DEFAULT '',
    status      TEXT NOT NULL DEFAULT 'running'
);

CREATE INDEX idx_search_query_log_started ON search_query_log(started_at);
CREATE INDEX idx_search_query_indexer_log_query ON search_query_indexer_log(query_id);

-- +goose Down
DROP TABLE IF EXISTS search_query_indexer_log;
DROP TABLE IF EXISTS search_query_log;
