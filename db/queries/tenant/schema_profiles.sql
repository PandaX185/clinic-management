-- sqlc-only declarations for tables whose real DDL lives in db/migrations/tenant/ (applied per clinic schema).
-- These are tenant-specific tables NOT in the global schema.

CREATE TABLE profiles (
    id           UUID PRIMARY KEY DEFAULT uuid_generate_v7(),
    user_id      UUID         NOT NULL UNIQUE REFERENCES public.users(id) ON DELETE CASCADE,
    display_name VARCHAR(255) NOT NULL,
    status       VARCHAR(20)  NOT NULL DEFAULT 'active'
                 CHECK (status IN ('active', 'inactive')),
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE INDEX idx_profiles_status ON profiles(status);

-- name: GetProfileByUserID :one
SELECT * FROM profiles WHERE user_id = $1;

-- name: GetProfileByID :one
SELECT * FROM profiles WHERE id = $1;

-- name: UpsertPatientProfile :one
INSERT INTO profiles (user_id, display_name)
VALUES ($1, $2)
ON CONFLICT (user_id) DO UPDATE SET
    display_name = COALESCE(EXCLUDED.display_name, profiles.display_name),
    updated_at = now()
RETURNING *;

-- name: ListProfiles :many
SELECT
    p.id,
    p.user_id,
    p.display_name,
    p.status,
    p.created_at,
    p.updated_at,
    COALESCE(array_agg(r.name ORDER BY r.name) FILTER (WHERE r.name IS NOT NULL), ARRAY[]::varchar[])::text[] AS role_names
FROM profiles p
LEFT JOIN profile_roles pr ON pr.profile_id = p.id
LEFT JOIN roles r ON r.id = pr.role_id
GROUP BY p.id
ORDER BY p.display_name;

-- name: CreateProfile :one
INSERT INTO profiles (user_id, display_name)
VALUES ($1, $2)
RETURNING *;

-- name: ListProfilesByRole :many
SELECT
    p.id,
    p.user_id,
    p.display_name,
    p.status,
    p.created_at,
    p.updated_at
FROM profiles p
JOIN profile_roles pr ON pr.profile_id = p.id
JOIN roles r ON r.id = pr.role_id AND r.name = $1
ORDER BY p.display_name;

-- RBAC (tenant-specific) ---------------------------------------------

CREATE TABLE roles (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v7(),
    name        VARCHAR(50) NOT NULL UNIQUE,
    description TEXT
);

CREATE TABLE permissions (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v7(),
    name        VARCHAR(100) NOT NULL UNIQUE,
    description TEXT
);

CREATE TABLE role_permissions (
    role_id       UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    permission_id UUID NOT NULL REFERENCES permissions(id) ON DELETE CASCADE,
    PRIMARY KEY (role_id, permission_id)
);

CREATE TABLE profile_roles (
    profile_id UUID NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
    role_id    UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    PRIMARY KEY (profile_id, role_id)
);

-- name: ListUserRoles :many
SELECT r.id, r.name
FROM roles r
JOIN profile_roles pr ON pr.role_id = r.id
WHERE pr.profile_id = $1;

-- name: GetRoleByName :one
SELECT * FROM roles WHERE name = $1;

-- name: AssignRoleToProfile :exec
INSERT INTO profile_roles (profile_id, role_id)
VALUES ($1, $2)
ON CONFLICT DO NOTHING;

-- Appointment types ---------------------------------------------------

CREATE TABLE appointment_types (
    id               UUID PRIMARY KEY DEFAULT uuid_generate_v7(),
    name             VARCHAR(100) NOT NULL,
    duration_minutes INT          NOT NULL CHECK (duration_minutes > 0),
    price            DECIMAL(12,2) NOT NULL DEFAULT 0 CHECK (price >= 0),
    color            VARCHAR(20),
    icon             VARCHAR(50),
    is_active        BOOLEAN      NOT NULL DEFAULT true,
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ  NOT NULL DEFAULT now()
);

-- name: ListAppointmentTypes :many
SELECT * FROM appointment_types WHERE is_active = true ORDER BY name;

-- name: GetAppointmentTypeByID :one
SELECT * FROM appointment_types WHERE id = $1;

-- name: CreateAppointmentType :one
INSERT INTO appointment_types (name, duration_minutes, price, color, icon)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: UpdateAppointmentType :one
UPDATE appointment_types
SET name = $2,
    duration_minutes = $3,
    price = $4,
    color = $5,
    icon = $6,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- Appointments --------------------------------------------------------

CREATE TABLE appointments (
    id                  UUID PRIMARY KEY DEFAULT uuid_generate_v7(),
    profile_id          UUID        NOT NULL REFERENCES profiles(id) ON DELETE RESTRICT,
    doctor_profile_id   UUID        NOT NULL REFERENCES profiles(id) ON DELETE RESTRICT,
    appointment_type_id UUID        NOT NULL REFERENCES appointment_types(id) ON DELETE RESTRICT,
    scheduled_start TIMESTAMPTZ NOT NULL,
    scheduled_end   TIMESTAMPTZ NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'scheduled',
    visit_notes     TEXT,
    follow_up_date  DATE,
    cancellation_reason TEXT,
    version     INT NOT NULL DEFAULT 1,
    created_by  UUID REFERENCES public.users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT valid_appointment_range CHECK (scheduled_end > scheduled_start)
);

-- Queue ----------------------------------------------------------------

CREATE TABLE queue_entries (
    id             UUID PRIMARY KEY DEFAULT uuid_generate_v7(),
    appointment_id UUID REFERENCES appointments(id) ON DELETE SET NULL,
    profile_id     UUID        NOT NULL REFERENCES profiles(id) ON DELETE RESTRICT,
    status VARCHAR(20) NOT NULL DEFAULT 'waiting',
    priority      INT NOT NULL DEFAULT 0,
    checked_in_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    called_at     TIMESTAMPTZ,
    started_at    TIMESTAMPTZ,
    completed_at  TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Idempotency --------------------------------------------------------

CREATE TABLE idempotency_keys (
    key             VARCHAR(255) NOT NULL,
    endpoint        VARCHAR(255) NOT NULL,
    user_id         UUID,
    request_hash    VARCHAR(64)  NOT NULL,
    response_status INT          NOT NULL,
    response_body   JSONB,
    expires_at      TIMESTAMPTZ  NOT NULL,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT now(),
    PRIMARY KEY (key, endpoint)
);

-- name: GetIdempotentResponse :one
SELECT key, endpoint, user_id, request_hash, response_status, response_body, expires_at, created_at
FROM idempotency_keys WHERE key = $1 AND endpoint = $2;

-- name: InsertIdempotentResponse :exec
INSERT INTO idempotency_keys (key, endpoint, user_id, request_hash, response_status, response_body, expires_at)
VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: DeleteExpiredIdempotencyKeys :execrows
DELETE FROM idempotency_keys WHERE expires_at < now();