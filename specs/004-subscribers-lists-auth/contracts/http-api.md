# Contract — HTTP API

New endpoints added by Phase 2. All are tenant-scoped and mount under the
existing `/t/{slug}/api` route group, so each request already passes through
`requireUser` (control-plane authentication) and `resolveTenant`
(tenant-resolution + membership cross-check). Phase 2 adds a third middleware,
`authz`, which resolves the request's `Principal` (tenant-plane session or
scoped API key) and attaches it to the context; handlers then enforce
permissions.

Conventions inherited from Phase 1: JSON request/response bodies; errors use the
shared envelope `{ "error": { "slug": ..., "message": ... } }`; status codes are
produced solely by `internal/api/errmap.go`. Phase 2 adds the mapping
`apperr.Forbidden → 403`.

## Authentication & status codes

| Situation | Category | HTTP |
|---|---|---|
| No / invalid session or API key | `apperr.Authorization` | 401 |
| Authenticated but permission not held | `apperr.Forbidden` | 403 |
| Validation failure | `apperr.IncorrectInput` | 422 |
| Duplicate (e.g. subscriber email) | `apperr.Conflict` | 409 |
| Missing resource | `apperr.NotFound` | 404 |

A request may authenticate **either** with the tenant-plane session cookie
**or** with `Authorization: Bearer <api-key>`. An API key's permissions are the
key's scoped subset.

## Lists (US1)

| Method & path | Permission | Notes |
|---|---|---|
| `POST /t/{slug}/api/lists` | `lists:manage` | Create a list. 422 on blank name, 409 on duplicate name. |
| `GET /t/{slug}/api/lists` | `lists:get` | Paginated. |
| `GET /t/{slug}/api/lists/{id}` | `lists:get` | 404 if absent. |
| `PUT /t/{slug}/api/lists/{id}` | `lists:manage` (tenant or per-list) | Rename / describe / retag. |
| `DELETE /t/{slug}/api/lists/{id}` | `lists:manage` (tenant or per-list) | Removes memberships; subscribers untouched. |

## Subscribers (US1)

| Method & path | Permission | Notes |
|---|---|---|
| `POST /t/{slug}/api/subscribers` | `subscribers:manage` | Body: email, name, attributes, optional list ids. 409 on duplicate email. |
| `GET /t/{slug}/api/subscribers` | `subscribers:get` | Paginated; supports `q` text search. |
| `GET /t/{slug}/api/subscribers/{id}` | `subscribers:get` | |
| `PUT /t/{slug}/api/subscribers/{id}` | `subscribers:manage` | Name, attributes, state. |
| `DELETE /t/{slug}/api/subscribers/{id}` | `subscribers:manage` | Removes from all its lists. |
| `POST /t/{slug}/api/subscribers/{id}/lists` | `subscribers:manage` + per-list `lists:manage` | Add to a list. |
| `DELETE /t/{slug}/api/subscribers/{id}/lists/{listId}` | as above | Remove from a list. |
| `PUT /t/{slug}/api/subscribers/{id}/lists/{listId}` | as above | Change subscription status. |

## Segments (US4)

| Method & path | Permission | Notes |
|---|---|---|
| `POST /t/{slug}/api/subscribers/query` | `subscribers:get` | Body: a segment query. Returns matching subscribers (paginated) + total count. 422 on a malformed query. |
| `POST /t/{slug}/api/subscribers/query/count` | `subscribers:get` | Returns only the count. |

## Import & export (US3)

| Method & path | Permission | Notes |
|---|---|---|
| `POST /t/{slug}/api/import` | `subscribers:import` | `multipart/form-data`: a CSV or ZIP file + target list ids. Stages the file, enqueues a job, returns the job id. 202 Accepted. |
| `POST /t/{slug}/api/export` | `subscribers:export` | Body: selection (`all` / `list:<id>` / segment). Enqueues a job, returns the job id. 202 Accepted. |
| `GET /t/{slug}/api/jobs/{id}` | `subscribers:import` or `subscribers:export` | Job status: state, progress, created/updated/failed counts, failures. |
| `GET /t/{slug}/api/jobs/{id}/download` | `subscribers:export` | Downloads the generated CSV once the export job is `completed`. 409 if not ready. |

**CSV format**: a header row is required. Reserved header names map to subscriber
fields (`email` — required, `name`, `state`); every other column maps to a
custom attribute. Invalid rows are skipped and reported in the job's `failures`.

## Roles & RBAC (US2)

| Method & path | Permission | Notes |
|---|---|---|
| `POST /t/{slug}/api/roles` | `roles:manage` | Create a role with a permission set. |
| `GET /t/{slug}/api/roles` | `roles:get` | List roles. |
| `PUT /t/{slug}/api/roles/{id}` | `roles:manage` | Rename / change permissions. |
| `DELETE /t/{slug}/api/roles/{id}` | `roles:manage` | 409 if still assigned (caller must reassign first). |
| `PUT /t/{slug}/api/users/{userId}/role` | `roles:manage` | Assign a tenant-level role. |
| `PUT /t/{slug}/api/users/{userId}/lists/{listId}/role` | `roles:manage` | Grant a per-list role. |
| `DELETE /t/{slug}/api/users/{userId}/lists/{listId}/role` | `roles:manage` | Remove a per-list role. |

Every write here produces an `audit_log` record.

## API keys (US5)

| Method & path | Permission | Notes |
|---|---|---|
| `POST /t/{slug}/api/api-keys` | `apikeys:manage` | Body: name + permission subset. Returns the raw key **once**. |
| `GET /t/{slug}/api/api-keys` | `apikeys:get` | Lists keys (metadata only, never the token). |
| `DELETE /t/{slug}/api/api-keys/{id}` | `apikeys:manage` | Revokes the key. |

API key issuance and revocation produce `audit_log` records.

## Workspace session & TOTP 2FA (US5)

| Method & path | Auth | Notes |
|---|---|---|
| `POST /t/{slug}/api/session` | control-plane authenticated | Opens a tenant-plane working session. If the user has TOTP enabled, returns a `totp-pending` session. |
| `POST /t/{slug}/api/session/totp` | `totp-pending` session | Body: a TOTP code or a recovery code. Activates the session. 401 on a wrong/missing code. |
| `DELETE /t/{slug}/api/session` | active session | Closes the session. |
| `POST /t/{slug}/api/me/totp` | active session | Begin TOTP enrolment — returns a provisioning secret/URI. |
| `POST /t/{slug}/api/me/totp/confirm` | active session | Confirm enrolment with a code; returns one-time recovery codes. |
| `DELETE /t/{slug}/api/me/totp` | active session, re-authenticated | Disable TOTP. |

A `totp-pending` session carries no permissions — every guarded endpoint returns
401 until the challenge is met.
