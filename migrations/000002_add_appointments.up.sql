-- 000002_add_appointments.up.sql
-- Appointment table with exclusion constraint for concurrency control
-- Implements ADR-002: PostgreSQL Exclusion Constraints for appointment conflict prevention
-- Extensions are created in 000000_extensions.up.sql

-- Appointments table
CREATE TABLE appointments (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v7(),
    patient_id UUID NOT NULL REFERENCES patients(id) ON DELETE RESTRICT,
    doctor_id UUID NOT NULL REFERENCES doctors(id) ON DELETE RESTRICT,
    schedule_id UUID REFERENCES doctor_schedules(id) ON DELETE SET NULL,
    
    -- Time range (timezone-aware)
    start_time TIMESTAMPTZ NOT NULL,
    end_time TIMESTAMPTZ NOT NULL,
    
    -- Status lifecycle: scheduled -> confirmed -> completed | cancelled | no_show
    status VARCHAR(20) NOT NULL DEFAULT 'scheduled'
        CHECK (status IN ('scheduled', 'confirmed', 'completed', 'cancelled', 'no_show')),
    
    -- Optional notes
    notes TEXT,
    cancellation_reason TEXT,
    
    -- Audit fields
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    
    -- Ensure valid time range
    CONSTRAINT valid_time_range CHECK (end_time > start_time)
);

-- Exclusion constraint: prevents overlapping appointments for the same doctor
-- Only applies to active appointments (scheduled, confirmed)
-- Uses tstzrange for timezone-aware time range overlap detection (&& operator)
ALTER TABLE appointments ADD CONSTRAINT no_overlapping_appointments
    EXCLUDE USING GIST (
        doctor_id WITH =,
        tstzrange(start_time, end_time) WITH &&
    ) WHERE (status IN ('scheduled', 'confirmed'));

-- Indexes for query performance
CREATE INDEX idx_appointments_doctor_id ON appointments(doctor_id);
CREATE INDEX idx_appointments_patient_id ON appointments(patient_id);
CREATE INDEX idx_appointments_schedule_id ON appointments(schedule_id);
CREATE INDEX idx_appointments_start_time ON appointments(start_time);
CREATE INDEX idx_appointments_status ON appointments(status);
CREATE INDEX idx_appointments_doctor_start_time ON appointments(doctor_id, start_time);

-- Composite index for common query: doctor's appointments in date range
CREATE INDEX idx_appointments_doctor_time_range 
    ON appointments(doctor_id, start_time, end_time) 
    WHERE status IN ('scheduled', 'confirmed');

-- Trigger to auto-update updated_at
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER update_appointments_updated_at
    BEFORE UPDATE ON appointments
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();