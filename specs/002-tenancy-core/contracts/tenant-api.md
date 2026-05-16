# Contract: Tenant-Scoped API

Endpoints scoped to a single tenant, mounted under `/t/{slug}/api`. Every route below passes
through the **tenant resolution middleware** before its handler runs.

- Content type: `application/json`.
- Authentication: the `nv_session` cookie; the auth middleware runs before tenant resolution.
- Errors: `{ "error": "<machine_code>", "message": "<human readable>" }`.

## Tenant resolution middleware

Runs on every `/t/{slug}/...` request, in order:

1. **Authenticate** — resolve the `nv_session` cookie to a valid session and its platform user.
   No valid session → `401 Unauthorized`, `error: "unauthenticated"`.
2. **Resolve** — load the tenant whose `slug` matches the path segment.
3. **Cross-check** — confirm a `platform_user_tenants` row links the session's user to the
   resolved tenant.
4. **Reject opaquely** — if the slug matches no tenant **or** the user is not a member, respond
   `404 Not Found`, `error: "tenant_not_found"`. The two cases are indistinguishable, so tenant
   existence is never revealed to a non-member (FR-013).
5. **Bind** — on success, place the resolved `tenant_id` and the membership in the request
   context. Tenant-plane handlers open their database work through `tenant.WithTenant`, which
   runs the request inside a transaction bound to that `tenant_id` (see `rls-isolation.md`).

The middleware never trusts the slug alone — access always requires the authenticated session
to own a membership in the resolved tenant (FR-012).

## GET /t/{slug}/api/tenant

Return the tenant and its members.

Response `200 OK`:

```json
{
  "tenant": { "id", "slug", "name", "status": "active" },
  "members": [ { "user_id", "email", "name", "role": "owner" } ]
}
```

## GET /t/{slug}/api/settings

Return the tenant's settings row. The handler reads `tenant_settings` inside a tenant-bound
transaction; RLS guarantees only this tenant's row is visible.

Response `200 OK`: `{ "settings": { "display_name", "timezone" } }`.

## PUT /t/{slug}/api/settings

Update the tenant's settings. The write runs inside a tenant-bound transaction; the RLS
`WITH CHECK` clause makes it impossible to write another tenant's row.

Request: `{ "display_name": "Acme", "timezone": "Europe/Madrid" }`

Responses:

- `200 OK` — `{ "settings": { "display_name", "timezone" } }`.
- `422 Unprocessable Entity` — `error: "validation_failed"` for an empty display name or an
  unknown timezone.

## POST /t/{slug}/api/invitations

Invite a teammate to this tenant by email.

Request: `{ "email": "grace@example.com" }`

Behavior: creates a `pending` `invitations` row with a freshly generated token and an expiry.
The response carries the acceptance link for the inviter to share (research §8).

Responses:

- `201 Created`:

  ```json
  {
    "invitation": { "id", "email", "status": "pending", "expires_at" },
    "accept_url": "https://app.example.com/invite/<token>"
  }
  ```

  `accept_url` is the **only** time the raw token is returned.
- `200 OK` — `error: "already_member"` when the email already belongs to a member of this
  tenant; no invitation is created (FR-010).
- `409 Conflict` — `error: "invitation_exists"` when a pending invitation for that email
  already exists in this tenant.
- `422 Unprocessable Entity` — `error: "validation_failed"` for a malformed email.

## GET /t/{slug}/api/invitations

List this tenant's pending invitations.

Response `200 OK`: `{ "invitations": [ { "id", "email", "status", "created_at", "expires_at" } ] }`.

## DELETE /t/{slug}/api/invitations/{id}

Revoke a pending invitation (sets `status = 'revoked'`).

Responses:

- `204 No Content` — revoked.
- `404 Not Found` — `error: "invitation_not_found"` when no pending invitation with that id
  exists in this tenant.

## Status code summary

| Code | Meaning |
|---|---|
| `200` / `201` / `204` | Success |
| `401` | No valid session |
| `404` | Tenant not resolvable for this user, or invitation not found |
| `409` | A pending invitation for that email already exists |
| `422` | Request body failed validation |
