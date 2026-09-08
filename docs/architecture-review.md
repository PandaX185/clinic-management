# Architecture Review — findings + ranked plan

Date: 2026-09-07. Discussion outcome; P0 items implemented in a follow-up commit.

## Verdict

Well-architected for the target scale (a handful to dozens of clinics, single
Postgres). The hard guarantees are solved at the DB layer, which is where they
belong:

- Schema-per-tenant isolation via transaction-local `set_config('search_path')`
  — no connection leaks across tenants (`internal/platform/database/scoped.go`).
- Double-booking blocked by a GiST exclusion constraint; payment idempotency by
  `payments.appointment_id UNIQUE`; booking idempotency by the `idempotency_keys`
  PK — DB-enforced, not app-enforced.
- Appointment state machine (`scheduling/service/model.go`) + versioned CAS
  transitions.
- Shared, bounded pgxpool (20 conns), no per-tenant pools.

The scaling axis is the *number of tenants*: portal/booking do fan-out reads
across every tenant schema (one transaction per clinic). API instances scale out
trivially (stateless); the single Postgres is the ceiling — acceptable for this
domain if deliberate.

## Findings

### Definite bugs (P0)
1. **Idempotency cleaner never cleans.** `IdempotencyCleaner.Run` deletes via
   `db.New(c.pool)` on the raw pool (`scheduling/repo/cleaner.go:45`), i.e. the
   default `public` search path — but `idempotency_keys` lives in `tenant_<slug>`
   schemas. It errors every tick; the table grows unbounded.
2. **Payments double-publish race.** Two concurrent Pay requests can both reach
   the `appointment.paid` publish (loser of `MarkPaid` re-reads and falls through,
   `payments/service/service.go:94-120`) → duplicate event.
3. **Notification publish errors silently dropped** (`notification/types.go`:
   marshal + publish errors ignored) — events vanish with no log or metric.

### Design weak spots (P1/P2)
- No Prometheus/Grafana/alerting anywhere; `/metrics` has no Go runtime or DB
  pool stats (`metrics.go:21` — custom registry excludes default collectors).
- Nats DLQ has no consumer/alarm; dead letters accumulate silently.
- `TenantMiddleware` resolves the slug per request, uncached (`server/clinic.go:103`).
- Portal/booking fan-out across all clinics rather than the user's memberships
  (`config` count grows the tx count linearly): `ListAppointments`, `ListMyQueues`,
  `FindDoctor`.
- Memberships N+1 (`clinic/repo/postgres.go:73-90` + `wiring/memberships.go`).
- `RescheduleAppointment` has no status CAS (`WHERE id=$1` only).
- No pagination on portal appointment list, `ListProfiles`, queue list.
- `queue_entries.profile_id` unindexed (used by portal my-queue).
- Pool limits hardcoded (20/2), no `pgxpool.Stat` visibility.
- Cleaner interval == TTL (default 24h) → keys live up to ~48h.
- `ProvisionTenant` re-runs migration SQL with no version tracking.
- Rate limiter is fixed-window per-IP (60/min); fail-open is deliberate.
- CORS absent (tracked in `docs/frontend-plan.md`).

## Ranked plan

- **P0 — correctness** (done in this branch):
  1. Idempotency cleaner sweeps every tenant schema.
  2. Payments publish `appointment.paid` only when this request wins the
     `pending -> paid` transition.
  3. Surface notification publish failures (log + metric).
- **P1 — observability:** runtime + DB-pool + NATS metrics; Prometheus/Grafana
  in compose; alerting; DLQ consumer/alarm.
- **P2 — scaling hygiene:** cache slug resolution; scope fan-out to user
  memberships; pagination on list endpoints; memberships N+1 fix;
  `queue_entries(profile_id)` index; env-driven pool limits; reschedule status
  CAS.
- **P3 — hardening:** per-user login rate limit; real notifier (SMS/email);
  outbox for notification events (durable, also removes the P0.2 class entirely);
  tenant migration versioning; CORS.

### Recommended follow-up worth calling out
The durable fix for the payments-notification problem is a **DB outbox**: write
the outbound event in the same transaction as the `pending -> paid` update, and
let a relay worker publish to NATS. It removes both the double-publish race and
the drop-on-NATS-failure gap. Proposed as P3/P2 work, not done here.