# Phase 1 Design — Data Model

Domain entities, value objects, invariants, state transitions, and the
PostgreSQL schema for Phase 2. Every tenant-plane table carries `tenant_id` and
RLS (see research.md Decision 3). Domain entities are constructed only through
validating constructors; persisted rows are rehydrated through a separate,
explicitly labelled hydration path (constitution VI).

## Bounded context: `audience`

### List (aggregate)

Fields: `id`, `tenantID`, `name`, `description`, `visibility` (`public` |
`private`), `optIn` (`single` | `double`), `tags []string`, `createdAt`,
`updatedAt`.

Invariants:
- `name` is non-empty after trimming and unique within the tenant.
- `visibility` and `optIn` are one of their allowed values.
- Constructor `NewList(tenantID, name, description, visibility, optIn, tags)`
  rejects any violation; `HydrateList(...)` rebuilds a persisted list without
  re-validation.

Behaviors: `Rename`, `Describe`, `Retag`. No public setters.

### Subscriber (aggregate)

Fields: `id`, `tenantID`, `email`, `name`, `state` (`enabled` | `disabled` |
`blocklisted`), `attributes` (custom attributes value object), `createdAt`,
`updatedAt`.

Invariants:
- `email` is syntactically valid and unique within the tenant (case-insensitive).
- `state` is one of the allowed values; new subscribers start `enabled`.
- `attributes` is well-formed structured key/value data (see value object).

State transitions (`state`):
```
enabled  ⇄ disabled          (Disable / Enable)
enabled  → blocklisted        (Blocklist)
disabled → blocklisted        (Blocklist)
blocklisted → enabled         (Unblocklist — explicit, audited at app layer)
```

Behaviors: `Rename`, `SetAttributes`, `Enable`, `Disable`, `Blocklist`,
`Unblocklist`.

### Custom Attributes (value object)

Free-form structured key/value data attached to a subscriber (research.md
Decision 8 stores it as `jsonb`). The value object guarantees the content is
well-formed (keys are non-empty strings; values are JSON scalars, arrays, or
nested objects). No tenant-defined schema is enforced (spec assumption).

### SubscriberList Membership (entity)

The link between a subscriber and a list.

Fields: `tenantID`, `subscriberID`, `listID`, `subscriptionStatus`
(`unconfirmed` | `confirmed` | `unsubscribed`), `createdAt`, `updatedAt`.

Invariants:
- At most one membership per `(subscriberID, listID)`.
- New membership starts `unconfirmed`.

State transitions (`subscriptionStatus`):
```
unconfirmed → confirmed       (Confirm)
unconfirmed → unsubscribed    (Unsubscribe)
confirmed   → unsubscribed    (Unsubscribe)
unsubscribed → confirmed      (Resubscribe)
```

Deleting a subscriber or a list removes its memberships (FK `ON DELETE CASCADE`);
the *other* side of each membership is untouched (FR-011).

### Segment (value object)

A validated query selecting subscribers. A tree of conditions:
- **Field conditions** — over `email`, `name`, `state`.
- **Attribute conditions** — over a custom-attribute JSON path with an operator
  (`eq`, `neq`, `exists`, `contains`, comparison for numeric/date scalars).
- **Membership conditions** — over list id + subscription status.
- Conditions combine with `AND` / `OR` groups.

Invariants: every referenced field is known; every operator is valid for its
operand type; the tree is well-formed. Construction rejects a malformed query
(FR-015) before it ever reaches the adapter. The adapter translates a *validated*
Segment to parameterized SQL (research.md Decision 8).

### ImportJob (aggregate)

Fields: `id`, `tenantID`, `requestedBy`, `mode` (`upsert`), `targetListIDs
[]uuid`, `status`, `fileName`, `createdCount`, `updatedCount`, `failedCount`,
`failures []RowFailure`, `createdAt`, `startedAt`, `finishedAt`.

State transitions (`status`):
```
pending → running → completed
                  → failed
```

Invariants: counts are non-negative; `completed`/`failed` is terminal; a job
carries the staged file until it reaches a terminal state.

### ExportJob (aggregate)

Fields: `id`, `tenantID`, `requestedBy`, `selection` (`all` | `list:<id>` |
`segment`), `segment` (optional), `status`, `rowCount`, `createdAt`,
`startedAt`, `finishedAt`. Same `status` transitions as ImportJob. On
completion the generated CSV is staged for download.

## Bounded context: `iam`

### TenantUser (aggregate)

Fields: `id`, `tenantID`, `platformUserID` (link to control-plane identity —
research.md Decision 4), `email`, `name`, `status` (`active` | `suspended`),
`totpEnabled`, `totpSecret` (encrypted, optional), `createdAt`, `updatedAt`.

Invariants:
- `platformUserID` is set and unique within the tenant (one tenant-plane user
  per platform identity per tenant).
- `totpSecret` is present iff `totpEnabled` is true.

Behaviors: `EnableTOTP(secret)`, `DisableTOTP`, `Suspend`, `Reactivate`.

### Session (aggregate) — tenant-plane working session

Fields: `id`, `tenantID`, `userID`, `tokenHash`, `state` (`totp-pending` |
`active` | `revoked`), `createdAt`, `expiresAt`, `revokedAt`.

State transitions:
```
(open, no 2FA)        → active
(open, user has TOTP) → totp-pending → active   (VerifyTOTPChallenge)
active                → revoked                 (CloseSession / expiry)
totp-pending          → revoked                 (challenge abandoned / expiry)
```

Invariants: the raw token is returned once at creation and stored only as a
hash; a `totp-pending` session grants no permissions; an expired or revoked
session authenticates nothing.

### Role (aggregate)

Fields: `id`, `tenantID`, `name`, `permissions []Permission`, `createdAt`,
`updatedAt`.

Invariants: `name` non-empty and unique within the tenant; every permission is
from the known catalogue (`contracts/permissions.md`).

Behaviors: `Rename`, `SetPermissions`.

### Permission (value object)

A `resource:action` string drawn from a fixed catalogue. Equality and set
membership are value semantics. Effective-permission computation
(`EffectivePermissions(tenantRole, listRole)` = union) lives here as a pure
function (research.md Decision 5).

### Principal (value object)

The resolved actor of a request: `kind` (`session` | `api-key`), `tenantID`,
`actorID`, `tenantPermissions` (set), and per-list role references. Built by the
`AuthenticatePrincipal` query; carried in request context. Exposes
`Can(permission)` and `CanOnList(permission, listID)`.

### APIKey (aggregate)

Fields: `id`, `tenantID`, `name`, `tokenHash`, `permissions []Permission`
(a least-privilege subset), `createdBy`, `createdAt`, `lastUsedAt`,
`revokedAt`.

Invariants: the raw key is shown once, stored as a hash; `permissions` is a
subset of the catalogue; a revoked key (`revokedAt` set) authenticates nothing.

### RecoveryCode (entity)

Fields: `tenantID`, `userID`, `codeHash`, `usedAt`. Issued as a batch when TOTP
is enabled; single-use (`usedAt` set on consumption); stored hashed.

### AuditRecord (entity)

Fields: `id`, `tenantID`, `actorID`, `actorKind`, `action`, `target`,
`metadata` (`jsonb`), `createdAt`. Append-only; written for privileged actions
(role create/update/delete, role assignment/revocation, API key
issuance/revocation) — FR-028, SC-010.

## PostgreSQL schema

Reused unchanged: `tenants`, `platform_users`, `tenant_settings` (satisfies the
spec's `settings`). New migrations:

### Migration `000005_tenant_access_schema` — `iam` tables (all tenant-plane, RLS)

- `users` — `id`, `tenant_id`, `platform_user_id` (→ `platform_users`), `email`
  (`citext`), `name`, `status`, `totp_enabled`, `totp_secret` (encrypted bytes,
  nullable), timestamps. Unique `(tenant_id, platform_user_id)` and
  `(tenant_id, email)`.
- `sessions` — `id`, `tenant_id`, `user_id` (→ `users`), `token_hash` unique,
  `state`, `created_at`, `expires_at`, `revoked_at`.
- `roles` — `id`, `tenant_id`, `name`, `permissions text[]`, timestamps.
  Unique `(tenant_id, name)`.
- `user_roles` — `tenant_id`, `user_id`, `role_id`. PK `(tenant_id, user_id)`
  (one tenant-level role per user).
- `user_list_roles` — `tenant_id`, `user_id`, `list_id`, `role_id`.
  PK `(tenant_id, user_id, list_id)`. (FK to `lists` added with `000006`.)
- `api_keys` — `id`, `tenant_id`, `name`, `token_hash` unique, `permissions
  text[]`, `created_by`, `created_at`, `last_used_at`, `revoked_at`.
- `recovery_codes` — `tenant_id`, `user_id`, `code_hash`, `used_at`.
- `audit_log` — `id`, `tenant_id`, `actor_id`, `actor_kind`, `action`,
  `target`, `metadata jsonb`, `created_at`.

Each table: `tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE`,
`ENABLE` + `FORCE ROW LEVEL SECURITY`, and the `tenant_isolation` policy from
`000004`. `GRANT SELECT, INSERT, UPDATE, DELETE ... TO nvelope_app`.

### Migration `000006_audience_schema` — `audience` tables (all tenant-plane, RLS)

- `lists` — `id`, `tenant_id`, `name`, `description`, `visibility`, `optin`,
  `tags text[]`, timestamps. Unique `(tenant_id, name)`.
- `subscribers` — `id`, `tenant_id`, `email` (`citext`), `name`, `state`,
  `attributes jsonb NOT NULL DEFAULT '{}'`, timestamps. Unique
  `(tenant_id, email)`. GIN index on `attributes` for segment queries.
- `subscriber_lists` — `tenant_id`, `subscriber_id` (→ `subscribers`),
  `list_id` (→ `lists`), `subscription_status`, timestamps.
  PK `(subscriber_id, list_id)`. Both FKs `ON DELETE CASCADE`.

Same `tenant_id` + RLS + grant treatment. Adds the deferred FK from
`user_list_roles.list_id` → `lists(id)`.

### Migration `000007_import_export_jobs` — job tracking (tenant-plane, RLS)

- `import_export_jobs` — `id`, `tenant_id`, `kind` (`import` | `export`),
  `requested_by`, `status`, `params jsonb` (target lists / selection /
  segment), `file_name`, `file_bytes bytea` (staged upload or generated
  export — research.md Decision 2), `created_count`, `updated_count`,
  `failed_count`, `row_count`, `failures jsonb`, `created_at`, `started_at`,
  `finished_at`.

`tenant_id` + RLS + grant treatment.

### River queue tables

Installed by River's own migrator (`riverpgxv5`), invoked from `cmd/migrate`
after the `golang-migrate` steps. Not tenant-plane — River rows carry the tenant
id inside the job payload, and the import/export workers re-bind `app.tenant_id`
via `WithTenant` before touching any tenant-plane table.

## Entity → requirement traceability

| Entity / table | Requirements |
|---|---|
| List, `lists` | FR-005, FR-011, FR-012 |
| Subscriber, `subscribers` | FR-006, FR-007, FR-009, FR-010, FR-012 |
| Membership, `subscriber_lists` | FR-008, FR-009, FR-011 |
| Segment | FR-013, FR-014, FR-015, FR-016 |
| ImportJob / ExportJob, `import_export_jobs` | FR-017–FR-021 |
| TenantUser, `users` | FR-001, FR-029, FR-033, FR-034 |
| Session, `sessions` | FR-001, FR-029, FR-034 |
| Role / Permission, `roles`, `user_roles`, `user_list_roles` | FR-022–FR-027 |
| Principal | FR-024, FR-025 |
| APIKey, `api_keys` | FR-030, FR-031, FR-032 |
| RecoveryCode, `recovery_codes` | FR-035 |
| AuditRecord, `audit_log` | FR-028 |
| `tenant_settings` (reused) | FR-004 |
