-- Global identity extension for multi-tenancy. sqlc reads the real migration
-- files (see sqlc.yaml); this file exists so the tenants table is part of
-- the same migration lineage applied by golang-migrate.

CREATE TABLE tenants (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v7(),
    name VARCHAR(255) NOT NULL,
    slug VARCHAR(63) NOT NULL UNIQUE CHECK (slug ~ '^[a-z][a-z0-9_]{0,62}$'),
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TRIGGER trg_tenants_updated_at BEFORE UPDATE ON tenants
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Staff/doctor bindings: which clinics appear in a user's tenant list at
-- login. Patients are not listed here — they act in any active clinic and
-- get an auto-provisioned per-tenant profile instead.
CREATE TABLE user_tenants (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v7(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, tenant_id)
);

CREATE INDEX idx_user_tenants_user ON user_tenants(user_id);

-- Seed the default clinic that existing single-tenant data migrates into.
INSERT INTO tenants (name, slug) VALUES ('Default Clinic', 'default')
ON CONFLICT (slug) DO NOTHING;

-- sqlc visibility only: profiles physically live in each tenant schema
-- (db/migrations/tenant/000002_profiles.up.sql). Declared here so generated
-- code type-checks; identical column shape.
-- (sqlc parses this file but the table is schema-qualified at runtime via
-- search_path, so no name collision occurs.)
