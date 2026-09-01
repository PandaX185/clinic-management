-- The auth service keeps a single active refresh token per user: login and
-- refresh both upsert via ON CONFLICT (user_id). Enforce that uniqueness on
-- the backing table (previously only token_hash was unique, so the upsert
-- threw 42P10 at runtime).
ALTER TABLE user_refresh_tokens
    ADD CONSTRAINT user_refresh_tokens_user_id_key UNIQUE (user_id);