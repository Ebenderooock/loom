-- name: CreateProxy :one
INSERT INTO proxies (id, kind, name, enabled, config_json, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
RETURNING *;

-- name: GetProxy :one
SELECT * FROM proxies WHERE id = ? LIMIT 1;

-- name: ListProxies :many
SELECT * FROM proxies ORDER BY name ASC;

-- name: ReplaceProxy :one
UPDATE proxies
SET kind        = ?,
    name        = ?,
    enabled     = ?,
    config_json = ?,
    updated_at  = CURRENT_TIMESTAMP
WHERE id = ?
RETURNING *;

-- name: PatchProxy :one
UPDATE proxies
SET kind        = COALESCE(sqlc.narg('kind'),        kind),
    name        = COALESCE(sqlc.narg('name'),        name),
    enabled     = COALESCE(sqlc.narg('enabled'),     enabled),
    config_json = COALESCE(sqlc.narg('config_json'), config_json),
    updated_at  = CURRENT_TIMESTAMP
WHERE id = sqlc.arg('id')
RETURNING *;

-- name: DeleteProxy :exec
DELETE FROM proxies WHERE id = ?;

-- name: ListIndexerIDsByProxyID :many
SELECT id FROM indexers WHERE proxy_id = ?;
