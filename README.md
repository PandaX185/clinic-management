# Clinic Appointment System

> Production-grade backend for managing patients, doctors, schedules, and appointments with strong concurrency guarantees, async notifications, and full observability.

---

## Architecture

```
+-----------------------------------------------------------------+
|                        Clinic Appointment API                   |
+-----------------------------------------------------------------+
|  REST API (Chi)  |  JWT Auth  |  OpenAPI 3.1  |  Observability  |
+-----------------------------------------------------------------+
|  Domain Layer:  Patients | Doctors | Appointments | Schedules    |
+-----------------------------------------------------------------+
|  Data Layer:    PostgreSQL (uuidv7, exclusion constraints)      |
|  Cache:         Redis (sessions, rate limiting, idempotency)    |
|  Message Bus:   NATS JetStream (async notifications, DLQ)       |
+-----------------------------------------------------------------+
```

## Features

| Domain        | Capabilities |
|---------------|--------------|
| Patients      | CRUD, search, medical notes, emergency contacts |
| Doctors       | Profiles, specializations, schedules, availability |
| Appointments  | Booking, cancellation, rescheduling, concurrency-safe (exclusion constraint) |
| Schedules     | Recurring patterns, exceptions (holidays, sick days) |
| Auth          | JWT (access + refresh), role-based access (Patient, Doctor, Staff, Admin) |
| Notifications | Async via NATS JetStream, retries, DLQ, idempotency |
| Observability | Structured logging, Prometheus metrics, OpenTelemetry tracing, health/ready endpoints |

---

## Tech Stack

| Layer         | Technology |
|---------------|------------|
| Language      | Go 1.22+ |
| Router        | Chi v5 |
| Database      | PostgreSQL 16 + pgx + sqlc |
| Migrations    | golang-migrate |
| Cache         | Redis 7 |
| Message Bus   | NATS JetStream |
| Auth          | JWT (RS256) + bcrypt |
| Observability | OpenTelemetry, Prometheus, Zap |
| CI/CD         | GitHub Actions |
| Container     | Docker + Docker Compose |

---

## Quick Start

### Prerequisites

- Go 1.22+
- Docker + Docker Compose
- PostgreSQL 16 (or use Docker)
- Redis 7 (or use Docker)
- NATS JetStream (or use Docker)

### Local Development

```bash
# Clone and enter
git clone https://github.com/PandaX185/clinic-management.git
cd clinic-management

# Start dependencies
docker-compose up -d

# Run migrations
make migrate-up

# Start server
make run

# Health check
curl http://localhost:8080/health
curl http://localhost:8080/ready
```

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| DATABASE_URL | postgres://clinic:clinic@localhost:5432/clinic?sslmode=disable | PostgreSQL connection |
| REDIS_URL | redis://localhost:6379 | Redis connection |
| NATS_URL | nats://localhost:4222 | NATS JetStream connection |
| JWT_SECRET | required | JWT signing secret |
| JWT_REFRESH_SECRET | required | Refresh token secret |
| LOG_LEVEL | info | Log level |
| PORT | 8080 | HTTP port |

---

## API Documentation

| Resource | Endpoints |
|----------|-----------|
| Auth | POST /api/v1/auth/login, POST /api/v1/auth/refresh |
| Patients | POST /api/v1/patients, GET /api/v1/patients/{id}, PATCH /api/v1/patients/{id} |
| Doctors | POST /api/v1/doctors, GET /api/v1/doctors, GET /api/v1/doctors/{id}/availability |
| Appointments | POST /api/v1/appointments, GET /api/v1/appointments, GET /api/v1/appointments/{id}, POST /api/v1/appointments/{id}/cancel, POST /api/v1/appointments/{id}/reschedule |
| Health | GET /health, GET /ready |

> **Full OpenAPI 3.1 spec:** [api/openapi.yaml](api/openapi.yaml) (work in progress)

---

## Testing

```bash
# Run all tests with race detector
make test-race

# Run specific package
go test ./internal/... -race

# Coverage report
make test-coverage
```

---

## Docker

```bash
# Build image
make docker-build

# Run with Docker Compose
docker-compose up -d

# View logs
docker-compose logs -f api
```

---

## Project Structure

```
clinic-management/
├── cmd/
│   └── api/                 # Application entry point
├── internal/
│   ├── auth/                # JWT auth, login/refresh, middleware
│   ├── appointment/         # Appointment domain logic
│   ├── doctor/              # Doctor domain logic
│   ├── patient/             # Patient domain logic
│   ├── notification/        # NATS JetStream notifications
│   ├── platform/            # Shared: config, db, redis, nats, logging, metrics, tracing
│   └── ...                  # Other domains (patient, doctor, etc.)
├── migrations/              # SQL migrations (golang-migrate)
├── api/
│   └── openapi.yaml         # OpenAPI 3.1 specification
├── .github/
│   ├── workflows/           # GitHub Actions CI
│   └── pull_request_template.md
├── docker-compose.yml
├── Dockerfile
├── Makefile
├── sqlc.yaml
├── go.mod / go.sum
└── CONTRIBUTING.md
```

---

## Security

- **Passwords**: bcrypt (cost 12)
- **Tokens**: JWT RS256, short-lived access (15m) + rotating refresh (7d)
- **Concurrency**: PostgreSQL exclusion constraints (btree_gist)
- **Idempotency**: Redis-backed keys with TTL
- **Rate limiting**: Redis-backed, per-IP and per-user
- **Secrets**: Environment variables only, never in code

---

## Observability

| Signal | Implementation |
|--------|---------------|
| Logs | Structured JSON (Zap), request IDs, correlation IDs |
| Metrics | Prometheus (HTTP latency, DB latency, cache hit/miss, queue depth) |
| Traces | OpenTelemetry (HTTP, DB, Redis, NATS spans) |
| Health | /health (liveness), /ready (readiness + deps) |

---

## License

MIT License — see [LICENSE](LICENSE) for details.

---

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for development workflow, branch strategy, and PR process.

---

## Contact

**Clinic Appointment System** — Production-grade healthcare backend  
GitHub: [PandaX185/clinic-management](https://github.com/PandaX185/clinic-management)