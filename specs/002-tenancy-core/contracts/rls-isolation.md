# Contract: Row-Level Security Isolation

The internal contract that guarantees tenant data isolation at the storage layer. It binds the
database role, the RLS policies, and the per-request transaction helper together. Spec
FR-014–FR-019 and Constitution Principle I depend on this contract holding.

## Database role contract

- The application connects at runtime as the role `nvelope_app`.
- `nvelope_app` is **not** a superuser, **not** `BYPASSRLS`, and **not** the owner of any
  tenant-plane table.
- `nvelope_app` is granted only `SELECT, INSERT, UPDATE, DELETE` on the tables it needs — no
  DDL, no `CREATE ROLE`.
- Schema migrations run as a separate privileged role via `NVELOPE_MIGRATE_DATABASE_URL`.

Because `nvelope_app` holds none of the RLS-bypassing attributes, every policy below applies to
it unconditionally.

## Tenant-plane table contract

Every tenant-plane table (Phase 1: `tenant_settings`; Phase 2+: `subscribers`, `lists`, …):

1. Has a non-null `tenant_id uuid` column.
2. Has `ROW LEVEL SECURITY` both `ENABLE`d and `FORCE`d (so even the table owner is subject).
3. Has one policy applying to **all** commands:

```sql
USING      (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid)
WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid)
```

- `USING` filters every `SELECT`, `UPDATE`, and `DELETE` — rows of other tenants are invisible.
- `WITH CHECK` rejects any `INSERT`/`UPDATE` that would create or move a row into a different
  tenant.
- When `app.tenant_id` is unset, `current_setting(..., true)` yields `NULL` and the predicate
  is `NULL`: **no rows match** — fail-closed, never fail-open.

## Per-request binding contract

Tenant-plane access goes through one helper:

```go
func WithTenant(ctx context.Context, pool *pgxpool.Pool, tenantID uuid.UUID,
    fn func(ctx context.Context, tx pgx.Tx) error) error
```

`WithTenant`:

1. Begins a transaction.
2. Runs `SELECT set_config('app.tenant_id', $1, true)` with `tenantID` as a bound parameter
   — `true` makes the setting transaction-local, so it cannot leak across pooled connections.
3. Invokes `fn` with the bound transaction.
4. Commits if `fn` returns `nil`; rolls back otherwise.

Rules:

- Tenant-plane reads and writes **must** happen inside a `WithTenant` callback. A query against
  a tenant-plane table outside one sees zero rows (the GUC is unset) — fail-closed.
- The `tenant_id` passed to `WithTenant` is always the value resolved and cross-checked by the
  tenant resolution middleware (`tenant-api.md`), never a client-supplied value.
- Control-plane access (`platform_users`, `tenants`, …) uses the pool directly; those tables
  have no RLS and need no binding.

## Isolation guarantees (verified by automated tests)

With two tenants A and B each owning a `tenant_settings` row, while bound to tenant A:

| Operation | Required result |
|---|---|
| `SELECT * FROM tenant_settings` (no `WHERE`) | Returns only A's row |
| `UPDATE tenant_settings SET ...` (no `WHERE`) | Affects only A's row; B unchanged |
| `DELETE FROM tenant_settings` (no `WHERE`) | Deletes only A's row; B's row remains |
| `INSERT INTO tenant_settings (tenant_id, ...) VALUES (B, ...)` | Rejected by `WITH CHECK` |
| `SELECT * FROM tenant_settings` outside `WithTenant` | Returns zero rows |

The test suite issues these statements **with the application-level `tenant_id` filter
deliberately omitted** and asserts the database alone confines every operation to tenant A
(FR-016, FR-017, FR-019). The suite connects as `nvelope_app` against a real PostgreSQL
instance — never a mock (Constitution Principle II).
