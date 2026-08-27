-- Schema v2 — Per-tenant clinical schema (applied inside each tenant_<slug>).
-- Concurrency invariants preserved from v1:
--   * no_overlapping_appointments exclusion constraint (btree_gist)
--   * idempotency_keys unique constraint
-- Audit and notification tables intentionally removed.

CREATE TABLE profiles (
    id           UUID PRIMARY KEY DEFAULT uuid_generate_v7(),
    user_id      UUID         NOT NULL UNIQUE REFERENCES public.users(id) ON DELETE CASCADE,
    display_name VARCHAR(255) NOT NULL,
    status       VARCHAR(20)  NOT NULL DEFAULT 'active'
                 CHECK (status IN ('active', 'inactive')),
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE INDEX idx_profiles_role ON profiles(status);

-- RBAC ---------------------------------------------------------------

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


-- Appointments --------------------------------------------------------

CREATE TABLE appointments (
    id                  UUID PRIMARY KEY DEFAULT uuid_generate_v7(),
    profile_id          UUID        NOT NULL REFERENCES profiles(id) ON DELETE RESTRICT,
    doctor_profile_id   UUID        NOT NULL REFERENCES profiles(id) ON DELETE RESTRICT,
    appointment_type_id UUID        NOT NULL REFERENCES appointment_types(id) ON DELETE RESTRICT,

    scheduled_start TIMESTAMPTZ NOT NULL,
    scheduled_end   TIMESTAMPTZ NOT NULL,

    status VARCHAR(20) NOT NULL DEFAULT 'scheduled'
        CHECK (status IN ('scheduled', 'confirmed', 'checked_in', 'in_progress',
                          'completed', 'cancelled', 'no_show')),

    visit_notes     TEXT,
    follow_up_date  DATE,
    cancellation_reason TEXT,

    version     INT NOT NULL DEFAULT 1,
    created_by  UUID REFERENCES public.users(id) ON DELETE SET NULL,

    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT valid_appointment_range CHECK (scheduled_end > scheduled_start)
);

-- BR-01: the database itself rejects overlapping active slots.
ALTER TABLE appointments ADD CONSTRAINT no_overlapping_appointments
    EXCLUDE USING GIST (
        doctor_profile_id WITH =,
        tstzrange(scheduled_start, scheduled_end) WITH &&
    ) WHERE (status IN ('scheduled', 'confirmed', 'checked_in'));

CREATE INDEX idx_appointments_profile ON appointments(profile_id, scheduled_start DESC);
CREATE INDEX idx_appointments_doctor_range ON appointments(doctor_profile_id, scheduled_start)
    WHERE status IN ('scheduled', 'confirmed', 'checked_in');
CREATE INDEX idx_appointments_status_start ON appointments(status, scheduled_start);


-- Queue ----------------------------------------------------------------

CREATE TABLE queue_entries (
    id             UUID PRIMARY KEY DEFAULT uuid_generate_v7(),
    appointment_id UUID REFERENCES appointments(id) ON DELETE SET NULL,
    profile_id     UUID        NOT NULL REFERENCES profiles(id) ON DELETE RESTRICT,

    status VARCHAR(20) NOT NULL DEFAULT 'waiting'
        CHECK (status IN ('waiting', 'called', 'in_progress', 'completed',
                          'skipped', 'cancelled')),

    priority      INT NOT NULL DEFAULT 0,

    checked_in_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    called_at     TIMESTAMPTZ,
    started_at    TIMESTAMPTZ,
    completed_at  TIMESTAMPTZ,

    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Position is derived from (priority DESC, checked_in_at ASC) among active
-- entries; no stored position column by design.
CREATE INDEX idx_queue_active ON queue_entries(status, priority DESC, checked_in_at)
    WHERE status IN ('waiting', 'called', 'in_progress');
CREATE INDEX idx_queue_appointment ON queue_entries(appointment_id);


-- Schedules ------------------------------------------------------------

CREATE TABLE doctor_schedules (
    id               UUID PRIMARY KEY DEFAULT uuid_generate_v7(),
    doctor_profile_id UUID       NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
    day_of_week      SMALLINT    NOT NULL CHECK (day_of_week BETWEEN 0 AND 6),
    start_time       TIME        NOT NULL,
    end_time         TIME        NOT NULL CHECK (end_time > start_time),
    slot_duration    INT         NOT NULL DEFAULT 30 CHECK (slot_duration > 0),
    is_active        BOOLEAN     NOT NULL DEFAULT true,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (doctor_profile_id, day_of_week, start_time, end_time)
);

CREATE INDEX idx_doctor_schedules_doctor ON doctor_schedules(doctor_profile_id);


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


-- Payments -------------------------------------------------------------

CREATE TABLE payments (
    id             UUID PRIMARY KEY DEFAULT uuid_generate_v7(),
    appointment_id UUID         NOT NULL UNIQUE REFERENCES appointments(id) ON DELETE RESTRICT,
    amount         DECIMAL(12,2) NOT NULL CHECK (amount >= 0),
    currency       VARCHAR(3)   NOT NULL DEFAULT 'EGP',
    method         VARCHAR(20)  NOT NULL CHECK (method IN ('cash', 'card', 'e_wallet')),
    status         VARCHAR(20)  NOT NULL DEFAULT 'pending'
                   CHECK (status IN ('pending', 'paid', 'failed', 'refunded')),
    paid_at        TIMESTAMPTZ,
    reference      VARCHAR(255),
    created_at     TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE INDEX idx_payments_status ON payments(status, created_at);


-- Idempotency (BR-07) ----------------------------------------------------

CREATE TABLE idempotency_keys (
    key             VARCHAR(255) NOT NULL,
    endpoint        VARCHAR(255) NOT NULL,
    user_id         UUID REFERENCES public.users(id) ON DELETE SET NULL,
    request_hash    VARCHAR(64)  NOT NULL,
    response_status INT          NOT NULL,
    response_body   JSONB,
    expires_at      TIMESTAMPTZ  NOT NULL,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT now(),
    PRIMARY KEY (key, endpoint)
);

CREATE INDEX idx_idempotency_keys_expires ON idempotency_keys(expires_at);
