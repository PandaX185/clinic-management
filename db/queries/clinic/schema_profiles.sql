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

-- name: CountProfilesByRole :one
SELECT COUNT(*)
FROM profiles p
JOIN profile_roles pr ON pr.profile_id = p.id
JOIN roles r ON r.id = pr.role_id AND r.name = $1
WHERE p.status = 'active';

-- name: ListProfilesByRolePaginated :many
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
WHERE p.status = 'active'
ORDER BY p.display_name
LIMIT $2 OFFSET $3;

-- name: ProfileHasRole :one
SELECT EXISTS (
    SELECT 1
    FROM profile_roles pr
    JOIN roles r ON r.id = pr.role_id
    WHERE pr.profile_id = $1 AND r.name = $2
);

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

-- Doctor schedules ----------------------------------------------------

CREATE TABLE doctor_schedules (
    id                UUID PRIMARY KEY DEFAULT uuid_generate_v7(),
    doctor_profile_id UUID        NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
    day_of_week       SMALLINT    NOT NULL CHECK (day_of_week BETWEEN 0 AND 6),
    start_time        TIME        NOT NULL,
    end_time          TIME        NOT NULL CHECK (end_time > start_time),
    slot_duration     INT         NOT NULL DEFAULT 30 CHECK (slot_duration > 0),
    is_active         BOOLEAN     NOT NULL DEFAULT true,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (doctor_profile_id, day_of_week, start_time, end_time)
);

CREATE INDEX idx_doctor_schedules_doctor ON doctor_schedules(doctor_profile_id);
CREATE INDEX idx_doctor_schedules_day ON doctor_schedules(day_of_week);

-- name: ListDoctorSchedulesOnDay :many
-- Active schedule windows for an active doctor on a given week day (0 = Sunday).
SELECT s.id, s.doctor_profile_id, s.day_of_week, s.start_time, s.end_time,
    s.is_active, s.created_at, s.updated_at
FROM doctor_schedules s
WHERE s.doctor_profile_id = $1
  AND s.is_active = true
  AND s.day_of_week = EXTRACT(DOW FROM sqlc.arg('date')::timestamptz)::int
  AND EXISTS (SELECT 1 FROM profiles p WHERE p.id = s.doctor_profile_id AND p.status = 'active')
ORDER BY s.start_time;

-- name: ListDoctorSchedules :many
-- Every weekly window for a doctor, active or not, newest-edited last.
SELECT id, doctor_profile_id, day_of_week, start_time, end_time, slot_duration, is_active, created_at, updated_at
FROM doctor_schedules
WHERE doctor_profile_id = $1
ORDER BY day_of_week, start_time;

-- name: DeleteDoctorSchedules :exec
-- Removes every weekly window for a doctor, so a PUT can replace the set.
DELETE FROM doctor_schedules WHERE doctor_profile_id = $1;

-- name: InsertDoctorSchedule :one
INSERT INTO doctor_schedules (doctor_profile_id, day_of_week, start_time, end_time, slot_duration, is_active)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, doctor_profile_id, day_of_week, start_time, end_time, slot_duration, is_active, created_at, updated_at;

-- Schedule exceptions ----------------------------------------------------

CREATE TABLE schedule_exceptions (
    id                UUID PRIMARY KEY DEFAULT uuid_generate_v7(),
    doctor_profile_id UUID     NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
    date              DATE     NOT NULL,
    start_time        TIME,
    end_time          TIME,
    type              VARCHAR(20) NOT NULL
        CHECK (type IN ('leave', 'holiday', 'unavailable', 'extra_hours')),
    reason            TEXT,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT valid_exception_hours CHECK (
        type = 'extra_hours' OR (start_time IS NOT NULL AND end_time IS NOT NULL AND end_time > start_time)
    )
);

CREATE INDEX idx_schedule_exceptions_doctor_date ON schedule_exceptions(doctor_profile_id, date);

-- name: ListScheduleExceptions :many
SELECT id, doctor_profile_id, date, start_time, end_time, type, reason, created_at, updated_at
FROM schedule_exceptions
WHERE doctor_profile_id = $1
ORDER BY date, start_time;

-- name: ListScheduleExceptionsForDate :many
SELECT id, doctor_profile_id, date, start_time, end_time, type, reason, created_at, updated_at
FROM schedule_exceptions
WHERE doctor_profile_id = $1 AND date = $2
ORDER BY start_time;

-- name: CreateScheduleException :one
INSERT INTO schedule_exceptions (doctor_profile_id, date, start_time, end_time, type, reason)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, doctor_profile_id, date, start_time, end_time, type, reason, created_at, updated_at;

-- name: DeleteScheduleException :exec
DELETE FROM schedule_exceptions WHERE id = $1;

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