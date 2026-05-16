---
description: "Task list for the DDD / Clean Architecture refactor of the backend"
---

# Tasks: DDD / Clean Architecture Refactor of the Backend

**Input**: Design documents from `/specs/003-ddd-architecture-refactor/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/

**Tests**: Test tasks are included. Constitution Principle II (Test-Backed
Delivery) is NON-NEGOTIABLE and FR-011 mandates layer-organized tests, so tests
are part of the work, not optional. This is a strangler refactor of existing,
already-tested code: the existing `internal/api` and `test/` suites are the
behavior baseline and MUST keep passing unchanged (FR-012); new test tasks add
the per-layer coverage (domain unit, adapter integration, handler unit).

**Organization**: Tasks are grouped by user story. Because this is a refactor,
the stories build on each other (US2 needs US1's domain types; US3 needs US2's
adapters) — each is independently *testable* at its checkpoint, but they are
executed in priority order, not in parallel.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[Story]**: US1, US2, US3 — maps to the spec's user stories
- Every task names exact file paths

## Path Conventions

Go monorepo. Backend code under `internal/` and `cmd/`; the refactor target
layout is in plan.md "Source Code". Strangler approach: new layered packages are
built alongside the existing `internal/auth`, `internal/tenant`, `internal/api`
flat code; callers cut over per increment; old code is deleted in the final phase.

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Tooling and skeleton needed before any layer is written

- [x] T001 Install the dependency-rule linter: `go install github.com/roblaszczak/go-cleanarch@latest`; verify it runs against the repo root with `go-cleanarch` and document the command in `specs/003-ddd-architecture-refactor/quickstart.md` is already present — confirm it matches.
- [x] T002 [P] Add a `lint-arch` target to `Makefile` that runs `go-cleanarch`, and a `verify` target that runs `go build ./...`, `go vet ./...`, `go-cleanarch`, and `go test ./...` (the per-increment verification bundle from quickstart.md).
- [x] T003 Create the empty package directories for the target layout from plan.md: `internal/platform/apperr/`, `internal/platform/decorator/`, `internal/auth/domain/`, `internal/auth/app/command/`, `internal/auth/app/query/`, `internal/auth/adapters/`, `internal/tenant/domain/`, `internal/tenant/app/command/`, `internal/tenant/app/query/`, `internal/tenant/adapters/` (add a `doc.go` package declaration to each so `go build ./...` stays green).

**Checkpoint**: `make verify` passes against the unchanged codebase plus empty packages.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Shared building blocks every bounded context depends on — the typed
error kind and the CQRS handler decorators. Plan increment 1.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete — the
domain layer (US1) returns `apperr` errors and the app layer (US3) uses the
decorators.

- [x] T004 [P] Implement the typed error in `internal/platform/apperr/apperr.go`: an error carrying a machine-readable `slug` (string) and a `Category` (enum: `IncorrectInput`, `Conflict`, `NotFound`, `Authorization`, `Unknown`), with constructors per category, a `Slug()`/`Category()` accessor pair, and `errors.Is`/`errors.As` support. See research.md §6 and contracts/layering.md.
- [x] T005 [P] Unit-test `internal/platform/apperr/apperr_test.go`: category construction, slug round-trip, `errors.As` extraction, wrapping behavior. No infrastructure.
- [x] T006 [P] Implement generic CQRS decorators in `internal/platform/decorator/decorator.go`: `CommandHandler[C]` and `QueryHandler[Q,R]` interfaces, plus `ApplyCommandDecorators`/`ApplyQueryDecorators` that wrap a handler with structured `slog` logging (handler name, duration, error). See research.md §7 and PATTERNS.md #2.
- [x] T007 [P] Unit-test `internal/platform/decorator/decorator_test.go`: a wrapped handler runs, logs once, and propagates result/error unchanged.

**Checkpoint**: `internal/platform/*` compiles, is unit-tested, and imports only stdlib (`go-cleanarch` clean).

---

## Phase 3: User Story 1 - Business rules isolated and unit-testable (Priority: P1) 🎯 MVP

**Goal**: Extract pure domain entities for both bounded contexts — validating
constructors, hydration paths, behavior methods, typed errors, and
consumer-owned repository interfaces — covered by infrastructure-free unit tests.
Plan increment 2.

**Independent Test**: `go test ./internal/auth/domain/... ./internal/tenant/domain/...`
passes with no database and no Docker; `go list -deps` of those packages shows
no `net/http`, `pgx`, or `chi` import (quickstart.md "Verifying the layering").

### Tests for User Story 1 ⚠️ write first, watch fail, then implement

- [x] T008 [P] [US1] Unit tests for the auth domain in `internal/auth/domain/user_test.go` and `internal/auth/domain/session_test.go`: `NewUser` rejects bad email / empty name; Email value object normalization; Password length bounds (8–72); `Session.IsLive`/`Revoke`/expiry. Per data-model.md "auth context".
- [x] T009 [P] [US1] Unit tests for the tenant domain in `internal/tenant/domain/{tenant,invitation,settings}_test.go`: Slug validity + reserved-set + `DeriveSlug`; `NewTenant`; Membership role validation; Invitation `IsAcceptable`/`Accept`/`Revoke` state machine and opaque error; `TenantSettings` `Rename`/`SetTimezone`. Per data-model.md "tenant context".

### Implementation for User Story 1

- [x] T010 [P] [US1] Implement the auth User aggregate in `internal/auth/domain/user.go`: unexported fields, getters, `NewUser` validating constructor, `HydrateUser` (documented "persistence only — not a constructor"). Port the rules from today's `internal/auth/service.go` (`validateCredentials`, `ValidEmail`, `normalizeEmail`).
- [x] T011 [P] [US1] Implement Email and Password value objects in `internal/auth/domain/credentials.go`: `NewEmail` (trim + shape regex), `NewPassword` (8–72 byte bound); both return `apperr` `IncorrectInput` on failure.
- [x] T012 [P] [US1] Implement the Session aggregate in `internal/auth/domain/session.go`: `NewSession(userID, ttl)`, `HydrateSession`, `IsLive(now)`, `Revoke(now)`. Port from `internal/auth/sessions.go`.
- [x] T013 [P] [US1] Declare the auth typed errors in `internal/auth/domain/errors.go` as `apperr` values: `ErrEmailTaken` (Conflict), `ErrUserNotFound` (NotFound), `ErrSessionInvalid` (Authorization), `ErrInvalidCredentials` (Authorization). Preserve the response slugs from contracts/http-api.md.
- [x] T014 [US1] Declare the auth repository interfaces in `internal/auth/domain/repository.go`: `UserRepository`, `SessionRepository` per contracts/repositories.md (depends on T010, T012 for the entity types).
- [x] T015 [P] [US1] Implement the Tenant aggregate and Slug value object in `internal/tenant/domain/tenant.go`: unexported fields, `NewTenant(name, slug)`, `HydrateTenant`, `Slug` value object with the 3–63 char rule + reserved set, `DeriveSlug(name)`. Port from `internal/tenant/tenants.go`.
- [x] T016 [P] [US1] Implement the Membership aggregate in `internal/tenant/domain/membership.go`: `Role` enum (`owner`, `admin`), `NewMembership`, `HydrateMembership`.
- [x] T017 [P] [US1] Implement the Invitation aggregate in `internal/tenant/domain/invitation.go`: `NewInvitation`, `HydrateInvitation`, `IsAcceptable(now)`, `Accept(now)`, `Revoke()` and the `InvitationStatus` state machine. Port from `internal/tenant/invitations.go`; keep the opaque `invitation_not_found`.
- [x] T018 [P] [US1] Implement the TenantSettings aggregate in `internal/tenant/domain/settings.go`: `NewTenantSettings`, `HydrateTenantSettings`, `Rename`, `SetTimezone`. Port the validation from `internal/tenant/settings.go`.
- [x] T019 [P] [US1] Declare the tenant typed errors in `internal/tenant/domain/errors.go` as `apperr` values: `ErrSlugTaken` (Conflict), `ErrTenantNotFound` (NotFound), `ErrNotMember` (NotFound — opaque), `ErrInvitationExists` (Conflict), `ErrInvitationNotFound` (NotFound). Preserve slugs from contracts/http-api.md.
- [x] T020 [US1] Declare the tenant repository interfaces in `internal/tenant/domain/repository.go`: `TenantRepository`, `InvitationRepository`, `SettingsRepository` per contracts/repositories.md (depends on T015–T018).

**Checkpoint**: domain unit suites green; both `domain` packages import only stdlib + `apperr` (`go-cleanarch` clean). Old flat code still serves traffic. **MVP — independently demoable as a tested, pure domain layer.**

---

## Phase 4: User Story 2 - Persistence behind repository interfaces (Priority: P2)

**Goal**: Implement the domain-owned repository interfaces with pgx in an
adapters layer; move the RLS-bound transaction into the tenant adapter; add the
bcrypt password adapter. Plan increment 3.

**Independent Test**: `go test ./internal/auth/adapters/... ./internal/tenant/adapters/...`
passes against a real PostgreSQL instance (via `internal/dbtest`), and the
US1 domain unit suite still passes unchanged.

### Tests for User Story 2 ⚠️ write first, watch fail, then implement

- [x] T021 [P] [US2] Integration tests in `internal/auth/adapters/users_pg_test.go` and `sessions_pg_test.go`: create/get/lookup user, `ErrEmailTaken` on duplicate, issue/resolve/revoke session, `ErrSessionInvalid` for expired/revoked tokens. Real DB via `internal/dbtest`. Port assertions from `internal/auth/users_test.go` and `sessions_test.go`.
- [x] T022 [P] [US2] Integration tests in `internal/tenant/adapters/{tenants,invitations,settings}_pg_test.go`: workspace creation transaction, slug-taken, memberships/members, invitation lifecycle, and RLS-bound settings get/update. Port assertions from `internal/tenant/{tenants,invitations,rls}_test.go`.

### Implementation for User Story 2

- [x] T023 [P] [US2] Implement `UserRepository` in `internal/auth/adapters/users_pg.go`: a private `pgxUser` row struct mapped to/from `domain.User`; translate `pgx.ErrNoRows` and SQLSTATE 23505 to the typed domain errors. Port SQL from `internal/auth/users.go`.
- [x] T024 [P] [US2] Implement `SessionRepository` in `internal/auth/adapters/sessions_pg.go`: issue/resolve/revoke by token hash, using `internal/token` for hashing. Port SQL from `internal/auth/sessions.go`.
- [x] T025 [P] [US2] Implement the `PasswordHasher` adapter in `internal/auth/adapters/password_bcrypt.go`: wrap `internal/auth/password.go`'s bcrypt hash/verify behind the interface declared by `auth/app`.
- [x] T026 [P] [US2] Move the RLS helper to `internal/tenant/adapters/rls.go`: relocate `WithTenant` from `internal/tenant/rls.go` unchanged; it becomes adapter-private. See research.md §5.
- [x] T027 [US2] Implement `TenantRepository` in `internal/tenant/adapters/tenants_pg.go`: `CreateWorkspace` runs the tenant + owner-membership + initial-settings inserts in one transaction, binding `app.tenant_id` before the settings insert; plus get-by-slug/id, memberships, role, members. Port SQL from `internal/tenant/tenants.go` (depends on T026).
- [x] T028 [P] [US2] Implement `InvitationRepository` in `internal/tenant/adapters/invitations_pg.go`: create, get-pending-by-token-hash, list-pending, and the `Update` closure (load → fn → persist) used for accept/revoke. Port SQL from `internal/tenant/invitations.go`; translate SQLSTATE 23505/22P02.
- [x] T029 [US2] Implement `SettingsRepository` in `internal/tenant/adapters/settings_pg.go`: `Get` and `Update` each run inside a `WithTenant` transaction so RLS is bound; the app layer never sees the GUC. Port SQL from `internal/tenant/settings.go` (depends on T026).

**Checkpoint**: adapter integration suites green against a real DB; `go-cleanarch` clean (adapters depend inward on `domain` only). US1 domain suite still green. Old flat code still serves traffic.

---

## Phase 5: User Story 3 - Application layer (commands/queries) and HTTP confined to ports (Priority: P3)

**Goal**: Add command/query handlers in business language, the composition root,
then cut the HTTP layer over to call handlers and map errors in one place — with
byte-identical API behavior. Plan increments 4 and 5.

**Independent Test**: `go test ./internal/api/... ./test/...` passes with no
assertion changes — every route returns the same status codes, JSON shapes, and
error slugs as before (contracts/http-api.md); the cross-tenant isolation suite
still passes as `nvelope_app`.

### Application layer — increment 4

- [x] T030 [P] [US3] Auth command handlers in `internal/auth/app/command/`: `signup.go` (`SignUp`), `login.go` (`LogIn`), `logout.go` (`LogOut`), `register_invited_user.go` (`RegisterInvitedUser` — account creation for the invite-accept flow). Each is a handler type with a `NewXxxHandler` constructor that panics on nil deps; port orchestration from `internal/auth/service.go`.
- [x] T031 [P] [US3] Auth query handler in `internal/auth/app/query/authenticate_session.go`: `AuthenticateSession` resolves a raw session token to a `domain.User` (cookie → session → user), returning a read-model view. Port from `internal/auth/middleware.go` `resolveUser`.
- [x] T032 [US3] Auth application struct in `internal/auth/app/application.go`: `Application{ Commands, Queries }` plus the `PasswordHasher` interface declaration (depends on T030, T031).
- [x] T033 [P] [US3] Tenant command handlers in `internal/tenant/app/command/`: `create_workspace.go`, `invite_teammate.go`, `accept_invitation.go`, `revoke_invitation.go`, `update_settings.go`. `AcceptInvitation` MUST preserve today's single-transaction atomicity across account creation, invitation accept, and membership add (see `internal/api/platform_handlers.go` `handleAcceptInvitation`) — coordinate with the auth side via an interface declared here, wired at the composition root. Port orchestration from `internal/tenant/*.go`.
- [x] T034 [P] [US3] Tenant query handlers in `internal/tenant/app/query/`: `list_workspaces.go`, `resolve_workspace.go` (slug + membership cross-check, opaque), `workspace_members.go`, `get_settings.go`, `pending_invitations.go`, `look_up_invitation.go`. Each returns a flat read-model struct whose JSON matches contracts/http-api.md.
- [x] T035 [US3] Tenant application struct in `internal/tenant/app/application.go`: `Application{ Commands, Queries }` (depends on T033, T034).
- [x] T036 [P] [US3] Handler unit tests in `internal/auth/app/**/*_test.go` and `internal/tenant/app/**/*_test.go`: exercise each command/query against in-memory repository fakes implementing the domain interfaces — no database.
- [x] T037 [US3] Composition root in `internal/service/application.go`: `NewApplication(ctx, deps)` constructs the pgx adapters, builds every handler with `decorator` wrappers applied, returns the wired `Application{ Auth, Tenant }` + cleanup func; add a parallel test constructor that wires in-memory fakes. See research.md §8 (depends on T032, T035, and all of Phase 4).

### Ports cutover — increment 5

- [x] T038 [US3] Single error mapping in `internal/api/errmap.go`: map `apperr.Category` → HTTP status and write the `{error, message}` envelope; replace the sentinel-matching `fail`/`validationMessage` in `internal/api/server.go`. Statuses and slugs per contracts/http-api.md.
- [x] T039 [US3] Rewrite `internal/api/middleware.go`: session-cookie auth middleware calls the `AuthenticateSession` query; tenant-resolution middleware calls the `ResolveWorkspace` query. Replace direct `auth`/`tenant` package calls and the `*pgxpool.Pool` field; move cookie helpers (`SetSessionCookie` etc.) here from `internal/auth/middleware.go`.
- [x] T040 [US3] Rewrite `internal/api/platform_handlers.go`: each handler decodes input, builds one command/query, invokes its handler, renders result or routes the error through `errmap`. No business decisions in handlers (SC-003). Preserve every response shape in contracts/http-api.md, including the `accept-invitation` `already_member` / `email_taken` quirks.
- [x] T041 [US3] Rewrite `internal/api/tenant_handlers.go` the same way; remove the direct `tenant.WithTenant` call (settings now go through `SettingsRepository`/the query+command handlers).
- [x] T042 [US3] Update `internal/api/server.go`: `Server` holds the wired `Application` instead of `*pgxpool.Pool`; `New` takes the `Application`; the route table in `Handler()` is unchanged (same paths/methods).
- [x] T043 [US3] Thin `cmd/api/main.go`: open the pool, call `service.NewApplication`, pass the `Application` to `api.New`, start the server; remove the inline wiring.

**Checkpoint**: `go test ./internal/api/... ./test/...` green with zero assertion changes; the isolation suite passes as `nvelope_app`; the service starts and serves the quickstart.md smoke test. All traffic now flows through the layered code.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Remove the superseded code and lock the architecture in. Plan increment 6.

- [x] T044 Delete the superseded flat files now that nothing imports them: `internal/auth/service.go`, `users.go`, `sessions.go`, `password.go`, `middleware.go`, `internal/auth/helpers_test.go` (and its tests once ported); `internal/tenant/tenants.go`, `invitations.go`, `settings.go`, `rls.go`, `middleware.go`, `helpers_test.go`; relocated `internal/tenant/rls.go`. Confirm `go build ./...` and `go test ./...` stay green.
- [x] T045 Verify no orphaned tests remain: any assertion from the old `internal/auth/*_test.go` / `internal/tenant/*_test.go` not covered by the new domain/adapter suites is ported to the correct layer (FR-011); delete the old test files once covered.
- [x] T046 [P] Add `go-cleanarch` to the CI pipeline (the workflow under `.github/` or the CI config added in commit `8272259`) as a required step, so the inward dependency rule (SC-002) fails the build on violation.
- [x] T047 [P] Update `PATTERNS.md` and `docs/architecture.md` §10 to describe the calibrated `domain`/`app`/`adapters` + shared `api` layout actually adopted, so the docs stay consistent with the constitution (constitution Governance clause).
- [x] T048 Run the full quickstart.md verification bundle (`go build`, `go vet`, `go-cleanarch`, `go test ./...`) and the manual smoke test; confirm SC-001 through SC-006 hold.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: no dependencies — start immediately.
- **Foundational (Phase 2)**: depends on Setup — BLOCKS all user stories (`apperr` is used by every domain package; `decorator` by every app handler).
- **US1 (Phase 3)**: depends on Foundational. Delivers the MVP — a tested pure domain layer.
- **US2 (Phase 4)**: depends on US1 — adapters implement the domain interfaces declared in T014/T020.
- **US3 (Phase 5)**: depends on US2 — handlers and the composition root need the concrete repositories.
- **Polish (Phase 6)**: depends on US3 — old code can only be deleted once all traffic flows through the new layers.

### Story independence note

Unlike a feature build, these stories are sequential by nature (each layer
consumes the one below). Each is independently *testable* at its checkpoint, and
each leaves the build green and the service runnable (strangler approach), so the
work is still safely shippable increment by increment — but the stories cannot be
staffed in parallel.

### Within each phase

- Tests are written before the implementation they cover and watched fail first.
- Domain entities (T010–T013, T015–T019) before the repository interfaces that name them (T014, T020).
- Repository interfaces before their adapters (Phase 4).
- Adapters before the composition root (T037).
- App handlers (T030–T036) before the ports cutover (T038–T043).
- Ports cutover before deleting old code (Phase 6).

### Parallel Opportunities

- Phase 1: T002 ∥ (T001→T003).
- Phase 2: T004/T005 ∥ T006/T007 (apperr and decorator are independent).
- Phase 3: the two test tasks T008 ∥ T009; then all entity tasks marked [P] across the two contexts run in parallel — T010, T011, T012, T013 (auth) and T015, T016, T017, T018, T019 (tenant) touch separate files. The interface tasks T014 and T020 each wait on their context's entities.
- Phase 4: T021 ∥ T022; adapters T023/T024/T025/T026/T028 are [P]; T027 waits on T026, T029 waits on T026.
- Phase 5: T030 ∥ T031 ∥ T033 ∥ T034 (separate files); T036 is [P]; T032/T035 wait on their handlers; T037 waits on both applications; the ports cutover T038–T043 is mostly sequential (shared `internal/api` files and the router).

---

## Parallel Example: User Story 1

```bash
# After T008/T009 (tests) are written and failing, build entities in parallel:
Task: "Implement auth User aggregate in internal/auth/domain/user.go"          # T010
Task: "Implement Email/Password value objects in internal/auth/domain/credentials.go"  # T011
Task: "Implement Session aggregate in internal/auth/domain/session.go"          # T012
Task: "Declare auth typed errors in internal/auth/domain/errors.go"             # T013
Task: "Implement Tenant + Slug in internal/tenant/domain/tenant.go"             # T015
Task: "Implement Membership in internal/tenant/domain/membership.go"            # T016
Task: "Implement Invitation in internal/tenant/domain/invitation.go"            # T017
Task: "Implement TenantSettings in internal/tenant/domain/settings.go"          # T018
Task: "Declare tenant typed errors in internal/tenant/domain/errors.go"         # T019
# Then T014 (auth repo interfaces) and T020 (tenant repo interfaces).
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup.
2. Complete Phase 2: Foundational (`apperr`, `decorator`) — blocks all stories.
3. Complete Phase 3: US1 — the pure, unit-tested domain layer.
4. **STOP and VALIDATE**: domain unit suites green, `go-cleanarch` clean, service still runs on the old code path.

### Incremental Delivery (strangler)

1. Setup + Foundational → shared building blocks ready.
2. US1 → tested pure domain, old code still serving → ship.
3. US2 → repositories implemented and integration-tested, old code still serving → ship.
4. US3 → handlers + composition root + ports cutover, all traffic on new layers → ship.
5. Polish → delete old code, enforce `go-cleanarch` in CI → ship.

Every step ends with `make verify` green and the service runnable — no increment
leaves the build red or the API behavior changed.

---

## Notes

- [P] = different files, no dependency on an incomplete task.
- This is a behavior-preserving refactor: if an existing `internal/api` or
  `test/` assertion has to change, that is a regression, not a refactor — stop
  and fix the code, not the test (spec edge case "Behavior preservation").
- Preserve every response slug and status in contracts/http-api.md exactly.
- Keep every test `t.Parallel()`-safe.
- Commit after each task or logical group; never commit a red build.
