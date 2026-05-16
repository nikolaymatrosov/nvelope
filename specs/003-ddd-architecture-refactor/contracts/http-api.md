# Contract: HTTP API — the regression baseline

This is the **frozen** external surface. The refactor is behavior-preserving
(FR-001, SC-001): after the work, every route below MUST respond with the same
status codes, JSON shapes, error slugs, and cookie behavior as before. This file
is the checklist the endpoint/component tests verify against — it documents the
*current* contract, which the refactor must not alter.

## Routes

| Method | Path | Auth | Notes |
|---|---|---|---|
| GET | `/healthz` | none | liveness/readiness |
| POST | `/api/platform/signup` | none | creates user + session; sets cookie; `201` |
| POST | `/api/platform/login` | none | issues session; sets cookie; `200` |
| GET | `/api/platform/invitations/{token}` | none | invitation lookup by raw token |
| POST | `/api/platform/invitations/{token}/accept` | optional | accept; body required only when not logged in |
| POST | `/api/platform/logout` | required | revokes session; clears cookie; `204` |
| GET | `/api/platform/me` | required | current user + memberships |
| POST | `/api/platform/tenants` | required | create workspace; `201` |
| GET | `/api/platform/tenants` | required | list caller's workspaces |
| GET | `/t/{slug}/api/tenant` | required + member | tenant info + members |
| GET | `/t/{slug}/api/settings` | required + member | tenant settings |
| PUT | `/t/{slug}/api/settings` | required + member | update settings; `200` |
| POST | `/t/{slug}/api/invitations` | required + member | create invitation; `201` |
| GET | `/t/{slug}/api/invitations` | required + member | list pending invitations |
| DELETE | `/t/{slug}/api/invitations/{id}` | required + member | revoke invitation; `204` |

## Response envelopes (unchanged)

- **Success**: handler-specific JSON object, e.g. `{"user": {...}}`,
  `{"tenant": {...}}`, `{"tenants": [...]}`, `{"settings": {...}}`,
  `{"invitation": {...}, "accept_url": "..."}`.
- **Error**: `{"error": "<slug>", "message": "<human text>"}`.
- A `204` response has an empty body.

## Error slugs (must remain byte-identical)

| Slug | Status | Raised by |
|---|---|---|
| `invalid_body` | 400 | malformed / non-JSON request body |
| `unauthenticated` | 401 | missing or invalid session on a required-auth route |
| `invalid_credentials` | 401 | login: unknown email or wrong password (identical for both) |
| `email_taken` | 409 | signup with an already-registered email |
| `slug_taken` | 409 | workspace slug already in use |
| `invitation_exists` | 409 | pending invitation for that email already exists |
| `invitation_not_found` | 404 | unknown/expired/revoked/accepted invitation (opaque) |
| `tenant_not_found` | 404 | unknown slug **or** caller not a member (opaque — never 403) |
| `validation_failed` | 422 | input failed a domain validation rule |
| `email_taken` (accept-invitation) | 409 | invitee already has an account — must log in first |
| `internal_error` | 500 | unrecognized/unexpected error |

The `accept-invitation` route also returns the success-shaped
`{"error": "already_member", "message": ...}` with status `200` when the invitee
is already a member — this quirk is preserved as-is.

## Behaviors that MUST survive the refactor

- **Account-enumeration resistance**: `login` returns identical `invalid_credentials`
  for unknown email and wrong password; invitation lookup returns identical
  `invitation_not_found` regardless of why the token is not usable.
- **Opaque cross-tenant denial**: an unknown slug and a non-member both yield
  `404 tenant_not_found` — never `403`, so membership cannot be probed.
- **Session cookie**: name `nv_session`; `HttpOnly`, `Secure`, `SameSite=Lax`,
  `Path=/`; `MaxAge` from the session TTL; logout sends `MaxAge=-1`.
- **Token secrecy**: raw session/invite tokens appear only in the issuing
  response (cookie or `accept_url`); only SHA-256 hashes are persisted.
- **RLS isolation**: tenant-plane reads/writes (`/t/{slug}/api/settings`) execute
  inside an `app.tenant_id`-bound transaction and fail closed when unbound.
