-- name: CreateDownloadClient :one
INSERT INTO download_clients (
    id, name, kind, protocol, enabled, priority,
    host, port, tls, username, password,
    config_json,
    category_default, save_path_default,
    remove_completed, remove_failed,
    created_at, updated_at
)
VALUES (
    $1, $2, $3, $4, $5, $6,
    $7, $8, $9, $10, $11,
    $12,
    $13, $14,
    $15, $16,
    NOW(), NOW()
)
RETURNING *;

-- name: GetDownloadClient :one
SELECT * FROM download_clients WHERE id = $1 LIMIT 1;

-- name: ListDownloadClients :many
SELECT * FROM download_clients ORDER BY priority ASC, name ASC;

-- name: ListEnabledDownloadClients :many
SELECT * FROM download_clients WHERE enabled = TRUE ORDER BY priority ASC, name ASC;

-- name: ReplaceDownloadClient :one
UPDATE download_clients
SET name              = $2,
    kind              = $3,
    protocol          = $4,
    enabled           = $5,
    priority          = $6,
    host              = $7,
    port              = $8,
    tls               = $9,
    username          = $10,
    password          = $11,
    config_json       = $12,
    category_default  = $13,
    save_path_default = $14,
    remove_completed  = $15,
    remove_failed     = $16,
    updated_at        = NOW()
WHERE id = $1
RETURNING *;

-- name: PatchDownloadClient :one
UPDATE download_clients
SET name              = COALESCE(sqlc.narg('name'), name),
    enabled           = COALESCE(sqlc.narg('enabled'), enabled),
    priority          = COALESCE(sqlc.narg('priority'), priority),
    category_default  = COALESCE(sqlc.narg('category_default'), category_default),
    save_path_default = COALESCE(sqlc.narg('save_path_default'), save_path_default),
    remove_completed  = COALESCE(sqlc.narg('remove_completed'), remove_completed),
    remove_failed     = COALESCE(sqlc.narg('remove_failed'), remove_failed),
    updated_at        = NOW()
WHERE id = $1
RETURNING *;

-- name: DeleteDownloadClient :exec
DELETE FROM download_clients WHERE id = $1;

-- name: UpsertDownloadClientHealth :exec
INSERT INTO download_client_health (
    client_id, status,
    last_checked_at, last_success_at, last_failure_at,
    last_error, consecutive_failures,
    last_free_space_bytes, last_categories_json
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT(client_id) DO UPDATE SET
    status                = EXCLUDED.status,
    last_checked_at       = EXCLUDED.last_checked_at,
    last_success_at       = EXCLUDED.last_success_at,
    last_failure_at       = EXCLUDED.last_failure_at,
    last_error            = EXCLUDED.last_error,
    consecutive_failures  = EXCLUDED.consecutive_failures,
    last_free_space_bytes = EXCLUDED.last_free_space_bytes,
    last_categories_json  = EXCLUDED.last_categories_json;

-- name: GetDownloadClientHealth :one
SELECT * FROM download_client_health WHERE client_id = $1 LIMIT 1;

-- name: ListDownloadClientHealth :many
SELECT * FROM download_client_health;
