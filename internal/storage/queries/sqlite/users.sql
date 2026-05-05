-- name: CreateUser :one
INSERT INTO users (username, password_hash, email, role)
VALUES (?, ?, ?, ?)
RETURNING *;

-- name: GetUserByUsername :one
SELECT * FROM users WHERE username = ? LIMIT 1;

-- name: GetUserByID :one
SELECT * FROM users WHERE id = ? LIMIT 1;

-- name: CountUsers :one
SELECT COUNT(*) FROM users;

-- name: UpdateUserPassword :exec
UPDATE users SET password_hash = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?;

-- name: UpdateUserAdmin :exec
UPDATE users SET username = ?, password_hash = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?;

-- name: UpdateUserOIDC :one
UPDATE users
SET email = ?, role = ?, updated_at = CURRENT_TIMESTAMP
WHERE id = ?
RETURNING *;
