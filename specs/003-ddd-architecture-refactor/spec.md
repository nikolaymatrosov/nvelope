# Feature Specification: DDD / Clean Architecture Refactor of the Backend

**Feature Branch**: `003-ddd-architecture-refactor`

**Created**: 2026-05-16

**Status**: Draft

**Input**: User description: "refactor code using /go-ddd-architecture skill"

## Overview

The backend (`internal/auth`, `internal/tenant`, `internal/api`) currently mixes
business rules, raw SQL, and transaction orchestration inside flat package-level
functions, and HTTP handlers call those functions directly. This refactor
reorganizes the backend into clearly separated layers — domain, application,
adapters, and ports — so business rules live in one place, persistence is
swappable, and each concern can be tested in isolation. It is a structural
change only: the externally observable behavior of the service does not change.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Business rules isolated and unit-testable (Priority: P1)

A maintainer needs to change or add a business rule (e.g. password policy, slug
rules, invitation acceptance conditions) and verify it without standing up a
database or HTTP server. After this story, business rules live in a domain layer
that has no knowledge of HTTP, SQL, or frameworks, and every rule is covered by
fast unit tests.

**Why this priority**: This is the core of the refactor and the largest source
of current maintenance pain — rules are tangled with infrastructure, so they
cannot be reasoned about or tested independently. Extracting the domain layer
delivers value on its own even before later stories land.

**Independent Test**: Can be fully tested by running the domain-layer unit
suite, which compiles and passes with no database, no Docker, and no network —
proving the rules are decoupled from infrastructure.

**Acceptance Scenarios**:

1. **Given** a domain entity, **When** it is constructed with invalid input,
   **Then** construction fails with a typed domain error and no invalid entity
   is produced.
2. **Given** the domain layer, **When** its dependencies are inspected, **Then**
   it imports nothing from the transport (HTTP) or persistence (SQL driver)
   layers.
3. **Given** an existing business rule (credential validation, slug validation,
   invitation expiry/status checks), **When** the domain unit suite runs,
   **Then** that rule is exercised by a test that needs no infrastructure.

---

### User Story 2 - Persistence behind repository interfaces (Priority: P2)

A maintainer needs to evolve or replace how data is stored without touching
business logic. After this story, persistence is expressed through repository
interfaces owned by the domain, with SQL implementations isolated in an adapters
layer, and repository behavior is verified by integration tests against a real
database.

**Why this priority**: Builds on the extracted domain. It removes SQL from
business code and makes the data layer independently verifiable, but it depends
on the domain types from Story 1 existing first.

**Independent Test**: Can be fully tested by running the repository integration
suite against a real PostgreSQL instance and confirming all persistence
operations behave correctly, while the domain unit suite still passes unchanged.

**Acceptance Scenarios**:

1. **Given** a repository interface declared by the domain, **When** a SQL
   implementation is provided in the adapters layer, **Then** the application
   layer depends only on the interface, not the implementation.
2. **Given** a tenant-plane data operation, **When** it executes through a
   repository, **Then** it still runs inside a transaction bound to the tenant
   (`app.tenant_id`) so Row-Level Security remains the authoritative backstop.
3. **Given** the repository integration suite, **When** it runs, **Then** it
   exercises a real PostgreSQL instance connected as the least-privileged
   application role — no mocked database.

---

### User Story 3 - Application layer split into commands and queries; HTTP confined to ports (Priority: P3)

A maintainer needs request handling to follow a predictable shape so that
locating "where does X happen" is unambiguous. After this story, the application
layer is split into command handlers (state changes) and query handlers (reads),
each named in business language, and HTTP concerns are confined to a ports
layer that translates requests into commands/queries and maps domain errors to
responses.

**Why this priority**: It completes the layering and the read/write separation,
giving the predictable end-to-end request flow. It depends on the domain and
repositories from Stories 1 and 2.

**Independent Test**: Can be fully tested by running the endpoint/component
suite and confirming every existing API route still returns the same status
codes and response shapes, with all request handling flowing through the
command/query layer.

**Acceptance Scenarios**:

1. **Given** an incoming HTTP request, **When** it is handled, **Then** the
   handler builds a command or query, invokes its handler, and maps the result
   or domain error to a response — performing no business decisions itself.
2. **Given** a state-changing operation, **When** it is invoked, **Then** it is
   a command handler that returns only an error; **Given** a read operation,
   **Then** it is a query handler that returns data shaped for the caller.
3. **Given** the command and query handlers, **When** they are named, **Then**
   they use business language (e.g. "sign up", "create workspace", "invite
   teammate", "accept invitation") rather than generic CRUD verbs where a domain
   meaning exists.

---

### Edge Cases

- **Behavior preservation**: Every existing automated test must continue to pass
  with no change to its assertions about API behavior. If a test must change, it
  is a signal the refactor altered observable behavior — which is out of scope.
- **RLS transaction boundary**: The repository abstraction must not break the
  Row-Level Security pattern — tenant-plane reads and writes must still execute
  inside a transaction with `app.tenant_id` bound, and must still fail closed
  when it is unset.
- **Cross-tenant isolation**: Cross-tenant denials must remain opaque (a `404`,
  never a `403`); the isolation suite must still prove tenant A cannot read or
  write tenant B's rows with application-level filters omitted.
- **Account-enumeration resistance**: Login and invitation-lookup flows must
  still return identical responses for "unknown" and "wrong" cases after the
  rules move into the domain layer.
- **Import cycles**: If extracting a layer produces a Go import cycle, an inner
  layer is depending on an outer layer — that must be resolved by declaring an
  interface in the inner layer, not by merging the layers back together.
- **Partial increments**: Each refactor increment must leave the build and the
  full test suite green; the service must be runnable after every increment.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The refactor MUST preserve all externally observable API behavior —
  identical routes, request shapes, status codes, response bodies, and error
  envelopes.
- **FR-002**: Business rules and invariants MUST reside in a domain layer that
  has no dependency on HTTP transport, SQL drivers, routing, or web frameworks.
- **FR-003**: Domain entities MUST own their state through private fields and
  expose behavior methods; constructing or transitioning an entity MUST validate
  its invariants so an invalid entity cannot exist in memory.
- **FR-004**: Persistence MUST be expressed through repository interfaces
  declared by the domain layer; concrete SQL implementations MUST live in a
  separate adapters layer.
- **FR-005**: The application layer MUST be split into command handlers (state
  changes, returning only an error) and query handlers (reads, returning data
  shaped for the caller).
- **FR-006**: HTTP transport concerns MUST be confined to a ports layer; port
  handlers translate requests into commands/queries and map domain errors to
  responses, and MUST contain no business decision logic.
- **FR-007**: Layer dependencies MUST flow inward only (ports and adapters depend
  on the application layer, which depends on the domain layer); no inner layer
  may import an outer layer.
- **FR-008**: Domain errors MUST remain typed values, and the mapping from
  domain errors to transport-specific responses MUST stay centralized in the
  ports layer.
- **FR-009**: Tenant-plane data access MUST continue to run inside a
  transaction bound to the tenant (`app.tenant_id`), so Row-Level Security
  remains the authoritative isolation backstop after the refactor.
- **FR-010**: Cross-tenant isolation guarantees MUST be unchanged — fail-closed
  RLS and opaque `404` denials.
- **FR-011**: The test suite MUST be organized by layer: unit tests for the
  domain (no infrastructure), integration tests for repositories (against a real
  PostgreSQL instance, no mocked database), and endpoint/component tests for the
  wired service.
- **FR-012**: Every existing test MUST continue to pass, including the
  cross-tenant isolation suite running as the least-privileged application role.
- **FR-013**: Command and query handlers MUST be named in business language
  reflecting the domain action, rather than generic CRUD verbs, wherever a
  distinct domain meaning exists.
- **FR-014**: Persistence and transport data shapes MUST be kept separate from
  domain types; explicit mapping MUST translate between database rows, domain
  entities, and response payloads.
- **FR-015**: Security properties MUST be preserved — passwords and tokens
  remain stored only as hashes, and account-enumeration resistance is retained.
- **FR-016**: The refactor MUST be delivered in independently shippable
  increments, each leaving the build and the full test suite green and the
  service runnable.

### Key Entities

- **User**: A platform identity (email, name). Owns credential validation and
  never carries the password hash outside persistence.
- **Session**: A login session tied to a user, with issue, resolve, and revoke
  behaviors and an expiry invariant.
- **Tenant (Workspace)**: An isolated workspace with a slug and status. Owns slug
  validation and derivation rules.
- **Membership**: The association of a user with a tenant in a given role.
- **Invitation**: A pending, expiring grant of tenant membership; owns the
  status/expiry rules governing whether it can be accepted or revoked.
- **Tenant Settings**: Per-tenant configuration, the first tenant-plane entity,
  accessed only through the RLS-bound transaction.
- **Repository (per aggregate)**: A domain-declared interface describing the
  persistence operations the application layer needs, with no SQL detail leaking
  through it.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 100% of the pre-refactor automated tests pass after the refactor
  with no change to their assertions about API behavior.
- **SC-002**: The domain layer has zero dependencies on transport or persistence
  infrastructure, verifiable by an automated dependency/layering check.
- **SC-003**: No HTTP handler contains a business decision; every business rule
  is locatable within a single domain package by following the documented
  layout.
- **SC-004**: Introducing an alternative persistence implementation requires
  changes confined to the adapters layer, with zero edits to domain or
  application code.
- **SC-005**: Every delivered increment merges with a green build and a green
  full test suite; no increment leaves the service unbuildable or non-runnable.
- **SC-006**: Every business rule is covered by a unit test that runs with no
  database or container, and every repository operation is covered by an
  integration test against a real database.

## Assumptions

- This is a structure-only refactor: no new features, endpoints, or behavior
  changes are introduced (consistent with the constitution's "incremental,
  shippable phases" / YAGNI principle).
- The existing PostgreSQL schema, migrations, RLS model, and the
  `nvelope_app` role are unchanged — only Go code organization changes.
- Purely mechanical infrastructure packages (`config`, `logging`, `health`,
  `db`, `token`) have no business logic to extract and remain as-is, used as
  shared infrastructure.
- The `auth` flows are comparatively CRUD-like; the go-ddd-architecture skill
  explicitly cautions against over-applying these patterns to thin services.
  The depth of treatment for each area will be calibrated during planning —
  full domain extraction where genuine rules exist (credential/password policy,
  slug rules, invitation lifecycle, RLS), lighter treatment for pure
  pass-throughs — rather than mechanically applying every layer everywhere.
- The frontend (`frontend/`) is out of scope.
- The `worker`, `scheduler`, and `migrate` commands carry no business logic this
  phase and are out of scope beyond any dependency-wiring adjustments.
- The service is treated as a single service module reorganized into
  domain/application/adapters/ports layers; each aggregate gets its own domain
  sub-package.
- Existing test infrastructure (real-database test helpers) is reused; no test
  framework change is implied.

## Out of Scope

- New API endpoints, new fields, or any change to request/response contracts.
- Database schema or migration changes.
- Frontend changes.
- Refactoring of the `worker`/`scheduler`/`migrate` commands beyond wiring.
- Introducing event sourcing, message queues, or additional persistence
  backends (the repository abstraction merely makes them possible later).
