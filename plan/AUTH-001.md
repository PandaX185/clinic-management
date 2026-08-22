# AUTH-001 — JWT Auth Middleware + Auth Endpoints

## Scope
- argon2id password hashing (`golang.org/x/crypto/argon2`), PHC string format `$argon2id$v=19$m=65536,t=3,p=4$<salt>$<hash>`, with verify.
- JWT (HS256, hand-rolled with stdlib HMAC to avoid a new dep): access 15m, refresh 168h. Claims: `sub`, `email`, `roles`, `jti`, `iat`, `exp`, `typ`.
- Redis-backed refresh revocation: key `refresh:<jti>` = user_id, EX = refresh TTL. Refresh checks existence, rotates (delete old jti, issue new pair). Logout deletes key. Revoked tokens rejected at refresh; middleware rejects revoked refresh usage implicitly since only access tokens hit the middleware (access tokens are short-lived and not stored).
- Endpoints: POST /api/v1/auth/{register,login,refresh,logout}. Errors: 400 invalid input, 401 bad credentials/token, 409 duplicate email, 500 internal.
- Middleware: `RequireAuth` parses Bearer token, validates sig/expiry, injects user id/email/roles into context. `RequireRole("admin", ...)` → 403 if no match.

## Package layout (internal/auth)
- `password.go` — HashPassword / VerifyPassword (constant-time compare via subtle).
- `jwt.go` — Claims struct, SignToken(secret, claims), ParseToken(secret, tokenStr). Separate access & refresh secrets.
- `store.go` — small interfaces: `UserStore`, `RoleStore`, `TokenStore` so unit tests need no live DB/Redis.
- `redisstore.go` — TokenStore impl over go-redis (`Set/Exists/Delete` on `refresh:<jti>`).
- `middleware.go` — RequireAuth + RequireRole + context helpers.
- `handler.go` — Handler{store deps} with Register/Login/Refresh/Logout.
- tests: password_test.go, jwt_test.go, middleware_test.go, handler_test.go (table-driven, httptest, in-memory fake store).

## Wiring
- cmd/api/main.go: build `db.New(dbpool)` queries + redis client into auth.NewHandler / redis token store; replace placeholder routes; register/logout behind RequireAuth group for logout.

## Verification
go build ./... && go vet ./... && go test ./... (unit only; integration needs live services — deferred).

## Out of scope / deferred
- Email verification flow, rate limiting, audit events on NATS.
- Access-token denylist (access tokens are 15m; revocation applies to refresh tokens per spec).
