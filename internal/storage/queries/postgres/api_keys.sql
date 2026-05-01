-- name: CreateAPIKey :one
INSERT INTO api_keys (user_id, name, key_hash, prefix, expires_at)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetAPIKeyByHash :one
SELECT * FROM api_keys WHERE key_hash = $1 LIMIT 1;

-- name: ListAPIKeysForUser :many
SELECT * FROM api_keys WHERE user_id = $1 ORDER BY created_at DESC;

-- name: RevokeAPIKey :exec
DELETE FROM api_keys WHERE id = $1 AND user_id = $2;

-- name: TouchAPIKey :exec
UPDATE api_keys SET last_used_at = NOW() WHERE id = $1;

-- name: GetAPIKeyByID :one
SELECT * FROM api_keys WHERE id = $1 LIMIT 1;
