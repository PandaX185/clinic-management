-- 000001_init_schema.down.sql
-- Rollback baseline schema

DROP TABLE IF EXISTS doctor_schedule_exceptions;
DROP TABLE IF EXISTS doctor_schedules;
DROP TABLE IF EXISTS patients;
DROP TABLE IF EXISTS doctors;
DROP TABLE IF EXISTS user_roles;
DROP TABLE IF EXISTS roles;
DROP TABLE IF EXISTS users;

-- Note: Extensions are dropped in 000000_extensions.down.sql