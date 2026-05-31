# Tasks: Email Verification on Registration

**Input**: Design documents from `specs/016-email-verification-registration/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/http-api.md

**Tests**: Test tasks ARE included. Constitution Principle II (Test-Backed Delivery)
is NON-NEGOTIABLE — email sending, the verification flow, and the login gate are
critical paths and must carry automated coverage at the appropriate layer.

**Organization**: Tasks are grouped by user story (US1–US3 from spec.md) so each
story is an independently testable increment.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependency on an incomplete task)
- **[Story]**: US1, US2, US3 — the user story the task serves
- Every task description carries an exact file path

## Path Conventions

Multi-service Go + frontend app. Backend under `internal/`, `cmd/`; frontend
under `frontend/src/`. Paths are repository-root-relative per CLAUDE.md.

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Schema and configuration scaffolding the rest of the feature compiles against.

- [X] T001 [P] Create migration `internal/db/migrations/000022_email_verification.up.sql` (add nullable `email_verified_at` to `platform_users`; backfill existing rows `SET email_verified_at = now()`; `CREATE TABLE email_verification_tokens` with `id`/`user_id`/`token_hash`/`expires_at`/`created_at`/`consumed_at` + the `user_id` index) and `internal/db/migrations/000022_email_verification.down.sql` (drop table, drop column) per data-model.md
- [X] T002 [P] Add config fields to `internal/config/config.go`: `VerificationSenderDomain`, `VerificationSenderName`, `EmailVerificationTTL`, `RegistrationAllowedDomains` (comma-split), `VerificationResendLimit`, `VerificationResendWindow` — including env reads in `Load`, `applyDefaults` (sender name `nvelope`, TTL `24h`, resend limit `5`, window `1h`), and `Validate` (`VerificationSenderDomain` required, durations/limit positive) per quickstart.md
- [X] T003 [P] Add cases to `internal/config/config_test.go` covering the new defaults and the required `VerificationSenderDomain` validation error

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Domain scaffolding and signature changes every user story depends on.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete — the
`HydrateUser` signature change here breaks compilation until T007 is done.

- [X] T004 [P] Add `Domain() string` accessor (portion after `@`, lower-cased) to the `Email` value object in `internal/auth/domain/credentials.go`
- [X] T005 [P] Add the three typed errors to `internal/auth/domain/errors.go`: `ErrEmailDomainNotAllowed` (`apperr.NewIncorrectInput`, slug `email_domain_not_allowed`), `ErrEmailNotVerified` (`apperr.NewForbidden`, slug `email_not_verified`), `ErrVerificationLinkInvalid` (`apperr.NewIncorrectInput`, slug `verification_link_invalid`)
- [X] T006 [P] Add verification state to `User` in `internal/auth/domain/user.go`: private `emailVerifiedAt *time.Time`, accessors `IsEmailVerified()`/`EmailVerifiedAt()`, idempotent `MarkEmailVerified(now time.Time)`, and the new `emailVerifiedAt` parameter on `HydrateUser`
- [X] T007 Update `internal/auth/adapters/users_pg.go` so every `HydrateUser` call passes the new argument and every user-loading query (`GetByID`, `Create`, `CreateWithSession`, `GetCredentials`, `LookupByEmail`) selects `email_verified_at` — restores compilation (depends on T006)
- [X] T008 [P] Unit-test `Email.Domain()` in `internal/auth/domain/credentials_test.go`
- [X] T009 [P] Unit-test `User` verification behavior (`IsEmailVerified`, `MarkEmailVerified` idempotency) in `internal/auth/domain/user_test.go`

**Checkpoint**: Domain compiles with verification state; user stories can begin.

---

## Phase 3: User Story 1 - Verify email before account is usable (Priority: P1) 🎯 MVP

**Goal**: A new account is created unverified with no session, is emailed a
single-use verification link from the service domain via Postbox, and can sign in
only after the link is opened.

**Independent Test**: Register a new account → confirm `201` with no session
cookie → confirm a verification email is enqueued/sent → confirm login is refused
with `403 email_not_verified` → open the link → confirm login then succeeds.

### Tests for User Story 1

- [X] T010 [P] [US1] Unit-test the `EmailVerification` entity (constructor validation, `IsExpired`, `IsConsumed`) in `internal/auth/domain/email_verification_test.go`
- [X] T011 [P] [US1] Unit-test the `VerifyEmail` command (valid → verified, consumed → already-verified, expired → invalid, unknown → invalid) with fakes in `internal/auth/app/command/verify_email_test.go`
- [X] T012 [P] [US1] Integration-test the `EmailVerificationRepository` (`Issue`, `GetByTokenHash`, `Consume`) against the real Postgres container in `internal/auth/adapters/email_verifications_pg_test.go`
- [X] T013 [P] [US1] Component-test the `VerificationWorker` with a fake mailer + real user repository in `internal/auth/adapters/verification_worker_test.go`
- [X] T014 [US1] Extend `internal/auth/app/command/signup_test.go`: account created unverified, no session, verification token issued, send job enqueued
- [X] T015 [US1] Extend `internal/auth/app/command/login_test.go`: unverified account returns `ErrEmailNotVerified`; verified account logs in
- [X] T016 [US1] Extend `internal/auth/adapters/users_pg_test.go`: `CreateWithVerification` atomicity and `MarkEmailVerified`

### Implementation for User Story 1

- [X] T017 [P] [US1] Create the `EmailVerification` entity (`NewEmailVerification`, `IsExpired`, `IsConsumed`, `HydrateEmailVerification`) in `internal/auth/domain/email_verification.go`
- [X] T018 [US1] In `internal/auth/domain/repository.go` declare the `EmailVerificationRepository`, `VerificationMailer`, and `VerificationEnqueuer` ports, and extend `UserRepository` with `CreateWithVerification` and `MarkEmailVerified` (depends on T017)
- [X] T019 [US1] Implement the pgx `EmailVerificationRepository` — `Issue` (delete unconsumed rows for the user, then insert), `GetByTokenHash`, `Consume` — in `internal/auth/adapters/email_verifications_pg.go` (depends on T018)
- [X] T020 [US1] Implement `CreateWithVerification` (atomic user + token insert via `pgx.BeginFunc`) and `MarkEmailVerified` in `internal/auth/adapters/users_pg.go` (depends on T018)
- [X] T021 [P] [US1] Add `VerificationSendArgs` (kind `auth.verification_send`) and `EnqueueVerificationSend` to `internal/platform/jobs/jobs.go`
- [X] T022 [US1] Add the `verificationMailer` composition-root bridge over the campaign `Messenger` to `internal/service/bridges.go` (depends on T018)
- [X] T023 [US1] Implement the `VerificationWorker` River worker plus `en`/`ru` subject + text/HTML body builders in `internal/auth/adapters/verification_worker.go` (depends on T017, T018, T021)
- [X] T024 [US1] Implement the `VerifyEmail` command (look up by token hash, branch invalid/expired/consumed/live, consume token + mark user verified in one transaction) in `internal/auth/app/command/verify_email.go` (depends on T017, T018)
- [X] T025 [US1] Modify the `SignUp` command in `internal/auth/app/command/signup.go`: stop issuing a session, use `CreateWithVerification` to create the user + first token, enqueue the send job, return a verification-required result (depends on T018, T020, T021)
- [X] T026 [US1] Modify the `LogIn` command in `internal/auth/app/command/login.go` to return `ErrEmailNotVerified` when `!user.IsEmailVerified()` after the password check (depends on T006, T007)
- [X] T027 [US1] Register `VerifyEmail` in `internal/auth/app/application.go` and construct/decorate the updated `SignUp`/`LogIn` and new `VerifyEmail` handlers in `internal/auth/app/command/services.go` (depends on T024, T025, T026)
- [X] T028 [US1] Update `handleSignup` (drop the session cookie, return the verification payload) and add `handleVerifyEmail` in `internal/api/platform_handlers.go` (depends on T027)
- [X] T029 [US1] Register `POST /api/platform/verify-email` in `internal/api/server.go` (depends on T028)
- [X] T030 [US1] Wire the `EmailVerificationRepository`, `VerificationEnqueuer`, and updated auth handlers into `cmd/api/main.go` (depends on T027)
- [X] T031 [US1] Register the `VerificationWorker` and wire the `verificationMailer` bridge into `cmd/worker/main.go` (depends on T022, T023)
- [X] T032 [P] [US1] Create the `frontend/src/routes/verify-email.tsx` route that reads `?token`, calls the verify endpoint, and renders verified / already-verified / invalid states
- [X] T033 [US1] Update `frontend/src/routes/signup.tsx` so a successful signup shows a "check your inbox" screen instead of navigating into the app
- [X] T034 [US1] Add the `verifyEmail` call (and adjust the `signup` return shape) in the frontend API client and add the `en`/`ru` signup + verify strings to the i18n catalogs

**Checkpoint**: User Story 1 is fully functional — register, verify, then sign in.

---

## Phase 4: User Story 2 - Restrict registration to permitted email domains (Priority: P2)

**Goal**: Registration is refused before any account or email when the email's
domain is not on the configured allowlist; an empty list is unrestricted.

**Independent Test**: With `NVELOPE_REGISTRATION_ALLOWED_DOMAINS=example.com` set,
register `ada@other.com` → `422 email_domain_not_allowed`, no account/email;
register `ada@example.com` → succeeds.

### Tests for User Story 2

- [X] T035 [P] [US2] Unit-test `RegistrationPolicy` (allowlist hit/miss, empty list = unrestricted, case-insensitive, whitespace-tolerant) in `internal/auth/domain/registration_policy_test.go`
- [X] T036 [US2] Extend `internal/auth/app/command/signup_test.go` with a disallowed-domain case asserting `ErrEmailDomainNotAllowed` and that no user row or job is created

### Implementation for User Story 2

- [X] T037 [P] [US2] Create the `RegistrationPolicy` value object (`NewRegistrationPolicy`, `Allows`, `IsRestricted`) in `internal/auth/domain/registration_policy.go`
- [X] T038 [US2] Inject `RegistrationPolicy` into the `SignUp` command and reject `ErrEmailDomainNotAllowed` before any persistence or enqueue in `internal/auth/app/command/signup.go` (depends on T037, T025)
- [X] T039 [US2] Extend `NewSignUpHandler` / handler construction in `internal/auth/app/command/services.go` to accept the `RegistrationPolicy` (depends on T038)
- [X] T040 [US2] Build the `RegistrationPolicy` from `config.RegistrationAllowedDomains` and inject it in `cmd/api/main.go` (depends on T039)
- [X] T041 [US2] Surface the `email_domain_not_allowed` error message on `frontend/src/routes/signup.tsx` and add its `en`/`ru` strings to the i18n catalogs

**Checkpoint**: User Stories 1 and 2 both work; disallowed domains are refused cleanly.

---

## Phase 5: User Story 3 - Request a new verification email (Priority: P3)

**Goal**: An unverified user can request a fresh verification email; the new link
supersedes the prior one and resends are throttled per account. Responses never
reveal whether an address is registered.

**Independent Test**: For an unverified account, call the resend endpoint → `202`,
a fresh link arrives, the old link stops working; exceed the configured limit →
`429 verification_resend_throttled`.

### Tests for User Story 3

- [X] T042 [P] [US3] Unit-test the `ResendEmailVerification` command (unknown email → silent 202, unverified → token issued + job enqueued, already-verified → no email, throttled) with fakes in `internal/auth/app/command/resend_verification_test.go`
- [X] T043 [US3] Extend `internal/auth/adapters/email_verifications_pg_test.go` to assert `Issue` deletes prior unconsumed rows (resend invalidation, FR-012)

### Implementation for User Story 3

- [X] T044 [P] [US3] Declare the `ResendThrottle` port (`Allow(ctx, key) (bool, error)`) in `internal/auth/domain/repository.go`
- [X] T045 [US3] Add the `resendThrottle` composition-root bridge over the Redis sliding-window limiter to `internal/service/bridges.go` (depends on T044)
- [X] T046 [US3] Implement the `ResendEmailVerification` command (look up user by email, throttle check, issue new token, enqueue send; enumeration-safe for unknown/verified accounts) in `internal/auth/app/command/resend_verification.go` (depends on T018, T044)
- [X] T047 [US3] Register `ResendEmailVerification` in `internal/auth/app/application.go` and construct/decorate it in `internal/auth/app/command/services.go` (depends on T046)
- [X] T048 [US3] Add the `verification_resend_throttled` → `429` slug override to `statusForSlug` in `internal/api/errmap.go`
- [X] T049 [US3] Add `handleResendVerification` to `internal/api/platform_handlers.go` (depends on T047)
- [X] T050 [US3] Register `POST /api/platform/verify-email/resend` in `internal/api/server.go` (depends on T049)
- [X] T051 [US3] Wire the `ResendThrottle` bridge and the resend handler into `cmd/api/main.go` (depends on T045, T047)
- [X] T052 [US3] Add a resend action to `frontend/src/routes/verify-email.tsx` and the login screen, and add its `en`/`ru` strings to the i18n catalogs

**Checkpoint**: All three user stories are independently functional.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Verification and hardening across the whole feature.

- [X] T053 [P] Review every `verify-email` and `verify-email/resend` response path for enumeration-resistance (FR-018) — confirm no response distinguishes a registered from an unregistered address
- [X] T054 Run the standard verification bundle: `make test` (full suite incl. testcontainers), linting, and a clean `go run cmd/migrate/main.go up` apply
- [ ] T055 Execute the `specs/016-email-verification-registration/quickstart.md` manual walk-through end to end
- [ ] T056 [P] Verify the frontend signup → check-inbox → verify-email → login flow in a browser, including the invalid-link and resend paths

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — start immediately.
- **Foundational (Phase 2)**: Depends on Setup. **Blocks all user stories** — the `HydrateUser` change (T006/T007) must land first or the package will not compile.
- **User Stories (Phase 3–5)**: All depend on Foundational.
- **Polish (Phase 6)**: Depends on the user stories being delivered.

### User Story Dependencies

- **US1 (P1)**: Depends only on Foundational. MVP.
- **US2 (P2)**: Depends on Foundational; `RegistrationPolicy` itself (T037) is independent, but T038 edits `signup.go` after T025, so US2 wiring lands after US1's signup change.
- **US3 (P3)**: Depends on Foundational **and US1** — it reuses `EmailVerificationRepository.Issue`, `EnqueueVerificationSend`, and the `VerificationWorker` built in US1.

### Within Each User Story

- Tests are written before/alongside implementation and must fail first.
- Domain entities → ports → adapters → commands → app wiring → HTTP → composition root → frontend.
- `signup.go` is touched by both US1 (T025) and US2 (T038) — sequential, not parallel.

### Parallel Opportunities

- Setup: T001, T002, T003 all `[P]`.
- Foundational: T004, T005, T006 `[P]`; then T007; then T008, T009 `[P]`.
- US1 tests T010–T013 `[P]`; impl T017 and T021 `[P]` early; frontend T032 `[P]`.
- US2 T035 and T037 `[P]`.
- US3 T042 and T044 `[P]`.

---

## Parallel Example: User Story 1

```bash
# Tests for User Story 1 (write first, expect failure):
Task: "Unit-test EmailVerification entity in internal/auth/domain/email_verification_test.go"
Task: "Unit-test VerifyEmail command in internal/auth/app/command/verify_email_test.go"
Task: "Integration-test EmailVerificationRepository in internal/auth/adapters/email_verifications_pg_test.go"
Task: "Component-test VerificationWorker in internal/auth/adapters/verification_worker_test.go"

# Independent implementation tasks that can start together:
Task: "Create EmailVerification entity in internal/auth/domain/email_verification.go"
Task: "Add VerificationSendArgs + EnqueueVerificationSend in internal/platform/jobs/jobs.go"
Task: "Create frontend/src/routes/verify-email.tsx"
```

---

## Implementation Strategy

### MVP First (User Story 1 only)

1. Phase 1: Setup.
2. Phase 2: Foundational (CRITICAL — blocks everything).
3. Phase 3: User Story 1.
4. **STOP and VALIDATE** — register, verify, sign in. This is a shippable MVP:
   email verification works even without the allowlist or resend.

### Incremental Delivery

1. Setup + Foundational → foundation ready.
2. US1 → verify-before-use works → demo (MVP).
3. US2 → domain allowlist enforced → demo.
4. US3 → resend + throttling → demo.

### Notes

- `[P]` = different files, no dependency on an incomplete task.
- All new data is control-plane — no tenant-isolation tests required (see plan.md Constitution Check).
- Commit after each task or logical group; do not skip the Phase 6 verification bundle.
