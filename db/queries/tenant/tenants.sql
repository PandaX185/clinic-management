-- name: CreateTenant :one
INSERT INTO tenants (name, slug)
VALUES ($1, $2)
RETURNING *;

-- name: GetTenantBySlug :one
SELECT * FROM tenants WHERE slug = $1;

-- name: GetTenantByID :one
SELECT * FROM tenants WHERE id = $1;

-- name: ListTenants :many
SELECT * FROM tenants WHERE is_active = true ORDER BY created_at DESC;

-- name: SetTenantActive :exec
UPDATE tenants SET is_active = $2 WHERE id = $1;

-- name: AddUserTenant :one
-- Staff/doctor binding so the clinic appears in the user's tenant list.
INSERT INTO user_tenants (user_id, tenant_id)
VALUES ($1, $2)
ON CONFLICT (user_id, tenant_id) DO NOTHING
RETURNING *;

-- name: ListTenantsForUser :many
-- Clinics where the user holds a staff/doctor binding. Patients browse all
-- active tenants and need no binding to act inside one.
SELECT t.* FROM tenants t
JOIN user_tenants ut ON ut.tenant_id = t.id
WHERE ut.user_id = $1 AND ut.is_active = true AND t.is_active = true;

-- GetProfileForTenant / UpsertPatientProfile live in profile.sql and run
-- with search_path pinned to the active tenant schema.

-- name: GetProfileForTenant :one
SELECT user_id, role, is_active FROM profiles WHERE user_id = $1;

-- name: UpsertPatientProfile :one
INSERT INTO profiles (user_id, role)
VALUES ($1, 'patient')
ON CONFLICT (user_id) DO UPDATE SET is_active = true
RETURNING user_id, role, is_active;
