-- name: CreateProxy :one
INSERT INTO proxies (id, kind, name, enabled, config_json, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
RETURNING *;

-- name: GetProxy :one
SELECT * FROM proxies WHERE id = $1 LIMIT 1;

-- name: ListProxies :many
SELECT * FROM proxies ORDER BY name ASC;

-- name: ReplaceProxy :one
UPDATE proxies
SET kind        = $2,
    name        = $3,
    enabled     = $4,
    config_json = $5,
    updated_at  = NOW()
WHERE id = $1
RETURNING *;

-- name: PatchProxy :one
UPDATE proxies
SET kind        = COALESCE(sqlc.narg('kind'),        kind),
    name        = COALESCE(sqlc.narg('name'),        name),
    enabled     = COALESCE(sqlc.narg('enabled'),     enabled),
    config_json = COALESCE(sqlc.narg('config_json'), config_json),
    updated_at  = NOW()
WHERE id = sqlc.arg('id')
RETURNING *;

-- name: DeleteProxy :exec
DELETE FROM proxies WHERE id = $1;

-- name: ListIndexerIDsByProxyID :many
SELECT id FROM indexers WHERE proxy_id = $1;
