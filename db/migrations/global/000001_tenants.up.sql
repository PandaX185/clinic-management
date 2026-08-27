-- Schema v2 — Global schema (applied once per database).
-- Replaces the v1 lineage: users are phone-first (E.164), tenants gain
-- configuration, audit/notification tables are gone.

CREATE TABLE users (
    id            UUID PRIMARY KEY DEFAULT uuid_generate_v7(),
    phone         VARCHAR(20)  NOT NULL UNIQUE, -- E.164, e.g. +201****5678
    password_hash VARCHAR(255) NOT NULL,
    full_name     VARCHAR(255) NOT NULL,
    status        VARCHAR(20)  NOT NULL DEFAULT 'active'
                  CHECK (status IN ('active', 'deactivated')),
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE TABLE user_refresh_tokens (
    id               UUID PRIMARY KEY DEFAULT uuid_generate_v7(),
    user_id          UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash       VARCHAR(64) NOT NULL UNIQUE,
    expires_at       TIMESTAMPTZ NOT NULL,
    revoked          BOOLEAN     NOT NULL DEFAULT FALSE,
    replaced_by_hash VARCHAR(64),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_user_refresh_tokens_user_id ON user_refresh_tokens(user_id);

CREATE TABLE tenants (
    id         UUID PRIMARY KEY DEFAULT uuid_generate_v7(),
    name       VARCHAR(255) NOT NULL,
    slug       VARCHAR(63)  NOT NULL UNIQUE CHECK (slug ~ '^[a-z][a-z0-9_]{0,62}$'),
    status     VARCHAR(20)  NOT NULL DEFAULT 'active'
               CHECK (status IN ('active', 'inactive')),
    created_at TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE TABLE tenant_configs (
    id            UUID PRIMARY KEY DEFAULT uuid_generate_v7(),
    tenant_id     UUID        NOT NULL UNIQUE REFERENCES tenants(id) ON DELETE CASCADE,
    language      VARCHAR(10) NOT NULL DEFAULT 'en',
    timezone      VARCHAR(64) NOT NULL DEFAULT 'UTC',
    opening_hours JSONB       NOT NULL DEFAULT '{}',
    settings      JSONB       NOT NULL DEFAULT '{}',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Seed the default clinic that existing single-tenant data migrates into.
INSERT INTO tenants (name, slug) VALUES ('Default Clinic', 'default')
ON CONFLICT (slug) DO UPDATE SET slug = EXCLUDED.slug;