---
description: "Task list for Phase 2 — Subscribers, Lists & Auth"
---

# Tasks: Phase 2 — Subscribers, Lists & Auth

**Input**: Design documents from `/specs/004-subscribers-lists-auth/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/

**Tests**: Test tasks ARE included — constitution Principle II (Test-Backed
Delivery) is non-negotiable and makes tests mandatory, not optional. Critical
paths (tenant isolation, asynchronous job processing) get integration coverage
against real boundaries.

**Organization**: Tasks are grouped by user story. Phases are numbered in
delivery order; the story each phase delivers is named in its heading.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[Story]**: US1–US5, mapping to the spec's user stories
- All paths are repository-relative

## Path Conventions

Multi-service Go monorepo. New code lives under `internal/iam/`,
`internal/audience/`, `internal/platform/`, `internal/api/`; migrations under
`internal/db/migrations/`; isolation tests under `test/`.

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Add the new dependencies and tooling Phase 2 needs.

- [X] T001 Add River dependencies (`github.com/riverqueue/river`, `github.com/riverqueue/river/riverdriver/riverpgxv5`) to `go.mod` and run `go mod tidy`
- [X] T002 Add the TOTP dependency (`github.com/pquerna/otp`) to `go.mod` and run `go mod tidy`
- [X] T003 [P] Extend `internal/config/config.go` with `TOTP_ENCRYPTION_KEY` (fail-fast if missing) and River worker settings (queue, per-tenant concurrency)
- [X] T004 [P] Add `internal/iam` and `internal/audience` to the `go-cleanarch` invocation in the CI workflow so the inward-dependency rule is enforced for the new contexts

**Checkpoint**: Dependencies resolve; `go build ./...` succeeds.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Shared building blocks every later phase depends on.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete.

- [X] T005 Create `internal/platform/tenantdb/tenantdb.go` — exported `WithTenant(ctx, pool, tenantID, fn)` RLS-bound transaction helper, moved and exported from `internal/tenant/adapters/rls.go`
- [X] T006 [P] Add `internal/platform/tenantdb/tenantdb_test.go` — integration test proving the transaction binds `app.tenant_id` and fails closed when unset
- [X] T007 Update `internal/tenant/adapters/*.go` to call `tenantdb.WithTenant` and delete `internal/tenant/adapters/rls.go`; confirm the Phase 1 suite still passes
- [X] T008 [P] Add the `Forbidden` category (HTTP 403) to `internal/platform/apperr/apperr.go` with a constructor helper, and cover it in `internal/platform/apperr/apperr_test.go`
- [X] T009 Map `apperr.Forbidden` → HTTP 403 in `internal/api/errmap.go` and assert it in `internal/api/errmap_test.go`

**Checkpoint**: `WithTenant` is shared; the typed-error path supports 403; full existing suite green.

---

## Phase 3: User Story 1 — Manage lists and subscribers (Priority: P1) 🎯 MVP

**Goal**: A tenant operator can create/edit/delete lists and subscribers, attach
subscribers to lists with custom attributes, and change subscription state — all
tenant-isolated.

**Independent Test**: As an authenticated tenant member, create a list, create
subscribers with custom attributes, attach them, edit, change state, delete a
subscriber and a list; confirm records are visible only within the tenant.

**Note on access**: US1 endpoints are gated by the existing Phase 1
authenticated-membership middleware only. Fine-grained permission enforcement is
layered on in US2.

### Schema for User Story 1

- [X] T010 [US1] Create migration `internal/db/migrations/000006_audience_schema.{up,down}.sql` — `lists`, `subscribers` (with `attributes jsonb` + GIN index), `subscriber_lists`, each with `tenant_id`, `ENABLE`/`FORCE` RLS, the `tenant_isolation` policy, and `nvelope_app` grants

### Domain for User Story 1

- [X] T011 [P] [US1] `List` entity with validating constructor + hydration path in `internal/audience/domain/list.go`
- [X] T012 [P] [US1] `Subscriber` entity with state transitions (enabled/disabled/blocklisted) in `internal/audience/domain/subscriber.go`
- [X] T013 [P] [US1] Custom-attributes value object in `internal/audience/domain/attributes.go`
- [X] T014 [P] [US1] `Membership` entity with subscription-status transitions in `internal/audience/domain/membership.go`
- [X] T015 [P] [US1] Typed domain errors in `internal/audience/domain/errors.go`
- [X] T016 [US1] Repository interfaces (`ListRepository`, `SubscriberRepository`, `MembershipRepository`) in `internal/audience/domain/repository.go`
- [X] T017 [P] [US1] Domain unit tests for list, subscriber (state transitions), attributes, and membership in `internal/audience/domain/*_test.go`

### Adapters for User Story 1

- [X] T018 [P] [US1] `ListRepository` pgx implementation in `internal/audience/adapters/lists_pg.go`
- [X] T019 [P] [US1] `SubscriberRepository` pgx implementation (CRUD, `UpsertByEmail`, `Search`) in `internal/audience/adapters/subscribers_pg.go`
- [X] T020 [P] [US1] `MembershipRepository` pgx implementation in `internal/audience/adapters/memberships_pg.go`
- [X] T021 [US1] Repository integration tests against a real DB via `internal/dbtest` in `internal/audience/adapters/*_test.go`

### Application for User Story 1

- [X] T022 [US1] `Application` struct (Commands + Queries) in `internal/audience/app/application.go`
- [X] T023 [P] [US1] List command handlers (`CreateList`, `UpdateList`, `DeleteList`) in `internal/audience/app/command/`
- [X] T024 [P] [US1] Subscriber command handlers (`CreateSubscriber`, `UpdateSubscriber`, `DeleteSubscriber`) in `internal/audience/app/command/`
- [X] T025 [P] [US1] Membership command handlers (`AddToList`, `RemoveFromList`, `ChangeSubscriptionState`) in `internal/audience/app/command/`
- [X] T026 [P] [US1] Query handlers (`ListLists`, `GetList`, `SearchSubscribers`, `GetSubscriber`) in `internal/audience/app/query/`
- [X] T027 [US1] Application handler unit tests with in-memory repository fakes in `internal/audience/app/**/*_test.go`

### Transport & wiring for User Story 1

- [X] T028 [US1] List & subscriber HTTP handlers in `internal/api/audience_handlers.go` per `contracts/http-api.md`
- [X] T029 [US1] Mount the audience routes under `/t/{slug}/api` in `internal/api/server.go` and add the `audience` application to the `Server` struct
- [X] T030 [US1] Wire the `audience` application (adapters + decorated handlers) into the composition root `internal/service/application.go`

### Tests for User Story 1

- [X] T031 [US1] Endpoint/component tests for list & subscriber CRUD (incl. duplicate-email 409) in `internal/api/audience_handlers_test.go`
- [X] T032 [US1] Extend `test/isolation_test.go` to prove cross-tenant denial for `lists`, `subscribers`, `subscriber_lists` as `nvelope_app`

**Checkpoint**: US1 is fully functional and independently testable — MVP ready.

---

## Phase 4: User Story 2 — Role-based access gates (Priority: P1)

**Goal**: A tenant administrator defines roles, assigns them at the tenant level
and per-list, and the system allows/denies actions by effective permissions.

**Independent Test**: Create a limited role, assign it to a second user, confirm
allowed vs. denied actions; grant a per-list role and confirm access widens for
that list only.

**Depends on**: US1 (per-list roles reference `lists`).

### Schema for User Story 2

- [X] T033 [US2] Create migration `internal/db/migrations/000005_tenant_access_schema.{up,down}.sql` — `users`, `sessions`, `roles`, `user_roles`, `user_list_roles`, `api_keys`, `recovery_codes`, `audit_log`, each tenant-plane with RLS and `nvelope_app` grants (the `api_keys`/`recovery_codes` columns are used in US5)

> Migration files apply in numeric order; `000005` is created here but applies before `000006`. The `user_list_roles.list_id` FK to `lists` is added in this migration as deferred and validated, or added in a follow-up `000008` if `lists` is not yet present in the target environment — keep both migrations apply-clean from empty.

### Domain for User Story 2

- [X] T034 [P] [US2] `Permission` value object + the catalogue from `contracts/permissions.md` + pure `EffectivePermissions` union function in `internal/iam/domain/permission.go`
- [X] T035 [P] [US2] `Role` entity (validating constructor, `Rename`, `SetPermissions`) in `internal/iam/domain/role.go`
- [X] T036 [P] [US2] `TenantUser` entity (linked to `platform_user_id`) in `internal/iam/domain/user.go`
- [X] T037 [P] [US2] `Session` entity with state machine (`totp-pending`/`active`/`revoked`) in `internal/iam/domain/session.go`
- [X] T038 [P] [US2] `Principal` value object (`Can`, `CanOnList`) in `internal/iam/domain/principal.go`
- [X] T039 [P] [US2] Typed domain errors in `internal/iam/domain/errors.go`
- [X] T040 [US2] Repository interfaces (`UserRepository`, `SessionRepository`, `RoleRepository`) in `internal/iam/domain/repository.go`
- [X] T041 [P] [US2] Domain unit tests for permissions (union), role, session transitions, principal in `internal/iam/domain/*_test.go`

### Adapters for User Story 2

- [X] T042 [P] [US2] `RoleRepository` pgx implementation incl. `AssignTenantRole`, `AssignListRole`, `RemoveListRole`, `EffectiveFor` in `internal/iam/adapters/roles_pg.go`
- [X] T043 [P] [US2] `UserRepository` pgx implementation in `internal/iam/adapters/users_pg.go`
- [X] T044 [P] [US2] `SessionRepository` pgx implementation in `internal/iam/adapters/sessions_pg.go`
- [X] T045 [US2] Repository integration tests against a real DB in `internal/iam/adapters/*_test.go`

### Application for User Story 2

- [X] T046 [US2] `Application` struct in `internal/iam/app/application.go`
- [X] T047 [P] [US2] Role command handlers (`CreateRole`, `UpdateRole`, `DeleteRole`, `AssignRole`, `AssignListRole`, `RevokeRole`) in `internal/iam/app/command/`
- [X] T048 [P] [US2] Workspace-session command handlers (`OpenWorkspaceSession`, `CloseSession`) in `internal/iam/app/command/`
- [X] T049 [P] [US2] Query handlers (`AuthenticatePrincipal` resolving session → `Principal`, `Authorize`, `ListRoles`) in `internal/iam/app/query/`
- [X] T050 [US2] Application handler unit tests with in-memory fakes in `internal/iam/app/**/*_test.go`

### Membership integration

- [X] T051 [US2] Extend the Phase 1 tenant flows (`internal/tenant/app/command/` create-workspace and accept-invitation) to provision a tenant-plane `users` row and assign a bootstrap **Owner** role on first membership

### Transport & enforcement for User Story 2

- [X] T052 [US2] `authz` middleware in `internal/api/authz_middleware.go` — resolve the tenant-plane session into a `Principal` and attach it to the request context
- [X] T053 [US2] Role-management & workspace-session HTTP handlers in `internal/api/iam_handlers.go` per `contracts/http-api.md`
- [X] T054 [US2] Mount iam routes + the `authz` middleware in `internal/api/server.go`; wire the iam application into `internal/service/application.go`
- [X] T055 [US2] Add permission enforcement (`Principal.Can` / `CanOnList`) to the start of every guarded audience handler/command from US1 and every iam handler, returning `apperr.Forbidden`
- [X] T056 [US2] Write `audit_log` records for role creation/update/delete and role assignment/revocation (via an `AuditRepository`/recorder in `internal/iam/adapters/audit_pg.go`)

### Tests for User Story 2

- [X] T057 [US2] RBAC allow-path and deny-path endpoint tests, incl. tenant-level vs. per-list scoping and permission-change-takes-effect, in `internal/api/iam_handlers_test.go`
- [X] T058 [US2] Extend `test/isolation_test.go` for `users`, `sessions`, `roles`, `user_roles`, `user_list_roles`, `audit_log`

**Checkpoint**: US1 and US2 both work independently; access gates enforced.

---

## Phase 5: User Story 3 — Import and export subscribers (Priority: P2)

**Goal**: A tenant operator imports subscribers from CSV/ZIP (upsert by email)
and exports subscribers (all / by list) to CSV, as non-blocking jobs.

**Independent Test**: Upload a CSV of new + existing emails, confirm
created/updated/skipped counts; export a list and confirm the CSV contents.

**Depends on**: US1 (subscribers/lists). Export-by-segment is added in US4.

### Job infrastructure

- [X] T059 [US3] Create `internal/platform/jobs/jobs.go` — River client construction, a `JobEnqueuer`, and worker-registration helpers with per-tenant fairness config
- [X] T060 [US3] Wire River's migrator into `cmd/migrate/main.go` so `migrate up` also installs the queue tables; confirm clean apply/revert
- [X] T061 [US3] Create migration `internal/db/migrations/000007_import_export_jobs.{up,down}.sql` — `import_export_jobs` (status, counts, `params jsonb`, staged `file_bytes bytea`, `failures jsonb`), tenant-plane with RLS and grants

### Domain for User Story 3

- [X] T062 [P] [US3] `ImportJob` entity (status machine + counts) in `internal/audience/domain/importjob.go`
- [X] T063 [P] [US3] `ExportJob` entity in `internal/audience/domain/exportjob.go`
- [X] T064 [US3] Add `JobRepository` to `internal/audience/domain/repository.go` and a `JobEnqueuer` interface in `internal/audience/app/`
- [X] T065 [P] [US3] Domain unit tests for job state transitions in `internal/audience/domain/*job_test.go`

### Adapters for User Story 3

- [X] T066 [P] [US3] CSV/ZIP codec (decode upload, encode export, reserved-header mapping) in `internal/audience/adapters/csv_codec.go`
- [X] T067 [P] [US3] `JobRepository` pgx implementation incl. staged-file read/write in `internal/audience/adapters/jobs_pg.go`
- [X] T068 [US3] Import River worker (stream staged file, upsert by email, skip invalid rows, record counts) in `internal/audience/adapters/import_worker.go`
- [X] T069 [US3] Export River worker (build CSV for `all`/`by-list`, stage result) in `internal/audience/adapters/export_worker.go`
- [X] T070 [US3] Adapter tests: codec unit tests; job repository + import/export workers integration-tested against a real DB **and a real River queue** in `internal/audience/adapters/*_test.go`

### Application for User Story 3

- [X] T071 [P] [US3] `StartImport` / `StartExport` command handlers (stage file, enqueue job) in `internal/audience/app/command/`
- [X] T072 [P] [US3] `GetJobStatus` query handler in `internal/audience/app/query/`
- [X] T073 [US3] Application handler unit tests in `internal/audience/app/**/*_test.go`

### Transport & wiring for User Story 3

- [X] T074 [US3] Import/export/job HTTP handlers (multipart upload, 202 Accepted, status, download) in `internal/api/audience_handlers.go`
- [X] T075 [US3] Register the import & export River workers in `cmd/worker/main.go` (replacing the idle scaffold) and wire the River client through `internal/service/application.go`

### Tests for User Story 3

- [X] T076 [US3] Endpoint/component tests: import a CSV and a ZIP-wrapped CSV with new+existing+invalid rows; export a list; verify counts and downloaded contents — in `internal/api/audience_handlers_test.go`

**Checkpoint**: US1–US3 work independently; bulk import/export runs on the queue.

---

## Phase 6: User Story 4 — Segment-based subscriber selection (Priority: P2)

**Goal**: A tenant operator defines a query over fields, custom attributes, and
list membership, and gets the matching subscriber set and count.

**Independent Test**: With varied subscribers, run a query combining an
attribute condition and a list condition; confirm the exact matching set and
count; drive an export from it.

**Depends on**: US1 (subscribers); integrates with US3 (export-by-segment).

### Domain for User Story 4

- [ ] T077 [P] [US4] `Segment` value object — condition tree, validation of known fields/operators (rejects malformed queries) — in `internal/audience/domain/segment.go`
- [ ] T078 [P] [US4] `Segment` domain unit tests incl. malformed-query rejection in `internal/audience/domain/segment_test.go`

### Adapters for User Story 4

- [ ] T079 [US4] Add `RunSegment` and `CountSegment` to `internal/audience/adapters/subscribers_pg.go` — translate a validated `Segment` to a parameterized SQL `WHERE` clause (parameterized JSON operators for attributes)
- [ ] T080 [US4] Segment-translation integration tests (attribute, membership, combined conditions, empty result) against a real DB in `internal/audience/adapters/subscribers_pg_test.go`

### Application for User Story 4

- [ ] T081 [P] [US4] `RunSegment` query handler (matching set + count) in `internal/audience/app/query/`
- [ ] T082 [US4] Extend `StartExport` to accept a `segment` selection in `internal/audience/app/command/` and the export worker in `internal/audience/adapters/export_worker.go`
- [ ] T083 [US4] Application handler unit tests for `RunSegment` in `internal/audience/app/query/`

### Transport for User Story 4

- [ ] T084 [US4] Segment query endpoints (`POST /subscribers/query`, `.../query/count`) in `internal/api/audience_handlers.go`

### Tests for User Story 4

- [ ] T085 [US4] Endpoint tests for segment queries and segment-driven export in `internal/api/audience_handlers_test.go`

**Checkpoint**: US1–US4 work independently.

---

## Phase 7: User Story 5 — Scoped API keys and TOTP 2FA (Priority: P3)

**Goal**: Administrators issue scoped, revocable API keys; users enable TOTP
two-factor auth on sign-in with one-time recovery codes.

**Independent Test**: Create a read-only API key, confirm read succeeds and
write returns 403, revoke it and confirm 401; enable TOTP and confirm sign-in
requires a current code.

**Depends on**: US2 (iam context, `Principal`, `authz` middleware).

### Domain for User Story 5

- [ ] T086 [P] [US5] `APIKey` entity (scoped permission subset, revocation) in `internal/iam/domain/apikey.go`
- [ ] T087 [P] [US5] `TOTP` secret value object + `RecoveryCode` entity in `internal/iam/domain/totp.go`
- [ ] T088 [US5] Add `APIKeyRepository`, `RecoveryCodeRepository`, and the `TOTP` capability interface to `internal/iam/domain/repository.go` / `internal/iam/app/`
- [ ] T089 [P] [US5] Domain unit tests for API key scoping/revocation and TOTP/recovery code in `internal/iam/domain/*_test.go`

### Adapters for User Story 5

- [ ] T090 [P] [US5] `APIKeyRepository` pgx implementation in `internal/iam/adapters/apikeys_pg.go`
- [ ] T091 [P] [US5] `RecoveryCodeRepository` pgx implementation in `internal/iam/adapters/recovery_codes_pg.go`
- [ ] T092 [P] [US5] TOTP adapter over `pquerna/otp` with config-keyed secret encryption/decryption in `internal/iam/adapters/totp.go`
- [ ] T093 [US5] Adapter integration tests in `internal/iam/adapters/*_test.go`

### Application for User Story 5

- [ ] T094 [P] [US5] API key command handlers (`IssueAPIKey`, `RevokeAPIKey`) + `ListAPIKeys` query in `internal/iam/app/`
- [ ] T095 [P] [US5] TOTP command handlers (`EnableTOTP`, `ConfirmTOTP`, `DisableTOTP`, `VerifyTOTPChallenge`) in `internal/iam/app/command/`
- [ ] T096 [US5] Extend `OpenWorkspaceSession` to return a `totp-pending` session when the user has TOTP enabled; extend `AuthenticatePrincipal` to also resolve an API-key credential into a `Principal`
- [ ] T097 [US5] Application handler unit tests in `internal/iam/app/**/*_test.go`

### Transport for User Story 5

- [ ] T098 [US5] API key and TOTP/2FA HTTP handlers in `internal/api/iam_handlers.go` per `contracts/http-api.md`
- [ ] T099 [US5] Extend `authz_middleware` to accept `Authorization: Bearer <api-key>` and reject `totp-pending` sessions on guarded routes
- [ ] T100 [US5] Write `audit_log` records for API key issuance and revocation

### Tests for User Story 5

- [ ] T101 [US5] Endpoint tests: scoped API key allowed/denied/revoked; TOTP enable→challenge→activate, wrong-code refusal, recovery-code use, disable — in `internal/api/iam_handlers_test.go`
- [ ] T102 [US5] Extend `test/isolation_test.go` for `api_keys` and `recovery_codes`

**Checkpoint**: All five user stories independently functional.

---

## Phase 8: Polish & Cross-Cutting Concerns

**Purpose**: Phase-exit hardening across all stories.

- [ ] T103 [P] Confirm every new command/query handler is wrapped with the logging decorators in `internal/service/application.go`
- [ ] T104 [P] Add an `AuditTrail` query + `GET /t/{slug}/api/audit` endpoint gated by `audit:get`
- [ ] T105 Run `go-cleanarch` for `internal/iam` and `internal/audience`; fix any inward-dependency violations
- [ ] T106 Run `go test -race ./...` and resolve any data races (River workers, concurrent imports)
- [ ] T107 Verify a clean migration apply from an empty database (`000005`–`000007` + River) and a clean revert
- [ ] T108 Execute the `quickstart.md` smoke walk end-to-end against a running API + worker

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: no dependencies — start immediately
- **Foundational (Phase 2)**: depends on Setup — BLOCKS all user stories
- **US1 (Phase 3)**: depends on Foundational
- **US2 (Phase 4)**: depends on Foundational + US1 (per-list roles reference `lists`)
- **US3 (Phase 5)**: depends on Foundational + US1
- **US4 (Phase 6)**: depends on Foundational + US1; T082 integrates with US3
- **US5 (Phase 7)**: depends on Foundational + US2 (iam context, `Principal`, `authz`)
- **Polish (Phase 8)**: depends on all desired stories being complete

### Story Independence

- US1 is the MVP — fully testable alone (Phase 1 membership gating).
- US2 layers permission enforcement onto US1's endpoints (T055) but is itself testable via its own role/assignment endpoints.
- US3 and US4 are both P2; US4 may be built before or after US3 — the only coupling is T082 (segment export), which simply extends export.
- US5 is independent of US3/US4 and only needs the US2 iam context.

### Within Each User Story

- Schema → Domain → Adapters → Application → Transport/wiring → Tests.
- Domain unit tests precede or accompany domain code; adapter integration tests require the schema task done.
- `[P]` tasks touch different files and may run concurrently.

---

## Parallel Execution Examples

### User Story 1 domain (after T010)

```text
T011  List entity            internal/audience/domain/list.go
T012  Subscriber entity      internal/audience/domain/subscriber.go
T013  Attributes value obj   internal/audience/domain/attributes.go
T014  Membership entity      internal/audience/domain/membership.go
T015  Domain errors          internal/audience/domain/errors.go
```

### User Story 2 domain (after T033)

```text
T034  Permission + catalogue internal/iam/domain/permission.go
T035  Role entity            internal/iam/domain/role.go
T036  TenantUser entity      internal/iam/domain/user.go
T037  Session entity         internal/iam/domain/session.go
T038  Principal value obj    internal/iam/domain/principal.go
```

---

## Implementation Strategy

### MVP First

1. Phase 1 Setup → Phase 2 Foundational → Phase 3 (US1).
2. **STOP and VALIDATE**: a tenant operator can manage lists and subscribers.
3. Deploy/demo the MVP.

### Incremental Delivery

Each phase ends green and shippable, matching the plan's five increments:
US1 (manage) → US2 (RBAC) → US3 (import/export) → US4 (segmentation) →
US5 (API keys & TOTP) → Polish. The phase exits when Phase 8 passes.

### Parallel Team Strategy

After Foundational: US1 must land first; then US3 and US4 can proceed in
parallel with US2; US5 starts once US2's iam context exists.

---

## Notes

- `[P]` = different files, no incomplete dependencies.
- Every tenant-plane table gets RLS in its migration and an isolation-suite case (T032, T058, T102) — constitution Principle I.
- Import/export — a Critical Path — is tested against a real River queue, not mocked (T070) — constitution Principle II.
- Commit after each task or logical group; keep the build and suite green at every checkpoint.
