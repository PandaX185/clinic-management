-- name: CreateAppointment :one
INSERT INTO appointments (
    patient_id, doctor_id, schedule_id, start_time, end_time, status, notes, created_by
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8
) RETURNING id, patient_id, doctor_id, schedule_id, start_time, end_time, status, notes, cancellation_reason, created_by, created_at, updated_at;

-- name: GetAppointment :one
SELECT id, patient_id, doctor_id, schedule_id, start_time, end_time, status, notes, cancellation_reason, created_by, created_at, updated_at
FROM appointments
WHERE id = $1;

-- name: ListAppointmentsByDoctor :many
SELECT id, patient_id, doctor_id, schedule_id, start_time, end_time, status, notes, cancellation_reason, created_by, created_at, updated_at
FROM appointments
WHERE doctor_id = $1
  AND start_time >= $2
  AND end_time <= $3
  AND status IN ('scheduled', 'confirmed')
ORDER BY start_time ASC;

-- name: ListAppointmentsByPatient :many
SELECT id, patient_id, doctor_id, schedule_id, start_time, end_time, status, notes, cancellation_reason, created_by, created_at, updated_at
FROM appointments
WHERE patient_id = $1
ORDER BY start_time DESC;

-- name: UpdateAppointmentStatus :one
UPDATE appointments
SET status = $2, updated_at = now()
WHERE id = $1
RETURNING id, patient_id, doctor_id, schedule_id, start_time, end_time, status, notes, cancellation_reason, created_by, created_at, updated_at;

-- name: CancelAppointment :one
UPDATE appointments
SET status = 'cancelled', cancellation_reason = $2, updated_at = now()
WHERE id = $1
RETURNING id, patient_id, doctor_id, schedule_id, start_time, end_time, status, notes, cancellation_reason, created_by, created_at, updated_at;

-- name: RescheduleAppointment :one
UPDATE appointments
SET start_time = $2, end_time = $3, updated_at = now()
WHERE id = $1
RETURNING id, patient_id, doctor_id, schedule_id, start_time, end_time, status, notes, cancellation_reason, created_by, created_at, updated_at;

-- name: CheckAppointmentConflict :one
SELECT EXISTS (
    SELECT 1 FROM appointments
    WHERE doctor_id = $1
      AND status IN ('scheduled', 'confirmed')
      AND tstzrange(start_time, end_time) && tstzrange($2, $3)
      AND id != COALESCE($4, '00000000-0000-0000-0000-000000000000'::uuid)
) AS conflict_exists;

-- name: CountAppointmentsByDoctorInRange :one
SELECT COUNT(*) FROM appointments
WHERE doctor_id = $1
  AND start_time >= $2
  AND end_time <= $3
  AND status IN ('scheduled', 'confirmed');