-- name: CreatePatient :one
INSERT INTO patients (
    full_name, date_of_birth, gender, phone, address,
    emergency_contact_name, emergency_contact_phone, medical_notes
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetPatientByID :one
SELECT * FROM patients WHERE id = $1;

-- name: UpdatePatient :one
UPDATE patients SET
    full_name = COALESCE(sqlc.narg('full_name'), full_name),
    date_of_birth = COALESCE(sqlc.narg('date_of_birth'), date_of_birth),
    gender = COALESCE(sqlc.narg('gender'), gender),
    phone = COALESCE(sqlc.narg('phone'), phone),
    address = COALESCE(sqlc.narg('address'), address),
    emergency_contact_name = COALESCE(sqlc.narg('emergency_contact_name'), emergency_contact_name),
    emergency_contact_phone = COALESCE(sqlc.narg('emergency_contact_phone'), emergency_contact_phone),
    medical_notes = COALESCE(sqlc.narg('medical_notes'), medical_notes)
WHERE id = sqlc.arg('id')
RETURNING *;

-- name: SearchPatients :many
SELECT * FROM patients
WHERE sqlc.arg('search')::text = '' OR full_name ILIKE '%' || sqlc.arg('search') || '%'
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: CountPatients :one
SELECT count(*) FROM patients
WHERE sqlc.arg('search')::text = '' OR full_name ILIKE '%' || sqlc.arg('search') || '%';

-- name: DeletePatient :execrows
DELETE FROM patients WHERE id = $1;

-- name: GetPatientIDByUserID :one
SELECT id FROM patients WHERE user_id = $1;
