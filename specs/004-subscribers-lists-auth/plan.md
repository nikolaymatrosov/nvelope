# Implementation Plan: Phase 2 — Subscribers, Lists & Auth

**Branch**: `004-subscribers-lists-auth` | **Date**: 2026-05-17 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/004-subscribers-lists-auth/spec.md`

## Summary

Phase 2 makes a tenant workspace usable: operators can manage **lists** and
**subscribers**, **import/export** them in bulk, segment them with queries, and
all of it is gated by **tenant RBAC** with scoped API keys and TOTP two-factor
auth. The work adds two new bounded contexts to the existing layered backend —
`internal/audience` (lists, subscribers, memberships, segments, import/export
jobs) and `internal/iam` (tenant-plane users, sessions, roles, permissions, API
keys, TOTP, audit log) — each with the calibrated `domain`/`app`/`adapters`
split established by Phase 1's refactor (003). Every new tenant-plane table
carries `tenant_id` and Row-Level Security, reusing the `tenant_settings`
pattern from migration `000004`; the cross-tenant isolation suite is extended to
cover all of them.

Bulk import/export is the platform's first genuine asynchronous workload, so
this phase introduces the durable, retry-capable job queue the constitution
mandates (River on PostgreSQL) and wires `cmd/worker` to consume it. The
RLS-bound transaction helper (`withTenant`, currently private in
`internal/tenant/adapters`) is promoted to a shared `internal/platform` package
so both new contexts use one implementation. Authorization is enforced at the
transport edge: a principal (session or scoped API key) is resolved by
middleware, and command/query handlers check effective permissions —
tenant-level unioned with per-list — before acting.

The phase is delivered as five user-story-aligned, independently shippable
increments and exits with a green test suite (including RBAC allow/deny and
extended isolation tests) and a clean migration apply.

## Technical Context

**Language/Version**: Go 1.26.

**Primary Dependencies**: All existing — `chi` (routing), `jackc/pgx/v5`
(PostgreSQL pool/driver), `golang-migrate/migrate/v4` (migrations),
`knadh/koanf/v2` (config), `log/slog` (logging), `stretchr/testify` (tests),
`golang.org/x/crypto/bcrypt` (password hashing). **New runtime dependencies**:
`riverqueue/river` + `riverqueue/river/riverpgxv5` (durable PostgreSQL-backed
job queue for import/export); `pquerna/otp` (TOTP code generation/validation
and provisioning URIs). CSV and ZIP handling use the standard library
(`encoding/csv`, `archive/zip`). No object-storage dependency is introduced
(see research.md). Go generics continue to back the command/query handler and
decorator types.

**Storage**: PostgreSQL 17. Phase 2 adds tenant-plane tables — `lists`,
`subscribers`, `subscriber_lists`, `users`, `sessions`, `roles`, `user_roles`,
`user_list_roles`, `api_keys`, `recovery_codes`, `audit_log`,
`import_export_jobs` — each with `tenant_id` + RLS following the `000004`
pattern. The existing `tenant_settings` table satisfies the spec's `settings`
requirement and is reused unchanged. River installs and owns its own queue
tables via its migrator.

**Testing**: Go stdlib `testing` + `testify/require`. Domain logic gets pure
unit tests with no infrastructure; repository adapters get integration tests
against a real PostgreSQL instance via `internal/dbtest`; import/export jobs are
tested against a real River queue on a real database (Critical Path coverage per
constitution II); the wired service keeps endpoint/component tests; the
cross-tenant isolation suite in `test/isolation_test.go` is extended to every
new tenant-plane table. RBAC is covered by explicit allow-path and deny-path
tests.

**Target Platform**: Linux containers on Kubernetes; builds and runs on
macOS/Linux for dev.

**Project Type**: Web application — multi-service Go backend monorepo + React
frontend. The frontend is out of scope for this plan.

**Performance Goals**: An import of 50,000 subscribers (CSV or ZIP-wrapped)
completes without blocking the operator's session and exposes observable
progress (SC-004, SC-005). Segment queries over a tenant's subscriber set return
a result and an accurate count in interactive time. No hard latency target is
set for interactive CRUD beyond standard web expectations.

**Constraints**: Tenant isolation is enforced at the data layer — every
tenant-plane read/write runs inside a tenant-bound transaction and fails closed
when `app.tenant_id` is unset. API keys, session tokens, and TOTP recovery codes
are never stored in readable form (hashed at rest); TOTP shared secrets are
stored encrypted at rest. The job queue must apply per-tenant fairness so one
tenant's large import cannot starve another's. Services stay stateless — all job
state lives in PostgreSQL. The build and full test suite stay green after every
increment.

**Scale/Scope**: 2 new bounded contexts; ~10 new domain aggregates/value-objects
(List, Subscriber, Membership, Segment, ImportJob, ExportJob, TenantUser,
Session, Role, APIKey, plus Permission and TOTP value objects); 12 new
tenant-plane tables; ~3 new migrations plus River's migrator; ~30 new
command/query handlers; new HTTP handlers under the existing tenant-scoped
routes. Delivered in 5 increments.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Assessment | Status |
|---|---|---|
| I. Tenant Isolation by Default | Every new table is tenant-plane and carries `tenant_id` from its first schema version with an `ENABLE`/`FORCE` RLS policy identical in shape to `tenant_settings` (`000004`). Tenant-plane access goes only through the shared `WithTenant` RLS-bound transaction helper, which fails closed when the GUC is unset. `test/isolation_test.go` is extended to prove a tenant cannot read or write another tenant's lists, subscribers, memberships, users, sessions, roles, API keys, or audit records even with the application filter omitted (FR-002, FR-003, SC-002). Control-plane (`platform_*`) and tenant-plane tables stay separate. | PASS |
| II. Test-Backed Delivery (NON-NEGOTIABLE) | Domain rules get pure unit tests; repositories get real-DB integration tests; import/export — a Critical Path (asynchronous job processing) — is tested against a real River queue on a real database, not mocked; RBAC gets allow-path and deny-path coverage (SC-003); the isolation suite is extended (SC-002). Each of the 5 increments exits with a green full suite and a clean migration apply (SC-011). | PASS |
| III. Incremental, Shippable Phases | Five user-story-aligned increments (US1→US5), each independently deployable and demonstrable. River and the two contexts are introduced because this phase's scope requires them — not speculatively. Deferred explicitly: campaigns, sending, double-opt-in confirmation, subscriber self-service, object storage, tenant-wide 2FA enforcement (YAGNI — later phases / out of scope). | PASS |
| IV. Security & Consent by Design | This phase *is* the platform's access-control layer. RBAC gates every action; API keys are scoped to a least-privilege permission subset and revocable; TOTP 2FA protects sign-in; privileged actions (role and key management) are written to an attributable audit log (FR-028, SC-010). API key tokens, session tokens, and recovery codes are stored only as hashes; TOTP secrets are encrypted at rest. Authorization is structural — resolved at the transport edge and enforced in handlers — not retrofitted. | PASS |
| V. Operable & Observable Services | Bulk import/export runs on a durable, retry-capable queue (River) with per-tenant fairness so no tenant starves another; jobs are resumable across worker restarts because all state is in PostgreSQL. Services stay stateless. Every new command/query handler is wrapped with the existing logging decorators for uniform observability. | PASS |
| VI. Layered Architecture & Domain Integrity | Two new bounded contexts each use the calibrated `domain`/`app`/`adapters` split. Domain entities (List, Subscriber, Role, …) are built only through validating constructors with a separate labelled hydration path; repository and external-service interfaces are declared by the consumer (domain/app) and implemented by adapters; the typed `apperr` error crosses boundaries and is mapped to HTTP status in exactly one place (`internal/api/errmap.go`); commands and queries stay distinct handler types under `command/` and `query/`; everything is wired through plain constructors in the `service.NewApplication` composition root. The full per-aggregate `ports/` split is still not adopted — nvelope remains one HTTP service. | PASS |

**Result**: PASS — no violations; Complexity Tracking not required. Two design
choices were weighed against YAGNI and resolved deliberately, both recorded in
research.md: (1) introducing the River job queue now rather than later — it is
*required* the moment bulk import/export enters scope, and a constitution
mandate (Principle V) makes a bespoke ad-hoc queue the wrong call; (2) **not**
introducing object storage — uploaded import files and generated export files
are staged in PostgreSQL rows, deferring an object-storage abstraction until a
later phase actually needs large media. Re-checked after Phase 1 design: still
PASS — the design adds only the two runtime dependencies named above and the
shared `platform/tenantdb` + `platform/jobs` packages (constitution: "shared
infrastructure lives once").

## Project Structure

### Documentation (this feature)

```text
specs/004-subscribers-lists-auth/
├── plan.md              # This file
├── spec.md              # Feature specification
├── research.md          # Phase 0 output — technical decisions & rationale
├── data-model.md        # Phase 1 output — entities, invariants, state transitions, schema
├── quickstart.md        # Phase 1 output — build, migrate, verify, test-by-layer
├── contracts/           # Phase 1 output
│   ├── http-api.md       # new HTTP endpoints — request/response shapes & status codes
│   ├── permissions.md    # the permission-string catalogue and RBAC evaluation rules
│   └── repositories.md   # domain-owned repository & service interfaces
├── checklists/
│   └── requirements.md
└── tasks.md             # Phase 2 output (/speckit-tasks — not created here)
```

### Source Code (repository root)

```text
nvelope/
├── cmd/
│   ├── api/main.go               # unchanged — calls the composition root
│   ├── worker/main.go            # CHANGED — registers River workers, consumes the queue
│   └── migrate/main.go           # CHANGED — also runs River's migrator
├── internal/
│   ├── platform/                 # shared cross-cutting building blocks
│   │   ├── apperr/               # CHANGED — add Forbidden (403) category for RBAC denials
│   │   ├── decorator/            # unchanged — command/query logging decorators
│   │   ├── tenantdb/             # NEW — exported WithTenant RLS-bound tx helper
│   │   │   ├── tenantdb.go        #   (moved & exported from tenant/adapters/rls.go)
│   │   │   └── tenantdb_test.go
│   │   └── jobs/                 # NEW — River client setup, enqueue + worker-registration helpers
│   │       ├── jobs.go
│   │       └── jobs_test.go
│   ├── iam/                      # NEW bounded context — tenant-plane identity & access
│   │   ├── domain/               # pure — no transport/persistence imports
│   │   │   ├── user.go            # TenantUser entity: validating constructor + hydration path
│   │   │   ├── session.go         # tenant-plane Session: issue/expiry/revoke/2FA-pending states
│   │   │   ├── role.go            # Role entity: named permission set
│   │   │   ├── permission.go      # Permission value object + catalogue + effective-permission rule
│   │   │   ├── apikey.go          # APIKey entity: scoped permission subset, revocation
│   │   │   ├── totp.go            # TOTP secret value object + recovery codes
│   │   │   ├── audit.go           # AuditRecord entity
│   │   │   ├── principal.go       # Principal value object — the resolved actor + effective perms
│   │   │   ├── repository.go      # consumer-owned repository interfaces
│   │   │   ├── errors.go          # typed domain errors
│   │   │   └── *_test.go          # unit tests, no infra
│   │   ├── app/
│   │   │   ├── application.go     # Application: Commands + Queries structs
│   │   │   ├── command/           # CreateRole, UpdateRole, DeleteRole, AssignRole,
│   │   │   │                      #   AssignListRole, RevokeRole, IssueAPIKey, RevokeAPIKey,
│   │   │   │                      #   EnableTOTP, ConfirmTOTP, DisableTOTP,
│   │   │   │                      #   OpenWorkspaceSession, VerifyTOTPChallenge, CloseSession
│   │   │   └── query/             # AuthenticatePrincipal (session|api-key → Principal),
│   │   │                          #   Authorize (effective-permission check), ListRoles,
│   │   │                          #   ListAPIKeys, AuditTrail
│   │   └── adapters/              # implement domain interfaces
│   │       ├── users_pg.go
│   │       ├── sessions_pg.go
│   │       ├── roles_pg.go
│   │       ├── apikeys_pg.go
│   │       ├── audit_pg.go
│   │       ├── totp.go            # pquerna/otp-backed TOTP adapter + secret encryption
│   │       └── *_test.go          # integration tests, real DB
│   ├── audience/                 # NEW bounded context — lists & subscribers
│   │   ├── domain/
│   │   │   ├── list.go            # List entity + visibility/opt-in value objects
│   │   │   ├── subscriber.go      # Subscriber entity: email, state, custom attributes
│   │   │   ├── membership.go      # SubscriberList membership + subscription-state transitions
│   │   │   ├── attributes.go      # custom-attribute value object (free-form structured data)
│   │   │   ├── segment.go         # Segment query value object + validation
│   │   │   ├── importjob.go       # ImportJob entity: pending→running→completed/failed + counts
│   │   │   ├── exportjob.go       # ExportJob entity
│   │   │   ├── repository.go      # consumer-owned repository interfaces
│   │   │   ├── errors.go
│   │   │   └── *_test.go
│   │   ├── app/
│   │   │   ├── application.go
│   │   │   ├── command/           # CreateList, UpdateList, DeleteList,
│   │   │   │                      #   CreateSubscriber, UpdateSubscriber, DeleteSubscriber,
│   │   │   │                      #   AddToList, RemoveFromList, ChangeSubscriptionState,
│   │   │   │                      #   StartImport, StartExport
│   │   │   └── query/             # ListLists, GetList, SearchSubscribers, GetSubscriber,
│   │   │                          #   RunSegment (matching set + count), GetJobStatus
│   │   └── adapters/
│   │       ├── lists_pg.go
│   │       ├── subscribers_pg.go        # owns segment-query → SQL translation
│   │       ├── memberships_pg.go
│   │       ├── jobs_pg.go               # import/export job records + staged file bytes
│   │       ├── csv_codec.go             # CSV/ZIP decode & encode
│   │       ├── import_worker.go         # River worker: streams staged file, upserts
│   │       ├── export_worker.go         # River worker: builds CSV, stages result
│   │       └── *_test.go
│   ├── api/                      # transport layer — the one HTTP surface
│   │   ├── server.go             # CHANGED — mounts iam + audience routes
│   │   ├── iam_handlers.go       # NEW — role, API key, 2FA, workspace-session endpoints
│   │   ├── audience_handlers.go  # NEW — list, subscriber, segment, import/export endpoints
│   │   ├── authz_middleware.go   # NEW — resolve Principal (session|API key), attach to ctx
│   │   ├── middleware.go         # CHANGED — tenant-resolution still applies
│   │   ├── errmap.go             # CHANGED — map apperr.Forbidden → 403
│   │   ├── respond.go            # unchanged
│   │   └── *_test.go             # endpoint/component tests
│   ├── service/
│   │   ├── application.go        # CHANGED — wire iam + audience handlers, River client
│   │   └── *_test.go
│   ├── tenant/                   # CHANGED — Phase 1 membership flows also provision a
│   │   │                          #   tenant-plane iam user (see research.md, decision 4)
│   │   └── adapters/rls.go       # REMOVED — logic moves to platform/tenantdb
│   ├── config/  db/  health/  logging/  token/   # mostly unchanged shared infrastructure
│   └── dbtest/                   # unchanged real-DB test helper
└── test/
    ├── isolation_test.go         # CHANGED — extended to every new tenant-plane table
    └── migrate_test.go           # unchanged
```

**Structure Decision**: Keep the monorepo and the Phase 1 calibrated layout. Two
new bounded contexts (`internal/iam`, `internal/audience`) each get the moderate
`domain`/`app`/`adapters` three-layer split — not the full per-aggregate `ports/`
explosion, because nvelope still exposes a single HTTP service (constitution:
"layer scope is proportional to need"). The single transport layer
`internal/api` gains handler files and an authorization middleware for both new
contexts. The RLS-bound transaction helper is promoted from
`internal/tenant/adapters/rls.go` to a new shared `internal/platform/tenantdb`
package so all three tenant-plane contexts (`tenant`, `iam`, `audience`) share
one implementation (constitution: "shared infrastructure lives once"). River
client construction and worker registration live once in `internal/platform/jobs`.
The composition root stays `service.NewApplication`.

## Implementation Increments

The phase is a sequence of independently shippable increments, each aligned to a
user story, each leaving the build green and the service runnable (Principle III).

1. **Shared foundations & access schema (enables US1–US5)** — Promote
   `withTenant` to `platform/tenantdb`; add `platform/jobs` (River client) and
   the River migrator wiring in `cmd/migrate`; add the `apperr.Forbidden`
   category. Add migration `000005` for the iam tenant-plane tables (`users`,
   `sessions`, `roles`, `user_roles`, `user_list_roles`, `api_keys`,
   `recovery_codes`, `audit_log`). Extend the isolation suite to these tables.
   No behavior cut over yet.
2. **Manage lists & subscribers (US1, P1)** — Migration `000006` for `lists`,
   `subscribers`, `subscriber_lists`. The `audience` domain, repositories, CRUD
   command/query handlers, HTTP handlers, and custom attributes. Isolation suite
   extended. Delivers the first demonstrable slice: a tenant operator manages an
   audience.
3. **RBAC access gates (US2, P1)** — The `iam` domain (Role, Permission,
   Principal, TenantUser, Session), repositories, role-management handlers, the
   `authz_middleware` principal resolver, and per-handler permission enforcement
   (tenant-level ∪ per-list). Phase 1 membership flows extended to provision a
   tenant-plane user. Allow/deny tests.
4. **Import & export + segmentation (US3 + US4, P2)** — Migration `000007` for
   `import_export_jobs` and staged-file storage. The segment query value object
   and its SQL translation; the `RunSegment` query. CSV/ZIP codec; `StartImport`
   / `StartExport` commands enqueueing River jobs; the import/export River
   workers in `cmd/worker`; `GetJobStatus`. Job tests against a real queue.
5. **Scoped API keys & TOTP 2FA (US5, P3)** — API key issuance/revocation with
   scoped permissions; API-key authentication path in `authz_middleware`; TOTP
   enable/confirm/disable, the 2FA challenge on opening a workspace session, and
   recovery codes. Audit records for key and role management verified end-to-end.

## Complexity Tracking

No constitution violations — section intentionally empty. The two YAGNI-sensitive
choices (introducing River; not introducing object storage) are justified inline
in the Constitution Check and detailed in research.md; neither is a deviation
from a principle.
