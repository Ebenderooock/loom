-- name: UpdateIndexerCapsCache :exec
UPDATE indexer_health
SET    last_caps_json = $1
WHERE  indexer_id     = $2;

-- name: GetIndexerCapsCache :one
SELECT last_caps_json
FROM   indexer_health
WHERE  indexer_id = $1
LIMIT  1;
