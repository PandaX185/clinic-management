-- Per-tenant clinical schema teardown (reverse dependency order).
DROP TABLE IF EXISTS idempotency_keys;
DROP TABLE IF EXISTS notifications;
DROP TABLE IF EXISTS audit_logs;
DROP TABLE IF EXISTS appointments;
DROP TABLE IF EXISTS doctor_schedule_exceptions;
DROP TABLE IF EXISTS doctor_schedules;
DROP TABLE IF EXISTS patients;
DROP TABLE IF EXISTS doctors;
