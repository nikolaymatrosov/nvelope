<!--
Sync Impact Report
==================
Version change: 1.0.0 → 1.1.0
Bump rationale: MINOR — adds one new core principle (VI. Layered Architecture &
  Domain Integrity) and materially expands the Architectural Constraints section
  with code-organization rules derived from PATTERNS.md. No principle removed or
  redefined; existing guidance is unchanged.

Modified principles: none renamed or redefined.

Added sections:
  Principle VI. Layered Architecture & Domain Integrity
  Architectural Constraints — three new bullets (code organization, shared
    infrastructure, command/query separation & decorators)

Removed sections: none

Templates requiring updates:
  ✅ .specify/templates/plan-template.md — "Constitution Check" gate is resolved
     dynamically against this file ("Gates determined based on constitution
     file"); no hardcoded principle references, no edit required.
  ✅ .specify/templates/spec-template.md — no constitution/principle references;
     no edit required.
  ✅ .specify/templates/tasks-template.md — no constitution/principle references;
     no edit required.

Follow-up TODOs: none
-->

# nvelope Constitution

## Core Principles

### I. Tenant Isolation by Default

Every tenant's data MUST be inaccessible to every other tenant. Isolation MUST be
enforced as defense in depth — the data layer is the authoritative backstop, never
application code alone. Any feature that touches tenant data MUST carry automated
tests proving that one tenant cannot read or write another tenant's data even when
an application-level filter is accidentally omitted. Control-plane data and
tenant-scoped data MUST remain clearly separated.

Rationale: a single isolation gap breaks the platform's core promise to its
customers. Correctness here is non-negotiable and cannot rely on developer
discipline at every call site.

### II. Test-Backed Delivery (NON-NEGOTIABLE)

No increment is "done" until its behavior is covered by automated tests and those
tests pass. Critical paths — tenant isolation, email sending, billing and quota
enforcement, and asynchronous job processing — MUST have integration coverage that
exercises real boundaries rather than mocking them away. Every delivery phase MUST
exit with a green test suite and a clean schema migration.

Rationale: the platform's risk concentrates in cross-cutting behaviors that unit
tests miss; verifying them against real boundaries is the only reliable safeguard.

### III. Incremental, Shippable Phases

Scope MUST be delivered in independently shippable increments. Each increment MUST
be deployable, verified, and demonstrable on its own. Build for the current phase
only — speculative future needs MUST NOT drive present design (YAGNI). Full feature
parity is an end state reached across phases, never a precondition for earlier
phases.

Rationale: phased delivery keeps the system continuously releasable, surfaces risk
early, and prevents a large up-front scope from blocking all value.

### IV. Security & Consent by Design

Authentication, authorization, verification of external requests, scoped and
least-privilege credentials, audit logging of privileged actions, and explicit
subscriber consent MUST be designed into every feature from the start — never
retrofitted. External services MUST be reached only through authenticated,
least-privilege paths. Privileged or cross-tenant actions MUST be auditable.

Rationale: deliverability, legal compliance, and tenant trust depend on security
and consent being structural properties of the system, not optional add-ons.

### V. Operable & Observable Services

Services MUST be stateless and horizontally scalable, holding no session or work
state that prevents adding or replacing instances. Structured logging, metrics, and
tracing MUST make the running system debuggable in production. Asynchronous and
long-running work MUST be durable and resumable, so a lost or restarted instance
never drops or duplicates work.

Rationale: a multi-tenant SaaS must scale and recover without operator heroics;
observability and durable work are what make incidents diagnosable and survivable.

### VI. Layered Architecture & Domain Integrity

Code MUST be organized so that dependencies point inward only: business logic MUST
NOT import transport, persistence, or external-client packages. The following rules
are non-negotiable regardless of how many physical layers a context uses:

- **Domain logic stays pure.** Business rules and invariants live on domain
  entities, not in anaemic structs manipulated by services. Entities MUST be
  constructible only through validating constructors so an invalid state is
  unrepresentable; loading persisted data uses a separate, explicitly labelled
  hydration path that is documented as "persistence only, not a constructor".
- **Contracts are owned by the consumer.** Repository and external-service
  interfaces MUST be declared by the package that depends on them (the domain or
  use-case layer); infrastructure adapters implement those interfaces. The domain
  defines what it needs; infrastructure conforms.
- **Errors are typed and mapped once.** Errors that cross a domain boundary MUST
  carry a machine-readable kind (e.g. a slug plus an error category). Translation
  from that typed error to a transport status code MUST happen in exactly one
  place; domain and use-case code MUST NOT know about HTTP, gRPC, or status codes,
  and transport code MUST NOT branch on error strings.
- **Composition is explicit.** Dependencies are wired through plain constructors at
  a single composition root; no runtime DI framework and no hidden global state.
  Tests substitute doubles through that same wiring path.

Rationale: an inward dependency rule plus consumer-owned contracts keeps the
codebase testable and changeable as it grows; entities that cannot be built invalid
remove a whole class of bugs; concentrating transport mapping and wiring prevents
infrastructure concerns from leaking into business logic.

## Architectural Constraints

- **Multi-tenancy model**: a single shared datastore with a clear separation
  between control-plane data and tenant-scoped data. Tenant identity is part of
  every tenant-scoped record from the first schema version and is never retrofitted.
- **External-service abstraction**: third-party dependencies (mail delivery,
  payments, object storage) MUST sit behind thin abstractions so they can be tested
  and replaced without rippling through the codebase.
- **Asynchronous work**: long-running and background work runs on a durable,
  retry-capable queue with fairness across tenants, so no single tenant can starve
  others.
- **Bounded consumption**: resource usage is bounded per tenant; quotas and rate
  limits are enforced centrally and applied consistently across all instances.
- **Reference, not copy**: a proven reference implementation may inform the domain
  model and algorithms, but it MUST be adapted to the multi-tenant context rather
  than copied wholesale.
- **Layer scope is proportional to need**: the dependency rule of Principle VI is
  always enforced, but the number of physical layers is not. A small bounded
  context MAY collapse layers (e.g. keep store, service, and handler files in one
  package). A full ports/app/domain/adapters split and contract-first code
  generation are adopted only when multiple services or teams make the ceremony
  worthwhile (YAGNI); they MUST NOT be introduced speculatively.
- **Shared infrastructure lives once**: cross-cutting concerns — server bootstrap
  and middleware, authentication, structured logging, metrics, and shared clients —
  MUST be implemented once in shared packages and reused, never copied per service.
  Per-service entrypoints stay thin.
- **Separate read and write paths**: commands that mutate state and queries that
  read it SHOULD be modelled as distinct handler types and MUST NOT be conflated in
  a single object. Cross-cutting handler behavior (logging, metrics, tracing)
  SHOULD be applied through generic decorators/wrappers so every use case gets
  observability uniformly rather than via hand-written, drift-prone boilerplate.

## Development Workflow & Quality Gates

- Every delivery phase has explicit, written exit criteria; a phase is not complete
  until those criteria are met.
- Code review MUST verify compliance with the Core Principles. Tenant isolation,
  security, test coverage, and the inward dependency rule are review gates, not
  afterthoughts.
- Implementation plans MUST pass a Constitution Check before work begins. Any
  deviation MUST be recorded with its justification and the simpler alternative that
  was rejected.
- The standard verification bundle for every phase: the full automated test suite,
  tenant-isolation tests, linting, and a clean migration apply.
- Architecture, requirements, and the implementation plan are kept under version
  control as the single source of truth and updated whenever decisions change.

## Governance

- This constitution supersedes ad-hoc practice. Where guidance conflicts, the
  constitution prevails.
- Amendments MUST be proposed in writing, reviewed, and recorded with a version bump
  and a dated entry in the Sync Impact Report.
- Versioning policy (semantic):
  - **MAJOR**: removal or backward-incompatible redefinition of a principle or
    governance rule.
  - **MINOR**: a new principle or section, or materially expanded guidance.
  - **PATCH**: clarifications, wording, and non-semantic refinements.
- Compliance is reviewed at every plan gate and in every code review. Unjustified
  complexity or unaddressed principle violations block merge.
- Runtime, contributor-facing development guidance lives in the repository docs
  (`docs/architecture.md`, `docs/implementation-plan.md`, `docs/user-stories.md`,
  `PATTERNS.md`, and `CLAUDE.md`). These MUST stay consistent with this
  constitution; on conflict, the constitution governs and the docs are corrected.

**Version**: 1.1.0 | **Ratified**: 2026-05-16 | **Last Amended**: 2026-05-16
