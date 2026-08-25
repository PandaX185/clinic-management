# Clinic Management

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

| Layer      | Tech |
|------------|------|
| Language   | Go 1.25 |
| Router     | Chi v5 |
| Database   | PostgreSQL 16 + pgx/v5 + sqlc |
| Migrations | golang-migrate + embedded tenant migrations |
| Cache      | Redis 7 (rate limiting) |
| Messaging  | NATS JetStream (optional at boot) |
| Auth       | JWT HS256 + bcrypt |

## Quick start

```bash
git clone https://github.com/PandaX185/clinic-management.git
cd clinic-management

# Start dependencies
docker-compose up -d

# Apply migrations
make migrate-up

# Run
make run
```

### Migrating an existing single-tenant database

If you ran an older version, `cmd/migrate-to-tenants` moves existing clinical data into a `default` clinic schema:

```bash
go run ./cmd/migrate-to-tenants
```

## API sketch

All clinical endpoints require `X-Tenant-ID: <clinic uuid>`.

```
POST /api/v1/auth/register          # global sign-up
POST /api/v1/auth/login             # global login
GET  /api/v1/tenants                # list clinics
GET  /api/v1/tenants/mine           # your clinics
GET  /api/v1/doctors                # doctors at X-Tenant-ID
GET  /api/v1/appointments           # your appointments at X-Tenant-ID
POST /api/v1/appointments           # book (patient_id is forced to you)
POST /api/v1/appointments/{id}/cancel | /reschedule | /confirm | /complete | /no-show
GET  /metrics                       # Prometheus
```

## Project structure

```
├── cmd/
│   ├── api/                    # entry point + wiring
│   └── migrate-to-tenants/     # one-shot legacy data migration
├── internal/
│   ├── auth/                   # JWT, login/refresh, middleware
│   ├── tenant/                 # tenants, memberships, profiles
│   ├── appointment/            # domain logic, scoped repository
│   ├── doctor/  patient/       # scoped repositories
│   ├── notification/           # NATS worker + store
│   └── platform/               # config, db (ScopedPool), redis, nats, metrics
├── db/
│   ├── migrations/global/      # tenants, users — applied once
│   ├── migrations/tenant/      # clinical tables — applied per clinic
│   └── queries/                # sqlc sources
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
