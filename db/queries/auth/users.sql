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

-- Staff bindings ---

-- name: AddUserTenant :one
INSERT INTO user_tenants (user_id, tenant_id)
VALUES ($1, $2)
ON CONFLICT (user_id, tenant_id) DO NOTHING
RETURNING *;

-- name: ListTenantsForUser :many
SELECT t.* FROM tenants t
JOIN user_tenants ut ON ut.tenant_id = t.id
WHERE ut.user_id = $1 AND ut.is_active = true AND t.status = 'active';

-- name: DeactivateUserTenant :exec
UPDATE user_tenants SET is_active = false WHERE user_id = $1 AND tenant_id = $2;