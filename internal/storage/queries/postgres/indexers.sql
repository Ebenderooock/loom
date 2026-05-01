-- name: CreateIndexer :one
INSERT INTO indexers (id, kind, name, enabled, priority, config_json, categories_json, tags_json, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW(), NOW())
RETURNING *;

-- name: GetIndexer :one
SELECT * FROM indexers WHERE id = $1 LIMIT 1;

-- name: ListIndexers :many
SELECT * FROM indexers ORDER BY priority ASC, name ASC;

-- name: ListEnabledIndexers :many
SELECT * FROM indexers WHERE enabled = TRUE ORDER BY priority ASC, name ASC;

-- name: ReplaceIndexer :one
UPDATE indexers
SET kind            = $2,
    name            = $3,
    enabled         = $4,
    priority        = $5,
    config_json     = $6,
    categories_json = $7,
    tags_json       = $8,
    updated_at      = NOW()
WHERE id = $1
RETURNING *;

-- name: PatchIndexer :one
UPDATE indexers
SET name      = COALESCE(sqlc.narg('name'), name),
    enabled   = COALESCE(sqlc.narg('enabled'), enabled),
    priority  = COALESCE(sqlc.narg('priority'), priority),
    tags_json = COALESCE(sqlc.narg('tags_json'), tags_json),
    updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: DeleteIndexer :exec
DELETE FROM indexers WHERE id = $1;

-- name: UpsertIndexerHealth :exec
INSERT INTO indexer_health (indexer_id, status, last_checked_at, last_success_at, latency_ms, last_error)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT(indexer_id) DO UPDATE SET
    status          = EXCLUDED.status,
    last_checked_at = EXCLUDED.last_checked_at,
    last_success_at = EXCLUDED.last_success_at,
    latency_ms      = EXCLUDED.latency_ms,
    last_error      = EXCLUDED.last_error;

-- name: GetIndexerHealth :one
SELECT * FROM indexer_health WHERE indexer_id = $1 LIMIT 1;

-- name: ListIndexerHealth :many
SELECT * FROM indexer_health;
