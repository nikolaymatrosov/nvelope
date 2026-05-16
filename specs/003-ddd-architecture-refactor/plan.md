# Implementation Plan: DDD / Clean Architecture Refactor of the Backend

**Branch**: `003-ddd-architecture-refactor` | **Date**: 2026-05-16 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/003-ddd-architecture-refactor/spec.md`

## Summary

Reorganize the backend (`internal/auth`, `internal/tenant`, `internal/api`) from
flat package-level functions that mix business rules, raw SQL, and transaction
orchestration into clearly separated layers, with no change to externally
observable API behavior. The refactor applies every non-negotiable rule of
constitution Principle VI — a pure domain with validating constructors,
domain-owned repository interfaces, typed errors mapped to transport in exactly
one place, separated command/query handlers, and an explicit composition root —
while **calibrating the physical layout** to the project's size: each bounded
context (`auth`, `tenant`) gets a moderate `domain` / `app` / `adapters` split
rather than the full per-aggregate `ports/app/domain/adapters` directory
explosion. This matches the constitution's "layer scope is proportional to need"
constraint and PATTERNS.md's own guidance that the full split pays off only with
multiple services or teams. The work proceeds as a strangler refactor: new
layered code is built alongside the old code, callers are cut over package by
package, and the build plus the full test suite (including the cross-tenant
isolation suite against a real database) stay green at every increment.

## Technical Context

**Language/Version**: Go 1.26.

**Primary Dependencies**: All existing — `chi` (routing), `jackc/pgx/v5`
(PostgreSQL pool/driver), `golang-migrate/migrate/v4` (migrations),
`knadh/koanf/v2` (config), `log/slog` (logging), `stretchr/testify` (tests),
`golang.org/x/crypto/bcrypt` (password hashing). No new runtime dependencies.
New dev/CI tool only: `go-cleanarch` to enforce the inward dependency rule in CI.
Go generics are used for the command/query handler and decorator types.

**Storage**: PostgreSQL 17 — schema, migrations, the `nvelope_app` role, and the
Row-Level Security model are all unchanged. This is a Go code-structure refactor.

**Testing**: Go stdlib `testing` + `testify/require`. Domain logic gets pure unit
tests with no infrastructure; repository adapters get integration tests against a
real PostgreSQL instance via the existing `internal/dbtest` helper; the wired
service keeps its endpoint/component tests; the cross-tenant isolation suite in
`test/isolation_test.go` continues to run as `nvelope_app`.

**Target Platform**: Linux containers on Kubernetes; builds and runs on
macOS/Linux for dev.

**Project Type**: Web application — multi-service Go backend monorepo + React
frontend. The frontend is out of scope.

**Performance Goals**: None. This is a behavior-preserving refactor; no latency or
throughput target changes.

**Constraints**: Zero change to observable API behavior — identical routes,
request/response shapes, status codes, and error envelopes. The RLS isolation
model is preserved: tenant-plane access still runs inside a transaction with
`app.tenant_id` bound and fails closed when unset. Every existing test passes
unchanged. The build and the full test suite stay green after every increment;
the service is runnable at each step. No new feature, endpoint, or schema change.

**Scale/Scope**: ~2,550 lines of backend Go across 2 bounded contexts (`auth`,
`tenant`); ~6 domain entities (User, Session, Tenant, Membership, Invitation,
TenantSettings); ~12 HTTP endpoints; 1 cross-tenant isolation suite. Refactor
delivered in 6 increments.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Assessment | Status |
|---|---|---|
| I. Tenant Isolation by Default | Preserved and reinforced. The RLS schema and `nvelope_app` role are untouched. The `WithTenant` transaction binding moves *inside* the tenant-plane repository adapter, so every tenant-plane read/write still executes under a bound `app.tenant_id` and fails closed when unset. The cross-tenant isolation suite is kept and still runs as `nvelope_app`; FR-009/FR-010 forbid any behavior change. | PASS |
| II. Test-Backed Delivery (NON-NEGOTIABLE) | Every existing test must pass unchanged (FR-012, SC-001). Tests are reorganized by layer: domain unit tests (no infra), repository integration tests against a real database (no mocked DB), and endpoint/component tests for the wired service. Each of the 6 increments exits with a green full suite (FR-016, SC-005). | PASS |
| III. Incremental, Shippable Phases | Strangler refactor in 6 independently shippable increments, each leaving the build green and the service runnable. No new features (YAGNI) — pure structural change. The calibrated layout deliberately avoids speculative ceremony. | PASS |
| IV. Security & Consent by Design | Preserved. Passwords stay bcrypt-hashed; session and invite tokens stay stored only as SHA-256 hashes; account-enumeration resistance on login and invite lookup is retained (FR-015). The refactor moves these rules into the domain layer but does not weaken them. | PASS |
| V. Operable & Observable Services | Services stay stateless — no session/work state moves into process memory. Structured `slog` logging continues and becomes *more* uniform: command/query handlers are wrapped with generic decorators so every use case gets logging consistently rather than via per-handler boilerplate (constitution "separate read and write paths" constraint). | PASS |
| VI. Layered Architecture & Domain Integrity | This refactor exists to satisfy Principle VI. All four non-negotiable rules are applied in full: domain logic becomes pure with validating constructors plus an explicitly labelled hydration path; repository and session-resolution interfaces are declared by the consumer (domain/app), implemented by adapters; typed errors carry a machine-readable kind and are mapped to HTTP status in exactly one place; dependencies are wired through plain constructors at a single composition root with no DI framework. Per the "layer scope is proportional to need" constraint, the *number* of physical layers is calibrated — a moderate `domain`/`app`/`adapters` split per context, not a speculative full split. | PASS |

**Result**: PASS — no violations; Complexity Tracking not required. The one design
choice weighed against YAGNI — how many physical layers to introduce — was
resolved *toward* the constitution: the full per-aggregate `ports/app/domain/
adapters` split prescribed by the go-ddd-architecture skill is **not** adopted,
because the constitution and PATTERNS.md both reserve it for multi-service /
multi-team contexts. The calibrated layout introduces only the structure the
demonstrated tangle requires. Re-checked after Phase 1 design: still PASS — the
design adds only the `go-cleanarch` CI linter and no runtime dependencies.

## Project Structure

### Documentation (this feature)

```text
specs/003-ddd-architecture-refactor/
├── plan.md              # This file
├── spec.md              # Feature specification
├── research.md          # Phase 0 output — refactor strategy & technical decisions
├── data-model.md        # Phase 1 output — domain entities, invariants, transitions
├── quickstart.md        # Phase 1 output — build, verify, layer-by-layer test
├── contracts/           # Phase 1 output
│   ├── http-api.md       # the unchanged HTTP surface — the regression contract
│   ├── layering.md       # the inward dependency rule and how it is enforced
│   └── repositories.md   # domain-owned repository & service interfaces
├── checklists/
│   └── requirements.md
└── tasks.md             # Phase 2 output (/speckit-tasks — not created here)
```

### Source Code (repository root)

```text
nvelope/
├── cmd/
│   └── api/main.go               # thinned — calls the composition root, starts server
├── internal/
│   ├── platform/                 # NEW — shared cross-cutting building blocks
│   │   ├── apperr/               # typed error: slug + category; errors crossing a domain boundary
│   │   │   ├── apperr.go
│   │   │   └── apperr_test.go
│   │   └── decorator/            # generic command/query handler decorators (logging)
│   │       ├── decorator.go
│   │       └── decorator_test.go
│   ├── auth/                     # bounded context: platform identity
│   │   ├── domain/               # pure — no transport/persistence imports
│   │   │   ├── user.go           # User entity: validating constructor + hydration path
│   │   │   ├── session.go        # Session entity: issue/expiry/revoke invariants
│   │   │   ├── credentials.go    # email + password-policy value objects
│   │   │   ├── repository.go     # UserRepository, SessionRepository interfaces
│   │   │   ├── errors.go         # typed domain errors
│   │   │   └── *_test.go         # unit tests, no infra
│   │   ├── app/                  # use cases
│   │   │   ├── application.go    # Auth application: Commands + Queries structs
│   │   │   ├── command/          # SignUp, LogIn, LogOut, RegisterInvitedUser
│   │   │   └── query/            # AuthenticateSession (cookie → user), lookups
│   │   └── adapters/             # implement domain interfaces
│   │       ├── users_pg.go
│   │       ├── sessions_pg.go
│   │       ├── password_bcrypt.go
│   │       └── *_test.go         # integration tests, real DB
│   ├── tenant/                   # bounded context: tenancy & RLS
│   │   ├── domain/
│   │   │   ├── tenant.go         # Tenant entity + slug rules (value object)
│   │   │   ├── membership.go
│   │   │   ├── invitation.go     # Invitation entity: pending→accepted/revoked/expired
│   │   │   ├── settings.go       # TenantSettings entity
│   │   │   ├── repository.go     # TenantRepository, InvitationRepository, SettingsRepository
│   │   │   ├── errors.go
│   │   │   └── *_test.go
│   │   ├── app/
│   │   │   ├── application.go
│   │   │   ├── command/          # CreateWorkspace, InviteTeammate, AcceptInvitation,
│   │   │   │                     #   RevokeInvitation, UpdateSettings
│   │   │   └── query/            # ListWorkspaces, ResolveWorkspace, WorkspaceMembers,
│   │   │                         #   GetSettings, PendingInvitations, LookUpInvitation
│   │   └── adapters/
│   │       ├── tenants_pg.go
│   │       ├── invitations_pg.go
│   │       ├── settings_pg.go    # owns the WithTenant RLS-bound transaction
│   │       ├── rls.go            # WithTenant helper (moved from tenant/rls.go)
│   │       └── *_test.go
│   ├── api/                      # transport layer (ports) — the one HTTP surface
│   │   ├── server.go             # router; holds the wired Application, not the pool
│   │   ├── platform_handlers.go  # build command/query → call handler → respond
│   │   ├── tenant_handlers.go
│   │   ├── middleware.go         # session-cookie + tenant-resolution middleware
│   │   ├── errmap.go             # the single apperr → HTTP status mapping
│   │   ├── respond.go            # JSON write + error envelope helpers
│   │   └── *_test.go             # endpoint/component tests
│   ├── service/                  # process lifecycle + NEW composition root
│   │   ├── service.go            # existing Run / RunnerFunc / Version
│   │   ├── application.go        # NewApplication(ctx, deps) — wires every layer
│   │   └── *_test.go
│   ├── config/  db/  health/  logging/  token/   # unchanged shared infrastructure
│   └── dbtest/                   # unchanged real-DB test helper
└── test/
    ├── isolation_test.go         # unchanged — cross-tenant isolation, real DB, nvelope_app
    └── migrate_test.go           # unchanged
```

**Structure Decision**: Keep the monorepo. The backend is reorganized into two
bounded contexts (`auth`, `tenant`), each with a calibrated three-layer split —
`domain` (pure rules, validating constructors, consumer-owned interfaces, typed
errors), `app` (command and query handlers under `command/` and `query/`
sub-packages so read and write paths stay distinct), and `adapters` (pgx
repository implementations, the bcrypt password adapter, and the RLS-bound
transaction). A single transport layer (`internal/api`) serves both contexts —
nvelope exposes one HTTP service, so a per-context `ports/` directory would be
ceremony without payoff. Cross-cutting building blocks (`platform/apperr`,
`platform/decorator`) live once. The composition root is `service.NewApplication`,
which constructs adapters and handlers with plain constructors and returns the
wired `Application`; `cmd/api/main.go` shrinks to calling it. The full
per-aggregate `ports/app/domain/adapters` split from the go-ddd-architecture
skill is deliberately not adopted (constitution: "layer scope is proportional to
need"; PATTERNS.md: the full split pays off only with multiple services/teams).

## Refactor Increments

The work is a strangler refactor — old code stays callable until each cutover —
so every increment ends green and shippable (FR-016, SC-005).

1. **Shared foundations** — `platform/apperr` (typed error: slug + category) and
   `platform/decorator` (generic command/query handler wrappers). New packages
   only; nothing cut over yet.
2. **Domain extraction (US1, P1)** — Extract pure domain entities for both
   contexts with validating constructors, hydration paths, behavior methods,
   typed errors, and repository interfaces. Cover every rule with unit tests.
   Old flat functions still serve traffic.
3. **Adapters (US2, P2)** — Implement the repository interfaces with pgx; move
   `WithTenant` into the tenant settings adapter; add the bcrypt password
   adapter. Integration tests against a real database.
4. **Application layer (US3a)** — Command and query handlers in business
   language, decorated for uniform logging; `service.NewApplication` composition
   root. Handler unit tests with in-memory repository fakes.
5. **Ports cutover (US3b)** — HTTP handlers and middleware call command/query
   handlers; `errmap.go` becomes the single apperr→status mapping. Endpoint
   tests confirm byte-identical responses.
6. **Cleanup & enforcement** — Delete the superseded flat functions; add
   `go-cleanarch` to CI to enforce the inward dependency rule.

## Complexity Tracking

No constitution violations — section intentionally empty.
