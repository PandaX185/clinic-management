-- Per-tenant clinical schema baseline. Applied inside each clinic's own
-- schema (tenant_<slug>). Global identity lives in the public schema.
-- ---------------------------------------------------------------------------
-- Doctors & patients (FR-PAT-01..05, FR-DOC-01..04)
-- ---------------------------------------------------------------------------

CREATE TABLE doctors (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v7(),
    user_id UUID NOT NULL UNIQUE REFERENCES public.users(id) ON DELETE CASCADE,
    specialization VARCHAR(255) NOT NULL,
    license_number VARCHAR(100) NOT NULL UNIQUE,
    bio TEXT,
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE patients (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v7(),
    user_id UUID UNIQUE REFERENCES public.users(id) ON DELETE SET NULL,
    full_name VARCHAR(255) NOT NULL,
    date_of_birth DATE,
    gender VARCHAR(20) CHECK (gender IN ('male', 'female', 'other')),
    phone VARCHAR(50),
    address TEXT,
    emergency_contact_name VARCHAR(255),
    emergency_contact_phone VARCHAR(50),
    medical_notes TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- NOTE: no indexes on searchable text columns (full_name, specialization,
-- is_active). Current queries scan small tables; revisit with pg_trgm GIN
-- if ILIKE '%term%' search ever needs index support.

-- Recurring weekly availability (day_of_week: 0 = Sunday .. 6 = Saturday)
CREATE TABLE doctor_schedules (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v7(),
    doctor_id UUID NOT NULL REFERENCES doctors(id) ON DELETE CASCADE,
    day_of_week SMALLINT NOT NULL CHECK (day_of_week BETWEEN 0 AND 6),
    start_time TIME NOT NULL,
    end_time TIME NOT NULL CHECK (end_time > start_time),
    slot_minutes INT NOT NULL DEFAULT 30 CHECK (slot_minutes > 0),
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (doctor_id, day_of_week, start_time, end_time)
);

CREATE INDEX idx_doctor_schedules_doctor_id ON doctor_schedules(doctor_id);

-- One-off exceptions: holidays / sick days / custom hours on a specific date
CREATE TABLE doctor_schedule_exceptions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v7(),
    doctor_id UUID NOT NULL REFERENCES doctors(id) ON DELETE CASCADE,
    exception_date DATE NOT NULL,
    is_unavailable BOOLEAN NOT NULL DEFAULT true,
    start_time TIME,
    end_time TIME,
    reason VARCHAR(255),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT valid_exception_hours CHECK (
        is_unavailable OR (start_time IS NOT NULL AND end_time IS NOT NULL AND end_time > start_time)
    )
);

CREATE INDEX idx_schedule_exceptions_doctor_date ON doctor_schedule_exceptions(doctor_id, exception_date);

-- ---------------------------------------------------------------------------
-- Appointments (FR-APT-01..09)
-- Lifecycle per BR-03: scheduled -> confirmed -> completed | cancelled | no_show
-- ---------------------------------------------------------------------------

CREATE TABLE appointments (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v7(),
    patient_id UUID NOT NULL REFERENCES patients(id) ON DELETE RESTRICT,
    doctor_id UUID NOT NULL REFERENCES doctors(id) ON DELETE RESTRICT,

    start_time TIMESTAMPTZ NOT NULL,
    end_time TIMESTAMPTZ NOT NULL,

    status VARCHAR(20) NOT NULL DEFAULT 'scheduled'
        CHECK (status IN ('scheduled', 'confirmed', 'completed', 'cancelled', 'no_show')),

    notes TEXT,
    cancellation_reason TEXT,
    version INT NOT NULL DEFAULT 1,

    created_by UUID REFERENCES public.users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT valid_appointment_range CHECK (end_time > start_time)
);

-- BR-01 + FR-APT-05/06: the database itself rejects overlapping active slots
-- even under fully concurrent requests; only active statuses block time (BR-06).
ALTER TABLE appointments ADD CONSTRAINT no_overlapping_appointments
    EXCLUDE USING GIST (
        doctor_id WITH =,
        tstzrange(start_time, end_time) WITH &&
    ) WHERE (status IN ('scheduled', 'confirmed'));

CREATE INDEX idx_appointments_patient ON appointments(patient_id, start_time DESC);
CREATE INDEX idx_appointments_doctor_range ON appointments(doctor_id, start_time)
    WHERE status IN ('scheduled', 'confirmed');
CREATE INDEX idx_appointments_status_start ON appointments(status, start_time);

-- ---------------------------------------------------------------------------
-- Cross-cutting operational entities (SRS 7.1: Audit Event, Notification,
-- Idempotency Record)
-- ---------------------------------------------------------------------------

CREATE TABLE audit_logs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v7(),
    actor_user_id UUID REFERENCES public.users(id) ON DELETE SET NULL,
    action VARCHAR(100) NOT NULL,
    entity_type VARCHAR(50) NOT NULL,
    entity_id UUID,
    details JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_audit_logs_entity ON audit_logs(entity_type, entity_id, created_at DESC);
CREATE INDEX idx_audit_logs_actor ON audit_logs(actor_user_id, created_at DESC);

CREATE TABLE notifications (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v7(),
    appointment_id UUID REFERENCES appointments(id) ON DELETE SET NULL,
    channel VARCHAR(20) NOT NULL CHECK (channel IN ('email', 'sms')),
    recipient VARCHAR(255) NOT NULL,
    subject TEXT,
    body TEXT NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'sent', 'failed', 'dead_letter')),
    attempts INT NOT NULL DEFAULT 0,
    last_error TEXT,
    nats_msg_id VARCHAR(128) UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_notifications_status ON notifications(status, created_at);

-- BR-07: client retries with the same key map to the original response.
CREATE TABLE idempotency_keys (
    key VARCHAR(255) NOT NULL,
    endpoint VARCHAR(255) NOT NULL,
    user_id UUID REFERENCES public.users(id) ON DELETE SET NULL,
    request_hash VARCHAR(64) NOT NULL,
    response_status INT NOT NULL,
    response_body JSONB,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (key, endpoint)
);

CREATE INDEX idx_idempotency_keys_expires ON idempotency_keys(expires_at);

-- ---------------------------------------------------------------------------
-- updated_at triggers
-- ---------------------------------------------------------------------------

CREATE OR REPLACE FUNCTION set_updated_at() RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_doctors_updated_at BEFORE UPDATE ON doctors
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER trg_patients_updated_at BEFORE UPDATE ON patients
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER trg_doctor_schedules_updated_at BEFORE UPDATE ON doctor_schedules
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER trg_doctor_schedule_exceptions_updated_at BEFORE UPDATE ON doctor_schedule_exceptions
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER trg_appointments_updated_at BEFORE UPDATE ON appointments
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER trg_notifications_updated_at BEFORE UPDATE ON notifications
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
