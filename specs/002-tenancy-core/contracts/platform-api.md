# Contract: Platform API

Endpoints for platform-level actions that are **not** scoped to a tenant: signing up, logging
in, creating and listing tenants, and accepting invitations. Mounted under `/api/platform`.

- Content type: `application/json` for request and response bodies.
- Authentication: an `HttpOnly`, `Secure`, `SameSite=Lax` session cookie (`nv_session`) set on
  signup/login. Endpoints marked **auth required** reject a missing/invalid session with `401`.
- Errors: `{ "error": "<machine_code>", "message": "<human readable>" }`.

## POST /api/platform/signup

Create a platform account and start a session.

Request:

```json
{ "email": "ada@example.com", "password": "correct horse", "name": "Ada Lovelace" }
```

Responses:

- `201 Created` — `{ "user": { "id", "email", "name" } }`; sets `nv_session` cookie.
- `409 Conflict` — `error: "email_taken"` when the email is already registered (FR-002).
- `422 Unprocessable Entity` — `error: "validation_failed"` for a bad email shape or a
  password shorter than 8 characters.

## POST /api/platform/login

Authenticate and start a session.

Request: `{ "email": "ada@example.com", "password": "correct horse" }`

Responses:

- `200 OK` — `{ "user": { "id", "email", "name" } }`; sets `nv_session` cookie.
- `401 Unauthorized` — `error: "invalid_credentials"`, message `"invalid email or password"`,
  returned identically whether the email is unknown or the password is wrong (research §10).

## POST /api/platform/logout

**Auth required.** Revokes the current session and clears the cookie.

Response: `204 No Content`.

## GET /api/platform/me

**Auth required.** Returns the current user and their tenant memberships.

Response `200 OK`:

```json
{
  "user": { "id", "email", "name" },
  "tenants": [ { "id", "slug", "name", "role": "owner" } ]
}
```

## POST /api/platform/tenants

**Auth required.** Create a tenant; the caller becomes its `owner`.

Request: `{ "name": "Acme Newsletters", "slug": "acme" }` — `slug` optional; when omitted it is
derived from `name`.

Behavior: in one transaction, inserts the `tenants` row, the caller's `platform_user_tenants`
row (`role = 'owner'`), and the tenant's `tenant_settings` row.

Responses:

- `201 Created` — `{ "tenant": { "id", "slug", "name", "status": "active" } }`.
- `409 Conflict` — `error: "slug_taken"` when the slug is already in use (FR-005).
- `422 Unprocessable Entity` — `error: "validation_failed"` for an empty name, a slug failing
  the regex, or a reserved slug.

## GET /api/platform/invitations/{token}

Public (no auth). Look up a pending invitation by its raw token, to render the accept screen.

Responses:

- `200 OK` — `{ "tenant": { "slug", "name" }, "email": "invitee@example.com" }`.
- `404 Not Found` — `error: "invitation_not_found"` for an unknown, expired, revoked, or
  already-accepted token (generic; research §10).

## POST /api/platform/invitations/{token}/accept

Accept an invitation and join its tenant.

- If the caller has a valid session, the invitation is accepted for that user.
- If the caller has no session, the request body must carry `{ "password", "name" }`; a new
  `platform_users` account is created for the invitation's email and a session is started
  (FR-008).

Behavior (single transaction): re-validate the invitation is `pending` and unexpired; create or
resolve the user; insert a `platform_user_tenants` row unless one already exists; set the
invitation `status = 'accepted'`, `accepted_by`, `accepted_at`.

Responses:

- `200 OK` — `{ "user": { "id", "email", "name" }, "tenant": { "id", "slug", "name" } }`;
  sets `nv_session` if a new account was created.
- `404 Not Found` — `error: "invitation_not_found"` when not pending/expired/revoked (FR-009).
- `409 Conflict` — `error: "email_taken"` when no session is supplied and the invitation email
  already has an account (the user should log in, then accept).
- `422 Unprocessable Entity` — `error: "validation_failed"` for a bad password/name on the
  signup-and-accept path.

## Status code summary

| Code | Meaning |
|---|---|
| `200` / `201` / `204` | Success |
| `401` | Missing/invalid session, or bad login credentials |
| `404` | Invitation not resolvable (generic) |
| `409` | Email or slug already taken |
| `422` | Request body failed validation |
