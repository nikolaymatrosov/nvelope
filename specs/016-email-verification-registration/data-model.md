# Phase 1 Data Model: Email Verification on Registration

**Feature**: 016-email-verification-registration
**Date**: 2026-05-22

All data here is **control-plane** (platform-wide identity), not tenant-scoped —
it sits alongside `platform_users` and `sessions`. No Row-Level Security applies,
consistent with those tables.

## Domain entities & value objects (`internal/auth/domain`)

### User (modified — `user.go`)

The existing `User` identity gains verification state.

| Field | Type | Notes |
|-------|------|-------|
| `emailVerifiedAt` | `*time.Time` | `nil` = unverified; set = the instant verification completed |

New behavior:

- `IsEmailVerified() bool` — `emailVerifiedAt != nil`.
- `EmailVerifiedAt() *time.Time` — accessor.
- `MarkEmailVerified(now time.Time)` — sets `emailVerifiedAt`; no-op if already
  set (idempotent, FR-010).
- `HydrateUser` signature gains an `emailVerifiedAt` parameter (persistence-only
  reconstruction, unchanged contract otherwise).

`NewUser` is unchanged: a freshly constructed user is always unverified.

### EmailVerification (new — `email_verification.go`)

A pending, time-bounded, single-use proof-of-ownership challenge bound to one
user. Always-valid construction; private fields.

| Field | Type | Notes |
|-------|------|-------|
| `id` | `string` | DB-assigned; empty until persisted |
| `userID` | `string` | owning `platform_users.id` |
| `tokenHash` | `string` | SHA-256 of the raw token; the raw token is never stored |
| `expiresAt` | `time.Time` | `createdAt + EmailVerificationTTL` |
| `createdAt` | `time.Time` | issue time |
| `consumedAt` | `*time.Time` | `nil` until the link is used |

Construction & behavior:

- `NewEmailVerification(userID string, ttl time.Duration) (*EmailVerification, error)`
  — validates `userID` non-empty and `ttl > 0`, sets `createdAt = now`,
  `expiresAt = now + ttl`. The raw token + hash are produced by the command via
  the `internal/token` package and passed to the repository (mirrors how
  `Session` + `token.New()`/`token.Hash()` cooperate).
- `IsExpired(now time.Time) bool` — `now.After(expiresAt)`.
- `IsConsumed() bool` — `consumedAt != nil`.
- `HydrateEmailVerification(...)` — persistence-only reconstruction; not a
  constructor, does not re-validate.

### RegistrationPolicy (new — `registration_policy.go`)

A value object encapsulating the configured email-domain allowlist.

- `NewRegistrationPolicy(domains []string) RegistrationPolicy` — normalizes each
  entry (trim whitespace, lower-case, drop empties); an empty result means
  unrestricted.
- `Allows(email Email) bool` — `true` when the list is empty, or when
  `email.Domain()` (lower-cased) is in the list.
- `IsRestricted() bool` — whether any domains are configured (for logging).

### Email (modified — `credentials.go`)

- New `Domain() string` — the portion after `@`, lower-cased. Used by
  `RegistrationPolicy`.

### Typed errors (modified — `errors.go`)

| Error | apperr category | Slug | HTTP |
|-------|-----------------|------|------|
| `ErrEmailDomainNotAllowed` | IncorrectInput | `email_domain_not_allowed` | 422 |
| `ErrEmailNotVerified` | Forbidden | `email_not_verified` | 403 |
| `ErrVerificationLinkInvalid` | IncorrectInput | `verification_link_invalid` | 422 |

`ErrEmailNotVerified` is the only new error the frontend branches on (to show the
resend action). `ErrVerificationLinkInvalid` is deliberately generic — it covers
both "no such token" and "expired", so verification responses cannot be used to
probe for accounts (FR-018).

## Ports (consumer-owned interfaces, `internal/auth/domain` or `app`)

- `EmailVerificationRepository`
  - `Issue(ctx, v *EmailVerification, tokenHash string) (*EmailVerification, error)`
    — deletes any unconsumed rows for `v.userID`, then inserts the new one
    (enforces FR-012 in one statement pair / transaction).
  - `GetByTokenHash(ctx, tokenHash string) (*EmailVerification, error)` — returns
    `ErrVerificationLinkInvalid` when absent.
  - `Consume(ctx, verificationID string, now time.Time) error` — sets
    `consumed_at`; intended to run inside the verify transaction.
- `UserRepository` (extended)
  - `CreateWithVerification(ctx, u *User, passwordHash string, issueVerification func(userID string) (*EmailVerification, tokenHash string, err error)) (*User, error)`
    — atomic user-insert + token-insert (see research R9).
  - `MarkEmailVerified(ctx, userID string, now time.Time) error`.
  - `GetCredentials` — unchanged signature; its query additionally selects
    `email_verified_at` so the returned `*User` carries verification state.
- `VerificationMailer` — `Send(ctx, VerificationEmail) error` (bridged to the
  campaign messenger / Postbox at the composition root).
- `VerificationEnqueuer` — `EnqueueVerificationSend(ctx, userID, rawToken string) error`.
- `ResendThrottle` — `Allow(ctx, key string) (bool, error)` (bridged to Redis).

`VerificationEmail` carries `To`, `ToName`, `Locale`, `VerifyURL`, `FromName`,
`FromAddress` — the rendered-message struct handed to the mailer (analogous to
`audiencedomain.ConfirmationEmail`).

## Background job (`internal/platform/jobs/jobs.go`)

### VerificationSendArgs (new)

| Field | JSON | Notes |
|-------|------|-------|
| `UserID` | `user_id` | the account to email |
| `VerificationToken` | `verification_token` | the **raw** token, needed to build the link; held only in the transient River job row, never persisted long-term (same treatment as `OptinSendArgs.ConfirmationToken`) |

- `Kind() string` → `"auth.verification_send"`.
- New enqueuer method `EnqueueVerificationSend(ctx, userID, rawToken string)`.
  Runs on the existing sending queue (`WorkerSendQueue`).

## Relational schema — migration `000022_email_verification`

### `up`

```sql
-- Verification state on the platform identity.
ALTER TABLE platform_users
    ADD COLUMN email_verified_at timestamptz;

-- Accounts that existed before this feature are out of scope: treat them as
-- already verified so the new login gate does not lock them out.
UPDATE platform_users
    SET email_verified_at = now()
    WHERE email_verified_at IS NULL;

-- Single-use, time-bounded email-ownership challenges. Control-plane (no RLS),
-- like platform_users and sessions. The raw token is never stored — only its
-- hash. A consumed row is kept (not deleted) so the verify path can tell
-- "already verified" apart from "invalid/expired".
CREATE TABLE email_verification_tokens (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     uuid NOT NULL REFERENCES platform_users (id) ON DELETE CASCADE,
    token_hash  text NOT NULL UNIQUE,
    expires_at  timestamptz NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    consumed_at timestamptz
);

CREATE INDEX email_verification_tokens_user_id_idx
    ON email_verification_tokens (user_id);
```

### `down`

```sql
DROP TABLE email_verification_tokens;
ALTER TABLE platform_users DROP COLUMN email_verified_at;
```

## State transitions

### Account verification state

```
unverified (email_verified_at IS NULL)
        │  valid verification link opened
        ▼
verified (email_verified_at = <instant>)   ── terminal; further link opens are no-ops
```

### Verification token lifecycle

```
issued (consumed_at IS NULL, now < expires_at)
   │                     │                         │
   │ link opened in time │ now >= expires_at        │ resend issued
   ▼                     ▼                         ▼
consumed              expired (still             deleted (superseded by
(consumed_at set)     consumed_at IS NULL)        the newer unconsumed row)
```

- Opening a **consumed** token → idempotent success ("already verified").
- Opening an **expired** or **deleted/unknown** token → `ErrVerificationLinkInvalid`
  (frontend offers resend).

## Validation rules (traceability to spec)

| Rule | Source |
|------|--------|
| Email domain must be on the allowlist when one is configured | FR-014, FR-015, FR-017 |
| Empty allowlist permits any valid email | FR-016 |
| New account is created unverified, no session issued | FR-002 |
| Verification link valid only within `EmailVerificationTTL` | FR-006 |
| Issuing a new link deletes prior unconsumed links for the user | FR-012 |
| Sign-in refused while `email_verified_at IS NULL` | FR-008 |
| `MarkEmailVerified` / opening a consumed link is idempotent | FR-010 |
| Verification / resend responses never reveal account existence | FR-018 |
| Allowlist evaluated only at registration time | FR-019 |
| `email_verified_at` records the verification instant | FR-020 |
