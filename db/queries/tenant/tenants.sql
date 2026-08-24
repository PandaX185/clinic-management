-- name: CreateTenant :one
INSERT INTO tenants (name, slug)
VALUES ($1, $2)
RETURNING *;

-- name: GetTenantBySlug :one
SELECT * FROM tenants WHERE slug = $1;

-- name: GetTenantByID :one
SELECT * FROM tenants WHERE id = $1;

-- name: ListTenants :many
SELECT * FROM tenants ORDER BY created_at DESC;

-- name: SetTenantActive :exec
UPDATE tenants SET is_active = $2 WHERE id = $1;

-- name: AddTenantMember :one
INSERT INTO user_tenants (user_id, tenant_id, role_name)
VALUES ($1, $2, $3)
ON CONFLICT (user_id, tenant_id) DO UPDATE
  SET role_name = EXCLUDED.role_name, is_active = true
RETURNING *;

-- name: ListMembershipsForUser :many
SELECT ut.user_id, ut.tenant_id, ut.role_name, ut.is_active,
       t.slug AS tenant_slug, t.name AS tenant_name
FROM user_tenants ut
JOIN tenants t ON t.id = ut.tenant_id
WHERE ut.user_id = $1 AND ut.is_active = true AND t.is_active = true;

-- name: GetMembership :one
SELECT ut.user_id, ut.tenant_id, ut.role_name, ut.is_active,
       t.slug AS tenant_slug, t.name AS tenant_name
FROM user_tenants ut
JOIN tenants t ON t.id = ut.tenant_id
WHERE ut.user_id = $1 AND ut.tenant_id = $2 AND ut.is_active = true;
