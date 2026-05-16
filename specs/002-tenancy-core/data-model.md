# Phase 1 Data Model: Tenancy Core

All Phase 1 tables are added by versioned migrations after the Phase 0 baseline. UUID primary
keys use `gen_random_uuid()` (pgcrypto, enabled in `000001`). Case-insensitive text uses
`citext` (enabled in `000002`).

Two planes:

- **Control plane** — no RLS: `platform_users`, `platform_sessions`, `tenants`,
  `platform_user_tenants`, `invitations`.
- **Tenant plane** — RLS + `FORCE`, non-null `tenant_id`: `tenant_settings`.

## Migrations

| Version | Name | Contents |
|---|---|---|
| `000002` | `app_role_and_extensions` | `CREATE EXTENSION citext`; create the restricted `nvelope_app` role |
| `000003` | `control_plane` | `platform_users`, `platform_sessions`, `tenants`, `platform_user_tenants`, `invitations`; grants to `nvelope_app` |
| `000004` | `tenant_settings_rls` | `tenant_settings`; `ENABLE`/`FORCE` RLS; isolation policy; grants to `nvelope_app` |

Each migration ships a paired `.down.sql` that reverts its schema. One
deliberate exception: `000002`'s down drops only the `citext` extension and
leaves the `nvelope_app` role in place — a role is a cluster-global object, not
per-database schema, and may be shared by other databases. The role is created
with `IF NOT EXISTS`, so re-applying is safe.

## Control-plane entities

### platform_users

An individual identity that can authenticate. Exists independently of any tenant.

| Column | Type | Notes |
|---|---|---|
| `id` | `uuid` | PK, `default gen_random_uuid()` |
| `email` | `citext` | **unique**, not null, validated shape |
| `password_hash` | `text` | not null, bcrypt |
| `name` | `text` | not null |
| `created_at` | `timestamptz` | not null, `default now()` |
| `updated_at` | `timestamptz` | not null, `default now()` |

Rules: email unique case-insensitively (FR-002); password ≥ 8 chars before hashing.

### platform_sessions

A live authenticated context for a platform user.

| Column | Type | Notes |
|---|---|---|
| `id` | `uuid` | PK, `default gen_random_uuid()` |
| `platform_user_id` | `uuid` | not null, FK → `platform_users(id)` `ON DELETE CASCADE` |
| `token_hash` | `text` | **unique**, not null — SHA-256 of the cookie token |
| `created_at` | `timestamptz` | not null, `default now()` |
| `expires_at` | `timestamptz` | not null |
| `revoked_at` | `timestamptz` | nullable |

A session is **valid** when `revoked_at IS NULL AND expires_at > now()`. Index on
`platform_user_id` for revoke-all-by-user.

### tenants

An isolated workspace that owns all business data created within it.

| Column | Type | Notes |
|---|---|---|
| `id` | `uuid` | PK, `default gen_random_uuid()` |
| `slug` | `citext` | **unique**, not null, matches slug regex, not reserved |
| `name` | `text` | not null, non-empty |
| `status` | `text` | not null, `default 'active'`, `CHECK (status IN ('active','suspended','deleted'))` |
| `created_at` | `timestamptz` | not null, `default now()` |
| `updated_at` | `timestamptz` | not null, `default now()` |

Phase 1 only ever sets `status = 'active'`; `suspended`/`deleted` exist for later phases.

### platform_user_tenants

Membership — links a platform user to a tenant and conveys workspace access.

| Column | Type | Notes |
|---|---|---|
| `platform_user_id` | `uuid` | FK → `platform_users(id)` `ON DELETE CASCADE` |
| `tenant_id` | `uuid` | FK → `tenants(id)` `ON DELETE CASCADE` |
| `role` | `text` | not null, `default 'admin'`, `CHECK (role IN ('owner','admin'))` |
| `created_at` | `timestamptz` | not null, `default now()` |

PK = `(platform_user_id, tenant_id)` — a user has at most one membership per tenant (FR-010).
The tenant creator gets `role = 'owner'`; invited members get `role = 'admin'`. Phase 1 does
not branch behavior on `role` (any member may invite — spec assumption); the column exists for
the RBAC work in later phases. Index on `tenant_id` for member listing.

### invitations

A pending grant of membership in one tenant, addressed to an email.

| Column | Type | Notes |
|---|---|---|
| `id` | `uuid` | PK, `default gen_random_uuid()` |
| `tenant_id` | `uuid` | not null, FK → `tenants(id)` `ON DELETE CASCADE` |
| `email` | `citext` | not null — invited address |
| `role` | `text` | not null, `default 'admin'`, `CHECK (role IN ('owner','admin'))` |
| `token_hash` | `text` | **unique**, not null — SHA-256 of the invite token |
| `status` | `text` | not null, `default 'pending'`, `CHECK (status IN ('pending','accepted','revoked','expired'))` |
| `invited_by` | `uuid` | not null, FK → `platform_users(id)` |
| `accepted_by` | `uuid` | nullable, FK → `platform_users(id)` |
| `created_at` | `timestamptz` | not null, `default now()` |
| `expires_at` | `timestamptz` | not null |
| `accepted_at` | `timestamptz` | nullable |

Partial unique index `(tenant_id, email) WHERE status = 'pending'` — at most one open
invitation per email per tenant. Index on `tenant_id`.

**Status transitions**:

```
pending ──accept──▶ accepted     (sets accepted_by, accepted_at)
pending ──revoke──▶ revoked
pending ──expiry──▶ expired      (expires_at < now(); enforced on read/accept)
```

Only a `pending`, unexpired invitation can be accepted (FR-009). Accepting an invitation whose
email already has a membership in the tenant is a no-op that marks the invitation `accepted`
without creating a duplicate `platform_user_tenants` row (FR-010).

## Tenant-plane entities

### tenant_settings

Per-tenant settings — the first tenant-plane table and the anchor for the RLS pattern. Exactly
one row per tenant, created in the same transaction as the tenant.

| Column | Type | Notes |
|---|---|---|
| `tenant_id` | `uuid` | PK, FK → `tenants(id)` `ON DELETE CASCADE` — also the RLS key |
| `display_name` | `text` | not null — defaults to the tenant name |
| `timezone` | `text` | not null, `default 'UTC'` |
| `created_at` | `timestamptz` | not null, `default now()` |
| `updated_at` | `timestamptz` | not null, `default now()` |

**RLS** (migration `000004`):

```sql
ALTER TABLE tenant_settings ENABLE ROW LEVEL SECURITY;
ALTER TABLE tenant_settings FORCE  ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON tenant_settings
  USING      (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid)
  WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);

GRANT SELECT, INSERT, UPDATE, DELETE ON tenant_settings TO nvelope_app;
```

With `app.tenant_id` unset, the policy predicate is `NULL` → no rows visible or writable
(deny by default). Every Phase 2+ tenant-plane table repeats this pattern.

## Entity relationships

```
platform_users 1───* platform_sessions
platform_users *───* tenants            (via platform_user_tenants)
platform_users 1───* invitations        (invited_by; accepted_by nullable)
tenants        1───* invitations
tenants        1───1 tenant_settings
```

## Validation summary

| Rule | Where enforced |
|---|---|
| Email unique, case-insensitive | `citext` + unique index on `platform_users.email` |
| Email shape valid | Application, before insert |
| Password ≥ 8 chars | Application, before hashing |
| Slug regex + not reserved | Application, before insert |
| Slug unique, case-insensitive | `citext` + unique index on `tenants.slug` |
| One membership per (user, tenant) | PK on `platform_user_tenants` |
| One open invitation per (tenant, email) | Partial unique index on `invitations` |
| Tenant-plane row belongs to one tenant | non-null `tenant_id` + RLS `WITH CHECK` |
| Cross-tenant read/write blocked | RLS policy + non-`BYPASSRLS` `nvelope_app` role |
