# Serverless On-Prem — Plan B4 (iam context on YDB) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:**
- Port the `iam` context to YDB: `YDBUsers`, `YDBSessions`, `YDBRoles`, `YDBAPIKeys`, `YDBRecoveryCodes` and `YDBAudit` behind the existing domain ports.
- Hold every new implementation to the same contract tests as its Postgres counterpart.

**Architecture:**
- **Migration.** One file, `internal/db/ydbmigrations/000005_iam.up.sql`, creates the eight iam tables: `users`, `sessions`, `roles`, `user_roles`, `user_list_roles`, `api_keys`, `recovery_codes` and `audit_log`. Every one is tenant-scoped.
- **Adapters.** The YDB adapters sit next to the pg adapters in `internal/iam/adapters` and call `db.Query()` / `DoTx` directly, like `internal/tenant/adapters/*_ydb.go`.
- **Contract tests.** The `*_pg_test.go` files become pg+ydb contract suites through an `eachBackend` harness, the same shape as auth's and tenant's.
  - No iam query reads a table another context owns. The YDB backend therefore uses a bare UUID for tenant, platform-user and list ids, as auth's `newUserID` does.
  - The pg backend keeps its raw-SQL seeders for those rows, because its foreign keys need them.
- **No foreign keys.** A role assignment reads the role inside the assigning `DoTx` and returns `ErrRoleNotFound` when it is missing. This stands in for the pg foreign key.
- **No arrays.** The `text[]` permission columns become a JSON array in a `Utf8` column.

**Tech Stack:**
- Go 1.26
- `github.com/ydb-platform/ydb-go-sdk/v3` v3.136.4 (query service)
- testcontainers-go, local-ydb `26.3.1.23`
- testify, `github.com/google/uuid`

**Design spec:** [docs/superpowers/specs/2026-11-14-serverless-onprem-design.md](../specs/2026-11-14-serverless-onprem-design.md).
- Binding sections:
  - "Storage layer (YDB)": Transactions; Schema and migrations; SQL translation rules, including the `text[]`, `citext` and foreign-key rows this plan adds.
  - "Spike findings (Plan A)": V4 and V6.
  - "Plan B2 planning spike", "Plan B3 planning spike" and "Plan B4 planning spike".
  - "Ceilings".
- The B4 spike verified every YQL construct this plan uses that B1–B3 did not:
  - `Bool` columns, and a nullable `String` column bound as `Optional<String>` and scanned into `*[]byte`;
  - two composite `GLOBAL UNIQUE SYNC` indexes on one table;
  - an `UPDATE` onto another row's unique-index value inside `DoTx`, which fails at the statement with `db.IsYDBConflict`;
  - `DELETE ... RETURNING` and `UPDATE ... WHERE <key> AND <guard> RETURNING`;
  - a two-column `JOIN`;
  - `COUNT(*)`, and `LIMIT $limit OFFSET $offset` with `Uint64` parameters;
  - a key-prefix `DELETE` followed by an `INSERT` of a just-deleted key in one `DoTx`.
- **Dry run.** During planning, every code block in this plan was applied task by task to the worktree, then reverted.
  - After each task, `go vet`, `golangci-lint run ./internal/iam/...` and that task's pg+ydb suites passed.
  - On the final state:
    - `go build ./... && go vet ./... && go test ./...` had no failing package;
    - `golangci-lint run` reported `0 issues.`;
    - `make lint-arch` reported `[arch] clean`.

**Scope:**
- This is the fourth Plan B plan. The port order is auth, tenant, iam, audience, campaign, deliverability, media.
- iam fits one plan: six adapters over about 850 lines of pg code, in three tasks.
- Not in scope:
  - Wiring the YDB adapters into the composition root (`internal/service/application.go:242` and `:607-611`). That is Plan C.
  - `internal/iam/adapters/totp.go`, which stores nothing.
  - The iam app and domain layers, which are unchanged.
  - The pg adapters. They are unchanged except for the package doc comment in `roles_pg.go`.

## Global Constraints

- **Branch.**
  - Work on `feat/serverless-onprem-b4` in the worktree `/Users/nikthespirit/experiments/nvelope/.claude/worktrees/serverless-onprem-b4`. Both already exist.
  - Do not create branches or worktrees.
  - The main checkout belongs to other sessions.
  - Never use bare `git stash` or `git stash pop`; the stash is shared.
- **Docker.**
  - Docker on this machine is OrbStack. Prefix every `go test` / `make test` / `make ci` with `export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock" && `.
  - Ryuk reaps test containers shortly after a run ends. A run that starts within that window reattaches to the previous run's containers.
  - The shared containers `nvelope-test-pg` and `nvelope-test-ydb` may belong to another session's live run. Never `docker rm` them.
- **YDB container.** It is `ydbplatform/local-ydb:26.3.1.23`, amd64 under Rosetta. A fresh container can take about 45 s to start, so use `-timeout 8m` on runs that touch YDB, and `-timeout 15m` for the full suite.
- **Known environmental flakes. None of these is caused by this plan:**
  - `internal/platform/ratelimit` `TestAllowIsConsistentUnderConcurrency` can fail with `redis eval failed: i/o timeout` under full-suite load. Re-run that package alone and record both runs.
  - pg-backed packages can all fail at once with `no migration found for version N`, where N is above `23`, or with `unexpected postmaster exit`. That means another worktree's run is using the shared `nvelope-test-pg`. Re-run once it is gone.
  - A YDB-backed test can fail with `No such container: <id>`, or hang until the timeout, when it starts while Ryuk is still reaping the previous run's YDB container. Re-run once.
  - **Until Task 3**, the pg `TestAPIKeysAddByTokenHashRevoke` and `TestAPIKeysAll` use fixed token hashes. A run that reattaches to the previous run's pg container fails them with `duplicate key value violates unique constraint "api_keys_token_hash_key" (SQLSTATE 23505)`. Wait a minute and re-run. Task 3 randomizes the hashes.
- **Dependencies.** No new Go modules and no `go.mod` change.
- **SaaS must not change.** Postgres adapters, River, Redis and every existing test keep their behaviour. `go test ./...` passes after every task.
- **YQL conventions** (spec "SQL translation rules" and V4):
  - Every query `DECLARE`s each parameter: `DECLARE $id AS Utf8;`.
  - A nullable parameter is declared `Optional<...>` and bound with `BeginOptional()...EndOptional()` from a pointer (`nil` is NULL).
  - Literals that target a `Utf8` column take the `u` suffix (`"active"u`); a bare `"..."` is `String`.
  - Every DDL statement is idempotent (`CREATE TABLE IF NOT EXISTS`).
  - A statement ends on a line ending in `;`. Never end a line with `;` inside a string literal or a `--` comment.
  - A secondary index is read only through an explicit `VIEW <index_name>`.
  - In a `JOIN`, every column is qualified with its table alias (`r.permissions`, `ulr.list_id`).
- **Tenant scoping.**
  - All eight iam tables are tenant-scoped. Their primary key starts with `tenant_id`, and every query filters by the `tenantID` the port receives.
  - `Sessions.ByTokenHash` and `APIKeys.ByTokenHash` read through a global unique `token_hash` index. They still carry the `tenant_id` predicate, because their ports receive the tenant and Postgres RLS filters by it.
- **IDs** are UUIDv4 strings generated in Go (`uuid.NewString()`) before any `DoTx`, and stored as `Utf8`.
- **No column defaults.** The YDB columns have no `DEFAULT`. Every `INSERT` writes every `NOT NULL` column from the domain value. Only `created_at` and `updated_at` come from `CurrentUtcTimestamp()`.
- **Case-insensitive matching** (Postgres `citext`):
  - iam `users.email` has **no** `email_key` column, unlike `platform_users` and `invitations`. `domain.NewTenantUser` lower-cases the email, and no other code writes the column, so the unique index on `(tenant_id, email)` reproduces `citext`. This follows `tenants.slug` (spec "Plan B4 planning spike").
  - Role names are case-sensitive, as the Postgres `text` column is.
- **Permissions** (`roles.permissions`, `api_keys.permissions`) are a JSON array in a `Utf8` column. They are written by `ydbPermissions` and read by `ydbPermissionsFrom`, which reuse `permStrings` and `permsFromStrings` from `roles_pg.go`.
- **Foreign keys.** YDB has none.
  - `AssignTenantRole` and `AssignListRole` read the role in the same `DoTx` and return `domain.ErrRoleNotFound` when it is missing.
  - The user and the list are not checked: users are never deleted, and lists belong to audience.
- **`ON CONFLICT DO UPDATE`** on `user_roles` and `user_list_roles` becomes `UPSERT`. Overwriting the previous role is the intent.
- **`DoTx` closures must be retry-safe** (V6):
  - only database operations inside, plus the port's `fn` callbacks;
  - ids, tokens, hashes and domain values are built before `DoTx`;
  - values the caller reads are assigned at the end of the closure;
  - never `append` to a slice, or write to a map, declared outside the closure.
- **Duplicate key mapping.**
  - "Already exists" comes from `db.IsYDBConflict(err)`. Never match on the message text.
  - Inside a `DoTx`, check it on the statement and also on the error `DoTx` returns, for a YDB that reports the conflict at commit.
  - Only roles map a conflict to a domain error (`ErrRoleNameTaken`), as on Postgres. A duplicate user, session, API key or recovery code returns the wrapped error, as on Postgres.
  - Never add `query.WithIdempotent()` to an insert path.
- **Commits.** Use conventional messages. **No `Co-Authored-By` trailer** of any kind.
- **Tooling.**
  - `goimports` is not installed, so use `gofmt -w`.
  - `golangci-lint` is on PATH. It lints test files too.
  - Run arch lint as `PATH="$HOME/go/bin:$PATH" make lint-arch`, with PATH set inline. A worktree guard refuses `export PATH` combined with git commands, and it refuses complex compound git commands. Run git commands one per line, as the commit steps show.

## File structure

| File | Responsibility |
|------|----------------|
| `internal/db/ydbmigrations/000005_iam.up.sql` | the eight iam tables |
| `internal/iam/adapters/users_ydb.go` | `YDBUsers`, the `ydbTenantParam` / `ydbTenantAndID` binders, `ydbOptionalBytes` |
| `internal/iam/adapters/sessions_ydb.go` | `YDBSessions` |
| `internal/iam/adapters/audit_ydb.go` | `YDBAudit` |
| `internal/iam/adapters/roles_ydb.go` | `YDBRoles`, `ydbRoleExists`, the `ydbPermissions` / `ydbPermissionsFrom` codec |
| `internal/iam/adapters/apikeys_ydb.go` | `YDBAPIKeys` |
| `internal/iam/adapters/recovery_codes_ydb.go` | `YDBRecoveryCodes`, the `ydbTenantAndUser` binder |
| `internal/iam/adapters/roles_pg.go` | package doc mentions the YDB implementations |
| `internal/iam/adapters/helpers_test.go` (rewritten in each task) | `backend`, `backends`, `eachBackend`, `newTenantUser`, the pg seeders |
| `internal/iam/adapters/users_test.go` (renamed from `users_pg_test.go`) | users contract suite |
| `internal/iam/adapters/sessions_test.go` (renamed from `sessions_pg_test.go`) | sessions contract suite |
| `internal/iam/adapters/audit_test.go` (new; its test moves out of `sessions_pg_test.go`) | audit contract suite |
| `internal/iam/adapters/roles_test.go` (renamed from `roles_pg_test.go`) | roles contract suite |
| `internal/iam/adapters/apikeys_test.go` (renamed from `apikeys_pg_test.go`) | API keys contract suite |
| `internal/iam/adapters/recovery_codes_test.go` (renamed from `recovery_codes_pg_test.go`) | recovery codes contract suite |

---

### Task 1: The iam schema, `YDBUsers`, `YDBSessions`, `YDBAudit` and their contract suites

**Files:**
- Create: `internal/db/ydbmigrations/000005_iam.up.sql`
- Create: `internal/iam/adapters/users_ydb.go`
- Create: `internal/iam/adapters/sessions_ydb.go`
- Create: `internal/iam/adapters/audit_ydb.go`
- Modify: `internal/iam/adapters/roles_pg.go:1-3` (package doc only)
- Modify: `internal/iam/adapters/helpers_test.go` (rewritten)
- Rename: `internal/iam/adapters/users_pg_test.go` → `internal/iam/adapters/users_test.go` (rewritten)
- Rename: `internal/iam/adapters/sessions_pg_test.go` → `internal/iam/adapters/sessions_test.go` (rewritten; its audit test moves to `audit_test.go`)
- Create: `internal/iam/adapters/audit_test.go`

**Interfaces:**
- Consumes:
  - `dbtest.YDBMigrated(t) *ydb.Driver` (`internal/dbtest/ydb.go`). It applies every `internal/db/ydbmigrations/*.up.sql` once per test binary, so the new file is picked up automatically.
  - `dbtest.AppPool(t) *pgxpool.Pool` and `dbtest.RandString() string` (`internal/dbtest`).
  - The ports `domain.UserRepository`, `domain.SessionRepository` and `domain.AuditRepository` (`internal/iam/domain/repository.go`), unchanged.
  - The hydrators `domain.HydrateTenantUser` and `domain.HydrateSession`, and the `domain.AuditRecord` struct (`internal/iam/domain`), unchanged.
- Produces:
  ```go
  // package adapters (internal/iam/adapters)
  type YDBUsers struct{ /* unexported */ }
  func NewYDBUsers(d *ydb.Driver) *YDBUsers
  type YDBSessions struct{ /* unexported */ }
  func NewYDBSessions(d *ydb.Driver) *YDBSessions
  type YDBAudit struct{ /* unexported */ }
  func NewYDBAudit(d *ydb.Driver) *YDBAudit
  func ydbTenantParam(tenantID string) query.ExecuteOption       // binds $tenant_id
  func ydbTenantAndID(tenantID, id string) query.ExecuteOption   // binds $tenant_id, $id
  // package adapters_test
  type backend struct {
      name     string
      users    func(t *testing.T) domain.UserRepository
      sessions func(t *testing.T) domain.SessionRepository
      audit    func(t *testing.T) domain.AuditRepository
      newTenant       func(t *testing.T) string
      newPlatformUser func(t *testing.T) string
  }
  func eachBackend(t *testing.T, body func(t *testing.T, b backend))
  func newTenantUser(t *testing.T, b backend, tenantID string) string
  func seedList(t *testing.T, pool *pgxpool.Pool, tenantID string) string       // kept; Task 2's pg newList wraps it
  func seedTenant(t *testing.T, pool *pgxpool.Pool) string
  func seedPlatformUser(t *testing.T, pool *pgxpool.Pool) string
  func seedTenantUser(t *testing.T, pool *pgxpool.Pool, tenantID string) string // kept for the pg-only suites; Task 3 deletes it
  ```
- Task 2 relies on:
  - the `roles`, `user_roles` and `user_list_roles` tables and the `idx_roles_tenant_name` index;
  - `ydbTenantParam` and `ydbTenantAndID`;
  - the harness above.
- Task 3 relies on the `api_keys` and `recovery_codes` tables, the `idx_api_keys_token_hash` index, the same binders and the harness.

- [ ] **Step 1: Write the iam migration**

Create `internal/db/ydbmigrations/000005_iam.up.sql`:

```sql
-- IAM (internal/iam): tenant-plane users, working sessions, roles, role
-- assignments, API keys, TOTP recovery codes and the audit log. Every table is
-- tenant-scoped: its primary key starts with tenant_id and every query filters
-- by it. No column has a DEFAULT, so the adapters write the values Postgres
-- takes from its column defaults. YDB has no foreign keys, and no iam query
-- reads a table another context owns.

-- email is stored lower-cased (domain.NewTenantUser lower-cases it), so the
-- unique index reproduces the uniqueness of the Postgres citext column.
CREATE TABLE IF NOT EXISTS users (
    tenant_id        Utf8      NOT NULL,
    id               Utf8      NOT NULL,
    platform_user_id Utf8      NOT NULL,
    email            Utf8      NOT NULL,
    name             Utf8      NOT NULL,
    status           Utf8      NOT NULL,
    totp_enabled     Bool      NOT NULL,
    totp_secret      String,
    created_at       Timestamp NOT NULL,
    updated_at       Timestamp NOT NULL,
    PRIMARY KEY (tenant_id, id),
    INDEX idx_users_platform_user GLOBAL UNIQUE SYNC ON (tenant_id, platform_user_id),
    INDEX idx_users_email GLOBAL UNIQUE SYNC ON (tenant_id, email)
);

-- A session is looked up by its token hash, which is globally unique. TTL
-- deletes a session once it has expired. An expired session authenticates
-- nothing (Session.IsActive), so no caller needs to read one back.
CREATE TABLE IF NOT EXISTS sessions (
    tenant_id  Utf8      NOT NULL,
    id         Utf8      NOT NULL,
    user_id    Utf8      NOT NULL,
    token_hash Utf8      NOT NULL,
    state      Utf8      NOT NULL,
    created_at Timestamp NOT NULL,
    expires_at Timestamp NOT NULL,
    revoked_at Timestamp,
    PRIMARY KEY (tenant_id, id),
    INDEX idx_sessions_token_hash GLOBAL UNIQUE SYNC ON (token_hash)
) WITH (TTL = Interval("PT0S") ON expires_at);

-- permissions is a JSON array of permission strings, only stored and read back
-- whole, so it is a plain Utf8 column (Postgres: text[]). Role names are
-- case-sensitive, as on Postgres.
CREATE TABLE IF NOT EXISTS roles (
    tenant_id   Utf8      NOT NULL,
    id          Utf8      NOT NULL,
    name        Utf8      NOT NULL,
    permissions Utf8      NOT NULL,
    created_at  Timestamp NOT NULL,
    updated_at  Timestamp NOT NULL,
    PRIMARY KEY (tenant_id, id),
    INDEX idx_roles_tenant_name GLOBAL UNIQUE SYNC ON (tenant_id, name)
);

-- At most one tenant-level role per user.
CREATE TABLE IF NOT EXISTS user_roles (
    tenant_id Utf8 NOT NULL,
    user_id   Utf8 NOT NULL,
    role_id   Utf8 NOT NULL,
    PRIMARY KEY (tenant_id, user_id)
);

-- At most one role per user and list.
CREATE TABLE IF NOT EXISTS user_list_roles (
    tenant_id Utf8 NOT NULL,
    user_id   Utf8 NOT NULL,
    list_id   Utf8 NOT NULL,
    role_id   Utf8 NOT NULL,
    PRIMARY KEY (tenant_id, user_id, list_id)
);

-- An API key is looked up by its token hash, which is globally unique.
CREATE TABLE IF NOT EXISTS api_keys (
    tenant_id    Utf8      NOT NULL,
    id           Utf8      NOT NULL,
    name         Utf8      NOT NULL,
    token_hash   Utf8      NOT NULL,
    permissions  Utf8      NOT NULL,
    created_by   Utf8      NOT NULL,
    created_at   Timestamp NOT NULL,
    last_used_at Timestamp,
    revoked_at   Timestamp,
    PRIMARY KEY (tenant_id, id),
    INDEX idx_api_keys_token_hash GLOBAL UNIQUE SYNC ON (token_hash)
);

CREATE TABLE IF NOT EXISTS recovery_codes (
    tenant_id Utf8      NOT NULL,
    user_id   Utf8      NOT NULL,
    code_hash Utf8      NOT NULL,
    used_at   Timestamp,
    PRIMARY KEY (tenant_id, user_id, code_hash)
);

-- Keyed in the order AuditRepository.All pages through (Postgres: the
-- (tenant_id, created_at DESC) index). metadata is JSON that is only stored
-- and read back whole, so it is a plain Utf8 column. The deliverability
-- context writes to this table too.
CREATE TABLE IF NOT EXISTS audit_log (
    tenant_id  Utf8      NOT NULL,
    created_at Timestamp NOT NULL,
    id         Utf8      NOT NULL,
    actor_id   Utf8      NOT NULL,
    actor_kind Utf8      NOT NULL,
    action     Utf8      NOT NULL,
    target     Utf8      NOT NULL,
    metadata   Utf8      NOT NULL,
    PRIMARY KEY (tenant_id, created_at, id)
);
```

- [ ] **Step 2: Rewrite the harness**

Replace the contents of `internal/iam/adapters/helpers_test.go` with the code below.
- The four pg seeders are kept unchanged. The pg backend uses `seedTenant` and `seedPlatformUser`.
- `seedList` and `seedTenantUser` stay because `roles_pg_test.go`, `apikeys_pg_test.go` and `recovery_codes_pg_test.go` still call them. Tasks 2 and 3 convert those suites.

```go
package adapters_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
	"github.com/nikolaymatrosov/nvelope/internal/iam/adapters"
	"github.com/nikolaymatrosov/nvelope/internal/iam/domain"
	"github.com/nikolaymatrosov/nvelope/internal/platform/tenantdb"
)

// backend is one storage implementation of the iam repositories under
// contract test. Every contract test body runs once per backend, so the
// Postgres and YDB adapters are held to the same behaviour.
type backend struct {
	name     string
	users    func(t *testing.T) domain.UserRepository
	sessions func(t *testing.T) domain.SessionRepository
	audit    func(t *testing.T) domain.AuditRepository
	// newTenant and newPlatformUser return ids of rows other contexts own. On
	// Postgres the iam tables carry foreign keys to them, so the pg backend
	// inserts real rows. YDB has no foreign keys and no iam query reads those
	// tables, so a bare UUID stands in.
	newTenant       func(t *testing.T) string
	newPlatformUser func(t *testing.T) string
}

var backends = []backend{
	{
		name: "pg",
		users: func(t *testing.T) domain.UserRepository {
			return adapters.NewUsers(dbtest.AppPool(t))
		},
		sessions: func(t *testing.T) domain.SessionRepository {
			return adapters.NewSessions(dbtest.AppPool(t))
		},
		audit: func(t *testing.T) domain.AuditRepository {
			return adapters.NewAudit(dbtest.AppPool(t))
		},
		newTenant:       func(t *testing.T) string { return seedTenant(t, dbtest.AppPool(t)) },
		newPlatformUser: func(t *testing.T) string { return seedPlatformUser(t, dbtest.AppPool(t)) },
	},
	{
		name: "ydb",
		users: func(t *testing.T) domain.UserRepository {
			return adapters.NewYDBUsers(dbtest.YDBMigrated(t))
		},
		sessions: func(t *testing.T) domain.SessionRepository {
			return adapters.NewYDBSessions(dbtest.YDBMigrated(t))
		},
		audit: func(t *testing.T) domain.AuditRepository {
			return adapters.NewYDBAudit(dbtest.YDBMigrated(t))
		},
		newTenant:       func(*testing.T) string { return uuid.NewString() },
		newPlatformUser: func(*testing.T) string { return uuid.NewString() },
	},
}

// eachBackend runs body once per backend, as parallel subtests named after it.
func eachBackend(t *testing.T, body func(t *testing.T, b backend)) {
	t.Helper()
	for _, b := range backends {
		t.Run(b.name, func(t *testing.T) {
			t.Parallel()
			body(t, b)
		})
	}
}

// newTenantUser adds a tenant-plane user with a fresh address through the
// backend's user repository and returns its id.
func newTenantUser(t *testing.T, b backend, tenantID string) string {
	t.Helper()
	u, err := domain.NewTenantUser(tenantID, b.newPlatformUser(t), dbtest.RandString()+"@example.com", "Member")
	require.NoError(t, err)
	id, err := b.users(t).Add(context.Background(), tenantID, u)
	require.NoError(t, err)
	return id
}

// seedList inserts a list row inside a tenant-bound transaction and returns
// its id — needed to satisfy the user_list_roles foreign key.
func seedList(t *testing.T, pool *pgxpool.Pool, tenantID string) string {
	t.Helper()
	var id string
	require.NoError(t, tenantdb.WithTenant(context.Background(), pool, tenantID,
		func(ctx context.Context, tx pgx.Tx) error {
			return tx.QueryRow(ctx,
				"INSERT INTO lists (tenant_id, name) VALUES ($1, $2) RETURNING id",
				tenantID, "L-"+dbtest.RandString()).Scan(&id)
		}))
	return id
}

// seedTenant inserts a tenant row directly and returns its id.
func seedTenant(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var id string
	require.NoError(t, pool.QueryRow(context.Background(),
		`INSERT INTO tenants (name, slug, status) VALUES ('Workspace', $1, 'active') RETURNING id`,
		"iam-"+dbtest.RandString()).Scan(&id))
	return id
}

// seedPlatformUser inserts a control-plane user row directly and returns its id.
func seedPlatformUser(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var id string
	require.NoError(t, pool.QueryRow(context.Background(),
		`INSERT INTO platform_users (email, password_hash, name)
		 VALUES ($1, 'hash', 'User') RETURNING id`,
		dbtest.RandString()+"@example.com").Scan(&id))
	return id
}

// seedTenantUser inserts a tenant-plane user and returns its id.
func seedTenantUser(t *testing.T, pool *pgxpool.Pool, tenantID string) string {
	t.Helper()
	platformID := seedPlatformUser(t, pool)
	u, err := domain.NewTenantUser(tenantID, platformID, dbtest.RandString()+"@example.com", "Member")
	require.NoError(t, err)
	id, err := adapters.NewUsers(pool).Add(context.Background(), tenantID, u)
	require.NoError(t, err)
	return id
}
```

- [ ] **Step 3: Rewrite the users tests as a contract suite**

```bash
git mv internal/iam/adapters/users_pg_test.go internal/iam/adapters/users_test.go
```

Replace the contents of `internal/iam/adapters/users_test.go` with the code below. Three tests and several assertions are new:
- `TestUsersAddRejectsDuplicates` pins both Postgres unique constraints. Its upper-case email pins `citext`.
- `TestUsersUpdateRollsBackOnClosureError` and `TestUsersAreTenantScoped` are new.
- `TestUsersUpdateTOTP` now also disables TOTP, which must store NULL and read back a `nil` secret.
- The `"not-a-uuid"` lookup pins the pg `IsInvalidInput` → not-found mapping.

```go
package adapters_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/iam/domain"
)

func TestUsersAddGetByPlatformUser(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.users(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		platformID := b.newPlatformUser(t)

		u, err := domain.NewTenantUser(tenantID, platformID, "user@example.com", "Pat")
		require.NoError(t, err)
		id, err := repo.Add(ctx, tenantID, u)
		require.NoError(t, err)
		require.NotEmpty(t, id, "the adapter assigns an id")

		got, err := repo.Get(ctx, tenantID, id)
		require.NoError(t, err)
		require.Equal(t, id, got.ID())
		require.Equal(t, tenantID, got.TenantID())
		require.Equal(t, platformID, got.PlatformUserID())
		require.Equal(t, "user@example.com", got.Email())
		require.Equal(t, "Pat", got.Name())
		require.Equal(t, domain.UserActive, got.Status())
		require.False(t, got.TOTPEnabled())
		require.Nil(t, got.TOTPSecret())
		require.WithinDuration(t, time.Now(), got.CreatedAt(), time.Minute, "the database stamps the insert")

		byPlatform, err := repo.ByPlatformUser(ctx, tenantID, platformID)
		require.NoError(t, err)
		require.Equal(t, got, byPlatform)

		_, err = repo.ByPlatformUser(ctx, tenantID, "00000000-0000-0000-0000-000000000000")
		require.ErrorIs(t, err, domain.ErrUserNotFound)
		_, err = repo.Get(ctx, tenantID, "00000000-0000-0000-0000-000000000000")
		require.ErrorIs(t, err, domain.ErrUserNotFound)
		_, err = repo.Get(ctx, tenantID, "not-a-uuid")
		require.ErrorIs(t, err, domain.ErrUserNotFound, "a malformed id is an opaque not-found")
	})
}

// TestUsersAddRejectsDuplicates pins the two Postgres unique constraints:
// (tenant_id, platform_user_id) and (tenant_id, email), the email ignoring
// case as the citext column does.
func TestUsersAddRejectsDuplicates(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.users(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		platformID := b.newPlatformUser(t)

		first, err := domain.NewTenantUser(tenantID, platformID, "dup@example.com", "First")
		require.NoError(t, err)
		_, err = repo.Add(ctx, tenantID, first)
		require.NoError(t, err)

		samePlatformUser, err := domain.NewTenantUser(tenantID, platformID, "other@example.com", "Second")
		require.NoError(t, err)
		_, err = repo.Add(ctx, tenantID, samePlatformUser)
		require.Error(t, err, "a platform user has one tenant-plane user per tenant")

		sameEmail, err := domain.NewTenantUser(tenantID, b.newPlatformUser(t), "DUP@Example.com", "Third")
		require.NoError(t, err)
		_, err = repo.Add(ctx, tenantID, sameEmail)
		require.Error(t, err, "an email is unique within a tenant, ignoring case")

		otherTenant := b.newTenant(t)
		elsewhere, err := domain.NewTenantUser(otherTenant, platformID, "dup@example.com", "Elsewhere")
		require.NoError(t, err)
		_, err = repo.Add(ctx, otherTenant, elsewhere)
		require.NoError(t, err, "the same platform user and email may join another tenant")
	})
}

func TestUsersUpdateTOTP(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.users(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		u, err := domain.NewTenantUser(tenantID, b.newPlatformUser(t), "totp@example.com", "Pat")
		require.NoError(t, err)
		id, err := repo.Add(ctx, tenantID, u)
		require.NoError(t, err)

		require.NoError(t, repo.Update(ctx, tenantID, id, func(u *domain.TenantUser) (*domain.TenantUser, error) {
			return u, u.EnableTOTP([]byte("encrypted-secret"))
		}))
		got, err := repo.Get(ctx, tenantID, id)
		require.NoError(t, err)
		require.True(t, got.TOTPEnabled())
		require.Equal(t, []byte("encrypted-secret"), got.TOTPSecret())

		require.NoError(t, repo.Update(ctx, tenantID, id, func(u *domain.TenantUser) (*domain.TenantUser, error) {
			u.DisableTOTP()
			u.Suspend()
			return u, nil
		}))
		got, err = repo.Get(ctx, tenantID, id)
		require.NoError(t, err)
		require.False(t, got.TOTPEnabled())
		require.Nil(t, got.TOTPSecret(), "disabling TOTP clears the stored secret")
		require.Equal(t, domain.UserSuspended, got.Status())
	})
}

func TestUsersUpdateRollsBackOnClosureError(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.users(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		id := newTenantUser(t, b, tenantID)

		sentinel := domain.ErrRoleNotFound
		err := repo.Update(ctx, tenantID, id, func(u *domain.TenantUser) (*domain.TenantUser, error) {
			u.Suspend()
			return nil, sentinel
		})
		require.ErrorIs(t, err, sentinel)

		got, err := repo.Get(ctx, tenantID, id)
		require.NoError(t, err)
		require.Equal(t, domain.UserActive, got.Status(), "a closure error rolls the transaction back")
	})
}

func TestUsersAreTenantScoped(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.users(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		otherTenant := b.newTenant(t)
		platformID := b.newPlatformUser(t)

		u, err := domain.NewTenantUser(tenantID, platformID, "scoped@example.com", "Pat")
		require.NoError(t, err)
		id, err := repo.Add(ctx, tenantID, u)
		require.NoError(t, err)

		_, err = repo.Get(ctx, otherTenant, id)
		require.ErrorIs(t, err, domain.ErrUserNotFound)
		_, err = repo.ByPlatformUser(ctx, otherTenant, platformID)
		require.ErrorIs(t, err, domain.ErrUserNotFound)
		err = repo.Update(ctx, otherTenant, id, func(u *domain.TenantUser) (*domain.TenantUser, error) {
			u.Suspend()
			return u, nil
		})
		require.ErrorIs(t, err, domain.ErrUserNotFound)

		got, err := repo.Get(ctx, tenantID, id)
		require.NoError(t, err)
		require.Equal(t, domain.UserActive, got.Status(), "another tenant cannot change the user")
	})
}
```

- [ ] **Step 4: Rewrite the sessions tests as a contract suite**

```bash
git mv internal/iam/adapters/sessions_pg_test.go internal/iam/adapters/sessions_test.go
```

Replace the contents of `internal/iam/adapters/sessions_test.go` with the code below.
- `TestAuditRecordAndAll` leaves this file. Step 5 moves it to `audit_test.go`.
- `TestSessionsAreTenantScoped` is new.
- No test reads an expired session back. On YDB, TTL may delete it at any time.

```go
package adapters_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
	"github.com/nikolaymatrosov/nvelope/internal/iam/domain"
)

func TestSessionsAddByTokenHash(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.sessions(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		userID := newTenantUser(t, b, tenantID)

		hash := "hash-" + dbtest.RandString()
		expiresAt := time.Now().Add(time.Hour)
		s, err := domain.NewSession(tenantID, userID, hash, false, expiresAt)
		require.NoError(t, err)
		id, err := repo.Add(ctx, tenantID, s)
		require.NoError(t, err)
		require.NotEmpty(t, id, "the adapter assigns an id")

		got, err := repo.ByTokenHash(ctx, tenantID, hash)
		require.NoError(t, err)
		require.Equal(t, id, got.ID())
		require.Equal(t, tenantID, got.TenantID())
		require.Equal(t, userID, got.UserID())
		require.Equal(t, hash, got.TokenHash())
		require.Equal(t, domain.SessionActive, got.State())
		require.WithinDuration(t, expiresAt, got.ExpiresAt(), time.Second)
		require.WithinDuration(t, time.Now(), got.CreatedAt(), time.Minute, "the database stamps the insert")
		require.Nil(t, got.RevokedAt())

		_, err = repo.ByTokenHash(ctx, tenantID, "no-such-hash")
		require.ErrorIs(t, err, domain.ErrSessionNotFound)
	})
}

func TestSessionsUpdateRevoke(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.sessions(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		userID := newTenantUser(t, b, tenantID)

		hash := "hash-" + dbtest.RandString()
		s, err := domain.NewSession(tenantID, userID, hash, true, time.Now().Add(time.Hour))
		require.NoError(t, err)
		id, err := repo.Add(ctx, tenantID, s)
		require.NoError(t, err)

		got, err := repo.ByTokenHash(ctx, tenantID, hash)
		require.NoError(t, err)
		require.Equal(t, domain.SessionTOTPPending, got.State())

		require.NoError(t, repo.Update(ctx, tenantID, id, func(s *domain.Session) (*domain.Session, error) {
			return s, s.CompleteTOTP()
		}))
		got, err = repo.ByTokenHash(ctx, tenantID, hash)
		require.NoError(t, err)
		require.Equal(t, domain.SessionActive, got.State())

		revokedAt := time.Now()
		require.NoError(t, repo.Update(ctx, tenantID, id, func(s *domain.Session) (*domain.Session, error) {
			s.Revoke(revokedAt)
			return s, nil
		}))
		got, err = repo.ByTokenHash(ctx, tenantID, hash)
		require.NoError(t, err)
		require.Equal(t, domain.SessionRevoked, got.State())
		require.NotNil(t, got.RevokedAt())
		require.WithinDuration(t, revokedAt, *got.RevokedAt(), time.Second)
	})
}

func TestSessionsAreTenantScoped(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.sessions(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		otherTenant := b.newTenant(t)
		userID := newTenantUser(t, b, tenantID)

		hash := "hash-" + dbtest.RandString()
		s, err := domain.NewSession(tenantID, userID, hash, false, time.Now().Add(time.Hour))
		require.NoError(t, err)
		id, err := repo.Add(ctx, tenantID, s)
		require.NoError(t, err)

		_, err = repo.ByTokenHash(ctx, otherTenant, hash)
		require.ErrorIs(t, err, domain.ErrSessionNotFound)
		revoke := func(s *domain.Session) (*domain.Session, error) {
			s.Revoke(time.Now())
			return s, nil
		}
		require.ErrorIs(t, repo.Update(ctx, otherTenant, id, revoke), domain.ErrSessionNotFound)
		require.ErrorIs(t, repo.Update(ctx, tenantID, "00000000-0000-0000-0000-000000000000", revoke),
			domain.ErrSessionNotFound)
		require.ErrorIs(t, repo.Update(ctx, tenantID, "not-a-uuid", revoke),
			domain.ErrSessionNotFound, "a malformed id is an opaque not-found")

		got, err := repo.ByTokenHash(ctx, tenantID, hash)
		require.NoError(t, err)
		require.Equal(t, domain.SessionActive, got.State(), "another tenant cannot revoke the session")
	})
}
```

- [ ] **Step 5: Write the audit contract suite**

Create `internal/iam/adapters/audit_test.go`. `TestAuditRecordAndAll` comes from `sessions_pg_test.go` and now checks every field. `TestAuditAllPagesNewestFirst` is new: it pins the newest-first order, the paging and the total count.

```go
package adapters_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/iam/domain"
)

func TestAuditRecordAndAll(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.audit(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		userID := newTenantUser(t, b, tenantID)

		rec := domain.NewAuditRecord(tenantID, userID, domain.PrincipalSession,
			"role.create", "role:Editor", map[string]any{"name": "Editor"})
		require.NoError(t, repo.Record(ctx, tenantID, rec))

		records, total, err := repo.All(ctx, tenantID, domain.Page{})
		require.NoError(t, err)
		require.Equal(t, 1, total)
		require.Len(t, records, 1)
		got := records[0]
		require.NotEmpty(t, got.ID, "the adapter assigns an id")
		require.Equal(t, tenantID, got.TenantID)
		require.Equal(t, userID, got.ActorID)
		require.Equal(t, domain.PrincipalSession, got.ActorKind)
		require.Equal(t, "role.create", got.Action)
		require.Equal(t, "role:Editor", got.Target)
		require.Equal(t, map[string]any{"name": "Editor"}, got.Metadata)
		require.WithinDuration(t, time.Now(), got.CreatedAt, time.Minute, "the database stamps the insert")
	})
}

func TestAuditAllPagesNewestFirst(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.audit(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		userID := newTenantUser(t, b, tenantID)

		for _, action := range []string{"first", "second", "third"} {
			require.NoError(t, repo.Record(ctx, tenantID,
				domain.NewAuditRecord(tenantID, userID, domain.PrincipalAPIKey, action, "", nil)))
		}
		actions := func(records []domain.AuditRecord) []string {
			out := make([]string, len(records))
			for i, r := range records {
				out[i] = r.Action
			}
			return out
		}

		page, total, err := repo.All(ctx, tenantID, domain.Page{Limit: 2})
		require.NoError(t, err)
		require.Equal(t, 3, total, "the total counts every record, not just the page")
		require.Equal(t, []string{"third", "second"}, actions(page))
		require.Equal(t, map[string]any{}, page[0].Metadata, "nil metadata is stored as an empty object")

		page, total, err = repo.All(ctx, tenantID, domain.Page{Offset: 2, Limit: 2})
		require.NoError(t, err)
		require.Equal(t, 3, total)
		require.Equal(t, []string{"first"}, actions(page))

		page, total, err = repo.All(ctx, b.newTenant(t), domain.Page{})
		require.NoError(t, err)
		require.Zero(t, total, "another tenant's records are not visible")
		require.Empty(t, page)
	})
}
```

- [ ] **Step 6: Run the suites to verify they fail**

Run: `go vet ./internal/iam/adapters/`

Expected: build failure `undefined: adapters.NewYDBUsers`, `undefined: adapters.NewYDBSessions` and `undefined: adapters.NewYDBAudit`.

- [ ] **Step 7: Implement `YDBUsers`**

Create `internal/iam/adapters/users_ydb.go`:

```go
package adapters

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	ydb "github.com/ydb-platform/ydb-go-sdk/v3"
	"github.com/ydb-platform/ydb-go-sdk/v3/query"

	"github.com/nikolaymatrosov/nvelope/internal/iam/domain"
)

// YDBUsers is the YDB-backed implementation of domain.UserRepository, for the
// serverless runtime. YDB has no row-level security, so every statement
// filters by tenant_id explicitly; the primary key (tenant_id, id) makes that
// filter a key-prefix lookup.
type YDBUsers struct {
	driver *ydb.Driver
}

var _ domain.UserRepository = (*YDBUsers)(nil)

// NewYDBUsers builds a YDBUsers repository over the driver.
func NewYDBUsers(d *ydb.Driver) *YDBUsers {
	return &YDBUsers{driver: d}
}

// ydbUserColumns is the projection ydbUserFromRow reads, in order.
const ydbUserColumns = `id, tenant_id, platform_user_id, email, name, status,
    totp_enabled, totp_secret, created_at, updated_at`

const ydbSelectUser = `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
SELECT ` + ydbUserColumns + `
FROM users
WHERE tenant_id = $tenant_id AND id = $id;`

// Add persists a new tenant-plane user under a fresh UUID and returns the id.
// A second user for the same platform user or email in the tenant collides
// with a unique index and is returned as an error, as on Postgres.
func (r *YDBUsers) Add(ctx context.Context, tenantID string, u *domain.TenantUser) (string, error) {
	id := uuid.NewString()
	if err := r.driver.Query().Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DECLARE $platform_user_id AS Utf8;
DECLARE $email AS Utf8;
DECLARE $name AS Utf8;
DECLARE $status AS Utf8;
DECLARE $totp_enabled AS Bool;
DECLARE $totp_secret AS Optional<String>;
INSERT INTO users
    (tenant_id, id, platform_user_id, email, name, status, totp_enabled, totp_secret,
     created_at, updated_at)
VALUES ($tenant_id, $id, $platform_user_id, $email, $name, $status, $totp_enabled, $totp_secret,
     CurrentUtcTimestamp(), CurrentUtcTimestamp());`,
		query.WithParameters(ydb.ParamsBuilder().
			Param("$tenant_id").Text(tenantID).
			Param("$id").Text(id).
			Param("$platform_user_id").Text(u.PlatformUserID()).
			Param("$email").Text(u.Email()).
			Param("$name").Text(u.Name()).
			Param("$status").Text(string(u.Status())).
			Param("$totp_enabled").Bool(u.TOTPEnabled()).
			Param("$totp_secret").BeginOptional().Bytes(ydbOptionalBytes(u.TOTPSecret())).EndOptional().
			Build())); err != nil {
		return "", fmt.Errorf("inserting user: %w", err)
	}
	return id, nil
}

// Update loads the user, runs fn, and persists the result in one serializable
// transaction. The SDK retries the transaction on conflict, so fn may run more
// than once and must be retry-safe.
func (r *YDBUsers) Update(ctx context.Context, tenantID, id string,
	fn func(*domain.TenantUser) (*domain.TenantUser, error)) error {

	return r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		loaded, err := ydbUserFromRow(tx.QueryRow(ctx, ydbSelectUser, ydbTenantAndID(tenantID, id)))
		if err != nil {
			return err
		}
		updated, err := fn(loaded)
		if err != nil {
			return err
		}
		if err := tx.Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DECLARE $name AS Utf8;
DECLARE $status AS Utf8;
DECLARE $totp_enabled AS Bool;
DECLARE $totp_secret AS Optional<String>;
UPDATE users SET
    name = $name, status = $status, totp_enabled = $totp_enabled, totp_secret = $totp_secret,
    updated_at = CurrentUtcTimestamp()
WHERE tenant_id = $tenant_id AND id = $id;`,
			query.WithParameters(ydb.ParamsBuilder().
				Param("$tenant_id").Text(tenantID).
				Param("$id").Text(id).
				Param("$name").Text(updated.Name()).
				Param("$status").Text(string(updated.Status())).
				Param("$totp_enabled").Bool(updated.TOTPEnabled()).
				Param("$totp_secret").BeginOptional().Bytes(ydbOptionalBytes(updated.TOTPSecret())).EndOptional().
				Build())); err != nil {
			return fmt.Errorf("updating user: %w", err)
		}
		return nil
	})
}

// Get returns the user, or domain.ErrUserNotFound.
func (r *YDBUsers) Get(ctx context.Context, tenantID, id string) (*domain.TenantUser, error) {
	return ydbUserFromRow(r.driver.Query().QueryRow(ctx, ydbSelectUser, ydbTenantAndID(tenantID, id)))
}

// ByPlatformUser returns the tenant-plane user for a control-plane identity,
// or domain.ErrUserNotFound.
func (r *YDBUsers) ByPlatformUser(ctx context.Context, tenantID, platformUserID string) (*domain.TenantUser, error) {
	return ydbUserFromRow(r.driver.Query().QueryRow(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $platform_user_id AS Utf8;
SELECT `+ydbUserColumns+`
FROM users VIEW idx_users_platform_user
WHERE tenant_id = $tenant_id AND platform_user_id = $platform_user_id;`,
		query.WithParameters(ydb.ParamsBuilder().
			Param("$tenant_id").Text(tenantID).
			Param("$platform_user_id").Text(platformUserID).
			Build())))
}

// ydbUserFromRow maps a single-row read in ydbUserColumns order to the user,
// translating "no row" into domain.ErrUserNotFound.
func ydbUserFromRow(row query.Row, err error) (*domain.TenantUser, error) {
	if errors.Is(err, query.ErrNoRows) {
		return nil, domain.ErrUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("loading user: %w", err)
	}
	var id, tenantID, platformUserID, email, name, status string
	var totpEnabled bool
	var totpSecret *[]byte
	var createdAt, updatedAt time.Time
	if err := row.Scan(&id, &tenantID, &platformUserID, &email, &name, &status,
		&totpEnabled, &totpSecret, &createdAt, &updatedAt); err != nil {
		return nil, fmt.Errorf("scanning user: %w", err)
	}
	var secret []byte
	if totpSecret != nil {
		secret = *totpSecret
	}
	return domain.HydrateTenantUser(id, tenantID, platformUserID, email, name,
		domain.UserStatus(status), totpEnabled, secret, createdAt, updatedAt), nil
}

// ydbOptionalBytes is b as an Optional<String> parameter: NULL when b is nil,
// as pgx writes a nil []byte.
func ydbOptionalBytes(b []byte) *[]byte {
	if b == nil {
		return nil
	}
	return &b
}

// ydbTenantParam binds the $tenant_id parameter.
func ydbTenantParam(tenantID string) query.ExecuteOption {
	return query.WithParameters(ydb.ParamsBuilder().Param("$tenant_id").Text(tenantID).Build())
}

// ydbTenantAndID binds the (tenant_id, id) primary key.
func ydbTenantAndID(tenantID, id string) query.ExecuteOption {
	return query.WithParameters(ydb.ParamsBuilder().
		Param("$tenant_id").Text(tenantID).
		Param("$id").Text(id).
		Build())
}
```

- [ ] **Step 8: Implement `YDBSessions`**

Create `internal/iam/adapters/sessions_ydb.go`:

```go
package adapters

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	ydb "github.com/ydb-platform/ydb-go-sdk/v3"
	"github.com/ydb-platform/ydb-go-sdk/v3/query"

	"github.com/nikolaymatrosov/nvelope/internal/iam/domain"
)

// YDBSessions is the YDB-backed implementation of domain.SessionRepository,
// for the serverless runtime. Only the hash of a session token is stored. The
// table's TTL deletes a session once it has expired.
type YDBSessions struct {
	driver *ydb.Driver
}

var _ domain.SessionRepository = (*YDBSessions)(nil)

// NewYDBSessions builds a YDBSessions repository over the driver.
func NewYDBSessions(d *ydb.Driver) *YDBSessions {
	return &YDBSessions{driver: d}
}

// ydbSessionColumns is the projection ydbSessionFromRow reads, in order.
const ydbSessionColumns = `id, tenant_id, user_id, token_hash, state, created_at, expires_at, revoked_at`

// Add persists a new session under a fresh UUID and returns the id.
func (r *YDBSessions) Add(ctx context.Context, tenantID string, s *domain.Session) (string, error) {
	id := uuid.NewString()
	if err := r.driver.Query().Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DECLARE $user_id AS Utf8;
DECLARE $token_hash AS Utf8;
DECLARE $state AS Utf8;
DECLARE $expires_at AS Timestamp;
INSERT INTO sessions (tenant_id, id, user_id, token_hash, state, created_at, expires_at)
VALUES ($tenant_id, $id, $user_id, $token_hash, $state, CurrentUtcTimestamp(), $expires_at);`,
		query.WithParameters(ydb.ParamsBuilder().
			Param("$tenant_id").Text(tenantID).
			Param("$id").Text(id).
			Param("$user_id").Text(s.UserID()).
			Param("$token_hash").Text(s.TokenHash()).
			Param("$state").Text(string(s.State())).
			Param("$expires_at").Timestamp(s.ExpiresAt()).
			Build())); err != nil {
		return "", fmt.Errorf("inserting session: %w", err)
	}
	return id, nil
}

// Update loads the session, runs fn, and persists the resulting state in one
// serializable transaction. The SDK retries the transaction on conflict, so fn
// may run more than once and must be retry-safe.
func (r *YDBSessions) Update(ctx context.Context, tenantID, id string,
	fn func(*domain.Session) (*domain.Session, error)) error {

	return r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		loaded, err := ydbSessionFromRow(tx.QueryRow(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
SELECT `+ydbSessionColumns+`
FROM sessions
WHERE tenant_id = $tenant_id AND id = $id;`, ydbTenantAndID(tenantID, id)))
		if err != nil {
			return err
		}
		updated, err := fn(loaded)
		if err != nil {
			return err
		}
		if err := tx.Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DECLARE $state AS Utf8;
DECLARE $revoked_at AS Optional<Timestamp>;
UPDATE sessions SET state = $state, revoked_at = $revoked_at
WHERE tenant_id = $tenant_id AND id = $id;`,
			query.WithParameters(ydb.ParamsBuilder().
				Param("$tenant_id").Text(tenantID).
				Param("$id").Text(id).
				Param("$state").Text(string(updated.State())).
				Param("$revoked_at").BeginOptional().Timestamp(updated.RevokedAt()).EndOptional().
				Build())); err != nil {
			return fmt.Errorf("updating session: %w", err)
		}
		return nil
	})
}

// ByTokenHash returns the tenant's session for a token hash, or
// domain.ErrSessionNotFound. It returns the session whatever its state; the
// caller decides whether it still authenticates.
func (r *YDBSessions) ByTokenHash(ctx context.Context, tenantID, tokenHash string) (*domain.Session, error) {
	return ydbSessionFromRow(r.driver.Query().QueryRow(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $token_hash AS Utf8;
SELECT `+ydbSessionColumns+`
FROM sessions VIEW idx_sessions_token_hash
WHERE token_hash = $token_hash AND tenant_id = $tenant_id;`,
		query.WithParameters(ydb.ParamsBuilder().
			Param("$tenant_id").Text(tenantID).
			Param("$token_hash").Text(tokenHash).
			Build())))
}

// ydbSessionFromRow maps a single-row read in ydbSessionColumns order to the
// session, translating "no row" into domain.ErrSessionNotFound.
func ydbSessionFromRow(row query.Row, err error) (*domain.Session, error) {
	if errors.Is(err, query.ErrNoRows) {
		return nil, domain.ErrSessionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("loading session: %w", err)
	}
	var id, tenantID, userID, tokenHash, state string
	var createdAt, expiresAt time.Time
	var revokedAt *time.Time
	if err := row.Scan(&id, &tenantID, &userID, &tokenHash, &state,
		&createdAt, &expiresAt, &revokedAt); err != nil {
		return nil, fmt.Errorf("scanning session: %w", err)
	}
	return domain.HydrateSession(id, tenantID, userID, tokenHash,
		domain.SessionState(state), createdAt, expiresAt, revokedAt), nil
}
```

- [ ] **Step 9: Implement `YDBAudit`**

Create `internal/iam/adapters/audit_ydb.go`. The count and the page share one `DoTx`, as they share one pg transaction. The rows are collected into a closure-local slice (spec V6 row `audit_pg.go:82`).

```go
package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	ydb "github.com/ydb-platform/ydb-go-sdk/v3"
	"github.com/ydb-platform/ydb-go-sdk/v3/query"

	"github.com/nikolaymatrosov/nvelope/internal/iam/domain"
)

// YDBAudit is the YDB-backed implementation of domain.AuditRepository, for the
// serverless runtime. The primary key (tenant_id, created_at, id) is the order
// All pages through.
type YDBAudit struct {
	driver *ydb.Driver
}

var _ domain.AuditRepository = (*YDBAudit)(nil)

// NewYDBAudit builds a YDBAudit repository over the driver.
func NewYDBAudit(d *ydb.Driver) *YDBAudit {
	return &YDBAudit{driver: d}
}

// Record appends one audit record under a fresh UUID. The database stamps
// created_at.
func (r *YDBAudit) Record(ctx context.Context, tenantID string, rec domain.AuditRecord) error {
	metadata, err := json.Marshal(rec.Metadata)
	if err != nil {
		return fmt.Errorf("encoding audit metadata: %w", err)
	}
	if err := r.driver.Query().Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DECLARE $actor_id AS Utf8;
DECLARE $actor_kind AS Utf8;
DECLARE $action AS Utf8;
DECLARE $target AS Utf8;
DECLARE $metadata AS Utf8;
INSERT INTO audit_log (tenant_id, created_at, id, actor_id, actor_kind, action, target, metadata)
VALUES ($tenant_id, CurrentUtcTimestamp(), $id, $actor_id, $actor_kind, $action, $target, $metadata);`,
		query.WithParameters(ydb.ParamsBuilder().
			Param("$tenant_id").Text(tenantID).
			Param("$id").Text(uuid.NewString()).
			Param("$actor_id").Text(rec.ActorID).
			Param("$actor_kind").Text(string(rec.ActorKind)).
			Param("$action").Text(rec.Action).
			Param("$target").Text(rec.Target).
			Param("$metadata").Text(string(metadata)).
			Build())); err != nil {
		return fmt.Errorf("inserting audit record: %w", err)
	}
	return nil
}

// All returns a page of the tenant's audit records, newest first, and the
// tenant's total record count. The count and the page are read in one
// transaction, so they agree.
func (r *YDBAudit) All(ctx context.Context, tenantID string, page domain.Page) ([]domain.AuditRecord, int, error) {
	page = page.Normalize()
	var records []domain.AuditRecord
	var total int
	err := r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		row, err := tx.QueryRow(ctx, `DECLARE $tenant_id AS Utf8;
SELECT COUNT(*) FROM audit_log WHERE tenant_id = $tenant_id;`, ydbTenantParam(tenantID))
		if err != nil {
			return fmt.Errorf("counting audit records: %w", err)
		}
		var count uint64
		if err := row.Scan(&count); err != nil {
			return fmt.Errorf("scanning audit count: %w", err)
		}
		// ponytail: OFFSET paging reads every skipped row; keyset paging on
		// (created_at, id) if the audit trail grows large.
		rs, err := tx.QueryResultSet(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $limit AS Uint64;
DECLARE $offset AS Uint64;
SELECT id, tenant_id, actor_id, actor_kind, action, target, metadata, created_at
FROM audit_log
WHERE tenant_id = $tenant_id
ORDER BY created_at DESC
LIMIT $limit OFFSET $offset;`,
			query.WithParameters(ydb.ParamsBuilder().
				Param("$tenant_id").Text(tenantID).
				Param("$limit").Uint64(uint64(page.Limit)).
				Param("$offset").Uint64(uint64(page.Offset)).
				Build()))
		if err != nil {
			return fmt.Errorf("listing audit records: %w", err)
		}
		// Collected into a local and assigned at the end: the SDK may re-run
		// the closure, and a retry must not append to rows a failed run read.
		var local []domain.AuditRecord
		for row, err := range rs.Rows(ctx) {
			if err != nil {
				return fmt.Errorf("listing audit records: %w", err)
			}
			var id, tID, actorID, actorKind, action, target, metadataJSON string
			var createdAt time.Time
			if err := row.Scan(&id, &tID, &actorID, &actorKind, &action, &target,
				&metadataJSON, &createdAt); err != nil {
				return fmt.Errorf("scanning audit record: %w", err)
			}
			metadata := map[string]any{}
			if err := json.Unmarshal([]byte(metadataJSON), &metadata); err != nil {
				return fmt.Errorf("decoding audit metadata: %w", err)
			}
			local = append(local, domain.AuditRecord{
				ID: id, TenantID: tID, ActorID: actorID,
				ActorKind: domain.PrincipalKind(actorKind), Action: action,
				Target: target, Metadata: metadata, CreatedAt: createdAt,
			})
		}
		records, total = local, int(count)
		return nil
	})
	if err != nil {
		return nil, 0, err
	}
	return records, total, nil
}
```

- [ ] **Step 10: Update the package doc**

In `internal/iam/adapters/roles_pg.go`, replace:

```go
// Package adapters implements the iam domain's repository and capability
// interfaces against PostgreSQL. Every tenant-plane operation runs inside the
// shared RLS-bound transaction (internal/platform/tenantdb).
```

with:

```go
// Package adapters implements the iam domain's repository and capability
// interfaces against PostgreSQL. Every tenant-plane operation runs inside the
// shared RLS-bound transaction (internal/platform/tenantdb). The *_ydb.go
// files are the YDB implementations of the repositories for the serverless
// runtime; they filter by tenant_id explicitly instead.
```

- [ ] **Step 11: Run the suites to verify they pass**

Run: `export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock" && go test ./internal/iam/adapters/ -count=1 -v -timeout 8m 2>&1 | grep -E -- '--- (PASS|FAIL)|^(ok|FAIL)' | grep -E 'FAIL|/pg|/ydb|^ok' | sort`

Expected:
- 20 `--- PASS` lines: every `TestUsers*`, `TestSessions*` and `TestAudit*` on both `/pg` and `/ydb`.
- No `FAIL` line. The pg-only `TestRoles*`, `TestAPIKeys*` and `TestRecoveryCodes*` still run; the filter hides their `PASS` lines but would show a `FAIL`.
- The package is `ok`.

Three cases are not yours to fix. Report BLOCKED with the verbatim output:
- **A `/pg` subtest fails.** The contract would then change behaviour for SaaS, or Postgres does not behave as the plan assumes. In particular, `TestUsersAddRejectsDuplicates/pg` failing on the upper-case email means the `citext` assumption is wrong.
- **The migration is rejected by YDB.**
- **`TestUsersUpdateTOTP/ydb` fails on the secret** (a non-`nil` value after `DisableTOTP`, or bytes that do not round-trip). That contradicts the spec's "Plan B4 planning spike" finding on `Optional<String>`.

Any other `/ydb` failure: fix only the YDB adapters or the migration, and record the verbatim error and the fix in the report.

- [ ] **Step 12: Full verification and commit**

Run: `export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock" && go build ./... && go vet ./... && go test ./... -timeout 15m 2>&1 | grep -vE '^(ok|\?)\s'`

Expected: no output. In other words, no failing package.

Run: `golangci-lint run`

Expected: `0 issues.`

Run: `PATH="$HOME/go/bin:$PATH" make lint-arch`

Expected: the last line is `[arch] clean`.

Run: `git diff --stat HEAD -- go.mod go.sum`

Expected: no output.

```bash
gofmt -l internal/iam/
git add internal/db/ydbmigrations/000005_iam.up.sql
git add internal/iam/adapters/users_ydb.go internal/iam/adapters/sessions_ydb.go internal/iam/adapters/audit_ydb.go internal/iam/adapters/roles_pg.go
git add internal/iam/adapters/helpers_test.go internal/iam/adapters/users_test.go internal/iam/adapters/sessions_test.go internal/iam/adapters/audit_test.go
git commit -m "feat(iam): add the YDB user, session and audit repositories under pg+ydb contract suites"
```

`gofmt -l` must print nothing. Afterwards, `git status --short` must print nothing: `users_pg_test.go` and `sessions_pg_test.go` are gone, and no file is left untracked.

---

### Task 2: `YDBRoles` and the roles contract suite

**Files:**
- Create: `internal/iam/adapters/roles_ydb.go`
- Modify: `internal/iam/adapters/helpers_test.go` (rewritten: adds the `roles` and `newList` fields)
- Rename: `internal/iam/adapters/roles_pg_test.go` → `internal/iam/adapters/roles_test.go` (rewritten)

**Interfaces:**
- Consumes (Task 1):
  - the `roles` table (primary key `(tenant_id, id)`, unique index `idx_roles_tenant_name` on `(tenant_id, name)`), `user_roles` (primary key `(tenant_id, user_id)`) and `user_list_roles` (primary key `(tenant_id, user_id, list_id)`);
  - `ydbTenantParam(tenantID string) query.ExecuteOption` and `ydbTenantAndID(tenantID, id string) query.ExecuteOption`;
  - the harness: `backend`, `backends`, `eachBackend`, `newTenantUser`, `seedList`, `seedTenant`, `seedPlatformUser`, `seedTenantUser`.
- Consumes `db.IsYDBConflict`, `permStrings` and `permsFromStrings` (`roles_pg.go`), and the port `domain.RoleRepository`, unchanged.
- Produces:
  ```go
  // package adapters (internal/iam/adapters)
  type YDBRoles struct{ /* unexported */ }
  func NewYDBRoles(d *ydb.Driver) *YDBRoles
  func ydbPermissions(perms []domain.Permission) string
  func ydbPermissionsFrom(raw string) ([]domain.Permission, error)
  // package adapters_test: backend gains
  roles   func(t *testing.T) domain.RoleRepository
  newList func(t *testing.T, tenantID string) string
  ```
- Task 3 relies on `ydbPermissions` and `ydbPermissionsFrom` for the `api_keys.permissions` column.

- [ ] **Step 1: Add the roles and list fields to the harness**

Replace the contents of `internal/iam/adapters/helpers_test.go` with the code below. It is Task 1's harness plus:
- a `roles` field;
- a `newList` field. The pg backend inserts a real list, because `user_list_roles.list_id` has a foreign key to `lists`. The YDB backend uses a bare UUID.

```go
package adapters_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
	"github.com/nikolaymatrosov/nvelope/internal/iam/adapters"
	"github.com/nikolaymatrosov/nvelope/internal/iam/domain"
	"github.com/nikolaymatrosov/nvelope/internal/platform/tenantdb"
)

// backend is one storage implementation of the iam repositories under
// contract test. Every contract test body runs once per backend, so the
// Postgres and YDB adapters are held to the same behaviour.
type backend struct {
	name     string
	users    func(t *testing.T) domain.UserRepository
	sessions func(t *testing.T) domain.SessionRepository
	roles    func(t *testing.T) domain.RoleRepository
	audit    func(t *testing.T) domain.AuditRepository
	// newTenant, newPlatformUser and newList return ids of rows other contexts
	// own. On Postgres the iam tables carry foreign keys to them, so the pg
	// backend inserts real rows. YDB has no foreign keys and no iam query reads
	// those tables, so a bare UUID stands in.
	newTenant       func(t *testing.T) string
	newPlatformUser func(t *testing.T) string
	newList         func(t *testing.T, tenantID string) string
}

var backends = []backend{
	{
		name: "pg",
		users: func(t *testing.T) domain.UserRepository {
			return adapters.NewUsers(dbtest.AppPool(t))
		},
		sessions: func(t *testing.T) domain.SessionRepository {
			return adapters.NewSessions(dbtest.AppPool(t))
		},
		roles: func(t *testing.T) domain.RoleRepository {
			return adapters.NewRoles(dbtest.AppPool(t))
		},
		audit: func(t *testing.T) domain.AuditRepository {
			return adapters.NewAudit(dbtest.AppPool(t))
		},
		newTenant:       func(t *testing.T) string { return seedTenant(t, dbtest.AppPool(t)) },
		newPlatformUser: func(t *testing.T) string { return seedPlatformUser(t, dbtest.AppPool(t)) },
		newList:         func(t *testing.T, tenantID string) string { return seedList(t, dbtest.AppPool(t), tenantID) },
	},
	{
		name: "ydb",
		users: func(t *testing.T) domain.UserRepository {
			return adapters.NewYDBUsers(dbtest.YDBMigrated(t))
		},
		sessions: func(t *testing.T) domain.SessionRepository {
			return adapters.NewYDBSessions(dbtest.YDBMigrated(t))
		},
		roles: func(t *testing.T) domain.RoleRepository {
			return adapters.NewYDBRoles(dbtest.YDBMigrated(t))
		},
		audit: func(t *testing.T) domain.AuditRepository {
			return adapters.NewYDBAudit(dbtest.YDBMigrated(t))
		},
		newTenant:       func(*testing.T) string { return uuid.NewString() },
		newPlatformUser: func(*testing.T) string { return uuid.NewString() },
		newList:         func(*testing.T, string) string { return uuid.NewString() },
	},
}

// eachBackend runs body once per backend, as parallel subtests named after it.
func eachBackend(t *testing.T, body func(t *testing.T, b backend)) {
	t.Helper()
	for _, b := range backends {
		t.Run(b.name, func(t *testing.T) {
			t.Parallel()
			body(t, b)
		})
	}
}

// newTenantUser adds a tenant-plane user with a fresh address through the
// backend's user repository and returns its id.
func newTenantUser(t *testing.T, b backend, tenantID string) string {
	t.Helper()
	u, err := domain.NewTenantUser(tenantID, b.newPlatformUser(t), dbtest.RandString()+"@example.com", "Member")
	require.NoError(t, err)
	id, err := b.users(t).Add(context.Background(), tenantID, u)
	require.NoError(t, err)
	return id
}

// seedList inserts a list row inside a tenant-bound transaction and returns
// its id — needed to satisfy the user_list_roles foreign key.
func seedList(t *testing.T, pool *pgxpool.Pool, tenantID string) string {
	t.Helper()
	var id string
	require.NoError(t, tenantdb.WithTenant(context.Background(), pool, tenantID,
		func(ctx context.Context, tx pgx.Tx) error {
			return tx.QueryRow(ctx,
				"INSERT INTO lists (tenant_id, name) VALUES ($1, $2) RETURNING id",
				tenantID, "L-"+dbtest.RandString()).Scan(&id)
		}))
	return id
}

// seedTenant inserts a tenant row directly and returns its id.
func seedTenant(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var id string
	require.NoError(t, pool.QueryRow(context.Background(),
		`INSERT INTO tenants (name, slug, status) VALUES ('Workspace', $1, 'active') RETURNING id`,
		"iam-"+dbtest.RandString()).Scan(&id))
	return id
}

// seedPlatformUser inserts a control-plane user row directly and returns its id.
func seedPlatformUser(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var id string
	require.NoError(t, pool.QueryRow(context.Background(),
		`INSERT INTO platform_users (email, password_hash, name)
		 VALUES ($1, 'hash', 'User') RETURNING id`,
		dbtest.RandString()+"@example.com").Scan(&id))
	return id
}

// seedTenantUser inserts a tenant-plane user and returns its id.
func seedTenantUser(t *testing.T, pool *pgxpool.Pool, tenantID string) string {
	t.Helper()
	platformID := seedPlatformUser(t, pool)
	u, err := domain.NewTenantUser(tenantID, platformID, dbtest.RandString()+"@example.com", "Member")
	require.NoError(t, err)
	id, err := adapters.NewUsers(pool).Add(context.Background(), tenantID, u)
	require.NoError(t, err)
	return id
}
```

- [ ] **Step 2: Rewrite the roles tests as a contract suite**

```bash
git mv internal/iam/adapters/roles_pg_test.go internal/iam/adapters/roles_test.go
```

Replace the contents of `internal/iam/adapters/roles_test.go` with the code below. One test and many assertions are new:
- `TestRolesAssignUnknownRoleFails` pins the Postgres foreign key from an assignment to its role.
- `TestRolesAddGetAll` now pins:
  - the order by name;
  - per-tenant name uniqueness;
  - tenant scoping;
  - an empty permission set.
- `TestRolesUpdate` now pins a rename onto a taken name (`ErrRoleNameTaken`, nothing changed) and keeping its own name.
- `TestRolesDeleteRejectsAssigned` now pins:
  - a per-list assignment keeping the role;
  - deleting a missing role;
  - deleting another tenant's role.
- `TestRolesEffectiveFor` now pins:
  - a user with no roles;
  - re-assignment overwriting the role;
  - `RemoveListRole`, including removing twice;
  - tenant scoping.

```go
package adapters_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/iam/domain"
)

func newRole(t *testing.T, tenantID, name string, perms ...domain.Permission) *domain.Role {
	t.Helper()
	r, err := domain.NewRole(tenantID, name, perms)
	require.NoError(t, err)
	return r
}

func roleNames(roles []*domain.Role) []string {
	out := make([]string, len(roles))
	for i, r := range roles {
		out[i] = r.Name()
	}
	return out
}

func TestRolesAddGetAll(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.roles(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		id, err := repo.Add(ctx, tenantID, newRole(t, tenantID, "Viewer", domain.PermListsGet, domain.PermRolesGet))
		require.NoError(t, err)
		require.NotEmpty(t, id, "the adapter assigns an id")

		got, err := repo.Get(ctx, tenantID, id)
		require.NoError(t, err)
		require.Equal(t, id, got.ID())
		require.Equal(t, tenantID, got.TenantID())
		require.Equal(t, "Viewer", got.Name())
		require.Equal(t, []domain.Permission{domain.PermListsGet, domain.PermRolesGet}, got.Permissions())

		_, err = repo.Add(ctx, tenantID, newRole(t, tenantID, "Viewer"))
		require.ErrorIs(t, err, domain.ErrRoleNameTaken)

		_, err = repo.Add(ctx, tenantID, newRole(t, tenantID, "Editor"))
		require.NoError(t, err)
		empty, err := repo.Add(ctx, tenantID, newRole(t, tenantID, "Empty"))
		require.NoError(t, err)
		gotEmpty, err := repo.Get(ctx, tenantID, empty)
		require.NoError(t, err)
		require.Empty(t, gotEmpty.Permissions())

		all, err := repo.All(ctx, tenantID)
		require.NoError(t, err)
		require.Equal(t, []string{"Editor", "Empty", "Viewer"}, roleNames(all), "every role, ordered by name")

		otherTenant := b.newTenant(t)
		_, err = repo.Add(ctx, otherTenant, newRole(t, otherTenant, "Viewer"))
		require.NoError(t, err, "role names are unique per tenant")
		_, err = repo.Get(ctx, otherTenant, id)
		require.ErrorIs(t, err, domain.ErrRoleNotFound, "another tenant's role is not visible")
		others, err := repo.All(ctx, otherTenant)
		require.NoError(t, err)
		require.Equal(t, []string{"Viewer"}, roleNames(others))

		_, err = repo.Get(ctx, tenantID, uuid.NewString())
		require.ErrorIs(t, err, domain.ErrRoleNotFound)
		_, err = repo.Get(ctx, tenantID, "not-a-uuid")
		require.ErrorIs(t, err, domain.ErrRoleNotFound, "a malformed id is an opaque not-found")
	})
}

func TestRolesUpdate(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.roles(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		id, err := repo.Add(ctx, tenantID, newRole(t, tenantID, "R"))
		require.NoError(t, err)
		require.NoError(t, repo.Update(ctx, tenantID, id, func(r *domain.Role) (*domain.Role, error) {
			if err := r.Rename("Renamed"); err != nil {
				return nil, err
			}
			return r, r.SetPermissions([]domain.Permission{domain.PermRolesGet})
		}))
		got, err := repo.Get(ctx, tenantID, id)
		require.NoError(t, err)
		require.Equal(t, "Renamed", got.Name())
		require.Equal(t, []domain.Permission{domain.PermRolesGet}, got.Permissions())

		// Keeping its own name is not a collision.
		require.NoError(t, repo.Update(ctx, tenantID, id, func(r *domain.Role) (*domain.Role, error) {
			return r, r.SetPermissions(nil)
		}))

		_, err = repo.Add(ctx, tenantID, newRole(t, tenantID, "Taken"))
		require.NoError(t, err)
		err = repo.Update(ctx, tenantID, id, func(r *domain.Role) (*domain.Role, error) {
			return r, r.Rename("Taken")
		})
		require.ErrorIs(t, err, domain.ErrRoleNameTaken)
		got, err = repo.Get(ctx, tenantID, id)
		require.NoError(t, err)
		require.Equal(t, "Renamed", got.Name(), "a rejected rename changes nothing")

		err = repo.Update(ctx, tenantID, uuid.NewString(), func(r *domain.Role) (*domain.Role, error) {
			return r, nil
		})
		require.ErrorIs(t, err, domain.ErrRoleNotFound)
	})
}

func TestRolesDeleteRejectsAssigned(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.roles(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		userID := newTenantUser(t, b, tenantID)

		tenantRole, err := repo.Add(ctx, tenantID, newRole(t, tenantID, "Assigned", domain.PermListsGet))
		require.NoError(t, err)
		require.NoError(t, repo.AssignTenantRole(ctx, tenantID, userID, tenantRole))
		require.ErrorIs(t, repo.Delete(ctx, tenantID, tenantRole), domain.ErrRoleInUse)

		listRole, err := repo.Add(ctx, tenantID, newRole(t, tenantID, "ListAssigned"))
		require.NoError(t, err)
		require.NoError(t, repo.AssignListRole(ctx, tenantID, userID, b.newList(t, tenantID), listRole))
		require.ErrorIs(t, repo.Delete(ctx, tenantID, listRole), domain.ErrRoleInUse,
			"a per-list assignment also keeps the role")

		// An unassigned role deletes cleanly.
		free, err := repo.Add(ctx, tenantID, newRole(t, tenantID, "Free"))
		require.NoError(t, err)
		require.NoError(t, repo.Delete(ctx, tenantID, free))
		_, err = repo.Get(ctx, tenantID, free)
		require.ErrorIs(t, err, domain.ErrRoleNotFound)

		require.ErrorIs(t, repo.Delete(ctx, tenantID, free), domain.ErrRoleNotFound)
		require.ErrorIs(t, repo.Delete(ctx, tenantID, "not-a-uuid"), domain.ErrRoleNotFound)
		require.ErrorIs(t, repo.Delete(ctx, b.newTenant(t), tenantRole), domain.ErrRoleNotFound,
			"another tenant cannot delete the role")
	})
}

func TestRolesEffectiveFor(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.roles(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		userID := newTenantUser(t, b, tenantID)

		tenantPerms, listPerms, err := repo.EffectiveFor(ctx, tenantID, userID)
		require.NoError(t, err)
		require.Empty(t, tenantPerms, "a user with no role has no permissions")
		require.Empty(t, listPerms)

		tenantRole, err := repo.Add(ctx, tenantID, newRole(t, tenantID, "Tenant", domain.PermListsGet))
		require.NoError(t, err)
		listRole, err := repo.Add(ctx, tenantID,
			newRole(t, tenantID, "ListEditor", domain.PermSubscribersManage))
		require.NoError(t, err)

		require.NoError(t, repo.AssignTenantRole(ctx, tenantID, userID, tenantRole))
		listID := b.newList(t, tenantID)
		require.NoError(t, repo.AssignListRole(ctx, tenantID, userID, listID, listRole))
		otherList := b.newList(t, tenantID)
		require.NoError(t, repo.AssignListRole(ctx, tenantID, userID, otherList, tenantRole))

		tenantPerms, listPerms, err = repo.EffectiveFor(ctx, tenantID, userID)
		require.NoError(t, err)
		require.Equal(t, []domain.Permission{domain.PermListsGet}, tenantPerms)
		require.Equal(t, map[string][]domain.Permission{
			listID:    {domain.PermSubscribersManage},
			otherList: {domain.PermListsGet},
		}, listPerms)

		// Assigning again replaces the role.
		require.NoError(t, repo.AssignTenantRole(ctx, tenantID, userID, listRole))
		require.NoError(t, repo.AssignListRole(ctx, tenantID, userID, listID, tenantRole))
		require.NoError(t, repo.RemoveListRole(ctx, tenantID, userID, otherList))
		require.NoError(t, repo.RemoveListRole(ctx, tenantID, userID, otherList), "removing twice is a no-op")

		tenantPerms, listPerms, err = repo.EffectiveFor(ctx, tenantID, userID)
		require.NoError(t, err)
		require.Equal(t, []domain.Permission{domain.PermSubscribersManage}, tenantPerms)
		require.Equal(t, map[string][]domain.Permission{listID: {domain.PermListsGet}}, listPerms)

		tenantPerms, listPerms, err = repo.EffectiveFor(ctx, b.newTenant(t), userID)
		require.NoError(t, err)
		require.Empty(t, tenantPerms, "another tenant sees none of the user's roles")
		require.Empty(t, listPerms)
	})
}

// TestRolesAssignUnknownRoleFails pins the Postgres foreign key from an
// assignment to its role: assigning a role that does not exist fails and
// grants nothing.
func TestRolesAssignUnknownRoleFails(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.roles(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		userID := newTenantUser(t, b, tenantID)

		require.Error(t, repo.AssignTenantRole(ctx, tenantID, userID, uuid.NewString()))
		require.Error(t, repo.AssignListRole(ctx, tenantID, userID, b.newList(t, tenantID), uuid.NewString()))

		tenantPerms, listPerms, err := repo.EffectiveFor(ctx, tenantID, userID)
		require.NoError(t, err)
		require.Empty(t, tenantPerms)
		require.Empty(t, listPerms)
	})
}
```

- [ ] **Step 3: Run the suite to verify it fails**

Run: `go vet ./internal/iam/adapters/`

Expected: build failure `undefined: adapters.NewYDBRoles`.

- [ ] **Step 4: Implement `YDBRoles`**

Create `internal/iam/adapters/roles_ydb.go`:
- `Update` maps a rename collision to `ErrRoleNameTaken` at the statement and again after `DoTx`. The spike saw it at the statement.
- `Delete` checks both assignment tables and then runs `DELETE ... RETURNING`, all in one `DoTx`.
- `AssignTenantRole` and `AssignListRole` read the role first (`ydbRoleExists`), then `UPSERT`.
- `EffectiveFor` builds its result locally and assigns it at the end of the closure.

```go
package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	ydb "github.com/ydb-platform/ydb-go-sdk/v3"
	"github.com/ydb-platform/ydb-go-sdk/v3/query"

	"github.com/nikolaymatrosov/nvelope/internal/db"
	"github.com/nikolaymatrosov/nvelope/internal/iam/domain"
)

// YDBRoles is the YDB-backed implementation of domain.RoleRepository, for the
// serverless runtime. A role's permissions are stored as a JSON array in a
// Utf8 column. YDB has no foreign keys, so an assignment checks that its role
// exists in the same transaction that writes it.
type YDBRoles struct {
	driver *ydb.Driver
}

var _ domain.RoleRepository = (*YDBRoles)(nil)

// NewYDBRoles builds a YDBRoles repository over the driver.
func NewYDBRoles(d *ydb.Driver) *YDBRoles {
	return &YDBRoles{driver: d}
}

// ydbRoleColumns is the projection ydbRoleFromRow reads, in order.
const ydbRoleColumns = `id, tenant_id, name, permissions, created_at, updated_at`

const ydbSelectRole = `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
SELECT ` + ydbRoleColumns + `
FROM roles
WHERE tenant_id = $tenant_id AND id = $id;`

// Add persists a new role under a fresh UUID and returns the id. It returns
// domain.ErrRoleNameTaken when the tenant already has a role with that name.
func (r *YDBRoles) Add(ctx context.Context, tenantID string, role *domain.Role) (string, error) {
	id := uuid.NewString()
	err := r.driver.Query().Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DECLARE $name AS Utf8;
DECLARE $permissions AS Utf8;
INSERT INTO roles (tenant_id, id, name, permissions, created_at, updated_at)
VALUES ($tenant_id, $id, $name, $permissions, CurrentUtcTimestamp(), CurrentUtcTimestamp());`,
		query.WithParameters(ydb.ParamsBuilder().
			Param("$tenant_id").Text(tenantID).
			Param("$id").Text(id).
			Param("$name").Text(role.Name()).
			Param("$permissions").Text(ydbPermissions(role.Permissions())).
			Build()))
	// Only the name index can collide: the id is fresh.
	if db.IsYDBConflict(err) {
		return "", domain.ErrRoleNameTaken
	}
	if err != nil {
		return "", fmt.Errorf("inserting role: %w", err)
	}
	return id, nil
}

// Update loads the role, runs fn, and persists the result in one serializable
// transaction. It returns domain.ErrRoleNameTaken when the new name belongs to
// another role of the tenant. The SDK retries the transaction on conflict, so
// fn may run more than once and must be retry-safe.
func (r *YDBRoles) Update(ctx context.Context, tenantID, id string,
	fn func(*domain.Role) (*domain.Role, error)) error {

	err := r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		loaded, err := ydbRoleFromRow(tx.QueryRow(ctx, ydbSelectRole, ydbTenantAndID(tenantID, id)))
		if err != nil {
			return err
		}
		updated, err := fn(loaded)
		if err != nil {
			return err
		}
		err = tx.Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DECLARE $name AS Utf8;
DECLARE $permissions AS Utf8;
UPDATE roles SET name = $name, permissions = $permissions, updated_at = CurrentUtcTimestamp()
WHERE tenant_id = $tenant_id AND id = $id;`,
			query.WithParameters(ydb.ParamsBuilder().
				Param("$tenant_id").Text(tenantID).
				Param("$id").Text(id).
				Param("$name").Text(updated.Name()).
				Param("$permissions").Text(ydbPermissions(updated.Permissions())).
				Build()))
		// An UPDATE onto another role's name hits the unique name index at
		// the statement (spec "Plan B4 planning spike").
		if db.IsYDBConflict(err) {
			return domain.ErrRoleNameTaken
		}
		if err != nil {
			return fmt.Errorf("updating role: %w", err)
		}
		return nil
	})
	// This covers a YDB that reports the collision at commit.
	if db.IsYDBConflict(err) {
		return domain.ErrRoleNameTaken
	}
	return err
}

// Delete removes the role. It returns domain.ErrRoleInUse when the role is
// still assigned, tenant-wide or per list, and domain.ErrRoleNotFound when the
// tenant has no such role. A concurrent assignment of the role aborts one of
// the two transactions, and the SDK retries it.
func (r *YDBRoles) Delete(ctx context.Context, tenantID, id string) error {
	return r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		// ponytail: each check scans the tenant's assignments (no role_id
		// index); add GLOBAL SYNC indexes on (tenant_id, role_id) if a tenant
		// has many users.
		for _, table := range []string{"user_roles", "user_list_roles"} {
			_, err := tx.QueryRow(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $role_id AS Utf8;
SELECT role_id FROM `+table+`
WHERE tenant_id = $tenant_id AND role_id = $role_id
LIMIT 1;`,
				query.WithParameters(ydb.ParamsBuilder().
					Param("$tenant_id").Text(tenantID).
					Param("$role_id").Text(id).
					Build()))
			if err == nil {
				return domain.ErrRoleInUse
			}
			if !errors.Is(err, query.ErrNoRows) {
				return fmt.Errorf("checking role assignments: %w", err)
			}
		}
		_, err := tx.QueryRow(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DELETE FROM roles
WHERE tenant_id = $tenant_id AND id = $id
RETURNING id;`, ydbTenantAndID(tenantID, id))
		if errors.Is(err, query.ErrNoRows) {
			return domain.ErrRoleNotFound
		}
		if err != nil {
			return fmt.Errorf("deleting role: %w", err)
		}
		return nil
	})
}

// Get returns the role, or domain.ErrRoleNotFound.
func (r *YDBRoles) Get(ctx context.Context, tenantID, id string) (*domain.Role, error) {
	return ydbRoleFromRow(r.driver.Query().QueryRow(ctx, ydbSelectRole, ydbTenantAndID(tenantID, id)))
}

// All returns every role in the tenant, ordered by name.
func (r *YDBRoles) All(ctx context.Context, tenantID string) ([]*domain.Role, error) {
	rs, err := r.driver.Query().QueryResultSet(ctx, `DECLARE $tenant_id AS Utf8;
SELECT `+ydbRoleColumns+`
FROM roles
WHERE tenant_id = $tenant_id
ORDER BY name;`, ydbTenantParam(tenantID))
	if err != nil {
		return nil, fmt.Errorf("listing roles: %w", err)
	}
	defer func() { _ = rs.Close(ctx) }()
	var roles []*domain.Role
	for row, err := range rs.Rows(ctx) {
		if err != nil {
			return nil, fmt.Errorf("listing roles: %w", err)
		}
		role, err := scanYDBRole(row)
		if err != nil {
			return nil, err
		}
		roles = append(roles, role)
	}
	return roles, nil
}

// AssignTenantRole sets a user's tenant-level role, replacing any previous
// one. It returns domain.ErrRoleNotFound when the tenant has no such role,
// standing in for the Postgres foreign key.
func (r *YDBRoles) AssignTenantRole(ctx context.Context, tenantID, userID, roleID string) error {
	return r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		if err := ydbRoleExists(ctx, tx, tenantID, roleID); err != nil {
			return err
		}
		if err := tx.Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $user_id AS Utf8;
DECLARE $role_id AS Utf8;
UPSERT INTO user_roles (tenant_id, user_id, role_id)
VALUES ($tenant_id, $user_id, $role_id);`,
			query.WithParameters(ydb.ParamsBuilder().
				Param("$tenant_id").Text(tenantID).
				Param("$user_id").Text(userID).
				Param("$role_id").Text(roleID).
				Build())); err != nil {
			return fmt.Errorf("assigning tenant role: %w", err)
		}
		return nil
	})
}

// AssignListRole grants a user a per-list role, replacing any previous role
// on that list. It returns domain.ErrRoleNotFound when the tenant has no such
// role, standing in for the Postgres foreign key. The list is not checked: the
// lists table belongs to the audience context.
func (r *YDBRoles) AssignListRole(ctx context.Context, tenantID, userID, listID, roleID string) error {
	return r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		if err := ydbRoleExists(ctx, tx, tenantID, roleID); err != nil {
			return err
		}
		if err := tx.Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $user_id AS Utf8;
DECLARE $list_id AS Utf8;
DECLARE $role_id AS Utf8;
UPSERT INTO user_list_roles (tenant_id, user_id, list_id, role_id)
VALUES ($tenant_id, $user_id, $list_id, $role_id);`,
			query.WithParameters(ydb.ParamsBuilder().
				Param("$tenant_id").Text(tenantID).
				Param("$user_id").Text(userID).
				Param("$list_id").Text(listID).
				Param("$role_id").Text(roleID).
				Build())); err != nil {
			return fmt.Errorf("assigning list role: %w", err)
		}
		return nil
	})
}

// RemoveListRole removes a user's per-list role. Removing a role the user does
// not hold is a no-op.
func (r *YDBRoles) RemoveListRole(ctx context.Context, tenantID, userID, listID string) error {
	if err := r.driver.Query().Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $user_id AS Utf8;
DECLARE $list_id AS Utf8;
DELETE FROM user_list_roles
WHERE tenant_id = $tenant_id AND user_id = $user_id AND list_id = $list_id;`,
		query.WithParameters(ydb.ParamsBuilder().
			Param("$tenant_id").Text(tenantID).
			Param("$user_id").Text(userID).
			Param("$list_id").Text(listID).
			Build())); err != nil {
		return fmt.Errorf("removing list role: %w", err)
	}
	return nil
}

// EffectiveFor loads a user's tenant-level permissions and the permissions of
// each per-list role they hold, in one transaction. The joins match the role
// on (tenant_id, id), so another tenant's role never contributes.
func (r *YDBRoles) EffectiveFor(ctx context.Context, tenantID, userID string) (
	[]domain.Permission, map[string][]domain.Permission, error) {

	params := query.WithParameters(ydb.ParamsBuilder().
		Param("$tenant_id").Text(tenantID).
		Param("$user_id").Text(userID).
		Build())
	var tenantPerms []domain.Permission
	var listPerms map[string][]domain.Permission
	err := r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		// The user has at most one tenant-level role. No row means none.
		tenantLevel := permsFromStrings(nil)
		row, err := tx.QueryRow(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $user_id AS Utf8;
SELECT r.permissions
FROM user_roles AS ur
JOIN roles AS r ON r.tenant_id = ur.tenant_id AND r.id = ur.role_id
WHERE ur.tenant_id = $tenant_id AND ur.user_id = $user_id;`, params)
		switch {
		case errors.Is(err, query.ErrNoRows):
		case err != nil:
			return fmt.Errorf("loading tenant role: %w", err)
		default:
			var raw string
			if err := row.Scan(&raw); err != nil {
				return fmt.Errorf("scanning tenant role: %w", err)
			}
			if tenantLevel, err = ydbPermissionsFrom(raw); err != nil {
				return err
			}
		}

		rs, err := tx.QueryResultSet(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $user_id AS Utf8;
SELECT ulr.list_id, r.permissions
FROM user_list_roles AS ulr
JOIN roles AS r ON r.tenant_id = ulr.tenant_id AND r.id = ulr.role_id
WHERE ulr.tenant_id = $tenant_id AND ulr.user_id = $user_id;`, params)
		if err != nil {
			return fmt.Errorf("loading list roles: %w", err)
		}
		// Built locally and assigned at the end: the SDK may re-run the
		// closure, and a retry must start from an empty map.
		perList := map[string][]domain.Permission{}
		for row, err := range rs.Rows(ctx) {
			if err != nil {
				return fmt.Errorf("loading list roles: %w", err)
			}
			var listID, raw string
			if err := row.Scan(&listID, &raw); err != nil {
				return fmt.Errorf("scanning list role: %w", err)
			}
			perms, err := ydbPermissionsFrom(raw)
			if err != nil {
				return err
			}
			perList[listID] = perms
		}
		tenantPerms, listPerms = tenantLevel, perList
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return tenantPerms, listPerms, nil
}

// ydbRoleExists returns domain.ErrRoleNotFound unless the tenant has the role.
// Reading the role inside the assigning transaction makes a concurrent Delete
// of it abort one of the two, as the Postgres foreign key would.
func ydbRoleExists(ctx context.Context, tx query.TxActor, tenantID, roleID string) error {
	_, err := tx.QueryRow(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
SELECT id FROM roles
WHERE tenant_id = $tenant_id AND id = $id;`, ydbTenantAndID(tenantID, roleID))
	if errors.Is(err, query.ErrNoRows) {
		return domain.ErrRoleNotFound
	}
	if err != nil {
		return fmt.Errorf("checking role: %w", err)
	}
	return nil
}

// ydbPermissions encodes a permission set as the JSON array the permissions
// columns store.
func ydbPermissions(perms []domain.Permission) string {
	raw, _ := json.Marshal(permStrings(perms)) // a []string always encodes
	return string(raw)
}

// ydbPermissionsFrom decodes a permissions column.
func ydbPermissionsFrom(raw string) ([]domain.Permission, error) {
	var perms []string
	if err := json.Unmarshal([]byte(raw), &perms); err != nil {
		return nil, fmt.Errorf("decoding permissions: %w", err)
	}
	return permsFromStrings(perms), nil
}

// ydbRoleFromRow maps a single-row read to the role, translating "no row"
// into domain.ErrRoleNotFound.
func ydbRoleFromRow(row query.Row, err error) (*domain.Role, error) {
	if errors.Is(err, query.ErrNoRows) {
		return nil, domain.ErrRoleNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("loading role: %w", err)
	}
	return scanYDBRole(row)
}

// scanYDBRole reads one row in ydbRoleColumns order.
func scanYDBRole(row query.Row) (*domain.Role, error) {
	var id, tenantID, name, raw string
	var createdAt, updatedAt time.Time
	if err := row.Scan(&id, &tenantID, &name, &raw, &createdAt, &updatedAt); err != nil {
		return nil, fmt.Errorf("scanning role: %w", err)
	}
	perms, err := ydbPermissionsFrom(raw)
	if err != nil {
		return nil, err
	}
	return domain.HydrateRole(id, tenantID, name, perms, createdAt, updatedAt), nil
}
```

- [ ] **Step 5: Run the suite to verify it passes**

Run: `export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock" && go test ./internal/iam/adapters/ -count=1 -v -timeout 8m 2>&1 | grep -E -- '--- (PASS|FAIL)|^(ok|FAIL)' | grep -E 'FAIL|/pg|/ydb|^ok' | sort`

Expected:
- 30 `--- PASS` lines: every `TestRoles*`, `TestUsers*`, `TestSessions*` and `TestAudit*` on both `/pg` and `/ydb`.
- No `FAIL` line. If `TestAPIKeysAddByTokenHashRevoke` or `TestAPIKeysAll` fails with `api_keys_token_hash_key`, see the known flakes.
- The package is `ok`.

Three cases are not yours to fix. Report BLOCKED with the verbatim output:
- **A `/pg` subtest fails.** In particular, `TestRolesAssignUnknownRoleFails/pg` failing means Postgres accepted an assignment to a role that does not exist.
- **`TestRolesUpdate/ydb` gets an error other than `ErrRoleNameTaken` on the rename collision.** Print the error with `%+v` in the report. It contradicts the spec's "Plan B4 planning spike" finding.
- **`TestRolesEffectiveFor/ydb` fails with a YQL syntax error on the two-column `JOIN`.** That also contradicts the spike.

Any other `/ydb` failure: fix only `roles_ydb.go`, and record the verbatim error and the fix in the report.

- [ ] **Step 6: Full verification and commit**

Run: `export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock" && go build ./... && go vet ./... && go test ./... -timeout 15m 2>&1 | grep -vE '^(ok|\?)\s'`

Expected: no output. In other words, no failing package.

Run: `golangci-lint run`

Expected: `0 issues.`

Run: `PATH="$HOME/go/bin:$PATH" make lint-arch`

Expected: the last line is `[arch] clean`.

Run: `git diff --stat HEAD -- go.mod go.sum`

Expected: no output.

```bash
gofmt -l internal/iam/
git add internal/iam/adapters/roles_ydb.go internal/iam/adapters/roles_test.go internal/iam/adapters/helpers_test.go
git commit -m "feat(iam): add the YDB role repository under a pg+ydb contract suite"
```

`gofmt -l` must print nothing. Afterwards, `git status --short` must print nothing: `roles_pg_test.go` is gone.

---

### Task 3: `YDBAPIKeys`, `YDBRecoveryCodes` and their contract suites

**Files:**
- Create: `internal/iam/adapters/apikeys_ydb.go`
- Create: `internal/iam/adapters/recovery_codes_ydb.go`
- Modify: `internal/iam/adapters/helpers_test.go` (rewritten: adds the `apiKeys` and `recoveryCodes` fields, deletes `seedTenantUser`)
- Rename: `internal/iam/adapters/apikeys_pg_test.go` → `internal/iam/adapters/apikeys_test.go` (rewritten)
- Rename: `internal/iam/adapters/recovery_codes_pg_test.go` → `internal/iam/adapters/recovery_codes_test.go` (rewritten)

**Interfaces:**
- Consumes (Task 1):
  - the `api_keys` table (primary key `(tenant_id, id)`, unique index `idx_api_keys_token_hash` on `token_hash`) and `recovery_codes` (primary key `(tenant_id, user_id, code_hash)`);
  - `ydbTenantParam` and `ydbTenantAndID`;
  - the harness.
- Consumes (Task 2): `ydbPermissions(perms []domain.Permission) string` and `ydbPermissionsFrom(raw string) ([]domain.Permission, error)`, and the `roles`/`newList` harness fields.
- Consumes the ports `domain.APIKeyRepository` and `domain.RecoveryCodeRepository`, unchanged.
- Produces:
  ```go
  // package adapters (internal/iam/adapters)
  type YDBAPIKeys struct{ /* unexported */ }
  func NewYDBAPIKeys(d *ydb.Driver) *YDBAPIKeys
  type YDBRecoveryCodes struct{ /* unexported */ }
  func NewYDBRecoveryCodes(d *ydb.Driver) *YDBRecoveryCodes
  func ydbTenantAndUser(tenantID, userID string) query.ExecuteOption // binds $tenant_id, $user_id
  // package adapters_test: backend gains
  apiKeys       func(t *testing.T) domain.APIKeyRepository
  recoveryCodes func(t *testing.T) domain.RecoveryCodeRepository
  // and seedTenantUser is deleted
  ```

- [ ] **Step 1: Complete the harness**

Replace the contents of `internal/iam/adapters/helpers_test.go` with the code below. It is Task 2's harness plus the `apiKeys` and `recoveryCodes` fields. `seedTenantUser` is gone, because Steps 2 and 3 remove its last callers.

```go
package adapters_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
	"github.com/nikolaymatrosov/nvelope/internal/iam/adapters"
	"github.com/nikolaymatrosov/nvelope/internal/iam/domain"
	"github.com/nikolaymatrosov/nvelope/internal/platform/tenantdb"
)

// backend is one storage implementation of the iam repositories under
// contract test. Every contract test body runs once per backend, so the
// Postgres and YDB adapters are held to the same behaviour.
type backend struct {
	name          string
	users         func(t *testing.T) domain.UserRepository
	sessions      func(t *testing.T) domain.SessionRepository
	roles         func(t *testing.T) domain.RoleRepository
	apiKeys       func(t *testing.T) domain.APIKeyRepository
	recoveryCodes func(t *testing.T) domain.RecoveryCodeRepository
	audit         func(t *testing.T) domain.AuditRepository
	// newTenant, newPlatformUser and newList return ids of rows other contexts
	// own. On Postgres the iam tables carry foreign keys to them, so the pg
	// backend inserts real rows. YDB has no foreign keys and no iam query reads
	// those tables, so a bare UUID stands in.
	newTenant       func(t *testing.T) string
	newPlatformUser func(t *testing.T) string
	newList         func(t *testing.T, tenantID string) string
}

var backends = []backend{
	{
		name: "pg",
		users: func(t *testing.T) domain.UserRepository {
			return adapters.NewUsers(dbtest.AppPool(t))
		},
		sessions: func(t *testing.T) domain.SessionRepository {
			return adapters.NewSessions(dbtest.AppPool(t))
		},
		roles: func(t *testing.T) domain.RoleRepository {
			return adapters.NewRoles(dbtest.AppPool(t))
		},
		apiKeys: func(t *testing.T) domain.APIKeyRepository {
			return adapters.NewAPIKeys(dbtest.AppPool(t))
		},
		recoveryCodes: func(t *testing.T) domain.RecoveryCodeRepository {
			return adapters.NewRecoveryCodes(dbtest.AppPool(t))
		},
		audit: func(t *testing.T) domain.AuditRepository {
			return adapters.NewAudit(dbtest.AppPool(t))
		},
		newTenant:       func(t *testing.T) string { return seedTenant(t, dbtest.AppPool(t)) },
		newPlatformUser: func(t *testing.T) string { return seedPlatformUser(t, dbtest.AppPool(t)) },
		newList:         func(t *testing.T, tenantID string) string { return seedList(t, dbtest.AppPool(t), tenantID) },
	},
	{
		name: "ydb",
		users: func(t *testing.T) domain.UserRepository {
			return adapters.NewYDBUsers(dbtest.YDBMigrated(t))
		},
		sessions: func(t *testing.T) domain.SessionRepository {
			return adapters.NewYDBSessions(dbtest.YDBMigrated(t))
		},
		roles: func(t *testing.T) domain.RoleRepository {
			return adapters.NewYDBRoles(dbtest.YDBMigrated(t))
		},
		apiKeys: func(t *testing.T) domain.APIKeyRepository {
			return adapters.NewYDBAPIKeys(dbtest.YDBMigrated(t))
		},
		recoveryCodes: func(t *testing.T) domain.RecoveryCodeRepository {
			return adapters.NewYDBRecoveryCodes(dbtest.YDBMigrated(t))
		},
		audit: func(t *testing.T) domain.AuditRepository {
			return adapters.NewYDBAudit(dbtest.YDBMigrated(t))
		},
		newTenant:       func(*testing.T) string { return uuid.NewString() },
		newPlatformUser: func(*testing.T) string { return uuid.NewString() },
		newList:         func(*testing.T, string) string { return uuid.NewString() },
	},
}

// eachBackend runs body once per backend, as parallel subtests named after it.
func eachBackend(t *testing.T, body func(t *testing.T, b backend)) {
	t.Helper()
	for _, b := range backends {
		t.Run(b.name, func(t *testing.T) {
			t.Parallel()
			body(t, b)
		})
	}
}

// newTenantUser adds a tenant-plane user with a fresh address through the
// backend's user repository and returns its id.
func newTenantUser(t *testing.T, b backend, tenantID string) string {
	t.Helper()
	u, err := domain.NewTenantUser(tenantID, b.newPlatformUser(t), dbtest.RandString()+"@example.com", "Member")
	require.NoError(t, err)
	id, err := b.users(t).Add(context.Background(), tenantID, u)
	require.NoError(t, err)
	return id
}

// seedList inserts a list row inside a tenant-bound transaction and returns
// its id — needed to satisfy the user_list_roles foreign key.
func seedList(t *testing.T, pool *pgxpool.Pool, tenantID string) string {
	t.Helper()
	var id string
	require.NoError(t, tenantdb.WithTenant(context.Background(), pool, tenantID,
		func(ctx context.Context, tx pgx.Tx) error {
			return tx.QueryRow(ctx,
				"INSERT INTO lists (tenant_id, name) VALUES ($1, $2) RETURNING id",
				tenantID, "L-"+dbtest.RandString()).Scan(&id)
		}))
	return id
}

// seedTenant inserts a tenant row directly and returns its id.
func seedTenant(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var id string
	require.NoError(t, pool.QueryRow(context.Background(),
		`INSERT INTO tenants (name, slug, status) VALUES ('Workspace', $1, 'active') RETURNING id`,
		"iam-"+dbtest.RandString()).Scan(&id))
	return id
}

// seedPlatformUser inserts a control-plane user row directly and returns its id.
func seedPlatformUser(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var id string
	require.NoError(t, pool.QueryRow(context.Background(),
		`INSERT INTO platform_users (email, password_hash, name)
		 VALUES ($1, 'hash', 'User') RETURNING id`,
		dbtest.RandString()+"@example.com").Scan(&id))
	return id
}
```

- [ ] **Step 2: Rewrite the API key tests as a contract suite**

```bash
git mv internal/iam/adapters/apikeys_pg_test.go internal/iam/adapters/apikeys_test.go
```

Replace the contents of `internal/iam/adapters/apikeys_test.go` with the code below.
- Token hashes are now random. The fixed ones collided on a reattached pg container (see the known flakes).
- `TestAPIKeysAreTenantScoped` is new.
- `TestAPIKeysAddByTokenHashRevoke` now checks every field, `TouchLastUsed`, and revoking an unknown or malformed id.
- `TestAPIKeysAll` now pins the newest-first order.

```go
package adapters_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
	"github.com/nikolaymatrosov/nvelope/internal/iam/domain"
)

func TestAPIKeysAddByTokenHashRevoke(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.apiKeys(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		userID := newTenantUser(t, b, tenantID)

		hash := "hash-" + dbtest.RandString()
		key, err := domain.NewAPIKey(tenantID, "CI", hash,
			[]domain.Permission{domain.PermSubscribersGet}, userID)
		require.NoError(t, err)
		id, err := repo.Add(ctx, tenantID, key)
		require.NoError(t, err)
		require.NotEmpty(t, id, "the adapter assigns an id")

		got, err := repo.ByTokenHash(ctx, tenantID, hash)
		require.NoError(t, err)
		require.Equal(t, id, got.ID())
		require.Equal(t, tenantID, got.TenantID())
		require.Equal(t, "CI", got.Name())
		require.Equal(t, hash, got.TokenHash())
		require.Equal(t, []domain.Permission{domain.PermSubscribersGet}, got.Permissions())
		require.Equal(t, userID, got.CreatedBy())
		require.WithinDuration(t, time.Now(), got.CreatedAt(), time.Minute, "the database stamps the insert")
		require.Nil(t, got.LastUsedAt())
		require.False(t, got.IsRevoked())

		require.NoError(t, repo.TouchLastUsed(ctx, tenantID, id))
		got, err = repo.ByTokenHash(ctx, tenantID, hash)
		require.NoError(t, err)
		require.NotNil(t, got.LastUsedAt())
		require.WithinDuration(t, time.Now(), *got.LastUsedAt(), time.Minute)

		require.NoError(t, repo.Revoke(ctx, tenantID, id))
		got, err = repo.ByTokenHash(ctx, tenantID, hash)
		require.NoError(t, err)
		require.True(t, got.IsRevoked())
		require.WithinDuration(t, time.Now(), *got.RevokedAt(), time.Minute)

		// Revoking again, or an unknown key, reports not found.
		require.ErrorIs(t, repo.Revoke(ctx, tenantID, id), domain.ErrAPIKeyNotFound)
		require.ErrorIs(t, repo.Revoke(ctx, tenantID, uuid.NewString()), domain.ErrAPIKeyNotFound)
		require.ErrorIs(t, repo.Revoke(ctx, tenantID, "not-a-uuid"), domain.ErrAPIKeyNotFound,
			"a malformed id is an opaque not-found")
	})
}

func TestAPIKeysByTokenHashMissing(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		_, err := b.apiKeys(t).ByTokenHash(context.Background(), b.newTenant(t), "nope")
		require.ErrorIs(t, err, domain.ErrAPIKeyNotFound)
	})
}

func TestAPIKeysAll(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.apiKeys(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		userID := newTenantUser(t, b, tenantID)

		for _, name := range []string{"one", "two"} {
			key, err := domain.NewAPIKey(tenantID, name, "hash-"+dbtest.RandString(), nil, userID)
			require.NoError(t, err)
			_, err = repo.Add(ctx, tenantID, key)
			require.NoError(t, err)
		}
		keys, err := repo.All(ctx, tenantID)
		require.NoError(t, err)
		require.Len(t, keys, 2)
		require.Equal(t, "two", keys[0].Name(), "newest first")
		require.Equal(t, "one", keys[1].Name())
		require.Empty(t, keys[0].Permissions())
	})
}

func TestAPIKeysAreTenantScoped(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.apiKeys(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		otherTenant := b.newTenant(t)
		userID := newTenantUser(t, b, tenantID)

		hash := "hash-" + dbtest.RandString()
		key, err := domain.NewAPIKey(tenantID, "CI", hash, nil, userID)
		require.NoError(t, err)
		id, err := repo.Add(ctx, tenantID, key)
		require.NoError(t, err)

		_, err = repo.ByTokenHash(ctx, otherTenant, hash)
		require.ErrorIs(t, err, domain.ErrAPIKeyNotFound)
		require.ErrorIs(t, repo.Revoke(ctx, otherTenant, id), domain.ErrAPIKeyNotFound)
		require.NoError(t, repo.TouchLastUsed(ctx, otherTenant, id))
		others, err := repo.All(ctx, otherTenant)
		require.NoError(t, err)
		require.Empty(t, others)

		got, err := repo.ByTokenHash(ctx, tenantID, hash)
		require.NoError(t, err)
		require.False(t, got.IsRevoked(), "another tenant cannot revoke the key")
		require.Nil(t, got.LastUsedAt(), "another tenant cannot touch the key")
	})
}
```

- [ ] **Step 3: Rewrite the recovery code tests as a contract suite**

```bash
git mv internal/iam/adapters/recovery_codes_pg_test.go internal/iam/adapters/recovery_codes_test.go
```

Replace the contents of `internal/iam/adapters/recovery_codes_test.go` with the code below.
- `TestRecoveryCodesAreTenantScoped` is new.
- `TestRecoveryCodesAddBatchReplacesPriorCodes` now re-issues a consumed code in the new batch. That runs a delete and an insert of the same key in one transaction.
- `TestRecoveryCodesDeleteForUser` now checks that another user's codes survive.

```go
package adapters_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRecoveryCodesAddBatchAndConsume(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.recoveryCodes(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		userID := newTenantUser(t, b, tenantID)

		require.NoError(t, repo.AddBatch(ctx, tenantID, userID, []string{"h1", "h2", "h3"}))

		consumed, err := repo.Consume(ctx, tenantID, userID, "h2")
		require.NoError(t, err)
		require.True(t, consumed, "an unused code is consumable")

		consumed, err = repo.Consume(ctx, tenantID, userID, "h2")
		require.NoError(t, err)
		require.False(t, consumed, "a code cannot be consumed twice")

		consumed, err = repo.Consume(ctx, tenantID, userID, "unknown")
		require.NoError(t, err)
		require.False(t, consumed, "an unknown code is not consumable")

		consumed, err = repo.Consume(ctx, tenantID, userID, "h1")
		require.NoError(t, err)
		require.True(t, consumed, "consuming one code leaves the others usable")
	})
}

func TestRecoveryCodesAddBatchReplacesPriorCodes(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.recoveryCodes(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		userID := newTenantUser(t, b, tenantID)

		require.NoError(t, repo.AddBatch(ctx, tenantID, userID, []string{"old", "kept"}))
		consumed, err := repo.Consume(ctx, tenantID, userID, "kept")
		require.NoError(t, err)
		require.True(t, consumed)

		require.NoError(t, repo.AddBatch(ctx, tenantID, userID, []string{"new", "kept"}))

		consumed, err = repo.Consume(ctx, tenantID, userID, "old")
		require.NoError(t, err)
		require.False(t, consumed, "a re-issued batch invalidates the prior codes")

		consumed, err = repo.Consume(ctx, tenantID, userID, "new")
		require.NoError(t, err)
		require.True(t, consumed)

		consumed, err = repo.Consume(ctx, tenantID, userID, "kept")
		require.NoError(t, err)
		require.True(t, consumed, "a code re-issued in the new batch is unused again")
	})
}

func TestRecoveryCodesDeleteForUser(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.recoveryCodes(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		userID := newTenantUser(t, b, tenantID)
		otherUser := newTenantUser(t, b, tenantID)

		require.NoError(t, repo.AddBatch(ctx, tenantID, userID, []string{"h1"}))
		require.NoError(t, repo.AddBatch(ctx, tenantID, otherUser, []string{"h1"}))
		require.NoError(t, repo.DeleteForUser(ctx, tenantID, userID))

		consumed, err := repo.Consume(ctx, tenantID, userID, "h1")
		require.NoError(t, err)
		require.False(t, consumed)

		consumed, err = repo.Consume(ctx, tenantID, otherUser, "h1")
		require.NoError(t, err)
		require.True(t, consumed, "another user's codes are untouched")
	})
}

func TestRecoveryCodesAreTenantScoped(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.recoveryCodes(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		otherTenant := b.newTenant(t)
		userID := newTenantUser(t, b, tenantID)

		require.NoError(t, repo.AddBatch(ctx, tenantID, userID, []string{"h1"}))

		consumed, err := repo.Consume(ctx, otherTenant, userID, "h1")
		require.NoError(t, err)
		require.False(t, consumed, "another tenant cannot consume the code")
		require.NoError(t, repo.DeleteForUser(ctx, otherTenant, userID))

		consumed, err = repo.Consume(ctx, tenantID, userID, "h1")
		require.NoError(t, err)
		require.True(t, consumed, "another tenant cannot delete the code")
	})
}
```

- [ ] **Step 4: Run the suites to verify they fail**

Run: `go vet ./internal/iam/adapters/`

Expected: build failure `undefined: adapters.NewYDBAPIKeys` and `undefined: adapters.NewYDBRecoveryCodes`.

- [ ] **Step 5: Implement `YDBAPIKeys`**

Create `internal/iam/adapters/apikeys_ydb.go`:

```go
package adapters

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	ydb "github.com/ydb-platform/ydb-go-sdk/v3"
	"github.com/ydb-platform/ydb-go-sdk/v3/query"

	"github.com/nikolaymatrosov/nvelope/internal/iam/domain"
)

// YDBAPIKeys is the YDB-backed implementation of domain.APIKeyRepository, for
// the serverless runtime. Only the hash of a key is stored, and its
// permissions are a JSON array in a Utf8 column, as for roles.
type YDBAPIKeys struct {
	driver *ydb.Driver
}

var _ domain.APIKeyRepository = (*YDBAPIKeys)(nil)

// NewYDBAPIKeys builds a YDBAPIKeys repository over the driver.
func NewYDBAPIKeys(d *ydb.Driver) *YDBAPIKeys {
	return &YDBAPIKeys{driver: d}
}

// ydbAPIKeyColumns is the projection scanYDBAPIKey reads, in order.
const ydbAPIKeyColumns = `id, tenant_id, name, token_hash, permissions, created_by,
    created_at, last_used_at, revoked_at`

// Add persists a new API key under a fresh UUID and returns the id.
func (r *YDBAPIKeys) Add(ctx context.Context, tenantID string, k *domain.APIKey) (string, error) {
	id := uuid.NewString()
	if err := r.driver.Query().Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DECLARE $name AS Utf8;
DECLARE $token_hash AS Utf8;
DECLARE $permissions AS Utf8;
DECLARE $created_by AS Utf8;
INSERT INTO api_keys (tenant_id, id, name, token_hash, permissions, created_by, created_at)
VALUES ($tenant_id, $id, $name, $token_hash, $permissions, $created_by, CurrentUtcTimestamp());`,
		query.WithParameters(ydb.ParamsBuilder().
			Param("$tenant_id").Text(tenantID).
			Param("$id").Text(id).
			Param("$name").Text(k.Name()).
			Param("$token_hash").Text(k.TokenHash()).
			Param("$permissions").Text(ydbPermissions(k.Permissions())).
			Param("$created_by").Text(k.CreatedBy()).
			Build())); err != nil {
		return "", fmt.Errorf("inserting API key: %w", err)
	}
	return id, nil
}

// ByTokenHash returns the tenant's API key for a token hash, revoked or not,
// or domain.ErrAPIKeyNotFound.
func (r *YDBAPIKeys) ByTokenHash(ctx context.Context, tenantID, tokenHash string) (*domain.APIKey, error) {
	row, err := r.driver.Query().QueryRow(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $token_hash AS Utf8;
SELECT `+ydbAPIKeyColumns+`
FROM api_keys VIEW idx_api_keys_token_hash
WHERE token_hash = $token_hash AND tenant_id = $tenant_id;`,
		query.WithParameters(ydb.ParamsBuilder().
			Param("$tenant_id").Text(tenantID).
			Param("$token_hash").Text(tokenHash).
			Build()))
	if errors.Is(err, query.ErrNoRows) {
		return nil, domain.ErrAPIKeyNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("loading API key: %w", err)
	}
	return scanYDBAPIKey(row)
}

// Revoke marks the API key revoked. It returns domain.ErrAPIKeyNotFound when
// the tenant has no such key or the key is already revoked: UPDATE ...
// RETURNING yields no row then.
func (r *YDBAPIKeys) Revoke(ctx context.Context, tenantID, id string) error {
	_, err := r.driver.Query().QueryRow(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
UPDATE api_keys SET revoked_at = CurrentUtcTimestamp()
WHERE tenant_id = $tenant_id AND id = $id AND revoked_at IS NULL
RETURNING id;`, ydbTenantAndID(tenantID, id))
	if errors.Is(err, query.ErrNoRows) {
		return domain.ErrAPIKeyNotFound
	}
	if err != nil {
		return fmt.Errorf("revoking API key: %w", err)
	}
	return nil
}

// TouchLastUsed records that the key was just used to authenticate. Touching
// an unknown key is a no-op.
func (r *YDBAPIKeys) TouchLastUsed(ctx context.Context, tenantID, id string) error {
	if err := r.driver.Query().Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
UPDATE api_keys SET last_used_at = CurrentUtcTimestamp()
WHERE tenant_id = $tenant_id AND id = $id;`, ydbTenantAndID(tenantID, id)); err != nil {
		return fmt.Errorf("touching API key: %w", err)
	}
	return nil
}

// All returns every API key in the tenant, newest first.
func (r *YDBAPIKeys) All(ctx context.Context, tenantID string) ([]*domain.APIKey, error) {
	rs, err := r.driver.Query().QueryResultSet(ctx, `DECLARE $tenant_id AS Utf8;
SELECT `+ydbAPIKeyColumns+`
FROM api_keys
WHERE tenant_id = $tenant_id
ORDER BY created_at DESC;`, ydbTenantParam(tenantID))
	if err != nil {
		return nil, fmt.Errorf("listing API keys: %w", err)
	}
	defer func() { _ = rs.Close(ctx) }()
	var keys []*domain.APIKey
	for row, err := range rs.Rows(ctx) {
		if err != nil {
			return nil, fmt.Errorf("listing API keys: %w", err)
		}
		k, err := scanYDBAPIKey(row)
		if err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, nil
}

// scanYDBAPIKey reads one row in ydbAPIKeyColumns order.
func scanYDBAPIKey(row query.Row) (*domain.APIKey, error) {
	var id, tenantID, name, tokenHash, raw, createdBy string
	var createdAt time.Time
	var lastUsedAt, revokedAt *time.Time
	if err := row.Scan(&id, &tenantID, &name, &tokenHash, &raw, &createdBy,
		&createdAt, &lastUsedAt, &revokedAt); err != nil {
		return nil, fmt.Errorf("scanning API key: %w", err)
	}
	perms, err := ydbPermissionsFrom(raw)
	if err != nil {
		return nil, err
	}
	return domain.HydrateAPIKey(id, tenantID, name, tokenHash, perms,
		createdBy, createdAt, lastUsedAt, revokedAt), nil
}
```

- [ ] **Step 6: Implement `YDBRecoveryCodes`**

Create `internal/iam/adapters/recovery_codes_ydb.go`:

```go
package adapters

import (
	"context"
	"errors"
	"fmt"

	ydb "github.com/ydb-platform/ydb-go-sdk/v3"
	"github.com/ydb-platform/ydb-go-sdk/v3/query"

	"github.com/nikolaymatrosov/nvelope/internal/iam/domain"
)

// YDBRecoveryCodes is the YDB-backed implementation of
// domain.RecoveryCodeRepository, for the serverless runtime. A code is keyed
// (tenant_id, user_id, code_hash), so a user's codes are one key-prefix range.
type YDBRecoveryCodes struct {
	driver *ydb.Driver
}

var _ domain.RecoveryCodeRepository = (*YDBRecoveryCodes)(nil)

// NewYDBRecoveryCodes builds a YDBRecoveryCodes repository over the driver.
func NewYDBRecoveryCodes(d *ydb.Driver) *YDBRecoveryCodes {
	return &YDBRecoveryCodes{driver: d}
}

const ydbDeleteRecoveryCodes = `DECLARE $tenant_id AS Utf8;
DECLARE $user_id AS Utf8;
DELETE FROM recovery_codes
WHERE tenant_id = $tenant_id AND user_id = $user_id;`

// AddBatch replaces a user's recovery codes with a fresh batch in one
// transaction. A retried transaction deletes and inserts the same rows again.
func (r *YDBRecoveryCodes) AddBatch(ctx context.Context, tenantID, userID string, codeHashes []string) error {
	return r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		if err := tx.Exec(ctx, ydbDeleteRecoveryCodes, ydbTenantAndUser(tenantID, userID)); err != nil {
			return fmt.Errorf("clearing recovery codes: %w", err)
		}
		// One statement per code: a batch is ten codes (recoveryCodeCount in
		// internal/iam/app/command).
		for _, hash := range codeHashes {
			if err := tx.Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $user_id AS Utf8;
DECLARE $code_hash AS Utf8;
INSERT INTO recovery_codes (tenant_id, user_id, code_hash)
VALUES ($tenant_id, $user_id, $code_hash);`,
				query.WithParameters(ydb.ParamsBuilder().
					Param("$tenant_id").Text(tenantID).
					Param("$user_id").Text(userID).
					Param("$code_hash").Text(hash).
					Build())); err != nil {
				return fmt.Errorf("inserting recovery code: %w", err)
			}
		}
		return nil
	})
}

// Consume marks a matching unused recovery code used, reporting whether one
// existed. UPDATE ... RETURNING yields a row only when it marked a code.
func (r *YDBRecoveryCodes) Consume(ctx context.Context, tenantID, userID, codeHash string) (bool, error) {
	_, err := r.driver.Query().QueryRow(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $user_id AS Utf8;
DECLARE $code_hash AS Utf8;
UPDATE recovery_codes SET used_at = CurrentUtcTimestamp()
WHERE tenant_id = $tenant_id AND user_id = $user_id AND code_hash = $code_hash AND used_at IS NULL
RETURNING code_hash;`,
		query.WithParameters(ydb.ParamsBuilder().
			Param("$tenant_id").Text(tenantID).
			Param("$user_id").Text(userID).
			Param("$code_hash").Text(codeHash).
			Build()))
	if errors.Is(err, query.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("consuming recovery code: %w", err)
	}
	return true, nil
}

// DeleteForUser removes every recovery code a user holds.
func (r *YDBRecoveryCodes) DeleteForUser(ctx context.Context, tenantID, userID string) error {
	if err := r.driver.Query().Exec(ctx, ydbDeleteRecoveryCodes, ydbTenantAndUser(tenantID, userID)); err != nil {
		return fmt.Errorf("deleting recovery codes: %w", err)
	}
	return nil
}

// ydbTenantAndUser binds the (tenant_id, user_id) key prefix.
func ydbTenantAndUser(tenantID, userID string) query.ExecuteOption {
	return query.WithParameters(ydb.ParamsBuilder().
		Param("$tenant_id").Text(tenantID).
		Param("$user_id").Text(userID).
		Build())
}
```

- [ ] **Step 7: Run the suites to verify they pass**

Run: `export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock" && go test ./internal/iam/... -count=1 -v -timeout 8m 2>&1 | grep -E -- '--- (PASS|FAIL)|^(ok|FAIL)' | grep -E 'FAIL|/pg|/ydb|^ok' | sort`

Expected:
- 46 `--- PASS` lines: all 23 contract tests on both `/pg` and `/ydb`.
- No `FAIL` line.
- `internal/iam/adapters`, `internal/iam/app/command`, `internal/iam/app/query` and `internal/iam/domain` are `ok`.

Run the same command a second time, straight away.

Expected: the same result. The run reattaches to the previous run's containers, so this proves that no fixed key collides any more.

Two cases are not yours to fix. Report BLOCKED with the verbatim output:
- **A `/pg` subtest fails.**
- **`TestRecoveryCodesAddBatchReplacesPriorCodes/ydb` fails with a conflict on `kept`.** Print the error with `%+v` in the report. It contradicts the spec's "Plan B4 planning spike" finding on a delete followed by an insert in one transaction.

Any other `/ydb` failure: fix only `apikeys_ydb.go` or `recovery_codes_ydb.go`, and record the verbatim error and the fix in the report.

- [ ] **Step 8: Full verification and commit**

Run: `export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock" && go build ./... && go vet ./... && go test ./... -timeout 15m 2>&1 | grep -vE '^(ok|\?)\s'`

Expected: no output. In other words, no failing package.

Run: `golangci-lint run`

Expected: `0 issues.`

Run: `PATH="$HOME/go/bin:$PATH" make lint-arch`

Expected: the last line is `[arch] clean`.

Run: `git diff --stat HEAD -- go.mod go.sum`

Expected: no output.

Run: `ls internal/iam/adapters/*_pg_test.go`

Expected: `no matches found` (or `No such file or directory`). Every iam suite is now a contract suite.

```bash
gofmt -l internal/iam/
git add internal/iam/adapters/apikeys_ydb.go internal/iam/adapters/recovery_codes_ydb.go internal/iam/adapters/helpers_test.go
git add internal/iam/adapters/apikeys_test.go internal/iam/adapters/recovery_codes_test.go
git commit -m "feat(iam): add the YDB API key and recovery code repositories under pg+ydb contract suites"
```

`gofmt -l` must print nothing. Afterwards, `git status --short` must print nothing.

---

## Done criteria for Plan B4

- `go build ./... && go vet ./... && go test ./...` passes. Every eachBackend test in `internal/iam/adapters` passes on both `/pg` and `/ydb`.
- `golangci-lint run` reports `0 issues.`, and `make lint-arch` reports `[arch] clean`.
- No `go.mod` or `go.sum` change.
- No `*_pg_test.go` is left in `internal/iam/adapters`.
- Three tests pass on both backends, so YDB reproduces the Postgres constraints that have no YDB construct:
  - `TestUsersAddRejectsDuplicates`: `citext` uniqueness, without an `email_key` column.
  - `TestRolesUpdate`: the unique role name on rename.
  - `TestRolesAssignUnknownRoleFails`: the assignment's foreign key to its role.
- The final report lists what stays open for the next plans:
  - **Plan C wiring.** The serverless composition root wires:
    - `NewYDBUsers`, `NewYDBSessions`, `NewYDBRoles`, `NewYDBAPIKeys` and `NewYDBRecoveryCodes` in place of the pg constructors in `buildIAM` (`internal/service/application.go:607-611`);
    - `NewYDBAudit` in place of `iamadapters.NewAudit` (`application.go:242`). The api layer shares that writer.
  - **Audience plan.** On pg, `user_list_roles.list_id` has a foreign key to `lists` with `ON DELETE CASCADE` (`000006_audience_schema.up.sql`). YDB has no cascade, so the cascade is required. `YDBLists.Delete` must run `DELETE FROM user_list_roles WHERE tenant_id = $tenant_id AND list_id = $list_id` inside the same `DoTx` as the list delete. That is a scan of the tenant's rows. Without it, a dangling row keeps its role in use, so `YDBRoles.Delete` returns `ErrRoleInUse` forever and nothing in the UI lists the stale assignment.
  - **Deliverability plan.** `internal/deliverability/adapters/audit.go` writes the shared `audit_log`. Its YDB variant must write the same columns:
    - `created_at` from `CurrentUtcTimestamp()`, as part of the primary key;
    - `metadata` as JSON text in the `Utf8` column, always a valid JSON object and never `""`. `YDBAudit.All` returns an error on metadata it cannot decode, so one bad row fails every page that contains it.
  - **Media plan.** `media_assets.uploaded_by` references iam `users(id)` with `ON DELETE SET NULL`. iam users are never deleted, so YDB needs nothing for it.
  - **Accepted YDB/pg differences.** No contract test covers these:
    - Assigning an unknown role: pg returns a wrapped foreign-key error; YDB returns `ErrRoleNotFound`.
    - Assigning another tenant's role id: pg accepts it, because foreign-key checks bypass RLS, although the role then grants nothing. YDB returns `ErrRoleNotFound`.
    - Assigning a role to an unknown user or list: pg returns a foreign-key error; YDB accepts it.
    - `TouchLastUsed` with a malformed id: pg returns an invalid-input error, which the app ignores; YDB does nothing.
    - `YDBRoles.All` orders by `name` by bytes, and pg by the database collation, so mixed-case and non-ASCII names order differently.
    - YDB accepts what pg rejects. None of these is reachable through the domain constructors or the app flows:
      - values the `CHECK` constraints forbid (`users.status`, `sessions.state`, `audit_log.actor_kind`);
      - unknown users where pg has foreign keys (`sessions.user_id`, `api_keys.created_by`, `recovery_codes.user_id`);
      - a malformed user id in `RecoveryCodes`: pg returns an invalid-input error, YDB returns false or does nothing.
    - Once TTL has deleted an expired TOTP-pending session, a request answers `ErrUnauthenticated` instead of `ErrTOTPRequired`.
  - **Ceilings.** These are marked `ponytail:` in code and listed under the spec's "Ceilings":
    - `YDBRoles.Delete` scans the tenant's assignments;
    - `YDBAudit.All` pages with `OFFSET`, which the spec's `count(*) + OFFSET` ceiling covers.
