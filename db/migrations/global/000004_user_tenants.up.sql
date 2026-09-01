-- Global membership index: which users belong to which tenants.
-- This is the source of truth for "my clinics" (/tenants/mine, /auth/tenants)
-- and is maintained alongside profile creation in each tenant schema.
CREATE TABLE user_tenants (
    user_id   UUID NOT NULL REFERENCES public.users(id)   ON DELETE CASCADE,
    tenant_id UUID NOT NULL REFERENCES public.tenants(id) ON DELETE CASCADE,
    PRIMARY KEY (user_id, tenant_id)
);

CREATE INDEX idx_user_tenants_tenant ON user_tenants(tenant_id);
