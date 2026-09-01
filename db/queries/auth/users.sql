-- Auth (schema v2: phone-only identity)

-- name: GetUserByPhone :one
SELECT * FROM users WHERE phone = $1;

-- name: GetUserByID :one
SELECT * FROM users WHERE id = $1;

-- name: GetUserAdminFlag :one
SELECT is_admin FROM users WHERE id = $1;

-- name: CreateUser :one
INSERT INTO users (phone, password_hash, full_name)
VALUES ($1, $2, $3)
RETURNING *;

-- name: UpdateUserStatus :exec
UPDATE users SET status = $2 WHERE id = $1;

-- Refresh tokens (hashed storage for validation/revocation)

-- name: InsertRefreshToken :exec
INSERT INTO user_refresh_tokens (user_id, token_hash, expires_at)
VALUES ($1, $2, $3)
ON CONFLICT (user_id) DO UPDATE SET
    token_hash = EXCLUDED.token_hash,
    expires_at = EXCLUDED.expires_at,
    revoked = false;

-- name: DeleteRefreshToken :exec
DELETE FROM user_refresh_tokens
WHERE user_id = $1 AND token_hash = $2;

-- name: ValidateRefreshToken :one
SELECT expires_at FROM user_refresh_tokens
WHERE user_id = $1 AND token_hash = $2 AND revoked = false AND expires_at > now();