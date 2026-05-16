# Phase 0 Research: DDD / Clean Architecture Refactor

This document records the technical decisions behind the refactor strategy.
There were no open `NEEDS CLARIFICATION` items from the spec; the single
load-bearing decision (how literally to follow the go-ddd-architecture skill's
layout) was resolved with the user — calibrated layering — and is recorded below.

## 1. Layer calibration: how many physical layers

**Decision**: Each bounded context (`auth`, `tenant`) gets a three-layer split —
`domain`, `app`, `adapters` — and the two contexts share a single transport
layer (`internal/api`). The full per-aggregate `ports/app/domain/adapters`
directory split from the go-ddd-architecture skill is **not** adopted.

**Rationale**: Constitution Principle VI's *dependency rule and domain-integrity
rules are non-negotiable and always enforced*, but the Architectural Constraints
state "layer scope is proportional to need" — the full split is reserved for
multi-service / multi-team contexts. PATTERNS.md (the project's own distillation
of the reference app) says the same: the 5-layer split and codegen "pay off
mainly once you have multiple services or teams", while domain-owned repository
interfaces, CQRS decorators, and slug-error mapping "transfer best to a smaller
project". nvelope is one HTTP service with a small team, so a single shared
transport layer and a moderate per-context split capture all the benefit without
speculative ceremony.

**Alternatives considered**: (a) Full skill layout — rejected as speculative
ceremony the constitution explicitly defers. (b) Collapse everything into one
package per context (store + service + handler files together) — rejected
because the spec documents real, current pain from exactly that tangle; the
domain must become independently importable and unit-testable.

## 2. Refactor sequencing: strangler, domain-first

**Decision**: Refactor as a strangler — build the layered code beside the old
flat functions, cut callers over package by package, delete the old code last.
Order: shared foundations → domain → adapters → app → ports → cleanup.

**Rationale**: The go-ddd-architecture skill's refactoring guidance is explicit:
"Start with the domain. Extract entities... Get them under unit tests. Then
extract the repository. Then introduce the application layer. Ports last." The
strangler approach is what lets every increment satisfy FR-016 / SC-005 — green
build and green suite at each step — because traffic never depends on a
half-migrated package.

**Alternatives considered**: Big-bang rewrite of all three packages at once —
rejected; it would leave the build red for the duration and violate the
incremental-delivery principle.

## 3. Domain model: validating constructors + hydration path

**Decision**: Domain entities have unexported fields and getter methods. Each
aggregate has exactly one validating constructor returning `(*T, error)` and a
separate, explicitly labelled hydration function for repository loads (e.g.
`HydrateUser(...)` documented "persistence only — not a constructor"). Behavior
lives on the entity (`Invitation.Accept()`, `Session.IsExpired()`), not in
services.

**Rationale**: Constitution Principle VI mandates exactly this — "entities MUST
be constructible only through validating constructors so an invalid state is
unrepresentable; loading persisted data uses a separate, explicitly labelled
hydration path." It matches PATTERNS.md #3. Today's `auth.User`, `tenant.Tenant`,
etc. are anaemic structs with public fields and JSON tags; validation lives in
free functions (`validateCredentials`, `ValidateSlug`). The refactor moves the
rules onto the types.

**Alternatives considered**: Keep public-field structs and validate in the app
layer — rejected; it leaves invalid states representable and violates Principle VI.

## 4. Repository interfaces owned by the consumer

**Decision**: Each domain package declares the repository interfaces it needs
(`UserRepository`, `SessionRepository`, `TenantRepository`,
`InvitationRepository`, `SettingsRepository`). Multi-step mutations that must be
transactional use an update-with-closure signature
(`Update(ctx, id, func(*T) (*T, error)) error`). The pgx implementations live in
each context's `adapters/`.

**Rationale**: Constitution Principle VI — "Repository interfaces MUST be
declared by the package that depends on them; infrastructure adapters implement
those interfaces." The closure pattern (PATTERNS.md #4) keeps load→mutate→save
atomic and is the natural home for the RLS-bound transaction (see §5). Today the
SQL is inlined into `auth`/`tenant` functions that take `db.Querier` or
`*pgxpool.Pool` directly — the domain depends on the driver.

**Alternatives considered**: A single generic repository interface — rejected;
each aggregate's access pattern differs and a generic CRUD interface would lose
domain meaning.

## 5. Preserving Row-Level Security under the repository abstraction

**Decision**: The `WithTenant` helper (currently `internal/tenant/rls.go`) moves
into the tenant `adapters/` package. Tenant-plane repository methods
(`SettingsRepository`) open a `WithTenant` transaction internally, so the
`app.tenant_id` GUC is bound for the duration of the data access. The domain and
app layers never see the transaction or the GUC — they call
`SettingsRepository.Get(ctx, tenantID)` and the adapter handles the binding.
`CreateWorkspace`, which inserts a tenant, its owner membership, and the initial
`tenant_settings` row in one transaction, keeps that whole unit inside a single
adapter method that binds the new tenant before the settings insert.

**Rationale**: Constitution Principle I makes the data layer the authoritative
isolation backstop; FR-009/FR-010 forbid any behavior change. Pushing the GUC
binding down into the adapter is what keeps the domain pure *and* keeps RLS
fail-closed. Today `WithTenant` leaks into the HTTP handlers
(`tenant_handlers.go` calls `tenant.WithTenant(...)` directly) — the refactor
removes that leak.

**Alternatives considered**: Expose a transaction object up to the app layer —
rejected; it would re-couple the use-case layer to pgx and the RLS mechanism.

## 6. Typed errors and one-place transport mapping

**Decision**: `internal/platform/apperr` defines an error carrying a
machine-readable `slug` (stable token, e.g. `slug_taken`) plus a `category`
(e.g. `IncorrectInput`, `Conflict`, `NotFound`, `Authorization`, `Unknown`).
Domain packages return these. `internal/api/errmap.go` is the *single* place that
maps category → HTTP status and writes the `{error, message}` envelope.

**Rationale**: Constitution Principle VI — "errors that cross a domain boundary
MUST carry a machine-readable kind... translation to a transport status code
MUST happen in exactly one place; domain code MUST NOT know about HTTP." This
generalizes today's `Server.fail`, which already centralizes mapping but does so
by `errors.Is`-matching a hand-maintained list of sentinel errors from two
packages. The category field removes the per-error switch arms. PATTERNS.md #5
is the model. The existing stable `error` slugs in API responses
(`email_taken`, `slug_taken`, `invitation_not_found`, `tenant_not_found`,
`validation_failed`, …) MUST be preserved exactly (FR-001).

**Alternatives considered**: Keep sentinel `errors.New` values and `errors.Is`
matching — rejected; it scales poorly and forces the transport layer to import
every domain's error set.

## 7. CQRS handlers and generic decorators

**Decision**: Each use case is its own handler type with a `NewXxxHandler`
constructor. Commands implement `Handle(ctx, cmd) error`; queries implement
`Handle(ctx, q) (R, error)`. `platform/decorator` provides generic wrappers
(`ApplyCommandDecorators`, `ApplyQueryDecorators`) that add structured `slog`
logging uniformly. Read paths return flat read-model structs shaped for the
response, never domain entities.

**Rationale**: Constitution Architectural Constraints — "commands that mutate
state and queries that read it SHOULD be modelled as distinct handler types...
cross-cutting handler behavior SHOULD be applied through generic
decorators/wrappers." PATTERNS.md #2 names this as one of the patterns that
"transfers best to a smaller project". Go 1.26 generics make the decorator
type-safe with no reflection.

**Alternatives considered**: One `AuthService` struct with all methods —
rejected; it conflates read and write paths and gives no uniform decoration
seam. Per-handler hand-written logging — rejected as drift-prone boilerplate the
constitution explicitly discourages.

## 8. Composition root

**Decision**: Add `service.NewApplication(ctx, deps)` — a single function that
constructs the pgx-backed adapters, builds every command/query handler (applying
decorators), and returns the wired `Application{ Auth, Tenant }` value plus a
cleanup func. `internal/api` holds that `Application` instead of a raw
`*pgxpool.Pool`. `cmd/api/main.go` shrinks to opening the pool, calling
`NewApplication`, and starting the server. Tests get a parallel constructor that
substitutes in-memory repository fakes through the same wiring path.

**Rationale**: Constitution Principle VI — "dependencies are wired through plain
constructors at a single composition root; no runtime DI framework and no hidden
global state. Tests substitute doubles through that same wiring path."
PATTERNS.md #6. Today `api.New` receives the pool and every handler reaches into
package-level functions with it — there is no composition seam.

**Alternatives considered**: A DI framework (wire, fx) — rejected; the
constitution forbids a runtime DI framework and the dependency graph is small
enough for plain constructors.

## 9. Enforcing the dependency rule

**Decision**: Add `go-cleanarch` to the CI pipeline to fail the build if an
inner layer imports an outer one. Domain packages are also kept honest by their
import lists alone (no `net/http`, no `pgx`).

**Rationale**: Constitution Principle VI's inward dependency rule is a stated
review gate; PATTERNS.md #1 notes the reference app enforces it with
`go-cleanarch` in CI. Automating it (SC-002) makes the rule a build failure
rather than a review judgement call.

**Alternatives considered**: Rely on code review only — rejected; SC-002 asks for
an *automated* layering check, and review alone lets violations slip in.

## 10. Test layering

**Decision**: Domain rules → pure unit tests, no DB/Docker. Repository adapters →
integration tests against a real PostgreSQL via `internal/dbtest`, connected as
`nvelope_app`. App handlers → unit tests with in-memory repository fakes.
Wired service → endpoint/component tests (existing `internal/api` tests).
Cross-tenant isolation → unchanged `test/isolation_test.go`. Every test stays
`t.Parallel()`-safe.

**Rationale**: Constitution Principle II forbids mocking the database away on
critical paths; the go-ddd-architecture skill's testing guidance maps each layer
to a test type. The existing tests already follow real-DB integration for the
data layer — the refactor relocates tests to sit beside their layer (FR-011)
without weakening them, and FR-012 requires all of them keep passing.

**Alternatives considered**: Mock the database in repository tests — rejected;
violates Principle II and the spec's explicit "no mocked database" requirement.
