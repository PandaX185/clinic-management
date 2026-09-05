-- Appointments (schema v2: profile-based, typed, queue-ready)

-- name: CreateAppointment :one
INSERT INTO appointments (
    profile_id, doctor_profile_id, appointment_type_id,
    scheduled_start, scheduled_end, status,
    visit_notes, follow_up_date, cancellation_reason,
    version, created_by
) VALUES (
    $1, $2, $3, $4, $5, $6,
    $7, $8, $9, $10, $11
)
RETURNING id, profile_id, doctor_profile_id, appointment_type_id,
    scheduled_start, scheduled_end, status,
    visit_notes, follow_up_date, cancellation_reason,
    version, created_by, created_at, updated_at;

-- name: GetAppointmentByID :one
SELECT id, profile_id, doctor_profile_id, appointment_type_id,
    scheduled_start, scheduled_end, status,
    visit_notes, follow_up_date, cancellation_reason,
    version, created_by, created_at, updated_at
FROM appointments WHERE id = $1;

-- name: ListAppointments :many
SELECT id, profile_id, doctor_profile_id, appointment_type_id,
    scheduled_start, scheduled_end, status,
    visit_notes, follow_up_date, cancellation_reason,
    version, created_by, created_at, updated_at
FROM appointments
WHERE (CAST($1 AS UUID) = '00000000-0000-0000-0000-000000000000'
       OR profile_id = $1 OR doctor_profile_id = $1)
  AND (CAST($2 AS VARCHAR) = '' OR status = CAST($2 AS VARCHAR))
ORDER BY scheduled_start DESC
LIMIT $3 OFFSET $4;

-- name: CountAppointments :one
SELECT COUNT(*) FROM appointments
WHERE (CAST($1 AS UUID) = '00000000-0000-0000-0000-000000000000'
       OR profile_id = $1 OR doctor_profile_id = $1)
  AND (CAST($2 AS VARCHAR) = '' OR status = CAST($2 AS VARCHAR));

-- name: RescheduleAppointment :one
UPDATE appointments SET
    scheduled_start = $2,
    scheduled_end   = $3,
    version         = version + 1
WHERE id = $1
RETURNING id, profile_id, doctor_profile_id, appointment_type_id,
    scheduled_start, scheduled_end, status,
    visit_notes, follow_up_date, cancellation_reason,
    version, created_by, created_at, updated_at;

-- name: TransitionAppointmentStatus :one
UPDATE appointments SET
    status = $2,
    cancellation_reason = $3,
    version = version + 1
WHERE id = $1 AND status = $4
RETURNING id, profile_id, doctor_profile_id, appointment_type_id,
    scheduled_start, scheduled_end, status,
    visit_notes, follow_up_date, cancellation_reason,
    version, created_by, created_at, updated_at;

-- name: ListAppointmentsByUser :many
-- Appointments of a single patient across a clinic: joins through the
-- patient's profile so the query is automatically scoped to one user.
SELECT a.id, a.profile_id, a.doctor_profile_id, a.appointment_type_id,
    a.scheduled_start, a.scheduled_end, a.status,
    a.visit_notes, a.follow_up_date, a.cancellation_reason,
    a.version, a.created_by, a.created_at, a.updated_at
FROM appointments a
JOIN profiles p ON p.id = a.profile_id
WHERE p.user_id = $1
ORDER BY a.scheduled_start DESC;

-- name: GetAppointmentByIDAndUser :one
-- Ownership-scoped read: the requested appointment is only returned when it
-- belongs to the given user (through their patient profile).
SELECT a.id, a.profile_id, a.doctor_profile_id, a.appointment_type_id,
    a.scheduled_start, a.scheduled_end, a.status,
    a.visit_notes, a.follow_up_date, a.cancellation_reason,
    a.version, a.created_by, a.created_at, a.updated_at
FROM appointments a
JOIN profiles p ON p.id = a.profile_id
WHERE a.id = $1 AND p.user_id = $2;

-- name: ListAppointmentsForDoctorDate :many
-- Appointments overlapping the [from, to) window for a doctor; used to
-- compute busy intervals when building available slots.
SELECT id, profile_id, doctor_profile_id, appointment_type_id,
    scheduled_start, scheduled_end, status,
    visit_notes, follow_up_date, cancellation_reason,
    version, created_by, created_at, updated_at
FROM appointments
WHERE doctor_profile_id = $1
  AND scheduled_start < $3
  AND scheduled_end > $2
  AND status IN ('scheduled', 'confirmed')
ORDER BY scheduled_start;