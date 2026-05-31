# Quickstart: Email Verification on Registration

**Feature**: 016-email-verification-registration

## What this feature changes

A new platform account is no longer logged in immediately at signup. Instead it
is created in an **unverified** state and emailed a single-use verification link
from the configured service domain (`nvelope.ru`) via Postbox. Sign-in is
refused until the link is opened. Registration is additionally constrained to a
configured allowlist of email domains.

## New / changed configuration

Set these `NVELOPE_`-prefixed environment variables (see `internal/config`):

| Variable | Required | Default | Purpose |
|----------|----------|---------|---------|
| `NVELOPE_VERIFICATION_SENDER_DOMAIN` | yes | — | Service domain verification email is sent from, e.g. `nvelope.ru` |
| `NVELOPE_VERIFICATION_SENDER_NAME` | no | `nvelope` | Display name on the From header |
| `NVELOPE_EMAIL_VERIFICATION_TTL` | no | `24h` | How long a verification link stays valid |
| `NVELOPE_REGISTRATION_ALLOWED_DOMAINS` | no | empty (unrestricted) | Comma-separated email-domain allowlist |
| `NVELOPE_VERIFICATION_RESEND_LIMIT` | no | `5` | Resend requests allowed per window per account |
| `NVELOPE_VERIFICATION_RESEND_WINDOW` | no | `1h` | Resend throttle window |

`NVELOPE_PUBLIC_BASE_URL` (already exists) is the origin verification links are
built from. The service domain must already be a verified Postbox sending
identity — that operational setup is out of scope for this feature.

## Apply the migration

```sh
go run cmd/migrate/main.go up
```

Migration `000022_email_verification` adds `platform_users.email_verified_at`,
creates `email_verification_tokens`, and backfills existing accounts as verified
so they are not locked out.

## Manual walk-through

1. **Restricted registration** — with `NVELOPE_REGISTRATION_ALLOWED_DOMAINS=example.com`
   set, `POST /api/platform/signup` with `ada@other.com` → `422
   email_domain_not_allowed`, no account created. With `ada@example.com` → `201`,
   body `{ "verification": { "required": true, ... } }`, no session cookie.
2. **Verification email** — the `cmd/worker` process sends the email within ~1
   minute; the link is `<PublicBaseURL>/verify-email?token=...`.
3. **Login blocked** — `POST /api/platform/login` with the correct password
   before verifying → `403 email_not_verified`.
4. **Verify** — opening the link drives the frontend `/verify-email` page, which
   calls `POST /api/platform/verify-email` → `200 { "status": "verified" }`.
   Re-opening the same link → `200 { "status": "already_verified" }`.
5. **Login succeeds** — `POST /api/platform/login` now returns `200` + session
   cookie.
6. **Resend** — `POST /api/platform/verify-email/resend` with the account email
   → `202`; the previous link stops working and a fresh one arrives. Exceeding
   `NVELOPE_VERIFICATION_RESEND_LIMIT` → `429 verification_resend_throttled`.

## Tests to run

```sh
make test          # full suite, incl. testcontainers Postgres
```

Coverage added by this feature:

- **Unit** (`internal/auth/domain`) — `EmailVerification` expiry/consumption,
  `RegistrationPolicy.Allows` (allowlist, empty list, case-insensitivity),
  `Email.Domain`.
- **Command unit** (`internal/auth/app/command`) — `SignUp` (allowlist reject,
  unverified creation, job enqueued), `LogIn` (unverified refusal), `VerifyEmail`
  (valid / consumed / expired / unknown), `ResendEmailVerification` (unknown vs
  unverified vs verified, throttle) — with fakes.
- **Integration** (`internal/auth/adapters`) — `email_verifications_pg` and the
  extended `users_pg` against the real Postgres container.
- **Component** (`internal/auth/adapters`) — `VerificationWorker` with a fake
  mailer and a real user repository.

## Where the code lives

| Concern | Path |
|---------|------|
| Domain entities, policy, errors | `internal/auth/domain/` |
| Commands (signup, login, verify, resend) | `internal/auth/app/command/` |
| Repository + worker adapters | `internal/auth/adapters/` |
| Job args + enqueuer | `internal/platform/jobs/jobs.go` |
| Mailer / throttle bridges | `internal/service/bridges.go` |
| HTTP handlers + routes + error map | `internal/api/` |
| Config | `internal/config/config.go` |
| Migration | `internal/db/migrations/000022_email_verification.*.sql` |
| Composition roots | `cmd/api/main.go`, `cmd/worker/main.go` |
| Frontend signup + verify pages | `frontend/src/routes/` |
