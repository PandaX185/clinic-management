-- sqlc-only declarations for per-tenant tables used by tenant queries.
-- Real DDL: db/migrations/tenant/*.up.sql (profiles).
CREATE TABLE profiles (
    user_id UUID PRIMARY KEY,
    role VARCHAR(50) NOT NULL DEFAULT 'patient',
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

