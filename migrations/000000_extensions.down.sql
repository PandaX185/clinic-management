-- 000000_extensions.down.sql
-- Rollback core extensions

DROP EXTENSION IF EXISTS uuidv7;
DROP EXTENSION IF EXISTS "uuid-ossp";
DROP EXTENSION IF EXISTS btree_gist;