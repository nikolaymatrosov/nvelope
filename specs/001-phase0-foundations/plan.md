# Implementation Plan: Phase 0 — Foundations & Docs

**Branch**: `001-phase0-foundations` | **Date**: 2026-05-16 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/001-phase0-foundations/spec.md`

## Summary

Stand up the nvelope monorepo skeleton: three runnable Go services (`api`, `worker`,
`scheduler`) sharing one config and structured-logging package, a PostgreSQL connection
with `golang-migrate` versioned migrations and a clean baseline migration, a React/Vite
frontend skeleton, a CI pipeline that builds/tests/lints every change, per-service
Dockerfiles, and base Kubernetes manifests plus a skeleton Helm chart. The phase delivers
skeletons only — no tenancy, auth, sending, or billing logic — so later phases have a
predictable, verified place to add code.

## Technical Context

**Language/Version**: Go 1.26 (backend services, migration runner); TypeScript 5.x on Node.js 22 LTS (frontend)

**Primary Dependencies**:
- Backend: `chi` (HTTP router, API service only), `jackc/pgx/v5` (PostgreSQL driver/pool), `golang-migrate/migrate/v4` (migrations, library + CLI), `knadh/koanf/v2` (config from env + optional file), `log/slog` (stdlib structured logging)
- Frontend: React 19, Vite 6, `vitest` + React Testing Library
- Tooling: `golangci-lint` (Go lint), ESLint + Prettier (frontend lint)

**Storage**: PostgreSQL 17 (single database; Phase 0 creates only the `golang-migrate` version table and one baseline migration enabling the `pgcrypto` extension)

**Testing**: Go stdlib `testing` + `testify/require` for backend; `vitest` for frontend; migration apply/revert verified against a real PostgreSQL instance

**Target Platform**: Linux containers on Kubernetes; services build and run on macOS/Linux for development

**Project Type**: Web application — multi-service Go backend monorepo + React frontend in one repository

**Performance Goals**: None for Phase 0 (skeletons). Operational baseline only: each service starts in < 5 s and shuts down gracefully within a 10 s drain window.

**Constraints**: Services MUST be stateless; configuration MUST fail fast on missing/invalid required values; secrets (DB password) MUST never appear in logs; the app connects to PostgreSQL as a non-superuser role.

**Scale/Scope**: 3 backend service binaries, 1 shared internal config package, 1 shared logging package, 1 baseline migration, 1 frontend skeleton, 1 CI workflow, 3 Dockerfiles, base K8s manifests + skeleton Helm chart. No business endpoints beyond health checks.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Assessment | Status |
|---|---|---|
| I. Tenant Isolation by Default | Phase 0 introduces no tenant-scoped data or tables. The baseline migration only enables a database extension. The control-plane / tenant-plane split begins in Phase 1. No isolation surface exists to violate. | PASS (N/A this phase) |
| II. Test-Backed Delivery (NON-NEGOTIABLE) | Plan includes automated tests: config loading (valid + missing/invalid required values), structured-log field presence, graceful-shutdown behavior, and a migration apply→revert→apply test against a real PostgreSQL instance. CI runs the full suite + lint + a clean migration apply. Phase exits green. | PASS |
| III. Incremental, Shippable Phases | Phase 0 is the first independently shippable increment. No River queue, no RLS, no auth, no sending — those are explicitly later phases. Build for this phase only (YAGNI). | PASS |
| IV. Security & Consent by Design | No auth surface this phase. Security groundwork respected: DB password is a config secret never logged; the app connects as a non-superuser PostgreSQL role from the start; container images run as a non-root user. | PASS |
| V. Operable & Observable Services | Directly served by this phase: all three services are stateless, emit structured JSON logs via `slog`, expose a health/readiness signal, and shut down gracefully on `SIGTERM`/`SIGINT`. | PASS |

**Result**: PASS — no violations, Complexity Tracking not required. Re-checked after Phase 1 design: still PASS (design introduces no new dependencies or structures beyond those evaluated above).

## Project Structure

### Documentation (this feature)

```text
specs/001-phase0-foundations/
├── plan.md              # This file
├── spec.md              # Feature specification
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/           # Phase 1 output
│   ├── health-endpoint.md
│   └── service-config.md
├── checklists/
│   └── requirements.md
└── tasks.md             # Phase 2 output (/speckit-tasks — not created here)
```

### Source Code (repository root)

```text
nvelope/
├── go.mod                       # single Go module for the backend monorepo
├── go.sum
├── Makefile                     # build / run / test / lint / migrate targets
├── cmd/
│   ├── api/main.go              # API service entrypoint
│   ├── worker/main.go           # Worker service entrypoint
│   ├── scheduler/main.go        # Scheduler service entrypoint
│   └── migrate/main.go          # golang-migrate CLI wrapper (up/down/create/version)
├── internal/
│   ├── config/                  # shared config: load from env (+ optional file), validate, fail fast
│   │   ├── config.go
│   │   └── config_test.go
│   ├── logging/                 # shared slog setup (JSON handler, service field)
│   │   ├── logging.go
│   │   └── logging_test.go
│   ├── service/                 # shared run loop: startup, health, graceful shutdown
│   │   ├── service.go
│   │   └── service_test.go
│   └── db/
│       ├── db.go                # pgx pool open/close
│       └── migrations/
│           ├── 000001_baseline.up.sql
│           └── 000001_baseline.down.sql
├── test/
│   └── migrate_test.go          # apply→revert→apply against real PostgreSQL
├── frontend/
│   ├── package.json
│   ├── vite.config.ts
│   ├── tsconfig.json
│   ├── index.html
│   └── src/
│       ├── main.tsx
│       ├── App.tsx
│       └── App.test.tsx
├── deploy/
│   ├── docker/
│   │   ├── api.Dockerfile
│   │   ├── worker.Dockerfile
│   │   └── scheduler.Dockerfile
│   ├── k8s/
│   │   ├── api-deployment.yaml
│   │   ├── worker-deployment.yaml
│   │   └── scheduler-deployment.yaml
│   └── helm/
│       └── nvelope/             # skeleton chart: Chart.yaml, values.yaml, templates/
├── docker-compose.yml           # local PostgreSQL for development
├── .github/workflows/ci.yml     # build + test + lint pipeline
├── .golangci.yml
├── .env.example
└── docs/                        # already committed: architecture.md, user-stories.md, implementation-plan.md
```

**Structure Decision**: Web-application monorepo. A single Go module at the repo root
hosts all three services plus the migration CLI, sharing `internal/config`,
`internal/logging`, and `internal/service`. The frontend is an isolated `frontend/`
package. This matches the target layout in `docs/architecture.md` §10 and keeps later
phases' code in predictable locations.

## Complexity Tracking

No constitution violations — section intentionally empty.
