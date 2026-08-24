-- name: CreateAppointment :one
INSERT INTO appointments (patient_id, doctor_id, start_time, end_time, status, notes, created_by)
VALUES ($1, $2, $3, $4, 'scheduled', $5, $6)
RETURNING *;

-- name: GetAppointmentByID :one
SELECT * FROM appointments WHERE id = $1;

-- name: ListAppointments :many
SELECT * FROM appointments
WHERE (sqlc.narg('patient_id')::uuid IS NULL OR patient_id = sqlc.narg('patient_id'))
  AND (sqlc.narg('doctor_id')::uuid IS NULL OR doctor_id = sqlc.narg('doctor_id'))
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status'))
  AND (sqlc.narg('from_time')::timestamptz IS NULL OR end_time > sqlc.narg('from_time'))
  AND (sqlc.narg('to_time')::timestamptz IS NULL OR start_time < sqlc.narg('to_time'))
ORDER BY start_time DESC
LIMIT $1 OFFSET $2;

-- name: CountAppointments :one
SELECT count(*) FROM appointments
WHERE (sqlc.narg('patient_id')::uuid IS NULL OR patient_id = sqlc.narg('patient_id'))
  AND (sqlc.narg('doctor_id')::uuid IS NULL OR doctor_id = sqlc.narg('doctor_id'))
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status'))
  AND (sqlc.narg('from_time')::timestamptz IS NULL OR end_time > sqlc.narg('from_time'))
  AND (sqlc.narg('to_time')::timestamptz IS NULL OR start_time < sqlc.narg('to_time'));

-- name: TransitionAppointmentStatus :one
UPDATE appointments SET status = $2,
    cancellation_reason = COALESCE(sqlc.narg('cancellation_reason'), cancellation_reason),
    version = version + 1
WHERE id = $1 AND status = sqlc.arg('expected_status')
RETURNING *;

-- name: RescheduleAppointment :one
UPDATE appointments SET start_time = $2, end_time = $3, version = version + 1
WHERE id = $1 AND status IN ('scheduled', 'confirmed')
RETURNING *;

-- name: ListActiveAppointmentsForDoctorInRange :many
SELECT * FROM appointments
WHERE doctor_id = $1 AND status IN ('scheduled', 'confirmed')
  AND tstzrange(start_time, end_time) && tstzrange(@range_start::timestamptz, @range_end::timestamptz);

-- name: InsertNotification :one
INSERT INTO notifications (id, appointment_id, channel, recipient, subject, body, nats_msg_id)
VALUES (@id::uuid, sqlc.narg('appointment_id'), @channel::varchar, @recipient::varchar, @subject::text, @body::text, @msg_id::text)
ON CONFLICT (nats_msg_id) DO NOTHING
RETURNING *;

-- name: MarkNotificationSent :exec
UPDATE notifications SET status = 'sent' WHERE id = $1;

-- name: MarkNotificationFailed :exec
UPDATE notifications SET status = 'failed', attempts = attempts + 1, last_error = $2 WHERE id = $1;

-- name: MarkNotificationDead :exec
UPDATE notifications SET status = 'dead_letter', attempts = attempts + 1, last_error = $2 WHERE id = $1;

-- name: GetNotificationByMsgID :one
SELECT * FROM notifications WHERE nats_msg_id = $1;

-- name: GetPatientContactEmail :one
-- Resolve the notification recipient: prefer the linked user account's
-- email, fall back to the patients.phone column only for SMS later.
SELECT u.email AS contact FROM appointments a
JOIN patients p ON p.id = a.patient_id
LEFT JOIN users u ON u.id = p.user_id
WHERE a.id = $1;

-- name: InsertAuditLog :exec
INSERT INTO audit_logs (actor_user_id, action, entity_type, entity_id, details)
VALUES ($1, $2, $3, $4, $5);

-- name: DeleteExpiredIdempotencyKeys :execrows
DELETE FROM idempotency_keys WHERE expires_at < now();

-- name: GetIdempotentResponse :one
SELECT user_id, request_hash, response_status, response_body FROM idempotency_keys
WHERE key = $1 AND endpoint = $2 AND expires_at > now();

-- name: InsertIdempotentResponse :one
-- DO UPDATE ... WHERE false + RETURNING: concurrent inserts of the same key
-- serialize; the loser gets the winner's row back instead of silently
-- committing a duplicate booking.
INSERT INTO idempotency_keys (key, endpoint, user_id, request_hash, response_status, response_body, expires_at)
VALUES ($1, $2, $3, $4, $5, $6, now() + make_interval(secs => sqlc.arg('ttl_seconds')::int))
ON CONFLICT (key, endpoint) DO UPDATE SET key = EXCLUDED.key WHERE false
RETURNING key;
