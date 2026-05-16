---
description: "Task list for Phase 1 — Tenancy Core implementation"
---

# Tasks: Phase 1 — Tenancy Core

**Input**: Design documents from `/specs/002-tenancy-core/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/

**Tests**: Test tasks ARE included — Constitution Principle II (Test-Backed Delivery) is
non-negotiable, and spec FR-019 explicitly requires automated cross-tenant isolation tests.
Critical-path tests run against a real PostgreSQL instance, never a mock.

**Organization**: Tasks are grouped by user story so each story can be implemented and tested
independently.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[Story]**: US1, US2, US3 — maps to the user stories in spec.md
- Every task lists an exact file path

## Path Conventions

Web-application monorepo (per plan.md): Go backend at the repo root (`cmd/`, `internal/`,
`test/`), React frontend under `frontend/`.

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Project dependencies and configuration needed before any implementation.

- [X] T001 Add the `golang.org/x/crypto` dependency for bcrypt — run `go get golang.org/x/crypto/bcrypt` and commit the updated `go.mod` and `go.sum`
- [X] T002 [P] Extend `Config` in `internal/config/config.go` with `MigrateDatabaseURL`, `SessionTTL`, `InviteTTL`, and `BaseURL`, including defaults (`SessionTTL`/`InviteTTL` = 168h, `MigrateDatabaseURL` falls back to `DatabaseURL`) and validation
- [X] T003 [P] Update `internal/config/config_test.go` to cover the new config fields, their defaults, and validation
- [X] T004 [P] Add the Phase 1 variables (`NVELOPE_MIGRATE_DATABASE_URL`, `NVELOPE_SESSION_TTL`, `NVELOPE_INVITE_TTL`, `NVELOPE_BASE_URL`) with comments to `.env.example`

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Schema, RLS infrastructure, and HTTP scaffolding that every user story depends on.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete.

- [X] T005 Update `cmd/migrate/main.go` to connect via `NVELOPE_MIGRATE_DATABASE_URL` (the privileged role), falling back to `NVELOPE_DATABASE_URL` when unset
- [X] T006 Create migration `internal/db/migrations/000002_app_role_and_extensions.up.sql` and `.down.sql` — enable the `citext` extension and create the restricted `nvelope_app` role (`LOGIN`, dev-default password, NOT superuser, NOT `BYPASSRLS`)
- [X] T007 Create migration `internal/db/migrations/000003_control_plane.up.sql` and `.down.sql` — `platform_users`, `platform_sessions`, `tenants`, `platform_user_tenants`, `invitations` tables with constraints/indexes per data-model.md, and `GRANT` DML to `nvelope_app`
- [X] T008 Create migration `internal/db/migrations/000004_tenant_settings_rls.up.sql` and `.down.sql` — the `tenant_settings` table, `ENABLE`+`FORCE ROW LEVEL SECURITY`, the `tenant_isolation` policy (`USING`/`WITH CHECK` on `app.tenant_id`), and `GRANT` DML to `nvelope_app`
- [X] T009 Extend `test/migrate_test.go` to verify migrations 000002–000004 apply and revert cleanly against a real PostgreSQL instance
- [X] T010 [P] Create `internal/api/respond.go` — JSON response writer and the `{error, message}` error-envelope helpers used by all handlers
- [X] T011 [P] Create `internal/tenant/rls.go` — the `WithTenant(ctx, pool, tenantID, fn)` helper that opens a transaction, runs `SELECT set_config('app.tenant_id', $1, true)`, and commits/rolls back
- [X] T012 [P] Create `internal/tenant/rls_test.go` — verify `WithTenant` binds the tenant for the transaction's lifetime and rolls back when `fn` returns an error (real DB)
- [X] T013 Create `internal/api/router.go` — a chi router that mounts `/healthz` and exposes registration points for `/api/platform/...` and `/t/{slug}/api/...`; update `cmd/api/main.go` to build its router via `internal/api`

**Checkpoint**: Schema migrates cleanly, the RLS helper works, and the API skeleton runs —
user story implementation can begin.

---

## Phase 3: User Story 1 - Sign up and create a workspace (Priority: P1) 🎯 MVP

**Goal**: A new person registers a platform account, logs in, and creates a tenant — becoming
its first (owner) member — reachable at a unique workspace address.

**Independent Test**: On a clean database, register an account, log in, and create a tenant;
confirm an authenticated session is established, the tenant exists with a unique slug, and the
creator is recorded as an `owner` member.

### Tests for User Story 1

- [X] T014 [P] [US1] Unit tests for bcrypt hash/verify in `internal/auth/password_test.go`
- [X] T015 [P] [US1] Integration tests for the `platform_sessions` store (issue, resolve-by-token, revoke, expiry) in `internal/auth/sessions_test.go` (real DB)
- [X] T016 [P] [US1] Tests for the tenants store and slug validation (regex, reserved words, case-insensitive uniqueness) in `internal/tenant/tenants_test.go` (real DB)
- [X] T017 [P] [US1] Integration tests for signup, login, logout, `me`, and tenant create/list endpoints in `internal/api/handlers_test.go` (real DB)

### Implementation for User Story 1

- [X] T018 [P] [US1] Implement bcrypt password hash/verify (cost 12) in `internal/auth/password.go`
- [X] T019 [P] [US1] Implement the `platform_users` store (create, get-by-email, get-by-id) in `internal/auth/users.go`
- [X] T020 [US1] Implement the `platform_sessions` store (issue with hashed token, resolve-by-token, revoke) in `internal/auth/sessions.go`
- [X] T021 [US1] Implement the session-cookie auth middleware (resolve `nv_session`, attach the user to the request context, `401` on miss) in `internal/auth/middleware.go`
- [X] T022 [US1] Implement signup/login/logout orchestration (validation, hashing, session issue/revoke) in `internal/auth/service.go`
- [X] T023 [P] [US1] Implement the `tenants` + `platform_user_tenants` store, including slug validation and reserved-word checks, in `internal/tenant/tenants.go`
- [X] T024 [US1] Implement the `tenant_settings` store (create, get, update) — used through `WithTenant` — in `internal/tenant/settings.go`
- [X] T025 [US1] Implement the tenant-creation flow in `internal/tenant/tenants.go` — one transaction inserting the `tenants` row, the creator's `owner` `platform_user_tenants` row, and the tenant's initial `tenant_settings` row (depends on T023, T024, T011)
- [X] T026 [US1] Implement the platform handlers (signup, login, logout, `me`, create/list tenants) per `contracts/platform-api.md` in `internal/api/platform_handlers.go`
- [X] T027 [US1] Register the US1 platform routes (`/api/platform/...`) in `internal/api/router.go`
- [X] T028 [P] [US1] Create the typed API client (signup, login, logout, me, tenants) in `frontend/src/lib/api.ts`
- [X] T029 [P] [US1] Create the signup page in `frontend/src/routes/signup.tsx`
- [X] T030 [P] [US1] Create the login page in `frontend/src/routes/login.tsx`
- [X] T031 [P] [US1] Create the tenant-creation page in `frontend/src/routes/tenants.new.tsx`
- [X] T032 [US1] Replace the placeholder `frontend/src/routes/index.tsx` with the tenant-list / entry page (redirects to login when unauthenticated)

**Checkpoint**: User Story 1 is fully functional — signup, login, and tenant creation work end
to end. This is the MVP.

---

## Phase 4: User Story 3 - Tenant data stays isolated (Priority: P1)

**Goal**: Every tenant-plane read/write is confined to one tenant at the storage layer, and a
non-member is denied access to a tenant workspace opaquely.

**Independent Test**: Seed two tenants' `tenant_settings` rows directly; connected as
`nvelope_app` and bound to tenant A, run reads/updates/deletes with the application-level
tenant filter omitted and confirm tenant B is never read, modified, or deleted, and that an
insert targeting tenant B is rejected. Request tenant B's workspace as a non-member and confirm
a `404`.

### Tests for User Story 3

- [X] T033 [P] [US3] Cross-tenant isolation suite in `test/isolation_test.go` — seed two tenants, connect as `nvelope_app`, and assert (with app-level filters omitted) that reads/updates/deletes never cross tenants and that an `INSERT` targeting another tenant is rejected by the RLS `WITH CHECK`
- [X] T034 [P] [US3] Tenant resolution middleware tests in `internal/tenant/middleware_test.go` — a member is allowed; a non-member and an unknown slug both yield an identical `404`

### Implementation for User Story 3

- [X] T035 [US3] Implement the tenant resolution + membership cross-check middleware (resolve `{slug}`, verify `platform_user_tenants`, opaque `404` on miss, bind `tenant_id` into the request context) in `internal/tenant/middleware.go`
- [X] T036 [US3] Implement the tenant-scoped handlers (`GET tenant` info + members, `GET`/`PUT settings`) per `contracts/tenant-api.md` in `internal/api/tenant_handlers.go`
- [X] T037 [US3] Register the `/t/{slug}/api/...` routes behind the auth + resolution middleware in `internal/api/router.go`
- [X] T038 [P] [US3] Create the tenant workspace page (tenant info, member list, settings view/edit) in `frontend/src/routes/t.$slug.tsx`

**Checkpoint**: Cross-tenant isolation is proven by the test suite, and non-members are denied
opaquely. User Stories 1 and 3 both work independently.

---

## Phase 5: User Story 2 - Invite a teammate (Priority: P2)

**Goal**: A member invites a teammate by email; the teammate accepts and joins the same tenant.

**Independent Test**: As a member of an existing tenant, invite a second email; accept the
invitation as that person (creating an account on the accept path); confirm they become a
member and can reach the same tenant workspace.

### Tests for User Story 2

- [X] T039 [P] [US2] Invitations store tests in `internal/tenant/invitations_test.go` — create, lookup-by-token, accept, revoke, expiry, and the already-member no-op (real DB)
- [X] T040 [P] [US2] Integration tests for the invite, invitation-lookup, and accept endpoints (including the signup-and-accept path) in `internal/api/handlers_test.go`

### Implementation for User Story 2

- [X] T041 [US2] Implement the `invitations` store with token generation and SHA-256 hashing (create, lookup-by-token-hash, list-pending, revoke) in `internal/tenant/invitations.go`
- [X] T042 [US2] Implement the invitation-acceptance flow (re-validate pending/unexpired, create-or-resolve user, insert membership unless present, mark `accepted`) in `internal/tenant/invitations.go` (depends on T041)
- [X] T043 [US2] Add the platform invitation handlers (`GET /invitations/{token}`, `POST /invitations/{token}/accept` with the signup-and-accept path) to `internal/api/platform_handlers.go`
- [X] T044 [US2] Add the tenant invitation handlers (`POST` create invite returning `accept_url`, `GET` list, `DELETE` revoke) to `internal/api/tenant_handlers.go`
- [X] T045 [US2] Register the US2 invitation routes in `internal/api/router.go`
- [X] T046 [P] [US2] Create the invitation-acceptance page in `frontend/src/routes/invite.$token.tsx`
- [X] T047 [US2] Add the invite UI (send invite, copy `accept_url`, list pending invitations) to `frontend/src/routes/t.$slug.tsx`

**Checkpoint**: All three user stories are independently functional.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Verification and hardening across all stories.

- [X] T048 Security pass — confirm no secrets or raw tokens are logged, only token **hashes** are persisted, and the session cookie is `HttpOnly`/`Secure`/`SameSite=Lax`
- [X] T049 [P] Update `docs/architecture.md` / `docs/implementation-plan.md` if any Phase 1 decision diverged from the original design
- [X] T050 [P] Run `make lint` and resolve all findings
- [X] T051 Run the full `make test` (including the cross-tenant isolation suite) and confirm a green result
- [X] T052 Verify `make migrate-up` then `make migrate-down` apply and revert cleanly end to end
- [X] T053 Walk `specs/002-tenancy-core/quickstart.md` end to end (API and frontend) and confirm every exit-checklist item passes

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — start immediately.
- **Foundational (Phase 2)**: Depends on Setup — BLOCKS all user stories.
- **User Stories (Phases 3–5)**: All depend on Foundational. US1 and US3 are both P1 and
  mutually independent; US2 (P2) builds on US1.
- **Polish (Phase 6)**: Depends on all targeted user stories being complete.

### User Story Dependencies

- **US1 (P1)**: Depends only on Foundational.
- **US3 (P1)**: Depends only on Foundational. Its core (isolation suite + resolution
  middleware) is fully independent of US1 — the isolation test seeds tenants directly. The
  `GET`/`PUT settings` handlers reuse the `tenant_settings` store built in US1 (T024); if US3
  is built before US1, create that store as part of US3.
- **US2 (P2)**: Depends on Foundational and integrates with US1 — inviting requires an existing
  tenant and member. T043/T044/T047 extend files first created in US1/US3.

### Within Each User Story

- Tests are written first and must fail before implementation.
- Stores/models before services; services before endpoints; endpoints before route
  registration; backend before the frontend pages that consume it.

### Parallel Opportunities

- Setup: T002, T003, T004 in parallel.
- Foundational: T010, T011, T012 in parallel (after T005–T009).
- US1 tests: T014, T015, T016, T017 in parallel.
- US1 implementation: T018, T019, T023 in parallel; the frontend pages T028–T031 in parallel.
- US3: T033 and T034 in parallel; T038 parallel with backend work.
- US2: T039 and T040 in parallel.
- Once Foundational is done, US1 and US3 can be staffed by different developers in parallel.

---

## Parallel Example: User Story 1

```bash
# Tests for User Story 1 together:
Task: "Unit tests for bcrypt hash/verify in internal/auth/password_test.go"
Task: "Integration tests for the platform_sessions store in internal/auth/sessions_test.go"
Task: "Tests for the tenants store and slug validation in internal/tenant/tenants_test.go"
Task: "Integration tests for signup/login/logout/me + tenant endpoints in internal/api/handlers_test.go"

# Independent implementation files together:
Task: "Implement bcrypt password hash/verify in internal/auth/password.go"
Task: "Implement the platform_users store in internal/auth/users.go"
Task: "Implement the tenants store in internal/tenant/tenants.go"
```

---

## Implementation Strategy

### MVP First (User Story 1 only)

1. Complete Phase 1: Setup.
2. Complete Phase 2: Foundational (CRITICAL — blocks all stories).
3. Complete Phase 3: User Story 1.
4. **STOP and VALIDATE**: a user can sign up, log in, and create a tenant.
5. Deploy/demo if ready.

### Incremental Delivery

1. Setup + Foundational → foundation ready.
2. Add US1 → test independently → demo (MVP: signup + tenant creation).
3. Add US3 → run the isolation suite → demo (isolation guarantee proven).
4. Add US2 → test independently → demo (team invites).
5. Polish → full verification bundle green.

### Phase 1 exit criteria (from spec.md)

A user can sign up, create a tenant, and invite a teammate; the cross-tenant isolation tests
pass. All three user stories must be complete and the Phase 6 verification bundle green.

---

## Notes

- `[P]` = different files, no dependency on an incomplete task.
- `[Story]` labels map tasks to spec.md user stories for traceability.
- Critical-path tests (isolation, auth, middleware) run against real PostgreSQL — never mocked
  (Constitution Principle II).
- Commit after each task or logical group.
- Stop at any checkpoint to validate a story independently.
