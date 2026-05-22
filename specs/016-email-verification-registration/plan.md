# Implementation Plan: Email Verification on Registration

**Branch**: `016-email-verification-registration` | **Date**: 2026-05-22 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/016-email-verification-registration/spec.md`

## Summary

New platform accounts are created in an **unverified** state and emailed a
single-use, time-bounded verification link from the configured service domain
(`nvelope.ru`) via Postbox. Sign-in is refused until the link is opened.
Registration is additionally constrained to a configured **allowlist** of email
domains (empty list = unrestricted). An unverified user can request a fresh
verification email, subject to per-account throttling.

Technical approach: extend the existing `auth` bounded context. A new
`EmailVerification` domain entity and `email_verification_tokens` table model the
challenge; a `RegistrationPolicy` value object built from config enforces the
allowlist; the verification email is delivered through a new durable River job
(`auth.verification_send`) whose worker sends via the existing campaign
`Messenger`/Postbox path through a composition-root bridge. The change reuses the
double-opt-in pattern (`optin.send`) end to end, so no new infrastructure is
introduced.

## Technical Context

**Language/Version**: Go 1.25 (existing module `github.com/nikolaymatrosov/nvelope`); TypeScript / React for the frontend signup + verify pages

**Primary Dependencies**: chi v5 (HTTP routing), pgx v5 (PostgreSQL), River + riverpgxv5 (durable job queue), koanf (config), existing `internal/platform/postbox` SES-compatible client, existing `internal/token` (token generate/hash), Redis sliding-window limiter (resend throttle); TanStack Start/Router on the frontend

**Storage**: PostgreSQL 17 — one new control-plane table `email_verification_tokens`, one new column `platform_users.email_verified_at` (migration `000022`)

**Testing**: `go test ./...` / `make test`; integration tests use `postgres:17` via testcontainers-go (`nvelope-test-pg`)

**Target Platform**: Linux server — four stateless Go services (`cmd/api`, `cmd/worker`, `cmd/scheduler`, `cmd/consumer`); the verification worker runs in `cmd/worker`

**Project Type**: Multi-service web application (Go API + workers, TanStack Start frontend) — modifies the existing `auth` bounded context

**Performance Goals**: Verification email dispatched within 1 minute of signup (SC-002); signup HTTP latency unaffected by mail delivery (job is async)

**Constraints**: Verification/resend responses MUST NOT leak account existence (FR-018); raw tokens never persisted (hash at rest); the allowlist check MUST run before any row write or job enqueue (FR-015)

**Scale/Scope**: Low-volume control-plane transactional mail (one email per registration / resend); ~25 changed or new files across backend and frontend; pre-existing accounts backfilled as verified and otherwise out of scope

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

Evaluated against `.specify/memory/constitution.md` v1.1.0.

- **I. Tenant Isolation by Default** — PASS. All new data (`platform_users.email_verified_at`,
  `email_verification_tokens`) is **control-plane**, not tenant-scoped, exactly
  like `platform_users` and `sessions`. No RLS applies, consistent with those
  tables; there is no cross-tenant data path to test. The control-plane vs
  tenant-scoped separation is preserved.
- **II. Test-Backed Delivery (NON-NEGOTIABLE)** — PASS. Email sending is a
  critical path: the plan adds integration coverage for the new repository and
  the extended user repository against a real Postgres container, plus a
  component test for the `VerificationWorker`. Domain and command logic get unit
  tests. Migration `000022` is a clean reversible apply.
- **III. Incremental, Shippable Phases** — PASS. Delivered as one shippable
  increment; the spec's three prioritized user stories (P1 verify-before-use, P2
  allowlist, P3 resend) map to the implementation phases below. No speculative
  scope — backend email i18n stays minimal (string builders, R8), no template
  engine.
- **IV. Security & Consent by Design** — PASS. The feature *is* a security
  control. Tokens are hashed at rest like session tokens; verify/resend responses
  are enumeration-resistant (R2, R5); resend is throttled (R7); the service
  sender domain is reached through the existing authenticated Postbox path.
- **V. Operable & Observable Services** — PASS. The worker is stateless and the
  job durable/retryable (River). Command handlers are wrapped in the standard
  `decorator` handlers, so logging/metrics/tracing apply uniformly with no
  bespoke boilerplate.
- **VI. Layered Architecture & Domain Integrity** — PASS. Business rules live on
  domain types (`EmailVerification`, `RegistrationPolicy`, `User`) with
  validating constructors and a separate hydration path. New ports
  (`EmailVerificationRepository`, `VerificationMailer`, `VerificationEnqueuer`,
  `ResendThrottle`) are declared by the consumer (auth) and implemented by
  adapters / composition-root bridges. Errors are typed `apperr` values mapped to
  HTTP once in `errmap.go`. Wiring stays in the `cmd/*` composition roots.

**Result**: PASS — no violations. Complexity Tracking is intentionally empty.

*Post-design re-check (after Phase 1)*: The data model and contracts introduce no
new layer crossings, no tenant-scoped data, and no new external-service code
(the mailer reuses `Messenger`/Postbox via a bridge). Constitution Check still
PASSES; no entries required in Complexity Tracking.

## Project Structure

### Documentation (this feature)

```text
specs/016-email-verification-registration/
├── plan.md              # This file
├── spec.md              # Feature specification
├── research.md          # Phase 0 output — decisions R1–R9
├── data-model.md        # Phase 1 output — entities, ports, schema
├── quickstart.md        # Phase 1 output — config + manual walk-through
├── contracts/
│   └── http-api.md      # Phase 1 output — endpoint + job contracts
├── checklists/
│   └── requirements.md  # Spec quality checklist (from /speckit-specify)
└── tasks.md             # Phase 2 output (/speckit-tasks — NOT created here)
```

### Source Code (repository root)

The feature modifies the existing `auth` bounded context, which already uses the
full `domain` / `app` / `adapters` split. New files marked `(+)`, modified `(~)`.

```text
internal/auth/
├── domain/
│   ├── user.go                      (~) verification state + behavior
│   ├── credentials.go               (~) Email.Domain() accessor
│   ├── email_verification.go        (+) EmailVerification entity
│   ├── registration_policy.go       (+) RegistrationPolicy value object
│   ├── errors.go                    (~) 3 new typed errors
│   └── repository.go                (~) new ports; UserRepository extended
├── app/
│   ├── application.go               (~) register VerifyEmail / ResendVerification
│   └── command/
│       ├── services.go              (~) construct + decorate new handlers
│       ├── signup.go                (~) allowlist check, no session, issue token
│       ├── login.go                 (~) refuse unverified accounts
│       ├── verify_email.go          (+) VerifyEmail command
│       └── resend_verification.go   (+) ResendEmailVerification command
└── adapters/
    ├── users_pg.go                  (~) CreateWithVerification, MarkEmailVerified, GetCredentials
    ├── email_verifications_pg.go    (+) EmailVerificationRepository impl
    └── verification_worker.go       (+) River VerificationWorker + email templates

internal/platform/jobs/jobs.go       (~) VerificationSendArgs + EnqueueVerificationSend
internal/config/config.go            (~) verification + allowlist config, defaults, validation
internal/service/bridges.go          (~) VerificationMailer + ResendThrottle bridges
internal/api/
├── server.go                        (~) routes: verify-email, verify-email/resend
├── platform_handlers.go             (~) signup change + verify/resend handlers
└── errmap.go                        (~) verification_resend_throttled → 429 slug override
internal/db/migrations/
├── 000022_email_verification.up.sql    (+)
└── 000022_email_verification.down.sql  (+)
cmd/api/main.go                      (~) wire new command handlers + enqueuer
cmd/worker/main.go                   (~) register VerificationWorker

frontend/src/
├── routes/signup.tsx                (~) on success → "check your inbox" screen
├── routes/verify-email.tsx          (+) consumes ?token, calls verify endpoint
└── (api client + i18n catalogs)     (~) signup/verify/resend calls + en/ru strings
```

**Structure Decision**: Extend the existing `auth` bounded context in place
rather than create a new context. The constitution's "layer scope is proportional
to need" rule and the fact that `auth` already carries the `domain`/`app`/
`adapters` split make this the lowest-ceremony correct option — verification is
an identity concern that belongs with the user aggregate. The verification email
worker lives under `auth/adapters` (a River worker is an adapter, mirroring
`audience/adapters/optin_worker.go`).

## Implementation Phases

These phases are an ordering guide; `/speckit-tasks` produces the detailed task
list. Phases map to the spec's prioritized user stories.

**Phase A — Foundations (schema + config + domain)**: migration `000022`; config
fields (R3, R4, R7) with defaults + validation; `EmailVerification`,
`RegistrationPolicy`, `Email.Domain()`, `User` verification state, new errors;
pure unit tests. Exit: domain unit tests green.

**Phase B — User Story 1, verify-before-use (P1)**: `EmailVerificationRepository`
+ extended `UserRepository` adapters and integration tests; `VerificationMailer`
port + bridge; `VerificationSendArgs` job + enqueuer; `VerificationWorker` +
component test; `SignUp` change (no session, issue token, enqueue); `LogIn`
unverified gate; `VerifyEmail` command; HTTP `signup`/`login`/`verify-email`
wiring; frontend "check your inbox" + `/verify-email` page. Exit: a registrant
can verify and then sign in (SC-001, SC-003, SC-006).

**Phase C — User Story 2, domain allowlist (P2)**: inject `RegistrationPolicy`
into `SignUp`; reject `ErrEmailDomainNotAllowed` before any write; command tests.
(Mostly delivered in Phase A's domain work — this phase wires and tests it.)
Exit: disallowed domains are refused with no account/email (SC-004).

**Phase D — User Story 3, resend (P3)**: `ResendEmailVerification` command;
`ResendThrottle` port + Redis bridge; `verify-email/resend` endpoint + `errmap`
override; frontend resend action on the verify and login screens. Exit: resend
issues a fresh link, supersedes the old one, and throttles abuse (FR-011–FR-013).

Each phase exits with the standard verification bundle: full test suite, lint,
and a clean migration apply (constitution Development Workflow & Quality Gates).

## Complexity Tracking

No constitution violations — this section is intentionally empty.
