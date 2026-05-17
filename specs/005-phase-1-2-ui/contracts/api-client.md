# Contract: Typed API Client Surface

The frontend's externally-facing contract is the **typed API client**
(`src/lib/api.ts`) — the single transport layer every screen depends on
(Constitution Principle VI). This file pins the client's method surface to the
backend routes in `internal/api/server.go`. The backend API is fixed and
stable; this contract makes the client conform to it.

Conventions:
- All paths are same-origin; the browser sends the session cookie
  (`credentials: "include"`). The Vite dev server proxies `/api` and
  `^/t/[^/]+/api` to the Go service.
- Tenant-scoped methods take `slug` as their **first argument** so a call site
  cannot omit the tenant (tenant-isolation safety, Principle I).
- Response casing per endpoint is noted; see research.md Decision 2.
- Every method resolves to `ApiResult<T> = { status, ok, data }`; errors are
  normalized to `ApiError` by the shared error layer (research.md Decision 4).

## Platform plane — `/api/platform/*` (snake_case)

| Client method | HTTP | Path | Auth | Result |
| --- | --- | --- | --- | --- |
| `signup(email, password, name)` | POST | `/signup` | none | `200` session established |
| `login(email, password)` | POST | `/login` | none | `200` / `401` non-specific error |
| `logout()` | POST | `/logout` | user | `200` |
| `me()` | GET | `/me` | user | `{ user, tenants }` |
| `createTenant(name, slug)` | POST | `/tenants` | user | `201` / `409` slug conflict |
| `listTenants()` | GET | `/tenants` | user | `{ tenants }` |
| `getInvitation(token)` | GET | `/invitations/{token}` | none | invitation lookup |
| `acceptInvitation(token, password?, name?)` | POST | `/invitations/{token}/accept` | none/user | `200` member joined |

## Tenant plane — `/t/{slug}/api/*`

### Workspace, settings, invitations (snake_case)

| Client method | HTTP | Path | Permission |
| --- | --- | --- | --- |
| `tenant(slug)` | GET | `/tenant` | membership |
| `getSettings(slug)` | GET | `/settings` | `settings:get` |
| `updateSettings(slug, body)` | PUT | `/settings` | `settings:manage` |
| `invite(slug, email)` | POST | `/invitations` | (member) → `201 { accept_url }` |
| `listInvitations(slug)` | GET | `/invitations` | (member) |
| `revokeInvitation(slug, id)` | DELETE | `/invitations/{id}` | (member) → `204` |

### Workspace session (no principal required — these establish one)

| Client method | HTTP | Path | Notes |
| --- | --- | --- | --- |
| `openSession(slug)` | POST | `/session` | `201 { state }` — `active` or `totp_pending` |
| `closeSession(slug)` | DELETE | `/session` | `204` |
| `verifySessionTOTP(slug, code)` | POST | `/session/totp` | `200 { state }` |

### Lists (PascalCase responses) — guarded

| Client method | HTTP | Path | Permission |
| --- | --- | --- | --- |
| `createList(slug, body)` | POST | `/lists` | `lists:manage` → `201 { id }` |
| `listLists(slug, page)` | GET | `/lists?limit=&offset=` | `lists:get` → `{ lists, total }` |
| `getList(slug, id)` | GET | `/lists/{id}` | `lists:get` (per-list) → `{ list }` |
| `updateList(slug, id, body)` | PUT | `/lists/{id}` | `lists:manage` (per-list) → `204` |
| `deleteList(slug, id)` | DELETE | `/lists/{id}` | `lists:manage` (per-list) → `204` |

### Subscribers (PascalCase responses) — guarded

| Client method | HTTP | Path | Permission |
| --- | --- | --- | --- |
| `createSubscriber(slug, body)` | POST | `/subscribers` | `subscribers:manage` → `201 { id }` |
| `searchSubscribers(slug, q, page)` | GET | `/subscribers?q=&limit=&offset=` | `subscribers:get` |
| `querySubscribers(slug, segment, page)` | POST | `/subscribers/query` | `subscribers:get` |
| `countSubscribers(slug, segment)` | POST | `/subscribers/query/count` | `subscribers:get` → `{ total }` |
| `getSubscriber(slug, id)` | GET | `/subscribers/{id}` | `subscribers:get` |
| `updateSubscriber(slug, id, body)` | PUT | `/subscribers/{id}` | `subscribers:manage` → `204` |
| `deleteSubscriber(slug, id)` | DELETE | `/subscribers/{id}` | `subscribers:manage` → `204` |
| `addToList(slug, id, listId)` | POST | `/subscribers/{id}/lists` | `subscribers:manage` (per-list) |
| `removeFromList(slug, id, listId)` | DELETE | `/subscribers/{id}/lists/{listId}` | `subscribers:manage` (per-list) |
| `changeSubscription(slug, id, listId, status)` | PUT | `/subscribers/{id}/lists/{listId}` | `subscribers:manage` (per-list) |

### Import / export jobs — guarded

| Client method | HTTP | Path | Permission |
| --- | --- | --- | --- |
| `startImport(slug, file, listIds)` | POST | `/import` | `subscribers:import` — **multipart** → `202 { job_id }` |
| `startExport(slug, body)` | POST | `/export` | `subscribers:export` → `202 { job_id }` |
| `jobStatus(slug, id)` | GET | `/jobs/{id}` | (principal) → `JobStatusView` |
| `downloadExport(slug, id)` | GET | `/jobs/{id}/download` | (principal) — browser navigation |

`startImport` requires a multipart-aware request path the current JSON-only
`request` helper does not have (research.md Decision 10).

### Roles & assignments (PascalCase responses) — guarded

| Client method | HTTP | Path | Permission |
| --- | --- | --- | --- |
| `createRole(slug, name, permissions)` | POST | `/roles` | `roles:manage` → `201 { id }` |
| `listRoles(slug)` | GET | `/roles` | `roles:get` → `{ roles }` |
| `updateRole(slug, id, body)` | PUT | `/roles/{id}` | `roles:manage` → `204` |
| `deleteRole(slug, id)` | DELETE | `/roles/{id}` | `roles:manage` → `204` |
| `assignRole(slug, userId, roleId)` | PUT | `/users/{userId}/role` | `roles:manage` → `204` |
| `assignListRole(slug, userId, listId, roleId)` | PUT | `/users/{userId}/lists/{listId}/role` | `roles:manage` → `204` |
| `removeListRole(slug, userId, listId)` | DELETE | `/users/{userId}/lists/{listId}/role` | `roles:manage` → `204` |

### API keys (PascalCase responses) — guarded

| Client method | HTTP | Path | Permission |
| --- | --- | --- | --- |
| `issueAPIKey(slug, name, permissions)` | POST | `/api-keys` | `apikeys:manage` → `201 { id, token }` |
| `listAPIKeys(slug)` | GET | `/api-keys` | `apikeys:get` → `{ api_keys }` |
| `revokeAPIKey(slug, id)` | DELETE | `/api-keys/{id}` | `apikeys:manage` → `204` |

`token` from `issueAPIKey` is the secret — shown exactly once (FR-028).

### TOTP enrolment & audit — guarded

| Client method | HTTP | Path | Permission |
| --- | --- | --- | --- |
| `enableTOTP(slug)` | POST | `/me/totp` | session principal → `{ secret, uri }` |
| `confirmTOTP(slug, secret, code)` | POST | `/me/totp/confirm` | session principal → `{ recovery_codes }` |
| `disableTOTP(slug)` | DELETE | `/me/totp` | session principal → `204` |
| `auditTrail(slug, page)` | GET | `/audit?limit=&offset=` | `audit:get` → `{ records, total }` |

## Error contract

Every non-2xx response carries `{ "error": "<slug>", "message": "<text>" }`.
The client raises/normalizes `ApiError { status, slug, message }`. Status
categories (`internal/api/errmap.go`): `401` unauthenticated, `403` forbidden,
`404` not found, `409` conflict, `422` incorrect input, `500` internal. The
global error handler routes `401`/`403`/`404` (research.md Decision 4); screens
never branch on `message` text.

## Contract tests

The client contract is verified by Vitest tests that mock `fetch` and assert:
1. Each method calls the correct HTTP verb + path (slug interpolated).
2. Tenant methods cannot be invoked without a slug (type-level + runtime).
3. Multipart `startImport` sends `file` and repeated `list_ids` fields.
4. A non-2xx body normalizes to `ApiError` with the right `status`/`slug`.
5. PascalCase responses deserialize into the PascalCase view types unchanged.
