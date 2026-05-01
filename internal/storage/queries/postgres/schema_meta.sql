-- name: GetSchemaMeta :one
SELECT value FROM schema_meta WHERE key = $1 LIMIT 1;

-- name: SetSchemaMeta :exec
INSERT INTO schema_meta (key, value) VALUES ($1, $2)
ON CONFLICT(key) DO UPDATE SET value = excluded.value;
