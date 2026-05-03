-- name: UpdateIndexerCapsCache :exec
UPDATE indexer_health
SET    last_caps_json = ?
WHERE  indexer_id     = ?;

-- name: GetIndexerCapsCache :one
SELECT last_caps_json
FROM   indexer_health
WHERE  indexer_id = ?
LIMIT  1;
