# Clinic Appointment System

> Production-grade backend for managing patients, doctors, schedules, and appointments — strong concurrency guarantees enforced in PostgreSQL, async notifications over NATS JetStream, full observability.

Implements the **Clinic Appointment & Management Backend SRS v1.0**.

---

## Architecture

Layered clean architecture: HTTP → service (domain rules) → repository (PostgreSQL). Cross-cutting infrastructure lives in `internal/platform`; each domain module owns its models, business logic, transport, and persistence behind narrow interfaces.

```
cmd/api/                      entrypoint: config, wiring, graceful shutdown
internal/
  platform/
    config/                   env-only configuration (caarlos0/env)
    logger/                   structured zap logging
    database/                 pgx pool
    redis/                    cache / rate limiting / idempotency support
    nats/                     JetStream connection + stream provisioning
    metrics/                  Prometheus registry + collectors
    apperr/                   typed domain errors → HTTP mapping
    db/sqlc/                  generated data access (sqlc, pgx/v5)
  auth/                       register/login/refresh, JWT HS256, RBAC middleware
  patient/                    patient CRUD + search
  doctor/                     profiles, weekly schedules, exceptions, availability engine
  appointment/                booking (tx-safe), lifecycle state machine, reschedule/cancel
  notification/               event forwarder, durable JetStream worker, retry/DLQ
  server/                     router assembly, middleware chain, health endpoints
db/
  migrations/                 versioned SQL (golang-migrate)
  queries/                    sqlc sources per domain
```

### Concurrency & integrity model

| Concern | Mechanism | SRS |
|---|---|---|
| Double booking | `no_overlapping_appointments` exclusion constraint (GiST on `doctor_id` + `tstzrange`) — DB rejects overlaps even under fully concurrent requests | BR-01, FR-APT-05/06 |
| Client retries | `Idempotency-Key` header → response persisted in `idempotency_keys` inside the same transaction as the booking; racing requests replay after commit | BR-07, FR-APT-07 |
| State machine | explicit transition table (`scheduled → confirmed → completed/cancelled/no_show`) + optimistic `version` column | BR-03/04 |
| Availability | recurring weekly schedules − schedule exceptions − booked appointments, computed per slot | FR-DOC-03 |

---

## Tech Stack

| Layer         | Technology |
|---------------|------------|
| Language      | Go 1.25 |
| Router        | Gin |
| Database      | PostgreSQL 16 + pgx/v5 + sqlc |
| Migrations    | golang-migrate |
| Cache         | Redis 7 |
| Message Bus   | NATS JetStream (work-queue retention, dedupe via `Nats-Msg-Id`, DLQ) |
| Auth          | JWT HS256 + bcrypt |
| Observability | Zap (structured JSON), Prometheus `/metrics`, health/readiness probes |
| CI            | GitHub Actions (fmt, vet, sqlc drift check, race tests) |

---

## Quick Start

```bash
cp .env.example .env          # set JWT_SECRET / JWT_REFRESH_SECRET (openssl rand -hex 32)

docker compose up -d postgres redis nats   # or: docker compose up -d  (full stack)
make migrate-up
make run

curl http://localhost:8080/health
curl http://localhost:8080/ready
```

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| DATABASE_URL | required | PostgreSQL connection string |
| REDIS_URL | redis://localhost:6379/0 | Redis connection |
| NATS_URL | nats://localhost:4222 | NATS JetStream |
| JWT_SECRET | required | Access-token signing key |
| JWT_REFRESH_SECRET | required | Refresh-token signing key |
| ACCESS_TOKEN_TTL / REFRESH_TOKEN_TTL | 15m / 168h | Token lifetimes |
| PORT | 8080 | HTTP listen port |
| LOG_LEVEL / LOG_FORMAT | info / json | Logging |
| RATE_LIMIT_PER_MINUTE | 60 | Per-IP fixed window |
| IDEMPOTENCY_TTL | 24h | Booking replay window |

---

## API Overview

Base path: `/api/v1`

| Resource | Endpoints |
|----------|-----------|
| Auth | POST /auth/register · POST /auth/login · POST /auth/refresh · GET /auth/me |
| Patients *(staff)* | POST/PATCH/DELETE /patients · GET /patients · GET /patients/{id} |
| Doctors *(admin/staff writes)* | GET /doctors · GET /doctors/{id}/availability?from&to · POST/{id}/schedules · POST/{id}/exceptions |
| Appointments | POST /appointments (`Idempotency-Key` honored) · GET /appointments?patient_id&status&from&to · GET /{id} · POST /{id}/cancel · POST /{id}/reschedule · POST /{id}/confirm · POST /{id}/complete · POST /{id}/no-show |
| Ops | GET /health · GET /ready · GET /metrics |

Errors follow one shape: `{"error": "..."}` with status from the typed domain error (400/401/403/404/409/500).

---

## Testing

```bash
make test-race       # all tests with race detector
make test-coverage   # HTML coverage report
go test ./internal/appointment/ -run TestCanTransition -v
```

Critical invariants under test: lifecycle transitions (terminal states frozen), availability slot expansion/exception cutting/past filtering, JWT type confusion and cross-secret rejection.

---

## License

MIT — see [LICENSE](LICENSE).
