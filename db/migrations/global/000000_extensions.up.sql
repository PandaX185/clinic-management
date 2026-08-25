-- Extensions required by the schema:
--   btree_gist: GiST support for scalar equality in the appointments
--               exclusion constraint (no_overlapping_appointments)
--   pgcrypto:   gen_random_uuid() used by uuid_generate_v7()
CREATE EXTENSION IF NOT EXISTS btree_gist;
CREATE EXTENSION IF NOT EXISTS pgcrypto;
