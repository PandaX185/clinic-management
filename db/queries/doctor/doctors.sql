-- name: CreateDoctor :one
INSERT INTO doctors (user_id, specialization, license_number, bio)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetDoctorByID :one
SELECT d.*, u.full_name, u.email AS user_email FROM doctors d
JOIN users u ON u.id = d.user_id
WHERE d.id = $1;

-- name: ListDoctors :many
SELECT d.*, u.full_name, u.email AS user_email FROM doctors d
JOIN users u ON u.id = d.user_id
WHERE (@is_active::boolean = false OR d.is_active = @is_active)
  AND sqlc.arg('specialization')::text = '' OR d.specialization ILIKE '%' || sqlc.arg('specialization') || '%'
ORDER BY u.full_name
LIMIT $1 OFFSET $2;

-- name: CreateDoctorSchedule :one
INSERT INTO doctor_schedules (doctor_id, day_of_week, start_time, end_time, slot_minutes)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: ListDoctorSchedules :many
SELECT * FROM doctor_schedules
WHERE doctor_id = $1 AND is_active = true
ORDER BY day_of_week, start_time;

-- name: DeactivateDoctorSchedule :exec
UPDATE doctor_schedules SET is_active = false WHERE id = $1;

-- name: CreateScheduleException :one
INSERT INTO doctor_schedule_exceptions (doctor_id, exception_date, is_unavailable, start_time, end_time, reason)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: ListScheduleExceptions :many
SELECT * FROM doctor_schedule_exceptions
WHERE doctor_id = $1 AND exception_date BETWEEN $2 AND $3
ORDER BY exception_date;
