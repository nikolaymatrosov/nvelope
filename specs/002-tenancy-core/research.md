# Phase 0 Research: Phase 1 — Tenancy Core

Resolves the open technical decisions behind the tenancy foundation. Each entry records the
decision, why it was chosen, and the alternatives rejected.

## 1. Row-Level Security enforcement model

**Decision**: Tenant-plane tables get `ENABLE ROW LEVEL SECURITY` **and** `FORCE ROW LEVEL
SECURITY`. Each table carries a non-null `tenant_id uuid` and one policy:

```sql
USING      (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid)
WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid)
```

The application connects as a dedicated `nvelope_app` role that is **not** a superuser, **not**
`BYPASSRLS`, and **not** the owner of the tables.

**Rationale**:

- `FORCE` makes the policy apply even to the table owner, so a misrun as the owner role still
  cannot leak data.
- `nullif(current_setting('app.tenant_id', true), '')` returns `NULL` when the GUC is unset
  (`missing_ok = true` avoids an error). A `NULL` comparison matches no rows — the default is
  **deny**, so a query issued outside a tenant-bound transaction sees nothing.
- `WITH CHECK` with the same predicate blocks writing or moving a row into another tenant.
- A non-`BYPASSRLS`, non-owner role is the authoritative backstop the constitution (Principle I)
  requires — isolation does not depend on every `WHERE tenant_id = …` being present.

**Alternatives rejected**:

- *Separate schema or database per tenant* — operationally heavy, breaks the architecture's
  shared-database decision, and does not scale to many small tenants.
- *Application-only filtering* — one missing clause leaks data; explicitly forbidden by the
  constitution.
- *RLS without `FORCE`* — the table owner bypasses policies; a migration/ops mistake could
  expose data.

## 2. Per-request tenant binding helper

**Decision**: A `tenant.WithTenant(ctx, pool, tenantID, fn)` helper opens a transaction, runs
`SELECT set_config('app.tenant_id', $1, true)`, invokes `fn` with the transaction, then commits
(or rolls back on error). The `true` third argument makes the setting **transaction-local**, so
it cannot leak to another request that reuses the pooled connection.

`set_config(..., true)` is used instead of `SET LOCAL app.tenant_id = '<uuid>'` because
`SET LOCAL` cannot take a bound parameter — building it by string concatenation would be an
injection risk. `set_config` accepts `$1` safely.

**Rationale**: transaction-local scope + parameterized binding is the safe, pooling-correct
pattern. Control-plane access (no tenant) uses the pool directly without binding.

**Alternatives rejected**:

- *Session-level `SET`* — leaks across pooled connections.
- *String-built `SET LOCAL`* — injection surface.

## 3. App role provisioning and migration connection

**Decision**: Two database connection strings.

- `NVELOPE_DATABASE_URL` — runtime connection, the restricted `nvelope_app` role. Used by all
  three services.
- `NVELOPE_MIGRATE_DATABASE_URL` — privileged connection (owner/admin role) used **only** by
  the `migrate` CLI. Falls back to `NVELOPE_DATABASE_URL` when unset, for dev convenience.

A migration creates the `nvelope_app` role idempotently (`DO $$ ... CREATE ROLE ... $$`) with
`LOGIN` and grants it DML on control-plane and tenant-plane tables. The role password is set
out of band: `ALTER ROLE` in `docker-compose`/CI bootstrap for dev, and by operations for
production. Migrations are owned by the privileged role.

**Rationale**: migrations need DDL and `CREATE ROLE`; the runtime must not. Keeping role
creation in a migration makes the isolation tests self-contained and reproducible in CI.

**Alternatives rejected**:

- *Single superuser connection everywhere* — superuser bypasses RLS; defeats the backstop.
- *Provision the role entirely outside migrations* — isolation tests could not stand up a
  correct database from scratch in CI.

## 4. Platform session mechanism

**Decision**: Server-side sessions in a control-plane `platform_sessions` table. On login the
server generates 32 bytes from `crypto/rand`, stores the SHA-256 hash of the token, and returns
the raw token in an `HttpOnly`, `Secure`, `SameSite=Lax` cookie. Each request looks the session
up by token hash and checks `expires_at`/`revoked_at`. Logout sets `revoked_at`. TTL is
configurable via `NVELOPE_SESSION_TTL` (default `168h`).

**Rationale**: server-side sessions are immediately revocable, keep the services stateless
(state lives in Postgres, satisfying Principle V), and avoid signing-key management. Storing
only the hash means a database leak does not yield usable tokens.

**Alternatives rejected**:

- *Stateless signed cookies / JWT* — cannot revoke before expiry; needs key rotation
  machinery; larger blast radius on key compromise.
- *In-memory sessions* — breaks horizontal scalability.

## 5. Password hashing

**Decision**: `bcrypt` via `golang.org/x/crypto/bcrypt` at cost 12.

**Rationale**: well-understood, self-contained (salt embedded in the hash), no separate tuning
parameters to mismanage, adequate for this platform's threat model.

**Alternatives rejected**: *argon2id* — stronger but adds three tuning knobs (memory, time,
parallelism) that must be chosen and kept consistent; unnecessary complexity for Phase 1.
*scrypt* — similar tradeoff with less ecosystem familiarity.

## 6. Tenant resolution and session cross-check

**Decision**: A chi middleware mounted on `/t/{slug}/...`:

1. Extracts `{slug}` and loads the tenant from the control plane.
2. Requires a valid platform session (the auth middleware runs first).
3. Verifies a `platform_user_tenants` row links the session's user to the resolved tenant.
4. On any failure — unknown slug, no session, or non-member — responds `404 Not Found` with a
   generic body, so the existence of a tenant is never revealed to a non-member.
5. On success, stores the resolved `tenant_id` in the request context; tenant-plane handlers
   read it and call `tenant.WithTenant`.

**Rationale**: directly implements spec FR-011/FR-012/FR-013. A uniform `404` for "unknown" and
"forbidden" prevents tenant enumeration.

**Alternatives rejected**: *`403` for non-members* — distinguishes "exists but forbidden" from
"does not exist", leaking tenant existence.

## 7. First tenant-plane table — `tenant_settings`

**Decision**: Phase 1 introduces exactly one tenant-plane table, `tenant_settings` (one row per
tenant, created with the tenant). It anchors the RLS pattern and gives the cross-tenant
isolation tests a real table to exercise.

**Rationale**: the isolation guarantee (Principle I, spec FR-016/FR-019) must be proven against
a genuine RLS-protected table, not a synthetic test fixture. Every tenant needs settings
eventually, so this is not speculative — it is the smallest honest anchor for the pattern.
Phase 2's tenant-plane tables (`subscribers`, `lists`, …) then follow the identical pattern.

**Alternatives rejected**: *test-only fixture table* — would not exercise the real migration
path; *defer all tenant-plane tables to Phase 2* — leaves the Phase 1 RLS pattern and isolation
tests with nothing real to validate.

## 8. Team-invite delivery

**Decision**: Inviting a teammate creates an `invitations` row with a random token (32 bytes,
base64url; only the SHA-256 hash is stored) and an expiry (`NVELOPE_INVITE_TTL`, default
`168h`). The acceptance link (`{NVELOPE_BASE_URL}/invite/{token}`) is returned to the inviter in
the API response and shown in the UI to copy and share. Automated email delivery is **deferred**
to a later phase.

**Rationale**: the email-sending pipeline (queue + Postbox messenger) does not exist until
Phase 3. Surfacing a copyable link satisfies the spec journey ("teammate receives an
invitation, accepts it") and the exit criterion without pulling Phase 3 scope forward (YAGNI,
Principle III). This refines the spec assumption "an email-delivery capability is assumed":
Phase 1 surfaces the link directly; wiring it to email lands with the sending pipeline.

**Alternatives rejected**: *build SMTP/Postbox sending now* — pulls Phase 3 scope into Phase 1.

## 9. Identifier and credential validation

**Decision**:

- **Email** — stored as `citext` (case-insensitive), validated against a basic RFC-5322-ish
  shape; unique across `platform_users`.
- **Tenant slug** — `citext`, must match `^[a-z0-9](?:[a-z0-9-]{1,61}[a-z0-9])$`, excludes a
  reserved list (`api`, `t`, `admin`, `login`, `signup`, `invite`, `static`, `healthz`); unique
  across `tenants`. Supplied by the creator or derived from the tenant name.
- **Password** — minimum 8 characters; no maximum below bcrypt's 72-byte input limit.

`citext` requires `CREATE EXTENSION citext`, added in a migration.

**Rationale**: case-insensitive email/slug uniqueness avoids duplicate-by-case accounts and
tenants; the reserved list keeps slugs from colliding with fixed path segments; the slug regex
keeps slugs safe inside a URL path.

**Alternatives rejected**: *`lower()` functional unique indexes* — works but spreads `lower()`
across every query; `citext` keeps call sites clean.

## 10. Account-enumeration posture

**Decision**: Login failures return a single generic message ("invalid email or password")
regardless of whether the email exists. Invitation-token lookups for an invalid/expired token
return a generic `404`. Signup with an already-registered email **does** return a specific
"email already registered" error.

**Rationale**: login and invite endpoints are the high-value enumeration targets and are kept
generic. Signup unavoidably reveals existence — a generic signup error would block legitimate
users from understanding why registration failed. This is the standard, accepted tradeoff and
satisfies spec FR-020 for the endpoints where it matters.

**Alternatives rejected**: *generic signup errors* — degrades usability for no real protection,
since an attacker can probe the same fact via the login endpoint's timing or the signup form's
own behavior.
