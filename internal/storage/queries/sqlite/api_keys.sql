-- name: CreateAPIKey :one
INSERT INTO api_keys (user_id, name, key_hash, prefix, expires_at)
VALUES (?, ?, ?, ?, ?)
RETURNING *;

-- name: GetAPIKeyByHash :one
SELECT * FROM api_keys WHERE key_hash = ? LIMIT 1;

-- name: ListAPIKeysForUser :many
SELECT * FROM api_keys WHERE user_id = ? ORDER BY created_at DESC;

-- name: RevokeAPIKey :exec
DELETE FROM api_keys WHERE id = ? AND user_id = ?;

-- name: TouchAPIKey :exec
UPDATE api_keys SET last_used_at = CURRENT_TIMESTAMP WHERE id = ?;

-- name: GetAPIKeyByID :one
SELECT * FROM api_keys WHERE id = ? LIMIT 1;
