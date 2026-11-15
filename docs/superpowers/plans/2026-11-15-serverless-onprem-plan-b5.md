# Serverless On-Prem — Plan B5 (audience core on YDB) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:**
- Port the core of the `audience` context to YDB: `YDBLists`, `YDBSubscribers` (with a YQL segment translator), `YDBMemberships` and `YDBFields` behind the existing domain ports.
- Hold every new implementation to the same contract tests as its Postgres counterpart.

**Architecture:**
- **Migration.** One file, `internal/db/ydbmigrations/000006_audience.up.sql`, creates the four core audience tables: `lists`, `subscribers`, `subscriber_lists` and `subscriber_fields`. Every one is tenant-scoped.
- **Adapters.** The YDB adapters sit next to the pg adapters in `internal/audience/adapters` and call `db.Query()` / `DoTx` directly, like `internal/iam/adapters/*_ydb.go`.
- **Segments.** `segment_yql.go` mirrors `segment_sql.go`: it turns a validated `domain.Segment` into a YQL predicate over `subscribers AS s`, with every operand bound as a `Utf8` parameter.
- **Contract tests.** The `*_pg_test.go` files for these four repositories, and the preferences suite that drives them through the app layer, become pg+ydb contract suites through an `eachBackend` harness, the same shape as iam's.
  - No audience query reads a table another context owns. The YDB backend therefore uses a bare UUID for tenant ids, as iam's `newTenant` does.
  - The pg backend keeps its raw-SQL tenant seeder, because its foreign keys need it.
- **No foreign keys.**
  - `YDBMemberships.Attach` reads the subscriber and the list inside its `DoTx` and returns `ErrSubscriberNotFound` or `ErrListNotFound`.
  - `YDBSubscribers.Delete` and `YDBLists.Delete` delete the rows Postgres cascades to, in the same `DoTx`. For a list, that includes the iam `user_list_roles` rows (the B4 handover note).
- **No arrays.** `lists.tags` (`text[]`) becomes a JSON array in a `Utf8` column.

**Tech Stack:**
- Go 1.26
- `github.com/ydb-platform/ydb-go-sdk/v3` v3.136.4 (query service)
- testcontainers-go, local-ydb `26.3.1.23`
- testify, `github.com/google/uuid`

**Design spec:** [docs/superpowers/specs/2026-11-14-serverless-onprem-design.md](../specs/2026-11-14-serverless-onprem-design.md).
- Binding sections:
  - "Storage layer (YDB)": Transactions; Schema and migrations; SQL translation rules, including the `jsonb`, `text[]`, `citext`, foreign-key, segment-translator and `ILIKE` rows this plan updates.
  - "Spike findings (Plan A)": V4, V5 and V6.
  - "Plan B2 planning spike" to "Plan B5 planning spike".
  - "Ceilings".
- The B5 spike verified every YQL construct this plan uses that B1–B4 did not:
  - a `JsonDocument` column, bound from bytes and scanned into `string`;
  - `JSON_VALUE` and `JSON_EXISTS` with the attribute key bound through `$.keyvalue() ? (@.name == $k)`, and `CAST(JSON_VALUE(...) AS Double)` for numeric comparison;
  - `ILIKE` on `Utf8` columns with a bound pattern, against Cyrillic data;
  - `s.id IN (SELECT ... FROM subscriber_lists VIEW ...)` inside `OR`, `COUNT(*)` and a page;
  - a `JOIN` from a `GLOBAL SYNC` index ordered by a right-side column;
  - `DELETE ... ON SELECT ... VIEW` inside `DoTx`.
- **Dry run.** During planning, every code block in this plan was applied task by task to the worktree, then reverted.
  - After each task, `go vet`, `golangci-lint run ./internal/audience/...` and that task's pg+ydb suites passed.
  - `TestSubscribersUpsertByEmailIsRaceFree` and `TestSubscribersRunSegmentOperators` passed ten repeated runs (`-count=10`).
  - On the final state:
    - `go build ./... && go test ./...` had no failing package;
    - `golangci-lint run` reported `0 issues.`;
    - `make lint-arch` reported `[arch] clean`.

**Scope:**
- This is the fifth Plan B plan. The port order is auth, tenant, iam, audience, campaign, deliverability, media. Audience takes two plans; this one is the core. Plan B6 ports `import_export_jobs`, `subscription_pages` and `pending_subscriptions`.
- Not in scope:
  - `jobs_pg.go`, `subscription_pages_pg.go` and `pending_subscriptions_pg.go`, and their suites `jobs_pg_test.go` and `optin_pg_test.go`. Those suites stay pg-only and must keep passing. They use the helpers this plan rewrites (`addList`, `newSubscriber`, `seedTenant`), so the rewritten helpers keep accepting the pg repositories.
  - The workers (`import_worker.go`, `export_worker.go`, `optin_worker.go`). They depend only on the ports.
  - The submission throttle. `ydbthrottle.Keyed` already matches `domain.SubmissionThrottle`, and Plan C wires it.
  - Wiring the YDB adapters into the composition root (`internal/service/application.go:446` and `:682-688`). That is Plan C.
  - The audience app and domain layers, which are unchanged.
  - The pg adapters. They are unchanged except for the package doc comment in `lists_pg.go`.

## Global Constraints

- **Branch.**
  - Work on `feat/serverless-onprem` in the worktree `/Users/nikthespirit/experiments/nvelope-wt/serverless-onprem`. Both already exist, and the branch starts at `main` (`779a558`) plus the commit that adds this plan.
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
  - `internal/api` `TestPublicSubscriptionCrossTenantIsolation` can fail on a reused pg database (a fixed `hash-a` token). Re-run once.
  - A YDB-backed test can fail with `No such container: <id>`, or hang until the timeout, when it starts while Ryuk is still reaping the previous run's YDB container. Re-run once.
- **Dependencies.** No new Go modules and no `go.mod` change.
- **SaaS must not change.** Postgres adapters, River, Redis and every existing test keep their behaviour. `go test ./...` passes after every task.
- **YQL conventions** (spec "SQL translation rules" and V4):
  - Every query `DECLARE`s each parameter: `DECLARE $id AS Utf8;`.
  - A `JsonDocument` parameter is declared `DECLARE $attributes AS JsonDocument;` and bound with `JSONDocumentFromBytes`. A `JsonDocument` column is scanned into a `string`.
  - Literals that target a `Utf8` column take the `u` suffix (`"unsubscribed"u`); a bare `"..."` is `String`.
  - Every DDL statement is idempotent (`CREATE TABLE IF NOT EXISTS`).
  - A statement ends on a line ending in `;`. Never end a line with `;` inside a string literal or a `--` comment.
  - A secondary index is read only through an explicit `VIEW <index_name>`.
  - In a `JOIN`, every column is qualified with its table alias (`s.created_at`, `sl.list_id`). Every subscribers query aliases the table as `s`, so one projection (`ydbSubscriberColumns`) serves them all.
- **Tenant scoping.** All four tables are tenant-scoped. Their primary key starts with `tenant_id`, and every query filters by the `tenantID` the port receives, subqueries included.
- **IDs** are UUIDv4 strings generated in Go (`uuid.NewString()`) before any `DoTx`, and stored as `Utf8`.
- **No column defaults.** The YDB columns have no `DEFAULT`. Every `INSERT` writes every `NOT NULL` column from the domain value. Only `created_at` and `updated_at` come from `CurrentUtcTimestamp()`.
- **Case-insensitive matching** (Postgres `citext` and `ILIKE`):
  - `subscribers.email` has **no** `email_key` column. `domain.NewSubscriber` lower-cases the email, and no other code writes the column, so the unique index on `(tenant_id, email)` reproduces `citext`.
  - The segment translator lower-cases an `email` operand in Go before comparing it.
  - `Search` and every `contains` condition use YQL `ILIKE`, which the B5 spike verified against Cyrillic data.
  - List names and field slugs are case-sensitive, as the Postgres `text` columns are.
- **Tags** (`lists.tags`) are a JSON array in a `Utf8` column, written by `ydbStrings` and read by `ydbStringsFrom`. An empty or `nil` slice is stored as `[]`.
- **Foreign keys.** YDB has none.
  - `YDBMemberships.Attach` reads the subscriber and the list in the same `DoTx` and returns `domain.ErrSubscriberNotFound` or `domain.ErrListNotFound` when either is missing.
  - `YDBSubscribers.Delete` deletes the subscriber's `subscriber_lists` rows in the same `DoTx`.
  - `YDBLists.Delete` deletes the list's `subscriber_lists` rows and its iam `user_list_roles` rows in the same `DoTx`.
  - The campaign tables Postgres also cascades into (`campaign_recipients`, `campaign_lists`) are not on YDB yet. They are left to the campaign plan.
- **`DoTx` closures must be retry-safe** (V6):
  - only database operations inside, plus the port's `fn` callbacks;
  - ids, encoded attributes and domain values are built before `DoTx`;
  - values the caller reads are assigned at the end of the closure;
  - never `append` to a slice, or write to a map, declared outside the closure.
- **Duplicate key mapping.**
  - "Already exists" comes from `db.IsYDBConflict(err)`. Never match on the message text.
  - Inside a `DoTx`, check it on the statement and also on the error `DoTx` returns, for a YDB that reports the conflict at commit.
  - The mappings are the Postgres ones: list name → `ErrListNameTaken`, subscriber email → `ErrSubscriberEmailTaken`, field slug → `ErrFieldSlugTaken`, membership key → `ErrMembershipExists`.
  - Never add `query.WithIdempotent()` to an insert path.
- **Commits.** Use conventional messages. **No `Co-Authored-By` trailer** of any kind.
- **Tooling.**
  - `goimports` is not installed, so use `gofmt -w`.
  - `golangci-lint` is on PATH. It lints test files too.
  - Run arch lint as `PATH="$HOME/go/bin:$PATH" make lint-arch`, with PATH set inline. A worktree guard refuses `export PATH` combined with git commands, and it refuses complex compound git commands. Run git commands one per line, as the commit steps show.

## File structure

| File | Responsibility |
|------|----------------|
| `internal/db/ydbmigrations/000006_audience.up.sql` | the four core audience tables |
| `internal/audience/adapters/lists_ydb.go` | `YDBLists`; the shared helpers `ydbTenantParam`, `ydbTenantAndID`, `ydbTenantAndPage`, `ydbCount`, `ydbStrings`, `ydbStringsFrom` |
| `internal/audience/adapters/fields_ydb.go` | `YDBFields` |
| `internal/audience/adapters/segment_yql.go` | `translateSegmentYQL`, `ydbSegmentDeclares` |
| `internal/audience/adapters/subscribers_ydb.go` | `YDBSubscribers`, `ydbSegmentParams` |
| `internal/audience/adapters/memberships_ydb.go` | `YDBMemberships`, `ydbRowExists`, `ydbMembershipKey` |
| `internal/audience/adapters/lists_pg.go` | package doc mentions the YDB implementations |
| `internal/audience/adapters/helpers_test.go` (rewritten in each task) | `backend`, `backends`, `eachBackend`, `seedTenant`, `timeZeroAdapters` |
| `internal/audience/adapters/lists_test.go` (renamed from `lists_pg_test.go`) | lists contract suite |
| `internal/audience/adapters/fields_test.go` (renamed from `fields_postgres_test.go`) | fields contract suite |
| `internal/audience/adapters/subscribers_test.go` (renamed from `subscribers_pg_test.go`) | subscribers contract suite, including the segment operator matrix |
| `internal/audience/adapters/memberships_test.go` (renamed from `memberships_pg_test.go`) | memberships contract suite, including the list-delete cascade |
| `internal/audience/adapters/preferences_test.go` (renamed from `preferences_pg_test.go`) | preferences and unsubscribe flows on both backends |
| `internal/audience/adapters/lists_ydb_test.go` (new) | YDB-only: a list delete removes its iam per-list roles |

---

### Task 1: The audience schema, `YDBLists`, `YDBFields` and their contract suites

**Files:**
- Create: `internal/db/ydbmigrations/000006_audience.up.sql`
- Create: `internal/audience/adapters/lists_ydb.go`
- Create: `internal/audience/adapters/fields_ydb.go`
- Modify: `internal/audience/adapters/lists_pg.go:1-3` (package doc only)
- Modify: `internal/audience/adapters/helpers_test.go` (rewritten)
- Rename: `internal/audience/adapters/lists_pg_test.go` → `internal/audience/adapters/lists_test.go` (rewritten)
- Rename: `internal/audience/adapters/fields_postgres_test.go` → `internal/audience/adapters/fields_test.go` (rewritten)

**Interfaces:**
- Consumes:
  - `dbtest.YDBMigrated(t) *ydb.Driver` (`internal/dbtest/ydb.go`). It applies every `internal/db/ydbmigrations/*.up.sql` once per test binary, so the new file is picked up automatically.
  - `dbtest.AppPool(t) *pgxpool.Pool` and `dbtest.RandString() string` (`internal/dbtest`).
  - `db.IsYDBConflict(err error) bool` (`internal/db/ydb.go`).
  - The ports `domain.ListRepository` and `domain.FieldRepository` (`internal/audience/domain/repository.go`), unchanged.
  - `domain.HydrateList`, `domain.HydrateField` and `domain.IsBuiltinFieldSlug` (`internal/audience/domain`), unchanged.
  - The iam table `user_list_roles (tenant_id, user_id, list_id, role_id)`, created by `000005_iam.up.sql`.
- Produces:
  ```go
  // package adapters (internal/audience/adapters)
  type YDBLists struct{ /* unexported */ }
  func NewYDBLists(d *ydb.Driver) *YDBLists
  type YDBFields struct{ /* unexported */ }
  func NewYDBFields(d *ydb.Driver) *YDBFields
  func ydbTenantParam(tenantID string) query.ExecuteOption                       // binds $tenant_id
  func ydbTenantAndID(tenantID, id string) query.ExecuteOption                   // binds $tenant_id, $id
  func ydbTenantAndPage(tenantID string, page domain.Page) query.ExecuteOption   // binds $tenant_id, $limit, $offset
  func ydbCount(ctx context.Context, tx query.TxActor, q string, opt query.ExecuteOption) (int, error)
  func ydbStrings(s []string) string
  func ydbStringsFrom(raw string) ([]string, error)
  // package adapters_test
  type backend struct {
      name      string
      lists     func(t *testing.T) domain.ListRepository
      fields    func(t *testing.T) domain.FieldRepository
      newTenant func(t *testing.T) string
  }
  func eachBackend(t *testing.T, body func(t *testing.T, b backend))
  func addList(t *testing.T, repo domain.ListRepository, tenantID, name string) string
  func newList(t *testing.T, tenantID, name string) *domain.List
  func seedTenant(t *testing.T, pool *pgxpool.Pool) string   // kept; the pg backend and the pg-only suites use it
  ```
- Task 2 relies on:
  - the `subscribers` and `subscriber_lists` tables and their indexes `idx_subscribers_email` and `idx_subscriber_lists_list`;
  - every helper above;
  - `YDBLists.Delete`, which already deletes the list's `subscriber_lists` and `user_list_roles` rows. Task 2 adds the tests that observe it.

- [ ] **Step 1: Write the audience migration**

Create `internal/db/ydbmigrations/000006_audience.up.sql`:

```sql
-- Audience core (internal/audience): lists, subscribers, list memberships and
-- the subscriber custom-field registry. Every table is tenant-scoped: its
-- primary key starts with tenant_id and every query filters by it. No column
-- has a DEFAULT, so the adapters write the values Postgres takes from its
-- column defaults. YDB has no foreign keys: the adapters check a membership's
-- subscriber and list, and delete the rows Postgres would cascade.

-- tags is a JSON array of strings, only stored and read back whole, so it is a
-- plain Utf8 column (Postgres: text[]). List names are case-sensitive, as the
-- Postgres text column is.
CREATE TABLE IF NOT EXISTS lists (
    tenant_id   Utf8      NOT NULL,
    id          Utf8      NOT NULL,
    name        Utf8      NOT NULL,
    description Utf8      NOT NULL,
    visibility  Utf8      NOT NULL,
    optin       Utf8      NOT NULL,
    tags        Utf8      NOT NULL,
    created_at  Timestamp NOT NULL,
    updated_at  Timestamp NOT NULL,
    PRIMARY KEY (tenant_id, id),
    INDEX idx_lists_tenant_name GLOBAL UNIQUE SYNC ON (tenant_id, name)
);

-- email is stored lower-cased (domain.NewSubscriber lower-cases it), so the
-- unique index reproduces the uniqueness of the Postgres citext column.
-- attributes is a JsonDocument because segment queries read into it with
-- JSON_VALUE and JSON_EXISTS.
CREATE TABLE IF NOT EXISTS subscribers (
    tenant_id  Utf8         NOT NULL,
    id         Utf8         NOT NULL,
    email      Utf8         NOT NULL,
    name       Utf8         NOT NULL,
    state      Utf8         NOT NULL,
    attributes JsonDocument NOT NULL,
    created_at Timestamp    NOT NULL,
    updated_at Timestamp    NOT NULL,
    PRIMARY KEY (tenant_id, id),
    INDEX idx_subscribers_email GLOBAL UNIQUE SYNC ON (tenant_id, email)
);

-- Keyed by subscriber, so a subscriber's memberships are a key-prefix read.
-- The list index serves InList, list-membership segment conditions and the
-- cascade when a list is deleted.
CREATE TABLE IF NOT EXISTS subscriber_lists (
    tenant_id           Utf8      NOT NULL,
    subscriber_id       Utf8      NOT NULL,
    list_id             Utf8      NOT NULL,
    subscription_status Utf8      NOT NULL,
    created_at          Timestamp NOT NULL,
    updated_at          Timestamp NOT NULL,
    PRIMARY KEY (tenant_id, subscriber_id, list_id),
    INDEX idx_subscriber_lists_list GLOBAL SYNC ON (tenant_id, list_id)
);

CREATE TABLE IF NOT EXISTS subscriber_fields (
    tenant_id     Utf8      NOT NULL,
    id            Utf8      NOT NULL,
    slug          Utf8      NOT NULL,
    display_name  Utf8      NOT NULL,
    type          Utf8      NOT NULL,
    default_value Utf8      NOT NULL,
    position      Int64     NOT NULL,
    created_at    Timestamp NOT NULL,
    updated_at    Timestamp NOT NULL,
    PRIMARY KEY (tenant_id, id),
    INDEX idx_subscriber_fields_slug GLOBAL UNIQUE SYNC ON (tenant_id, slug)
);
```

- [ ] **Step 2: Rewrite the harness**

Replace `internal/audience/adapters/helpers_test.go` with:

```go
package adapters_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/audience/adapters"
	"github.com/nikolaymatrosov/nvelope/internal/audience/domain"
	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
)

// timeZeroAdapters is a fixed timestamp for hydrating entities in tests.
func timeZeroAdapters() time.Time { return time.Unix(0, 0).UTC() }

// backend is one storage implementation of the audience repositories under
// contract test. Every contract test body runs once per backend, so the
// Postgres and YDB adapters are held to the same behaviour.
type backend struct {
	name   string
	lists  func(t *testing.T) domain.ListRepository
	fields func(t *testing.T) domain.FieldRepository
	// newTenant returns the id of a tenant row, which the tenant context
	// owns. On Postgres the audience tables carry a foreign key to it, so the
	// pg backend inserts a real row. YDB has no foreign keys and no audience
	// query reads the tenants table, so a bare UUID stands in.
	newTenant func(t *testing.T) string
}

var backends = []backend{
	{
		name: "pg",
		lists: func(t *testing.T) domain.ListRepository {
			return adapters.NewLists(dbtest.AppPool(t))
		},
		fields: func(t *testing.T) domain.FieldRepository {
			return adapters.NewFields(dbtest.AppPool(t))
		},
		newTenant: func(t *testing.T) string { return seedTenant(t, dbtest.AppPool(t)) },
	},
	{
		name: "ydb",
		lists: func(t *testing.T) domain.ListRepository {
			return adapters.NewYDBLists(dbtest.YDBMigrated(t))
		},
		fields: func(t *testing.T) domain.FieldRepository {
			return adapters.NewYDBFields(dbtest.YDBMigrated(t))
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

// seedTenant inserts a tenant row directly and returns its id. The audience
// adapters need a real tenant to satisfy the tenant_id foreign key and the RLS
// policy. The row is inserted with raw SQL rather than through the tenant
// context's adapter so the audience context's tests import no other context.
func seedTenant(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var id string
	require.NoError(t, pool.QueryRow(context.Background(),
		`INSERT INTO tenants (name, slug, status) VALUES ('Workspace', $1, 'active') RETURNING id`,
		"aud-"+dbtest.RandString()).Scan(&id))
	return id
}
```

- [ ] **Step 3: Rewrite the lists tests as a contract suite**

Run:

```bash
git mv internal/audience/adapters/lists_pg_test.go internal/audience/adapters/lists_test.go
```

Replace `internal/audience/adapters/lists_test.go` with the code below. `addList` now takes the port, so the pg-only suites that pass `adapters.NewLists(pool)` (`memberships_pg_test.go`, `preferences_pg_test.go`, `optin_pg_test.go`) still compile.

```go
package adapters_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/audience/domain"
)

func newList(t *testing.T, tenantID, name string) *domain.List {
	t.Helper()
	l, err := domain.NewList(tenantID, name, "desc", domain.VisibilityPrivate, domain.OptInSingle, []string{"x"})
	require.NoError(t, err)
	return l
}

// addList persists a list and returns its id.
func addList(t *testing.T, repo domain.ListRepository, tenantID, name string) string {
	t.Helper()
	id, err := repo.Add(context.Background(), tenantID, newList(t, tenantID, name))
	require.NoError(t, err)
	return id
}

func TestListsAddGetAll(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.lists(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		addList(t, repo, tenantID, "Newsletter")
		addList(t, repo, tenantID, "Announcements")

		lists, total, err := repo.All(ctx, tenantID, domain.DefaultPage)
		require.NoError(t, err)
		require.Equal(t, 2, total)
		require.Len(t, lists, 2)
		require.Equal(t, "Announcements", lists[0].Name(), "lists are ordered by name")

		got, err := repo.Get(ctx, tenantID, lists[0].ID())
		require.NoError(t, err)
		require.Equal(t, "Announcements", got.Name())
		require.Equal(t, tenantID, got.TenantID())
		require.Equal(t, "desc", got.Description())
		require.Equal(t, domain.VisibilityPrivate, got.Visibility())
		require.Equal(t, domain.OptInSingle, got.OptIn())
		require.Equal(t, []string{"x"}, got.Tags())
		require.False(t, got.CreatedAt().IsZero())

		page, total, err := repo.All(ctx, tenantID, domain.Page{Offset: 1, Limit: 1})
		require.NoError(t, err)
		require.Equal(t, 2, total, "the total ignores the page")
		require.Len(t, page, 1)
		require.Equal(t, "Newsletter", page[0].Name())
	})
}

func TestListsAddWithoutTagsReadsBackEmpty(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.lists(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		l, err := domain.NewList(tenantID, "Plain", "", domain.VisibilityPublic, domain.OptInDouble, nil)
		require.NoError(t, err)
		id, err := repo.Add(ctx, tenantID, l)
		require.NoError(t, err)

		got, err := repo.Get(ctx, tenantID, id)
		require.NoError(t, err)
		require.NotNil(t, got.Tags())
		require.Empty(t, got.Tags())
		require.Equal(t, domain.VisibilityPublic, got.Visibility())
		require.Equal(t, domain.OptInDouble, got.OptIn())
	})
}

func TestListsAddDuplicateNameConflicts(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.lists(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		addList(t, repo, tenantID, "Dup")
		_, err := repo.Add(ctx, tenantID, newList(t, tenantID, "Dup"))
		require.ErrorIs(t, err, domain.ErrListNameTaken)

		// Another tenant may reuse the name.
		other := b.newTenant(t)
		addList(t, repo, other, "Dup")
	})
}

func TestListsUpdateAndDelete(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.lists(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		id := addList(t, repo, tenantID, "Old")

		require.NoError(t, repo.Update(ctx, tenantID, id, func(l *domain.List) (*domain.List, error) {
			return l, l.Rename("Renamed")
		}))
		got, err := repo.Get(ctx, tenantID, id)
		require.NoError(t, err)
		require.Equal(t, "Renamed", got.Name())

		require.NoError(t, repo.Delete(ctx, tenantID, id))
		_, err = repo.Get(ctx, tenantID, id)
		require.ErrorIs(t, err, domain.ErrListNotFound)
		require.ErrorIs(t, repo.Delete(ctx, tenantID, id), domain.ErrListNotFound)
	})
}

func TestListsUpdateOntoTakenNameConflicts(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.lists(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		addList(t, repo, tenantID, "Taken")
		id := addList(t, repo, tenantID, "Free")

		err := repo.Update(ctx, tenantID, id, func(l *domain.List) (*domain.List, error) {
			return l, l.Rename("Taken")
		})
		require.ErrorIs(t, err, domain.ErrListNameTaken)
		got, err := repo.Get(ctx, tenantID, id)
		require.NoError(t, err)
		require.Equal(t, "Free", got.Name(), "a failed update writes nothing")

		require.ErrorIs(t, repo.Update(ctx, tenantID, "not-a-uuid",
			func(l *domain.List) (*domain.List, error) { return l, nil }), domain.ErrListNotFound)
	})
}

func TestListsGetMissing(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.lists(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		_, err := repo.Get(ctx, tenantID, "not-a-uuid")
		require.ErrorIs(t, err, domain.ErrListNotFound)
	})
}

func TestListsTenantIsolation(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.lists(t)
		ctx := context.Background()
		tenantA, tenantB := b.newTenant(t), b.newTenant(t)

		idA := addList(t, repo, tenantA, "Mine")

		lists, total, err := repo.All(ctx, tenantB, domain.DefaultPage)
		require.NoError(t, err)
		require.Zero(t, total)
		require.Empty(t, lists)
		_, err = repo.Get(ctx, tenantB, idA)
		require.ErrorIs(t, err, domain.ErrListNotFound)
		require.ErrorIs(t, repo.Delete(ctx, tenantB, idA), domain.ErrListNotFound)
		_, err = repo.Get(ctx, tenantA, idA)
		require.NoError(t, err, "another tenant's delete leaves the list alone")
	})
}
```

- [ ] **Step 4: Rewrite the fields tests as a contract suite**

Run:

```bash
git mv internal/audience/adapters/fields_postgres_test.go internal/audience/adapters/fields_test.go
```

Replace `internal/audience/adapters/fields_test.go` with:

```go
package adapters_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/audience/domain"
)

func newField(t *testing.T, tenantID, slug, displayName string, pos int) *domain.Field {
	t.Helper()
	f, err := domain.NewField(tenantID, slug, displayName, domain.FieldTypeText, "", pos)
	require.NoError(t, err)
	return f
}

func TestFields_AddAndAll(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.fields(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		id1, err := repo.Add(ctx, tenantID, newField(t, tenantID, "country", "Country", 1))
		require.NoError(t, err)
		require.NotEmpty(t, id1)

		id2, err := repo.Add(ctx, tenantID, newField(t, tenantID, "plan_tier", "Plan tier", 2))
		require.NoError(t, err)

		all, err := repo.All(ctx, tenantID)
		require.NoError(t, err)
		require.Len(t, all, 2)
		require.Equal(t, "country", all[0].Slug())
		require.Equal(t, "plan_tier", all[1].Slug())
		require.Equal(t, id1, all[0].ID())
		require.Equal(t, id2, all[1].ID())
		require.Equal(t, 2, all[1].Position())
		require.Equal(t, domain.FieldTypeText, all[1].Type())
		require.False(t, all[1].BuiltIn())
	})
}

func TestFields_AddDuplicateSlug(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.fields(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		_, err := repo.Add(ctx, tenantID, newField(t, tenantID, "country", "Country", 0))
		require.NoError(t, err)
		_, err = repo.Add(ctx, tenantID, newField(t, tenantID, "country", "Country dup", 1))
		require.ErrorIs(t, err, domain.ErrFieldSlugTaken)
	})
}

func TestFields_AddRejectsBuiltinSlug(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.fields(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		_, err := repo.Add(ctx, tenantID, newField(t, tenantID, "first_name", "First name", 0))
		require.ErrorIs(t, err, domain.ErrFieldBuiltinSlug)
	})
}

func TestFields_UpdateAndGet(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.fields(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		id, err := repo.Add(ctx, tenantID, newField(t, tenantID, "country", "Country", 0))
		require.NoError(t, err)

		err = repo.Update(ctx, tenantID, id, func(f *domain.Field) (*domain.Field, error) {
			require.NoError(t, f.Rename("Region"))
			require.NoError(t, f.Retype(domain.FieldTypeNumber))
			f.SetDefaultValue("US")
			return f, nil
		})
		require.NoError(t, err)

		got, err := repo.Get(ctx, tenantID, id)
		require.NoError(t, err)
		require.Equal(t, "Region", got.DisplayName())
		require.Equal(t, domain.FieldTypeNumber, got.Type())
		require.Equal(t, "US", got.DefaultValue())
		require.Equal(t, "country", got.Slug(), "the slug is immutable")

		require.ErrorIs(t, repo.Update(ctx, tenantID, "not-a-uuid",
			func(f *domain.Field) (*domain.Field, error) { return f, nil }), domain.ErrFieldNotFound)
	})
}

func TestFields_Delete(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.fields(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		id, err := repo.Add(ctx, tenantID, newField(t, tenantID, "country", "Country", 0))
		require.NoError(t, err)
		require.NoError(t, repo.Delete(ctx, tenantID, id))

		_, err = repo.Get(ctx, tenantID, id)
		require.ErrorIs(t, err, domain.ErrFieldNotFound)
		require.ErrorIs(t, repo.Delete(ctx, tenantID, id), domain.ErrFieldNotFound)
	})
}

func TestFields_Reorder(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.fields(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		id1, err := repo.Add(ctx, tenantID, newField(t, tenantID, "country", "Country", 0))
		require.NoError(t, err)
		id2, err := repo.Add(ctx, tenantID, newField(t, tenantID, "plan_tier", "Plan tier", 1))
		require.NoError(t, err)

		require.NoError(t, repo.Reorder(ctx, tenantID, map[string]int{id1: 5, id2: 0}))

		all, err := repo.All(ctx, tenantID)
		require.NoError(t, err)
		require.Equal(t, "plan_tier", all[0].Slug(), "lower position is listed first")
		require.Equal(t, "country", all[1].Slug())
		require.Equal(t, 5, all[1].Position())
	})
}

func TestFields_ReorderUnknownIDWritesNothing(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.fields(t)
		ctx := context.Background()
		tenantID, other := b.newTenant(t), b.newTenant(t)

		id, err := repo.Add(ctx, tenantID, newField(t, tenantID, "country", "Country", 0))
		require.NoError(t, err)
		foreign, err := repo.Add(ctx, other, newField(t, other, "country", "Country", 0))
		require.NoError(t, err)

		err = repo.Reorder(ctx, tenantID, map[string]int{id: 3, foreign: 4})
		require.ErrorIs(t, err, domain.ErrFieldNotFound, "another tenant's field is not found")

		got, err := repo.Get(ctx, tenantID, id)
		require.NoError(t, err)
		require.Equal(t, 0, got.Position(), "the reorder is all or nothing")
	})
}

func TestFields_TenantIsolation(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.fields(t)
		ctx := context.Background()
		tenantA, tenantB := b.newTenant(t), b.newTenant(t)

		idA, err := repo.Add(ctx, tenantA, newField(t, tenantA, "country", "Country", 0))
		require.NoError(t, err)

		// Tenant B must not see tenant A's rows.
		listB, err := repo.All(ctx, tenantB)
		require.NoError(t, err)
		require.Empty(t, listB)

		_, err = repo.Get(ctx, tenantB, idA)
		require.ErrorIs(t, err, domain.ErrFieldNotFound)

		// The same slug is free in another tenant.
		_, err = repo.Add(ctx, tenantB, newField(t, tenantB, "country", "Country", 0))
		require.NoError(t, err)
	})
}
```

- [ ] **Step 5: Run the suites to verify they fail**

Run: `go vet ./internal/audience/adapters/`

Expected: build failure `undefined: adapters.NewYDBLists` and `undefined: adapters.NewYDBFields`.

- [ ] **Step 6: Implement `YDBLists`**

Create `internal/audience/adapters/lists_ydb.go`. It also holds the binders and codecs the other YDB adapters share.

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

	"github.com/nikolaymatrosov/nvelope/internal/audience/domain"
	"github.com/nikolaymatrosov/nvelope/internal/db"
)

// YDBLists is the YDB-backed implementation of domain.ListRepository, for the
// serverless runtime. YDB has no row-level security, so every statement
// filters by tenant_id explicitly. A list's tags are stored as a JSON array in
// a Utf8 column.
type YDBLists struct {
	driver *ydb.Driver
}

var _ domain.ListRepository = (*YDBLists)(nil)

// NewYDBLists builds a YDBLists repository over the driver.
func NewYDBLists(d *ydb.Driver) *YDBLists {
	return &YDBLists{driver: d}
}

// ydbListColumns is the projection scanYDBList reads, in order.
const ydbListColumns = `id, tenant_id, name, description, visibility, optin, tags, created_at, updated_at`

const ydbSelectList = `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
SELECT ` + ydbListColumns + `
FROM lists
WHERE tenant_id = $tenant_id AND id = $id;`

// Add persists a new list under a fresh UUID and returns the id. It returns
// domain.ErrListNameTaken when the tenant already has a list with that name.
func (r *YDBLists) Add(ctx context.Context, tenantID string, l *domain.List) (string, error) {
	id := uuid.NewString()
	err := r.driver.Query().Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DECLARE $name AS Utf8;
DECLARE $description AS Utf8;
DECLARE $visibility AS Utf8;
DECLARE $optin AS Utf8;
DECLARE $tags AS Utf8;
INSERT INTO lists (tenant_id, id, name, description, visibility, optin, tags, created_at, updated_at)
VALUES ($tenant_id, $id, $name, $description, $visibility, $optin, $tags,
    CurrentUtcTimestamp(), CurrentUtcTimestamp());`,
		query.WithParameters(ydb.ParamsBuilder().
			Param("$tenant_id").Text(tenantID).
			Param("$id").Text(id).
			Param("$name").Text(l.Name()).
			Param("$description").Text(l.Description()).
			Param("$visibility").Text(string(l.Visibility())).
			Param("$optin").Text(string(l.OptIn())).
			Param("$tags").Text(ydbStrings(l.Tags())).
			Build()))
	// Only the name index can collide: the id is fresh.
	if db.IsYDBConflict(err) {
		return "", domain.ErrListNameTaken
	}
	if err != nil {
		return "", fmt.Errorf("inserting list: %w", err)
	}
	return id, nil
}

// Update loads the list, runs fn, and persists the result in one serializable
// transaction. It returns domain.ErrListNameTaken when the new name belongs to
// another list of the tenant. The SDK retries the transaction on conflict, so
// fn may run more than once and must be retry-safe.
func (r *YDBLists) Update(ctx context.Context, tenantID, id string,
	fn func(*domain.List) (*domain.List, error)) error {

	err := r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		loaded, err := ydbListFromRow(tx.QueryRow(ctx, ydbSelectList, ydbTenantAndID(tenantID, id)))
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
DECLARE $description AS Utf8;
DECLARE $visibility AS Utf8;
DECLARE $optin AS Utf8;
DECLARE $tags AS Utf8;
UPDATE lists SET name = $name, description = $description, visibility = $visibility,
    optin = $optin, tags = $tags, updated_at = CurrentUtcTimestamp()
WHERE tenant_id = $tenant_id AND id = $id;`,
			query.WithParameters(ydb.ParamsBuilder().
				Param("$tenant_id").Text(tenantID).
				Param("$id").Text(id).
				Param("$name").Text(updated.Name()).
				Param("$description").Text(updated.Description()).
				Param("$visibility").Text(string(updated.Visibility())).
				Param("$optin").Text(string(updated.OptIn())).
				Param("$tags").Text(ydbStrings(updated.Tags())).
				Build()))
		// An UPDATE onto another list's name hits the unique name index at
		// the statement (spec "Plan B4 planning spike").
		if db.IsYDBConflict(err) {
			return domain.ErrListNameTaken
		}
		if err != nil {
			return fmt.Errorf("updating list: %w", err)
		}
		return nil
	})
	// This covers a YDB that reports the collision at commit.
	if db.IsYDBConflict(err) {
		return domain.ErrListNameTaken
	}
	return err
}

// Delete removes the list and, in the same transaction, the rows Postgres
// deletes by foreign-key cascade: the list's memberships and the iam per-list
// role assignments on it. It returns domain.ErrListNotFound when absent.
func (r *YDBLists) Delete(ctx context.Context, tenantID, id string) error {
	return r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		_, err := tx.QueryRow(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DELETE FROM lists
WHERE tenant_id = $tenant_id AND id = $id
RETURNING id;`, ydbTenantAndID(tenantID, id))
		if errors.Is(err, query.ErrNoRows) {
			return domain.ErrListNotFound
		}
		if err != nil {
			return fmt.Errorf("deleting list: %w", err)
		}
		// ponytail: one transaction deletes every membership of the list; page
		// the delete if a list outgrows what one YDB transaction can hold.
		if err := tx.Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DELETE FROM subscriber_lists ON
SELECT tenant_id, subscriber_id, list_id
FROM subscriber_lists VIEW idx_subscriber_lists_list
WHERE tenant_id = $tenant_id AND list_id = $id;`, ydbTenantAndID(tenantID, id)); err != nil {
			return fmt.Errorf("deleting list memberships: %w", err)
		}
		// user_list_roles belongs to iam. Postgres cascades into it through
		// its foreign key to lists; without this delete a dangling row would
		// keep its role in use forever. The scan covers the tenant's per-list
		// role assignments, a handful per user.
		if err := tx.Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DELETE FROM user_list_roles
WHERE tenant_id = $tenant_id AND list_id = $id;`, ydbTenantAndID(tenantID, id)); err != nil {
			return fmt.Errorf("deleting list roles: %w", err)
		}
		return nil
	})
}

// Get returns the list, or domain.ErrListNotFound.
func (r *YDBLists) Get(ctx context.Context, tenantID, id string) (*domain.List, error) {
	return ydbListFromRow(r.driver.Query().QueryRow(ctx, ydbSelectList, ydbTenantAndID(tenantID, id)))
}

// All returns a page of the tenant's lists, ordered by name, and the total
// count, read in one transaction.
func (r *YDBLists) All(ctx context.Context, tenantID string, page domain.Page) ([]*domain.List, int, error) {
	page = page.Normalize()
	var lists []*domain.List
	var total int
	err := r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		count, err := ydbCount(ctx, tx, `DECLARE $tenant_id AS Utf8;
SELECT COUNT(*) FROM lists WHERE tenant_id = $tenant_id;`, ydbTenantParam(tenantID))
		if err != nil {
			return fmt.Errorf("counting lists: %w", err)
		}
		// ponytail: OFFSET paging reads every skipped row (spec "Ceilings").
		rs, err := tx.QueryResultSet(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $limit AS Uint64;
DECLARE $offset AS Uint64;
SELECT `+ydbListColumns+`
FROM lists
WHERE tenant_id = $tenant_id
ORDER BY name
LIMIT $limit OFFSET $offset;`, ydbTenantAndPage(tenantID, page))
		if err != nil {
			return fmt.Errorf("listing lists: %w", err)
		}
		// Collected into a local and assigned at the end: the SDK may re-run
		// the closure, and a retry must not append to rows a failed run read.
		var local []*domain.List
		for row, err := range rs.Rows(ctx) {
			if err != nil {
				return fmt.Errorf("listing lists: %w", err)
			}
			l, err := scanYDBList(row)
			if err != nil {
				return err
			}
			local = append(local, l)
		}
		lists, total = local, count
		return nil
	})
	if err != nil {
		return nil, 0, err
	}
	return lists, total, nil
}

// ydbListFromRow maps a single-row read to the list, translating "no row"
// into domain.ErrListNotFound.
func ydbListFromRow(row query.Row, err error) (*domain.List, error) {
	if errors.Is(err, query.ErrNoRows) {
		return nil, domain.ErrListNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("loading list: %w", err)
	}
	return scanYDBList(row)
}

// scanYDBList reads one row in ydbListColumns order.
func scanYDBList(row query.Row) (*domain.List, error) {
	var id, tenantID, name, description, visibility, optin, rawTags string
	var createdAt, updatedAt time.Time
	if err := row.Scan(&id, &tenantID, &name, &description, &visibility, &optin,
		&rawTags, &createdAt, &updatedAt); err != nil {
		return nil, fmt.Errorf("scanning list: %w", err)
	}
	tags, err := ydbStringsFrom(rawTags)
	if err != nil {
		return nil, fmt.Errorf("decoding list tags: %w", err)
	}
	return domain.HydrateList(id, tenantID, name, description,
		domain.Visibility(visibility), domain.OptIn(optin), tags, createdAt, updatedAt), nil
}

// ydbStrings encodes a string slice as the JSON array a Utf8 column stores
// (Postgres: text[]). A nil slice encodes as [], as Postgres reads back '{}'.
func ydbStrings(s []string) string {
	if s == nil {
		s = []string{}
	}
	raw, _ := json.Marshal(s) // a []string always encodes
	return string(raw)
}

// ydbStringsFrom decodes a column written by ydbStrings.
func ydbStringsFrom(raw string) ([]string, error) {
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ydbCount runs a single-row COUNT(*) query.
func ydbCount(ctx context.Context, tx query.TxActor, q string, opt query.ExecuteOption) (int, error) {
	row, err := tx.QueryRow(ctx, q, opt)
	if err != nil {
		return 0, err
	}
	var n uint64
	if err := row.Scan(&n); err != nil {
		return 0, err
	}
	return int(n), nil
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

// ydbTenantAndPage binds $tenant_id, $limit and $offset for a normalized page.
func ydbTenantAndPage(tenantID string, page domain.Page) query.ExecuteOption {
	return query.WithParameters(ydb.ParamsBuilder().
		Param("$tenant_id").Text(tenantID).
		Param("$limit").Uint64(uint64(page.Limit)).
		Param("$offset").Uint64(uint64(page.Offset)).
		Build())
}
```

- [ ] **Step 7: Implement `YDBFields`**

Create `internal/audience/adapters/fields_ydb.go`:

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

	"github.com/nikolaymatrosov/nvelope/internal/audience/domain"
	"github.com/nikolaymatrosov/nvelope/internal/db"
)

// YDBFields is the YDB-backed implementation of domain.FieldRepository, for
// the serverless runtime. Built-in pseudo-rows are never persisted, as on
// Postgres.
type YDBFields struct {
	driver *ydb.Driver
}

var _ domain.FieldRepository = (*YDBFields)(nil)

// NewYDBFields builds a YDBFields repository over the driver.
func NewYDBFields(d *ydb.Driver) *YDBFields {
	return &YDBFields{driver: d}
}

// ydbFieldColumns is the projection scanYDBField reads, in order.
const ydbFieldColumns = `id, tenant_id, slug, display_name, type, default_value, position, created_at, updated_at`

const ydbSelectField = `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
SELECT ` + ydbFieldColumns + `
FROM subscriber_fields
WHERE tenant_id = $tenant_id AND id = $id;`

// Add persists a new field under a fresh UUID and returns the id. It returns
// domain.ErrFieldBuiltinSlug for a built-in slug and domain.ErrFieldSlugTaken
// when the tenant already has a field with that slug.
func (r *YDBFields) Add(ctx context.Context, tenantID string, f *domain.Field) (string, error) {
	if domain.IsBuiltinFieldSlug(f.Slug()) {
		return "", domain.ErrFieldBuiltinSlug
	}
	id := uuid.NewString()
	err := r.driver.Query().Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DECLARE $slug AS Utf8;
DECLARE $display_name AS Utf8;
DECLARE $type AS Utf8;
DECLARE $default_value AS Utf8;
DECLARE $position AS Int64;
INSERT INTO subscriber_fields
    (tenant_id, id, slug, display_name, type, default_value, position, created_at, updated_at)
VALUES ($tenant_id, $id, $slug, $display_name, $type, $default_value, $position,
    CurrentUtcTimestamp(), CurrentUtcTimestamp());`,
		query.WithParameters(ydb.ParamsBuilder().
			Param("$tenant_id").Text(tenantID).
			Param("$id").Text(id).
			Param("$slug").Text(f.Slug()).
			Param("$display_name").Text(f.DisplayName()).
			Param("$type").Text(string(f.Type())).
			Param("$default_value").Text(f.DefaultValue()).
			Param("$position").Int64(int64(f.Position())).
			Build()))
	// Only the slug index can collide: the id is fresh.
	if db.IsYDBConflict(err) {
		return "", domain.ErrFieldSlugTaken
	}
	if err != nil {
		return "", fmt.Errorf("inserting subscriber field: %w", err)
	}
	return id, nil
}

// Get returns the field, or domain.ErrFieldNotFound.
func (r *YDBFields) Get(ctx context.Context, tenantID, id string) (*domain.Field, error) {
	return ydbFieldFromRow(r.driver.Query().QueryRow(ctx, ydbSelectField, ydbTenantAndID(tenantID, id)))
}

// Update loads the field, runs fn, and persists the result in one serializable
// transaction. The slug is immutable, as on Postgres. The SDK retries the
// transaction on conflict, so fn may run more than once and must be
// retry-safe.
func (r *YDBFields) Update(ctx context.Context, tenantID, id string,
	fn func(*domain.Field) (*domain.Field, error)) error {

	return r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		loaded, err := ydbFieldFromRow(tx.QueryRow(ctx, ydbSelectField, ydbTenantAndID(tenantID, id)))
		if err != nil {
			return err
		}
		updated, err := fn(loaded)
		if err != nil {
			return err
		}
		if err := tx.Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DECLARE $display_name AS Utf8;
DECLARE $type AS Utf8;
DECLARE $default_value AS Utf8;
DECLARE $position AS Int64;
UPDATE subscriber_fields SET display_name = $display_name, type = $type,
    default_value = $default_value, position = $position, updated_at = CurrentUtcTimestamp()
WHERE tenant_id = $tenant_id AND id = $id;`,
			query.WithParameters(ydb.ParamsBuilder().
				Param("$tenant_id").Text(tenantID).
				Param("$id").Text(id).
				Param("$display_name").Text(updated.DisplayName()).
				Param("$type").Text(string(updated.Type())).
				Param("$default_value").Text(updated.DefaultValue()).
				Param("$position").Int64(int64(updated.Position())).
				Build())); err != nil {
			return fmt.Errorf("updating subscriber field: %w", err)
		}
		return nil
	})
}

// Delete removes the field. It returns domain.ErrFieldNotFound when absent.
func (r *YDBFields) Delete(ctx context.Context, tenantID, id string) error {
	_, err := r.driver.Query().QueryRow(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DELETE FROM subscriber_fields
WHERE tenant_id = $tenant_id AND id = $id
RETURNING id;`, ydbTenantAndID(tenantID, id))
	if errors.Is(err, query.ErrNoRows) {
		return domain.ErrFieldNotFound
	}
	if err != nil {
		return fmt.Errorf("deleting subscriber field: %w", err)
	}
	return nil
}

// All returns every field of the tenant in display order: position, then
// creation time.
func (r *YDBFields) All(ctx context.Context, tenantID string) ([]*domain.Field, error) {
	rs, err := r.driver.Query().QueryResultSet(ctx, `DECLARE $tenant_id AS Utf8;
SELECT `+ydbFieldColumns+`
FROM subscriber_fields
WHERE tenant_id = $tenant_id
ORDER BY position, created_at;`, ydbTenantParam(tenantID))
	if err != nil {
		return nil, fmt.Errorf("listing subscriber fields: %w", err)
	}
	defer func() { _ = rs.Close(ctx) }()
	var fields []*domain.Field
	for row, err := range rs.Rows(ctx) {
		if err != nil {
			return nil, fmt.Errorf("listing subscriber fields: %w", err)
		}
		f, err := scanYDBField(row)
		if err != nil {
			return nil, err
		}
		fields = append(fields, f)
	}
	return fields, nil
}

// Reorder sets the position of every given field in one transaction. It
// returns domain.ErrFieldNotFound, and writes nothing, when an id is not a
// field of the tenant.
func (r *YDBFields) Reorder(ctx context.Context, tenantID string, positions map[string]int) error {
	return r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		for id, pos := range positions {
			_, err := tx.QueryRow(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DECLARE $position AS Int64;
UPDATE subscriber_fields SET position = $position, updated_at = CurrentUtcTimestamp()
WHERE tenant_id = $tenant_id AND id = $id
RETURNING id;`,
				query.WithParameters(ydb.ParamsBuilder().
					Param("$tenant_id").Text(tenantID).
					Param("$id").Text(id).
					Param("$position").Int64(int64(pos)).
					Build()))
			if errors.Is(err, query.ErrNoRows) {
				return domain.ErrFieldNotFound
			}
			if err != nil {
				return fmt.Errorf("reordering subscriber field %s: %w", id, err)
			}
		}
		return nil
	})
}

// ydbFieldFromRow maps a single-row read to the field, translating "no row"
// into domain.ErrFieldNotFound.
func ydbFieldFromRow(row query.Row, err error) (*domain.Field, error) {
	if errors.Is(err, query.ErrNoRows) {
		return nil, domain.ErrFieldNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("loading subscriber field: %w", err)
	}
	return scanYDBField(row)
}

// scanYDBField reads one row in ydbFieldColumns order.
func scanYDBField(row query.Row) (*domain.Field, error) {
	var id, tenantID, slug, displayName, fieldType, defaultValue string
	var position int64
	var createdAt, updatedAt time.Time
	if err := row.Scan(&id, &tenantID, &slug, &displayName, &fieldType, &defaultValue,
		&position, &createdAt, &updatedAt); err != nil {
		return nil, fmt.Errorf("scanning subscriber field: %w", err)
	}
	return domain.HydrateField(id, tenantID, slug, displayName, domain.FieldType(fieldType),
		defaultValue, int(position), false, createdAt, updatedAt), nil
}
```

- [ ] **Step 8: Update the package doc**

In `internal/audience/adapters/lists_pg.go`, replace the first three lines (the package comment) with:

```go
// Package adapters implements the audience domain's repository interfaces
// against PostgreSQL. Every tenant-plane operation runs inside the shared
// RLS-bound transaction (internal/platform/tenantdb). The *_ydb.go files are
// the YDB implementations of the repositories for the serverless runtime;
// they filter by tenant_id explicitly instead.
```

- [ ] **Step 9: Run the suites to verify they pass**

Run: `export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock" && go test ./internal/audience/adapters/ -count=1 -v -timeout 8m 2>&1 | grep -E -- '--- (PASS|FAIL)|^(ok|FAIL)' | grep -E 'FAIL|/pg|/ydb|^ok' | sort`

Expected:
- 30 `--- PASS` lines: every `TestLists*` and `TestFields_*` on both `/pg` and `/ydb`.
- No `FAIL` line. The pg-only suites (subscribers, memberships, preferences, jobs, opt-in) still run; the filter hides their `PASS` lines but would show a `FAIL`.
- The package is `ok`.

Two cases are not yours to fix. Report BLOCKED with the verbatim output:
- **A `/pg` subtest fails.** The contract would then change behaviour for SaaS, or Postgres does not behave as the plan assumes.
- **The migration is rejected by YDB.**

Any other `/ydb` failure: fix only the YDB adapters or the migration, and record the verbatim error and the fix in the report.

- [ ] **Step 10: Full verification and commit**

Run: `export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock" && go build ./... && go vet ./... && go test ./... -timeout 15m 2>&1 | grep -vE '^(ok|\?)\s'`

Expected: no output. In other words, no failing package.

Run: `golangci-lint run`

Expected: `0 issues.`

Run: `PATH="$HOME/go/bin:$PATH" make lint-arch`

Expected: the last line is `[arch] clean`.

Run: `git diff --stat HEAD -- go.mod go.sum`

Expected: no output.

```bash
gofmt -l internal/audience/
git add internal/db/ydbmigrations/000006_audience.up.sql
git add internal/audience/adapters/lists_ydb.go internal/audience/adapters/fields_ydb.go internal/audience/adapters/lists_pg.go
git add internal/audience/adapters/helpers_test.go internal/audience/adapters/lists_test.go internal/audience/adapters/fields_test.go
git commit -m "feat(audience): add the YDB list and field repositories under pg+ydb contract suites"
```

`gofmt -l` must print nothing. Afterwards, `git status --short` must print nothing: `lists_pg_test.go` and `fields_postgres_test.go` are gone, and no file is left untracked.

---

### Task 2: `YDBSubscribers` with the YQL segment translator, `YDBMemberships` and their contract suites

**Files:**
- Create: `internal/audience/adapters/segment_yql.go`
- Create: `internal/audience/adapters/subscribers_ydb.go`
- Create: `internal/audience/adapters/memberships_ydb.go`
- Create: `internal/audience/adapters/lists_ydb_test.go`
- Modify: `internal/audience/adapters/helpers_test.go` (rewritten)
- Rename: `internal/audience/adapters/subscribers_pg_test.go` → `internal/audience/adapters/subscribers_test.go` (rewritten)
- Rename: `internal/audience/adapters/memberships_pg_test.go` → `internal/audience/adapters/memberships_test.go` (rewritten)
- Rename: `internal/audience/adapters/preferences_pg_test.go` → `internal/audience/adapters/preferences_test.go` (rewritten)

**Interfaces:**
- Consumes:
  - Everything Task 1 produces.
  - The ports `domain.SubscriberRepository` and `domain.MembershipRepository` (`internal/audience/domain/repository.go`), unchanged.
  - `domain.HydrateSubscriber`, `domain.HydrateAttributes`, `domain.HydrateMembership`, `(*domain.Membership).ChangeStatus` and the segment types `domain.Segment`, `domain.Node`, `domain.FieldCondition`, `domain.AttrCondition`, `domain.MemberCondition` (`internal/audience/domain`), unchanged.
  - From the pg adapters in the same package: `marshalAttributes(a domain.Attributes) ([]byte, error)` (`subscribers_pg.go`) and `sqlComparison(op domain.SegmentOp) string` (`segment_sql.go`), unchanged.
  - The app handlers `command.NewUpdatePreferencesHandler`, `command.NewPublicUnsubscribeHandler` and `query.NewGetPreferencesHandler`, unchanged.
- Produces:
  ```go
  // package adapters (internal/audience/adapters)
  type YDBSubscribers struct{ /* unexported */ }
  func NewYDBSubscribers(d *ydb.Driver) *YDBSubscribers
  type YDBMemberships struct{ /* unexported */ }
  func NewYDBMemberships(d *ydb.Driver) *YDBMemberships
  func translateSegmentYQL(seg domain.Segment) (string, []ydbSegmentArg)
  // package adapters_test
  type backend struct {
      name        string
      lists       func(t *testing.T) domain.ListRepository
      subscribers func(t *testing.T) domain.SubscriberRepository
      memberships func(t *testing.T) domain.MembershipRepository
      fields      func(t *testing.T) domain.FieldRepository
      newTenant   func(t *testing.T) string
  }
  func newSubscriber(t *testing.T, tenantID, email string) *domain.Subscriber   // kept; jobs_pg_test.go uses it
  func addSubscriber(t *testing.T, repo domain.SubscriberRepository, tenantID, email string) string
  func emails(subs []*domain.Subscriber) []string                               // sorted
  ```
- Plan B6 relies on the harness above. It adds the jobs, subscription-page and pending-subscription repositories to `backend`.

- [ ] **Step 1: Extend the harness**

Replace `internal/audience/adapters/helpers_test.go` with the code below. Only the `subscribers` and `memberships` fields and their two constructors per backend are new.

```go
package adapters_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/audience/adapters"
	"github.com/nikolaymatrosov/nvelope/internal/audience/domain"
	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
)

// timeZeroAdapters is a fixed timestamp for hydrating entities in tests.
func timeZeroAdapters() time.Time { return time.Unix(0, 0).UTC() }

// backend is one storage implementation of the audience repositories under
// contract test. Every contract test body runs once per backend, so the
// Postgres and YDB adapters are held to the same behaviour.
type backend struct {
	name        string
	lists       func(t *testing.T) domain.ListRepository
	subscribers func(t *testing.T) domain.SubscriberRepository
	memberships func(t *testing.T) domain.MembershipRepository
	fields      func(t *testing.T) domain.FieldRepository
	// newTenant returns the id of a tenant row, which the tenant context
	// owns. On Postgres the audience tables carry a foreign key to it, so the
	// pg backend inserts a real row. YDB has no foreign keys and no audience
	// query reads the tenants table, so a bare UUID stands in.
	newTenant func(t *testing.T) string
}

var backends = []backend{
	{
		name: "pg",
		lists: func(t *testing.T) domain.ListRepository {
			return adapters.NewLists(dbtest.AppPool(t))
		},
		subscribers: func(t *testing.T) domain.SubscriberRepository {
			return adapters.NewSubscribers(dbtest.AppPool(t))
		},
		memberships: func(t *testing.T) domain.MembershipRepository {
			return adapters.NewMemberships(dbtest.AppPool(t))
		},
		fields: func(t *testing.T) domain.FieldRepository {
			return adapters.NewFields(dbtest.AppPool(t))
		},
		newTenant: func(t *testing.T) string { return seedTenant(t, dbtest.AppPool(t)) },
	},
	{
		name: "ydb",
		lists: func(t *testing.T) domain.ListRepository {
			return adapters.NewYDBLists(dbtest.YDBMigrated(t))
		},
		subscribers: func(t *testing.T) domain.SubscriberRepository {
			return adapters.NewYDBSubscribers(dbtest.YDBMigrated(t))
		},
		memberships: func(t *testing.T) domain.MembershipRepository {
			return adapters.NewYDBMemberships(dbtest.YDBMigrated(t))
		},
		fields: func(t *testing.T) domain.FieldRepository {
			return adapters.NewYDBFields(dbtest.YDBMigrated(t))
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

// seedTenant inserts a tenant row directly and returns its id. The audience
// adapters need a real tenant to satisfy the tenant_id foreign key and the RLS
// policy. The row is inserted with raw SQL rather than through the tenant
// context's adapter so the audience context's tests import no other context.
func seedTenant(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var id string
	require.NoError(t, pool.QueryRow(context.Background(),
		`INSERT INTO tenants (name, slug, status) VALUES ('Workspace', $1, 'active') RETURNING id`,
		"aud-"+dbtest.RandString()).Scan(&id))
	return id
}
```

- [ ] **Step 2: Rewrite the subscribers tests as a contract suite**

Run:

```bash
git mv internal/audience/adapters/subscribers_pg_test.go internal/audience/adapters/subscribers_test.go
```

Replace `internal/audience/adapters/subscribers_test.go` with the code below. Beyond the converted tests, it adds:
- `TestSubscribersUpsertByEmailKeepsState`: an upsert never re-enables a blocklisted subscriber, as Postgres `ON CONFLICT DO UPDATE SET name, attributes` does not.
- `TestSubscribersUpsertByEmailIsRaceFree`: eight concurrent upserts of one new email create exactly one row.
- `TestSubscribersDeleteCascadesMemberships`.
- Cyrillic and case-insensitive cases in `TestSubscribersSearch`. On YDB these close the spec's V1 `ILIKE` follow-up.
- `TestSubscribersTenantIsolation`.
- `TestSubscribersRunSegmentOperators`: every operator on fields, attributes and memberships, plus nested groups. It holds the YQL translator to the Postgres one.

```go
package adapters_test

import (
	"context"
	"sort"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/audience/domain"
)

func newSubscriber(t *testing.T, tenantID, email string) *domain.Subscriber {
	t.Helper()
	attrs, err := domain.NewAttributes(map[string]any{"plan": "pro"})
	require.NoError(t, err)
	s, err := domain.NewSubscriber(tenantID, email, "Pat", attrs)
	require.NoError(t, err)
	return s
}

// addSubscriber persists a subscriber and returns its id.
func addSubscriber(t *testing.T, repo domain.SubscriberRepository, tenantID, email string) string {
	t.Helper()
	id, err := repo.Add(context.Background(), tenantID, newSubscriber(t, tenantID, email))
	require.NoError(t, err)
	return id
}

// addSubscriberNamed persists a subscriber with the given email, name and
// attributes and returns its id.
func addSubscriberNamed(t *testing.T, repo domain.SubscriberRepository, tenantID, email, name string,
	attrs map[string]any) string {
	t.Helper()
	a, err := domain.NewAttributes(attrs)
	require.NoError(t, err)
	s, err := domain.NewSubscriber(tenantID, email, name, a)
	require.NoError(t, err)
	id, err := repo.Add(context.Background(), tenantID, s)
	require.NoError(t, err)
	return id
}

// addSubscriberWith persists a subscriber with the given email and attributes.
func addSubscriberWith(t *testing.T, repo domain.SubscriberRepository, tenantID, email string,
	attrs map[string]any) string {
	t.Helper()
	return addSubscriberNamed(t, repo, tenantID, email, "Pat", attrs)
}

func mustSegment(t *testing.T, node domain.Node) domain.Segment {
	t.Helper()
	seg, err := domain.NewSegment(node)
	require.NoError(t, err)
	return *seg
}

// emails returns the subscribers' emails, sorted.
func emails(subs []*domain.Subscriber) []string {
	out := make([]string, len(subs))
	for i, s := range subs {
		out[i] = s.Email()
	}
	sort.Strings(out)
	return out
}

func TestSubscribersAddGet(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.subscribers(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		id := addSubscriberWith(t, repo, tenantID, "a@example.com",
			map[string]any{"plan": "pro", "seats": 3, "tags": []any{"x"}, "nested": map[string]any{"k": "v"}})
		subs, total, err := repo.Search(ctx, tenantID, "", domain.DefaultPage)
		require.NoError(t, err)
		require.Equal(t, 1, total)
		require.Equal(t, id, subs[0].ID())

		got, err := repo.Get(ctx, tenantID, id)
		require.NoError(t, err)
		require.Equal(t, tenantID, got.TenantID())
		require.Equal(t, "a@example.com", got.Email())
		require.Equal(t, "Pat", got.Name())
		require.Equal(t, domain.StateEnabled, got.State())
		require.Equal(t, map[string]any{
			"plan": "pro", "seats": float64(3), "tags": []any{"x"}, "nested": map[string]any{"k": "v"},
		}, got.Attributes().Values(), "attributes round-trip as decoded JSON")
		require.False(t, got.CreatedAt().IsZero())

		_, err = repo.Get(ctx, tenantID, "not-a-uuid")
		require.ErrorIs(t, err, domain.ErrSubscriberNotFound)
	})
}

func TestSubscribersDuplicateEmailConflicts(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.subscribers(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		addSubscriber(t, repo, tenantID, "dup@example.com")
		_, err := repo.Add(ctx, tenantID, newSubscriber(t, tenantID, "DUP@example.com"))
		require.ErrorIs(t, err, domain.ErrSubscriberEmailTaken, "emails are unique case-insensitively")

		// Another tenant may reuse the email.
		other := b.newTenant(t)
		addSubscriber(t, repo, other, "dup@example.com")
	})
}

func TestSubscribersUpsertByEmail(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.subscribers(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		created, err := repo.UpsertByEmail(ctx, tenantID, newSubscriber(t, tenantID, "up@example.com"))
		require.NoError(t, err)
		require.True(t, created, "a new email inserts")

		updated := newSubscriber(t, tenantID, "UP@example.com")
		updated.Rename("Changed")
		created, err = repo.UpsertByEmail(ctx, tenantID, updated)
		require.NoError(t, err)
		require.False(t, created, "an existing email updates")

		subs, total, err := repo.Search(ctx, tenantID, "up@", domain.DefaultPage)
		require.NoError(t, err)
		require.Equal(t, 1, total)
		require.Equal(t, "Changed", subs[0].Name())
	})
}

func TestSubscribersUpsertByEmailKeepsState(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.subscribers(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		id := addSubscriber(t, repo, tenantID, "blocked@example.com")
		require.NoError(t, repo.Update(ctx, tenantID, id, func(s *domain.Subscriber) (*domain.Subscriber, error) {
			s.Blocklist()
			return s, nil
		}))

		created, err := repo.UpsertByEmail(ctx, tenantID, newSubscriber(t, tenantID, "blocked@example.com"))
		require.NoError(t, err)
		require.False(t, created)
		got, err := repo.Get(ctx, tenantID, id)
		require.NoError(t, err)
		require.Equal(t, domain.StateBlocklisted, got.State(), "an upsert never re-enables a subscriber")
	})
}

func TestSubscribersUpsertByEmailIsRaceFree(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.subscribers(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		const n = 8
		var wg sync.WaitGroup
		results := make([]bool, n)
		errs := make([]error, n)
		for i := range n {
			wg.Go(func() {
				results[i], errs[i] = repo.UpsertByEmail(ctx, tenantID, newSubscriber(t, tenantID, "race@example.com"))
			})
		}
		wg.Wait()

		createdCount := 0
		for i := range n {
			require.NoError(t, errs[i])
			if results[i] {
				createdCount++
			}
		}
		require.Equal(t, 1, createdCount, "exactly one concurrent upsert creates the subscriber")
		_, total, err := repo.Search(ctx, tenantID, "race@", domain.DefaultPage)
		require.NoError(t, err)
		require.Equal(t, 1, total)
	})
}

func TestSubscribersUpdateStateAndDelete(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.subscribers(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		id := addSubscriber(t, repo, tenantID, "s@example.com")

		require.NoError(t, repo.Update(ctx, tenantID, id, func(s *domain.Subscriber) (*domain.Subscriber, error) {
			s.Blocklist()
			s.Rename("Renamed")
			return s, nil
		}))
		got, err := repo.Get(ctx, tenantID, id)
		require.NoError(t, err)
		require.Equal(t, domain.StateBlocklisted, got.State())
		require.Equal(t, "Renamed", got.Name())

		require.ErrorIs(t, repo.Update(ctx, tenantID, "not-a-uuid",
			func(s *domain.Subscriber) (*domain.Subscriber, error) { return s, nil }), domain.ErrSubscriberNotFound)

		require.NoError(t, repo.Delete(ctx, tenantID, id))
		_, err = repo.Get(ctx, tenantID, id)
		require.ErrorIs(t, err, domain.ErrSubscriberNotFound)
		require.ErrorIs(t, repo.Delete(ctx, tenantID, id), domain.ErrSubscriberNotFound)
	})
}

func TestSubscribersDeleteCascadesMemberships(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		subs, lists, memberships := b.subscribers(t), b.lists(t), b.memberships(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		listID := addList(t, lists, tenantID, "Newsletter")
		id := addSubscriber(t, subs, tenantID, "gone@example.com")
		require.NoError(t, memberships.Attach(ctx, tenantID, id, listID, domain.SubscriptionConfirmed))

		require.NoError(t, subs.Delete(ctx, tenantID, id))
		got, err := memberships.ForSubscriber(ctx, tenantID, id)
		require.NoError(t, err)
		require.Empty(t, got, "the subscriber's memberships go with it")
		_, total, err := subs.InList(ctx, tenantID, listID, domain.DefaultPage)
		require.NoError(t, err)
		require.Zero(t, total)
	})
}

func TestSubscribersSearch(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.subscribers(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		addSubscriberNamed(t, repo, tenantID, "alice@example.com", "Алиса Петрова", nil)
		addSubscriberNamed(t, repo, tenantID, "bob@example.com", "Bob", nil)

		for _, tc := range []struct {
			q    string
			want []string
		}{
			{"alice", []string{"alice@example.com"}},
			{"ALICE@EXAMPLE", []string{"alice@example.com"}},
			{"петрова", []string{"alice@example.com"}},
			{"АЛИСА", []string{"alice@example.com"}},
			{"  bob ", []string{"bob@example.com"}},
			{"example.com", []string{"alice@example.com", "bob@example.com"}},
			{"", []string{"alice@example.com", "bob@example.com"}},
			{"nobody", []string{}},
		} {
			subs, total, err := repo.Search(ctx, tenantID, tc.q, domain.DefaultPage)
			require.NoError(t, err, tc.q)
			require.Equal(t, tc.want, emails(subs), tc.q)
			require.Equal(t, len(tc.want), total, tc.q)
		}

		page, total, err := repo.Search(ctx, tenantID, "", domain.Page{Offset: 0, Limit: 1})
		require.NoError(t, err)
		require.Equal(t, 2, total, "the total ignores the page")
		require.Len(t, page, 1)
	})
}

func TestSubscribersTenantIsolation(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.subscribers(t)
		ctx := context.Background()
		tenantA, tenantB := b.newTenant(t), b.newTenant(t)

		idA := addSubscriber(t, repo, tenantA, "mine@example.com")

		subs, total, err := repo.Search(ctx, tenantB, "", domain.DefaultPage)
		require.NoError(t, err)
		require.Zero(t, total)
		require.Empty(t, subs)
		_, err = repo.Get(ctx, tenantB, idA)
		require.ErrorIs(t, err, domain.ErrSubscriberNotFound)
		require.ErrorIs(t, repo.Delete(ctx, tenantB, idA), domain.ErrSubscriberNotFound)
		count, err := repo.CountSegment(ctx, tenantB, mustSegment(t, domain.Node{
			Field: &domain.FieldCondition{Field: "email", Op: domain.OpExists},
		}))
		require.NoError(t, err)
		require.Zero(t, count)
		_, err = repo.Get(ctx, tenantA, idA)
		require.NoError(t, err)
	})
}

func TestSubscribersRunSegmentAttribute(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.subscribers(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		addSubscriberWith(t, repo, tenantID, "pro@example.com", map[string]any{"plan": "pro"})
		addSubscriberWith(t, repo, tenantID, "free@example.com", map[string]any{"plan": "free"})

		seg := mustSegment(t, domain.Node{
			Attr: &domain.AttrCondition{Key: "plan", Op: domain.OpEq, Value: "pro"},
		})
		subs, total, err := repo.RunSegment(ctx, tenantID, seg, domain.DefaultPage)
		require.NoError(t, err)
		require.Equal(t, 1, total)
		require.Equal(t, "pro@example.com", subs[0].Email())
	})
}

func TestSubscribersRunSegmentMembership(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo, lists, memberships := b.subscribers(t), b.lists(t), b.memberships(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		listID := addList(t, lists, tenantID, "Newsletter")
		memberID := addSubscriberWith(t, repo, tenantID, "in@example.com", nil)
		addSubscriberWith(t, repo, tenantID, "out@example.com", nil)
		require.NoError(t, memberships.Attach(ctx, tenantID, memberID, listID, domain.SubscriptionConfirmed))

		seg := mustSegment(t, domain.Node{
			Member: &domain.MemberCondition{ListID: listID},
		})
		subs, total, err := repo.RunSegment(ctx, tenantID, seg, domain.DefaultPage)
		require.NoError(t, err)
		require.Equal(t, 1, total)
		require.Equal(t, "in@example.com", subs[0].Email())
	})
}

func TestSubscribersRunSegmentCombined(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo, lists, memberships := b.subscribers(t), b.lists(t), b.memberships(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		listID := addList(t, lists, tenantID, "Newsletter")
		match := addSubscriberWith(t, repo, tenantID, "match@example.com", map[string]any{"plan": "pro"})
		wrongAttr := addSubscriberWith(t, repo, tenantID, "wrong@example.com", map[string]any{"plan": "free"})
		addSubscriberWith(t, repo, tenantID, "nomember@example.com", map[string]any{"plan": "pro"})
		require.NoError(t, memberships.Attach(ctx, tenantID, match, listID, domain.SubscriptionConfirmed))
		require.NoError(t, memberships.Attach(ctx, tenantID, wrongAttr, listID, domain.SubscriptionConfirmed))

		seg := mustSegment(t, domain.Node{
			Conj: domain.ConjAnd,
			Children: []domain.Node{
				{Attr: &domain.AttrCondition{Key: "plan", Op: domain.OpEq, Value: "pro"}},
				{Member: &domain.MemberCondition{ListID: listID}},
			},
		})
		subs, total, err := repo.RunSegment(ctx, tenantID, seg, domain.DefaultPage)
		require.NoError(t, err)
		require.Equal(t, 1, total)
		require.Equal(t, "match@example.com", subs[0].Email())

		count, err := repo.CountSegment(ctx, tenantID, seg)
		require.NoError(t, err)
		require.Equal(t, 1, count)
	})
}

func TestSubscribersRunSegmentEmptyResult(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.subscribers(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		addSubscriberWith(t, repo, tenantID, "free@example.com", map[string]any{"plan": "free"})

		seg := mustSegment(t, domain.Node{
			Attr: &domain.AttrCondition{Key: "plan", Op: domain.OpEq, Value: "enterprise"},
		})
		subs, total, err := repo.RunSegment(ctx, tenantID, seg, domain.DefaultPage)
		require.NoError(t, err)
		require.Equal(t, 0, total)
		require.Empty(t, subs)
	})
}

// TestSubscribersRunSegmentOperators holds the YQL segment translator to the
// Postgres one: every operator, on fields, attributes and memberships, plus
// nested groups, selects the same subscribers on both backends.
func TestSubscribersRunSegmentOperators(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo, lists, memberships := b.subscribers(t), b.lists(t), b.memberships(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		listID := addList(t, lists, tenantID, "Newsletter")
		alice := addSubscriberNamed(t, repo, tenantID, "alice@example.com", "Алиса Петрова",
			map[string]any{"plan": "pro", "n": 42, "s42": "42", "vip": true, "a.b": "dot"})
		bob := addSubscriberNamed(t, repo, tenantID, "bob@example.com", "Bob",
			map[string]any{"plan": "free", "n": 7})
		carol := addSubscriberNamed(t, repo, tenantID, "carol@example.com", "Carol", nil)
		require.NoError(t, repo.Update(ctx, tenantID, carol, func(s *domain.Subscriber) (*domain.Subscriber, error) {
			s.Blocklist()
			return s, nil
		}))
		require.NoError(t, memberships.Attach(ctx, tenantID, alice, listID, domain.SubscriptionConfirmed))
		require.NoError(t, memberships.Attach(ctx, tenantID, bob, listID, domain.SubscriptionUnconfirmed))

		const (
			a = "alice@example.com"
			o = "bob@example.com"
			c = "carol@example.com"
		)
		field := func(f string, op domain.SegmentOp, v string) domain.Node {
			return domain.Node{Field: &domain.FieldCondition{Field: f, Op: op, Value: v}}
		}
		attr := func(k string, op domain.SegmentOp, v any) domain.Node {
			return domain.Node{Attr: &domain.AttrCondition{Key: k, Op: op, Value: v}}
		}
		member := func(status string) domain.Node {
			return domain.Node{Member: &domain.MemberCondition{ListID: listID, Status: status}}
		}
		for _, tc := range []struct {
			name string
			node domain.Node
			want []string
		}{
			{"email eq is case-insensitive", field("email", domain.OpEq, "ALICE@Example.com"), []string{a}},
			{"email neq", field("email", domain.OpNeq, "alice@example.com"), []string{o, c}},
			{"name contains Cyrillic", field("name", domain.OpContains, "петров"), []string{a}},
			{"name contains ignores case", field("name", domain.OpContains, "OB"), []string{o}},
			{"state eq", field("state", domain.OpEq, "blocklisted"), []string{c}},
			{"name exists", field("name", domain.OpExists, ""), []string{a, o, c}},
			{"attr exists", attr("plan", domain.OpExists, nil), []string{a, o}},
			{"attr eq", attr("plan", domain.OpEq, "pro"), []string{a}},
			{"attr neq skips a missing key", attr("plan", domain.OpNeq, "pro"), []string{o}},
			{"attr contains ignores case", attr("plan", domain.OpContains, "RO"), []string{a}},
			{"attr gt", attr("n", domain.OpGt, 10), []string{a}},
			{"attr lt", attr("n", domain.OpLt, 10), []string{o}},
			{"attr gte", attr("n", domain.OpGte, 42), []string{a}},
			{"attr lte", attr("n", domain.OpLte, 7), []string{o}},
			{"attr gt on a numeric string", attr("s42", domain.OpGt, "10"), []string{a}},
			{"attr eq on a number", attr("n", domain.OpEq, float64(42)), []string{a}},
			{"attr eq on a boolean", attr("vip", domain.OpEq, true), []string{a}},
			{"attr key with a dot is literal", attr("a.b", domain.OpEq, "dot"), []string{a}},
			{"member any status", member(""), []string{a, o}},
			{"member with status", member(string(domain.SubscriptionConfirmed)), []string{a}},
			{"or group", domain.Node{Conj: domain.ConjOr, Children: []domain.Node{
				attr("plan", domain.OpEq, "free"), member(string(domain.SubscriptionConfirmed)),
			}}, []string{a, o}},
			{"nested groups", domain.Node{Conj: domain.ConjAnd, Children: []domain.Node{
				member(""),
				{Conj: domain.ConjOr, Children: []domain.Node{
					attr("n", domain.OpGt, 10), field("name", domain.OpEq, "Bob"),
				}},
			}}, []string{a, o}},
		} {
			seg := mustSegment(t, tc.node)
			subs, total, err := repo.RunSegment(ctx, tenantID, seg, domain.DefaultPage)
			require.NoError(t, err, tc.name)
			require.Equal(t, tc.want, emails(subs), tc.name)
			require.Equal(t, len(tc.want), total, tc.name)
			count, err := repo.CountSegment(ctx, tenantID, seg)
			require.NoError(t, err, tc.name)
			require.Equal(t, len(tc.want), count, tc.name)
		}
	})
}
```

- [ ] **Step 3: Rewrite the memberships tests as a contract suite**

Run:

```bash
git mv internal/audience/adapters/memberships_pg_test.go internal/audience/adapters/memberships_test.go
```

Replace `internal/audience/adapters/memberships_test.go` with the code below. `TestListsDeleteRemovesMemberships` lives here because it needs all three repositories.

```go
package adapters_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/audience/domain"
)

// seedListAndSubscriber creates one list and one subscriber, returning their
// ids — the pair needed to exercise a membership.
func seedListAndSubscriber(t *testing.T, b backend, tenantID string) (listID, subscriberID string) {
	t.Helper()
	listID = addList(t, b.lists(t), tenantID, "L")
	subscriberID = addSubscriber(t, b.subscribers(t), tenantID, "m@example.com")
	return listID, subscriberID
}

func TestMembershipAttachDetach(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.memberships(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		listID, subscriberID := seedListAndSubscriber(t, b, tenantID)

		require.NoError(t, repo.Attach(ctx, tenantID, subscriberID, listID, domain.SubscriptionUnconfirmed))

		got, err := repo.ForSubscriber(ctx, tenantID, subscriberID)
		require.NoError(t, err)
		require.Len(t, got, 1)
		require.Equal(t, tenantID, got[0].TenantID())
		require.Equal(t, subscriberID, got[0].SubscriberID())
		require.Equal(t, listID, got[0].ListID())
		require.Equal(t, domain.SubscriptionUnconfirmed, got[0].Status())

		require.ErrorIs(t, repo.Attach(ctx, tenantID, subscriberID, listID, domain.SubscriptionConfirmed),
			domain.ErrMembershipExists)
		got, err = repo.ForSubscriber(ctx, tenantID, subscriberID)
		require.NoError(t, err)
		require.Equal(t, domain.SubscriptionUnconfirmed, got[0].Status(), "a rejected attach writes nothing")

		require.NoError(t, repo.Detach(ctx, tenantID, subscriberID, listID))
		got, err = repo.ForSubscriber(ctx, tenantID, subscriberID)
		require.NoError(t, err)
		require.Empty(t, got)
	})
}

func TestMembershipAttachUnknownSubscriberOrListFails(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.memberships(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		listID, subscriberID := seedListAndSubscriber(t, b, tenantID)

		// Postgres rejects both through its foreign keys; YDB checks the rows
		// in the attaching transaction.
		require.Error(t, repo.Attach(ctx, tenantID, uuid.NewString(), listID, domain.SubscriptionConfirmed))
		require.Error(t, repo.Attach(ctx, tenantID, subscriberID, uuid.NewString(), domain.SubscriptionConfirmed))
		got, err := repo.ForSubscriber(ctx, tenantID, subscriberID)
		require.NoError(t, err)
		require.Empty(t, got)
	})
}

func TestMembershipSetStatus(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.memberships(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		listID, subscriberID := seedListAndSubscriber(t, b, tenantID)
		require.NoError(t, repo.Attach(ctx, tenantID, subscriberID, listID, domain.SubscriptionUnconfirmed))

		require.NoError(t, repo.SetStatus(ctx, tenantID, subscriberID, listID, domain.SubscriptionConfirmed))
		got, err := repo.ForSubscriber(ctx, tenantID, subscriberID)
		require.NoError(t, err)
		require.Equal(t, domain.SubscriptionConfirmed, got[0].Status())

		require.Error(t,
			repo.SetStatus(ctx, tenantID, subscriberID, listID, domain.SubscriptionUnconfirmed),
			"an invalid transition is rejected by the domain state machine")
		got, err = repo.ForSubscriber(ctx, tenantID, subscriberID)
		require.NoError(t, err)
		require.Equal(t, domain.SubscriptionConfirmed, got[0].Status())

		require.ErrorIs(t,
			repo.SetStatus(ctx, tenantID, subscriberID, uuid.NewString(), domain.SubscriptionConfirmed),
			domain.ErrMembershipNotFound)
	})
}

func TestMembershipDetachMissing(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.memberships(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		err := repo.Detach(ctx, tenantID, "not-a-uuid", "also-not")
		require.ErrorIs(t, err, domain.ErrMembershipNotFound)
	})
}

func TestMembershipTenantIsolation(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.memberships(t)
		ctx := context.Background()
		tenantA, tenantB := b.newTenant(t), b.newTenant(t)
		listID, subscriberID := seedListAndSubscriber(t, b, tenantA)
		require.NoError(t, repo.Attach(ctx, tenantA, subscriberID, listID, domain.SubscriptionConfirmed))

		got, err := repo.ForSubscriber(ctx, tenantB, subscriberID)
		require.NoError(t, err)
		require.Empty(t, got)
		require.ErrorIs(t, repo.Detach(ctx, tenantB, subscriberID, listID), domain.ErrMembershipNotFound)
		got, err = repo.ForSubscriber(ctx, tenantA, subscriberID)
		require.NoError(t, err)
		require.Len(t, got, 1)
	})
}

func TestListsDeleteRemovesMemberships(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		lists, subs, repo := b.lists(t), b.subscribers(t), b.memberships(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		gone := addList(t, lists, tenantID, "Gone")
		kept := addList(t, lists, tenantID, "Kept")
		subscriberID := addSubscriber(t, subs, tenantID, "m@example.com")
		require.NoError(t, repo.Attach(ctx, tenantID, subscriberID, gone, domain.SubscriptionConfirmed))
		require.NoError(t, repo.Attach(ctx, tenantID, subscriberID, kept, domain.SubscriptionConfirmed))

		require.NoError(t, lists.Delete(ctx, tenantID, gone))

		got, err := repo.ForSubscriber(ctx, tenantID, subscriberID)
		require.NoError(t, err)
		require.Len(t, got, 1, "only the deleted list's membership goes")
		require.Equal(t, kept, got[0].ListID())
		_, total, err := subs.InList(ctx, tenantID, gone, domain.DefaultPage)
		require.NoError(t, err)
		require.Zero(t, total)
	})
}
```

- [ ] **Step 4: Rewrite the preferences tests as a contract suite**

Run:

```bash
git mv internal/audience/adapters/preferences_pg_test.go internal/audience/adapters/preferences_test.go
```

Replace `internal/audience/adapters/preferences_test.go` with the code below. The unknown-list case in `TestInListExcludesUnsubscribed` uses a fresh UUID, not a malformed id. On Postgres, `InList` with a malformed id fails with "commit unexpectedly resulted in rollback" (spec "Plan B5 planning spike"). No caller passes one, and this plan does not change the pg adapter.

```go
package adapters_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	audiencecommand "github.com/nikolaymatrosov/nvelope/internal/audience/app/command"
	audiencequery "github.com/nikolaymatrosov/nvelope/internal/audience/app/query"
	"github.com/nikolaymatrosov/nvelope/internal/audience/domain"
)

func TestUpdatePreferencesAppliesNameAndMemberships(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		lists, subscribers, memberships := b.lists(t), b.subscribers(t), b.memberships(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		news := addList(t, lists, tenantID, "Newsletter")
		announce := addList(t, lists, tenantID, "Announcements")
		subID := addSubscriber(t, subscribers, tenantID, "alice@example.com")
		require.NoError(t, memberships.Attach(ctx, tenantID, subID, news, domain.SubscriptionConfirmed))
		require.NoError(t, memberships.Attach(ctx, tenantID, subID, announce, domain.SubscriptionConfirmed))

		update := audiencecommand.NewUpdatePreferencesHandler(subscribers, memberships)
		require.NoError(t, update.Handle(ctx, audiencecommand.UpdatePreferences{
			TenantID:     tenantID,
			SubscriberID: subID,
			Name:         "Alice A.",
			Lists:        map[string]bool{news: true, announce: false},
		}))

		get := audiencequery.NewGetPreferencesHandler(subscribers, memberships, lists)
		view, err := get.Handle(ctx, audiencequery.GetPreferences{
			TenantID: tenantID, SubscriberID: subID,
		})
		require.NoError(t, err)
		require.Equal(t, "Alice A.", view.Name)
		got := map[string]bool{}
		for _, l := range view.Lists {
			got[l.ListID] = l.Subscribed
		}
		require.True(t, got[news], "Newsletter remains subscribed")
		require.False(t, got[announce], "Announcements is unsubscribed")
	})
}

func TestPublicUnsubscribeAllAndSingleList(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		lists, subscribers, memberships := b.lists(t), b.subscribers(t), b.memberships(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		news := addList(t, lists, tenantID, "Newsletter")
		announce := addList(t, lists, tenantID, "Announcements")

		// Unsubscribing from one list leaves the other confirmed.
		subID := addSubscriber(t, subscribers, tenantID, "alice@example.com")
		require.NoError(t, memberships.Attach(ctx, tenantID, subID, news, domain.SubscriptionConfirmed))
		require.NoError(t, memberships.Attach(ctx, tenantID, subID, announce, domain.SubscriptionConfirmed))

		unsub := audiencecommand.NewPublicUnsubscribeHandler(memberships)
		require.NoError(t, unsub.Handle(ctx, audiencecommand.PublicUnsubscribe{
			TenantID: tenantID, SubscriberID: subID, ListID: news,
		}))
		ms, err := memberships.ForSubscriber(ctx, tenantID, subID)
		require.NoError(t, err)
		statuses := map[string]domain.SubscriptionStatus{}
		for _, m := range ms {
			statuses[m.ListID()] = m.Status()
		}
		require.Equal(t, domain.SubscriptionUnsubscribed, statuses[news])
		require.Equal(t, domain.SubscriptionConfirmed, statuses[announce])

		// Unsubscribing from all moves every membership to unsubscribed.
		other := addSubscriber(t, subscribers, tenantID, "bob@example.com")
		require.NoError(t, memberships.Attach(ctx, tenantID, other, news, domain.SubscriptionConfirmed))
		require.NoError(t, memberships.Attach(ctx, tenantID, other, announce, domain.SubscriptionConfirmed))
		require.NoError(t, unsub.Handle(ctx, audiencecommand.PublicUnsubscribe{
			TenantID: tenantID, SubscriberID: other,
		}))
		ms, err = memberships.ForSubscriber(ctx, tenantID, other)
		require.NoError(t, err)
		require.Len(t, ms, 2)
		for _, m := range ms {
			require.Equal(t, domain.SubscriptionUnsubscribed, m.Status(),
				"every membership unsubscribed when no list is specified")
		}
	})
}

func TestInListExcludesUnsubscribed(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		lists, subscribers, memberships := b.lists(t), b.subscribers(t), b.memberships(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		listID := addList(t, lists, tenantID, "Newsletter")
		active := addSubscriber(t, subscribers, tenantID, "active@example.com")
		pending := addSubscriber(t, subscribers, tenantID, "pending@example.com")
		gone := addSubscriber(t, subscribers, tenantID, "gone@example.com")
		addSubscriber(t, subscribers, tenantID, "outsider@example.com")
		require.NoError(t, memberships.Attach(ctx, tenantID, active, listID, domain.SubscriptionConfirmed))
		require.NoError(t, memberships.Attach(ctx, tenantID, pending, listID, domain.SubscriptionUnconfirmed))
		require.NoError(t, memberships.Attach(ctx, tenantID, gone, listID, domain.SubscriptionConfirmed))
		require.NoError(t, memberships.SetStatus(ctx, tenantID, gone, listID, domain.SubscriptionUnsubscribed))

		got, total, err := subscribers.InList(ctx, tenantID, listID, domain.Page{Limit: 50})
		require.NoError(t, err)
		require.Equal(t, 2, total)
		require.Equal(t, []string{"active@example.com", "pending@example.com"}, emails(got),
			"an unsubscribed member is excluded from the list's recipient set")

		page, total, err := subscribers.InList(ctx, tenantID, listID, domain.Page{Offset: 1, Limit: 1})
		require.NoError(t, err)
		require.Equal(t, 2, total, "the total ignores the page")
		require.Len(t, page, 1)

		_, total, err = subscribers.InList(ctx, tenantID, uuid.NewString(), domain.DefaultPage)
		require.NoError(t, err)
		require.Zero(t, total)
	})
}
```

- [ ] **Step 5: Write the YDB-only list-role cascade test**

Create `internal/audience/adapters/lists_ydb_test.go`. No audience port can observe the iam per-list roles, and Postgres deletes them through its foreign key, so this test runs on YDB only and seeds the rows with raw YQL.

```go
package adapters_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	ydb "github.com/ydb-platform/ydb-go-sdk/v3"
	"github.com/ydb-platform/ydb-go-sdk/v3/query"

	"github.com/nikolaymatrosov/nvelope/internal/audience/adapters"
	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
)

// TestYDBListsDeleteRemovesListRoles covers the stand-in for a Postgres
// cascade that no audience port can observe: the iam per-list role
// assignments on a deleted list. Postgres deletes them through the
// user_list_roles foreign key, so the test runs on YDB only.
func TestYDBListsDeleteRemovesListRoles(t *testing.T) {
	t.Parallel()
	d := dbtest.YDBMigrated(t)
	ctx := context.Background()
	tenantID, userID := uuid.NewString(), uuid.NewString()
	lists := adapters.NewYDBLists(d)
	gone := addList(t, lists, tenantID, "Gone")
	kept := addList(t, lists, tenantID, "Kept")
	for _, listID := range []string{gone, kept} {
		require.NoError(t, d.Query().Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $user_id AS Utf8;
DECLARE $list_id AS Utf8;
DECLARE $role_id AS Utf8;
INSERT INTO user_list_roles (tenant_id, user_id, list_id, role_id)
VALUES ($tenant_id, $user_id, $list_id, $role_id);`,
			query.WithParameters(ydb.ParamsBuilder().
				Param("$tenant_id").Text(tenantID).
				Param("$user_id").Text(userID).
				Param("$list_id").Text(listID).
				Param("$role_id").Text(uuid.NewString()).
				Build())))
	}

	require.NoError(t, lists.Delete(ctx, tenantID, gone))

	rs, err := d.Query().QueryResultSet(ctx, `DECLARE $tenant_id AS Utf8;
SELECT list_id FROM user_list_roles WHERE tenant_id = $tenant_id;`,
		query.WithParameters(ydb.ParamsBuilder().Param("$tenant_id").Text(tenantID).Build()))
	require.NoError(t, err)
	defer func() { _ = rs.Close(ctx) }()
	var left []string
	for row, err := range rs.Rows(ctx) {
		require.NoError(t, err)
		var listID string
		require.NoError(t, row.Scan(&listID))
		left = append(left, listID)
	}
	require.Equal(t, []string{kept}, left, "only the deleted list's role assignments go")
}
```

- [ ] **Step 6: Run the suites to verify they fail**

Run: `go vet ./internal/audience/adapters/`

Expected: build failure `undefined: adapters.NewYDBSubscribers` and `undefined: adapters.NewYDBMemberships`.

- [ ] **Step 7: Implement the YQL segment translator**

Create `internal/audience/adapters/segment_yql.go`. It mirrors `segment_sql.go` node by node. The differences are the ones in the spec's translation rules:
- every operand, the attribute key included, is a `Utf8` parameter `$p1`, `$p2`, ...;
- `JSON_VALUE` / `JSON_EXISTS` with `$.keyvalue()` replace `->>` / `?`;
- `CAST(... AS Double)` replaces `::numeric`;
- `IN (SELECT ...)` replaces the correlated `EXISTS`.

```go
package adapters

import (
	"fmt"
	"strings"

	"github.com/nikolaymatrosov/nvelope/internal/audience/domain"
)

// ydbSegmentArg is one Utf8 parameter of a translated segment.
type ydbSegmentArg struct {
	name, value string
}

// ydbSegmentTranslator turns a validated domain.Segment into a parameterized
// YQL predicate over the subscribers table aliased s. It mirrors
// segmentTranslator (segment_sql.go). Every operand, attribute keys included,
// is bound as a Utf8 parameter: the query is user-authored, so no value is
// ever interpolated into the YQL text.
type ydbSegmentTranslator struct {
	args []ydbSegmentArg
}

// translateSegmentYQL returns the predicate and its parameters, named $p1,
// $p2, ... The predicate also reads $tenant_id, which the caller binds.
func translateSegmentYQL(seg domain.Segment) (string, []ydbSegmentArg) {
	t := &ydbSegmentTranslator{}
	clause := t.node(seg.Root())
	if clause == "" {
		clause = "true"
	}
	return clause, t.args
}

// param records an argument and returns its parameter name.
func (t *ydbSegmentTranslator) param(v string) string {
	name := fmt.Sprintf("$p%d", len(t.args)+1)
	t.args = append(t.args, ydbSegmentArg{name: name, value: v})
	return name
}

// node translates one condition-tree node.
func (t *ydbSegmentTranslator) node(n domain.Node) string {
	if n.Conj != "" || len(n.Children) > 0 {
		parts := make([]string, 0, len(n.Children))
		for _, c := range n.Children {
			parts = append(parts, t.node(c))
		}
		sep := " AND "
		if n.Conj == domain.ConjOr {
			sep = " OR "
		}
		return "(" + strings.Join(parts, sep) + ")"
	}
	switch {
	case n.Field != nil:
		return t.field(*n.Field)
	case n.Attr != nil:
		return t.attr(*n.Attr)
	case n.Member != nil:
		return t.member(*n.Member)
	default:
		return "true"
	}
}

// field translates a subscriber-field condition. The field name is validated
// by the domain, so it is safe to embed.
func (t *ydbSegmentTranslator) field(c domain.FieldCondition) string {
	col := "s." + c.Field
	value := c.Value
	if c.Field == "email" {
		// Postgres compares the citext email case-insensitively. YDB stores
		// it lower-cased (domain.NewSubscriber), so lower the operand.
		value = strings.ToLower(value)
	}
	switch c.Op {
	case domain.OpEq:
		return fmt.Sprintf("%s = %s", col, t.param(value))
	case domain.OpNeq:
		return fmt.Sprintf("%s != %s", col, t.param(value))
	case domain.OpContains:
		return fmt.Sprintf(`%s ILIKE "%%"u || %s || "%%"u`, col, t.param(value))
	case domain.OpExists:
		return fmt.Sprintf(`%s != ""u`, col)
	default:
		return "true"
	}
}

// attr translates a custom-attribute condition over the JsonDocument column.
// The JsonPath picks the member whose name equals the bound key through
// keyvalue(), so the key is never part of the path text. JSON_VALUE returns a
// scalar as text, as Postgres ->> does, and NULL for an object or array.
func (t *ydbSegmentTranslator) attr(c domain.AttrCondition) string {
	path := fmt.Sprintf(`"$.keyvalue() ? (@.name == $k).value" PASSING %s AS k`, t.param(c.Key))
	value := "JSON_VALUE(s.attributes, " + path + ")"
	operand := func() string { return t.param(fmt.Sprintf("%v", c.Value)) }
	switch c.Op {
	case domain.OpExists:
		return "JSON_EXISTS(s.attributes, " + path + ")"
	case domain.OpEq:
		return fmt.Sprintf("%s = %s", value, operand())
	case domain.OpNeq:
		return fmt.Sprintf("%s != %s", value, operand())
	case domain.OpContains:
		return fmt.Sprintf(`%s ILIKE "%%"u || %s || "%%"u`, value, operand())
	case domain.OpGt, domain.OpLt, domain.OpGte, domain.OpLte:
		// A value that is not a number casts to NULL and matches nothing;
		// Postgres fails the whole query instead.
		return fmt.Sprintf("CAST(%s AS Double) %s CAST(%s AS Double)",
			value, sqlComparison(c.Op), operand())
	default:
		return "true"
	}
}

// member translates a list-membership condition.
func (t *ydbSegmentTranslator) member(c domain.MemberCondition) string {
	cond := "tenant_id = $tenant_id AND list_id = " + t.param(c.ListID)
	if c.Status != "" {
		cond += " AND subscription_status = " + t.param(c.Status)
	}
	return "s.id IN (SELECT subscriber_id FROM subscriber_lists VIEW idx_subscriber_lists_list WHERE " +
		cond + ")"
}

// ydbSegmentDeclares returns the DECLARE statements for a segment's
// parameters.
func ydbSegmentDeclares(args []ydbSegmentArg) string {
	var b strings.Builder
	for _, a := range args {
		b.WriteString("DECLARE " + a.name + " AS Utf8;\n")
	}
	return b.String()
}
```

- [ ] **Step 8: Implement `YDBSubscribers`**

Create `internal/audience/adapters/subscribers_ydb.go`:

```go
package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	ydb "github.com/ydb-platform/ydb-go-sdk/v3"
	"github.com/ydb-platform/ydb-go-sdk/v3/query"

	"github.com/nikolaymatrosov/nvelope/internal/audience/domain"
	"github.com/nikolaymatrosov/nvelope/internal/db"
)

// YDBSubscribers is the YDB-backed implementation of
// domain.SubscriberRepository, for the serverless runtime. Attributes are a
// JsonDocument, which segment queries read with JSON_VALUE and JSON_EXISTS.
// YDB has no foreign keys, so Delete removes the subscriber's memberships in
// the same transaction, as the Postgres cascade does.
type YDBSubscribers struct {
	driver *ydb.Driver
}

var _ domain.SubscriberRepository = (*YDBSubscribers)(nil)

// NewYDBSubscribers builds a YDBSubscribers repository over the driver.
func NewYDBSubscribers(d *ydb.Driver) *YDBSubscribers {
	return &YDBSubscribers{driver: d}
}

// ydbSubscriberColumns is the projection scanYDBSubscriber reads, in order.
// Every query aliases the subscribers table as s.
const ydbSubscriberColumns = `s.id, s.tenant_id, s.email, s.name, s.state, s.attributes,
    s.created_at, s.updated_at`

const ydbSelectSubscriber = `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
SELECT ` + ydbSubscriberColumns + `
FROM subscribers AS s
WHERE s.tenant_id = $tenant_id AND s.id = $id;`

// Add persists a new subscriber under a fresh UUID and returns the id. It
// returns domain.ErrSubscriberEmailTaken when the tenant already has that
// email.
func (r *YDBSubscribers) Add(ctx context.Context, tenantID string, s *domain.Subscriber) (string, error) {
	attrs, err := marshalAttributes(s.Attributes())
	if err != nil {
		return "", err
	}
	id := uuid.NewString()
	err = r.driver.Query().Exec(ctx, ydbInsertSubscriber, ydbSubscriberParams(tenantID, id, s, attrs))
	// Only the email index can collide: the id is fresh.
	if db.IsYDBConflict(err) {
		return "", domain.ErrSubscriberEmailTaken
	}
	if err != nil {
		return "", fmt.Errorf("inserting subscriber: %w", err)
	}
	return id, nil
}

const ydbInsertSubscriber = `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DECLARE $email AS Utf8;
DECLARE $name AS Utf8;
DECLARE $state AS Utf8;
DECLARE $attributes AS JsonDocument;
INSERT INTO subscribers (tenant_id, id, email, name, state, attributes, created_at, updated_at)
VALUES ($tenant_id, $id, $email, $name, $state, $attributes,
    CurrentUtcTimestamp(), CurrentUtcTimestamp());`

// ydbSubscriberParams binds every column ydbInsertSubscriber writes.
func ydbSubscriberParams(tenantID, id string, s *domain.Subscriber, attrs []byte) query.ExecuteOption {
	return query.WithParameters(ydb.ParamsBuilder().
		Param("$tenant_id").Text(tenantID).
		Param("$id").Text(id).
		Param("$email").Text(s.Email()).
		Param("$name").Text(s.Name()).
		Param("$state").Text(string(s.State())).
		Param("$attributes").JSONDocumentFromBytes(attrs).
		Build())
}

// Update loads the subscriber, runs fn, and persists its name, state and
// attributes in one serializable transaction. The email is immutable, as on
// Postgres. The SDK retries the transaction on conflict, so fn may run more
// than once and must be retry-safe.
func (r *YDBSubscribers) Update(ctx context.Context, tenantID, id string,
	fn func(*domain.Subscriber) (*domain.Subscriber, error)) error {

	return r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		loaded, err := ydbSubscriberFromRow(tx.QueryRow(ctx, ydbSelectSubscriber, ydbTenantAndID(tenantID, id)))
		if err != nil {
			return err
		}
		updated, err := fn(loaded)
		if err != nil {
			return err
		}
		attrs, err := marshalAttributes(updated.Attributes())
		if err != nil {
			return err
		}
		if err := tx.Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DECLARE $name AS Utf8;
DECLARE $state AS Utf8;
DECLARE $attributes AS JsonDocument;
UPDATE subscribers SET name = $name, state = $state, attributes = $attributes,
    updated_at = CurrentUtcTimestamp()
WHERE tenant_id = $tenant_id AND id = $id;`,
			query.WithParameters(ydb.ParamsBuilder().
				Param("$tenant_id").Text(tenantID).
				Param("$id").Text(id).
				Param("$name").Text(updated.Name()).
				Param("$state").Text(string(updated.State())).
				Param("$attributes").JSONDocumentFromBytes(attrs).
				Build())); err != nil {
			return fmt.Errorf("updating subscriber: %w", err)
		}
		return nil
	})
}

// UpsertByEmail creates the subscriber if its email is new, or updates the
// name and attributes of the existing one, reporting whether a new row was
// created. The existing subscriber's state is kept, as on Postgres, so an
// import never re-enables a blocklisted address. Concurrent upserts of one
// email abort each other, and the SDK retries the loser, which then sees the
// winner's row.
func (r *YDBSubscribers) UpsertByEmail(ctx context.Context, tenantID string,
	s *domain.Subscriber) (bool, error) {

	attrs, err := marshalAttributes(s.Attributes())
	if err != nil {
		return false, err
	}
	newID := uuid.NewString()
	var created bool
	err = r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		row, err := tx.QueryRow(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $email AS Utf8;
SELECT id FROM subscribers VIEW idx_subscribers_email
WHERE tenant_id = $tenant_id AND email = $email;`,
			query.WithParameters(ydb.ParamsBuilder().
				Param("$tenant_id").Text(tenantID).
				Param("$email").Text(s.Email()).
				Build()))
		if errors.Is(err, query.ErrNoRows) {
			if err := tx.Exec(ctx, ydbInsertSubscriber,
				ydbSubscriberParams(tenantID, newID, s, attrs)); err != nil {
				return fmt.Errorf("inserting subscriber: %w", err)
			}
			created = true
			return nil
		}
		if err != nil {
			return fmt.Errorf("looking up subscriber by email: %w", err)
		}
		var id string
		if err := row.Scan(&id); err != nil {
			return fmt.Errorf("scanning subscriber id: %w", err)
		}
		if err := tx.Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DECLARE $name AS Utf8;
DECLARE $attributes AS JsonDocument;
UPDATE subscribers SET name = $name, attributes = $attributes, updated_at = CurrentUtcTimestamp()
WHERE tenant_id = $tenant_id AND id = $id;`,
			query.WithParameters(ydb.ParamsBuilder().
				Param("$tenant_id").Text(tenantID).
				Param("$id").Text(id).
				Param("$name").Text(s.Name()).
				Param("$attributes").JSONDocumentFromBytes(attrs).
				Build())); err != nil {
			return fmt.Errorf("updating subscriber: %w", err)
		}
		created = false
		return nil
	})
	if err != nil {
		return false, err
	}
	return created, nil
}

// Delete removes the subscriber and, in the same transaction, its
// memberships, as the Postgres cascade does. It returns
// domain.ErrSubscriberNotFound when absent.
func (r *YDBSubscribers) Delete(ctx context.Context, tenantID, id string) error {
	return r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		_, err := tx.QueryRow(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DELETE FROM subscribers
WHERE tenant_id = $tenant_id AND id = $id
RETURNING id;`, ydbTenantAndID(tenantID, id))
		if errors.Is(err, query.ErrNoRows) {
			return domain.ErrSubscriberNotFound
		}
		if err != nil {
			return fmt.Errorf("deleting subscriber: %w", err)
		}
		if err := tx.Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DELETE FROM subscriber_lists
WHERE tenant_id = $tenant_id AND subscriber_id = $id;`, ydbTenantAndID(tenantID, id)); err != nil {
			return fmt.Errorf("deleting subscriber memberships: %w", err)
		}
		return nil
	})
}

// Get returns the subscriber, or domain.ErrSubscriberNotFound.
func (r *YDBSubscribers) Get(ctx context.Context, tenantID, id string) (*domain.Subscriber, error) {
	return ydbSubscriberFromRow(r.driver.Query().QueryRow(ctx, ydbSelectSubscriber,
		ydbTenantAndID(tenantID, id)))
}

// Search returns a page of subscribers whose email or name contains q, case
// insensitively, and the total count. An empty q matches all subscribers.
func (r *YDBSubscribers) Search(ctx context.Context, tenantID, q string,
	page domain.Page) ([]*domain.Subscriber, int, error) {

	page = page.Normalize()
	pattern := "%" + strings.ToLower(strings.TrimSpace(q)) + "%"
	const where = `WHERE s.tenant_id = $tenant_id
    AND ($pattern = "%%"u OR s.email ILIKE $pattern OR s.name ILIKE $pattern)`
	params := func(withPage bool) query.ExecuteOption {
		b := ydb.ParamsBuilder().
			Param("$tenant_id").Text(tenantID).
			Param("$pattern").Text(pattern)
		if withPage {
			b = b.Param("$limit").Uint64(uint64(page.Limit)).
				Param("$offset").Uint64(uint64(page.Offset))
		}
		return query.WithParameters(b.Build())
	}
	return r.page(ctx, "searching subscribers",
		`DECLARE $tenant_id AS Utf8;
DECLARE $pattern AS Utf8;
SELECT COUNT(*) FROM subscribers AS s
`+where+`;`, params(false),
		`DECLARE $tenant_id AS Utf8;
DECLARE $pattern AS Utf8;
DECLARE $limit AS Uint64;
DECLARE $offset AS Uint64;
SELECT `+ydbSubscriberColumns+`
FROM subscribers AS s
`+where+`
ORDER BY s.created_at DESC
LIMIT $limit OFFSET $offset;`, params(true))
}

// InList returns a page of the subscribers attached to one list and the
// total count. An unsubscribed membership is excluded: a subscriber who has
// left a list is no longer a recipient of, nor exported as a member of, it.
func (r *YDBSubscribers) InList(ctx context.Context, tenantID, listID string,
	page domain.Page) ([]*domain.Subscriber, int, error) {

	page = page.Normalize()
	const from = `FROM subscriber_lists VIEW idx_subscriber_lists_list AS sl
JOIN subscribers AS s ON s.tenant_id = sl.tenant_id AND s.id = sl.subscriber_id
WHERE sl.tenant_id = $tenant_id AND sl.list_id = $list_id
    AND sl.subscription_status != "unsubscribed"u`
	params := func(withPage bool) query.ExecuteOption {
		b := ydb.ParamsBuilder().
			Param("$tenant_id").Text(tenantID).
			Param("$list_id").Text(listID)
		if withPage {
			b = b.Param("$limit").Uint64(uint64(page.Limit)).
				Param("$offset").Uint64(uint64(page.Offset))
		}
		return query.WithParameters(b.Build())
	}
	return r.page(ctx, "listing list subscribers",
		`DECLARE $tenant_id AS Utf8;
DECLARE $list_id AS Utf8;
SELECT COUNT(*) `+from+`;`, params(false),
		`DECLARE $tenant_id AS Utf8;
DECLARE $list_id AS Utf8;
DECLARE $limit AS Uint64;
DECLARE $offset AS Uint64;
SELECT `+ydbSubscriberColumns+`
`+from+`
ORDER BY s.created_at DESC
LIMIT $limit OFFSET $offset;`, params(true))
}

// RunSegment translates a validated Segment to YQL and returns a page of the
// matching subscribers and the total count.
func (r *YDBSubscribers) RunSegment(ctx context.Context, tenantID string, seg domain.Segment,
	page domain.Page) ([]*domain.Subscriber, int, error) {

	page = page.Normalize()
	where, args := translateSegmentYQL(seg)
	declares := ydbSegmentDeclares(args)
	return r.page(ctx, "running segment",
		`DECLARE $tenant_id AS Utf8;
`+declares+`SELECT COUNT(*) FROM subscribers AS s
WHERE s.tenant_id = $tenant_id AND (`+where+`);`, ydbSegmentParams(tenantID, args, nil),
		`DECLARE $tenant_id AS Utf8;
DECLARE $limit AS Uint64;
DECLARE $offset AS Uint64;
`+declares+`SELECT `+ydbSubscriberColumns+`
FROM subscribers AS s
WHERE s.tenant_id = $tenant_id AND (`+where+`)
ORDER BY s.created_at DESC
LIMIT $limit OFFSET $offset;`, ydbSegmentParams(tenantID, args, &page))
}

// CountSegment returns only the count of subscribers matching a Segment.
func (r *YDBSubscribers) CountSegment(ctx context.Context, tenantID string, seg domain.Segment) (int, error) {
	where, args := translateSegmentYQL(seg)
	row, err := r.driver.Query().QueryRow(ctx, `DECLARE $tenant_id AS Utf8;
`+ydbSegmentDeclares(args)+`SELECT COUNT(*) FROM subscribers AS s
WHERE s.tenant_id = $tenant_id AND (`+where+`);`, ydbSegmentParams(tenantID, args, nil))
	if err != nil {
		return 0, fmt.Errorf("counting segment: %w", err)
	}
	var n uint64
	if err := row.Scan(&n); err != nil {
		return 0, fmt.Errorf("counting segment: %w", err)
	}
	return int(n), nil
}

// ydbSegmentParams binds $tenant_id, the segment's parameters and, for a page
// query, $limit and $offset.
func ydbSegmentParams(tenantID string, args []ydbSegmentArg, page *domain.Page) query.ExecuteOption {
	b := ydb.ParamsBuilder().Param("$tenant_id").Text(tenantID)
	for _, a := range args {
		b = b.Param(a.name).Text(a.value)
	}
	if page != nil {
		b = b.Param("$limit").Uint64(uint64(page.Limit)).
			Param("$offset").Uint64(uint64(page.Offset))
	}
	return query.WithParameters(b.Build())
}

// page runs a COUNT(*) query and a page query in one transaction and returns
// the page and the total.
// ponytail: COUNT(*) plus OFFSET reads every matching row (spec "Ceilings");
// keyset paging if a tenant outgrows it.
func (r *YDBSubscribers) page(ctx context.Context, what string,
	countQuery string, countParams query.ExecuteOption,
	pageQuery string, pageParams query.ExecuteOption) ([]*domain.Subscriber, int, error) {

	var subs []*domain.Subscriber
	var total int
	err := r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		count, err := ydbCount(ctx, tx, countQuery, countParams)
		if err != nil {
			return fmt.Errorf("%s: %w", what, err)
		}
		rs, err := tx.QueryResultSet(ctx, pageQuery, pageParams)
		if err != nil {
			return fmt.Errorf("%s: %w", what, err)
		}
		// Collected into a local and assigned at the end: the SDK may re-run
		// the closure, and a retry must not append to rows a failed run read.
		var local []*domain.Subscriber
		for row, err := range rs.Rows(ctx) {
			if err != nil {
				return fmt.Errorf("%s: %w", what, err)
			}
			s, err := scanYDBSubscriber(row)
			if err != nil {
				return err
			}
			local = append(local, s)
		}
		subs, total = local, count
		return nil
	})
	if err != nil {
		return nil, 0, err
	}
	return subs, total, nil
}

// ydbSubscriberFromRow maps a single-row read to the subscriber, translating
// "no row" into domain.ErrSubscriberNotFound.
func ydbSubscriberFromRow(row query.Row, err error) (*domain.Subscriber, error) {
	if errors.Is(err, query.ErrNoRows) {
		return nil, domain.ErrSubscriberNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("loading subscriber: %w", err)
	}
	return scanYDBSubscriber(row)
}

// scanYDBSubscriber reads one row in ydbSubscriberColumns order.
func scanYDBSubscriber(row query.Row) (*domain.Subscriber, error) {
	var id, tenantID, email, name, state, rawAttrs string
	var createdAt, updatedAt time.Time
	if err := row.Scan(&id, &tenantID, &email, &name, &state, &rawAttrs,
		&createdAt, &updatedAt); err != nil {
		return nil, fmt.Errorf("scanning subscriber: %w", err)
	}
	raw := map[string]any{}
	if err := json.Unmarshal([]byte(rawAttrs), &raw); err != nil {
		return nil, fmt.Errorf("decoding subscriber attributes: %w", err)
	}
	return domain.HydrateSubscriber(id, tenantID, email, name, domain.State(state),
		domain.HydrateAttributes(raw), createdAt, updatedAt), nil
}
```

- [ ] **Step 9: Implement `YDBMemberships`**

Create `internal/audience/adapters/memberships_ydb.go`:

```go
package adapters

import (
	"context"
	"errors"
	"fmt"
	"time"

	ydb "github.com/ydb-platform/ydb-go-sdk/v3"
	"github.com/ydb-platform/ydb-go-sdk/v3/query"

	"github.com/nikolaymatrosov/nvelope/internal/audience/domain"
	"github.com/nikolaymatrosov/nvelope/internal/db"
)

// YDBMemberships is the YDB-backed implementation of
// domain.MembershipRepository, for the serverless runtime. YDB has no foreign
// keys, so Attach reads the subscriber and the list in the transaction that
// writes the membership. That stands in for the Postgres foreign keys, and
// makes a concurrent delete of either abort one of the two transactions.
type YDBMemberships struct {
	driver *ydb.Driver
}

var _ domain.MembershipRepository = (*YDBMemberships)(nil)

// NewYDBMemberships builds a YDBMemberships repository over the driver.
func NewYDBMemberships(d *ydb.Driver) *YDBMemberships {
	return &YDBMemberships{driver: d}
}

// ydbMembershipColumns is the projection scanYDBMembership reads, in order.
const ydbMembershipColumns = `tenant_id, subscriber_id, list_id, subscription_status, created_at, updated_at`

// Attach links a subscriber to a list with the given status. It returns
// domain.ErrSubscriberNotFound or domain.ErrListNotFound when the tenant has
// no such subscriber or list, and domain.ErrMembershipExists when the
// membership is already there.
func (r *YDBMemberships) Attach(ctx context.Context, tenantID, subscriberID, listID string,
	status domain.SubscriptionStatus) error {

	err := r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		if err := ydbRowExists(ctx, tx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
SELECT id FROM subscribers WHERE tenant_id = $tenant_id AND id = $id;`,
			tenantID, subscriberID, domain.ErrSubscriberNotFound); err != nil {
			return err
		}
		if err := ydbRowExists(ctx, tx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
SELECT id FROM lists WHERE tenant_id = $tenant_id AND id = $id;`,
			tenantID, listID, domain.ErrListNotFound); err != nil {
			return err
		}
		err := tx.Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $subscriber_id AS Utf8;
DECLARE $list_id AS Utf8;
DECLARE $subscription_status AS Utf8;
INSERT INTO subscriber_lists
    (tenant_id, subscriber_id, list_id, subscription_status, created_at, updated_at)
VALUES ($tenant_id, $subscriber_id, $list_id, $subscription_status,
    CurrentUtcTimestamp(), CurrentUtcTimestamp());`,
			query.WithParameters(ydb.ParamsBuilder().
				Param("$tenant_id").Text(tenantID).
				Param("$subscriber_id").Text(subscriberID).
				Param("$list_id").Text(listID).
				Param("$subscription_status").Text(string(status)).
				Build()))
		// The primary key collides at the statement (spec "Plan B2 planning
		// spike"), and the transaction is rolled back.
		if db.IsYDBConflict(err) {
			return domain.ErrMembershipExists
		}
		if err != nil {
			return fmt.Errorf("attaching membership: %w", err)
		}
		return nil
	})
	// This covers a YDB that reports the collision at commit.
	if db.IsYDBConflict(err) {
		return domain.ErrMembershipExists
	}
	return err
}

// Detach removes a membership. It returns domain.ErrMembershipNotFound when
// absent.
func (r *YDBMemberships) Detach(ctx context.Context, tenantID, subscriberID, listID string) error {
	_, err := r.driver.Query().QueryRow(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $subscriber_id AS Utf8;
DECLARE $list_id AS Utf8;
DELETE FROM subscriber_lists
WHERE tenant_id = $tenant_id AND subscriber_id = $subscriber_id AND list_id = $list_id
RETURNING list_id;`, ydbMembershipKey(tenantID, subscriberID, listID))
	if errors.Is(err, query.ErrNoRows) {
		return domain.ErrMembershipNotFound
	}
	if err != nil {
		return fmt.Errorf("detaching membership: %w", err)
	}
	return nil
}

// SetStatus changes a membership's subscription status through the domain
// state machine, in one serializable transaction. It returns
// domain.ErrMembershipNotFound when absent.
func (r *YDBMemberships) SetStatus(ctx context.Context, tenantID, subscriberID, listID string,
	status domain.SubscriptionStatus) error {

	return r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		row, err := tx.QueryRow(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $subscriber_id AS Utf8;
DECLARE $list_id AS Utf8;
SELECT `+ydbMembershipColumns+`
FROM subscriber_lists
WHERE tenant_id = $tenant_id AND subscriber_id = $subscriber_id AND list_id = $list_id;`,
			ydbMembershipKey(tenantID, subscriberID, listID))
		if errors.Is(err, query.ErrNoRows) {
			return domain.ErrMembershipNotFound
		}
		if err != nil {
			return fmt.Errorf("loading membership: %w", err)
		}
		m, err := scanYDBMembership(row)
		if err != nil {
			return err
		}
		if err := m.ChangeStatus(status); err != nil {
			return err
		}
		if err := tx.Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $subscriber_id AS Utf8;
DECLARE $list_id AS Utf8;
DECLARE $subscription_status AS Utf8;
UPDATE subscriber_lists SET subscription_status = $subscription_status,
    updated_at = CurrentUtcTimestamp()
WHERE tenant_id = $tenant_id AND subscriber_id = $subscriber_id AND list_id = $list_id;`,
			query.WithParameters(ydb.ParamsBuilder().
				Param("$tenant_id").Text(tenantID).
				Param("$subscriber_id").Text(subscriberID).
				Param("$list_id").Text(listID).
				Param("$subscription_status").Text(string(m.Status())).
				Build())); err != nil {
			return fmt.Errorf("updating membership: %w", err)
		}
		return nil
	})
}

// ForSubscriber returns every membership of one subscriber, oldest first.
func (r *YDBMemberships) ForSubscriber(ctx context.Context, tenantID, subscriberID string) ([]*domain.Membership, error) {
	rs, err := r.driver.Query().QueryResultSet(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
SELECT `+ydbMembershipColumns+`
FROM subscriber_lists
WHERE tenant_id = $tenant_id AND subscriber_id = $id
ORDER BY created_at;`, ydbTenantAndID(tenantID, subscriberID))
	if err != nil {
		return nil, fmt.Errorf("listing memberships: %w", err)
	}
	defer func() { _ = rs.Close(ctx) }()
	var out []*domain.Membership
	for row, err := range rs.Rows(ctx) {
		if err != nil {
			return nil, fmt.Errorf("listing memberships: %w", err)
		}
		m, err := scanYDBMembership(row)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, nil
}

// ydbRowExists runs a key lookup bound to ($tenant_id, $id) and returns
// notFound when it finds no row.
func ydbRowExists(ctx context.Context, tx query.TxActor, q, tenantID, id string, notFound error) error {
	_, err := tx.QueryRow(ctx, q, ydbTenantAndID(tenantID, id))
	if errors.Is(err, query.ErrNoRows) {
		return notFound
	}
	if err != nil {
		return fmt.Errorf("checking %s: %w", id, err)
	}
	return nil
}

// ydbMembershipKey binds the (tenant_id, subscriber_id, list_id) primary key.
func ydbMembershipKey(tenantID, subscriberID, listID string) query.ExecuteOption {
	return query.WithParameters(ydb.ParamsBuilder().
		Param("$tenant_id").Text(tenantID).
		Param("$subscriber_id").Text(subscriberID).
		Param("$list_id").Text(listID).
		Build())
}

// scanYDBMembership reads one row in ydbMembershipColumns order.
func scanYDBMembership(row query.Row) (*domain.Membership, error) {
	var tenantID, subscriberID, listID, status string
	var createdAt, updatedAt time.Time
	if err := row.Scan(&tenantID, &subscriberID, &listID, &status, &createdAt, &updatedAt); err != nil {
		return nil, fmt.Errorf("scanning membership: %w", err)
	}
	return domain.HydrateMembership(tenantID, subscriberID, listID,
		domain.SubscriptionStatus(status), createdAt, updatedAt), nil
}
```

- [ ] **Step 10: Run the suites to verify they pass**

Run: `export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock" && go test ./internal/audience/adapters/ -count=1 -v -timeout 8m 2>&1 | grep -E -- '--- (PASS|FAIL)|^(ok|FAIL)' | grep -E 'FAIL|/pg|/ydb|^ok|TestYDB' | sort`

Expected:
- 76 `--- PASS` lines ending in `/pg` or `/ydb`, 38 of each: Task 1's 15 tests and this task's 23 (14 `TestSubscribers*`, 5 `TestMembership*`, `TestListsDeleteRemovesMemberships`, `TestUpdatePreferencesAppliesNameAndMemberships`, `TestPublicUnsubscribeAllAndSingleList`, `TestInListExcludesUnsubscribed`).
- One `--- PASS: TestYDBListsDeleteRemovesListRoles` line.
- No `FAIL` line. The pg-only jobs and opt-in suites still run.
- The package is `ok`.

Run: `export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock" && go test ./internal/audience/adapters/ -run 'TestSubscribersUpsertByEmailIsRaceFree|TestSubscribersRunSegmentOperators' -count=10 -timeout 8m`

Expected: `ok`. Ten runs prove that the race test is not passing by luck.

Three cases are not yours to fix. Report BLOCKED with the verbatim output:
- **A `/pg` subtest fails.** In particular, `TestSubscribersSearch/pg` or `TestSubscribersRunSegmentOperators/pg` failing on a Cyrillic case means the test Postgres does not fold non-ASCII case. The contract would then be wrong for SaaS, not the YDB adapter.
- **`TestSubscribersRunSegmentOperators/ydb` fails on an attribute case.** It contradicts the spec's "Plan B5 planning spike" findings on `JSON_VALUE` with `keyvalue()`, or on `CAST`.
- **`TestSubscribersUpsertByEmailIsRaceFree/ydb` fails with a conflict error** (`PRECONDITION_FAILED`, `db.IsYDBConflict` true). The spike saw only SDK-retried aborts. Print the error with `%+v` in the report. Do not add a retry loop without a decision.

Any other `/ydb` failure: fix only `segment_yql.go`, `subscribers_ydb.go` or `memberships_ydb.go`, and record the verbatim error and the fix in the report.

- [ ] **Step 11: Full verification and commit**

Run: `export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock" && go build ./... && go vet ./... && go test ./... -timeout 15m 2>&1 | grep -vE '^(ok|\?)\s'`

Expected: no output. In other words, no failing package.

Run: `golangci-lint run`

Expected: `0 issues.`

Run: `PATH="$HOME/go/bin:$PATH" make lint-arch`

Expected: the last line is `[arch] clean`.

Run: `git diff --stat HEAD -- go.mod go.sum`

Expected: no output.

Run: `ls internal/audience/adapters/*_pg_test.go`

Expected: exactly `internal/audience/adapters/jobs_pg_test.go` and `internal/audience/adapters/optin_pg_test.go`. Plan B6 converts them.

```bash
gofmt -l internal/audience/
git add internal/audience/adapters/segment_yql.go internal/audience/adapters/subscribers_ydb.go internal/audience/adapters/memberships_ydb.go
git add internal/audience/adapters/helpers_test.go internal/audience/adapters/subscribers_test.go internal/audience/adapters/memberships_test.go
git add internal/audience/adapters/preferences_test.go internal/audience/adapters/lists_ydb_test.go
git commit -m "feat(audience): add the YDB subscriber and membership repositories and the YQL segment translator under pg+ydb contract suites"
```

`gofmt -l` must print nothing. Afterwards, `git status --short` must print nothing.

---

## Done criteria for Plan B5

- `go build ./... && go vet ./... && go test ./...` passes. Every eachBackend test in `internal/audience/adapters` passes on both `/pg` and `/ydb`, and `TestYDBListsDeleteRemovesListRoles` passes.
- `golangci-lint run` reports `0 issues.`, and `make lint-arch` reports `[arch] clean`.
- No `go.mod` or `go.sum` change.
- The only `*_pg_test.go` files left in `internal/audience/adapters` are `jobs_pg_test.go` and `optin_pg_test.go`.
- These tests pass on both backends, so YDB reproduces the Postgres behaviour that has no YDB construct:
  - `TestSubscribersDuplicateEmailConflicts`: `citext` uniqueness, without an `email_key` column.
  - `TestListsUpdateOntoTakenNameConflicts`: the unique list name on rename.
  - `TestSubscribersDeleteCascadesMemberships` and `TestListsDeleteRemovesMemberships`: the foreign-key cascades.
  - `TestMembershipAttachUnknownSubscriberOrListFails`: the membership's foreign keys.
  - `TestSubscribersRunSegmentOperators`: `jsonb` `?` and `->>`, `::numeric`, `citext` equality and Cyrillic `ILIKE`.
  - `TestSubscribersUpsertByEmailIsRaceFree`: the atomicity of `ON CONFLICT DO UPDATE`.
- The final report lists what stays open for the next plans:
  - **Plan C wiring.** The serverless composition root wires `NewYDBLists`, `NewYDBSubscribers`, `NewYDBMemberships` and `NewYDBFields` in place of the pg constructors at `internal/service/application.go:682-688`, and `NewYDBFields` in place of `audienceadapters.NewFields` at `application.go:446` (the merge-tag field provider).
  - **Plan B6 (audience: jobs, subscription pages, pending subscriptions).**
    - Extend `backend` with `jobs`, `pages` and `pendingSubscriptions`, and convert `jobs_pg_test.go` and `optin_pg_test.go`.
    - `subscription_pages.sending_domain_id` references `sending_domains` (`ON DELETE RESTRICT`) on Postgres. On YDB the page suite uses a bare UUID for it, as this plan does for tenants. Nothing on YDB stops a sending-domain delete while a page uses the domain.
    - `subscription_pages.target_list_ids` and `pending_subscriptions.target_list_ids` (`uuid[]`) become JSON arrays in `Utf8` columns, through `ydbStrings` / `ydbStringsFrom`.
    - `PendingSubscriptions.Upsert` (`ON CONFLICT (tenant_id, email, subscription_page_id) DO UPDATE ... RETURNING id`) becomes SELECT through a unique index, then UPDATE or INSERT in one `DoTx`, as `YDBSubscribers.UpsertByEmail` does. Its `confirmation_token_hash` lookup uses a global unique index, as iam sessions do.
    - `import_export_jobs.file_bytes` (`bytea`) is a nullable `String` column. The import upload cap (`maxUploadBytes`, 32 MB, `internal/api/audience_api.go:30`) is under the 50 MB row limit (V5).
    - The River-driven worker tests stay pg-only. A YDB worker test can call `Work(ctx, args)` directly.
  - **Campaign plan.** Postgres cascades a subscriber delete into `campaign_recipients` and a list delete into `campaign_lists`. When those tables land on YDB, either extend `YDBSubscribers.Delete` and `YDBLists.Delete`, or make their readers tolerate a dangling id.
  - **iam follow-up.** `YDBRoles.AssignListRole` (`internal/iam/adapters/roles_ydb.go:210-212`) does not check the list: B4 accepted that because `lists` was not on YDB yet. Now it is. Postgres rejects an assignment to an unknown list through the `user_list_roles` foreign key; on YDB the assignment succeeds, and so does one that commits after the list's delete. The dangling row keeps its role in use, so `YDBRoles.Delete` returns `ErrRoleInUse` until someone removes the assignment by the dead list id. `AssignListRole` should read `lists` by `(tenant_id, id)` inside its `DoTx` and fail when the list is missing, as `YDBMemberships.Attach` does.
    - Update: fixed in `5fadd55`; `AssignListRole` now reads `lists` and returns `ErrListNotFound`.
  - **Accepted YDB/pg differences.** No contract test pins these:
    - Attaching an unknown subscriber or list: pg returns a wrapped foreign-key error; YDB returns `ErrSubscriberNotFound` or `ErrListNotFound`.
    - Attaching another tenant's subscriber or list: pg accepts it, because foreign-key checks bypass RLS. YDB returns not-found.
    - A numeric segment comparison on a value that is not a number: pg fails the whole query; YDB matches nothing. Numbers compare as `Double` on YDB and as `numeric` on pg.
    - An attribute `eq`, `neq` or `contains` against an object or array value: pg compares its JSON text; YDB's `JSON_VALUE` is NULL, so it matches nothing.
    - `YDBLists.All` orders by `name` by bytes, and pg by the database collation.
    - `Reorder` with a malformed id: pg returns an invalid-input error; YDB returns `ErrFieldNotFound`.
    - A segment membership condition with a malformed list id: pg fails the query (invalid uuid input); YDB matches nothing.
    - `InList` with a malformed list id: pg fails with "commit unexpectedly resulted in rollback" (spec "Plan B5 planning spike"); YDB returns an empty page.
    - `Search` and `contains`: Postgres `LIKE` / `ILIKE` treat `\` as an escape character; YQL `ILIKE` has no default escape character.
    - YDB accepts values the Postgres `CHECK` constraints forbid (`lists.visibility`, `lists.optin`, `subscribers.state`, `subscriber_lists.subscription_status`, `subscriber_fields.type`, the slug pattern and the display-name length). None is reachable through the domain constructors.
  - **Ceilings.** These are marked `ponytail:` in code and listed under the spec's "Ceilings":
    - a segment's YQL text grows with its condition tree, up to the 10 KB query limit;
    - `YDBLists.Delete` removes every membership of the list in one transaction;
    - `YDBLists.All`, `Search`, `InList` and `RunSegment` page with `COUNT(*)` plus `OFFSET`.
  - **Found while planning, not fixed here.** Two callers find a subscriber's id by email through `Search`, which matches a substring (`%email%`) and returns the newest match: `ConfirmSubscriptionHandler.resolveID` (`internal/audience/app/command/confirm_subscription.go:101`) and `ImportWorker.attachToLists` (`internal/audience/adapters/import_worker.go:97`). For `a@example.com`, a newer `data@example.com` is returned instead, so a double-opt-in confirmation or an import attaches the wrong subscriber. This happens on both backends.
    - Update: fixed in `643e3e6`; `resolveID` is gone and both callers use `GetByEmail` (exact match).
  - **Found in the final review, not fixed here.** Both segment translators build an attribute operand with `fmt.Sprintf("%v", c.Value)`. A JSON number of 1,000,000 or more decodes to a `float64` and formats as `1e+06`, while Postgres `->>` and YDB `JSON_VALUE` read the stored number back as `1000000`, so `seats eq 1000000` never matches on either backend. `strconv.FormatFloat(v, 'f', -1, 64)` for `float64` operands fixes both.
    - Update: fixed in `4370199`; `segmentOperand` formats `float64` with `strconv.FormatFloat(v, 'f', -1, 64)`.
