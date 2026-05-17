# Phase 1 Data Model: Phases 1 & 2 — Frontend UI

This is the **frontend view model**: the TypeScript types the UI consumes,
mirrored from the backend's query read models and command inputs. The frontend
owns no persistent storage — these types live in `src/lib/api-types.ts` and are
the contract between the typed API client and the screens.

**Casing convention** (see research.md Decision 2): platform-plane and tenant
settings/invitation responses are **snake_case**; audience and IAM responses
are **PascalCase** (those Go view structs carry no `json:` tags). Types below
state the casing each endpoint actually returns. Do not rename fields in the
client.

## Platform plane (snake_case)

### PlatformAccount — `GET /api/platform/me`

| Field | Type | Notes |
| --- | --- | --- |
| `user.id` | string | Platform account id. |
| `user.name` | string | Display name. |
| `user.email` | string | Login email. |
| `tenants` | `Membership[]` | Workspaces the account belongs to. |

### Membership — element of `me.tenants` and `GET /api/platform/tenants`

| Field | Type | Notes |
| --- | --- | --- |
| `id` | string | Workspace id. |
| `slug` | string | Unique workspace address used in `/t/{slug}` URLs. |
| `name` | string | Workspace display name. |
| `status` | string | Membership status. |
| `role` | string | Member's workspace-level role name. |

### Invitation lookup — `GET /api/platform/invitations/{token}`

Returns the pending invitation addressed to an email, its workspace, status,
and expiry; drives the invitation-acceptance screen (FR-005).

## Workspace / tenant plane (snake_case)

### Workspace info — `GET /t/{slug}/api/tenant`

| Field | Type | Notes |
| --- | --- | --- |
| `tenant.name` | string | Workspace name shown in the shell. |
| `members` | `Member[]` | Current members — used to derive *my* role. |

### Member — element of `tenant.members`

| Field | Type | Notes |
| --- | --- | --- |
| `user_id` | string | Tenant-plane user id. |
| `email` | string | Member email. |
| `name` | string | Member display name. |
| `role` | string | Member's workspace-level role name. |

### WorkspaceInvitation — `GET /t/{slug}/api/invitations`

| Field | Type | Notes |
| --- | --- | --- |
| `id` | string | Invitation id (target of revoke). |
| `email` | string | Invited email address. |
| `status` | string | e.g. pending / expired. |
| `created_at` | string (ISO) | When sent. |
| `expires_at` | string (ISO) | When the invite lapses. |

`POST /t/{slug}/api/invitations` → `201 { accept_url }`.

### WorkspaceSettings — `GET` / `PUT /t/{slug}/api/settings`

| Field | Type | Notes |
| --- | --- | --- |
| `display_name` | string | Workspace display name (FR-030). |
| `timezone` | string | Workspace timezone. |

The UI renders/edits whatever fields this endpoint exposes (Assumptions).

## Audience plane — Lists & Subscribers (PascalCase)

### List — `GET /t/{slug}/api/lists` (`{ lists: List[], total }`), `GET .../lists/{id}` (`{ list }`)

| Field | Type | Notes |
| --- | --- | --- |
| `ID` | string | List id. |
| `Name` | string | List name. |
| `Description` | string | List description. |
| `Visibility` | string | `private` (default) or other visibility value. |
| `OptIn` | string | `single` (default) or other opt-in mode. |
| `Tags` | string[] | Free-form tags. |
| `CreatedAt` | string (ISO) | Creation time. |
| `UpdatedAt` | string (ISO) | Last update time. |

Create input (`POST /lists`): `{ name, description, visibility?, optin?, tags? }`
→ `201 { id }`. Update is `PUT /lists/{id}` → `204`. Delete is
`DELETE /lists/{id}` → `204` (explicit confirmation required, FR-010/FR-031).

### Subscriber — `GET /t/{slug}/api/subscribers` and `.../subscribers/{id}`

| Field | Type | Notes |
| --- | --- | --- |
| `ID` | string | Subscriber id. |
| `Email` | string | Unique within the workspace. |
| `Name` | string | Optional display name. |
| `State` | string | `enabled` (default) or other state. |
| `Attributes` | `Record<string, unknown>` | Free-form custom JSON (FR-013). |
| `Memberships` | `SubscriberMembership[]` | Lists this subscriber belongs to. |
| `CreatedAt` | string (ISO) | Creation time. |
| `UpdatedAt` | string (ISO) | Last update time. |

### SubscriberMembership — element of `Subscriber.Memberships`

| Field | Type | Notes |
| --- | --- | --- |
| `ListID` | string | The list the subscriber belongs to. |
| `Status` | string | Subscription state on that list (e.g. subscribed/unsubscribed). |

Create input (`POST /subscribers`): `{ email, name, attributes, list_ids }` →
`201 { id }`. Update (`PUT /subscribers/{id}`): `{ name, attributes, state }`.
List membership operations: `POST /subscribers/{id}/lists { list_id }`,
`DELETE /subscribers/{id}/lists/{listId}`,
`PUT /subscribers/{id}/lists/{listId} { status }` (FR-014). Duplicate email on
create returns a `409` the UI surfaces clearly (FR + acceptance scenario).

### Segment — request body for `POST /subscribers/query[/count]`

Recursive `Node` tree (PascalCase keys — the domain struct has no `json:`
tags). Each node is **either** a group **or** one leaf condition:

| Field | Type | Notes |
| --- | --- | --- |
| `Conj` | `"and" \| "or"` | Set on a group node. |
| `Children` | `Node[]` | Group's child nodes. |
| `Field` | `FieldCondition?` | Leaf — matches `email`/`name`/`state`. |
| `Attr` | `AttrCondition?` | Leaf — matches a custom-attribute key. |
| `Member` | `MemberCondition?` | Leaf — matches list membership. |

`FieldCondition { Field, Op, Value: string }`,
`AttrCondition { Key, Op, Value: unknown }`,
`MemberCondition { ListID, Status }`. `Op` ∈ `eq, neq, exists, contains, gt,
lt, gte, lte` (comparison ops only for ordered operands). Request body is
`{ segment: Node }`; response is `{ subscribers, total }` or `{ total }`.

## Jobs — Import & Export (PascalCase)

### Job status — `GET /t/{slug}/api/jobs/{id}`

| Field | Type | Notes |
| --- | --- | --- |
| `ID` | string | Job id. |
| `Kind` | string | `import` or `export`. |
| `Status` | string | Lifecycle state; non-terminal values trigger polling. |
| `FileName` | string | Source file name (import). |
| `CreatedCount` | number | Subscribers created. |
| `UpdatedCount` | number | Subscribers updated (upserts). |
| `FailedCount` | number | Rows that failed. |
| `RowCount` | number | Total rows processed. |
| `Failures` | `RowFailure[]` | Per-row failure detail. |

`RowFailure { Row: number, Reason: string }`. Start import:
`POST /import` multipart (`file`, repeated `list_ids`) → `202 { job_id }`.
Start export: `POST /export { selection, list_id?, segment? }` → `202
{ job_id }`. Download: `GET /jobs/{id}/download` (browser navigation).

## IAM plane — Roles, API keys, Audit (PascalCase)

### Role — `GET /t/{slug}/api/roles` (`{ roles: Role[] }`)

| Field | Type | Notes |
| --- | --- | --- |
| `ID` | string | Role id. |
| `Name` | string | Role name; `"Owner"` is the bootstrap all-permissions role. |
| `Permissions` | `Permission[]` | Permission slugs granted by the role. |
| `CreatedAt` | string (ISO) | Creation time. |
| `UpdatedAt` | string (ISO) | Last update time. |

Create (`POST /roles { name, permissions }`) → `201 { id }`. Update
(`PUT /roles/{id}`), delete (`DELETE /roles/{id}`). Assign at workspace level:
`PUT /users/{userId}/role { role_id }`. Per-list role:
`PUT /users/{userId}/lists/{listId}/role { role_id }`, removed with the
matching `DELETE` (FR-020).

### Permission (enum)

`lists:get`, `lists:manage`, `subscribers:get`, `subscribers:manage`,
`subscribers:import`, `subscribers:export`, `roles:get`, `roles:manage`,
`apikeys:get`, `apikeys:manage`, `audit:get`, `settings:get`,
`settings:manage`. The role editor offers exactly this set.

### EffectivePermissions — derived, not an endpoint

Computed client-side (research.md Decision 3): the signed-in user's id from
`me`, joined to their `Member.role` in `GET /tenant`, joined to that role's
`Permissions` in `GET /roles`. Drives FR-009 nav/action gating. Per-list grants
are not enumerable — per-list gating is reactive on `403`.

### APIKey — `GET /t/{slug}/api/api-keys` (`{ api_keys: APIKey[] }`)

| Field | Type | Notes |
| --- | --- | --- |
| `ID` | string | Key id. |
| `Name` | string | Key label. |
| `Permissions` | `Permission[]` | Scoped permissions. |
| `CreatedAt` | string (ISO) | Issue time. |
| `LastUsedAt` | string (ISO) \| null | Last use, or null. |
| `RevokedAt` | string (ISO) \| null | Revocation time, or null if active. |

Issue (`POST /api-keys { name, permissions }`) → `201 { id, token }` — `token`
is the **secret, shown exactly once** (FR-028). Revoke: `DELETE /api-keys/{id}`.

### AuditRecord — `GET /t/{slug}/api/audit` (`{ records, total }`)

| Field | Type | Notes |
| --- | --- | --- |
| `ID` | string | Record id. |
| `ActorID` | string | Who performed the action. |
| `ActorKind` | string | Actor type (session/API key). |
| `Action` | string | Action performed. |
| `Target` | string | Affected entity. |
| `Metadata` | `Record<string, unknown>` | Extra detail. |
| `CreatedAt` | string (ISO) | When it happened. |

### TOTP enrolment — `POST /me/totp`, `POST /me/totp/confirm`, `DELETE /me/totp`

Enable returns `{ secret, uri }` (URI renders the QR). Confirm posts
`{ secret, code }` and returns `{ recovery_codes }`. The session-open TOTP
challenge posts `{ code }` to `POST /t/{slug}/api/session/totp` (FR-026/FR-027).

## Workspace session state machine

`POST /t/{slug}/api/session` opens a session and returns `{ state }`:

```text
        ┌──────────────┐  TOTP not enabled
open ──▶│   (request)  │──────────────────▶ active
        └──────┬───────┘
               │ TOTP enabled
               ▼
         totp_pending ──POST /session/totp {code}──▶ active
               │  invalid/expired code
               └──────────────▶ totp_pending (retry, clear error)
```

`active` reveals the workspace shell; `totp_pending` renders the TOTP
challenge first (FR-027). `DELETE /t/{slug}/api/session` closes the session.

## Error envelope

Every error response is `{ "error": "<slug>", "message": "<human text>" }`.
Status codes (from `internal/api/errmap.go`): `401` unauthenticated, `403`
forbidden, `404` not found, `409` conflict, `422` incorrect input, `500`
internal. The client normalizes this into `ApiError { status, slug, message }`
(research.md Decision 4); screens branch on `slug`/status category, never on
message text.
