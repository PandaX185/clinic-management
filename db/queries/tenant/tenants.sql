-- Tenants (global registry) — schema v2

-- name: CreateTenant :one
INSERT INTO tenants (name, slug)
VALUES ($1, $2)
RETURNING *;

-- name: GetTenantBySlug :one
SELECT * FROM tenants WHERE slug = $1;

-- name: GetTenantByID :one
SELECT * FROM tenants WHERE id = $1;

-- name: ListTenants :many
SELECT * FROM tenants WHERE status = 'active' ORDER BY created_at DESC;

-- name: SetTenantActive :exec
UPDATE tenants SET status = CASE WHEN $2 THEN 'active' ELSE 'inactive' END WHERE id = $1;