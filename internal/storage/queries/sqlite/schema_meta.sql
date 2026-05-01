-- name: GetSchemaMeta :one
SELECT value FROM schema_meta WHERE key = ? LIMIT 1;

-- name: SetSchemaMeta :exec
INSERT INTO schema_meta (key, value) VALUES (?, ?)
ON CONFLICT(key) DO UPDATE SET value = excluded.value;
