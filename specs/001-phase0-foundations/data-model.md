# Phase 0 Data Model: Foundations & Docs

Phase 0 introduces **no application or tenant data schema**. It establishes only the
migration mechanism and one baseline migration. The conceptual entities from the spec are
configuration- and process-level constructs, not database tables; they are modelled here
as code structures.

## Database objects

### `schema_migrations` (managed by `golang-migrate`)

Created automatically by `golang-migrate` on first run. Not authored by hand.

| Column | Type | Purpose |
| --- | --- | --- |
| `version` | `bigint` | Current applied migration version. |
| `dirty` | `boolean` | True if a migration failed partway; blocks further migration until resolved. |

**State transitions**: empty database → after `000001` applied: `version = 1, dirty = false`
→ after `down`: row reset to `version = 0`. A failed apply sets `dirty = true`, which the
`cmd/migrate` tool surfaces as an error requiring manual `force`.

### Baseline migration `000001_baseline`

- **Up**: `CREATE EXTENSION IF NOT EXISTS pgcrypto;`
- **Down**: `DROP EXTENSION IF EXISTS pgcrypto;`

Provides `gen_random_uuid()` for the UUID primary keys every later-phase table uses. No
tables, no tenant data.

**Validation rule**: applying then reverting `000001` MUST return the database to its
original state (no `pgcrypto`, `version = 0`) — verified by `test/migrate_test.go`.

## Code-level entities

These correspond to the spec's Key Entities and live as Go types / project artifacts.

### `Config` (`internal/config`)

Typed struct decoded from environment (prefix `NVELOPE_`) and optional file.

| Field | Type | Required | Notes |
| --- | --- | --- | --- |
| `Service` | `string` | derived | One of `api`, `worker`, `scheduler`; set by the entrypoint, not user config. |
| `DatabaseURL` | `string` | yes | PostgreSQL DSN; connects as a non-superuser role. Secret — never logged. |
| `LogLevel` | `string` | no | `debug`/`info`/`warn`/`error`; defaults to `info`. |
| `HTTPAddr` | `string` | api only | Listen address for the API service; defaults to `:8080`. |
| `ShutdownTimeout` | `duration` | no | Graceful-drain bound; defaults to `10s`. |

**Validation rules**: `Validate()` returns an aggregated error naming every missing or
malformed required field. An unknown `LogLevel` value is rejected. Validation runs before
any service work begins.

### `Service` lifecycle (`internal/service`)

Not persisted. States: `starting` → `ready` → `draining` → `stopped`. A required-config
failure terminates in `starting` with a non-zero exit. `SIGINT`/`SIGTERM` moves `ready` →
`draining`; the drain context is bounded by `Config.ShutdownTimeout`.

### Migration file pair

A `NNNNNN_name.up.sql` / `NNNNNN_name.down.sql` pair in `internal/db/migrations`. The
`cmd/migrate create` command generates correctly named, sequentially numbered empty pairs.
Ordering is by the zero-padded numeric prefix.

### CI Pipeline Run

Not persisted in the application. A GitHub Actions workflow run with steps `build`,
`test`, `lint` (backend and frontend) plus a `migrate` step; aggregate outcome is
pass/fail and reported on the change.

### Container Image / Deployment Manifest

Build artifacts, not data. One image per backend service from `deploy/docker/*.Dockerfile`;
each referenced by exactly one `Deployment` in `deploy/k8s/` and by the Helm chart's
templated deployments.
