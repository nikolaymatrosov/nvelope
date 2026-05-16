---
description: "Task list for Phase 0 — Foundations & Docs"
---

# Tasks: Phase 0 — Foundations & Docs

**Input**: Design documents from `/specs/001-phase0-foundations/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/

**Tests**: Test tasks ARE included. The Constitution (Principle II — Test-Backed Delivery, NON-NEGOTIABLE) requires automated coverage, and plan.md explicitly scopes config, logging, lifecycle, and migration tests. The phase exit criteria require a green suite.

**Organization**: Tasks are grouped by user story so each story can be implemented and tested independently.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[Story]**: Which user story this task belongs to (US1–US4)
- All paths are relative to the repository root

## Path Conventions

Web-application monorepo per plan.md: single Go module at the repo root with
`cmd/`, `internal/`, `test/`; isolated `frontend/` package; `deploy/` for containers and manifests.

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Repository scaffolding and tooling so all later phases have a place to land.

- [x] T001 Create the monorepo directory structure (`cmd/{api,worker,scheduler,migrate}`, `internal/{config,logging,service,db,health}`, `internal/db/migrations`, `test/`, `frontend/`, `deploy/{docker,k8s,helm}`) per plan.md
- [x] T002 Initialize the Go module: create `go.mod` (module path, Go 1.25) and add backend dependencies (`github.com/go-chi/chi/v5`, `github.com/jackc/pgx/v5`, `github.com/golang-migrate/migrate/v4`, `github.com/knadh/koanf/v2`, `github.com/stretchr/testify`)
- [x] T003 [P] Configure `golangci-lint` in `.golangci.yml`
- [x] T004 [P] Create `Makefile` with `build`, `run-api`, `run-worker`, `run-scheduler`, `test`, and `lint` targets (migrate targets added in T025)
- [x] T005 [P] Create `docker-compose.yml` at the repo root with a PostgreSQL 17 service for local development
- [x] T006 [P] Create `.env.example` documenting every `NVELOPE_*` variable from `contracts/service-config.md`
- [x] T007 [P] Create `.gitignore` covering Go build artifacts, `frontend/node_modules`, `frontend/dist`, and `.env`

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Shared `internal/` packages that every backend service and the migration CLI depend on.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete.

- [x] T008 [P] Implement the shared config package in `internal/config/config.go`: typed `Config` struct, koanf load from `NVELOPE_`-prefixed env + optional file, and `Validate()` returning an aggregated error that names every missing/invalid required field (secrets never echoed)
- [x] T009 [P] Write config tests in `internal/config/config_test.go`: valid config loads; missing `NVELOPE_DATABASE_URL` fails naming the variable; invalid `NVELOPE_LOG_LEVEL` fails; the DSN value never appears in error text
- [x] T010 [P] Implement the shared logging package in `internal/logging/logging.go`: build a `*slog.Logger` with a JSON handler, configurable level, and a `service` attribute
- [x] T011 [P] Write logging tests in `internal/logging/logging_test.go`: emitted lines are valid JSON containing `time`, `level`, `msg`, and `service`
- [x] T012 Implement the PostgreSQL pool in `internal/db/db.go`: open/close a `pgxpool` from the config DSN with a connectivity ping (depends on T008)

**Checkpoint**: Shared foundation ready — user stories can now begin.

---

## Phase 3: User Story 1 - Backend services build and run locally (Priority: P1) 🎯 MVP

**Goal**: Three Go services (API, Worker, Scheduler) build, start with shared config and structured logging, report health, and shut down gracefully.

**Independent Test**: On a clean clone, run `make build` then start each service; confirm each emits a structured startup log line, the API answers `/healthz` with `200`, `Ctrl-C` shuts each down cleanly, and a service started with a missing required variable exits non-zero naming that variable.

### Implementation for User Story 1

- [x] T013 [US1] Implement the shared service lifecycle in `internal/service/service.go`: load+validate config, build logger, log a structured startup line with service name and build version, run the service's work, block on `SIGINT`/`SIGTERM`, and drain within `Config.ShutdownTimeout` (depends on T008, T010)
- [x] T014 [P] [US1] Write lifecycle tests in `internal/service/service_test.go`: graceful shutdown leaves no work running; startup log line carries `service` and `version`; a missing required config value causes a non-zero exit before work starts
- [x] T015 [US1] Implement the health handler in `internal/health/health.go` per `contracts/health-endpoint.md`: `200` JSON when ready, `503` JSON while starting or draining
- [x] T016 [P] [US1] Write health handler tests in `internal/health/health_test.go`: ready → `200` with `status/service/version`; draining → `503`
- [x] T017 [P] [US1] Implement the API service entrypoint in `cmd/api/main.go`: wire config, logging, db pool, and the service lifecycle; mount a `chi` router serving `GET /healthz` (depends on T012, T013, T015)
- [x] T018 [P] [US1] Implement the Worker service entrypoint in `cmd/worker/main.go`: wire config, logging, db pool, and the service lifecycle with an idle run loop and a Phase 1 TODO marker (depends on T012, T013)
- [x] T019 [P] [US1] Implement the Scheduler service entrypoint in `cmd/scheduler/main.go`: wire config, logging, db pool, and the service lifecycle with an idle run loop and a Phase 1 TODO marker (depends on T012, T013)
- [x] T020 [US1] Wire build-version injection (`-ldflags -X`) into the `Makefile` `build` target so each service reports its version
- [x] T021 [US1] Manually verify User Story 1 against `quickstart.md` §4: build, run all three, check `/healthz`, graceful shutdown, and fail-fast on a missing variable

**Checkpoint**: All three services build, run, log, and shut down cleanly — MVP is demonstrable.

---

## Phase 4: User Story 2 - Database schema can be versioned and migrated (Priority: P2)

**Goal**: Versioned migrations apply forward and revert backward against PostgreSQL, with a baseline migration that round-trips cleanly.

**Independent Test**: Against an empty PostgreSQL database, run `migrate up` (baseline applies, version recorded), `migrate up` again (no-op), `migrate down` (reverts), and `migrate create <name>` (generates a numbered up/down pair).

### Implementation for User Story 2

- [x] T022 [P] [US2] Create the baseline migration pair `internal/db/migrations/000001_baseline.up.sql` (`CREATE EXTENSION IF NOT EXISTS pgcrypto`) and `000001_baseline.down.sql` (`DROP EXTENSION IF EXISTS pgcrypto`)
- [x] T023 [US2] Implement the migration CLI in `cmd/migrate/main.go` wrapping `golang-migrate`: `up`, `down`, `version`, and `create` subcommands per `contracts/service-config.md`, reading `NVELOPE_DATABASE_URL` (depends on T008)
- [x] T024 [P] [US2] Write the migration integration test in `test/migrate_test.go`: against a real PostgreSQL instance, apply→assert version 1→revert→assert version 0→re-apply, all clean
- [x] T025 [US2] Add `migrate-up`, `migrate-down`, `migrate-version`, and `migrate-create` targets to the `Makefile`
- [x] T026 [US2] Document the migration authoring process (file naming, location, `migrate create` workflow) in `quickstart.md` and a short note in `docs/`

**Checkpoint**: Migration workflow proven end-to-end; the baseline applies and reverts cleanly.

---

## Phase 5: User Story 3 - Every change is automatically validated by CI (Priority: P2)

**Goal**: An automated pipeline builds, tests, and lints every change and reports a single pass/fail status.

**Independent Test**: Push the branch / open a PR; confirm the pipeline runs automatically, builds + tests + lints backend and frontend, applies the baseline migration against a PostgreSQL service container, and reports green. Introduce a deliberate failure and confirm it reports red with the failing step identifiable.

### Implementation for User Story 3

- [x] T027 [US3] Create `.github/workflows/ci.yml` triggered on push and pull request: a backend job (`go build ./...`, `go test ./...` with a PostgreSQL service container, `golangci-lint run`), a migration step applying the baseline against that container, and a frontend job (`npm ci`, `npm run lint`, `npm run test`, `npm run build`)
- [ ] T028 [US3] Verify the pipeline: confirm a green run on the Phase 0 codebase, then confirm a deliberately introduced build/test/lint failure produces a red run with the failing step identifiable — BLOCKED: requires a GitHub remote + push; every command the workflow runs (go build/test, migrate, golangci-lint, pnpm lint/test/build) passes locally

**Checkpoint**: Every change is gated by an automated build/test/lint pipeline.

---

## Phase 6: User Story 4 - Frontend skeleton and deployment artifacts exist (Priority: P3)

**Goal**: A runnable React/Vite frontend skeleton plus per-service container images and base Kubernetes/Helm manifests.

**Independent Test**: Run the frontend dev server and view the placeholder app; run the production build; build a container image for each backend service; dry-run-apply the K8s manifests and `helm lint` the chart.

### Implementation for User Story 4

- [x] T029 [P] [US4] Scaffold the frontend in `frontend/`: `package.json`, `vite.config.ts`, `tsconfig.json`, `index.html`, `src/main.tsx`, and `src/App.tsx` rendering a placeholder "nvelope" view (React 19 + TypeScript + Vite 6)
- [x] T030 [P] [US4] Add the frontend smoke test `frontend/src/App.test.tsx` and configure `vitest` (test script in `package.json`)
- [x] T031 [P] [US4] Configure ESLint + Prettier for the frontend (`frontend/.eslintrc`/`eslint.config.js`, `.prettierrc`, `lint` script in `package.json`)
- [x] T032 [P] [US4] Create the multi-stage `deploy/docker/api.Dockerfile` (Go builder → minimal non-root runtime)
- [x] T033 [P] [US4] Create the multi-stage `deploy/docker/worker.Dockerfile` (Go builder → minimal non-root runtime)
- [x] T034 [P] [US4] Create the multi-stage `deploy/docker/scheduler.Dockerfile` (Go builder → minimal non-root runtime)
- [x] T035 [P] [US4] Create base Kubernetes `Deployment` manifests `deploy/k8s/{api,worker,scheduler}-deployment.yaml`, with liveness/readiness probes on `/healthz` for the API
- [x] T036 [US4] Create the skeleton Helm chart in `deploy/helm/nvelope/` (`Chart.yaml`, `values.yaml`, templated deployments) and confirm `helm lint` passes

**Checkpoint**: Frontend skeleton runs and builds; container images build; manifests validate.

---

## Phase 7: Polish & Cross-Cutting Concerns

**Purpose**: Developer-facing documentation and end-to-end verification of the phase exit criteria.

- [x] T037 [P] Write developer setup documentation in `README.md` covering build/run/migrate/frontend steps and required tool versions (FR-017)
- [x] T038 Verify the design docs (`docs/architecture.md`, `docs/user-stories.md`, `docs/implementation-plan.md`) are committed under version control (FR-001)
- [x] T039 Run the full `quickstart.md` validation and tick its exit-criteria checklist
- [ ] T040 Confirm CI reports green on the branch (phase exit criterion) — BLOCKED: requires a GitHub remote + push

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — start immediately.
- **Foundational (Phase 2)**: Depends on Setup — BLOCKS all user stories.
- **User Story 1 (Phase 3)**: Depends on Foundational. No dependency on other stories.
- **User Story 2 (Phase 4)**: Depends on Foundational. Independent of US1/US3/US4.
- **User Story 3 (Phase 5)**: Depends on Foundational. The backend pipeline is independently testable; the frontend CI job is only fully exercised once US4 lands (soft dependency — US3 may be authored first and verified end-to-end after US4).
- **User Story 4 (Phase 6)**: Depends on Foundational. Container/manifest tasks reference the services from US1 but the artifacts themselves are independent.
- **Polish (Phase 7)**: Depends on all desired user stories being complete.

### User Story Dependencies

- **US1 (P1)**: Foundational only — fully independent. This is the MVP.
- **US2 (P2)**: Foundational only — fully independent of US1.
- **US3 (P2)**: Foundational only — backend pipeline independent; frontend job exercised after US4.
- **US4 (P3)**: Foundational only — independently testable (frontend dev/build, image build, manifest lint).

### Within Each User Story

- Shared lifecycle (T013) before service entrypoints (T017–T019).
- Baseline migration (T022) before the migration test (T024).
- Models/handlers before the code that wires them.

### Parallel Opportunities

- Setup: T003–T007 run in parallel after T001/T002.
- Foundational: T008–T011 run in parallel; T012 follows T008.
- Once Foundational completes, US1/US2/US3/US4 can be staffed in parallel.
- US1: T014 and T016 (tests) parallel; T017–T019 (entrypoints) parallel after T013.
- US4: T029–T035 are nearly all parallel (different files).

---

## Parallel Example: Foundational Phase

```bash
# After T001 + T002, launch the shared packages together:
Task: "Implement config package in internal/config/config.go"
Task: "Write config tests in internal/config/config_test.go"
Task: "Implement logging package in internal/logging/logging.go"
Task: "Write logging tests in internal/logging/logging_test.go"
```

## Parallel Example: User Story 1

```bash
# After T013 (service lifecycle), launch the three entrypoints together:
Task: "Implement API service entrypoint in cmd/api/main.go"
Task: "Implement Worker service entrypoint in cmd/worker/main.go"
Task: "Implement Scheduler service entrypoint in cmd/scheduler/main.go"
```

---

## Implementation Strategy

### MVP First (User Story 1 only)

1. Complete Phase 1: Setup.
2. Complete Phase 2: Foundational (CRITICAL — blocks all stories).
3. Complete Phase 3: User Story 1.
4. **STOP and VALIDATE**: build/run all three services, check health and graceful shutdown.
5. This is a demonstrable MVP — the project's "all three services build and start" exit criterion is met.

### Incremental Delivery

1. Setup + Foundational → foundation ready.
2. US1 → three runnable services (MVP).
3. US2 → migration workflow ("a migration applies cleanly" exit criterion).
4. US3 → CI pipeline ("CI is green" exit criterion).
5. US4 → frontend skeleton + deployment artifacts.
6. Polish → setup docs and full quickstart verification.

### Parallel Team Strategy

After Foundational completes: Developer A → US1, Developer B → US2, Developer C → US3+US4 (US3's frontend job verified once US4 lands).

---

## Notes

- [P] tasks = different files, no dependencies on incomplete tasks.
- Phase 0 is skeletons only — no tenancy, auth, sending, or billing logic (Constitution Principle III, YAGNI).
- Migration and DB-touching tests run against a real PostgreSQL instance, never mocks (Constitution Principle II).
- Commit after each task or logical group.
- Phase exit criteria: all three services build and start; CI is green; a migration applies cleanly.
