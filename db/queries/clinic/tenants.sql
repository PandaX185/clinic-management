-- Tenants (global registry) — schema v2

-- name: CreateClinic :one
INSERT INTO tenants (name, slug)
VALUES ($1, $2)
RETURNING *;

-- name: GetClinicBySlug :one
SELECT * FROM tenants WHERE slug = $1;

-- name: GetClinicByID :one
SELECT * FROM tenants WHERE id = $1;

-- name: ListClinics :many
SELECT * FROM tenants WHERE status = 'active' ORDER BY created_at DESC;

-- name: CountActiveClinics :one
SELECT COUNT(*) FROM tenants WHERE status = 'active';

-- name: ListClinicsPaginated :many
SELECT * FROM tenants WHERE status = 'active' ORDER BY created_at DESC LIMIT $1 OFFSET $2;

-- name: SetClinicActive :exec
UPDATE tenants SET status = CASE WHEN $2 THEN 'active' ELSE 'inactive' END WHERE id = $1;

-- User-tenancy membership index (global) ------------------------------

CREATE TABLE user_tenants (
    user_id   UUID NOT NULL REFERENCES public.users(id)   ON DELETE CASCADE,
    tenant_id UUID NOT NULL REFERENCES public.tenants(id) ON DELETE CASCADE,
    PRIMARY KEY (user_id, tenant_id)
);

CREATE INDEX idx_user_tenants_tenant ON user_tenants(tenant_id);

-- name: EnsureUserClinicMembership :exec
INSERT INTO user_tenants (user_id, tenant_id)
VALUES ($1, $2)
ON CONFLICT (user_id, tenant_id) DO NOTHING;

-- name: ListUserClinicIDs :many
SELECT tenant_id FROM user_tenants WHERE user_id = $1;