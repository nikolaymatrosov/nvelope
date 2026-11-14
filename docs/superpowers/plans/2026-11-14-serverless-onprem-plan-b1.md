# Serverless On-Prem — Plan B1 (YDB foundation and sending pilot) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:**
- Build the YDB storage foundation: the migration runner and the pg+ydb contract-test harness.
- Prove it on the smallest context, `sending`, with a working `YDBSendingDomains` repository held to the same contract tests as Postgres.
- Add the YDB throttle counter that replaces Redis in serverless mode.

**Architecture:**
- **Migrations.** YDB migrations live in `internal/db/ydbmigrations/`, one file per ported context. `db.MigrateYDB` applies each statement in its own `Exec` and records versions in `schema_migrations`.
- **Adapters.** YDB adapters sit next to the pg adapters behind the same domain ports. They call `db.Query().DoTx` directly and filter every query by `tenant_id`.
- **Contract tests.** Each repository test body runs once per backend through a tiny `eachBackend` helper.
- **Throttling.** `ydbthrottle.Counter` is a multi-window fixed-window counter in one serializable transaction. It backs the campaign `RateLimiter` and the two single-key throttle ports.

**Tech Stack:**
- Go 1.26
- `github.com/ydb-platform/ydb-go-sdk/v3` v3.136.4 (query service)
- `github.com/ydb-platform/ydb-go-genproto` (`Ydb.StatusIds`; already an indirect dependency)
- testcontainers-go v0.42.0, local-ydb `26.3.1.23`
- testify, `github.com/google/uuid`

**Design spec:** [docs/superpowers/specs/2026-11-14-serverless-onprem-design.md](../specs/2026-11-14-serverless-onprem-design.md). The binding sections are "Storage layer (YDB)", "Throttling", "Testing" and "Spike findings (Plan A)", especially V4 and V6.

**Scope:** This is the first of the Plan B plans. Later plans port tenant, iam, auth, audience, campaign, deliverability and media in that order. Each copies the Task 2 pattern: a migration file, an `*_ydb.go` adapter, and an `eachBackend` contract suite.

## Global Constraints

- **Branch.** Work on a branch created from `feat/serverless-onprem`, in its own worktree outside the main checkout. The main checkout belongs to other sessions.
- **Docker.** Docker on this machine is OrbStack. Prefix every `go test` / `make test` / `make ci` with `export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock" && `. Ryuk reaps test containers between runs, so every `go test` run starts fresh containers; reuse by name works only within one run.
- **YDB container.** It is `ydbplatform/local-ydb:26.3.1.23`, amd64 under Rosetta. A fresh container takes about 45 s to start, so use `-timeout 8m` on runs that touch YDB.
- **Dependencies.** No new Go modules. `github.com/ydb-platform/ydb-go-genproto` moves from indirect to direct, and `go mod tidy` does that.
- **SaaS must not change.** Postgres adapters, River, Redis and every existing test keep their behaviour. `go test ./...` passes after every task.
- **YQL conventions** (spec "SQL translation rules" and V4):
  - Every query `DECLARE`s each parameter: `DECLARE $tenant_id AS Utf8;`.
  - Literals that target a `Utf8` column take the `u` suffix (`"pending"u`); a bare `"..."` is `String`.
  - `CurrentUtcTimestamp() + Interval(...)` is `Optional<Timestamp>`; wrap it in `Unwrap(...)` for `NOT NULL` columns.
  - Avoid YQL keywords as identifiers: `throttle_key`, not `key`; `version_id`, not `version`.
  - Every DDL statement is idempotent (`CREATE TABLE IF NOT EXISTS`, `DROP ... IF EXISTS`).
  - A statement ends on a line ending in `;`. Never end a line with `;` inside a string literal.
- **Tenant scoping.** Tenant-scoped tables have a primary key that starts with `tenant_id`, and every query filters `WHERE tenant_id = $tenant_id`. The decision is recorded in the spec's Transactions section.
- **IDs** are UUIDv4 strings generated in Go (`uuid.NewString()`) and stored as `Utf8`.
- **`DoTx` closures must be retry-safe** (V6):
  - only database operations inside;
  - captured outcome variables are reset at the top of the closure;
  - never `append` to a slice declared outside the closure.
- **Duplicate key mapping.** "Already exists" comes from `db.IsYDBConflict(err)`: `PRECONDITION_FAILED` plus YQL issue 2012. Never match on the message text.
- **Commits.** Use conventional messages. **No `Co-Authored-By` trailer** of any kind.
- **Tooling.** `goimports` is not installed, so use `gofmt -w`. `golangci-lint` is on PATH. `make lint-arch` needs `export PATH="$HOME/go/bin:$PATH" && `.

## File structure

| File | Responsibility |
|------|----------------|
| `internal/db/ydbmigrate.go` | `MigrateYDB`, `MigrateYDBFrom`, statement splitter, version parser |
| `internal/db/ydbmigrations/000001_sending.up.sql` | `sending_domains` table |
| `internal/db/ydbmigrations/000002_throttle.up.sql` | `throttle_windows` table |
| `internal/db/ydb.go` | `IsYDBConflict` |
| `internal/db/ydbmigrate_internal_test.go` | pure unit tests: splitter, version parser |
| `internal/db/ydbmigrate_test.go` | runner integration tests against local YDB |
| `internal/dbtest/ydb.go` (modify) | add `YDBMigrated(t)` |
| `internal/sending/adapters/domains_ydb.go` | `YDBSendingDomains` |
| `internal/sending/adapters/domains_test.go` (renamed from `domains_pg_test.go`) | pg+ydb contract suite |
| `internal/sending/adapters/helpers_test.go` (modify) | `backend`, `backends`, `eachBackend` |
| `internal/sending/adapters/verify_worker.go` (modify) | reset `snooze` at the start of the closure (V6) |
| `internal/platform/ydbthrottle/counter.go` | `Window`, `Counter`, `Keyed` |
| `internal/platform/ydbthrottle/counter_test.go` | counter tests with a fixed clock |
| `internal/campaign/adapters/ratelimiter_ydb.go` (+ `_test.go`) | campaign `RateLimiter` over the counter |
| `Makefile` (modify) | `lint-arch`: forbid `ydb-platform` in domain and transport packages |

---

### Task 1: YDB migration runner, the sending schema, and `dbtest.YDBMigrated`

**Files:**
- Create: `internal/db/ydbmigrate.go`
- Create: `internal/db/ydbmigrations/000001_sending.up.sql`
- Create: `internal/db/ydbmigrate_internal_test.go`
- Create: `internal/db/ydbmigrate_test.go`
- Modify: `internal/dbtest/ydb.go` (append `YDBMigrated`)

**Interfaces:**
- Consumes: `dbtest.YDB(t) *ydb.Driver` (Plan A, `internal/dbtest/ydb.go`).
- Produces:
  ```go
  // package db
  var YDBMigrationsFS embed.FS
  func MigrateYDB(ctx context.Context, d *ydb.Driver) error
  func MigrateYDBFrom(ctx context.Context, q query.Executor, fsys fs.FS) error // *.up.sql at fsys root
  // package dbtest
  func YDBMigrated(t *testing.T) *ydb.Driver // YDB(t) with every embedded migration applied, once per test binary
  ```
- Table `sending_domains` exists after `YDBMigrated`. Task 2 relies on it.

- [ ] **Step 1: Write the failing unit tests**

Create `internal/db/ydbmigrate_internal_test.go`:

```go
package db

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSplitYQLStatements(t *testing.T) {
	t.Parallel()
	got := splitYQLStatements(`-- header comment

CREATE TABLE IF NOT EXISTS a (
    id Utf8 NOT NULL,
    PRIMARY KEY (id)
);
-- between
CREATE TABLE IF NOT EXISTS b (id Utf8 NOT NULL, PRIMARY KEY (id));

-- trailing comment only
`)
	require.Equal(t, []string{
		"-- header comment\n\nCREATE TABLE IF NOT EXISTS a (\n    id Utf8 NOT NULL,\n    PRIMARY KEY (id)\n);",
		"-- between\nCREATE TABLE IF NOT EXISTS b (id Utf8 NOT NULL, PRIMARY KEY (id));",
	}, got)
}

func TestYDBMigrationVersion(t *testing.T) {
	t.Parallel()
	v, err := ydbMigrationVersion("000012_audience.up.sql")
	require.NoError(t, err)
	require.Equal(t, uint64(12), v)

	_, err = ydbMigrationVersion("audience.up.sql")
	require.Error(t, err, "a name without a <version>_ prefix is rejected")

	_, err = ydbMigrationVersion("v1_audience.up.sql")
	require.Error(t, err, "a non-numeric version is rejected")
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./internal/db/ -run 'TestSplitYQLStatements|TestYDBMigrationVersion'`

Expected: build failure `undefined: splitYQLStatements` and `undefined: ydbMigrationVersion`.

- [ ] **Step 3: Write the sending migration**

Create `internal/db/ydbmigrations/000001_sending.up.sql`:

```sql
-- Sending domains (internal/sending). Tenant-scoped: every query filters by
-- tenant_id, and the primary key starts with it so that filter is a key-prefix
-- lookup. dkim_records is JSON that is only stored and read back whole, so it
-- is a plain Utf8 column.
CREATE TABLE IF NOT EXISTS sending_domains (
    tenant_id            Utf8      NOT NULL,
    id                   Utf8      NOT NULL,
    domain               Utf8      NOT NULL,
    status               Utf8      NOT NULL,
    dkim_records         Utf8      NOT NULL,
    spf_record           Utf8      NOT NULL,
    dmarc_record         Utf8      NOT NULL,
    postbox_identity_ref Utf8      NOT NULL,
    failure_reason       Utf8      NOT NULL,
    created_at           Timestamp NOT NULL,
    verified_at          Timestamp,
    last_checked_at      Timestamp,
    PRIMARY KEY (tenant_id, id),
    INDEX idx_sending_domains_tenant_domain GLOBAL UNIQUE SYNC ON (tenant_id, domain)
);
```

- [ ] **Step 4: Implement the runner**

Create `internal/db/ydbmigrate.go`:

```go
package db

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strconv"
	"strings"

	ydb "github.com/ydb-platform/ydb-go-sdk/v3"
	"github.com/ydb-platform/ydb-go-sdk/v3/query"
)

// YDBMigrationsFS holds the YDB schema migrations — one file per ported
// bounded context — applied by MigrateYDB for the serverless runtime.
//
//go:embed ydbmigrations/*.up.sql
var YDBMigrationsFS embed.FS

// MigrateYDB applies every embedded YDB migration that schema_migrations does
// not record yet, in version order.
func MigrateYDB(ctx context.Context, d *ydb.Driver) error {
	sub, err := fs.Sub(YDBMigrationsFS, "ydbmigrations")
	if err != nil {
		return fmt.Errorf("opening embedded ydb migrations: %w", err)
	}
	return MigrateYDBFrom(ctx, d.Query(), sub)
}

// MigrateYDBFrom applies the *.up.sql files at the root of fsys that
// schema_migrations does not record yet, in version order.
//
// YDB runs DDL outside transactions and caps query text at 10 KB, so every
// statement runs in its own Exec and a failure can leave a file half-applied.
// Each statement must therefore be idempotent (CREATE ... IF NOT EXISTS, DROP
// ... IF EXISTS): a re-run re-applies the unrecorded file from its first
// statement and converges. A version is recorded only after all of its
// statements succeed.
func MigrateYDBFrom(ctx context.Context, q query.Executor, fsys fs.FS) error {
	if err := q.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
    version_id Uint64    NOT NULL,
    applied_at Timestamp NOT NULL,
    PRIMARY KEY (version_id)
);`); err != nil {
		return fmt.Errorf("creating schema_migrations: %w", err)
	}
	applied, err := appliedYDBVersions(ctx, q)
	if err != nil {
		return err
	}
	names, err := fs.Glob(fsys, "*.up.sql")
	if err != nil {
		return fmt.Errorf("listing ydb migrations: %w", err)
	}
	type migration struct {
		version uint64
		name    string
	}
	migrations := make([]migration, 0, len(names))
	for _, name := range names {
		version, err := ydbMigrationVersion(name)
		if err != nil {
			return err
		}
		migrations = append(migrations, migration{version: version, name: name})
	}
	sort.Slice(migrations, func(i, j int) bool { return migrations[i].version < migrations[j].version })

	for _, m := range migrations {
		if applied[m.version] {
			continue
		}
		body, err := fs.ReadFile(fsys, m.name)
		if err != nil {
			return fmt.Errorf("reading ydb migration %s: %w", m.name, err)
		}
		for i, stmt := range splitYQLStatements(string(body)) {
			if err := q.Exec(ctx, stmt); err != nil {
				return fmt.Errorf("ydb migration %s, statement %d: %w", m.name, i+1, err)
			}
		}
		if err := q.Exec(ctx, `DECLARE $version_id AS Uint64;
UPSERT INTO schema_migrations (version_id, applied_at) VALUES ($version_id, CurrentUtcTimestamp());`,
			query.WithParameters(ydb.ParamsBuilder().Param("$version_id").Uint64(m.version).Build()),
		); err != nil {
			return fmt.Errorf("recording ydb migration %s: %w", m.name, err)
		}
	}
	return nil
}

// appliedYDBVersions reads the versions schema_migrations records.
func appliedYDBVersions(ctx context.Context, q query.Executor) (map[uint64]bool, error) {
	rs, err := q.QueryResultSet(ctx, "SELECT version_id FROM schema_migrations;")
	if err != nil {
		return nil, fmt.Errorf("reading schema_migrations: %w", err)
	}
	defer func() { _ = rs.Close(ctx) }()
	applied := map[uint64]bool{}
	for row, err := range rs.Rows(ctx) {
		if err != nil {
			return nil, fmt.Errorf("reading schema_migrations: %w", err)
		}
		var version uint64
		if err := row.Scan(&version); err != nil {
			return nil, fmt.Errorf("scanning schema_migrations: %w", err)
		}
		applied[version] = true
	}
	return applied, nil
}

// ydbMigrationVersion parses the leading number of a migration file name
// ("000001_sending.up.sql" → 1).
func ydbMigrationVersion(name string) (uint64, error) {
	digits, _, ok := strings.Cut(path.Base(name), "_")
	if !ok {
		return 0, fmt.Errorf("ydb migration %s: name must be <version>_<name>.up.sql", name)
	}
	version, err := strconv.ParseUint(digits, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("ydb migration %s: bad version: %w", name, err)
	}
	return version, nil
}

// splitYQLStatements splits a migration file into statements: a statement
// ends on a line whose trimmed text ends in ";". Chunks holding only blank
// lines and "--" comments are dropped.
//
// ponytail: line-based, so a ";" at the end of a line inside a string literal
// would split a statement; keep migration literals off line ends. Use a real
// YQL lexer if that ever bites.
func splitYQLStatements(sql string) []string {
	var out []string
	var cur strings.Builder
	flush := func() {
		if s := strings.TrimSpace(cur.String()); hasYQLCode(s) {
			out = append(out, s)
		}
		cur.Reset()
	}
	for _, line := range strings.Split(sql, "\n") {
		cur.WriteString(line)
		cur.WriteByte('\n')
		if strings.HasSuffix(strings.TrimSpace(line), ";") {
			flush()
		}
	}
	flush()
	return out
}

// hasYQLCode reports whether s holds anything besides blank lines and "--"
// comments.
func hasYQLCode(s string) bool {
	for _, line := range strings.Split(s, "\n") {
		if t := strings.TrimSpace(line); t != "" && !strings.HasPrefix(t, "--") {
			return true
		}
	}
	return false
}
```

- [ ] **Step 5: Run the unit tests to verify they pass**

Run: `go test ./internal/db/ -run 'TestSplitYQLStatements|TestYDBMigrationVersion' -v`

Expected: both PASS.

- [ ] **Step 6: Write the failing integration tests**

Create `internal/db/ydbmigrate_test.go`:

```go
package db_test

import (
	"context"
	"fmt"
	"math/rand/v2"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"
	ydb "github.com/ydb-platform/ydb-go-sdk/v3"
	"github.com/ydb-platform/ydb-go-sdk/v3/query"

	"github.com/nikolaymatrosov/nvelope/internal/db"
	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
)

func TestMigrateYDBAppliesEmbeddedSchema(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	d := dbtest.YDBMigrated(t)

	// sending_domains comes from 000001_sending.up.sql.
	row, err := d.Query().QueryRow(ctx, "SELECT COUNT(*) AS n FROM sending_domains;")
	require.NoError(t, err)
	var n uint64
	require.NoError(t, row.Scan(&n))

	require.NoError(t, db.MigrateYDB(ctx, d), "re-running over an up-to-date schema is a no-op")
}

func TestMigrateYDBConvergesAfterPartialFailure(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	q := dbtest.YDBMigrated(t).Query()

	// A version far above the real migrations and unique per run: the test
	// shares schema_migrations with the embedded set.
	version := 900_000_000 + rand.IntN(99_999_999)
	file := fmt.Sprintf("%d_partial.up.sql", version)
	tableA := fmt.Sprintf("mig_a_%d", version)
	tableB := fmt.Sprintf("mig_b_%d", version)
	createA := "CREATE TABLE IF NOT EXISTS " + tableA + " (id Utf8 NOT NULL, PRIMARY KEY (id));"
	createB := "CREATE TABLE IF NOT EXISTS " + tableB + " (id Utf8 NOT NULL, PRIMARY KEY (id));"

	broken := fstest.MapFS{file: {Data: []byte(createA + "\nTHIS IS NOT YQL;\n")}}
	require.Error(t, db.MigrateYDBFrom(ctx, q, broken), "the second statement fails")
	require.False(t, ydbVersionRecorded(t, q, version), "a half-applied file is not recorded")

	fixed := fstest.MapFS{file: {Data: []byte(createA + "\n" + createB + "\n")}}
	require.NoError(t, db.MigrateYDBFrom(ctx, q, fixed),
		"the re-run re-applies the file from its first statement and converges")
	require.True(t, ydbVersionRecorded(t, q, version))
	for _, table := range []string{tableA, tableB} {
		_, err := q.QueryRow(ctx, "SELECT COUNT(*) AS n FROM "+table+";")
		require.NoError(t, err, table)
	}
}

func ydbVersionRecorded(t *testing.T, q query.Executor, version int) bool {
	t.Helper()
	row, err := q.QueryRow(context.Background(), `DECLARE $version_id AS Uint64;
SELECT COUNT(*) AS n FROM schema_migrations WHERE version_id = $version_id;`,
		query.WithParameters(ydb.ParamsBuilder().Param("$version_id").Uint64(uint64(version)).Build()))
	require.NoError(t, err)
	var n uint64
	require.NoError(t, row.Scan(&n))
	return n == 1
}
```

- [ ] **Step 7: Run them to verify they fail**

Run: `go test ./internal/db/ -run 'TestMigrateYDB' -timeout 8m`

Expected: build failure `undefined: dbtest.YDBMigrated`.

- [ ] **Step 8: Add `YDBMigrated`**

Append to `internal/dbtest/ydb.go`. It already imports `context`, `sync`, `testing`, `time`, `require` and `ydb`. Add the import `"github.com/nikolaymatrosov/nvelope/internal/db"`, which `internal/dbtest/dbtest.go` already uses, so it causes no cycle.

```go
var (
	ydbMigrateOnce sync.Once
	ydbMigrateErr  error
)

// YDBMigrated returns a driver for the test YDB with every embedded YDB
// migration (internal/db/ydbmigrations) applied, once per test binary. Test
// binaries in one `go test ./...` run share the container and may race on the
// same idempotent DDL, so a failed attempt is retried until it converges.
func YDBMigrated(t *testing.T) *ydb.Driver {
	t.Helper()
	d := YDB(t)
	ydbMigrateOnce.Do(func() {
		deadline := time.Now().Add(ydbReadyTimeout)
		for {
			ydbMigrateErr = db.MigrateYDB(context.Background(), d)
			if ydbMigrateErr == nil || time.Now().After(deadline) {
				return
			}
			time.Sleep(time.Second)
		}
	})
	require.NoError(t, ydbMigrateErr)
	return d
}
```

- [ ] **Step 9: Run the integration tests to verify they pass**

Run: `export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock" && go test ./internal/db/ ./internal/dbtest/ -v -timeout 8m`

Expected: `TestMigrateYDBAppliesEmbeddedSchema`, `TestMigrateYDBConvergesAfterPartialFailure`, the two unit tests and the existing dbtest tests all PASS.

If `MigrateYDBConvergesAfterPartialFailure` fails because the broken file's error is not returned, or because the re-run does not converge, stop and report it verbatim. That failure path is the open V4 question this test exists to answer.

- [ ] **Step 10: Commit**

```bash
go build ./... && go vet ./internal/db/ ./internal/dbtest/
git add internal/db/ydbmigrate.go internal/db/ydbmigrations/000001_sending.up.sql \
  internal/db/ydbmigrate_internal_test.go internal/db/ydbmigrate_test.go internal/dbtest/ydb.go
git commit -m "feat(db): add the YDB migration runner and the sending schema"
```

---

### Task 2: `YDBSendingDomains` and the pg+ydb contract suite

**Files:**
- Create: `internal/db/ydb.go`
- Create: `internal/sending/adapters/domains_ydb.go`
- Rename: `internal/sending/adapters/domains_pg_test.go` → `internal/sending/adapters/domains_test.go` (rewritten)
- Modify: `internal/sending/adapters/helpers_test.go`
- Modify: `Makefile` (the `lint-arch` grep patterns)

**Interfaces:**
- Consumes: `dbtest.YDBMigrated(t)` and the `sending_domains` table from Task 1; `domain.SendingDomainRepository`, unchanged:
  `Add`, `Get`, `Update`, `All` and `PendingIDs` in `internal/sending/domain/repository.go`.
- Produces:
  ```go
  // package db
  func IsYDBConflict(err error) bool
  // package adapters (internal/sending/adapters)
  type YDBSendingDomains struct{ /* unexported */ }
  func NewYDBSendingDomains(d *ydb.Driver) *YDBSendingDomains
  // package adapters_test — the harness shape every later context copies
  type backend struct {
      name      string
      repo      func(t *testing.T) domain.SendingDomainRepository
      newTenant func(t *testing.T) string
  }
  func eachBackend(t *testing.T, body func(t *testing.T, b backend))
  ```

- [ ] **Step 1: Write the harness**

Replace `internal/sending/adapters/helpers_test.go` with:

```go
package adapters_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
	"github.com/nikolaymatrosov/nvelope/internal/sending/adapters"
	"github.com/nikolaymatrosov/nvelope/internal/sending/domain"
)

// seedTenant inserts a tenant row directly and returns its id. The sending
// adapters need a real tenant to satisfy the tenant_id foreign key and the RLS
// policy.
func seedTenant(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var id string
	require.NoError(t, pool.QueryRow(context.Background(),
		`INSERT INTO tenants (name, slug, status) VALUES ('Workspace', $1, 'active') RETURNING id`,
		"snd-"+dbtest.RandString()).Scan(&id))
	return id
}

// backend is one storage implementation of the sending repository under
// contract test. Every contract test body runs once per backend, so the
// Postgres and YDB adapters are held to the same behaviour.
type backend struct {
	name string
	// repo opens the backend's repository.
	repo func(t *testing.T) domain.SendingDomainRepository
	// newTenant returns a fresh tenant id valid for the backend: a real tenants
	// row for Postgres (foreign key + RLS), a bare UUID for YDB (no foreign
	// keys; isolation is the explicit tenant_id predicate).
	newTenant func(t *testing.T) string
}

var backends = []backend{
	{
		name: "pg",
		repo: func(t *testing.T) domain.SendingDomainRepository {
			return adapters.NewSendingDomains(dbtest.AppPool(t))
		},
		newTenant: func(t *testing.T) string { return seedTenant(t, dbtest.AppPool(t)) },
	},
	{
		name: "ydb",
		repo: func(t *testing.T) domain.SendingDomainRepository {
			return adapters.NewYDBSendingDomains(dbtest.YDBMigrated(t))
		},
		newTenant: func(*testing.T) string { return uuid.NewString() },
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
```

- [ ] **Step 2: Rewrite the repository tests as a contract suite**

```bash
git mv internal/sending/adapters/domains_pg_test.go internal/sending/adapters/domains_test.go
```

Replace the contents of `internal/sending/adapters/domains_test.go` with the code below. `newDomain` keeps its signature, because `verify_worker_test.go` uses it.

```go
package adapters_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/sending/adapters"
	"github.com/nikolaymatrosov/nvelope/internal/sending/domain"
)

func newDomain(t *testing.T, tenantID, name string) *domain.SendingDomain {
	t.Helper()
	d, err := domain.NewSendingDomain(tenantID, name)
	require.NoError(t, err)
	d.ApplyProvisioning("identity-"+name, []domain.DNSRecord{
		{Type: "CNAME", Name: "sel._domainkey." + name, Value: "sel.dkim"},
	}, adapters.ComposeSPF(), adapters.ComposeDMARC())
	return d
}

func TestSendingDomainsAddGetAll(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.repo(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		id, err := repo.Add(ctx, tenantID, newDomain(t, tenantID, "mail.acme.com"))
		require.NoError(t, err)
		_, err = repo.Add(ctx, tenantID, newDomain(t, tenantID, "news.acme.com"))
		require.NoError(t, err)

		got, err := repo.Get(ctx, tenantID, id)
		require.NoError(t, err)
		require.Equal(t, id, got.ID())
		require.Equal(t, tenantID, got.TenantID())
		require.Equal(t, "mail.acme.com", got.Domain())
		require.Equal(t, domain.StatusPending, got.Status())
		require.Equal(t, []domain.DNSRecord{
			{Type: "CNAME", Name: "sel._domainkey.mail.acme.com", Value: "sel.dkim"},
		}, got.DKIMRecords())
		require.Equal(t, adapters.ComposeSPF(), got.SPFRecord())
		require.Equal(t, adapters.ComposeDMARC(), got.DMARCRecord())
		require.Equal(t, "identity-mail.acme.com", got.IdentityRef())
		require.WithinDuration(t, time.Now(), got.CreatedAt(), time.Minute)
		require.Nil(t, got.VerifiedAt())
		require.Nil(t, got.LastCheckedAt())

		all, err := repo.All(ctx, tenantID)
		require.NoError(t, err)
		require.Len(t, all, 2)
		require.Equal(t, "mail.acme.com", all[0].Domain(), "All is ordered by domain")
		require.Equal(t, "news.acme.com", all[1].Domain())
	})
}

func TestSendingDomainsAddDuplicateConflicts(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.repo(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		_, err := repo.Add(ctx, tenantID, newDomain(t, tenantID, "mail.acme.com"))
		require.NoError(t, err)
		_, err = repo.Add(ctx, tenantID, newDomain(t, tenantID, "mail.acme.com"))
		require.ErrorIs(t, err, domain.ErrDomainAlreadyExists)

		// The same domain name is free in another tenant.
		other := b.newTenant(t)
		_, err = repo.Add(ctx, other, newDomain(t, other, "mail.acme.com"))
		require.NoError(t, err)
	})
}

func TestSendingDomainsUpdateTransition(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.repo(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		id, err := repo.Add(ctx, tenantID, newDomain(t, tenantID, "mail.acme.com"))
		require.NoError(t, err)
		pending, err := repo.PendingIDs(ctx, tenantID)
		require.NoError(t, err)
		require.Equal(t, []string{id}, pending)

		require.NoError(t, repo.Update(ctx, tenantID, id,
			func(d *domain.SendingDomain) (*domain.SendingDomain, error) {
				d.RecordCheck(time.Now())
				return d, d.MarkVerified(time.Now())
			}))

		got, err := repo.Get(ctx, tenantID, id)
		require.NoError(t, err)
		require.Equal(t, domain.StatusVerified, got.Status())
		require.NotNil(t, got.VerifiedAt())
		require.NotNil(t, got.LastCheckedAt())

		pending, err = repo.PendingIDs(ctx, tenantID)
		require.NoError(t, err)
		require.Empty(t, pending, "a verified domain is no longer pending")
	})
}

func TestSendingDomainsUpdatePropagatesClosureError(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.repo(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		id, err := repo.Add(ctx, tenantID, newDomain(t, tenantID, "mail.acme.com"))
		require.NoError(t, err)

		errBoom := errors.New("boom")
		err = repo.Update(ctx, tenantID, id, func(d *domain.SendingDomain) (*domain.SendingDomain, error) {
			_ = d.MarkVerified(time.Now()) // mutate, then fail
			return d, errBoom
		})
		require.ErrorIs(t, err, errBoom)

		got, err := repo.Get(ctx, tenantID, id)
		require.NoError(t, err)
		require.Equal(t, domain.StatusPending, got.Status(), "a failed closure persists nothing")
	})
}

func TestSendingDomainsGetMissing(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.repo(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		_, err := repo.Get(ctx, tenantID, "not-a-uuid")
		require.ErrorIs(t, err, domain.ErrDomainNotFound)

		err = repo.Update(ctx, tenantID, "not-a-uuid",
			func(d *domain.SendingDomain) (*domain.SendingDomain, error) { return d, nil })
		require.ErrorIs(t, err, domain.ErrDomainNotFound)
	})
}

// TestSendingDomainsCrossTenantIsolation proves one tenant's domains are
// invisible to another even when a foreign id is passed directly: RLS
// enforces it on Postgres, the explicit tenant_id predicate on YDB.
func TestSendingDomainsCrossTenantIsolation(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.repo(t)
		ctx := context.Background()
		tenantA := b.newTenant(t)
		tenantB := b.newTenant(t)

		idA, err := repo.Add(ctx, tenantA, newDomain(t, tenantA, "mail.acme.com"))
		require.NoError(t, err)

		_, err = repo.Get(ctx, tenantB, idA)
		require.ErrorIs(t, err, domain.ErrDomainNotFound)

		err = repo.Update(ctx, tenantB, idA, func(d *domain.SendingDomain) (*domain.SendingDomain, error) {
			return d, d.MarkVerified(time.Now())
		})
		require.ErrorIs(t, err, domain.ErrDomainNotFound)

		all, err := repo.All(ctx, tenantB)
		require.NoError(t, err)
		require.Empty(t, all)
		pending, err := repo.PendingIDs(ctx, tenantB)
		require.NoError(t, err)
		require.Empty(t, pending)

		got, err := repo.Get(ctx, tenantA, idA)
		require.NoError(t, err)
		require.Equal(t, domain.StatusPending, got.Status())
	})
}
```

- [ ] **Step 3: Run the suite to verify it fails**

Run: `go vet ./internal/sending/adapters/`

Expected: build failure `undefined: adapters.NewYDBSendingDomains`.

- [ ] **Step 4: Add `IsYDBConflict`**

Create `internal/db/ydb.go`:

```go
package db

import (
	"github.com/ydb-platform/ydb-go-genproto/protos/Ydb"
	ydb "github.com/ydb-platform/ydb-go-sdk/v3"
)

// ydbIssueConflictWithExistingKey is the YQL issue code YDB attaches to
// PRECONDITION_FAILED when an INSERT hits an existing primary key or a unique
// index value ("Conflict with existing key").
const ydbIssueConflictWithExistingKey = 2012

// IsYDBConflict reports whether err is YDB's "Conflict with existing key" — an
// INSERT that collided with an existing primary key or GLOBAL UNIQUE index
// value. It is the YDB counterpart of IsUniqueViolation. YDB reports both
// kinds of collision identically, so a caller cannot tell which key collided.
func IsYDBConflict(err error) bool {
	if !ydb.IsOperationError(err, Ydb.StatusIds_PRECONDITION_FAILED) {
		return false
	}
	conflict := false
	ydb.IterateByIssues(err, func(_ string, code Ydb.StatusIds_StatusCode, _ uint32) {
		if code == ydbIssueConflictWithExistingKey {
			conflict = true
		}
	})
	return conflict
}
```

- [ ] **Step 5: Implement the YDB repository**

Create `internal/sending/adapters/domains_ydb.go`:

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
	"github.com/nikolaymatrosov/nvelope/internal/sending/domain"
)

// YDBSendingDomains is the YDB-backed implementation of
// domain.SendingDomainRepository, for the serverless runtime. YDB has no
// row-level security, so every statement filters by tenant_id explicitly; the
// primary key (tenant_id, id) makes that filter a key-prefix lookup.
type YDBSendingDomains struct {
	driver *ydb.Driver
}

var _ domain.SendingDomainRepository = (*YDBSendingDomains)(nil)

// NewYDBSendingDomains builds a YDBSendingDomains repository over the driver.
func NewYDBSendingDomains(d *ydb.Driver) *YDBSendingDomains {
	return &YDBSendingDomains{driver: d}
}

const ydbSendingDomainColumns = `id, tenant_id, domain, status, dkim_records, spf_record,
    dmarc_record, postbox_identity_ref, failure_reason, created_at, verified_at, last_checked_at`

const ydbSelectSendingDomain = `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
SELECT ` + ydbSendingDomainColumns + `
FROM sending_domains
WHERE tenant_id = $tenant_id AND id = $id;`

// Add persists a new sending domain under a fresh UUID and returns the id. It
// returns domain.ErrDomainAlreadyExists when the tenant already has the domain.
func (r *YDBSendingDomains) Add(ctx context.Context, tenantID string, d *domain.SendingDomain) (string, error) {
	dkimJSON, err := json.Marshal(d.DKIMRecords())
	if err != nil {
		return "", fmt.Errorf("encoding dkim records: %w", err)
	}
	id := uuid.NewString()
	err = r.driver.Query().Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DECLARE $domain AS Utf8;
DECLARE $status AS Utf8;
DECLARE $dkim_records AS Utf8;
DECLARE $spf_record AS Utf8;
DECLARE $dmarc_record AS Utf8;
DECLARE $postbox_identity_ref AS Utf8;
DECLARE $failure_reason AS Utf8;
INSERT INTO sending_domains
    (tenant_id, id, domain, status, dkim_records, spf_record, dmarc_record,
     postbox_identity_ref, failure_reason, created_at)
VALUES ($tenant_id, $id, $domain, $status, $dkim_records, $spf_record, $dmarc_record,
     $postbox_identity_ref, $failure_reason, CurrentUtcTimestamp());`,
		query.WithParameters(ydb.ParamsBuilder().
			Param("$tenant_id").Text(tenantID).
			Param("$id").Text(id).
			Param("$domain").Text(d.Domain()).
			Param("$status").Text(string(d.Status())).
			Param("$dkim_records").Text(string(dkimJSON)).
			Param("$spf_record").Text(d.SPFRecord()).
			Param("$dmarc_record").Text(d.DMARCRecord()).
			Param("$postbox_identity_ref").Text(d.IdentityRef()).
			Param("$failure_reason").Text(d.FailureReason()).
			Build()))
	if db.IsYDBConflict(err) {
		return "", domain.ErrDomainAlreadyExists
	}
	if err != nil {
		return "", fmt.Errorf("inserting sending domain: %w", err)
	}
	return id, nil
}

// Get returns the domain, or domain.ErrDomainNotFound.
func (r *YDBSendingDomains) Get(ctx context.Context, tenantID, id string) (*domain.SendingDomain, error) {
	return ydbSendingDomainFromRow(r.driver.Query().QueryRow(ctx, ydbSelectSendingDomain, ydbTenantAndID(tenantID, id)))
}

// Update loads the domain, runs fn, and persists the result in one
// serializable transaction. The SDK retries the transaction on conflict, so fn
// may run more than once and must be retry-safe.
func (r *YDBSendingDomains) Update(ctx context.Context, tenantID, id string,
	fn func(*domain.SendingDomain) (*domain.SendingDomain, error)) error {

	return r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		loaded, err := ydbSendingDomainFromRow(tx.QueryRow(ctx, ydbSelectSendingDomain, ydbTenantAndID(tenantID, id)))
		if err != nil {
			return err
		}
		updated, err := fn(loaded)
		if err != nil {
			return err
		}
		dkimJSON, err := json.Marshal(updated.DKIMRecords())
		if err != nil {
			return fmt.Errorf("encoding dkim records: %w", err)
		}
		err = tx.Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DECLARE $status AS Utf8;
DECLARE $dkim_records AS Utf8;
DECLARE $spf_record AS Utf8;
DECLARE $dmarc_record AS Utf8;
DECLARE $postbox_identity_ref AS Utf8;
DECLARE $failure_reason AS Utf8;
DECLARE $verified_at AS Optional<Timestamp>;
DECLARE $last_checked_at AS Optional<Timestamp>;
UPDATE sending_domains SET
    status = $status, dkim_records = $dkim_records, spf_record = $spf_record,
    dmarc_record = $dmarc_record, postbox_identity_ref = $postbox_identity_ref,
    failure_reason = $failure_reason,
    verified_at = $verified_at, last_checked_at = $last_checked_at
WHERE tenant_id = $tenant_id AND id = $id;`,
			query.WithParameters(ydb.ParamsBuilder().
				Param("$tenant_id").Text(tenantID).
				Param("$id").Text(id).
				Param("$status").Text(string(updated.Status())).
				Param("$dkim_records").Text(string(dkimJSON)).
				Param("$spf_record").Text(updated.SPFRecord()).
				Param("$dmarc_record").Text(updated.DMARCRecord()).
				Param("$postbox_identity_ref").Text(updated.IdentityRef()).
				Param("$failure_reason").Text(updated.FailureReason()).
				Param("$verified_at").BeginOptional().Timestamp(updated.VerifiedAt()).EndOptional().
				Param("$last_checked_at").BeginOptional().Timestamp(updated.LastCheckedAt()).EndOptional().
				Build()))
		if err != nil {
			return fmt.Errorf("updating sending domain: %w", err)
		}
		return nil
	})
}

// All returns every sending domain of the tenant, ordered by domain name.
func (r *YDBSendingDomains) All(ctx context.Context, tenantID string) ([]*domain.SendingDomain, error) {
	rs, err := r.driver.Query().QueryResultSet(ctx, `DECLARE $tenant_id AS Utf8;
SELECT `+ydbSendingDomainColumns+`
FROM sending_domains
WHERE tenant_id = $tenant_id
ORDER BY domain;`, ydbTenant(tenantID))
	if err != nil {
		return nil, fmt.Errorf("listing sending domains: %w", err)
	}
	defer func() { _ = rs.Close(ctx) }()
	var domains []*domain.SendingDomain
	for row, err := range rs.Rows(ctx) {
		if err != nil {
			return nil, fmt.Errorf("listing sending domains: %w", err)
		}
		d, err := scanYDBSendingDomain(row)
		if err != nil {
			return nil, err
		}
		domains = append(domains, d)
	}
	return domains, nil
}

// PendingIDs lists the ids of the tenant's domains still awaiting
// verification.
func (r *YDBSendingDomains) PendingIDs(ctx context.Context, tenantID string) ([]string, error) {
	rs, err := r.driver.Query().QueryResultSet(ctx, `DECLARE $tenant_id AS Utf8;
SELECT id FROM sending_domains
WHERE tenant_id = $tenant_id AND status = "pending"u;`, ydbTenant(tenantID))
	if err != nil {
		return nil, fmt.Errorf("listing pending domains: %w", err)
	}
	defer func() { _ = rs.Close(ctx) }()
	var ids []string
	for row, err := range rs.Rows(ctx) {
		if err != nil {
			return nil, fmt.Errorf("listing pending domains: %w", err)
		}
		var id string
		if err := row.Scan(&id); err != nil {
			return nil, fmt.Errorf("scanning pending domain: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func ydbTenant(tenantID string) query.ExecuteOption {
	return query.WithParameters(ydb.ParamsBuilder().Param("$tenant_id").Text(tenantID).Build())
}

func ydbTenantAndID(tenantID, id string) query.ExecuteOption {
	return query.WithParameters(ydb.ParamsBuilder().
		Param("$tenant_id").Text(tenantID).
		Param("$id").Text(id).
		Build())
}

// ydbSendingDomainFromRow maps a single-row read to the domain, translating
// "no row" into domain.ErrDomainNotFound.
func ydbSendingDomainFromRow(row query.Row, err error) (*domain.SendingDomain, error) {
	if errors.Is(err, query.ErrNoRows) {
		return nil, domain.ErrDomainNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("loading sending domain: %w", err)
	}
	return scanYDBSendingDomain(row)
}

// scanYDBSendingDomain reads one row in ydbSendingDomainColumns order.
func scanYDBSendingDomain(row query.Row) (*domain.SendingDomain, error) {
	var id, tenantID, name, status, dkimJSON, spf, dmarc, identityRef, failureReason string
	var createdAt time.Time
	var verifiedAt, lastCheckedAt *time.Time
	if err := row.Scan(&id, &tenantID, &name, &status, &dkimJSON, &spf,
		&dmarc, &identityRef, &failureReason, &createdAt, &verifiedAt, &lastCheckedAt); err != nil {
		return nil, fmt.Errorf("scanning sending domain: %w", err)
	}
	var dkim []domain.DNSRecord
	if err := json.Unmarshal([]byte(dkimJSON), &dkim); err != nil {
		return nil, fmt.Errorf("decoding dkim records: %w", err)
	}
	return domain.HydrateSendingDomain(id, tenantID, name, domain.Status(status), dkim,
		spf, dmarc, identityRef, failureReason, createdAt, verifiedAt, lastCheckedAt), nil
}
```

- [ ] **Step 6: Run the contract suite to verify it passes**

Run: `export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock" && go test ./internal/sending/... -v -timeout 8m 2>&1 | grep -E '^(=== RUN|--- (PASS|FAIL)|PASS|FAIL|ok)' | grep -E 'FAIL|/pg|/ydb|^ok'`

Expected: every `TestSendingDomains*` shows `--- PASS` for both `/pg` and `/ydb`, and the package is `ok`. The verify-worker tests stay on pg and pass.

Three `/ydb`-only failures each point at one open spec item. Fix only within these rules, and record each case in the report:
- **`AddDuplicateConflicts`** gets an unmapped error. This means `IsYDBConflict` does not recognise the conflict (V4 "to verify"). Print the error with `%+v` in the report. Adjust `IsYDBConflict` only if the status or issue code differs from PRECONDITION_FAILED / 2012. Never fall back to matching the message text.
- **`GetMissing` or `CrossTenantIsolation`** fail on `Update` with a wrapped error. `DoTx` wraps the closure's `domain.ErrDomainNotFound` without `%w`, so `errors.Is` fails. Report it as BLOCKED with the error chain. The fix is a design decision about how adapters surface domain errors through `DoTx`.
- **`UpdatePropagatesClosureError`** finds the status changed. A failed closure was committed. Report it as BLOCKED.

- [ ] **Step 7: Extend the architecture guard**

In `Makefile`, in the `lint-arch` target, add `ydb-platform` to both forbidden-dependency patterns.

Replace:

```make
		| grep -E 'net/http|jackc/pgx|go-chi'
```

with:

```make
		| grep -E 'net/http|jackc/pgx|ydb-platform|go-chi'
```

Replace:

```make
		| grep -E 'jackc/pgx|nvelope/internal/[a-z]+/adapters'
```

with:

```make
		| grep -E 'jackc/pgx|ydb-platform|nvelope/internal/[a-z]+/adapters'
```

Run: `export PATH="$HOME/go/bin:$PATH" && make lint-arch`

Expected: `[arch] clean`.

- [ ] **Step 8: Full verification and commit**

Run: `go mod tidy && git diff --stat go.mod go.sum`

Expected: only `github.com/ydb-platform/ydb-go-genproto` moves from the indirect block to the direct block.

Run: `export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock" && go build ./... && go vet ./... && go test ./... -timeout 15m 2>&1 | grep -vE '^(ok|\?)\s'`

Expected: no output besides the build. In other words, no failing package.

Run: `golangci-lint run`

Expected: `0 issues.`

```bash
git add internal/db/ydb.go internal/sending/adapters/domains_ydb.go internal/sending/adapters/domains_test.go \
  internal/sending/adapters/helpers_test.go Makefile go.mod go.sum
git commit -m "feat(sending): add the YDB sending-domain repository under a pg+ydb contract suite"
```

---

### Task 3: Reset `snooze` at the top of the `VerifyWorker` closure (V6)

**Files:**
- Modify: `internal/sending/adapters/verify_worker.go:53-55`
- Test: `internal/sending/adapters/verify_worker_test.go`

**Interfaces:**
- Consumes: `domain.HydrateSendingDomain`, `adapters.NewVerifyWorker`, and the test helpers `fakeVerifier` and `verifyJob` already in `verify_worker_test.go`.
- Produces: no API change. `Work` no longer snoozes a domain that a retried `Update` run found terminal.

Background, from spec V6: `snooze = true` is only ever set, never reset. Under YDB, `Update` runs its closure inside `DoTx`, and `DoTx` may re-run it. A first run on a pending domain sets `snooze`. If a concurrent writer verifies the domain before the retry, the retry takes the `!d.IsPending()` early return, the stale `true` survives, and the worker snoozes a terminal domain. Postgres does not retry, so SaaS behaviour is unchanged.

- [ ] **Step 1: Write the failing test**

Append to `internal/sending/adapters/verify_worker_test.go`:

```go
// retryingDomains is a SendingDomainRepository whose Update runs fn twice, the
// way a retried YDB transaction does: first on a pending copy, then — after a
// concurrent writer verified the domain — on a verified copy. Only Get and
// Update are implemented; the embedded nil interface panics on anything else.
type retryingDomains struct {
	domain.SendingDomainRepository
	pending, verified *domain.SendingDomain
}

func (r retryingDomains) Get(context.Context, string, string) (*domain.SendingDomain, error) {
	return r.pending, nil
}

func (r retryingDomains) Update(_ context.Context, _, _ string,
	fn func(*domain.SendingDomain) (*domain.SendingDomain, error)) error {
	if _, err := fn(r.pending); err != nil {
		return err
	}
	_, err := fn(r.verified)
	return err
}

func TestVerifyWorkerRetriedUpdateDoesNotSnoozeTerminalDomain(t *testing.T) {
	t.Parallel()
	now := time.Now()
	pending := domain.HydrateSendingDomain("d1", "t1", "mail.acme.com", domain.StatusPending,
		nil, "", "", "identity", "", now, nil, nil)
	verified := domain.HydrateSendingDomain("d1", "t1", "mail.acme.com", domain.StatusVerified,
		nil, "", "", "identity", "", now, &now, &now)

	w := adapters.NewVerifyWorker(retryingDomains{pending: pending, verified: verified},
		fakeVerifier{verified: false}, time.Minute, 72*time.Hour)
	require.NoError(t, w.Work(context.Background(), verifyJob("t1", "d1")),
		"the last run of the retried transaction saw a verified domain, so the job must not snooze")
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./internal/sending/adapters/ -run TestVerifyWorkerRetriedUpdateDoesNotSnoozeTerminalDomain -v`

Expected: FAIL. The error is a `*rivertype.JobSnoozeError` (`river.JobSnooze(1m)`) where `nil` was expected.

The test needs no Docker, because it uses no database.

- [ ] **Step 3: Implement the reset**

In `internal/sending/adapters/verify_worker.go`, change the start of the `Update` closure from:

```go
		func(d *domain.SendingDomain) (*domain.SendingDomain, error) {
			if !d.IsPending() {
```

to:

```go
		func(d *domain.SendingDomain) (*domain.SendingDomain, error) {
			// The closure may run more than once (a retried YDB transaction), so
			// the outcome is decided by its last run only.
			snooze = false
			if !d.IsPending() {
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock" && go test ./internal/sending/adapters/ -run TestVerifyWorker -v -timeout 8m`

Expected: all `TestVerifyWorker*` tests PASS, including the four pg-backed ones and the new one.

- [ ] **Step 5: Commit**

```bash
git add internal/sending/adapters/verify_worker.go internal/sending/adapters/verify_worker_test.go
git commit -m "fix(sending): decide the verify snooze by the last run of a retried transaction"
```

---

### Task 4: `ydbthrottle` and the YDB campaign rate limiter

**Files:**
- Create: `internal/db/ydbmigrations/000002_throttle.up.sql`
- Create: `internal/platform/ydbthrottle/counter.go`
- Create: `internal/platform/ydbthrottle/counter_test.go`
- Create: `internal/campaign/adapters/ratelimiter_ydb.go`
- Create: `internal/campaign/adapters/ratelimiter_ydb_test.go`

**Interfaces:**
- Consumes: `dbtest.YDBMigrated(t)` (Task 1); `campaign/domain.RateLimiter` and `campaign/domain.Limit{Max int; Window time.Duration}` (`internal/campaign/domain/messenger.go`).
- Produces (Plan C wires these):
  ```go
  // package ydbthrottle
  type Window struct { Key string; Max int; Size time.Duration }
  func New(d *ydb.Driver) *Counter
  func (c *Counter) Allow(ctx context.Context, windows ...Window) (allowed bool, retryAfter time.Duration, err error)
  func (c *Counter) Keyed(prefix string, max int, size time.Duration) *Keyed
  func (k *Keyed) Allow(ctx context.Context, key string) (bool, error) // matches auth ResendThrottle, audience SubmissionThrottle
  // package adapters (internal/campaign/adapters)
  func NewYDBRateLimiter(counter *ydbthrottle.Counter, global domain.Limit) *YDBRateLimiter // implements domain.RateLimiter
  ```

- [ ] **Step 1: Write the migration**

Create `internal/db/ydbmigrations/000002_throttle.up.sql`:

```sql
-- Fixed-window throttle counters (internal/platform/ydbthrottle), the
-- serverless replacement for the Redis limiters. One row per key per window;
-- TTL deletes a row once its window has ended.
CREATE TABLE IF NOT EXISTS throttle_windows (
    throttle_key Utf8      NOT NULL,
    window_start Timestamp NOT NULL,
    hits         Uint64    NOT NULL,
    expire_at    Timestamp NOT NULL,
    PRIMARY KEY (throttle_key, window_start)
) WITH (TTL = Interval("PT0S") ON expire_at);
```

- [ ] **Step 2: Write the failing counter tests**

Create `internal/platform/ydbthrottle/counter_test.go`:

```go
package ydbthrottle

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
)

// newTestCounter returns a Counter on the test YDB whose clock reads *clock,
// so window arithmetic is deterministic.
func newTestCounter(t *testing.T, clock *time.Time) *Counter {
	t.Helper()
	c := New(dbtest.YDBMigrated(t))
	c.now = func() time.Time { return *clock }
	return c
}

// testKey returns a key unique to this test run; the table is shared.
func testKey(name string) string { return "test:" + uuid.NewString() + ":" + name }

func TestAllowAdmitsUpToMaxThenDenies(t *testing.T) {
	t.Parallel()
	clock := time.Date(2026, 11, 14, 12, 0, 30, 0, time.UTC)
	c := newTestCounter(t, &clock)
	w := Window{Key: testKey("max"), Max: 3, Size: time.Minute}
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		ok, _, err := c.Allow(ctx, w)
		require.NoError(t, err)
		require.True(t, ok, "hit %d is within the limit", i+1)
	}
	ok, retryAfter, err := c.Allow(ctx, w)
	require.NoError(t, err)
	require.False(t, ok, "the fourth hit exceeds Max")
	require.Equal(t, 30*time.Second, retryAfter, "the window [12:00, 12:01) ends 30s after 12:00:30")
}

func TestAllowRollsOverToTheNextWindow(t *testing.T) {
	t.Parallel()
	clock := time.Date(2026, 11, 14, 12, 0, 59, 0, time.UTC)
	c := newTestCounter(t, &clock)
	w := Window{Key: testKey("rollover"), Max: 1, Size: time.Minute}
	ctx := context.Background()

	ok, _, err := c.Allow(ctx, w)
	require.NoError(t, err)
	require.True(t, ok)
	ok, _, err = c.Allow(ctx, w)
	require.NoError(t, err)
	require.False(t, ok)

	clock = time.Date(2026, 11, 14, 12, 1, 0, 0, time.UTC)
	ok, _, err = c.Allow(ctx, w)
	require.NoError(t, err)
	require.True(t, ok, "a new window starts with a fresh budget")
}

func TestAllowCountsOnlyWhenEveryWindowHasRoom(t *testing.T) {
	t.Parallel()
	clock := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	c := newTestCounter(t, &clock)
	tight := Window{Key: testKey("tight"), Max: 1, Size: time.Minute}
	loose := Window{Key: testKey("loose"), Max: 5, Size: time.Minute}
	ctx := context.Background()

	ok, _, err := c.Allow(ctx, tight, loose)
	require.NoError(t, err)
	require.True(t, ok)
	ok, retryAfter, err := c.Allow(ctx, tight, loose)
	require.NoError(t, err)
	require.False(t, ok, "the tight window is full")
	require.Equal(t, time.Minute, retryAfter)

	// The denied hit did not count against the loose window: it still has
	// exactly 4 of its 5 hits left.
	for i := 0; i < 4; i++ {
		ok, _, err := c.Allow(ctx, loose)
		require.NoError(t, err)
		require.True(t, ok, "loose hit %d", i+2)
	}
	ok, _, err = c.Allow(ctx, loose)
	require.NoError(t, err)
	require.False(t, ok)
}

func TestAllowRejectsInvalidWindows(t *testing.T) {
	t.Parallel()
	clock := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	c := newTestCounter(t, &clock)
	ctx := context.Background()

	_, _, err := c.Allow(ctx)
	require.Error(t, err, "at least one window is required")
	_, _, err = c.Allow(ctx, Window{Key: testKey("zero"), Max: 0, Size: time.Minute})
	require.Error(t, err)
	_, _, err = c.Allow(ctx, Window{Key: testKey("nosize"), Max: 1})
	require.Error(t, err)
	_, _, err = c.Allow(ctx, Window{Max: 1, Size: time.Minute})
	require.Error(t, err)
}

func TestKeyedSeparatesKeys(t *testing.T) {
	t.Parallel()
	clock := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	k := newTestCounter(t, &clock).Keyed(testKey("resend")+":", 1, time.Hour)
	ctx := context.Background()

	ok, err := k.Allow(ctx, "user-a")
	require.NoError(t, err)
	require.True(t, ok)
	ok, err = k.Allow(ctx, "user-a")
	require.NoError(t, err)
	require.False(t, ok, "user-a used its one hit this hour")
	ok, err = k.Allow(ctx, "user-b")
	require.NoError(t, err)
	require.True(t, ok, "another key has its own window")
}

// Keyed matches the single-key throttle ports (auth ResendThrottle, audience
// SubmissionThrottle) structurally; Plan C's wiring relies on it.
var _ interface {
	Allow(context.Context, string) (bool, error)
} = (*Keyed)(nil)
```

- [ ] **Step 3: Run them to verify they fail**

Run: `go vet ./internal/platform/ydbthrottle/`

Expected: build failure. The package has no non-test Go files, so `New`, `Window` and `Keyed` are undefined.

- [ ] **Step 4: Implement the counter**

Create `internal/platform/ydbthrottle/counter.go`:

```go
// Package ydbthrottle is a fixed-window rate counter on YDB for the serverless
// runtime, where Redis is unavailable. It backs the campaign send limiter and
// the single-key anti-abuse throttles.
package ydbthrottle

import (
	"context"
	"errors"
	"fmt"
	"time"

	ydb "github.com/ydb-platform/ydb-go-sdk/v3"
	"github.com/ydb-platform/ydb-go-sdk/v3/query"
)

// Window is one fixed-window allowance: at most Max hits on Key per Size.
type Window struct {
	Key  string
	Max  int
	Size time.Duration
}

// Counter counts hits in fixed windows on the throttle_windows table
// (internal/db/ydbmigrations/000002_throttle.up.sql). It is safe for
// concurrent use.
//
// ponytail: fixed windows admit up to 2x Max across a window boundary, and
// every hit on a shared key (the global send window) writes the same row, so
// heavy concurrency turns into transaction retries. Move to a sliding log or
// sharded counters if either matters.
type Counter struct {
	db  *ydb.Driver
	now func() time.Time
}

// New builds a Counter over the driver.
func New(d *ydb.Driver) *Counter {
	return &Counter{db: d, now: time.Now}
}

const selectHits = `DECLARE $throttle_key AS Utf8;
DECLARE $window_start AS Timestamp;
SELECT hits FROM throttle_windows
WHERE throttle_key = $throttle_key AND window_start = $window_start;`

const upsertHits = `DECLARE $throttle_key AS Utf8;
DECLARE $window_start AS Timestamp;
DECLARE $hits AS Uint64;
DECLARE $expire_at AS Timestamp;
UPSERT INTO throttle_windows (throttle_key, window_start, hits, expire_at)
VALUES ($throttle_key, $window_start, $hits, $expire_at);`

// Allow admits one hit only when every window has room, and only then counts
// it in all of them, so a denial never spends one window's budget while
// another is full. When denied, retryAfter is the longest wait until a full
// window rolls over. It runs as one serializable transaction, which the SDK
// retries on conflict.
func (c *Counter) Allow(ctx context.Context, windows ...Window) (allowed bool, retryAfter time.Duration, err error) {
	if len(windows) == 0 {
		return false, 0, errors.New("ydbthrottle: at least one window is required")
	}
	for _, w := range windows {
		if w.Key == "" || w.Max <= 0 || w.Size <= 0 {
			return false, 0, fmt.Errorf("ydbthrottle: invalid window %+v: key, max and size are required", w)
		}
	}
	now := c.now().UTC()
	err = c.db.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		// The closure may run more than once (a retried transaction), so the
		// outcome is recomputed from scratch on every run.
		allowed, retryAfter = true, 0
		hits := make([]uint64, len(windows))
		for i, w := range windows {
			start := now.Truncate(w.Size)
			n, err := readHits(ctx, tx, w.Key, start)
			if err != nil {
				return err
			}
			hits[i] = n
			if n >= uint64(w.Max) {
				allowed = false
				if wait := start.Add(w.Size).Sub(now); wait > retryAfter {
					retryAfter = wait
				}
			}
		}
		if !allowed {
			return nil
		}
		for i, w := range windows {
			start := now.Truncate(w.Size)
			if err := tx.Exec(ctx, upsertHits, query.WithParameters(ydb.ParamsBuilder().
				Param("$throttle_key").Text(w.Key).
				Param("$window_start").Timestamp(start).
				Param("$hits").Uint64(hits[i]+1).
				Param("$expire_at").Timestamp(start.Add(w.Size)).
				Build())); err != nil {
				return fmt.Errorf("ydbthrottle: counting a hit on %q: %w", w.Key, err)
			}
		}
		return nil
	})
	if err != nil {
		return false, 0, err
	}
	return allowed, retryAfter, nil
}

// readHits returns the hits recorded for key in the window starting at start,
// or 0 when the window has no row yet.
func readHits(ctx context.Context, tx query.TxActor, key string, start time.Time) (uint64, error) {
	row, err := tx.QueryRow(ctx, selectHits, query.WithParameters(ydb.ParamsBuilder().
		Param("$throttle_key").Text(key).
		Param("$window_start").Timestamp(start).
		Build()))
	if errors.Is(err, query.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("ydbthrottle: reading %q: %w", key, err)
	}
	var n uint64
	if err := row.Scan(&n); err != nil {
		return 0, fmt.Errorf("ydbthrottle: scanning %q: %w", key, err)
	}
	return n, nil
}

// Keyed is a single-window throttle per key, namespaced by a prefix. Its
// Allow method matches the auth ResendThrottle and audience SubmissionThrottle
// ports.
type Keyed struct {
	counter *Counter
	prefix  string
	max     int
	size    time.Duration
}

// Keyed builds a per-key throttle admitting max hits per size window.
func (c *Counter) Keyed(prefix string, max int, size time.Duration) *Keyed {
	return &Keyed{counter: c, prefix: prefix, max: max, size: size}
}

// Allow reports whether one more hit on key fits its window, counting it if so.
func (k *Keyed) Allow(ctx context.Context, key string) (bool, error) {
	ok, _, err := k.counter.Allow(ctx, Window{Key: k.prefix + key, Max: k.max, Size: k.size})
	return ok, err
}
```

- [ ] **Step 5: Run the counter tests to verify they pass**

Run: `export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock" && go test ./internal/platform/ydbthrottle/ -v -timeout 8m`

Expected: all five tests PASS.

- [ ] **Step 6: Write the failing rate-limiter test**

Create `internal/campaign/adapters/ratelimiter_ydb_test.go`:

```go
package adapters_test

import (
	"context"
	"math/rand/v2"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/campaign/adapters"
	"github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
	"github.com/nikolaymatrosov/nvelope/internal/platform/ydbthrottle"
)

func TestYDBRateLimiterGlobalWindowSpansTenants(t *testing.T) {
	t.Parallel()
	// A run-unique window size gives this test its own "rl:global" row: rows
	// are keyed by (throttle_key, window_start), and window_start is now
	// truncated to the size.
	size := time.Hour + time.Duration(rand.IntN(1_000_000))*time.Millisecond
	limiter := adapters.NewYDBRateLimiter(ydbthrottle.New(dbtest.YDBMigrated(t)),
		domain.Limit{Max: 2, Window: size})
	perTenant := domain.Limit{Max: 10, Window: size}
	ctx := context.Background()
	tenantA, tenantB := uuid.NewString(), uuid.NewString()

	ok, _, err := limiter.Allow(ctx, tenantA, perTenant)
	require.NoError(t, err)
	require.True(t, ok)
	ok, _, err = limiter.Allow(ctx, tenantB, perTenant)
	require.NoError(t, err)
	require.True(t, ok)

	ok, retryAfter, err := limiter.Allow(ctx, tenantB, perTenant)
	require.NoError(t, err)
	require.False(t, ok, "the global window is full even though tenant B's own window has room")
	require.Positive(t, retryAfter)
	require.LessOrEqual(t, retryAfter, size)
}
```

Run: `go vet ./internal/campaign/adapters/`

Expected: build failure `undefined: adapters.NewYDBRateLimiter`.

- [ ] **Step 7: Implement the adapter**

Create `internal/campaign/adapters/ratelimiter_ydb.go`:

```go
package adapters

import (
	"context"
	"time"

	"github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
	"github.com/nikolaymatrosov/nvelope/internal/platform/ydbthrottle"
)

// YDBRateLimiter adapts the YDB fixed-window counter to the campaign context's
// RateLimiter port, for the serverless runtime: one send must fit both the
// tenant's window and the platform-wide window.
type YDBRateLimiter struct {
	counter *ydbthrottle.Counter
	global  domain.Limit
}

var _ domain.RateLimiter = (*YDBRateLimiter)(nil)

// NewYDBRateLimiter builds a YDBRateLimiter with the platform-wide cap.
func NewYDBRateLimiter(counter *ydbthrottle.Counter, global domain.Limit) *YDBRateLimiter {
	if counter == nil {
		panic("nil throttle counter")
	}
	return &YDBRateLimiter{counter: counter, global: global}
}

// Allow checks the per-tenant and global windows together for one send.
func (r *YDBRateLimiter) Allow(ctx context.Context, tenantID string,
	perTenant domain.Limit) (bool, time.Duration, error) {
	return r.counter.Allow(ctx,
		ydbthrottle.Window{Key: "rl:tenant:" + tenantID, Max: perTenant.Max, Size: perTenant.Window},
		ydbthrottle.Window{Key: "rl:global", Max: r.global.Max, Size: r.global.Window})
}
```

- [ ] **Step 8: Run the tests to verify they pass**

Run: `export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock" && go test ./internal/campaign/adapters/ -run TestYDBRateLimiter -v -timeout 8m`

Expected: PASS.

- [ ] **Step 9: Full verification and commit**

Run: `export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock" && go build ./... && go vet ./... && go test ./... -timeout 15m 2>&1 | grep -vE '^(ok|\?)\s'`

Expected: no failing package.

Run: `golangci-lint run && export PATH="$HOME/go/bin:$PATH" && make lint-arch`

Expected: `0 issues.` and `[arch] clean`.

```bash
git add internal/db/ydbmigrations/000002_throttle.up.sql internal/platform/ydbthrottle/ \
  internal/campaign/adapters/ratelimiter_ydb.go internal/campaign/adapters/ratelimiter_ydb_test.go
git commit -m "feat(ydbthrottle): add the YDB fixed-window counter and the campaign rate limiter"
```

---

## Done criteria for Plan B1

- `go build ./... && go vet ./... && go test ./...` passes, with every `TestSendingDomains*` passing on both `/pg` and `/ydb`.
- `golangci-lint run` reports `0 issues.` and `make lint-arch` reports `[arch] clean`, now also forbidding `ydb-platform` in domain and transport packages.
- `db.MigrateYDB` is proven idempotent and convergent after a partial failure (`TestMigrateYDBConvergesAfterPartialFailure`). This closes the V4 open item.
- `db.IsYDBConflict` is proven against a real duplicate (`TestSendingDomainsAddDuplicateConflicts/ydb`). This closes the V4 matcher item.
- No `go.mod` change except `ydb-go-genproto` becoming direct.
- The spec's open "to verify in Plan B" items that B1 did **not** close are listed in the final report, and stay with the context plans that own them:
  - `ILIKE` with a Utf8 column, a parameter and Cyrillic data: audience and deliverability;
  - the segment-translator JSON functions: audience;
  - the INSERT-conflict transaction-break behaviour for `BulkInsert`: campaign.
