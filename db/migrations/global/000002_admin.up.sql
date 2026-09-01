-- Add a global super-admin flag to users. This is distinct from the
-- per-clinic admin role (resolved from the tenant's profiles table): a global
-- admin can provision clinics and manage the tenant registry itself.
ALTER TABLE users
    ADD COLUMN is_admin BOOLEAN NOT NULL DEFAULT FALSE;
