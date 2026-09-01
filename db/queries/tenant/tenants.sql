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

-- User-tenancy membership index (global) ------------------------------

CREATE TABLE user_tenants (
    user_id   UUID NOT NULL REFERENCES public.users(id)   ON DELETE CASCADE,
    tenant_id UUID NOT NULL REFERENCES public.tenants(id) ON DELETE CASCADE,
    PRIMARY KEY (user_id, tenant_id)
);

CREATE INDEX idx_user_tenants_tenant ON user_tenants(tenant_id);

-- name: EnsureUserTenantMembership :exec
INSERT INTO user_tenants (user_id, tenant_id)
VALUES ($1, $2)
ON CONFLICT (user_id, tenant_id) DO NOTHING;

-- name: ListUserTenantIDs :many
SELECT tenant_id FROM user_tenants WHERE user_id = $1;