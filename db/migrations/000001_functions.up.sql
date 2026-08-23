-- Custom UUIDv7 generator: time-ordered primary keys, index-friendly.
-- Pure SQL body so both golang-migrate and sqlc can process this file.
CREATE EXTENSION IF NOT EXISTS btree_gist;
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE FUNCTION uuid_generate_v7() RETURNS uuid AS $func$
  SELECT encode(
    set_bit(
      set_bit(
        overlay(uuid_send(gen_random_uuid())
          placing substring(int8send((floor(extract(epoch FROM clock_timestamp()) * 1000))::bigint)::bytea FROM 3)
          FROM 1 FOR 6),
        52, 1),
      53, 1),
    'hex')::uuid;
$func$ LANGUAGE SQL VOLATILE;
