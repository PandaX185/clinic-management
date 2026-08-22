-- name: CreateUser :one
INSERT INTO users (
    email, password_hash, full_name, phone
) VALUES (
    $1, $2, $3, $4
) RETURNING id, email, password_hash, full_name, phone, is_active, email_verified, created_at, updated_at;

-- name: GetUserByEmail :one
SELECT id, email, password_hash, full_name, phone, is_active, email_verified, created_at, updated_at
FROM users
WHERE email = $1;

-- name: GetUserByID :one
SELECT id, email, password_hash, full_name, phone, is_active, email_verified, created_at, updated_at
FROM users
WHERE id = $1;

-- name: UpdateUser :one
UPDATE users
SET full_name = $2, phone = $3, is_active = $3, email_verified = $4, updated_at = now()
WHERE id = $1
RETURNING id, email, password_hash, full_name, phone, is_active, email_verified, created_at, updated_at;

-- name: UpdateUserPassword :exec
UPDATE users
SET password_hash = $2, updated_at = now()
WHERE id = $1;

-- name: VerifyUserEmail :exec
UPDATE users
SET email_verified = true, updated_at = now()
WHERE id = $1;

-- name: AssignUserRole :exec
INSERT INTO user_roles (user_id, role_id)
VALUES ($1, $2)
ON CONFLICT DO NOTHING;

-- name: GetUserRoles :many
SELECT r.id, r.name, r.description
FROM roles r
JOIN user_roles ur ON r.id = ur.role_id
WHERE ur.user_id = $1;

-- name: GetRoleByName :one
SELECT id, name, description, created_at
FROM roles
WHERE name = $1;