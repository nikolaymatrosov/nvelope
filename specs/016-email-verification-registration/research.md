# Phase 0 Research: Email Verification on Registration

**Feature**: 016-email-verification-registration
**Date**: 2026-05-22

This document resolves the open technical questions for the feature against the
existing nvelope codebase. Each entry follows Decision / Rationale / Alternatives.

## R1 — Delivery mechanism: async River job

**Decision**: Send the verification email through a new durable River job,
`auth.verification_send`, consumed by a new worker in the `cmd/worker` process.
The signup/resend command enqueues the job after the account + token rows are
committed.

**Rationale**: An exact precedent exists — double-opt-in confirmation emails are
sent via the `optin.send` River job (`internal/platform/jobs/jobs.go:116`,
`internal/audience/adapters/optin_worker.go`). River jobs are durable and
retried with backoff, so a transient Postbox outage does not lose the email and
a restarted worker re-sends rather than dropping it. This directly satisfies the
spec edge case "verification email fails to send → the account remains
unverified and the resend path recovers it" and SC-002 (dispatched within 1
minute). It also keeps the signup HTTP request fast.

**Alternatives considered**: Synchronous send inside the signup handler — simpler
wiring but couples registration latency to Postbox availability and provides no
retry; rejected. A bespoke outbox table — duplicates what River already gives us;
rejected (YAGNI, constitution III).

## R2 — Token storage and idempotency

**Decision**: A dedicated control-plane table `email_verification_tokens`
(`id`, `user_id`, `token_hash`, `expires_at`, `created_at`, `consumed_at`). The
raw token is generated with the existing `internal/token` package (same as
session tokens) and stored only as a SHA-256 hash. The verify path looks the
token up by hash and branches on its state:

- not found → `ErrVerificationLinkInvalid`
- found, `expired` and not consumed → `ErrVerificationLinkInvalid` (generic; the
  frontend offers resend)
- found, `consumed_at` set → idempotent success ("already verified")
- found, live → set `consumed_at`, set `platform_users.email_verified_at`

**Rationale**: Keeping a consumed row (rather than deleting it) is what lets the
system tell "already verified" apart from "invalid/expired" without leaking
account existence — this is exactly what FR-010 and SC-006 require. Hashing at
rest mirrors `sessions` and `pending_subscriptions.confirmation_token_hash`
(Principle IV). A separate table (not a column on `platform_users`) makes
resend-invalidation a plain `DELETE` of unconsumed rows for the user (FR-012).

**Alternatives considered**: Storing a single token hash + expiry directly on
`platform_users` — cannot represent superseded links cleanly and conflates a
challenge with the identity; rejected. Deleting the row on consume — loses the
"already verified" signal; rejected.

## R3 — Service sender domain and the transactional mailer

**Decision**: Add required config `VerificationSenderDomain` (e.g. `nvelope.ru`)
and optional `VerificationSenderName`. The verification email's From address is
composed as `no-reply@<VerificationSenderDomain>`. The auth context declares a
consumer-owned port `auth/domain.VerificationMailer`; a composition-root bridge
in `internal/service/bridges.go` implements it over the campaign context's
`Messenger` (the `PostboxMessenger` that builds RFC-822 MIME and calls
`postbox.Client.SendEmail`).

**Rationale**: Registration verification is a *control-plane* email sent before
any tenant exists, so it cannot use a tenant's verified sending domain the way
`optin.send` does — it must use a fixed service domain from config, exactly as
the user requested. The existing `confirmationMailer` bridge
(`internal/service/bridges.go:210`) is the precedent: the audience context
depends on an interface it owns, and the bridge adapts the campaign messenger.
Reusing `Messenger`/`PostboxMessenger` means no new Postbox client code and no
duplicated MIME assembly (Architectural Constraint: external-service abstraction;
"shared infrastructure lives once").

Making `VerificationSenderDomain` a **required** config value validated at
`config.Load` makes the spec edge case "service sender domain missing or
misconfigured" a fast startup failure rather than silently-unverifiable accounts.

**Alternatives considered**: A brand-new transactional-mail package talking to
Postbox directly — duplicates `PostboxMessenger`; rejected. Reusing a tenant
sending domain — impossible at registration time (no tenant); rejected.

## R4 — Registration domain allowlist

**Decision**: Add config `RegistrationAllowedDomains` (comma-separated list).
Model the rule as a `RegistrationPolicy` value object in `auth/domain`,
constructed once at the composition root from the config list and injected into
the `SignUp` handler. `RegistrationPolicy.Allows(Email) bool` lower-cases and
trims both sides; an **empty** list means unrestricted (FR-016). Add an
`Email.Domain()` accessor to the existing `Email` value object. A disallowed
domain fails with `ErrEmailDomainNotAllowed` *before* any row is written or job
enqueued (FR-015).

**Rationale**: The check is a business rule, so it belongs in the domain, not in
the HTTP handler (Principle VI: "no business `if` statements in `app/`
handlers"). A value object built from config keeps the handler injectable and
unit-testable with no infrastructure. The user confirmed allowlist semantics
(only listed domains may register).

**Alternatives considered**: A blocklist — explicitly rejected by the user.
Reading config inside the handler — couples the domain to config and hurts
testability; rejected.

## R5 — Refusing sign-in for unverified accounts

**Decision**: `LogIn.Handle` checks `user.IsEmailVerified()` *after* the password
is verified; an unverified account returns `ErrEmailNotVerified`
(`apperr.Forbidden` category → HTTP 403, via the existing `errmap.go`). No
session is issued. `SignUp` no longer issues a session at all. The
`GetCredentials` repository read is extended to select `email_verified_at` so
the check needs no extra round-trip.

**Rationale**: The user chose "blocked until verified". Checking *after* the
password verification means the 403 is only ever shown to someone who already
proved they own the account, so it does not weaken the existing
enumeration-resistance of login (FR-018). `apperr.Forbidden` already maps to 403
(`errmap.go:21`), letting the frontend distinguish "verify your email" from "bad
password" and surface the resend action.

**Alternatives considered**: Returning the generic `ErrInvalidCredentials` for
unverified accounts — gives the user no way to know they must verify; rejected.
A 401 — indistinguishable from bad credentials on the client; rejected.

## R6 — Existing accounts and the schema migration

**Decision**: Migration `000022` adds `email_verified_at timestamptz` (nullable)
to `platform_users` and, in the same migration, backfills every existing row
with `email_verified_at = now()`.

**Rationale**: Without the backfill, adding a nullable column would instantly
mark every pre-existing account unverified and lock them all out of login. The
spec assumption is explicit: this feature governs *new* registrations only and
existing accounts are out of scope. Backfilling treats all accounts that existed
before the feature as already verified, honouring that boundary and keeping the
migration a clean, reversible apply (constitution II: "clean schema migration").

**Alternatives considered**: A `NOT NULL DEFAULT now()` column — would also
backfill but then silently mark *future* inserts verified, defeating the feature;
rejected. No backfill — locks out existing users; rejected.

## R7 — Resend throttling

**Decision**: The auth app declares a `ResendThrottle` port
(`Allow(ctx, key) (bool, error)`). The composition root wires it to the existing
Redis sliding-window limiter that already backs the audience context's
`SubmissionThrottle`. The throttle key is the account email. Add config
`VerificationResendLimit` (default 5) and `VerificationResendWindow` (default
1h).

**Rationale**: FR-013 requires per-account throttling and the platform already
runs a Redis sliding-window limiter for an equivalent purpose (opt-in submission
throttling). Reusing it satisfies "shared infrastructure lives once" and avoids a
second rate-limit mechanism.

**Alternatives considered**: A DB-counter throttle — adds write load and a new
table for something Redis already does; rejected. No throttle — fails FR-013 and
invites inbox-flooding abuse; rejected.

## R8 — Email localization

**Decision**: The verification worker renders the email body in the user's
locale (`en`/`ru`) read from `platform_users.locale`, defaulting to English when
unset. `SignUp` accepts an optional `Locale` and persists it on the new account
when the request carries a supported `nv_locale` cookie (the same value the API
already resolves via `resolveAuthLocale`). Email bodies are plain Go string
builders with `en`/`ru` variants, kept in the worker file.

**Rationale**: The platform added per-user locale in spec 015
(`platform_users.locale`, migration `000021`). Localizing where the locale is
known satisfies the spec assumption with minimal scope. Plain string builders
match the existing `optin_worker.go` precedent (`confirmationText`/
`confirmationHTML`); introducing a backend template/i18n engine now would be
speculative (constitution III, YAGNI).

**Alternatives considered**: A full backend email-template + i18n catalog system
— large, speculative scope; rejected for this increment. English-only — ignores
existing locale support; rejected as a needless regression.

## R9 — Atomicity of account creation and token issuance

**Decision**: Extend the `UserRepository` with
`CreateWithVerification(ctx, user, passwordHash, issueVerification)` mirroring
the existing `CreateWithSession`: the user insert and the verification-token
insert run in one `pgx.BeginFunc` transaction. The verification email job is
enqueued by the command *after* the transaction commits (mirroring the
`optin.send` flow).

**Rationale**: The user row and its first token must be consistent — a user with
no token can never verify. `CreateWithSession`
(`internal/auth/adapters/users_pg.go:50`) is the established pattern for
"create user + one related row atomically". Enqueue-after-commit matches the
`optin.send` precedent; the rare crash between commit and enqueue is fully
recovered by the resend path, so a transactional enqueue is not worth crossing
the repository/queue layer boundary.

**Alternatives considered**: Transactional `InsertTx` of the River job inside the
repository closure — forces the river client into the persistence adapter,
violating the layer boundary; rejected. Two separate non-transactional inserts —
can strand a user with no token; rejected.
