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