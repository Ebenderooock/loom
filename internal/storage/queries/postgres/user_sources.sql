-- name: CreateUserSource :one
INSERT INTO user_sources (id, name, type, enabled, config, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
RETURNING *;

-- name: GetUserSource :one
SELECT * FROM user_sources WHERE id = $1 LIMIT 1;

-- name: GetUserSourceByName :one
SELECT * FROM user_sources WHERE name = $1 LIMIT 1;

-- name: ListUserSources :many
SELECT * FROM user_sources ORDER BY created_at DESC;

-- name: ListUserSourcesByType :many
SELECT * FROM user_sources WHERE type = $1 ORDER BY created_at DESC;

-- name: ListEnabledUserSources :many
SELECT * FROM user_sources WHERE enabled = true ORDER BY created_at DESC;

-- name: UpdateUserSource :one
UPDATE user_sources
SET name        = $2,
    type        = $3,
    enabled     = $4,
    config      = $5,
    updated_at  = NOW()
WHERE id = $1
RETURNING *;

-- name: PatchUserSource :one
UPDATE user_sources
SET name        = COALESCE(sqlc.narg('name'),    name),
    type        = COALESCE(sqlc.narg('type'),    type),
    enabled     = COALESCE(sqlc.narg('enabled'), enabled),
    config      = COALESCE(sqlc.narg('config'),  config),
    updated_at  = NOW()
WHERE id = sqlc.arg('id')
RETURNING *;

-- name: UpdateUserSourceLastSync :one
UPDATE user_sources
SET last_sync_at = NOW()
WHERE id = $1
RETURNING *;

-- name: DeleteUserSource :exec
DELETE FROM user_sources WHERE id = $1;

-- name: CountUserSources :one
SELECT COUNT(*) FROM user_sources;
