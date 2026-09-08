# Frontend Plan — lahza-web (Next.js)

Status: saved plan, not started.

## Decisions locked

- **Stack**: Next.js (App Router) + React 18 + TypeScript
- **UI**: Tailwind + shadcn/ui components
- **Package manager**: pnpm
- **API access — "both"**:
  - Dev: `next.config` rewrites `/api/* -> http://localhost:8080/api/*`
  - Prod: single origin behind nginx proxying `/api/v1` to the Go API
  - Plus: gin CORS middleware in the backend (env-configured `ALLOWED_ORIGINS`, default disabled) as a separate follow-up PR
- **Repo**: new repo `PandaX185/lahza-web` (private, matching backend). Assumed name — confirm before creating.

## Backend contract facts the UI is built on (verified)

- Auth: `POST /api/v1/auth/login`, `POST /auth/refresh` (rotates — old token revoked), `POST /auth/logout` (revokes), `POST /auth/register`. Access token = 15m JWT, refresh token passed in body (no httpOnly cookie today).
- Bootstrap: `GET /auth/me` (user) + `GET /auth/clinics` (clinics + your role in each) -> clinic picker -> attach `X-Tenant-ID` to tenant-scoped calls.
- Route tiers:
  - Public (no auth): `/auth/register`, `/auth/login`, `/auth/refresh`, `/auth/logout`, `/booking/*` (clinic discovery, doctors, slots)
  - Global auth (JWT only, no tenant header): `GET /clinics`, `GET /clinics/mine`, `GET /auth/me`, `GET /auth/clinics`, `/portal/*` (appointments, queue, pay, me)
  - Tenant-scoped (JWT + `X-Tenant-ID`): catalog (`/profiles`, `/doctors`, `/appointment-types`), queue (`/queue`), scheduling (`/appointments` + status transitions), payments refunds, `POST /clinics/:id/staff`
  - Super-admin (global `users.is_admin`): `POST /clinics` (provisioning)
- Roles: `admin`, `staff`, `doctor`, `nurse`, `manager`, `patient`. Any signed-in user has patient-grade access everywhere.
- Typed client: generate from the backend's published `docs/swagger.json` via `openapi-typescript`; never hand-write DTOs. CI drift-checks the committed snapshot to catch backend breaking changes.

## M0 — scaffold + auth shell

1. `create-next-app` (TS + Tailwind + ESLint) + `shadcn init`; add CI (typecheck, lint, test, build) + swagger-snapshot drift check.
2. API client: generated types + fetch wrapper (Bearer + `X-Tenant-ID` injection, single-flight auto-refresh on 401, logout).
3. Session/auth context + `<ActiveClinicProvider>`; login/register pages; clinic-picker gate when a tenant-scoped route is hit.
4. Route guards: public / authenticated / tenant-required (+ role gates per section).
5. README with how to run against the local docker stack (rewrites -> localhost:8080).

## M1 — patient + public booking

- SSR/SEO landing: clinic discovery + doctor/slot availability (`/booking/*`, server components).
- Patient portal: appointment list/detail, book (from slots), cancel, reschedule, pay, queue join/view.

## M2 — staff console (tenant-scoped)

Today board, queue board (add + status PATCH), appointment transitions (confirm/complete/no-show), book-on-behalf, patient directory.

## M3 — admin

Staff roles (`BindStaff`), appointment types CRUD, doctors + schedules/exceptions, clinic list + provisioning (super-admin), payment refunds.

## M4 — hardening

httpOnly cookie session, Playwright smoke against the docker stack, error/i18n polish, rate-limit UX.

## Backend follow-up (separate PR in lahza)

- gin CORS middleware with `ALLOWED_ORIGINS` env, default disabled.