-- 000000_extensions.up.sql
-- Core extensions required by the application
-- Runs FIRST before any schema migrations

-- UUIDv7 generation (RFC 9562) - time-ordered UUIDs
CREATE OR REPLACE FUNCTION uuid_generate_v7()
RETURNS uuid
LANGUAGE sql
AS $func$
SELECT (
    lpad(to_hex((extract(epoch from clock_timestamp()) * 1000)::bigint), 12, '0') || 
    substr(encode(gen_random_bytes(10), 'hex'), 1, 20)
)::uuid;
$func$;

-- Legacy UUID support (v4, v5) - for compatibility
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Exclusion constraints for concurrency control (appointments) - REQUIRED for exclusion constraint
CREATE EXTENSION IF NOT EXISTS btree_gist;

-- pgcrypto for gen_random_bytes (used by uuid_generate_v7)
CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- Optional: pg_trgm for fuzzy search (future use)
-- CREATE EXTENSION IF NOT EXISTS pg_trgm;