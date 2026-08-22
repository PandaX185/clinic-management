-- name: CreatePatient :one
INSERT INTO patients (
    user_id, date_of_birth, gender, address, emergency_contact_name, emergency_contact_phone, medical_notes
) VALUES (
    $1, $2, $3, $4, $5, $6, $7
) RETURNING id, user_id, date_of_birth, gender, address, emergency_contact_name, emergency_contact_phone, medical_notes, created_at, updated_at;

-- name: GetPatient :one
SELECT id, user_id, date_of_birth, gender, address, emergency_contact_name, emergency_contact_phone, medical_notes, created_at, updated_at
FROM patients
WHERE id = $1;

-- name: GetPatientByUserID :one
SELECT id, user_id, date_of_birth, gender, address, emergency_contact_name, emergency_contact_phone, medical_notes, created_at, updated_at
FROM patients
WHERE user_id = $1;

-- name: UpdatePatient :one
UPDATE patients
SET date_of_birth = $2, gender = $3, address = $4, emergency_contact_name = $5, emergency_contact_phone = $6, medical_notes = $7, updated_at = now()
WHERE id = $1
RETURNING id, user_id, date_of_birth, gender, address, emergency_contact_name, emergency_contact_phone, medical_notes, created_at, updated_at;

-- name: DeletePatient :exec
DELETE FROM patients WHERE id = $1;

-- name: ListPatients :many
SELECT id, user_id, date_of_birth, gender, address, emergency_contact_name, emergency_contact_phone, medical_notes, created_at, updated_at
FROM patients
ORDER BY created_at DESC;