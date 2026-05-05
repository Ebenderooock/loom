-- name: CreateUser :one
INSERT INTO users (username, password_hash, email, role)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetUserByUsername :one
SELECT * FROM users WHERE username = $1 LIMIT 1;

-- name: GetUserByID :one
SELECT * FROM users WHERE id = $1 LIMIT 1;

-- name: CountUsers :one
SELECT COUNT(*) FROM users;

-- name: UpdateUserPassword :exec
UPDATE users SET password_hash = $1, updated_at = NOW() WHERE id = $2;

-- name: UpdateUserAdmin :exec
UPDATE users SET username = $1, password_hash = $2, updated_at = NOW() WHERE id = $3;

-- name: UpdateUserOIDC :one
UPDATE users
SET email = $1, role = $2, updated_at = NOW()
WHERE id = $3
RETURNING *;
