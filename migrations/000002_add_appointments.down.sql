-- 000002_add_appointments.down.sql
-- Rollback appointments table

DROP TRIGGER IF EXISTS update_appointments_updated_at ON appointments;
DROP FUNCTION IF EXISTS update_updated_at_column();

DROP TABLE IF EXISTS appointments;

-- Note: Extensions are dropped in 000000_extensions.down.sql