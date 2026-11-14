# Serverless On-Prem — Plan B3 (tenant context on YDB, YDB onboarding) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:**
- Port the `tenant` context to YDB: `YDBTenants`, `YDBInvitations`, `YDBSettings` and `YDBBranding` behind the existing domain ports.
- Add the YDB variant of the cross-context onboarding transaction (`internal/service/onboarding.go`).
- Hold every new implementation to the same contract tests as its Postgres counterpart.

**Architecture:**
- **Migration.** One file, `internal/db/ydbmigrations/000004_tenant.up.sql`, creates `tenants`, `platform_user_tenants`, `invitations`, `tenant_settings` and `tenant_branding`.
- **Adapters.** The YDB adapters sit next to the pg adapters and call `db.Query()` / `DoTx` directly, like `internal/auth/adapters/users_ydb.go`.
- **Contract tests.** The `*_pg_test.go` files become pg+ydb contract suites through an `eachBackend` harness, the same shape as auth's. Test fixtures seed platform users through the auth repositories of the same backend.
- **No partial index.** Postgres allows one pending invitation per `(tenant_id, email)` through a partial unique index. YDB has no partial index, so the check and the insert share one serializable `DoTx`.
- **No INSERT-and-ignore inside a transaction.** Postgres `ON CONFLICT DO NOTHING` on memberships becomes SELECT-then-INSERT inside the transaction (`AddYDBMembership`).
- **YDB onboarding.** `ydbOnboarding` in `internal/service` runs each acceptance as one `DoTx`. It writes through helpers that the owning contexts export:
  - auth's `InsertYDBUser` and `InsertYDBSession`;
  - tenant's `AcceptYDBInvitation` and `AddYDBMembership`.

  Each table's SQL and the `email_key` derivation therefore live in one place.

**Tech Stack:**
- Go 1.26
- `github.com/ydb-platform/ydb-go-sdk/v3` v3.136.4 (query service)
- testcontainers-go, local-ydb `26.3.1.23`
- testify, `github.com/google/uuid`

**Design spec:** [docs/superpowers/specs/2026-11-14-serverless-onprem-design.md](../specs/2026-11-14-serverless-onprem-design.md).
- Binding sections:
  - "Storage layer (YDB)": Transactions; Schema and migrations, including the non-idempotent-statement rule; SQL translation rules.
  - "Spike findings (Plan A)": V4 and V6.
  - "Plan B2 planning spike".
  - "Plan B3 planning spike".
  - "Ceilings".
- The B3 spike verified every YQL construct this plan uses that B1 and B2 did not:
  - `JOIN`, with and without `VIEW` on the left side;
  - `ORDER BY` on a joined alias column that is not projected;
  - `UPSERT`;
  - `UPDATE ... RETURNING` through `tx.QueryRow` inside `DoTx`, with rollback on the zero-row path;
  - the concurrent SELECT-then-INSERT pending-invitation check.
- **Dry run.** During planning, every code block and edit in this plan was applied task by task to a scratch copy of the tree, then reverted. After each task, `go vet` and that task's pg+ydb suites passed. On the final state, `golangci-lint run` reported `0 issues.` and `make lint-arch` reported `[arch] clean`. The full `go test ./...` was not run.

**Scope:**
- This is the third Plan B plan. The port order is auth, tenant, iam, audience, campaign, deliverability, media.
- Not in scope:
  - Wiring the YDB adapters and `newYDBOnboarding` into the composition root (Plan C).
  - `memberDirectory` (`internal/service/onboarding.go`). It already composes `UserRepository` and `TenantRepository`, so it works over the YDB adapters unchanged.
  - The tenant app layer (`internal/tenant/app/...`), which is unchanged.

## Global Constraints

- **Branch.**
  - Work on `feat/serverless-onprem-b3` in the worktree `/Users/nikthespirit/experiments/nvelope/.claude/worktrees/serverless-onprem-b3`. Both already exist.
  - Do not create branches or worktrees.
  - The main checkout belongs to other sessions.
- **Docker.**
  - Docker on this machine is OrbStack. Prefix every `go test` / `make test` / `make ci` with `export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock" && `.
  - Ryuk reaps test containers between runs, so every `go test` run starts fresh containers.
- **YDB container.** It is `ydbplatform/local-ydb:26.3.1.23`, amd64 under Rosetta. A fresh container takes about 45 s to start, so use `-timeout 8m` on runs that touch YDB, and `-timeout 15m` for the full suite.
- **Known environmental flakes. None of these is caused by this plan:**
  - `internal/platform/ratelimit` `TestAllowIsConsistentUnderConcurrency` can fail with `redis eval failed: i/o timeout` under full-suite load. Re-run that package alone and record both runs.
  - pg-backed packages can all fail with `no migration found for version N`, where N is above `22`, or with `unexpected postmaster exit`. That means another worktree's shared `nvelope-test-pg` container is in use. Re-run once it is gone, and never `docker rm` it yourself.
  - A YDB-backed test can fail with `No such container: <id>`, or hang until the timeout, when it starts while Ryuk is still reaping the previous run's YDB container. Re-run once.
- **Dependencies.** No new Go modules and no `go.mod` change.
- **SaaS must not change.** Postgres adapters, River, Redis and every existing test keep their behaviour. `go test ./...` passes after every task.
- **YQL conventions** (spec "SQL translation rules" and V4):
  - Every query `DECLARE`s each parameter: `DECLARE $id AS Utf8;`.
  - Literals that target a `Utf8` column take the `u` suffix (`"pending"u`); a bare `"..."` is `String`.
  - Every DDL statement is idempotent (`CREATE TABLE IF NOT EXISTS`).
  - A statement ends on a line ending in `;`. Never end a line with `;` inside a string literal or a `--` comment.
  - A secondary index is read only through an explicit `VIEW <index_name>`.
  - In a `JOIN`, every column is qualified with its table alias (`m.role`, `u.email`).
- **Tenant scoping.**
  - `tenants` is a control-plane table with primary key `(id)`.
  - `platform_user_tenants`, `invitations`, `tenant_settings` and `tenant_branding` are tenant-scoped. Their primary key starts with `tenant_id`, and every query filters by the `tenantID` the port receives.
  - Two reads have no `tenantID` in their port, so they carry no tenant predicate:
    - `Invitations.GetPendingByTokenHash` reads through the global unique `token_hash` index.
    - `Tenants.ListMembershipsForUser` reads through the `platform_user_id` index.
- **IDs** are UUIDv4 strings generated in Go (`uuid.NewString()`) and stored as `Utf8`.
- **No column defaults.** The YDB columns have no `DEFAULT`. Where Postgres fills a value from a column default, the YDB statement writes it explicitly:
  - `tenants.status`: `"active"u`;
  - `tenant_settings.timezone`: `"UTC"u`;
  - `invitations.role`: `"admin"u`;
  - `invitations.status`: `"pending"u`.
- **Case-insensitive matching** (Postgres `citext`):
  - Slugs are stored lower-cased (`domain.NewSlug` lower-cases them), so `GetBySlug` lower-cases its argument.
  - `invitations.email` keeps the address as entered. `email_key` holds `strings.ToLower(email)`, which the tenant adapters derive in `ydbInvitationEmailKey`. It is compared only with other invitations' keys.
  - `platform_users` rows are written only through auth's `InsertYDBUser`, so `platform_users.email_key` keeps its single derivation (`ydbEmailKey` in `internal/auth/adapters/users_ydb.go`).
- **`ON CONFLICT DO NOTHING`** never becomes "INSERT and ignore the conflict" inside a multi-statement transaction. The conflict aborts the whole transaction (spec "Plan B2 planning spike"). It never becomes `UPSERT` either, because that would overwrite an owner's role. Use `AddYDBMembership`, which does SELECT-then-INSERT.
- **`DoTx` closures must be retry-safe** (V6):
  - only database operations inside, plus the port's `fn` callbacks;
  - ids, tokens, hashes and domain values are built before `DoTx`;
  - values the caller reads are assigned at the end of the closure;
  - never `append` to a slice declared outside the closure.
- **Duplicate key mapping.**
  - "Already exists" comes from `db.IsYDBConflict(err)`. Never match on the message text.
  - Check it on the statement and also on the error `DoTx` returns, as `users_ydb.go` does, for a YDB that reports the conflict at commit.
  - Never add `query.WithIdempotent()` to an insert path.
- **Commits.** Use conventional messages. **No `Co-Authored-By` trailer** of any kind.
- **Tooling.**
  - `goimports` is not installed, so use `gofmt -w`.
  - `golangci-lint` is on PATH. It lints test files too (`.golangci.yml` does not disable `run.tests`), so a type that only a test uses does not trip `unused`.
  - `make lint-arch` needs `export PATH="$HOME/go/bin:$PATH" && `.
  - The tenant adapter tests import `internal/auth/adapters` to seed users. This was checked during planning: `go-cleanarch` does not flag the import, and `make lint-arch` stays `[arch] clean`.

## File structure

| File | Responsibility |
|------|----------------|
| `internal/auth/domain/repository.go` | doc fix: the create callbacks may have overwrite-only side effects |
| `internal/db/ydbmigrations/000004_tenant.up.sql` | `tenants`, `platform_user_tenants`, `invitations`, `tenant_settings`, `tenant_branding` |
| `internal/tenant/adapters/tenants_ydb.go` | `YDBTenants`, the shared `AddYDBMembership`, the `ydbTenantParam` / `ydbMembershipKey` binders |
| `internal/tenant/adapters/settings_ydb.go` | `YDBSettings` |
| `internal/tenant/adapters/branding_ydb.go` | `YDBBranding` |
| `internal/tenant/adapters/invitations_ydb.go` | `YDBInvitations`, `ydbInvitationEmailKey`, and (Task 3) the shared `AcceptYDBInvitation` |
| `internal/tenant/adapters/doc.go` | package doc mentions the YDB implementations |
| `internal/tenant/adapters/helpers_test.go` (rewritten) | `backend`, `backends`, `eachBackend`, `newUser`, `createWorkspace` |
| `internal/tenant/adapters/tenants_test.go` (renamed from `tenants_pg_test.go`) | tenants contract suite |
| `internal/tenant/adapters/settings_test.go` (renamed from `settings_pg_test.go`) | settings contract suite |
| `internal/tenant/adapters/branding_test.go` (new) | branding contract suite |
| `internal/tenant/adapters/invitations_test.go` (renamed from `invitations_pg_test.go`) | invitations contract suite |
| `internal/auth/adapters/users_ydb.go`, `sessions_ydb.go` | export `InsertYDBUser` and `InsertYDBSession` |
| `internal/service/onboarding_ydb.go` (new) | `ydbOnboarding`, `newYDBOnboarding` |
| `internal/service/onboarding_test.go` (new) | onboarding contract suite (pg `onboarding` and `ydbOnboarding`) |

---

### Task 1: The tenant schema, `YDBTenants`, `YDBSettings`, `YDBBranding` and their contract suites

**Files:**
- Modify: `internal/auth/domain/repository.go:19-31` (doc comments only)
- Create: `internal/db/ydbmigrations/000004_tenant.up.sql`
- Create: `internal/tenant/adapters/tenants_ydb.go`
- Create: `internal/tenant/adapters/settings_ydb.go`
- Create: `internal/tenant/adapters/branding_ydb.go`
- Modify: `internal/tenant/adapters/doc.go`
- Modify: `internal/tenant/adapters/helpers_test.go` (rewritten)
- Rename: `internal/tenant/adapters/tenants_pg_test.go` → `internal/tenant/adapters/tenants_test.go` (rewritten)
- Rename: `internal/tenant/adapters/settings_pg_test.go` → `internal/tenant/adapters/settings_test.go` (rewritten)
- Create: `internal/tenant/adapters/branding_test.go`

**Interfaces:**
- Consumes:
  - `dbtest.YDBMigrated(t) *ydb.Driver` (`internal/dbtest/ydb.go`). It applies every `internal/db/ydbmigrations/*.up.sql` once per test binary, so the new file is picked up automatically.
  - `db.IsYDBConflict(err error) bool` (`internal/db/ydb.go`).
  - The ports `domain.TenantRepository`, `domain.SettingsRepository` (`internal/tenant/domain/repository.go`) and `domain.BrandingRepository` (`internal/tenant/domain/branding.go`), unchanged.
  - `authadapters.NewUsers(pool)` and `authadapters.NewYDBUsers(d)` (`internal/auth/adapters`), to seed users.
  - The `platform_users` table from `000003_auth.up.sql` (`id`, `email`, `name`, ...), joined by `ListMembers`.
- Produces:
  ```go
  // package adapters (internal/tenant/adapters)
  type YDBTenants struct{ /* unexported */ }
  func NewYDBTenants(d *ydb.Driver) *YDBTenants
  func AddYDBMembership(ctx context.Context, tx query.TxActor, m *domain.Membership) error
  func ydbTenantParam(tenantID string) query.ExecuteOption
  type YDBSettings struct{ /* unexported */ }
  func NewYDBSettings(d *ydb.Driver) *YDBSettings
  type YDBBranding struct{ /* unexported */ }
  func NewYDBBranding(d *ydb.Driver) *YDBBranding
  // package adapters_test
  const testUserName = "Test User"
  type backend struct {
      name     string
      tenants  func(t *testing.T) domain.TenantRepository
      settings func(t *testing.T) domain.SettingsRepository
      branding func(t *testing.T) domain.BrandingRepository
      users    func(t *testing.T) authdomain.UserRepository
  }
  func eachBackend(t *testing.T, body func(t *testing.T, b backend))
  func newUser(t *testing.T, b backend) (id, email string)
  func createWorkspace(t *testing.T, repo domain.TenantRepository, ownerID string) *domain.Tenant
  func insertUser(t *testing.T, pool *pgxpool.Pool) string // kept as is; Task 2 deletes it
  ```
- Task 2 relies on:
  - the `invitations` table and its `idx_invitations_token_hash` index;
  - `ydbTenantParam`;
  - the harness above.
- Task 3 relies on `AddYDBMembership`.

- [ ] **Step 1: Fix the create-callback doc comments and commit**

The two port comments say each callback "must have no side effects beyond its return values". That contradicts `internal/auth/app/command/signup.go:95`, whose closure overwrites an outer `rawToken`. The overwrite is safe because the last write wins, as the spec's V6 row says.

In `internal/auth/domain/repository.go`, replace:

```go
	// issueSession may be called more than once (a backend may retry the
	// transaction), so it must have no side effects beyond its return values;
	// only the last call's result is persisted.
```

with:

```go
	// issueSession may be called more than once (a backend may retry the
	// transaction), so any side effect must be overwrite-only (e.g. assigning
	// an outer variable that the next call overwrites); only the last call's
	// result is persisted.
```

Then replace:

```go
	// the user nor the challenge. issueVerification may be called more than
	// once (a backend may retry the transaction), so it must have no side
	// effects beyond its return values; only the last call's result is
	// persisted.
```

with:

```go
	// the user nor the challenge. issueVerification may be called more than
	// once (a backend may retry the transaction), so any side effect must be
	// overwrite-only (e.g. assigning an outer variable that the next call
	// overwrites); only the last call's result is persisted.
```

Run: `go build ./internal/auth/... && gofmt -l internal/auth/domain/`

Expected: no output.

```bash
git add internal/auth/domain/repository.go
git commit -m "docs(auth): allow overwrite-only side effects in the create callbacks"
```

- [ ] **Step 2: Write the tenant migration**

Create `internal/db/ydbmigrations/000004_tenant.up.sql`:

```sql
-- Tenant (internal/tenant): workspaces, memberships, invitations, settings and
-- branding. tenants is a control-plane table keyed by id. The other four are
-- tenant-scoped: their primary key starts with tenant_id and every query
-- filters by it. No column has a DEFAULT, so the adapters write the values
-- Postgres takes from its column defaults.

-- slug is stored lower-cased (the domain lower-cases it), so the unique index
-- reproduces the uniqueness of the Postgres citext column.
CREATE TABLE IF NOT EXISTS tenants (
    id         Utf8      NOT NULL,
    slug       Utf8      NOT NULL,
    name       Utf8      NOT NULL,
    status     Utf8      NOT NULL,
    created_at Timestamp NOT NULL,
    updated_at Timestamp NOT NULL,
    PRIMARY KEY (id),
    INDEX idx_tenants_slug GLOBAL UNIQUE SYNC ON (slug)
);

-- Keyed by tenant first for ListMembers and GetMembershipRole. The user index
-- serves ListMembershipsForUser.
CREATE TABLE IF NOT EXISTS platform_user_tenants (
    tenant_id        Utf8      NOT NULL,
    platform_user_id Utf8      NOT NULL,
    role             Utf8      NOT NULL,
    created_at       Timestamp NOT NULL,
    PRIMARY KEY (tenant_id, platform_user_id),
    INDEX idx_platform_user_tenants_user GLOBAL SYNC ON (platform_user_id)
);

-- email keeps the address as entered and email_key is its lower-cased form,
-- standing in for the Postgres citext column. Postgres allows one pending
-- invitation per (tenant_id, email) through a partial unique index. YDB has no
-- partial index, so YDBInvitations.Create checks for a pending row and inserts
-- in one serializable transaction. The accept link carries only the token, so
-- token_hash has a global unique index.
CREATE TABLE IF NOT EXISTS invitations (
    tenant_id   Utf8      NOT NULL,
    id          Utf8      NOT NULL,
    email       Utf8      NOT NULL,
    email_key   Utf8      NOT NULL,
    role        Utf8      NOT NULL,
    token_hash  Utf8      NOT NULL,
    status      Utf8      NOT NULL,
    invited_by  Utf8      NOT NULL,
    accepted_by Utf8,
    created_at  Timestamp NOT NULL,
    expires_at  Timestamp NOT NULL,
    accepted_at Timestamp,
    PRIMARY KEY (tenant_id, id),
    INDEX idx_invitations_token_hash GLOBAL UNIQUE SYNC ON (token_hash)
);

-- One row per tenant, created with the tenant.
CREATE TABLE IF NOT EXISTS tenant_settings (
    tenant_id    Utf8      NOT NULL,
    display_name Utf8      NOT NULL,
    timezone     Utf8      NOT NULL,
    created_at   Timestamp NOT NULL,
    updated_at   Timestamp NOT NULL,
    PRIMARY KEY (tenant_id)
);

-- At most one row per tenant. A tenant without a row uses the platform
-- defaults.
CREATE TABLE IF NOT EXISTS tenant_branding (
    tenant_id     Utf8      NOT NULL,
    logo_url      Utf8      NOT NULL,
    primary_color Utf8      NOT NULL,
    custom_css    Utf8      NOT NULL,
    updated_at    Timestamp NOT NULL,
    PRIMARY KEY (tenant_id)
);
```

- [ ] **Step 3: Rewrite the harness**

Replace the contents of `internal/tenant/adapters/helpers_test.go` with the code below.
- `insertUser` stays unchanged, because `invitations_pg_test.go` still calls it until Task 2.
- `createWorkspace` now takes the `domain.TenantRepository` interface. `*adapters.Tenants` satisfies it, so the existing calls in `invitations_pg_test.go` still compile.

```go
package adapters_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	authadapters "github.com/nikolaymatrosov/nvelope/internal/auth/adapters"
	authdomain "github.com/nikolaymatrosov/nvelope/internal/auth/domain"
	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
	"github.com/nikolaymatrosov/nvelope/internal/tenant/adapters"
	"github.com/nikolaymatrosov/nvelope/internal/tenant/domain"
)

// testUserName is the name of every platform user newUser seeds.
const testUserName = "Test User"

// backend is one storage implementation of the tenant repositories under
// contract test. Every contract test body runs once per backend, so the
// Postgres and YDB adapters are held to the same behaviour.
type backend struct {
	name     string
	tenants  func(t *testing.T) domain.TenantRepository
	settings func(t *testing.T) domain.SettingsRepository
	branding func(t *testing.T) domain.BrandingRepository
	// users seeds platform users through the auth context's repository for
	// the same backend. On Postgres, memberships and invitations carry foreign
	// keys to platform_users, and ListMembers joins it on both backends.
	users func(t *testing.T) authdomain.UserRepository
}

var backends = []backend{
	{
		name: "pg",
		tenants: func(t *testing.T) domain.TenantRepository {
			return adapters.NewTenants(dbtest.AppPool(t))
		},
		settings: func(t *testing.T) domain.SettingsRepository {
			return adapters.NewSettings(dbtest.AppPool(t))
		},
		branding: func(t *testing.T) domain.BrandingRepository {
			return adapters.NewBranding(dbtest.AppPool(t))
		},
		users: func(t *testing.T) authdomain.UserRepository {
			return authadapters.NewUsers(dbtest.AppPool(t))
		},
	},
	{
		name: "ydb",
		tenants: func(t *testing.T) domain.TenantRepository {
			return adapters.NewYDBTenants(dbtest.YDBMigrated(t))
		},
		settings: func(t *testing.T) domain.SettingsRepository {
			return adapters.NewYDBSettings(dbtest.YDBMigrated(t))
		},
		branding: func(t *testing.T) domain.BrandingRepository {
			return adapters.NewYDBBranding(dbtest.YDBMigrated(t))
		},
		users: func(t *testing.T) authdomain.UserRepository {
			return authadapters.NewYDBUsers(dbtest.YDBMigrated(t))
		},
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

// newUser seeds a platform user with a fresh address through the backend's
// auth repository and returns its id and email.
func newUser(t *testing.T, b backend) (id, email string) {
	t.Helper()
	addr := dbtest.RandString() + "@example.com"
	e, err := authdomain.NewEmail(addr)
	require.NoError(t, err)
	u, err := authdomain.NewUser(e, testUserName)
	require.NoError(t, err)
	created, err := b.users(t).Create(context.Background(), u, "hash")
	require.NoError(t, err)
	return created.ID(), addr
}

// createWorkspace creates a tenant with a fresh slug owned by ownerID and
// returns the persisted tenant.
func createWorkspace(t *testing.T, repo domain.TenantRepository, ownerID string) *domain.Tenant {
	t.Helper()
	tn, err := domain.NewTenant("Workspace", "ws-"+dbtest.RandString())
	require.NoError(t, err)
	created, err := repo.CreateWorkspace(context.Background(), tn, ownerID)
	require.NoError(t, err)
	return created
}

// insertUser inserts a platform user directly and returns its id. The tenant
// adapters need real users to satisfy the membership and invitation foreign
// keys; the auth context owns user creation, so the test inserts one directly.
func insertUser(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var id string
	err := pool.QueryRow(context.Background(),
		`INSERT INTO platform_users (email, password_hash, name)
		 VALUES ($1, 'hash', 'Test User') RETURNING id`,
		dbtest.RandString()+"@example.com").Scan(&id)
	require.NoError(t, err)
	return id
}
```

- [ ] **Step 4: Rewrite the tenants tests as a contract suite**

```bash
git mv internal/tenant/adapters/tenants_pg_test.go internal/tenant/adapters/tenants_test.go
```

Replace the contents of `internal/tenant/adapters/tenants_test.go` with the code below. Four tests and one assertion are new:
- `AddMembershipKeepsExistingRole` pins `ON CONFLICT DO NOTHING`.
- `ListMembers` and `ListMembershipsForUser` had no test before.
- The upper-case `GetBySlug` assertion pins `citext`.

```go
package adapters_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
	"github.com/nikolaymatrosov/nvelope/internal/tenant/domain"
)

func TestTenantsCreateWorkspace(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.tenants(t)
		ctx := context.Background()

		ownerID, _ := newUser(t, b)
		created := createWorkspace(t, repo, ownerID)
		require.NotEmpty(t, created.ID(), "the adapter assigns an id")
		require.Equal(t, "Workspace", created.Name())
		require.Equal(t, domain.StatusActive, created.Status())

		// The owner membership was created in the same transaction.
		role, err := repo.GetMembershipRole(ctx, ownerID, created.ID())
		require.NoError(t, err)
		require.Equal(t, domain.RoleOwner, role)

		// So was the initial settings row, named after the tenant.
		settings, err := b.settings(t).Get(ctx, created.ID())
		require.NoError(t, err)
		require.Equal(t, "Workspace", settings.DisplayName())
	})
}

func TestTenantsCreateRejectsDuplicateSlug(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.tenants(t)
		ctx := context.Background()

		ownerID, _ := newUser(t, b)
		slug := "dup-" + dbtest.RandString()

		first, err := domain.NewTenant("First", slug)
		require.NoError(t, err)
		_, err = repo.CreateWorkspace(ctx, first, ownerID)
		require.NoError(t, err)

		second, err := domain.NewTenant("Second", slug)
		require.NoError(t, err)
		_, err = repo.CreateWorkspace(ctx, second, ownerID)
		require.ErrorIs(t, err, domain.ErrSlugTaken)
	})
}

func TestTenantsGetBySlugAndID(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.tenants(t)
		ctx := context.Background()

		ownerID, _ := newUser(t, b)
		created := createWorkspace(t, repo, ownerID)

		bySlug, err := repo.GetBySlug(ctx, created.Slug().String())
		require.NoError(t, err)
		require.Equal(t, created, bySlug)

		// Slugs match ignoring case, as the citext column does on Postgres.
		byUpperSlug, err := repo.GetBySlug(ctx, strings.ToUpper(created.Slug().String()))
		require.NoError(t, err)
		require.Equal(t, created.ID(), byUpperSlug.ID())

		byID, err := repo.GetByID(ctx, created.ID())
		require.NoError(t, err)
		require.Equal(t, created, byID)

		_, err = repo.GetBySlug(ctx, "no-such-"+dbtest.RandString())
		require.ErrorIs(t, err, domain.ErrTenantNotFound)

		_, err = repo.GetByID(ctx, "not-a-uuid")
		require.ErrorIs(t, err, domain.ErrTenantNotFound, "a malformed id is an opaque not-found")
	})
}

func TestTenantsMembership(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.tenants(t)
		ctx := context.Background()

		ownerID, _ := newUser(t, b)
		created := createWorkspace(t, repo, ownerID)
		stranger, _ := newUser(t, b)

		_, err := repo.GetMembershipRole(ctx, stranger, created.ID())
		require.ErrorIs(t, err, domain.ErrNotMember)

		m, err := domain.NewMembership(stranger, created.ID(), domain.RoleAdmin)
		require.NoError(t, err)
		require.NoError(t, repo.AddMembership(ctx, m))

		role, err := repo.GetMembershipRole(ctx, stranger, created.ID())
		require.NoError(t, err)
		require.Equal(t, domain.RoleAdmin, role)

		// Adding the same membership again is a harmless no-op.
		require.NoError(t, repo.AddMembership(ctx, m))
	})
}

// TestTenantsAddMembershipKeepsExistingRole pins Postgres's ON CONFLICT DO
// NOTHING: re-adding an existing member never changes their role.
func TestTenantsAddMembershipKeepsExistingRole(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.tenants(t)
		ctx := context.Background()

		ownerID, _ := newUser(t, b)
		created := createWorkspace(t, repo, ownerID)

		m, err := domain.NewMembership(ownerID, created.ID(), domain.RoleAdmin)
		require.NoError(t, err)
		require.NoError(t, repo.AddMembership(ctx, m))

		role, err := repo.GetMembershipRole(ctx, ownerID, created.ID())
		require.NoError(t, err)
		require.Equal(t, domain.RoleOwner, role, "the owner is not demoted")
	})
}

func TestTenantsListMembers(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.tenants(t)
		ctx := context.Background()

		ownerID, ownerEmail := newUser(t, b)
		created := createWorkspace(t, repo, ownerID)
		adminID, adminEmail := newUser(t, b)
		m, err := domain.NewMembership(adminID, created.ID(), domain.RoleAdmin)
		require.NoError(t, err)
		require.NoError(t, repo.AddMembership(ctx, m))

		// Another workspace's owner is not a member of this one.
		outsiderID, _ := newUser(t, b)
		createWorkspace(t, repo, outsiderID)

		members, err := repo.ListMembers(ctx, created.ID())
		require.NoError(t, err)
		require.Equal(t, []domain.Member{
			{UserID: ownerID, Email: ownerEmail, Name: testUserName, Role: domain.RoleOwner},
			{UserID: adminID, Email: adminEmail, Name: testUserName, Role: domain.RoleAdmin},
		}, members, "only this tenant's members, oldest membership first")
	})
}

func TestTenantsListMembershipsForUser(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.tenants(t)
		ctx := context.Background()

		userID, _ := newUser(t, b)
		owned := createWorkspace(t, repo, userID)

		otherOwnerID, _ := newUser(t, b)
		joined := createWorkspace(t, repo, otherOwnerID)
		m, err := domain.NewMembership(userID, joined.ID(), domain.RoleAdmin)
		require.NoError(t, err)
		require.NoError(t, repo.AddMembership(ctx, m))

		memberships, err := repo.ListMembershipsForUser(ctx, userID)
		require.NoError(t, err)
		require.Equal(t, []domain.MembershipDetail{
			{Tenant: owned, Role: domain.RoleOwner},
			{Tenant: joined, Role: domain.RoleAdmin},
		}, memberships, "every tenant of the user with its role, oldest membership first")

		loner, _ := newUser(t, b)
		none, err := repo.ListMembershipsForUser(ctx, loner)
		require.NoError(t, err)
		require.Empty(t, none)
	})
}
```

- [ ] **Step 5: Rewrite the settings tests as a contract suite**

```bash
git mv internal/tenant/adapters/settings_pg_test.go internal/tenant/adapters/settings_test.go
```

Replace the contents of `internal/tenant/adapters/settings_test.go` with the code below. `TestSettingsAreTenantScoped` is new.

```go
package adapters_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/tenant/domain"
)

func TestSettingsGetReturnsInitialRow(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		ownerID, _ := newUser(t, b)
		ws := createWorkspace(t, b.tenants(t), ownerID)

		settings, err := b.settings(t).Get(context.Background(), ws.ID())
		require.NoError(t, err)
		require.Equal(t, ws.ID(), settings.TenantID())
		require.Equal(t, "Workspace", settings.DisplayName())
		require.Equal(t, "UTC", settings.Timezone(), "a new workspace starts in UTC")
	})
}

func TestSettingsUpdate(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.settings(t)
		ctx := context.Background()

		ownerID, _ := newUser(t, b)
		ws := createWorkspace(t, b.tenants(t), ownerID)

		err := repo.Update(ctx, ws.ID(), func(s *domain.TenantSettings) (*domain.TenantSettings, error) {
			if err := s.Rename("Renamed"); err != nil {
				return nil, err
			}
			if err := s.SetTimezone("Europe/Madrid"); err != nil {
				return nil, err
			}
			return s, nil
		})
		require.NoError(t, err)

		settings, err := repo.Get(ctx, ws.ID())
		require.NoError(t, err)
		require.Equal(t, "Renamed", settings.DisplayName())
		require.Equal(t, "Europe/Madrid", settings.Timezone())
	})
}

func TestSettingsUpdateRollsBackOnClosureError(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.settings(t)
		ctx := context.Background()

		ownerID, _ := newUser(t, b)
		ws := createWorkspace(t, b.tenants(t), ownerID)

		sentinel := domain.ErrTenantNotFound
		err := repo.Update(ctx, ws.ID(), func(s *domain.TenantSettings) (*domain.TenantSettings, error) {
			_ = s.Rename("Should Not Persist")
			return nil, sentinel
		})
		require.ErrorIs(t, err, sentinel)

		settings, err := repo.Get(ctx, ws.ID())
		require.NoError(t, err)
		require.Equal(t, "Workspace", settings.DisplayName(), "a closure error rolls the transaction back")
	})
}

func TestSettingsAreTenantScoped(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.settings(t)
		tenants := b.tenants(t)
		ctx := context.Background()

		ownerID, _ := newUser(t, b)
		first := createWorkspace(t, tenants, ownerID)
		second := createWorkspace(t, tenants, ownerID)

		err := repo.Update(ctx, first.ID(), func(s *domain.TenantSettings) (*domain.TenantSettings, error) {
			if err := s.Rename("First Only"); err != nil {
				return nil, err
			}
			return s, nil
		})
		require.NoError(t, err)

		untouched, err := repo.Get(ctx, second.ID())
		require.NoError(t, err)
		require.Equal(t, "Workspace", untouched.DisplayName(), "another tenant's settings are untouched")

		unknown := uuid.NewString()
		_, err = repo.Get(ctx, unknown)
		require.ErrorIs(t, err, domain.ErrTenantNotFound)
		err = repo.Update(ctx, unknown, func(s *domain.TenantSettings) (*domain.TenantSettings, error) {
			return s, nil
		})
		require.ErrorIs(t, err, domain.ErrTenantNotFound)
	})
}
```

- [ ] **Step 6: Write the branding contract suite**

Create `internal/tenant/adapters/branding_test.go`. Branding had no adapter test before.

```go
package adapters_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/tenant/domain"
)

func TestBrandingGetDefaultsWhenUnset(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		ownerID, _ := newUser(t, b)
		ws := createWorkspace(t, b.tenants(t), ownerID)

		got, err := b.branding(t).Get(context.Background(), ws.ID())
		require.NoError(t, err)
		require.Equal(t, domain.NewTenantBranding(ws.ID()), got, "no row means the platform defaults")
	})
}

func TestBrandingSaveUpserts(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.branding(t)
		ctx := context.Background()

		ownerID, _ := newUser(t, b)
		ws := createWorkspace(t, b.tenants(t), ownerID)

		first := domain.NewTenantBranding(ws.ID())
		require.NoError(t, first.SetLogoURL("https://cdn.example.com/logo.png"))
		require.NoError(t, first.SetPrimaryColor("#4f46e5"))
		require.NoError(t, first.SetCustomCSS("h1 { color: #4f46e5; }"))
		require.NoError(t, repo.Save(ctx, first))

		got, err := repo.Get(ctx, ws.ID())
		require.NoError(t, err)
		require.Equal(t, ws.ID(), got.TenantID())
		require.Equal(t, "https://cdn.example.com/logo.png", got.LogoURL())
		require.Equal(t, "#4f46e5", got.PrimaryColor())
		require.Equal(t, "h1 { color: #4f46e5; }", got.CustomCSS())
		require.WithinDuration(t, time.Now(), got.UpdatedAt(), time.Minute, "the database stamps the save")

		// Saving again replaces the whole row.
		second := domain.NewTenantBranding(ws.ID())
		require.NoError(t, second.SetPrimaryColor("#000"))
		require.NoError(t, repo.Save(ctx, second))

		got, err = repo.Get(ctx, ws.ID())
		require.NoError(t, err)
		require.Equal(t, "#000", got.PrimaryColor())
		require.Empty(t, got.LogoURL())
		require.Empty(t, got.CustomCSS())
	})
}

func TestBrandingIsTenantScoped(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.branding(t)
		tenants := b.tenants(t)
		ctx := context.Background()

		ownerID, _ := newUser(t, b)
		first := createWorkspace(t, tenants, ownerID)
		second := createWorkspace(t, tenants, ownerID)

		branded := domain.NewTenantBranding(first.ID())
		require.NoError(t, branded.SetPrimaryColor("#4f46e5"))
		require.NoError(t, repo.Save(ctx, branded))

		got, err := repo.Get(ctx, second.ID())
		require.NoError(t, err)
		require.Equal(t, domain.NewTenantBranding(second.ID()), got, "another tenant's branding is not visible")
	})
}
```

- [ ] **Step 7: Run the suites to verify they fail**

Run: `go vet ./internal/tenant/adapters/`

Expected: build failure `undefined: adapters.NewYDBTenants`, `undefined: adapters.NewYDBSettings` and `undefined: adapters.NewYDBBranding`.

- [ ] **Step 8: Implement `YDBTenants`**

Create `internal/tenant/adapters/tenants_ydb.go`:

```go
package adapters

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	ydb "github.com/ydb-platform/ydb-go-sdk/v3"
	"github.com/ydb-platform/ydb-go-sdk/v3/query"

	"github.com/nikolaymatrosov/nvelope/internal/db"
	"github.com/nikolaymatrosov/nvelope/internal/tenant/domain"
)

// YDBTenants is the YDB-backed implementation of domain.TenantRepository, for
// the serverless runtime. tenants is a control-plane table keyed by id. A
// membership row is keyed (tenant_id, platform_user_id), so the tenant-scoped
// reads are key-prefix lookups. ListMembershipsForUser is the one membership
// read with no tenant predicate: it lists a user's tenants through the user
// index.
type YDBTenants struct {
	driver *ydb.Driver
}

var _ domain.TenantRepository = (*YDBTenants)(nil)

// NewYDBTenants builds a YDBTenants repository over the driver.
func NewYDBTenants(d *ydb.Driver) *YDBTenants {
	return &YDBTenants{driver: d}
}

// CreateWorkspace inserts the tenant, its owner membership and the initial
// tenant_settings row in one transaction, and returns the persisted tenant.
// The id is generated before the closure, so a retried transaction writes the
// same rows. Postgres takes the status and the settings timezone from column
// defaults. YDB has none, so the statements write "active" and "UTC".
func (r *YDBTenants) CreateWorkspace(ctx context.Context, t *domain.Tenant, ownerID string) (*domain.Tenant, error) {
	id := uuid.NewString()
	owner, err := domain.NewMembership(ownerID, id, domain.RoleOwner)
	if err != nil {
		return nil, err
	}
	err = r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		err := tx.Exec(ctx, `DECLARE $id AS Utf8;
DECLARE $slug AS Utf8;
DECLARE $name AS Utf8;
INSERT INTO tenants (id, slug, name, status, created_at, updated_at)
VALUES ($id, $slug, $name, "active"u, CurrentUtcTimestamp(), CurrentUtcTimestamp());`,
			query.WithParameters(ydb.ParamsBuilder().
				Param("$id").Text(id).
				Param("$slug").Text(t.Slug().String()).
				Param("$name").Text(t.Name()).
				Build()))
		if db.IsYDBConflict(err) {
			return domain.ErrSlugTaken
		}
		if err != nil {
			return fmt.Errorf("inserting tenant: %w", err)
		}
		if err := AddYDBMembership(ctx, tx, owner); err != nil {
			return err
		}
		if err := tx.Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $display_name AS Utf8;
INSERT INTO tenant_settings (tenant_id, display_name, timezone, created_at, updated_at)
VALUES ($tenant_id, $display_name, "UTC"u, CurrentUtcTimestamp(), CurrentUtcTimestamp());`,
			query.WithParameters(ydb.ParamsBuilder().
				Param("$tenant_id").Text(id).
				Param("$display_name").Text(t.Name()).
				Build())); err != nil {
			return fmt.Errorf("inserting tenant settings: %w", err)
		}
		return nil
	})
	// The spike saw the conflict at the statement; this covers a YDB that
	// reports it at commit. Only the slug index can collide: the id is fresh.
	if db.IsYDBConflict(err) {
		return nil, domain.ErrSlugTaken
	}
	if err != nil {
		return nil, err
	}
	return domain.HydrateTenant(id, t.Slug().String(), t.Name(), string(domain.StatusActive)), nil
}

// GetBySlug returns the tenant with the given slug, or
// domain.ErrTenantNotFound. Slugs are stored lower-cased (domain.NewSlug
// lower-cases them), so lower-casing the lookup matches case-insensitively,
// as the Postgres citext column does.
func (r *YDBTenants) GetBySlug(ctx context.Context, slug string) (*domain.Tenant, error) {
	return ydbTenantFromRow(r.driver.Query().QueryRow(ctx, `DECLARE $slug AS Utf8;
SELECT id, slug, name, status
FROM tenants VIEW idx_tenants_slug
WHERE slug = $slug;`,
		query.WithParameters(ydb.ParamsBuilder().Param("$slug").Text(strings.ToLower(slug)).Build())))
}

// GetByID returns the tenant with the given id, or domain.ErrTenantNotFound.
func (r *YDBTenants) GetByID(ctx context.Context, id string) (*domain.Tenant, error) {
	return ydbTenantFromRow(r.driver.Query().QueryRow(ctx, `DECLARE $id AS Utf8;
SELECT id, slug, name, status
FROM tenants
WHERE id = $id;`,
		query.WithParameters(ydb.ParamsBuilder().Param("$id").Text(id).Build())))
}

// ydbSelectMembershipRole reads one membership by its full primary key.
const ydbSelectMembershipRole = `DECLARE $tenant_id AS Utf8;
DECLARE $platform_user_id AS Utf8;
SELECT role FROM platform_user_tenants
WHERE tenant_id = $tenant_id AND platform_user_id = $platform_user_id;`

// GetMembershipRole returns the user's role in the tenant, or
// domain.ErrNotMember.
func (r *YDBTenants) GetMembershipRole(ctx context.Context, userID, tenantID string) (domain.Role, error) {
	row, err := r.driver.Query().QueryRow(ctx, ydbSelectMembershipRole, ydbMembershipKey(tenantID, userID))
	if errors.Is(err, query.ErrNoRows) {
		return domain.Role{}, domain.ErrNotMember
	}
	if err != nil {
		return domain.Role{}, fmt.Errorf("loading membership: %w", err)
	}
	var role string
	if err := row.Scan(&role); err != nil {
		return domain.Role{}, fmt.Errorf("scanning membership: %w", err)
	}
	parsed, err := domain.NewRole(role)
	if err != nil {
		return domain.Role{}, fmt.Errorf("loading membership role: %w", err)
	}
	return parsed, nil
}

// AddMembership records a membership. Re-adding an existing member is a no-op
// that keeps their role.
func (r *YDBTenants) AddMembership(ctx context.Context, m *domain.Membership) error {
	err := r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		return AddYDBMembership(ctx, tx, m)
	})
	// A concurrent add of the same membership surfaces as a serializable
	// abort, which the SDK retries. This covers a YDB that reports it as a key
	// conflict at commit instead. Either way the member exists.
	if db.IsYDBConflict(err) {
		return nil
	}
	return err
}

// AddYDBMembership records m inside tx unless the user is already a member of
// the tenant, in which case the existing role is kept. It is the YDB form of
// Postgres's INSERT ... ON CONFLICT DO NOTHING:
//   - an INSERT that hits the existing key aborts the whole transaction (spec
//     "Plan B2 planning spike");
//   - an UPSERT would overwrite an owner's role.
//
// The read and the insert share the serializable transaction. A concurrent
// insert of the same membership aborts one of the two, and the SDK retries it.
// YDBTenants and the serverless onboarding (internal/service) share it.
func AddYDBMembership(ctx context.Context, tx query.TxActor, m *domain.Membership) error {
	_, err := tx.QueryRow(ctx, ydbSelectMembershipRole, ydbMembershipKey(m.TenantID(), m.UserID()))
	if err == nil {
		return nil
	}
	if !errors.Is(err, query.ErrNoRows) {
		return fmt.Errorf("checking membership: %w", err)
	}
	if err := tx.Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $platform_user_id AS Utf8;
DECLARE $role AS Utf8;
INSERT INTO platform_user_tenants (tenant_id, platform_user_id, role, created_at)
VALUES ($tenant_id, $platform_user_id, $role, CurrentUtcTimestamp());`,
		query.WithParameters(ydb.ParamsBuilder().
			Param("$tenant_id").Text(m.TenantID()).
			Param("$platform_user_id").Text(m.UserID()).
			Param("$role").Text(m.Role().String()).
			Build())); err != nil {
		return fmt.Errorf("inserting membership: %w", err)
	}
	return nil
}

// ListMembershipsForUser returns every tenant the user belongs to, with the
// user's role in each, oldest membership first.
func (r *YDBTenants) ListMembershipsForUser(ctx context.Context, userID string) ([]domain.MembershipDetail, error) {
	rs, err := r.driver.Query().QueryResultSet(ctx, `DECLARE $platform_user_id AS Utf8;
SELECT t.id, t.slug, t.name, t.status, m.role
FROM platform_user_tenants VIEW idx_platform_user_tenants_user AS m
JOIN tenants AS t ON t.id = m.tenant_id
WHERE m.platform_user_id = $platform_user_id
ORDER BY m.created_at;`,
		query.WithParameters(ydb.ParamsBuilder().Param("$platform_user_id").Text(userID).Build()))
	if err != nil {
		return nil, fmt.Errorf("listing memberships: %w", err)
	}
	defer func() { _ = rs.Close(ctx) }()
	memberships := []domain.MembershipDetail{}
	for row, err := range rs.Rows(ctx) {
		if err != nil {
			return nil, fmt.Errorf("listing memberships: %w", err)
		}
		var id, slug, name, status, role string
		if err := row.Scan(&id, &slug, &name, &status, &role); err != nil {
			return nil, fmt.Errorf("scanning membership: %w", err)
		}
		parsedRole, err := domain.NewRole(role)
		if err != nil {
			return nil, fmt.Errorf("scanning membership role: %w", err)
		}
		memberships = append(memberships, domain.MembershipDetail{
			Tenant: domain.HydrateTenant(id, slug, name, status),
			Role:   parsedRole,
		})
	}
	return memberships, nil
}

// ListMembers returns every member of the tenant, oldest membership first. It
// joins platform_users, which the auth context owns, for the email and name.
func (r *YDBTenants) ListMembers(ctx context.Context, tenantID string) ([]domain.Member, error) {
	rs, err := r.driver.Query().QueryResultSet(ctx, `DECLARE $tenant_id AS Utf8;
SELECT u.id, u.email, u.name, m.role
FROM platform_user_tenants AS m
JOIN platform_users AS u ON u.id = m.platform_user_id
WHERE m.tenant_id = $tenant_id
ORDER BY m.created_at;`, ydbTenantParam(tenantID))
	if err != nil {
		return nil, fmt.Errorf("listing members: %w", err)
	}
	defer func() { _ = rs.Close(ctx) }()
	members := []domain.Member{}
	for row, err := range rs.Rows(ctx) {
		if err != nil {
			return nil, fmt.Errorf("listing members: %w", err)
		}
		var userID, email, name, role string
		if err := row.Scan(&userID, &email, &name, &role); err != nil {
			return nil, fmt.Errorf("scanning member: %w", err)
		}
		parsedRole, err := domain.NewRole(role)
		if err != nil {
			return nil, fmt.Errorf("scanning member role: %w", err)
		}
		members = append(members, domain.Member{
			UserID: userID, Email: email, Name: name, Role: parsedRole,
		})
	}
	return members, nil
}

// ydbTenantParam binds the $tenant_id parameter.
func ydbTenantParam(tenantID string) query.ExecuteOption {
	return query.WithParameters(ydb.ParamsBuilder().Param("$tenant_id").Text(tenantID).Build())
}

// ydbMembershipKey binds the platform_user_tenants primary key.
func ydbMembershipKey(tenantID, userID string) query.ExecuteOption {
	return query.WithParameters(ydb.ParamsBuilder().
		Param("$tenant_id").Text(tenantID).
		Param("$platform_user_id").Text(userID).
		Build())
}

// ydbTenantFromRow maps a single-row read of (id, slug, name, status) to the
// tenant, translating "no row" into domain.ErrTenantNotFound.
func ydbTenantFromRow(row query.Row, err error) (*domain.Tenant, error) {
	if errors.Is(err, query.ErrNoRows) {
		return nil, domain.ErrTenantNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("loading tenant: %w", err)
	}
	var id, slug, name, status string
	if err := row.Scan(&id, &slug, &name, &status); err != nil {
		return nil, fmt.Errorf("scanning tenant: %w", err)
	}
	return domain.HydrateTenant(id, slug, name, status), nil
}
```

- [ ] **Step 9: Implement `YDBSettings`**

Create `internal/tenant/adapters/settings_ydb.go`:

```go
package adapters

import (
	"context"
	"errors"
	"fmt"

	ydb "github.com/ydb-platform/ydb-go-sdk/v3"
	"github.com/ydb-platform/ydb-go-sdk/v3/query"

	"github.com/nikolaymatrosov/nvelope/internal/tenant/domain"
)

// YDBSettings is the YDB-backed implementation of domain.SettingsRepository,
// for the serverless runtime. YDB has no row-level security, so every
// statement filters by the tenant_id primary key explicitly.
type YDBSettings struct {
	driver *ydb.Driver
}

var _ domain.SettingsRepository = (*YDBSettings)(nil)

// NewYDBSettings builds a YDBSettings repository over the driver.
func NewYDBSettings(d *ydb.Driver) *YDBSettings {
	return &YDBSettings{driver: d}
}

const ydbSelectSettings = `DECLARE $tenant_id AS Utf8;
SELECT display_name, timezone
FROM tenant_settings
WHERE tenant_id = $tenant_id;`

// Get returns the tenant's settings, or domain.ErrTenantNotFound.
func (r *YDBSettings) Get(ctx context.Context, tenantID string) (*domain.TenantSettings, error) {
	row, err := r.driver.Query().QueryRow(ctx, ydbSelectSettings, ydbTenantParam(tenantID))
	return ydbSettingsFromRow(tenantID, row, err)
}

// Update loads the tenant's settings, runs fn, and persists the result in one
// serializable transaction. The SDK retries the transaction on conflict, so fn
// may run more than once and must be retry-safe.
func (r *YDBSettings) Update(ctx context.Context, tenantID string,
	fn func(*domain.TenantSettings) (*domain.TenantSettings, error)) error {

	return r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		row, err := tx.QueryRow(ctx, ydbSelectSettings, ydbTenantParam(tenantID))
		loaded, err := ydbSettingsFromRow(tenantID, row, err)
		if err != nil {
			return err
		}
		updated, err := fn(loaded)
		if err != nil {
			return err
		}
		if err := tx.Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $display_name AS Utf8;
DECLARE $timezone AS Utf8;
UPDATE tenant_settings
SET display_name = $display_name, timezone = $timezone, updated_at = CurrentUtcTimestamp()
WHERE tenant_id = $tenant_id;`,
			query.WithParameters(ydb.ParamsBuilder().
				Param("$tenant_id").Text(tenantID).
				Param("$display_name").Text(updated.DisplayName()).
				Param("$timezone").Text(updated.Timezone()).
				Build())); err != nil {
			return fmt.Errorf("updating tenant settings: %w", err)
		}
		return nil
	})
}

// ydbSettingsFromRow maps a single-row read in ydbSelectSettings order to the
// settings, translating "no row" into domain.ErrTenantNotFound.
func ydbSettingsFromRow(tenantID string, row query.Row, err error) (*domain.TenantSettings, error) {
	if errors.Is(err, query.ErrNoRows) {
		return nil, domain.ErrTenantNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("loading tenant settings: %w", err)
	}
	var displayName, timezone string
	if err := row.Scan(&displayName, &timezone); err != nil {
		return nil, fmt.Errorf("scanning tenant settings: %w", err)
	}
	return domain.HydrateTenantSettings(tenantID, displayName, timezone), nil
}
```

- [ ] **Step 10: Implement `YDBBranding`**

Create `internal/tenant/adapters/branding_ydb.go`:

```go
package adapters

import (
	"context"
	"errors"
	"fmt"
	"time"

	ydb "github.com/ydb-platform/ydb-go-sdk/v3"
	"github.com/ydb-platform/ydb-go-sdk/v3/query"

	"github.com/nikolaymatrosov/nvelope/internal/tenant/domain"
)

// YDBBranding is the YDB-backed implementation of domain.BrandingRepository,
// for the serverless runtime. YDB has no row-level security, so every
// statement filters by the tenant_id primary key explicitly.
type YDBBranding struct {
	driver *ydb.Driver
}

var _ domain.BrandingRepository = (*YDBBranding)(nil)

// NewYDBBranding builds a YDBBranding repository over the driver.
func NewYDBBranding(d *ydb.Driver) *YDBBranding {
	return &YDBBranding{driver: d}
}

// Get returns the tenant's branding, or an empty (platform-default) value when
// no row exists.
func (r *YDBBranding) Get(ctx context.Context, tenantID string) (*domain.TenantBranding, error) {
	row, err := r.driver.Query().QueryRow(ctx, `DECLARE $tenant_id AS Utf8;
SELECT logo_url, primary_color, custom_css, updated_at
FROM tenant_branding
WHERE tenant_id = $tenant_id;`, ydbTenantParam(tenantID))
	if errors.Is(err, query.ErrNoRows) {
		return domain.NewTenantBranding(tenantID), nil
	}
	if err != nil {
		return nil, fmt.Errorf("loading tenant branding: %w", err)
	}
	var logoURL, primaryColor, customCSS string
	var updatedAt time.Time
	if err := row.Scan(&logoURL, &primaryColor, &customCSS, &updatedAt); err != nil {
		return nil, fmt.Errorf("scanning tenant branding: %w", err)
	}
	return domain.HydrateTenantBranding(tenantID, logoURL, primaryColor, customCSS, updatedAt), nil
}

// Save upserts the tenant's branding row, replacing every field.
func (r *YDBBranding) Save(ctx context.Context, b *domain.TenantBranding) error {
	if err := r.driver.Query().Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $logo_url AS Utf8;
DECLARE $primary_color AS Utf8;
DECLARE $custom_css AS Utf8;
UPSERT INTO tenant_branding (tenant_id, logo_url, primary_color, custom_css, updated_at)
VALUES ($tenant_id, $logo_url, $primary_color, $custom_css, CurrentUtcTimestamp());`,
		query.WithParameters(ydb.ParamsBuilder().
			Param("$tenant_id").Text(b.TenantID()).
			Param("$logo_url").Text(b.LogoURL()).
			Param("$primary_color").Text(b.PrimaryColor()).
			Param("$custom_css").Text(b.CustomCSS()).
			Build())); err != nil {
		return fmt.Errorf("saving tenant branding: %w", err)
	}
	return nil
}
```

- [ ] **Step 11: Update the package doc**

Replace the contents of `internal/tenant/adapters/doc.go` with:

```go
// Package adapters implements the tenant domain's repository interfaces against
// pgx, and owns the Row-Level-Security-bound transaction helper; the *_ydb.go
// files are the YDB implementations of the repositories for the serverless
// runtime.
package adapters
```

- [ ] **Step 12: Run the suites to verify they pass**

Run: `export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock" && go test ./internal/tenant/adapters/ -run 'TestTenants|TestSettings|TestBranding' -v -timeout 8m 2>&1 | grep -E '^(=== RUN|--- (PASS|FAIL)|PASS|FAIL|ok)' | grep -E 'FAIL|/pg|/ydb|^ok'`

Expected: every `TestTenants*`, `TestSettings*` and `TestBranding*` shows `--- PASS` for both `/pg` and `/ydb`. The package is `ok`.

Three cases are not yours to fix. Report BLOCKED with the verbatim output:
- **A `/pg` subtest fails.** The contract would then change behaviour for SaaS, or Postgres does not behave as the plan assumes. In particular, `TestTenantsGetBySlugAndID/pg` failing on the upper-case slug means the `citext` assumption is wrong.
- **The migration is rejected by YDB.**
- **`TestTenantsListMembers/ydb` or `TestTenantsListMembershipsForUser/ydb` fails with a YQL syntax error on the `JOIN`.** That contradicts the spec's "Plan B3 planning spike" finding.

Any other `/ydb` failure: fix only the YDB adapter or the migration, and record the verbatim error and the fix in the report.

- [ ] **Step 13: Full verification and commit**

Run: `export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock" && go build ./... && go vet ./... && go test ./... -timeout 15m 2>&1 | grep -vE '^(ok|\?)\s'`

Expected: no output besides the build. In other words, no failing package.

Run: `golangci-lint run && export PATH="$HOME/go/bin:$PATH" && make lint-arch`

Expected: `0 issues.` and `[arch] clean`.

Run: `git diff --stat HEAD -- go.mod go.sum`

Expected: no output.

```bash
gofmt -l internal/tenant/ internal/auth/
git add internal/db/ydbmigrations/000004_tenant.up.sql internal/tenant/adapters/tenants_ydb.go \
  internal/tenant/adapters/settings_ydb.go internal/tenant/adapters/branding_ydb.go \
  internal/tenant/adapters/doc.go internal/tenant/adapters/helpers_test.go \
  internal/tenant/adapters/tenants_test.go internal/tenant/adapters/settings_test.go \
  internal/tenant/adapters/branding_test.go
git commit -m "feat(tenant): add the YDB tenant, settings and branding repositories under pg+ydb contract suites"
```

`gofmt -l` must print nothing. `git status` must show `tenants_pg_test.go` → `tenants_test.go` and `settings_pg_test.go` → `settings_test.go` as renames, not as deletes plus untracked files.

---

### Task 2: `YDBInvitations` and the invitations contract suite

**Files:**
- Create: `internal/tenant/adapters/invitations_ydb.go`
- Modify: `internal/tenant/adapters/helpers_test.go` (rewritten: adds the `invitations` field, deletes `insertUser`)
- Rename: `internal/tenant/adapters/invitations_pg_test.go` → `internal/tenant/adapters/invitations_test.go` (rewritten)

**Interfaces:**
- Consumes (Task 1):
  - the `invitations` table, with primary key `(tenant_id, id)` and the `idx_invitations_token_hash` index;
  - `ydbTenantParam(tenantID string) query.ExecuteOption`;
  - the harness: `backend`, `backends`, `eachBackend`, `newUser`, `createWorkspace`.
- Consumes `db.IsYDBConflict`, and the port `domain.InvitationRepository` (`internal/tenant/domain/repository.go`), unchanged.
- Produces:
  ```go
  // package adapters (internal/tenant/adapters)
  type YDBInvitations struct{ /* unexported */ }
  func NewYDBInvitations(d *ydb.Driver) *YDBInvitations
  func ydbInvitationEmailKey(email string) string
  func ydbTenantAndID(tenantID, id string) query.ExecuteOption
  // package adapters_test: backend gains
  invitations func(t *testing.T) domain.InvitationRepository
  ```
- Task 3 adds `AcceptYDBInvitation` to `invitations_ydb.go`. It reuses that file's imports and `ydbTenantAndID`.

- [ ] **Step 1: Add the invitations backend field and drop `insertUser`**

Replace the contents of `internal/tenant/adapters/helpers_test.go` with the code below. It is Task 1's harness with an `invitations` field. `insertUser` is gone, because Step 2 removes its last caller.

```go
package adapters_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	authadapters "github.com/nikolaymatrosov/nvelope/internal/auth/adapters"
	authdomain "github.com/nikolaymatrosov/nvelope/internal/auth/domain"
	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
	"github.com/nikolaymatrosov/nvelope/internal/tenant/adapters"
	"github.com/nikolaymatrosov/nvelope/internal/tenant/domain"
)

// testUserName is the name of every platform user newUser seeds.
const testUserName = "Test User"

// backend is one storage implementation of the tenant repositories under
// contract test. Every contract test body runs once per backend, so the
// Postgres and YDB adapters are held to the same behaviour.
type backend struct {
	name        string
	tenants     func(t *testing.T) domain.TenantRepository
	invitations func(t *testing.T) domain.InvitationRepository
	settings    func(t *testing.T) domain.SettingsRepository
	branding    func(t *testing.T) domain.BrandingRepository
	// users seeds platform users through the auth context's repository for
	// the same backend. On Postgres, memberships and invitations carry foreign
	// keys to platform_users, and ListMembers joins it on both backends.
	users func(t *testing.T) authdomain.UserRepository
}

var backends = []backend{
	{
		name: "pg",
		tenants: func(t *testing.T) domain.TenantRepository {
			return adapters.NewTenants(dbtest.AppPool(t))
		},
		invitations: func(t *testing.T) domain.InvitationRepository {
			return adapters.NewInvitations(dbtest.AppPool(t))
		},
		settings: func(t *testing.T) domain.SettingsRepository {
			return adapters.NewSettings(dbtest.AppPool(t))
		},
		branding: func(t *testing.T) domain.BrandingRepository {
			return adapters.NewBranding(dbtest.AppPool(t))
		},
		users: func(t *testing.T) authdomain.UserRepository {
			return authadapters.NewUsers(dbtest.AppPool(t))
		},
	},
	{
		name: "ydb",
		tenants: func(t *testing.T) domain.TenantRepository {
			return adapters.NewYDBTenants(dbtest.YDBMigrated(t))
		},
		invitations: func(t *testing.T) domain.InvitationRepository {
			return adapters.NewYDBInvitations(dbtest.YDBMigrated(t))
		},
		settings: func(t *testing.T) domain.SettingsRepository {
			return adapters.NewYDBSettings(dbtest.YDBMigrated(t))
		},
		branding: func(t *testing.T) domain.BrandingRepository {
			return adapters.NewYDBBranding(dbtest.YDBMigrated(t))
		},
		users: func(t *testing.T) authdomain.UserRepository {
			return authadapters.NewYDBUsers(dbtest.YDBMigrated(t))
		},
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

// newUser seeds a platform user with a fresh address through the backend's
// auth repository and returns its id and email.
func newUser(t *testing.T, b backend) (id, email string) {
	t.Helper()
	addr := dbtest.RandString() + "@example.com"
	e, err := authdomain.NewEmail(addr)
	require.NoError(t, err)
	u, err := authdomain.NewUser(e, testUserName)
	require.NoError(t, err)
	created, err := b.users(t).Create(context.Background(), u, "hash")
	require.NoError(t, err)
	return created.ID(), addr
}

// createWorkspace creates a tenant with a fresh slug owned by ownerID and
// returns the persisted tenant.
func createWorkspace(t *testing.T, repo domain.TenantRepository, ownerID string) *domain.Tenant {
	t.Helper()
	tn, err := domain.NewTenant("Workspace", "ws-"+dbtest.RandString())
	require.NoError(t, err)
	created, err := repo.CreateWorkspace(context.Background(), tn, ownerID)
	require.NoError(t, err)
	return created
}
```

- [ ] **Step 2: Rewrite the invitations tests as a contract suite**

```bash
git mv internal/tenant/adapters/invitations_pg_test.go internal/tenant/adapters/invitations_test.go
```

Replace the contents of `internal/tenant/adapters/invitations_test.go` with the code below. The new tests pin what Postgres gets from its schema and the YDB adapter must reproduce in code:
- `EmailMatchesCaseInsensitively`: `citext`.
- `PendingUniquenessIsPerTenantAndStatus`: the scope of the partial unique index.
- `CreateIsRaceFree`: the index under concurrency. On YDB this is the serializable SELECT-then-INSERT.
- `GetPendingRejectsExpired` and `ListPending`: had no test before.

Two existing tests gain assertions:
- `UpdateRevoke`: the revoked token no longer resolves.
- `UpdateNotFound`: another tenant's id is not found.

```go
package adapters_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
	"github.com/nikolaymatrosov/nvelope/internal/tenant/domain"
)

func newInvitation(t *testing.T, tenantID, invitedBy string) *domain.Invitation {
	t.Helper()
	return inviteEmail(t, tenantID, invitedBy, dbtest.RandString()+"@example.com")
}

// inviteEmail builds a pending invitation for addr that expires in an hour.
func inviteEmail(t *testing.T, tenantID, invitedBy, addr string) *domain.Invitation {
	t.Helper()
	email, err := domain.NewEmail(addr)
	require.NoError(t, err)
	inv, err := domain.NewInvitation(tenantID, email, invitedBy, time.Hour)
	require.NoError(t, err)
	return inv
}

func accept(inv *domain.Invitation) (*domain.Invitation, error) {
	if err := inv.Accept(time.Now()); err != nil {
		return nil, err
	}
	return inv, nil
}

func revoke(inv *domain.Invitation) (*domain.Invitation, error) {
	if err := inv.Revoke(); err != nil {
		return nil, err
	}
	return inv, nil
}

func TestInvitationsCreateAndGet(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.invitations(t)
		ctx := context.Background()

		ownerID, _ := newUser(t, b)
		ws := createWorkspace(t, b.tenants(t), ownerID)

		inv := newInvitation(t, ws.ID(), ownerID)
		tokenHash := dbtest.RandString()
		created, err := repo.Create(ctx, inv, tokenHash)
		require.NoError(t, err)
		require.NotEmpty(t, created.ID(), "the adapter assigns an id")
		require.Equal(t, domain.InvitationPending, created.Status())
		require.Equal(t, ws.ID(), created.TenantID())
		require.Equal(t, ownerID, created.InvitedBy())
		require.WithinDuration(t, time.Now(), created.CreatedAt(), time.Minute)

		got, err := repo.GetPendingByTokenHash(ctx, tokenHash)
		require.NoError(t, err)
		require.Equal(t, created.ID(), got.ID())
		require.Equal(t, ws.ID(), got.TenantID())
		require.Equal(t, inv.Email().String(), got.Email().String())
		require.Equal(t, ownerID, got.InvitedBy())
		require.WithinDuration(t, inv.ExpiresAt(), got.ExpiresAt(), time.Second)
	})
}

func TestInvitationsCreateRejectsDuplicatePending(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.invitations(t)
		ctx := context.Background()

		ownerID, _ := newUser(t, b)
		ws := createWorkspace(t, b.tenants(t), ownerID)
		addr := dbtest.RandString() + "@example.com"

		_, err := repo.Create(ctx, inviteEmail(t, ws.ID(), ownerID, addr), dbtest.RandString())
		require.NoError(t, err)

		_, err = repo.Create(ctx, inviteEmail(t, ws.ID(), ownerID, addr), dbtest.RandString())
		require.ErrorIs(t, err, domain.ErrInvitationExists)
	})
}

// TestInvitationsEmailMatchesCaseInsensitively pins what the citext column
// gives Postgres. The pending-uniqueness check ignores case, and the address
// is returned as it was entered.
func TestInvitationsEmailMatchesCaseInsensitively(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.invitations(t)
		ctx := context.Background()

		ownerID, _ := newUser(t, b)
		ws := createWorkspace(t, b.tenants(t), ownerID)

		mixed := "Bob." + dbtest.RandString() + "@Example.COM"
		tokenHash := dbtest.RandString()
		_, err := repo.Create(ctx, inviteEmail(t, ws.ID(), ownerID, mixed), tokenHash)
		require.NoError(t, err)

		got, err := repo.GetPendingByTokenHash(ctx, tokenHash)
		require.NoError(t, err)
		require.Equal(t, mixed, got.Email().String(), "the address is stored as entered")

		_, err = repo.Create(ctx, inviteEmail(t, ws.ID(), ownerID, strings.ToLower(mixed)), dbtest.RandString())
		require.ErrorIs(t, err, domain.ErrInvitationExists, "emails differing only in case collide")
	})
}

// TestInvitationsPendingUniquenessIsPerTenantAndStatus pins the scope of the
// Postgres partial unique index (tenant_id, email) WHERE status = 'pending'.
// Another tenant may invite the same address, and the address may be invited
// again once the earlier invitation is no longer pending.
func TestInvitationsPendingUniquenessIsPerTenantAndStatus(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.invitations(t)
		tenants := b.tenants(t)
		ctx := context.Background()

		ownerID, _ := newUser(t, b)
		first := createWorkspace(t, tenants, ownerID)
		second := createWorkspace(t, tenants, ownerID)
		addr := dbtest.RandString() + "@example.com"

		pending, err := repo.Create(ctx, inviteEmail(t, first.ID(), ownerID, addr), dbtest.RandString())
		require.NoError(t, err)
		_, err = repo.Create(ctx, inviteEmail(t, second.ID(), ownerID, addr), dbtest.RandString())
		require.NoError(t, err, "another tenant may invite the same address")

		require.NoError(t, repo.Update(ctx, pending.ID(), first.ID(), revoke))
		_, err = repo.Create(ctx, inviteEmail(t, first.ID(), ownerID, addr), dbtest.RandString())
		require.NoError(t, err, "a revoked invitation no longer blocks a new one")
	})
}

// TestInvitationsCreateIsRaceFree pins the pending-uniqueness guarantee under
// concurrency. Postgres gets it from the partial unique index. YDB gets it from
// the serializable SELECT-then-INSERT, which the SDK retries on conflict.
func TestInvitationsCreateIsRaceFree(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.invitations(t)
		ctx := context.Background()

		ownerID, _ := newUser(t, b)
		ws := createWorkspace(t, b.tenants(t), ownerID)
		addr := dbtest.RandString() + "@example.com"

		const n = 5
		errs := make([]error, n)
		var wg sync.WaitGroup
		for i := range n {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, errs[i] = repo.Create(ctx, inviteEmail(t, ws.ID(), ownerID, addr), dbtest.RandString())
			}()
		}
		wg.Wait()

		created := 0
		for _, err := range errs {
			if err == nil {
				created++
				continue
			}
			require.ErrorIs(t, err, domain.ErrInvitationExists)
		}
		require.Equal(t, 1, created, "exactly one concurrent invitation wins")

		pending, err := repo.ListPending(ctx, ws.ID())
		require.NoError(t, err)
		require.Len(t, pending, 1)
	})
}

func TestInvitationsGetPendingNotFound(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		_, err := b.invitations(t).GetPendingByTokenHash(context.Background(), dbtest.RandString())
		require.ErrorIs(t, err, domain.ErrInvitationNotFound)
	})
}

func TestInvitationsGetPendingRejectsExpired(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.invitations(t)
		ctx := context.Background()

		ownerID, _ := newUser(t, b)
		ws := createWorkspace(t, b.tenants(t), ownerID)

		email, err := domain.NewEmail(dbtest.RandString() + "@example.com")
		require.NoError(t, err)
		expired, err := domain.NewInvitation(ws.ID(), email, ownerID, -time.Hour)
		require.NoError(t, err)
		tokenHash := dbtest.RandString()
		_, err = repo.Create(ctx, expired, tokenHash)
		require.NoError(t, err)

		_, err = repo.GetPendingByTokenHash(ctx, tokenHash)
		require.ErrorIs(t, err, domain.ErrInvitationNotFound, "an expired invitation does not resolve")
	})
}

func TestInvitationsListPending(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.invitations(t)
		tenants := b.tenants(t)
		ctx := context.Background()

		ownerID, _ := newUser(t, b)
		ws := createWorkspace(t, tenants, ownerID)
		other := createWorkspace(t, tenants, ownerID)

		older, err := repo.Create(ctx, newInvitation(t, ws.ID(), ownerID), dbtest.RandString())
		require.NoError(t, err)
		newer, err := repo.Create(ctx, newInvitation(t, ws.ID(), ownerID), dbtest.RandString())
		require.NoError(t, err)
		revoked, err := repo.Create(ctx, newInvitation(t, ws.ID(), ownerID), dbtest.RandString())
		require.NoError(t, err)
		require.NoError(t, repo.Update(ctx, revoked.ID(), ws.ID(), revoke))
		_, err = repo.Create(ctx, newInvitation(t, other.ID(), ownerID), dbtest.RandString())
		require.NoError(t, err)

		pending, err := repo.ListPending(ctx, ws.ID())
		require.NoError(t, err)
		ids := make([]string, 0, len(pending))
		for _, inv := range pending {
			ids = append(ids, inv.ID())
		}
		require.Equal(t, []string{newer.ID(), older.ID()}, ids, "pending only, this tenant only, newest first")

		fresh := createWorkspace(t, tenants, ownerID)
		none, err := repo.ListPending(ctx, fresh.ID())
		require.NoError(t, err)
		require.Empty(t, none)
	})
}

func TestInvitationsUpdateAccept(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.invitations(t)
		ctx := context.Background()

		ownerID, _ := newUser(t, b)
		ws := createWorkspace(t, b.tenants(t), ownerID)
		created, err := repo.Create(ctx, newInvitation(t, ws.ID(), ownerID), dbtest.RandString())
		require.NoError(t, err)

		require.NoError(t, repo.Update(ctx, created.ID(), ws.ID(), accept))

		// The persisted status is the accepted one.
		err = repo.Update(ctx, created.ID(), ws.ID(), func(inv *domain.Invitation) (*domain.Invitation, error) {
			require.Equal(t, domain.InvitationAccepted, inv.Status())
			return inv, nil
		})
		require.NoError(t, err)
	})
}

func TestInvitationsUpdateRevoke(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.invitations(t)
		ctx := context.Background()

		ownerID, _ := newUser(t, b)
		ws := createWorkspace(t, b.tenants(t), ownerID)
		tokenHash := dbtest.RandString()
		created, err := repo.Create(ctx, newInvitation(t, ws.ID(), ownerID), tokenHash)
		require.NoError(t, err)

		require.NoError(t, repo.Update(ctx, created.ID(), ws.ID(), revoke))

		_, err = repo.GetPendingByTokenHash(ctx, tokenHash)
		require.ErrorIs(t, err, domain.ErrInvitationNotFound, "a revoked invitation does not resolve")
	})
}

func TestInvitationsUpdateNotFound(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.invitations(t)
		tenants := b.tenants(t)
		ctx := context.Background()

		ownerID, _ := newUser(t, b)
		ws := createWorkspace(t, tenants, ownerID)

		noop := func(inv *domain.Invitation) (*domain.Invitation, error) { return inv, nil }

		err := repo.Update(ctx, "not-a-uuid", ws.ID(), noop)
		require.ErrorIs(t, err, domain.ErrInvitationNotFound, "a malformed id is an opaque not-found")

		created, err := repo.Create(ctx, newInvitation(t, ws.ID(), ownerID), dbtest.RandString())
		require.NoError(t, err)
		other := createWorkspace(t, tenants, ownerID)
		err = repo.Update(ctx, created.ID(), other.ID(), noop)
		require.ErrorIs(t, err, domain.ErrInvitationNotFound, "another tenant's invitation is not found")
	})
}
```

- [ ] **Step 3: Run the suite to verify it fails**

Run: `go vet ./internal/tenant/adapters/`

Expected: build failure `undefined: adapters.NewYDBInvitations`.

- [ ] **Step 4: Implement `YDBInvitations`**

Create `internal/tenant/adapters/invitations_ydb.go`:

```go
package adapters

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	ydb "github.com/ydb-platform/ydb-go-sdk/v3"
	"github.com/ydb-platform/ydb-go-sdk/v3/query"

	"github.com/nikolaymatrosov/nvelope/internal/db"
	"github.com/nikolaymatrosov/nvelope/internal/tenant/domain"
)

// YDBInvitations is the YDB-backed implementation of
// domain.InvitationRepository, for the serverless runtime. Only the hash of an
// invitation token is ever stored. email keeps the address as entered and
// email_key holds its lower-cased form, standing in for the Postgres citext
// column.
type YDBInvitations struct {
	driver *ydb.Driver
}

var _ domain.InvitationRepository = (*YDBInvitations)(nil)

// NewYDBInvitations builds a YDBInvitations repository over the driver.
func NewYDBInvitations(d *ydb.Driver) *YDBInvitations {
	return &YDBInvitations{driver: d}
}

// ydbInvitationColumns is the projection scanYDBInvitation reads, in order.
const ydbInvitationColumns = `id, tenant_id, email, status, invited_by, created_at, expires_at`

// Create persists a new pending invitation, storing only the token's hash, and
// returns the persisted invitation. It returns domain.ErrInvitationExists when
// the tenant already has a pending invitation for the same email, ignoring
// case.
//
// Postgres enforces that rule with a partial unique index. YDB has no partial
// index, so the check and the insert share one serializable transaction. Of two
// concurrent Creates for the same address, one is aborted. The SDK retries it,
// and the retry sees the other's row (spec "Plan B3 planning spike").
func (r *YDBInvitations) Create(ctx context.Context, inv *domain.Invitation, tokenHash string) (*domain.Invitation, error) {
	id := uuid.NewString()
	emailKey := ydbInvitationEmailKey(inv.Email().String())
	var created *domain.Invitation
	err := r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		// ponytail: the check reads the tenant's whole invitation history (a
		// key-prefix range, every status); add a GLOBAL SYNC index on
		// (tenant_id, email_key) if that history grows large.
		_, err := tx.QueryRow(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $email_key AS Utf8;
SELECT id FROM invitations
WHERE tenant_id = $tenant_id AND email_key = $email_key AND status = "pending"u
LIMIT 1;`,
			query.WithParameters(ydb.ParamsBuilder().
				Param("$tenant_id").Text(inv.TenantID()).
				Param("$email_key").Text(emailKey).
				Build()))
		if err == nil {
			return domain.ErrInvitationExists
		}
		if !errors.Is(err, query.ErrNoRows) {
			return fmt.Errorf("checking pending invitations: %w", err)
		}
		row, err := tx.QueryRow(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DECLARE $email AS Utf8;
DECLARE $email_key AS Utf8;
DECLARE $token_hash AS Utf8;
DECLARE $invited_by AS Utf8;
DECLARE $expires_at AS Timestamp;
INSERT INTO invitations
    (tenant_id, id, email, email_key, role, token_hash, status, invited_by, created_at, expires_at)
VALUES ($tenant_id, $id, $email, $email_key, "admin"u, $token_hash, "pending"u, $invited_by,
    CurrentUtcTimestamp(), $expires_at)
RETURNING created_at;`,
			query.WithParameters(ydb.ParamsBuilder().
				Param("$tenant_id").Text(inv.TenantID()).
				Param("$id").Text(id).
				Param("$email").Text(inv.Email().String()).
				Param("$email_key").Text(emailKey).
				Param("$token_hash").Text(tokenHash).
				Param("$invited_by").Text(inv.InvitedBy()).
				Param("$expires_at").Timestamp(inv.ExpiresAt()).
				Build()))
		// Only the token_hash index can collide (the id is fresh). Postgres maps
		// every unique violation here to ErrInvitationExists too, and a random
		// token makes the collision negligible.
		if db.IsYDBConflict(err) {
			return domain.ErrInvitationExists
		}
		if err != nil {
			return fmt.Errorf("inserting invitation: %w", err)
		}
		var createdAt time.Time
		if err := row.Scan(&createdAt); err != nil {
			return fmt.Errorf("scanning invitation: %w", err)
		}
		created = domain.HydrateInvitation(id, inv.TenantID(), inv.Email().String(),
			string(domain.InvitationPending), inv.InvitedBy(), createdAt, inv.ExpiresAt())
		return nil
	})
	// The spike saw the conflict at the statement; this covers a YDB that
	// reports it at commit.
	if db.IsYDBConflict(err) {
		return nil, domain.ErrInvitationExists
	}
	if err != nil {
		return nil, err
	}
	return created, nil
}

// GetPendingByTokenHash returns the pending, unexpired invitation for a token
// hash, or domain.ErrInvitationNotFound. Unknown, expired, revoked and
// already-accepted tokens all yield the same error, so existence is not
// leaked. It is the one invitations read with no tenant predicate: the accept
// link carries only the token, so the lookup goes through the global unique
// index on token_hash.
func (r *YDBInvitations) GetPendingByTokenHash(ctx context.Context, tokenHash string) (*domain.Invitation, error) {
	return ydbInvitationFromRow(r.driver.Query().QueryRow(ctx, `DECLARE $token_hash AS Utf8;
SELECT `+ydbInvitationColumns+`
FROM invitations VIEW idx_invitations_token_hash
WHERE token_hash = $token_hash AND status = "pending"u AND expires_at > CurrentUtcTimestamp();`,
		query.WithParameters(ydb.ParamsBuilder().Param("$token_hash").Text(tokenHash).Build())))
}

// ListPending returns the tenant's pending invitations, newest first.
func (r *YDBInvitations) ListPending(ctx context.Context, tenantID string) ([]*domain.Invitation, error) {
	rs, err := r.driver.Query().QueryResultSet(ctx, `DECLARE $tenant_id AS Utf8;
SELECT `+ydbInvitationColumns+`
FROM invitations
WHERE tenant_id = $tenant_id AND status = "pending"u
ORDER BY created_at DESC;`, ydbTenantParam(tenantID))
	if err != nil {
		return nil, fmt.Errorf("listing invitations: %w", err)
	}
	defer func() { _ = rs.Close(ctx) }()
	invitations := []*domain.Invitation{}
	for row, err := range rs.Rows(ctx) {
		if err != nil {
			return nil, fmt.Errorf("listing invitations: %w", err)
		}
		inv, err := scanYDBInvitation(row)
		if err != nil {
			return nil, err
		}
		invitations = append(invitations, inv)
	}
	return invitations, nil
}

// Update loads the invitation, runs fn, and persists the resulting status in
// one serializable transaction. It returns domain.ErrInvitationNotFound when
// the tenant has no invitation with that id. The SDK retries the transaction
// on conflict, so fn may run more than once and must be retry-safe.
func (r *YDBInvitations) Update(ctx context.Context, id, tenantID string,
	fn func(*domain.Invitation) (*domain.Invitation, error)) error {

	return r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		loaded, err := ydbInvitationFromRow(tx.QueryRow(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
SELECT `+ydbInvitationColumns+`
FROM invitations
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
DECLARE $status AS Utf8;
UPDATE invitations SET status = $status
WHERE tenant_id = $tenant_id AND id = $id;`,
			query.WithParameters(ydb.ParamsBuilder().
				Param("$tenant_id").Text(tenantID).
				Param("$id").Text(updated.ID()).
				Param("$status").Text(string(updated.Status())).
				Build())); err != nil {
			return fmt.Errorf("updating invitation: %w", err)
		}
		return nil
	})
}

// ydbInvitationEmailKey is the case-insensitive key for an invitee address. It
// is compared only with other invitations' keys, never with
// platform_users.email_key, so the tenant context derives it itself, as its
// own domain.Email validates the address itself.
func ydbInvitationEmailKey(email string) string { return strings.ToLower(email) }

// ydbTenantAndID binds the (tenant_id, id) primary key.
func ydbTenantAndID(tenantID, id string) query.ExecuteOption {
	return query.WithParameters(ydb.ParamsBuilder().
		Param("$tenant_id").Text(tenantID).
		Param("$id").Text(id).
		Build())
}

// ydbInvitationFromRow maps a single-row read to the invitation, translating
// "no row" into domain.ErrInvitationNotFound.
func ydbInvitationFromRow(row query.Row, err error) (*domain.Invitation, error) {
	if errors.Is(err, query.ErrNoRows) {
		return nil, domain.ErrInvitationNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("loading invitation: %w", err)
	}
	return scanYDBInvitation(row)
}

// scanYDBInvitation reads one row in ydbInvitationColumns order.
func scanYDBInvitation(row query.Row) (*domain.Invitation, error) {
	var id, tenantID, email, status, invitedBy string
	var createdAt, expiresAt time.Time
	if err := row.Scan(&id, &tenantID, &email, &status, &invitedBy, &createdAt, &expiresAt); err != nil {
		return nil, fmt.Errorf("scanning invitation: %w", err)
	}
	return domain.HydrateInvitation(id, tenantID, email, status, invitedBy, createdAt, expiresAt), nil
}
```

- [ ] **Step 5: Run the suite to verify it passes**

Run: `export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock" && go test ./internal/tenant/... -v -timeout 8m 2>&1 | grep -E '^(=== RUN|--- (PASS|FAIL)|PASS|FAIL|ok)' | grep -E 'FAIL|/pg|/ydb|^ok'`

Expected: every `TestInvitations*`, `TestTenants*`, `TestSettings*` and `TestBranding*` shows `--- PASS` for both `/pg` and `/ydb`. Every `internal/tenant/...` package is `ok`.

Two cases are not yours to fix. Report BLOCKED with the verbatim output:
- **A `/pg` subtest fails.** In particular, a failure in `TestInvitationsEmailMatchesCaseInsensitively/pg`, `TestInvitationsPendingUniquenessIsPerTenantAndStatus/pg` or `TestInvitationsCreateIsRaceFree/pg` means Postgres does not behave as the plan assumes.
- **`TestInvitationsCreateIsRaceFree/ydb` gets more than one success, or an error other than `ErrInvitationExists`.** Print the errors with `%+v` in the report. It contradicts the spec's "Plan B3 planning spike" finding.

Any other `/ydb` failure: fix only `invitations_ydb.go`, and record the verbatim error and the fix in the report.

- [ ] **Step 6: Full verification and commit**

Run: `export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock" && go build ./... && go vet ./... && go test ./... -timeout 15m 2>&1 | grep -vE '^(ok|\?)\s'`

Expected: no output besides the build. In other words, no failing package.

Run: `golangci-lint run && export PATH="$HOME/go/bin:$PATH" && make lint-arch`

Expected: `0 issues.` and `[arch] clean`.

Run: `git diff --stat HEAD -- go.mod go.sum`

Expected: no output.

```bash
gofmt -l internal/tenant/
git add internal/tenant/adapters/invitations_ydb.go internal/tenant/adapters/invitations_test.go \
  internal/tenant/adapters/helpers_test.go
git commit -m "feat(tenant): add the YDB invitation repository under a pg+ydb contract suite"
```

`gofmt -l` must print nothing, and `git status` must show `invitations_pg_test.go` → `invitations_test.go` as a rename.

---

### Task 3: The YDB onboarding and its contract suite

**Files:**
- Modify: `internal/auth/adapters/users_ydb.go` (export `InsertYDBUser`)
- Modify: `internal/auth/adapters/sessions_ydb.go` (export `InsertYDBSession`)
- Modify: `internal/tenant/adapters/invitations_ydb.go` (add `AcceptYDBInvitation`)
- Create: `internal/service/onboarding_ydb.go`
- Create: `internal/service/onboarding_test.go`

**Interfaces:**
- Consumes (B2): `insertYDBUser(ctx, q query.Executor, id string, u *domain.User, passwordHash string) error` and `insertYDBSession(ctx, q query.Executor, s *domain.Session, tokenHash string) error`. This task exports them under the same signatures.
- Consumes (Task 1): `tenantadapters.AddYDBMembership(ctx, tx query.TxActor, m *domain.Membership) error` and `tenantadapters.NewYDBTenants`.
- Consumes (Task 2): `tenantadapters.NewYDBInvitations`, `ydbTenantAndID`, and the `invitations` columns `status`, `accepted_by`, `accepted_at`, `expires_at`.
- Consumes the port `tenantcommand.Onboarding` (`internal/tenant/app/command/accept_invitation.go`), unchanged, and the pg `onboarding` / `newOnboarding` (`internal/service/onboarding.go`), unchanged.
- Produces:
  ```go
  // package adapters (internal/auth/adapters)
  func InsertYDBUser(ctx context.Context, q query.Executor, id string, u *domain.User, passwordHash string) error
  func InsertYDBSession(ctx context.Context, q query.Executor, s *domain.Session, tokenHash string) error
  // package adapters (internal/tenant/adapters)
  func AcceptYDBInvitation(ctx context.Context, tx query.TxActor, tenantID, invitationID, acceptedBy string) error
  // package service (internal/service)
  type ydbOnboarding struct{ /* unexported */ }
  func newYDBOnboarding(d *ydb.Driver, hasher authcommand.PasswordHasher, sessionTTL time.Duration) ydbOnboarding
  ```
- Plan C relies on `newYDBOnboarding` to wire the serverless composition root. Until then only `onboarding_test.go` uses it. `golangci-lint` lints test files, so `unused` stays quiet.

**Decision: how the two YDB writers of `platform_users` share the email key.**
- `ydbOnboarding` calls auth's exported `InsertYDBUser` and `InsertYDBSession` inside its own `DoTx`. It does not repeat their SQL, and it does not add an `Email.Key()` method.
- `ydbEmailKey` stays the single derivation of `platform_users.email_key`.
- The change is two renames. An `Email.Key()` method would still leave the insert SQL duplicated, and it would not help `LookupByEmail`, which takes a raw string.
- Tenant tables follow the same rule: the onboarding calls `AcceptYDBInvitation` and `AddYDBMembership`, which the tenant adapters export.

- [ ] **Step 1: Export the auth insert helpers**

Run: `sed -i '' 's/insertYDBUser/InsertYDBUser/g; s/insertYDBSession/InsertYDBSession/g' internal/auth/adapters/users_ydb.go internal/auth/adapters/sessions_ydb.go`

Then in `internal/auth/adapters/users_ydb.go`, replace:

```go
// InsertYDBUser inserts one platform_users row through q, which may be the
// query client or a transaction. A conflict on the unique email_key index maps
// to domain.ErrEmailTaken. YDB reports a primary-key collision the same way,
// which a random UUID makes negligible.
```

with:

```go
// InsertYDBUser inserts one platform_users row through q, which may be the
// query client or a transaction. It is the only writer of platform_users on
// YDB. The serverless onboarding (internal/service) calls it inside its own
// transaction, so email_key has a single derivation (ydbEmailKey). A conflict
// on the unique email_key index maps to domain.ErrEmailTaken. YDB reports a
// primary-key collision the same way, which a random UUID makes negligible.
```

In `internal/auth/adapters/sessions_ydb.go`, replace:

```go
// InsertYDBSession persists a session through q, which may be the query client
// or a transaction. A transaction lets YDBUsers issue a session atomically
// with the user it belongs to.
```

with:

```go
// InsertYDBSession persists a session through q, which may be the query client
// or a transaction. A transaction lets YDBUsers and the serverless onboarding
// (internal/service) issue a session atomically with the user it belongs to.
```

Run: `grep -rn 'insertYDBUser\|insertYDBSession' internal/ ; export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock" && go build ./... && go test ./internal/auth/adapters/ -run 'TestUsers|TestSessions' -timeout 8m`

Expected: `grep` prints nothing, the build succeeds, and the package is `ok`.

- [ ] **Step 2: Write the onboarding contract suite**

Create `internal/service/onboarding_test.go`. The pg `onboarding` had no direct test; only the `internal/api` handler tests reach it. One suite over both implementations holds the YDB variant to its behaviour. `TestOnboardingAcceptForNewUserRejectsTakenEmail` seeds the existing account with an upper-cased address, so it also pins the shared email-key derivation.

```go
package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	authadapters "github.com/nikolaymatrosov/nvelope/internal/auth/adapters"
	authdomain "github.com/nikolaymatrosov/nvelope/internal/auth/domain"
	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
	tenantadapters "github.com/nikolaymatrosov/nvelope/internal/tenant/adapters"
	tenantcommand "github.com/nikolaymatrosov/nvelope/internal/tenant/app/command"
	tenantdomain "github.com/nikolaymatrosov/nvelope/internal/tenant/domain"
	"github.com/nikolaymatrosov/nvelope/internal/token"
)

const onboardingPassword = "correct-horse-battery"

// onboardingBackend is one implementation of tenantcommand.Onboarding under
// contract test, with the repositories of the same backend that seed the
// fixtures and check the outcome.
type onboardingBackend struct {
	name        string
	onboarding  func(t *testing.T) tenantcommand.Onboarding
	users       func(t *testing.T) authdomain.UserRepository
	sessions    func(t *testing.T) authdomain.SessionRepository
	tenants     func(t *testing.T) tenantdomain.TenantRepository
	invitations func(t *testing.T) tenantdomain.InvitationRepository
}

var onboardingBackends = []onboardingBackend{
	{
		name: "pg",
		onboarding: func(t *testing.T) tenantcommand.Onboarding {
			return newOnboarding(dbtest.AppPool(t), authadapters.NewPasswordHasher(), time.Hour)
		},
		users: func(t *testing.T) authdomain.UserRepository {
			return authadapters.NewUsers(dbtest.AppPool(t))
		},
		sessions: func(t *testing.T) authdomain.SessionRepository {
			return authadapters.NewSessions(dbtest.AppPool(t))
		},
		tenants: func(t *testing.T) tenantdomain.TenantRepository {
			return tenantadapters.NewTenants(dbtest.AppPool(t))
		},
		invitations: func(t *testing.T) tenantdomain.InvitationRepository {
			return tenantadapters.NewInvitations(dbtest.AppPool(t))
		},
	},
	{
		name: "ydb",
		onboarding: func(t *testing.T) tenantcommand.Onboarding {
			return newYDBOnboarding(dbtest.YDBMigrated(t), authadapters.NewPasswordHasher(), time.Hour)
		},
		users: func(t *testing.T) authdomain.UserRepository {
			return authadapters.NewYDBUsers(dbtest.YDBMigrated(t))
		},
		sessions: func(t *testing.T) authdomain.SessionRepository {
			return authadapters.NewYDBSessions(dbtest.YDBMigrated(t))
		},
		tenants: func(t *testing.T) tenantdomain.TenantRepository {
			return tenantadapters.NewYDBTenants(dbtest.YDBMigrated(t))
		},
		invitations: func(t *testing.T) tenantdomain.InvitationRepository {
			return tenantadapters.NewYDBInvitations(dbtest.YDBMigrated(t))
		},
	},
}

// eachOnboardingBackend runs body once per backend, as parallel subtests named
// after it.
func eachOnboardingBackend(t *testing.T, body func(t *testing.T, b onboardingBackend)) {
	t.Helper()
	for _, b := range onboardingBackends {
		t.Run(b.name, func(t *testing.T) {
			t.Parallel()
			body(t, b)
		})
	}
}

// invited is a seeded workspace with one invitation.
type invited struct {
	tenantID     string
	ownerID      string
	invitationID string
	tokenHash    string
	email        string
}

// seedInvitation creates a workspace owner, their workspace and an invitation
// for a fresh address that expires ttl from now.
func seedInvitation(t *testing.T, b onboardingBackend, ttl time.Duration) invited {
	t.Helper()
	ctx := context.Background()
	ownerID := seedUser(t, b, dbtest.RandString()+"@example.com")

	tn, err := tenantdomain.NewTenant("Workspace", "ws-"+dbtest.RandString())
	require.NoError(t, err)
	ws, err := b.tenants(t).CreateWorkspace(ctx, tn, ownerID)
	require.NoError(t, err)

	addr := "invitee." + dbtest.RandString() + "@example.com"
	email, err := tenantdomain.NewEmail(addr)
	require.NoError(t, err)
	inv, err := tenantdomain.NewInvitation(ws.ID(), email, ownerID, ttl)
	require.NoError(t, err)
	tokenHash := dbtest.RandString()
	created, err := b.invitations(t).Create(ctx, inv, tokenHash)
	require.NoError(t, err)

	return invited{
		tenantID: ws.ID(), ownerID: ownerID, invitationID: created.ID(),
		tokenHash: tokenHash, email: addr,
	}
}

// seedUser registers a platform user with the address and returns its id.
func seedUser(t *testing.T, b onboardingBackend, addr string) string {
	t.Helper()
	email, err := authdomain.NewEmail(addr)
	require.NoError(t, err)
	u, err := authdomain.NewUser(email, "Existing User")
	require.NoError(t, err)
	created, err := b.users(t).Create(context.Background(), u, "hash")
	require.NoError(t, err)
	return created.ID()
}

func newUserAcceptance(inv invited) tenantcommand.NewUserAcceptance {
	return tenantcommand.NewUserAcceptance{
		InvitationID: inv.invitationID,
		TenantID:     inv.tenantID,
		Email:        inv.email,
		Password:     onboardingPassword,
		Name:         "Ada",
	}
}

func TestOnboardingAcceptForNewUser(t *testing.T) {
	t.Parallel()
	eachOnboardingBackend(t, func(t *testing.T, b onboardingBackend) {
		ctx := context.Background()
		inv := seedInvitation(t, b, time.Hour)

		got, err := b.onboarding(t).AcceptForNewUser(ctx, newUserAcceptance(inv))
		require.NoError(t, err)
		require.NotEmpty(t, got.ID)
		require.Equal(t, inv.email, got.Email)
		require.Equal(t, "Ada", got.Name)
		require.NotEmpty(t, got.SessionToken)

		// The account exists, with the password hashed.
		user, hash, err := b.users(t).GetCredentials(ctx, inv.email)
		require.NoError(t, err)
		require.Equal(t, got.ID, user.ID())
		require.True(t, authadapters.NewPasswordHasher().Verify(hash, onboardingPassword))

		// The returned token resolves to a live session for the new account.
		session, err := b.sessions(t).ResolveByTokenHash(ctx, token.Hash(got.SessionToken))
		require.NoError(t, err)
		require.Equal(t, got.ID, session.UserID())

		// The account joined as an admin, and the invitation is spent.
		role, err := b.tenants(t).GetMembershipRole(ctx, got.ID, inv.tenantID)
		require.NoError(t, err)
		require.Equal(t, tenantdomain.RoleAdmin, role)
		_, err = b.invitations(t).GetPendingByTokenHash(ctx, inv.tokenHash)
		require.ErrorIs(t, err, tenantdomain.ErrInvitationNotFound)
	})
}

// TestOnboardingAcceptForNewUserRejectsTakenEmail seeds the existing account
// with the invitee's address in upper case. The collision therefore depends on
// the case-insensitive email key, which citext gives Postgres and the shared
// InsertYDBUser gives YDB.
func TestOnboardingAcceptForNewUserRejectsTakenEmail(t *testing.T) {
	t.Parallel()
	eachOnboardingBackend(t, func(t *testing.T, b onboardingBackend) {
		ctx := context.Background()
		inv := seedInvitation(t, b, time.Hour)
		seedUser(t, b, strings.ToUpper(inv.email))

		_, err := b.onboarding(t).AcceptForNewUser(ctx, newUserAcceptance(inv))
		require.ErrorIs(t, err, authdomain.ErrEmailTaken)

		// The transaction rolled back, so the invitation is still pending.
		_, err = b.invitations(t).GetPendingByTokenHash(ctx, inv.tokenHash)
		require.NoError(t, err)
	})
}

func TestOnboardingAcceptForNewUserRollsBackOnSpentInvitation(t *testing.T) {
	t.Parallel()
	eachOnboardingBackend(t, func(t *testing.T, b onboardingBackend) {
		ctx := context.Background()
		inv := seedInvitation(t, b, time.Hour)
		err := b.invitations(t).Update(ctx, inv.invitationID, inv.tenantID,
			func(i *tenantdomain.Invitation) (*tenantdomain.Invitation, error) {
				if err := i.Revoke(); err != nil {
					return nil, err
				}
				return i, nil
			})
		require.NoError(t, err)

		_, err = b.onboarding(t).AcceptForNewUser(ctx, newUserAcceptance(inv))
		require.ErrorIs(t, err, tenantdomain.ErrInvitationNotFound)

		// The account insert rolled back with the failed acceptance.
		_, err = b.users(t).LookupByEmail(ctx, inv.email)
		require.ErrorIs(t, err, authdomain.ErrUserNotFound)
	})
}

func TestOnboardingAcceptForExistingUser(t *testing.T) {
	t.Parallel()
	eachOnboardingBackend(t, func(t *testing.T, b onboardingBackend) {
		ctx := context.Background()
		inv := seedInvitation(t, b, time.Hour)
		userID := seedUser(t, b, inv.email)
		acceptance := tenantcommand.ExistingUserAcceptance{
			InvitationID: inv.invitationID, TenantID: inv.tenantID, UserID: userID,
		}

		require.NoError(t, b.onboarding(t).AcceptForExistingUser(ctx, acceptance))

		role, err := b.tenants(t).GetMembershipRole(ctx, userID, inv.tenantID)
		require.NoError(t, err)
		require.Equal(t, tenantdomain.RoleAdmin, role)
		_, err = b.invitations(t).GetPendingByTokenHash(ctx, inv.tokenHash)
		require.ErrorIs(t, err, tenantdomain.ErrInvitationNotFound)

		// A spent invitation cannot be accepted again.
		err = b.onboarding(t).AcceptForExistingUser(ctx, acceptance)
		require.ErrorIs(t, err, tenantdomain.ErrInvitationNotFound)
	})
}

// TestOnboardingAcceptForExistingMemberKeepsRole pins the ON CONFLICT DO
// NOTHING of the Postgres membership insert: accepting never demotes a member.
func TestOnboardingAcceptForExistingMemberKeepsRole(t *testing.T) {
	t.Parallel()
	eachOnboardingBackend(t, func(t *testing.T, b onboardingBackend) {
		ctx := context.Background()
		inv := seedInvitation(t, b, time.Hour)

		require.NoError(t, b.onboarding(t).AcceptForExistingUser(ctx, tenantcommand.ExistingUserAcceptance{
			InvitationID: inv.invitationID, TenantID: inv.tenantID, UserID: inv.ownerID,
		}))

		role, err := b.tenants(t).GetMembershipRole(ctx, inv.ownerID, inv.tenantID)
		require.NoError(t, err)
		require.Equal(t, tenantdomain.RoleOwner, role, "the owner is not demoted")
	})
}

func TestOnboardingAcceptForExistingUserRejectsExpired(t *testing.T) {
	t.Parallel()
	eachOnboardingBackend(t, func(t *testing.T, b onboardingBackend) {
		ctx := context.Background()
		inv := seedInvitation(t, b, -time.Hour)
		userID := seedUser(t, b, dbtest.RandString()+"@example.com")

		err := b.onboarding(t).AcceptForExistingUser(ctx, tenantcommand.ExistingUserAcceptance{
			InvitationID: inv.invitationID, TenantID: inv.tenantID, UserID: userID,
		})
		require.ErrorIs(t, err, tenantdomain.ErrInvitationNotFound)

		_, err = b.tenants(t).GetMembershipRole(ctx, userID, inv.tenantID)
		require.ErrorIs(t, err, tenantdomain.ErrNotMember)
	})
}
```

- [ ] **Step 3: Run the suite to verify it fails**

Run: `go vet ./internal/service/`

Expected: build failure `undefined: newYDBOnboarding`.

- [ ] **Step 4: Add `AcceptYDBInvitation`**

Append to `internal/tenant/adapters/invitations_ydb.go`. It needs no new import.

```go
// AcceptYDBInvitation marks a pending, unexpired invitation of the tenant
// accepted by acceptedBy, inside tx. When the invitation is not (or no longer)
// acceptable it returns domain.ErrInvitationNotFound, so the caller's whole
// transaction rolls back with the opaque error. UPDATE ... RETURNING yields no
// row when the guard does not match (spec "Plan B3 planning spike"), which
// stands in for the Postgres RowsAffected check. The serverless onboarding
// (internal/service) calls it.
func AcceptYDBInvitation(ctx context.Context, tx query.TxActor, tenantID, invitationID, acceptedBy string) error {
	_, err := tx.QueryRow(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DECLARE $accepted_by AS Utf8;
UPDATE invitations
SET status = "accepted"u, accepted_by = $accepted_by, accepted_at = CurrentUtcTimestamp()
WHERE tenant_id = $tenant_id AND id = $id
    AND status = "pending"u AND expires_at > CurrentUtcTimestamp()
RETURNING id;`,
		query.WithParameters(ydb.ParamsBuilder().
			Param("$tenant_id").Text(tenantID).
			Param("$id").Text(invitationID).
			Param("$accepted_by").Text(acceptedBy).
			Build()))
	if errors.Is(err, query.ErrNoRows) {
		return domain.ErrInvitationNotFound
	}
	if err != nil {
		return fmt.Errorf("accepting invitation: %w", err)
	}
	return nil
}
```

- [ ] **Step 5: Implement `ydbOnboarding`**

Create `internal/service/onboarding_ydb.go`:

```go
package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	ydb "github.com/ydb-platform/ydb-go-sdk/v3"
	"github.com/ydb-platform/ydb-go-sdk/v3/query"

	authadapters "github.com/nikolaymatrosov/nvelope/internal/auth/adapters"
	authcommand "github.com/nikolaymatrosov/nvelope/internal/auth/app/command"
	authdomain "github.com/nikolaymatrosov/nvelope/internal/auth/domain"
	"github.com/nikolaymatrosov/nvelope/internal/db"
	tenantadapters "github.com/nikolaymatrosov/nvelope/internal/tenant/adapters"
	tenantcommand "github.com/nikolaymatrosov/nvelope/internal/tenant/app/command"
	tenantdomain "github.com/nikolaymatrosov/nvelope/internal/tenant/domain"
	"github.com/nikolaymatrosov/nvelope/internal/token"
)

// ydbOnboarding is the serverless (YDB) implementation of
// tenant/app/command.Onboarding, the counterpart of onboarding. Each operation
// is one serializable DoTx. Every row is written through a helper that the
// owning context exports:
//   - auth: InsertYDBUser and InsertYDBSession;
//   - tenant: AcceptYDBInvitation and AddYDBMembership.
//
// The email_key derivation and every table's SQL therefore have one source.
type ydbOnboarding struct {
	driver     *ydb.Driver
	hasher     authcommand.PasswordHasher
	sessionTTL time.Duration
}

var _ tenantcommand.Onboarding = ydbOnboarding{}

func newYDBOnboarding(d *ydb.Driver, hasher authcommand.PasswordHasher, sessionTTL time.Duration) ydbOnboarding {
	return ydbOnboarding{driver: d, hasher: hasher, sessionTTL: sessionTTL}
}

// AcceptForExistingUser marks the invitation accepted and records the
// membership for an already-registered user, in one transaction. A user who
// is already a member keeps their role.
func (o ydbOnboarding) AcceptForExistingUser(ctx context.Context, p tenantcommand.ExistingUserAcceptance) error {
	membership, err := tenantdomain.NewMembership(p.UserID, p.TenantID, tenantdomain.RoleAdmin)
	if err != nil {
		return err
	}
	return o.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		if err := tenantadapters.AcceptYDBInvitation(ctx, tx, p.TenantID, p.InvitationID, p.UserID); err != nil {
			return err
		}
		return tenantadapters.AddYDBMembership(ctx, tx, membership)
	})
}

// AcceptForNewUser creates the account, issues a session, marks the invitation
// accepted, and records the membership for a new user, all in one
// transaction. The user id, the session, the token and the password hash are
// fixed before DoTx. The closure runs only database statements, so the SDK can
// re-run it safely.
func (o ydbOnboarding) AcceptForNewUser(ctx context.Context, p tenantcommand.NewUserAcceptance) (tenantcommand.NewlyOnboardedUser, error) {
	email, err := authdomain.NewEmail(p.Email)
	if err != nil {
		return tenantcommand.NewlyOnboardedUser{}, err
	}
	if _, err := authdomain.NewPassword(p.Password); err != nil {
		return tenantcommand.NewlyOnboardedUser{}, err
	}
	user, err := authdomain.NewUser(email, p.Name)
	if err != nil {
		return tenantcommand.NewlyOnboardedUser{}, err
	}
	hash, err := o.hasher.Hash(p.Password)
	if err != nil {
		return tenantcommand.NewlyOnboardedUser{}, fmt.Errorf("hashing password: %w", err)
	}
	rawToken, err := token.New()
	if err != nil {
		return tenantcommand.NewlyOnboardedUser{}, err
	}
	userID := uuid.NewString()
	session, err := authdomain.NewSession(userID, o.sessionTTL)
	if err != nil {
		return tenantcommand.NewlyOnboardedUser{}, err
	}
	membership, err := tenantdomain.NewMembership(userID, p.TenantID, tenantdomain.RoleAdmin)
	if err != nil {
		return tenantcommand.NewlyOnboardedUser{}, err
	}

	err = o.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		if err := authadapters.InsertYDBUser(ctx, tx, userID, user, hash); err != nil {
			return err
		}
		if err := authadapters.InsertYDBSession(ctx, tx, session, token.Hash(rawToken)); err != nil {
			return err
		}
		if err := tenantadapters.AcceptYDBInvitation(ctx, tx, p.TenantID, p.InvitationID, userID); err != nil {
			return err
		}
		return tenantadapters.AddYDBMembership(ctx, tx, membership)
	})
	// The spike saw the conflict at the statement; this covers a YDB that
	// reports it at commit. Only the email_key index can collide: every other
	// key this transaction writes is freshly generated.
	if db.IsYDBConflict(err) {
		return tenantcommand.NewlyOnboardedUser{}, authdomain.ErrEmailTaken
	}
	if err != nil {
		return tenantcommand.NewlyOnboardedUser{}, err
	}
	return tenantcommand.NewlyOnboardedUser{
		ID:           userID,
		Email:        email.String(),
		Name:         user.Name(),
		SessionToken: rawToken,
	}, nil
}
```

- [ ] **Step 6: Run the suites to verify they pass**

Run: `export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock" && go test ./internal/service/ ./internal/auth/... ./internal/tenant/... -v -timeout 8m 2>&1 | grep -E '^(=== RUN|--- (PASS|FAIL)|PASS|FAIL|ok)' | grep -E 'FAIL|/pg|/ydb|^ok'`

Expected:
- Every `TestOnboarding*` shows `--- PASS` for both `/pg` and `/ydb`.
- Every auth and tenant contract test still passes on both backends.
- `internal/service` and every `internal/auth/...` and `internal/tenant/...` package is `ok`.

Two cases are not yours to fix. Report BLOCKED with the verbatim output:
- **A `/pg` subtest fails.** The pg `onboarding` does not behave as this contract assumes. Do not change `onboarding.go`; the contract or the plan is wrong.
- **`TestOnboardingAcceptForNewUserRejectsTakenEmail/ydb` fails.** That means `InsertYDBUser` is no longer the path the onboarding writes through, or the email-key derivation diverged. Print the error with `%+v` in the report.

Any other `/ydb` failure: fix only `onboarding_ydb.go` or `AcceptYDBInvitation`, and record the verbatim error and the fix in the report.

- [ ] **Step 7: Full verification and commit**

Run: `export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock" && go build ./... && go vet ./... && go test ./... -timeout 15m 2>&1 | grep -vE '^(ok|\?)\s'`

Expected: no output besides the build. In other words, no failing package.

Run: `golangci-lint run && export PATH="$HOME/go/bin:$PATH" && make lint-arch`

Expected: `0 issues.` and `[arch] clean`. If `unused` reports `newYDBOnboarding` or `ydbOnboarding`, the linter is not seeing `onboarding_test.go`. Report BLOCKED rather than adding a `//nolint` or a dummy caller.

Run: `git diff --stat HEAD -- go.mod go.sum`

Expected: no output.

```bash
gofmt -l internal/service/ internal/auth/ internal/tenant/
git add internal/auth/adapters/users_ydb.go internal/auth/adapters/sessions_ydb.go \
  internal/tenant/adapters/invitations_ydb.go internal/service/onboarding_ydb.go \
  internal/service/onboarding_test.go
git commit -m "feat(service): add the YDB onboarding under a pg+ydb contract suite"
```

`gofmt -l` must print nothing.

---

## Done criteria for Plan B3

- `go build ./... && go vet ./... && go test ./...` passes. Every eachBackend test in `internal/tenant/adapters` and every `TestOnboarding*` in `internal/service` passes on both `/pg` and `/ydb`.
- `golangci-lint run` reports `0 issues.`, and `make lint-arch` reports `[arch] clean`.
- No `go.mod` or `go.sum` change.
- `TestInvitationsEmailMatchesCaseInsensitively`, `TestInvitationsPendingUniquenessIsPerTenantAndStatus` and `TestInvitationsCreateIsRaceFree` pass on both backends. YDB therefore reproduces the `citext` partial unique index without a partial index.
- `TestOnboardingAcceptForNewUserRejectsTakenEmail/ydb` passes: the onboarding and `YDBUsers` share one email-key derivation.
- The final report lists what stays open for the next plans:
  - **Plan B4 (iam) needs nothing from the tenant tables.** The iam pg tables (`000005_tenant_access_schema.up.sql`) reference `tenants` and `platform_users` only through foreign keys, which YDB does not have. No iam adapter reads a tenant table. iam's own `users.email` is `citext` with `UNIQUE (tenant_id, email)`, so it needs its own lower-cased key column, the way `invitations.email_key` works here.
  - **Plan C wiring.** The serverless composition root wires:
    - `NewYDBTenants`, `NewYDBInvitations`, `NewYDBSettings` and `NewYDBBranding`;
    - `newYDBOnboarding` in place of `newOnboarding`;
    - `newMemberDirectory` over the YDB users and tenants repositories, which needs no change.
  - **Plan C single-tenant bootstrap.** `POST /api/platform/tenants` must return 409 once the tenant exists:
    - For the configured slug, `CreateWorkspace` already returns `ErrSlugTaken` (409).
    - Refusing a second tenant under any other slug needs a "does any tenant exist" read that `TenantRepository` lacks. It must sit in the same `DoTx` as the create to be race-free.
  - **Plan C `/cron/analytics-refresh`** needs the YDB form of `cmd/scheduler`'s `SELECT id FROM tenants WHERE status = 'active'` (`status = "active"u`).
  - **Campaign plan.** Continuing a transaction after an INSERT conflict (`Recipients.BulkInsert`) is still unverified. This plan avoided it with SELECT-then-INSERT and did not test it.
  - **Ceiling.** `YDBInvitations.Create` reads the tenant's whole invitation history for the pending check. It is marked `ponytail:` in code and listed under the spec's "Ceilings".
