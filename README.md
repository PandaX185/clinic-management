# Lahza (لحظة)

Multi-clinic appointment backend in Go — each clinic gets its own isolated Postgres schema, with global user accounts that work across clinics.

## How multi-tenancy works

- **One Postgres schema per clinic** (`tenant_<slug>`): patients, doctors, schedules, appointments, notifications, and audit logs all live inside the clinic's schema, so data is physically isolated.
- **Users are global**: a single account (email/password) works at every clinic. Roles are per clinic — the same person can be a doctor at one clinic and a patient at another.
- **Login is tenant-free.** The token identifies you, not a clinic.
- **Each request names its clinic** via the `X-Tenant-ID` header. The middleware validates it against the tenant registry, reads your role from that clinic's `profiles` table (with a short-lived cache), and pins the DB connection to that schema. No profile there? You get patient-level access — which any signed-in user has everywhere.
- **New clinics are provisioned programmatically**: creating a tenant runs the embedded clinical migrations inside a fresh schema.

## Features

| Domain        | Capabilities |
|---------------|--------------|
| Tenants       | Schema-per-clinic isolation, programmatic provisioning, admin endpoints |
| Patients      | Global login, auto-provisioned chart per clinic on first booking |
| Doctors       | Profiles bound per clinic, schedules, availability engine |
| Appointments  | Double-booking impossible (DB exclusion constraint), idempotent booking, state machine |
| Auth          | JWT access + refresh, role-based access resolved from per-clinic profiles |
| Notifications | Async via NATS JetStream, retries, DLQ |

## Stack

| Layer        | Tech |
|--------------|------|
| Language     | Go 1.26 |
| Router       | gin (chi removed) |
| HTTP Server  | net/http + single gin engine (incl. /metrics) |
| Database     | PostgreSQL 16 + pgx/v5 + sqlc |
| Migrations   | golang-migrate + embedded tenant migrations |
| Cache        | Redis 7 (rate limiting) |
| Messaging    | NATS JetStream (optional at boot) |
| Auth         | JWT HS256 + bcrypt + per-tenant RBAC + global super-admin |
| Logging      | log/slog (zap removed) |
| Connections  | shared retry: per-dependency context + timeout + backoff, all env-configurable (`*_CONNECT_*`)

## Quick start

```bash
git clone https://github.com/PandaX185/lahza.git
cd lahza

# Start dependencies
docker-compose up -d

# Apply migrations
make migrate-up

# Run
make run
```

## API sketch

Interactive docs (Swagger UI): `http://localhost:8080/swagger/index.html`
(spec: `/swagger/doc.json`). Regenerate after handler changes with `make swagger`.

All clinical endpoints require `X-Tenant-ID: <clinic uuid>`.

```
POST /api/v1/auth/register          # global sign-up
POST /api/v1/auth/login             # global login
GET  /api/v1/clinics                # list clinics
GET  /api/v1/clinics/mine           # your clinics
GET  /api/v1/auth/me                # your global profile (auth only)
GET  /api/v1/auth/clinics           # clinics you belong to (auth only)
POST /api/v1/clinics                # create clinic (global super-admin)
POST /api/v1/clinics/{id}/staff     # assign a role in a clinic (per-clinic admin)
GET  /api/v1/booking/clinics        # public clinic discovery (no auth)
GET  /api/v1/booking/clinics/{id}/slots   # doctor availability
GET  /api/v1/portal/appointments    # patient portal: your appointments
POST /api/v1/appointments           # book (patient_id is forced to you)
POST /api/v1/appointments/{id}/cancel | /reschedule | /confirm | /complete | /no-show
GET  /metrics                       # Prometheus
```

### Authorisation model

Two independent gates enforce access:

- **Per-clinic roles** (the common path): every clinical endpoint resolves the caller's role from the active clinic's `profiles → profile_roles → roles` via `X-Tenant-ID` (`TenantMiddleware`). Any signed-in user without a profile is treated as a `patient`. Staff/admin-only transitions (`confirm`, `complete`, `no-show`) and clinic management (`BindStaff`) are gated by `RequireRoles`.
- **Global super-admin**: provisioning a clinic (`POST /clinics`) has no tenant to resolve a role from, so it is gated on `users.is_admin` (`RequireGlobalAdmin`) instead. Set `users.is_admin = true` directly (there is no self-service path).

Standard roles (`admin`, `staff`, `doctor`, `nurse`, `manager`, `patient`) are seeded into every clinic schema at provision time.

## Project structure

```
├── cmd/
│   └── api/                    # thin entry point: lifecycle only (config, logger, db, shutdown)
├── internal/
│   ├── app/wiring/             # constructs the full dependency graph + router
│   ├── identity/               # JWT, login/refresh, middleware, memberships
│   ├── clinic/                 # clinic registry, memberships, per-clinic profiles
│   ├── catalog/                # profiles, doctors, services, appointment types
│   ├── scheduling/             # appointment lifecycle domain + scoped repository
│   ├── booking/                # public clinic discovery + slot availability
│   ├── portal/                 # patient portal: their profile + appointments
│   └── platform/               # config, db (ScopedPool), redis, nats, metrics
├── db/
│   ├── migrations/global/      # tenants, users — applied once
│   ├── migrations/tenant/      # clinical tables — applied per clinic
│   └── queries/                # sqlc sources (identity, clinic, scheduling)
└── sqlc.yaml
```

## Testing

```bash
make test-race      # unit tests + race detector
make vet            # static analysis
make test-coverage  # coverage report
```

## License

MIT — see [LICENSE](LICENSE).
