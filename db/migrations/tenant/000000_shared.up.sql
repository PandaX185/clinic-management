-- Shared objects used by every tenant schema. Run once per schema after
-- CREATE SCHEMA: the uuidv7 helper and updated_at trigger function are
-- schema-qualified references from tenant tables.
--
-- These live in public (created by global migration 000000/000002) — this
-- file exists so per-schema provisioning can verify they exist.
SELECT 1;
