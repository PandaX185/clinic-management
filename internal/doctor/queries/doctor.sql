-- name: CreateDoctor :one
INSERT INTO doctors (
    user_id, specialization, license_number, bio
) VALUES (
    $1, $2, $3, $4
) RETURNING id, user_id, specialization, license_number, bio, is_active, created_at, updated_at;

-- name: GetDoctor :one
SELECT id, user_id, specialization, license_number, bio, is_active, created_at, updated_at
FROM doctors
WHERE id = $1;

-- name: GetDoctorByUserID :one
SELECT id, user_id, specialization, license_number, bio, is_active, created_at, updated_at
FROM doctors
WHERE user_id = $1;

-- name: ListDoctors :many
SELECT id, user_id, specialization, license_number, bio, is_active, created_at, updated_at
FROM doctors
WHERE is_active = true
ORDER BY created_at DESC;

-- name: UpdateDoctor :one
UPDATE doctors
SET specialization = $2, license_number = $3, bio = $4, is_active = $5, updated_at = now()
WHERE id = $1
RETURNING id, user_id, specialization, license_number, bio, is_active, created_at, updated_at;

-- name: DeleteDoctor :exec
DELETE FROM doctors WHERE id = $1;

-- name: GetDoctorSchedule :many
SELECT id, doctor_id, day_of_week, start_time, end_time, is_active, created_at, updated_at
FROM doctor_schedules
WHERE doctor_id = $1 AND is_active = true
ORDER BY day_of_week, start_time;

-- name: CreateDoctorSchedule :one
INSERT INTO doctor_schedules (doctor_id, day_of_week, start_time, end_time)
VALUES ($1, $2, $3, $4)
RETURNING id, doctor_id, day_of_week, start_time, end_time, is_active, created_at, updated_at;

-- name: DeleteDoctorSchedule :exec
DELETE FROM doctor_schedules WHERE id = $1;

-- name: CreateDoctorScheduleException :one
INSERT INTO doctor_schedule_exceptions (doctor_id, exception_date, start_time, end_time, is_unavailable, reason)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, doctor_id, exception_date, start_time, end_time, is_unavailable, reason, created_at;

-- name: GetDoctorScheduleExceptions :many
SELECT id, doctor_id, exception_date, start_time, end_time, is_unavailable, reason, created_at
FROM doctor_schedule_exceptions
WHERE doctor_id = $1
ORDER BY exception_date;