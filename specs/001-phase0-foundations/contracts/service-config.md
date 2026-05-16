# Contract: Service Configuration & CLI

Phase 0 exposes two non-network interfaces to operators and developers: the environment
configuration each service consumes, and the migration CLI.

## Configuration interface (all three services)

Configuration is read from environment variables prefixed `NVELOPE_`, with an optional
`.env` file layered underneath. `.env.example` documents every key.

| Variable | Required | Default | Consumed by | Notes |
| --- | --- | --- | --- | --- |
| `NVELOPE_DATABASE_URL` | yes | — | all | PostgreSQL DSN; non-superuser role. Secret — never logged. |
| `NVELOPE_LOG_LEVEL` | no | `info` | all | One of `debug`, `warn`, `info`, `error`. Invalid value → startup fails. |
| `NVELOPE_HTTP_ADDR` | no | `:8080` | api | Listen address for the API service. |
| `NVELOPE_SHUTDOWN_TIMEOUT` | no | `10s` | all | Graceful-drain bound (Go duration string). |

### Behavior contract

- On startup each service loads config, then calls `Validate()`.
- If a required variable is missing or any variable is malformed, the service MUST write a
  structured error log naming each offending variable and exit with a non-zero status,
  before starting any work (FR-004, SC-003).
- Secret values (`NVELOPE_DATABASE_URL`) MUST NOT appear in any log line, including
  errors — errors reference the variable name only.
- Valid configuration produces a successful startup with no further config interaction.

## Migration CLI (`cmd/migrate`)

A first-party binary wrapping `golang-migrate`. Reads `NVELOPE_DATABASE_URL`.

| Command | Effect | Exit code |
| --- | --- | --- |
| `migrate up` | Applies all pending migrations. | `0` on success or nothing pending; non-zero on failure. |
| `migrate down` | Reverts the most recently applied migration. | `0` on success; non-zero on failure. |
| `migrate version` | Prints the current schema version and dirty state. | `0` always when reachable; non-zero if the database is unreachable. |
| `migrate create <name>` | Generates a sequentially numbered `up`/`down` SQL file pair in `internal/db/migrations`. | `0` on success. |

### Behavior contract

- `up` against an already-current database MUST make no changes and exit `0`.
- A failed or interrupted migration leaves `schema_migrations.dirty = true`; subsequent
  `up`/`down` MUST fail clearly until the dirty state is resolved.
- An unreachable or unauthorized database MUST produce a clear connection/permission
  error and leave the schema untouched.
- The same binary is reused as the pre-deploy migration job in `deploy/`.
