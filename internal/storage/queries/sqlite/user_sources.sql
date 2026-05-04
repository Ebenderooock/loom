-- name: CreateUserSource :one
INSERT INTO user_sources (id, name, type, enabled, config, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
RETURNING *;

-- name: GetUserSource :one
SELECT * FROM user_sources WHERE id = ? LIMIT 1;

-- name: GetUserSourceByName :one
SELECT * FROM user_sources WHERE name = ? LIMIT 1;

-- name: ListUserSources :many
SELECT * FROM user_sources ORDER BY created_at DESC;

-- name: ListUserSourcesByType :many
SELECT * FROM user_sources WHERE type = ? ORDER BY created_at DESC;

-- name: ListEnabledUserSources :many
SELECT * FROM user_sources WHERE enabled = 1 ORDER BY created_at DESC;

-- name: UpdateUserSource :one
UPDATE user_sources
SET name        = ?,
    type        = ?,
    enabled     = ?,
    config      = ?,
    updated_at  = CURRENT_TIMESTAMP
WHERE id = ?
RETURNING *;

-- name: PatchUserSource :one
UPDATE user_sources
SET name        = COALESCE(sqlc.narg('name'),    name),
    type        = COALESCE(sqlc.narg('type'),    type),
    enabled     = COALESCE(sqlc.narg('enabled'), enabled),
    config      = COALESCE(sqlc.narg('config'),  config),
    updated_at  = CURRENT_TIMESTAMP
WHERE id = sqlc.arg('id')
RETURNING *;

-- name: UpdateUserSourceLastSync :one
UPDATE user_sources
SET last_sync_at = CURRENT_TIMESTAMP
WHERE id = ?
RETURNING *;

-- name: DeleteUserSource :exec
DELETE FROM user_sources WHERE id = ?;

-- name: CountUserSources :one
SELECT COUNT(*) FROM user_sources;
