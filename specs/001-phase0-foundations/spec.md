# Feature Specification: Phase 0 — Foundations & Docs

**Feature Branch**: `001-phase0-foundations`

**Created**: 2026-05-16

**Status**: Draft

**Input**: User description: "Phase 0 — Foundations & Docs: create the nvelope repository and commit the design docs; scaffold the three Go services (cmd/api, cmd/worker, cmd/scheduler) with shared config loading and structured logging; set up PostgreSQL, golang-migrate migration tooling, and a React/Vite frontend skeleton; CI pipeline (build, test, lint), Dockerfiles for each service, and base Kubernetes/Helm manifests. Exit criteria: all three services build and start; CI is green; a migration applies cleanly."

## User Scenarios & Testing *(mandatory)*

<!--
  These user journeys describe Phase 0, a foundations phase. The "users" of this
  feature are the engineers who will build nvelope in later phases, plus the
  automated systems (CI, deployment) that act on their behalf. Each story is an
  independently shippable slice of project groundwork.
-->

### User Story 1 - Backend services build and run locally (Priority: P1)

A developer clones the repository and, with a single documented command per service, builds and starts each of the three backend services (API, Worker, Scheduler). Every service loads its configuration from a shared mechanism, emits structured logs, reports a healthy startup, and shuts down cleanly.

**Why this priority**: Every subsequent phase adds code to these three services. Without a runnable skeleton there is nowhere to put Phase 1+ work, and the project's stated exit criterion "all three services build and start" depends entirely on this story.

**Independent Test**: Clone the repo on a clean machine, run the documented build and start commands for each service, and confirm each one starts, prints structured startup logs, and stops cleanly. Delivers a working multi-service skeleton even if no other Phase 0 story is done.

**Acceptance Scenarios**:

1. **Given** a freshly cloned repository, **When** a developer runs the documented build command, **Then** all three services compile without errors.
2. **Given** a valid configuration, **When** a developer starts any of the three services, **Then** the service starts, emits a structured startup log line identifying the service and its version, and remains running.
3. **Given** a running service, **When** the developer sends a shutdown signal, **Then** the service stops gracefully without leaving orphaned processes.
4. **Given** a missing or invalid required configuration value, **When** a developer starts a service, **Then** the service refuses to start and reports a clear error naming the offending setting.
5. **Given** any of the three services, **When** it emits a log line, **Then** the line is machine-parseable structured output containing at minimum a timestamp, level, message, and service identifier.

---

### User Story 2 - Database schema can be versioned and migrated (Priority: P2)

A developer connects the project to a PostgreSQL database and applies versioned schema migrations forward and backward using the project's migration tooling. A baseline migration applies cleanly against an empty database.

**Why this priority**: Phase 1 onward depends on an evolving database schema. The migration workflow must exist and be proven before any schema work begins. The project's exit criterion "a migration applies cleanly" depends on this story.

**Independent Test**: Point the migration tooling at an empty PostgreSQL database, run the migrate-up command, and confirm the baseline migration applies without error and the recorded schema version advances. Run migrate-down and confirm it reverts cleanly.

**Acceptance Scenarios**:

1. **Given** an empty PostgreSQL database and the documented connection settings, **When** a developer runs the migrate-up command, **Then** all pending migrations apply successfully and the schema version is recorded.
2. **Given** a database at the latest schema version, **When** a developer runs migrate-up again, **Then** the command reports no pending changes and makes no modifications.
3. **Given** a database with applied migrations, **When** a developer runs the migrate-down command, **Then** the most recent migration is reverted and the recorded version decreases.
4. **Given** a developer who needs a new schema change, **When** they follow the documented process to add a migration, **Then** a correctly named, versioned, paired up/down migration file is created in the expected location.

---

### User Story 3 - Every change is automatically validated by CI (Priority: P2)

A developer opens a change against the repository. An automated pipeline builds the project, runs the test suite, and runs linting, and reports a single clear pass/fail status back on the change before it can be merged.

**Why this priority**: CI is the project's continuous guarantee that the foundations stay intact. The exit criterion "CI is green" depends on this story. It is P2 rather than P1 because the services and migrations must exist for CI to have something meaningful to validate.

**Independent Test**: Open a trivial change, observe the pipeline run automatically, and confirm it builds, tests, and lints the project and reports a pass/fail result. Introduce a deliberate failure and confirm the pipeline fails.

**Acceptance Scenarios**:

1. **Given** a change submitted to the repository, **When** the pipeline runs, **Then** it builds all backend services and the frontend, runs the full test suite, and runs linting.
2. **Given** a change that compiles, passes tests, and passes lint, **When** the pipeline completes, **Then** it reports a passing status.
3. **Given** a change that fails to build, fails a test, or fails a lint rule, **When** the pipeline completes, **Then** it reports a failing status and the failing step is identifiable from the output.
4. **Given** the initial Phase 0 codebase, **When** the pipeline runs, **Then** it reports a passing (green) status.

---

### User Story 4 - Frontend skeleton and deployment artifacts exist (Priority: P3)

A developer builds and runs the frontend application skeleton locally, and produces container images for each backend service that can be deployed to a Kubernetes cluster using the project's base deployment manifests.

**Why this priority**: The frontend and deployment artifacts are needed before frontend work (Phase 6) and any cluster deployment, but later phases of work can begin against the backend skeleton without them. They complete the foundations without blocking the P1/P2 critical path.

**Independent Test**: Run the documented frontend dev command and confirm a placeholder application loads in a browser. Build a container image for each backend service and confirm the base deployment manifests reference those images and are accepted by a Kubernetes cluster.

**Acceptance Scenarios**:

1. **Given** a freshly cloned repository, **When** a developer runs the documented frontend dev command, **Then** a placeholder frontend application builds and is viewable in a browser.
2. **Given** the frontend skeleton, **When** a developer runs the documented production build command, **Then** a deployable static bundle is produced without errors.
3. **Given** a backend service, **When** a developer runs the documented image-build command, **Then** a runnable container image is produced for that service.
4. **Given** the base deployment manifests, **When** they are applied to a Kubernetes cluster, **Then** the cluster accepts them as valid and schedules the three services.

---

### Edge Cases

- A required configuration value is absent or malformed → the affected service fails fast at startup with an explicit error rather than running in an undefined state.
- The migration tool runs against an unreachable or unauthorized database → the command fails with a clear connection/permission error and leaves the schema untouched.
- A migration is interrupted partway → the recorded schema version reflects only fully applied migrations, and re-running migrate-up resumes safely.
- Two services are started with conflicting settings (e.g. the same listening port) → the conflict surfaces as a clear startup error rather than a silent failure.
- The CI pipeline encounters a transient infrastructure failure unrelated to the change → the failure is distinguishable from a genuine build/test/lint failure.
- A developer runs the project on a machine missing a required tool version → setup documentation states the required versions so the gap is diagnosable.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The repository MUST contain the project design documentation (architecture, user stories, and implementation plan) under version control so that all later phases share a single source of truth.
- **FR-002**: The project MUST provide three independently buildable, independently runnable backend services corresponding to the API, Worker, and Scheduler roles described in the architecture.
- **FR-003**: All three backend services MUST load configuration through a single shared mechanism so configuration behavior is consistent across services.
- **FR-004**: Each backend service MUST fail to start, with a clear and specific error message, when a required configuration value is missing or invalid.
- **FR-005**: All three backend services MUST emit structured, machine-parseable logs that include at minimum a timestamp, severity level, message, and service identifier.
- **FR-006**: Each backend service MUST start up reporting a healthy state and MUST shut down gracefully on receiving a termination signal.
- **FR-007**: The project MUST integrate with a PostgreSQL database and provide documented connection configuration.
- **FR-008**: The project MUST provide schema migration tooling that applies versioned migrations forward and reverts them backward, and that records the current schema version.
- **FR-009**: The project MUST include at least one baseline migration that applies cleanly against an empty PostgreSQL database.
- **FR-010**: The project MUST document the process for authoring a new versioned migration, including file naming and location conventions.
- **FR-011**: The project MUST include a frontend application skeleton that builds and runs locally and produces a deployable static bundle.
- **FR-012**: The project MUST provide an automated continuous-integration pipeline that runs on every submitted change and performs a build, the test suite, and linting.
- **FR-013**: The CI pipeline MUST report a single clear pass/fail status, and a build, test, or lint failure MUST cause the pipeline to fail with the failing step identifiable.
- **FR-014**: The CI pipeline MUST report a passing (green) status for the Phase 0 codebase.
- **FR-015**: The project MUST provide a means to build a deployable container image for each of the three backend services.
- **FR-016**: The project MUST provide base Kubernetes deployment manifests for the three backend services that a cluster accepts as valid.
- **FR-017**: The repository MUST include developer setup documentation sufficient for a new developer to build and run every service, apply a migration, and run the frontend, including any required tool versions.
- **FR-018**: The repository structure MUST follow the layout defined in the architecture documentation so later phases have predictable locations for new code.

### Key Entities

- **Backend Service**: One of three deployable units (API, Worker, Scheduler). Each has a name/identifier, a configuration set, a logging output, and a lifecycle (start, ready, shut down).
- **Configuration**: The set of named settings a service needs to run (e.g. database connection, listening address, log level). Has a source, a validation state, and a required-vs-optional designation per setting.
- **Migration**: A versioned, ordered pair of forward and reverse schema changes. Has a version number, a name, an applied/unapplied state, and an apply order.
- **CI Pipeline Run**: A single automated validation of a change. Has a triggering change, a set of steps (build, test, lint), and an overall pass/fail outcome.
- **Container Image**: A deployable packaging of one backend service. Associated with exactly one service and referenced by the deployment manifests.
- **Deployment Manifest**: A declarative description of how a service runs on a cluster. References a container image and a service.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A new developer can clone the repository and, following only the setup documentation, build and start all three backend services within 15 minutes on a clean machine.
- **SC-002**: All three backend services start successfully and shut down cleanly with no orphaned processes in 100% of attempts with valid configuration.
- **SC-003**: Starting any service with a missing or invalid required setting fails immediately (within 5 seconds) and the error message names the specific setting in 100% of such cases.
- **SC-004**: The baseline migration applies cleanly to an empty database, and applying then reverting it returns the database to its original empty state, in 100% of attempts.
- **SC-005**: The CI pipeline runs automatically on every submitted change and reports a result without manual intervention.
- **SC-006**: The CI pipeline reports a passing status for the Phase 0 codebase, and a deliberately introduced build, test, or lint failure causes it to report a failing status, with the failing step identifiable from the output.
- **SC-007**: The frontend skeleton builds and serves a viewable placeholder application, and produces a deployable static bundle without errors.
- **SC-008**: A container image can be produced for each of the three backend services, and the base deployment manifests are accepted as valid by a Kubernetes cluster.
- **SC-009**: 100% of the design documentation (architecture, user stories, implementation plan) is present in the repository under version control.

## Assumptions

- The "users" of this phase are the engineers building nvelope and the automated CI/deployment systems acting on their behalf; there are no end-user-facing features in Phase 0.
- Locked-in technology choices from the architecture documentation are treated as given inputs, not decisions to revisit: Go backend services, a React/TypeScript frontend built with Vite, PostgreSQL, `golang-migrate` for migrations, and container deployment on Kubernetes.
- The CI pipeline runs on the hosting platform where the repository lives (the repository's native pipeline service); no separate CI provider needs to be selected.
- "Base Kubernetes/Helm manifests" means a minimal, deployable starting point — placeholder manifests or a skeleton Helm chart sufficient to schedule the three services — not production-hardened, autoscaled, or secret-managed configuration; production rollout is explicitly Phase 7.
- Phase 0 delivers skeletons only: services start and log but expose no business endpoints, the frontend shows only a placeholder, and the baseline migration may be minimal (e.g. an empty or version-tracking-only migration). No tenancy, auth, sending, or billing logic is in scope.
- A PostgreSQL instance is available to developers locally (e.g. via a local container) and to CI; provisioning a managed/hosted database is out of scope for this phase.
- Structured logging uses a conventional JSON-style format; the exact field set beyond timestamp, level, message, and service identifier is left to implementation.
- Redis and S3-compatible object storage, though part of the overall architecture, are not required to be provisioned in Phase 0 because no Phase 0 story depends on them.
