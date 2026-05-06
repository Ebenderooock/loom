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
    ?, ?, ?, ?, ?, ?,
    ?, ?, ?, ?, ?,
    ?,
    ?, ?,
    ?, ?,
    CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
)
RETURNING *;

-- name: GetDownloadClient :one
SELECT * FROM download_clients WHERE id = ? LIMIT 1;

-- name: ListDownloadClients :many
SELECT * FROM download_clients ORDER BY priority ASC, name ASC;

-- name: ListEnabledDownloadClients :many
SELECT * FROM download_clients WHERE enabled = 1 ORDER BY priority ASC, name ASC;

-- name: ReplaceDownloadClient :one
UPDATE download_clients
SET name              = ?,
    kind              = ?,
    protocol          = ?,
    enabled           = ?,
    priority          = ?,
    host              = ?,
    port              = ?,
    tls               = ?,
    username          = ?,
    password          = ?,
    config_json       = ?,
    category_default  = ?,
    save_path_default = ?,
    remove_completed  = ?,
    remove_failed     = ?,
    updated_at        = CURRENT_TIMESTAMP
WHERE id = ?
RETURNING *;

-- name: PatchDownloadClient :one
UPDATE download_clients
SET name              = COALESCE(sqlc.narg('name'), name),
    enabled           = COALESCE(sqlc.narg('enabled'), enabled),
    priority          = COALESCE(sqlc.narg('priority'), priority),
    host              = COALESCE(sqlc.narg('host'), host),
    port              = COALESCE(sqlc.narg('port'), port),
    tls               = COALESCE(sqlc.narg('tls'), tls),
    username          = COALESCE(sqlc.narg('username'), username),
    password          = COALESCE(sqlc.narg('password'), password),
    config_json       = COALESCE(sqlc.narg('config_json'), config_json),
    category_default  = COALESCE(sqlc.narg('category_default'), category_default),
    save_path_default = COALESCE(sqlc.narg('save_path_default'), save_path_default),
    remove_completed  = COALESCE(sqlc.narg('remove_completed'), remove_completed),
    remove_failed     = COALESCE(sqlc.narg('remove_failed'), remove_failed),
    updated_at        = CURRENT_TIMESTAMP
WHERE id = sqlc.arg('id')
RETURNING *;

-- name: DeleteDownloadClient :exec
DELETE FROM download_clients WHERE id = ?;

-- name: UpsertDownloadClientHealth :exec
INSERT INTO download_client_health (
    client_id, status,
    last_checked_at, last_success_at, last_failure_at,
    last_error, consecutive_failures,
    last_free_space_bytes, last_categories_json
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(client_id) DO UPDATE SET
    status                = excluded.status,
    last_checked_at       = excluded.last_checked_at,
    last_success_at       = excluded.last_success_at,
    last_failure_at       = excluded.last_failure_at,
    last_error            = excluded.last_error,
    consecutive_failures  = excluded.consecutive_failures,
    last_free_space_bytes = excluded.last_free_space_bytes,
    last_categories_json  = excluded.last_categories_json;

-- name: GetDownloadClientHealth :one
SELECT * FROM download_client_health WHERE client_id = ? LIMIT 1;

-- name: ListDownloadClientHealth :many
SELECT * FROM download_client_health;
