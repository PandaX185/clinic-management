-- Auth (schema v2: phone-only identity)

-- name: GetUserByPhone :one
SELECT * FROM users WHERE phone = $1;

-- name: GetUserByID :one
SELECT * FROM users WHERE id = $1;

-- name: CreateUser :one
INSERT INTO users (phone, password_hash, full_name)
VALUES ($1, $2, $3)
RETURNING *;

-- name: UpdateUserStatus :exec
UPDATE users SET status = $2 WHERE id = $1;