# Phase 0 Research: Foundations & Docs

All decisions below resolve the Technical Context. The architecture in `docs/architecture.md`
locks the broad stack (Go, React/Vite, PostgreSQL, `golang-migrate`, Kubernetes); this
research selects the concrete libraries and conventions for the Phase 0 skeleton. No
`NEEDS CLARIFICATION` markers remained from the spec.

## 1. Go version and module layout

- **Decision**: Go 1.25, single Go module rooted at the repo, three `main` packages under
  `cmd/` plus a `cmd/migrate` CLI wrapper. Shared code lives in `internal/`.
- **Rationale**: One module keeps shared `config`/`logging`/`service` packages trivially
  importable and gives CI one `go build ./...` / `go test ./...`. Three thin `main`
  packages keep services independently buildable and containerizable. Matches
  `architecture.md` §10.
- **Alternatives considered**: Go workspace with one module per service — rejected as
  premature; the services share substantial code and there is no versioning need. Separate
  repos — rejected; the implementation plan and architecture assume a monorepo.

## 2. Configuration loading

- **Decision**: `knadh/koanf/v2` with an environment-variable provider (prefix `NVELOPE_`)
  and an optional `.env`/file provider, decoded into a typed `Config` struct in
  `internal/config`. A `Validate()` method checks required fields and returns an aggregated
  error naming each offending key; services call it at startup and exit non-zero on failure.
- **Rationale**: A single shared package gives all three services identical config
  behavior (FR-003). koanf is lightweight, supports env + file layering, and has no heavy
  transitive tree. Fail-fast validation satisfies FR-004 and SC-003.
- **Alternatives considered**: `spf13/viper` — heavier, larger dependency surface than
  needed. Hand-rolled `os.Getenv` parsing — rejected; no typed decoding or layered
  file support, and validation would be duplicated per service. `caarlos0/env` — viable
  but file-source layering is weaker than koanf.

## 3. Structured logging

- **Decision**: Standard-library `log/slog` with the JSON handler. `internal/logging`
  builds a `*slog.Logger` carrying a `service` attribute and a configurable level; output
  is JSON to stdout.
- **Rationale**: `slog` is in the stdlib (zero dependency), emits machine-parseable JSON
  with timestamp/level/message, and attribute-carrying loggers satisfy the `service`
  identifier requirement (FR-005). Aligns with Constitution Principle V.
- **Alternatives considered**: `zerolog` / `zap` — faster, but Phase 0 has no logging hot
  path and stdlib avoids a dependency. Can be revisited only if profiling later demands it.

## 4. Service lifecycle (startup, health, graceful shutdown)

- **Decision**: A shared `internal/service` run loop: load+validate config, build logger,
  start the service's work (HTTP server for `api`; idle loops with a TODO marker for
  `worker`/`scheduler` in Phase 0), log a structured startup line with service name and
  build version, then block on `SIGINT`/`SIGTERM` and shut down within a bounded drain
  context.
- **Rationale**: Centralizes the lifecycle so all three services behave identically
  (FR-006), and graceful shutdown with no orphaned processes satisfies SC-002. Build
  version is injected via `-ldflags -X`.
- **Alternatives considered**: Per-service bespoke `main` — rejected; duplicates lifecycle
  logic and drifts. A supervisor framework — rejected as overkill (YAGNI, Principle III).

## 5. PostgreSQL access

- **Decision**: `jackc/pgx/v5` with `pgxpool` for the connection pool, opened in
  `internal/db` from the config DSN. Phase 0 uses the pool only for a connectivity check;
  no queries.
- **Rationale**: pgx is the de-facto Go PostgreSQL driver, supports the features later
  phases need (RLS `SET LOCAL`, LISTEN/NOTIFY, JSONB) without a swap, and pools cleanly.
- **Alternatives considered**: `database/sql` + `lib/pq` — `lib/pq` is in maintenance mode;
  pgx is the forward choice. An ORM — rejected; the project favors explicit SQL.

## 6. Migration tooling

- **Decision**: `golang-migrate/migrate/v4` used as a library, wrapped by a `cmd/migrate`
  binary exposing `up`, `down`, `version`, and `create`. Migration files live in
  `internal/db/migrations` as `NNNNNN_name.up.sql` / `.down.sql` pairs. A `Makefile`
  target and the CLI both drive it.
- **Rationale**: `golang-migrate` is locked in by the architecture. A first-party
  `cmd/migrate` wrapper means no separately installed CLI is required, keeps versions
  pinned via `go.mod`, and is reusable as the pre-deploy migration job. Satisfies FR-008,
  FR-010.
- **Alternatives considered**: `goose`, `atlas`, `dbmate` — all rejected; `golang-migrate`
  is the locked decision. Standalone `migrate` CLI binary — rejected; version drift and an
  extra install step.

## 7. Baseline migration content

- **Decision**: `000001_baseline` enables the `pgcrypto` extension
  (`CREATE EXTENSION IF NOT EXISTS pgcrypto`); the down migration drops it. This is the
  one non-empty baseline.
- **Rationale**: A migration must apply and revert cleanly (FR-009, SC-004). `pgcrypto`
  provides `gen_random_uuid()`, and every table in the architecture uses UUID primary
  keys — so this is genuine database-level groundwork, not speculative feature work. It
  proves the up/down chain end-to-end without introducing any tenant or feature schema.
- **Alternatives considered**: A truly empty migration — rejected; reverting an empty
  migration proves little, and `pgcrypto` is needed by the very first Phase 1 table.
  Creating Phase 1 tables now — rejected; violates Principle III (YAGNI).

## 8. Frontend skeleton

- **Decision**: React 19 + TypeScript on Vite 6, scaffolded in `frontend/`. A placeholder
  `App` component renders a static "nvelope" landing view. `vitest` + React Testing
  Library run one smoke test. `npm run build` produces the static bundle.
- **Rationale**: Vite + React + TS is the locked frontend stack. A placeholder app with
  one passing test satisfies FR-011 / SC-007 and gives CI something to build and test.
- **Alternatives considered**: Next.js — rejected; the architecture specifies a Vite SPA,
  not a meta-framework. Deferring the frontend entirely — rejected; FR-011 requires it.

## 9. CI pipeline

- **Decision**: GitHub Actions, one workflow `.github/workflows/ci.yml` triggered on push
  and pull request. Jobs: (a) backend — `go build ./...`, `go test ./...` with a
  PostgreSQL service container, `golangci-lint run`; (b) frontend — `npm ci`, `npm run
  lint`, `npm run test`, `npm run build`; (c) a migration job that applies the baseline
  against the service-container database. Overall status is the aggregate.
- **Rationale**: The repository is a git repo expected to be hosted on GitHub; Actions is
  the native pipeline service, so no provider needs choosing (per spec Assumptions). A
  PostgreSQL service container lets the migration and DB-touching tests run for real,
  honoring Principle II (real boundaries, not mocks). Satisfies FR-012–FR-014.
- **Alternatives considered**: GitLab CI / CircleCI — rejected; not the repo's native
  host. Mocking PostgreSQL in CI — rejected; Principle II requires real boundaries.

## 10. Containers and deployment manifests

- **Decision**: Three multi-stage Dockerfiles in `deploy/docker/` (Go builder stage →
  minimal `distroless/static` or `alpine` runtime, non-root user). Base Kubernetes
  `Deployment` manifests in `deploy/k8s/` for the three services, plus a skeleton Helm
  chart in `deploy/helm/nvelope/` (`Chart.yaml`, `values.yaml`, templated deployments).
  A repo-root `docker-compose.yml` runs PostgreSQL locally.
- **Rationale**: Per-service images keep deploys independent (FR-015). Base manifests +
  skeleton chart give a valid, schedulable starting point (FR-016, SC-008) without
  production hardening, which the spec defers to Phase 7. Non-root runtime images respect
  Principle IV.
- **Alternatives considered**: A single shared image with a command switch — rejected;
  couples deploy/scaling of the three services. Production-grade Helm (HPA, secrets,
  network policy) — rejected now; out of scope per spec Assumptions.

## 11. Linting

- **Decision**: `golangci-lint` configured by `.golangci.yml` for Go; ESLint + Prettier
  for the frontend. Both run in CI and are wired to `Makefile` / `npm` scripts.
- **Rationale**: Standard, widely adopted linters; running them in CI satisfies FR-012.
- **Alternatives considered**: `go vet` only — rejected; insufficient coverage. Biome for
  the frontend — viable but ESLint + Prettier is the more conventional default.
