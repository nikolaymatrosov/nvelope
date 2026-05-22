# HTTP API Contract: Email Verification on Registration

**Feature**: 016-email-verification-registration
**Date**: 2026-05-22

All endpoints are control-plane and mounted under `/api/platform` (chi router,
`internal/api/server.go`). Errors use the standard envelope
`{"error": {"code": "<slug>", "message": "<text>"}}` produced by `Server.fail` /
`writeError`. The status codes below come from the existing `errmap.go` mapping.

---

## `POST /api/platform/signup` — modified

Registers a platform account. **Behaviour change**: no session is issued and no
session cookie is set; the response tells the client to await email
verification.

**Request**

```json
{ "email": "ada@example.com", "password": "correct horse battery", "name": "Ada" }
```

**Responses**

| Status | When | Body |
|--------|------|------|
| 201 Created | account created, verification email enqueued | `{ "verification": { "required": true, "email": "ada@example.com" } }` |
| 422 Unprocessable Entity | invalid email/password shape | error `validation_failed` |
| 422 Unprocessable Entity | email domain not on the allowlist | error `email_domain_not_allowed` |
| 409 Conflict | email already registered | error `email_taken` |
| 400 Bad Request | body is not valid JSON | error `invalid_body` |

The domain-allowlist check runs before any row is written or job enqueued
(FR-015): a 422 `email_domain_not_allowed` guarantees no account and no email.

---

## `POST /api/platform/login` — modified

Authenticates with a password. **Behaviour change**: an unverified account is
refused.

**Request**: `{ "email": "...", "password": "..." }` (unchanged)

**Responses**

| Status | When | Body |
|--------|------|------|
| 200 OK | verified account, valid password | `{ "user": { ... } }` + session cookie (unchanged) |
| 401 Unauthorized | unknown email or wrong password | error `invalid_credentials` |
| 403 Forbidden | correct password, account not yet verified | error `email_not_verified` |

The 403 is returned only after the password is verified, so it does not leak
account existence (FR-018). The frontend uses the `email_not_verified` code to
offer the resend action.

---

## `POST /api/platform/verify-email` — new

Completes verification when the recipient opens the emailed link. The email link
points at the frontend page `/verify-email?token=<raw>`, which calls this
endpoint. Public (no session required).

**Request**

```json
{ "token": "<raw verification token from the email link>" }
```

**Responses**

| Status | When | Body |
|--------|------|------|
| 200 OK | valid live token → account now verified | `{ "verification": { "status": "verified" } }` |
| 200 OK | token already consumed → idempotent | `{ "verification": { "status": "already_verified" } }` |
| 422 Unprocessable Entity | unknown or expired token | error `verification_link_invalid` |
| 400 Bad Request | body is not valid JSON | error `invalid_body` |

Opening an already-used link never errors (FR-010). The `verification_link_invalid`
message is generic and offers no account information (FR-018); the frontend pairs
it with the resend form.

---

## `POST /api/platform/verify-email/resend` — new

Requests a fresh verification email. Public (the unverified person has no
session). Always returns the same shape regardless of whether the email matches
an account, so it cannot be used to enumerate accounts (FR-018).

**Request**

```json
{ "email": "ada@example.com" }
```

**Responses**

| Status | When | Body |
|--------|------|------|
| 202 Accepted | request accepted (email sent, or silently no-op for unknown/already-verified address) | `{ "verification": { "resent": true } }` |
| 429 Too Many Requests | per-account resend throttle exceeded | error `verification_resend_throttled` |
| 400 Bad Request | body is not valid JSON | error `invalid_body` |

Behaviour by account state (never disclosed to the caller):

- unknown email → no email sent, 202.
- unverified account → new token issued (prior unconsumed tokens deleted), job
  enqueued, 202.
- already-verified account → no email sent, 202.

The throttle is the one observable signal and is keyed per email; the
`verification_resend_throttled` slug maps to 429 via a new `errmap.go` slug
override.

---

## Internal contract — River job `auth.verification_send`

Not an HTTP endpoint; the contract between the enqueuer (API) and the worker
(`cmd/worker`).

**Payload** (`jobs.VerificationSendArgs`)

```json
{ "user_id": "<uuid>", "verification_token": "<raw token>" }
```

**Worker behaviour** (`auth/adapters.VerificationWorker`)

1. Load the user by `user_id`. If the user is missing or already verified →
   return `nil` (River redelivery is harmless; idempotent).
2. Build the verify URL: `<PublicBaseURL>/verify-email?token=<verification_token>`.
3. Render subject + text/HTML body in the user's locale (`en`/`ru`, default `en`).
4. Send via `VerificationMailer` with From
   `no-reply@<VerificationSenderDomain>`.
5. A send failure returns an error so River retries with backoff.
