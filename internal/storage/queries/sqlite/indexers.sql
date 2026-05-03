-- name: CreateIndexer :one
INSERT INTO indexers (id, kind, name, enabled, priority, config_json, categories_json, tags_json, proxy_id, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
RETURNING *;

-- name: GetIndexer :one
SELECT * FROM indexers WHERE id = ? LIMIT 1;

-- name: ListIndexers :many
SELECT * FROM indexers ORDER BY priority ASC, name ASC;

-- name: ListEnabledIndexers :many
SELECT * FROM indexers WHERE enabled = 1 ORDER BY priority ASC, name ASC;

-- name: ReplaceIndexer :one
UPDATE indexers
SET kind            = ?,
    name            = ?,
    enabled         = ?,
    priority        = ?,
    config_json     = ?,
    categories_json = ?,
    tags_json       = ?,
    proxy_id        = ?,
    updated_at      = CURRENT_TIMESTAMP
WHERE id = ?
RETURNING *;

-- name: PatchIndexer :one
UPDATE indexers
SET name      = COALESCE(sqlc.narg('name'), name),
    enabled   = COALESCE(sqlc.narg('enabled'), enabled),
    priority  = COALESCE(sqlc.narg('priority'), priority),
    tags_json = COALESCE(sqlc.narg('tags_json'), tags_json),
    updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg('id')
RETURNING *;

-- name: SetIndexerProxyID :exec
-- Used by PATCH /api/v1/indexers/{id} to attach (or clear, when the
-- value is NULL) a proxy. We can't fold this into PatchIndexer with
-- COALESCE alone because "no change" and "explicit clear" need to be
-- distinguishable, and our other patch fields use COALESCE-on-NULL.
UPDATE indexers
SET proxy_id = ?,
    updated_at = CURRENT_TIMESTAMP
WHERE id = ?;

-- name: DeleteIndexer :exec
DELETE FROM indexers WHERE id = ?;

-- name: UpsertIndexerHealth :exec
INSERT INTO indexer_health (indexer_id, status, last_checked_at, last_success_at, latency_ms, last_error)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(indexer_id) DO UPDATE SET
    status          = excluded.status,
    last_checked_at = excluded.last_checked_at,
    last_success_at = excluded.last_success_at,
    latency_ms      = excluded.latency_ms,
    last_error      = excluded.last_error;

-- name: GetIndexerHealth :one
SELECT * FROM indexer_health WHERE indexer_id = ? LIMIT 1;

-- name: ListIndexerHealth :many
SELECT * FROM indexer_health;
