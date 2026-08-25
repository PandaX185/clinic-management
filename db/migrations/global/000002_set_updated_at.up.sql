-- Shared trigger function used by both the global schema (tenants, users)
-- and every per-tenant clinical schema. Kept as its own migration so it can
-- be referenced independently by sqlc and by tenant-schema provisioning.
CREATE OR REPLACE FUNCTION set_updated_at() RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
