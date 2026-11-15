# Serverless On-Prem — Plan B6 (audience jobs, subscription pages, pending subscriptions on YDB) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:**
- Port the rest of the `audience` context to YDB: `YDBSubscriptionPages`, `YDBPendingSubscriptions` and `YDBJobs` behind the existing domain ports, held to the same contract tests as their Postgres counterparts.
- Fix three bugs the B5 reviews found, on both backends: the subscriber lookup by email in double-opt-in confirmation and import, the formatting of large numbers in segment conditions, and the missing list check in `YDBRoles.AssignListRole`.

**Architecture:**
- **Migration.** One file, `internal/db/ydbmigrations/000007_audience_optin_jobs.up.sql`, creates `subscription_pages`, `pending_subscriptions` and `import_export_jobs`. Every one is tenant-scoped.
- **Adapters.** The YDB adapters sit next to the pg adapters in `internal/audience/adapters` and call `db.Query()` / `DoTx` directly, like the B5 adapters. They reuse the B5 helpers (`ydbTenantParam`, `ydbTenantAndID`, `ydbStrings`, `ydbStringsFrom`) and the pg adapter's `marshalAttributes`, `importParams`, `jobRow`, `importJobFromRow` and `exportJobFromRow`.
- **Job files in Object Storage.** A YDB cell holds at most 16 MiB, and an import upload may be 32 MB (spec "Plan B6 planning spike"). `YDBJobs` keeps the job row in YDB and the file as a private object `jobs/<tenant_id>/<job_id>` in the media bucket, written before the row that references it. The port does not change.
- **Contract tests.** `jobs_pg_test.go`, `optin_pg_test.go` and `optin_worker_test.go` become pg+ydb contract suites through the B5 `eachBackend` harness, which gains `pages`, `pending`, `jobs` and `newSendingDomain`. The worker tests call `Work(ctx, args)` directly; River dispatch stays covered by `internal/api`.
- **Test infrastructure.** `internal/dbtest/s3.go` serves the shared fakecloud S3 container (`nvelope-test-s3-strict`) to every package. The media blob-store test moves onto it.

**Tech Stack:**
- Go 1.26
- `github.com/ydb-platform/ydb-go-sdk/v3` v3.136.4 (query service)
- `github.com/aws/aws-sdk-go-v2/service/s3` (already in `go.mod`, used by media)
- testcontainers-go, local-ydb `26.3.1.23`, fakecloud `0.50.0`
- testify, `github.com/google/uuid`

**Design spec:** [docs/superpowers/specs/2026-11-14-serverless-onprem-design.md](../specs/2026-11-14-serverless-onprem-design.md).
- Binding sections:
  - "Storage layer (YDB)": Transactions; Schema and migrations; SQL translation rules, including the `bytea` row this plan adds.
  - "Spike findings (Plan A)": V5 and V6.
  - "Plan B2 planning spike" to "Plan B6 planning spike".
  - "Ceilings".
- The B6 spike found the two YDB limits this plan designs around: a 16 MiB cell and a transaction write buffer under 64 MiB. Every other YQL construct this plan uses was verified by B1–B5.
- **Dry run.** During planning, every code block in this plan was applied task by task to the worktree, then reverted.
  - After each task, `go vet`, `golangci-lint run` and that task's pg+ydb suites passed.
  - Each fix in Tasks 3–5 was sabotage-checked: with the fix reverted, its new test fails on both backends (Task 5: on `/ydb`).
  - `TestPendingSubscriptionsUpsertIsRaceFree` and `TestSubscribersUpsertByEmailIsRaceFree` passed twenty repeated runs (`-count=10`, twice).
  - On the final state:
    - `go build ./... && go vet ./... && go test ./...` had no failing package;
    - `golangci-lint run` reported `0 issues.`;
    - `make lint-arch` reported `[arch] clean`;
    - `go.mod` and `go.sum` were unchanged.

**Scope:**
- This is the sixth Plan B plan and the second of two for audience. The port order continues with campaign, deliverability and media.
- Not in scope:
  - Wiring the YDB adapters and the S3 client into the composition root. That is Plan C4 (see "Done criteria").
  - The submission throttle. `ydbthrottle.Keyed` already matches `domain.SubmissionThrottle`; C4 wires it.
  - The workers' logic. `import_worker.go` changes only in Task 3's lookup.
  - The pg adapters. They change only in Task 2's two helper extractions (no behaviour change) and Task 3's new `GetByEmail`.

## Global Constraints

- **Branch.**
  - Work on `feat/serverless-onprem` in the worktree `/Users/nikthespirit/experiments/nvelope-wt/serverless-onprem`. Both already exist, and the branch starts at `main` (`6e714f1`) plus the commit that adds this plan.
  - Do not create branches or worktrees.
  - The main checkout belongs to other sessions.
  - Never use bare `git stash` or `git stash pop`; the stash is shared.
- **Docker.**
  - Docker on this machine is OrbStack. Prefix every `go test` / `make test` / `make ci` with `export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock" && `.
  - Ryuk reaps test containers shortly after a run ends. A run that starts within that window reattaches to the previous run's containers.
  - The shared containers `nvelope-test-pg`, `nvelope-test-ydb`, `nvelope-test-s3-strict` and `nvelope-test-sqs` may belong to another session's live run. Never `docker rm` them.
- **YDB container.** It is `ydbplatform/local-ydb:26.3.1.23`, amd64 under Rosetta. A fresh container can take about 45 s to start, so use `-timeout 8m` on runs that touch YDB, and `-timeout 15m` for the full suite.
- **Known environmental flakes. None of these is caused by this plan:**
  - `internal/platform/ratelimit` `TestAllowIsConsistentUnderConcurrency` can fail with `redis eval failed: i/o timeout` under full-suite load. Re-run that package alone and record both runs.
  - pg-backed packages can all fail at once with `no migration found for version N`, where N is above `24`, or with `unexpected postmaster exit`. That means another worktree's run is using the shared `nvelope-test-pg`. Re-run once it is gone.
  - `internal/api` `TestPublicSubscriptionCrossTenantIsolation` can fail on a reused pg database (a fixed `hash-a` token). Re-run once.
  - A YDB-backed test can fail with `No such container: <id>`, hang until the timeout, or exit with status 1 and no test failure, when it starts while Ryuk is still reaping the previous run's container. Re-run once.
- **Dependencies.** No new Go modules and no `go.mod` change.
- **SaaS behaviour.** Postgres adapters, River, Redis and every existing test keep their behaviour, except for the three bug fixes in Tasks 3–5, which change behaviour on both backends on purpose. `go test ./...` passes after every task.
- **YQL conventions** (spec "SQL translation rules" and V4):
  - Every query `DECLARE`s each parameter: `DECLARE $id AS Utf8;`. A nullable column's parameter is `Optional<...>` and is bound with `BeginOptional()...EndOptional()` from a pointer.
  - Literals that target a `Utf8` column take the `u` suffix (`"[]"u`); a bare `"..."` is `String`. An `Int64` literal takes the `l` suffix (`0l`).
  - Every DDL statement is idempotent (`CREATE TABLE IF NOT EXISTS`).
  - A statement ends on a line ending in `;`. Never end a line with `;` inside a string literal or a `--` comment.
  - A secondary index is read only through an explicit `VIEW <index_name>`.
- **Tenant scoping.** All three tables are tenant-scoped. Their primary key starts with `tenant_id`, and every query filters by the `tenantID` the port receives. Object keys start with `jobs/<tenant_id>/`.
- **IDs** are UUIDv4 strings generated in Go (`uuid.NewString()`) before any `DoTx`, and stored as `Utf8`.
- **No column defaults.** The YDB columns have no `DEFAULT`. Every `INSERT` writes every `NOT NULL` column: a new job's counts are `0l` and its failures `"[]"u`, as the Postgres defaults are. Only `created_at` and `updated_at` come from `CurrentUtcTimestamp()`.
- **JSON in `Utf8` columns.** `target_list_ids` (both tables) are written by `ydbStrings` and read by `ydbStringsFrom`. `subscription_pages.fields`, `pending_subscriptions.attributes`, `import_export_jobs.params` and `failures` are JSON only stored and read back whole, so they are `Utf8`, not `JsonDocument`.
- **Case-insensitive matching.** `subscription_pages.slug`, `pending_subscriptions.email` and `subscribers.email` are lower-cased by their domain constructors, so plain unique indexes reproduce the Postgres uniqueness. `YDBSubscribers.GetByEmail` lower-cases its argument, as the Postgres `citext` comparison ignores case.
- **Foreign keys.** YDB has none. `YDBRoles.AssignListRole` reads `lists` in its `DoTx` (Task 5). The page and sending-domain foreign keys need no stand-in (spec "Plan B6 planning spike").
- **Object Storage** (`YDBJobs`):
  - The object is written before the row that references it. When the row write fails, the object is deleted best-effort.
  - Objects are private: `PutObject` sets no ACL.
  - S3 calls never run inside a `DoTx` closure.
- **`DoTx` closures must be retry-safe** (V6):
  - only database operations inside, plus the port's `fn` callbacks;
  - ids, encoded JSON and domain values are built before `DoTx`;
  - values the caller reads are assigned at the end of the closure;
  - never `append` to a slice, or write to a map, declared outside the closure.
- **Duplicate key mapping.**
  - "Already exists" comes from `db.IsYDBConflict(err)`. Never match on the message text.
  - Inside a `DoTx`, check it on the statement and also on the error `DoTx` returns, for a YDB that reports the conflict at commit.
  - A page slug collision maps to `domain.ErrSubscriptionPageSlugTaken`, as on Postgres.
  - Never add `query.WithIdempotent()` to an insert path.
- **Commits.** Use conventional messages. **No `Co-Authored-By` trailer** of any kind.
- **Tooling.**
  - `goimports` is not installed, so use `gofmt -w`.
  - `golangci-lint` is on PATH. It lints test files too.
  - Run arch lint as `PATH="$HOME/go/bin:$PATH" make lint-arch`, with PATH set inline. A worktree guard refuses `export PATH` combined with git commands, and it refuses complex compound git commands. Run git commands one per line, as the commit steps show.

## File structure

| File | Responsibility |
|------|----------------|
| `internal/db/ydbmigrations/000007_audience_optin_jobs.up.sql` | the three tables |
| `internal/audience/adapters/subscription_pages_ydb.go` | `YDBSubscriptionPages` |
| `internal/audience/adapters/pending_subscriptions_ydb.go` | `YDBPendingSubscriptions` |
| `internal/audience/adapters/jobs_ydb.go` | `YDBJobs`: job rows in YDB, job files in Object Storage |
| `internal/audience/adapters/jobs_pg.go` | gains `exportParamsJSON` and `jobSummaryFromRow`, shared with `YDBJobs` |
| `internal/dbtest/s3.go` | `S3Endpoint`, `S3Client` and the test bucket constants over the shared fakecloud S3 container |
| `internal/audience/adapters/helpers_test.go` | the harness gains `pages`, `pending`, `jobs` and `newSendingDomain` |
| `internal/audience/adapters/optin_test.go` | was `optin_pg_test.go`; page, pending and confirmation contract suites |
| `internal/audience/adapters/optin_worker_test.go` | the opt-in worker on both backends |
| `internal/audience/adapters/jobs_test.go` | was `jobs_pg_test.go`; job contract suite, `TestYDBJobsFilesArePrivate`, the import and export workers on both backends |
| `internal/media/adapters/blobstore_s3_test.go` | uses `dbtest.S3Endpoint` instead of its own container code |
| `internal/audience/domain/repository.go`, `subscribers_pg.go`, `subscribers_ydb.go`, the two `fakeSubscribers`, `confirm_subscription.go`, `import_worker.go` | `SubscriberRepository.GetByEmail` and its two callers (Task 3) |
| `internal/audience/adapters/segment_sql.go`, `segment_yql.go` | `segmentOperand` (Task 4) |
| `internal/iam/domain/errors.go`, `internal/iam/adapters/roles_ydb.go`, iam `helpers_test.go`, `roles_test.go` | `ErrListNotFound` and the list check (Task 5) |

---

### Task 1: Subscription pages and pending subscriptions on YDB

**Files:**
- Create: `internal/db/ydbmigrations/000007_audience_optin_jobs.up.sql`
- Create: `internal/audience/adapters/subscription_pages_ydb.go`
- Create: `internal/audience/adapters/pending_subscriptions_ydb.go`
- Modify: `internal/audience/adapters/helpers_test.go`
- Rename: `internal/audience/adapters/optin_pg_test.go` → `internal/audience/adapters/optin_test.go` (rewritten)
- Modify: `internal/audience/adapters/optin_worker_test.go`

**Interfaces:**
- Consumes:
  - The ports `domain.SubscriptionPageRepository` (`internal/audience/domain/subscription_page.go`) and `domain.PendingSubscriptionRepository` (`internal/audience/domain/pending_subscription.go`), unchanged.
  - `domain.HydrateSubscriptionPage`, `domain.HydratePendingSubscription`, `domain.HydrateAttributes`, unchanged.
  - From B5 (`lists_ydb.go`): `ydbTenantParam(tenantID string) query.ExecuteOption`, `ydbTenantAndID(tenantID, id string) query.ExecuteOption`, `ydbStrings([]string) string`, `ydbStringsFrom(string) ([]string, error)`.
  - From the pg adapters: `marshalAttributes(a domain.Attributes) ([]byte, error)` (`subscribers_pg.go`).
  - From the B5 harness: `eachBackend`, `backend`, `addList`, `addSubscriber`, `seedTenant`.
- Produces:
  ```go
  // package adapters
  func NewYDBSubscriptionPages(d *ydb.Driver) *YDBSubscriptionPages
  func NewYDBPendingSubscriptions(d *ydb.Driver) *YDBPendingSubscriptions
  // package adapters_test
  type backend struct {
      // B5 fields, plus:
      pages            func(t *testing.T) domain.SubscriptionPageRepository
      pending          func(t *testing.T) domain.PendingSubscriptionRepository
      newSendingDomain func(t *testing.T, tenantID string) string
  }
  func addSubscriptionPage(t *testing.T, b backend, tenantID, slug string) (pageID, listID string)
  func newPending(t *testing.T, tenantID, pageID, email, listID, rawToken string, expiresAt time.Time) *domain.PendingSubscription
  ```

- [ ] **Step 1: Write the migration**

Create `internal/db/ydbmigrations/000007_audience_optin_jobs.up.sql`. It creates all three tables; Task 2 uses `import_export_jobs`. The runner embeds every `*.up.sql` file, so nothing else registers it.

```sql
-- Audience public subscription and bulk jobs (internal/audience): the public
-- subscription pages, the double-opt-in pending subscriptions they create, and
-- the import/export job records. Every table is tenant-scoped: its primary key
-- starts with tenant_id and every query filters by it. No column has a
-- DEFAULT, so the adapters write the values Postgres takes from its column
-- defaults. YDB has no foreign keys and no array type: list ids are a JSON
-- array in a Utf8 column.

-- slug is stored lower-cased (domain.NewSubscriptionPage lower-cases it), so
-- the unique index matches the Postgres UNIQUE (tenant_id, slug). fields is a
-- JSON array that is only stored and read back whole.
CREATE TABLE IF NOT EXISTS subscription_pages (
    tenant_id         Utf8      NOT NULL,
    id                Utf8      NOT NULL,
    slug              Utf8      NOT NULL,
    title             Utf8      NOT NULL,
    target_list_ids   Utf8      NOT NULL,
    fields            Utf8      NOT NULL,
    sending_domain_id Utf8      NOT NULL,
    from_name         Utf8      NOT NULL,
    from_local_part   Utf8      NOT NULL,
    active            Bool      NOT NULL,
    created_at        Timestamp NOT NULL,
    updated_at        Timestamp NOT NULL,
    PRIMARY KEY (tenant_id, id),
    INDEX idx_subscription_pages_slug GLOBAL UNIQUE SYNC ON (tenant_id, slug)
);

-- email is stored lower-cased (domain.NewPendingSubscription lower-cases it),
-- so the unique index reproduces the Postgres citext UNIQUE (tenant_id, email,
-- subscription_page_id) that Upsert targets. The token index serves the
-- confirmation-link lookup. attributes is only stored and read back whole.
CREATE TABLE IF NOT EXISTS pending_subscriptions (
    tenant_id               Utf8      NOT NULL,
    id                      Utf8      NOT NULL,
    subscription_page_id    Utf8      NOT NULL,
    email                   Utf8      NOT NULL,
    attributes              Utf8      NOT NULL,
    target_list_ids         Utf8      NOT NULL,
    confirmation_token_hash Utf8      NOT NULL,
    expires_at              Timestamp NOT NULL,
    created_at              Timestamp NOT NULL,
    PRIMARY KEY (tenant_id, id),
    INDEX idx_pending_subscriptions_email GLOBAL UNIQUE SYNC ON (tenant_id, email, subscription_page_id),
    INDEX idx_pending_subscriptions_token GLOBAL UNIQUE SYNC ON (tenant_id, confirmation_token_hash)
);

-- The staged upload (import) or generated file (export) lives in Object
-- Storage, not in the row: a YDB cell holds at most 16 MiB and an upload may
-- be 32 MB. file_key is NULL until the job has a file. params and failures are
-- JSON only stored and read back whole.
CREATE TABLE IF NOT EXISTS import_export_jobs (
    tenant_id     Utf8      NOT NULL,
    id            Utf8      NOT NULL,
    kind          Utf8      NOT NULL,
    requested_by  Utf8      NOT NULL,
    status        Utf8      NOT NULL,
    params        Utf8      NOT NULL,
    file_name     Utf8      NOT NULL,
    file_key      Utf8,
    created_count Int64     NOT NULL,
    updated_count Int64     NOT NULL,
    failed_count  Int64     NOT NULL,
    row_count     Int64     NOT NULL,
    failures      Utf8      NOT NULL,
    created_at    Timestamp NOT NULL,
    started_at    Timestamp,
    finished_at   Timestamp,
    PRIMARY KEY (tenant_id, id)
);
```

- [ ] **Step 2: Extend the harness**

Replace `internal/audience/adapters/helpers_test.go` with the code below. Only `pages`, `pending`, `newSendingDomain` and their constructors per backend are new. `seedSendingDomain` already exists in the old `optin_pg_test.go` and stays in `optin_test.go`.

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
	pages       func(t *testing.T) domain.SubscriptionPageRepository
	pending     func(t *testing.T) domain.PendingSubscriptionRepository
	// newTenant returns the id of a tenant row, which the tenant context
	// owns. On Postgres the audience tables carry a foreign key to it, so the
	// pg backend inserts a real row. YDB has no foreign keys and no audience
	// query reads the tenants table, so a bare UUID stands in.
	newTenant func(t *testing.T) string
	// newSendingDomain returns the id of a sending domain of the tenant,
	// which the sending context owns. On Postgres a subscription page carries
	// a foreign key to it, so the pg backend inserts a real row. No audience
	// query reads sending_domains, so a bare UUID stands in on YDB.
	newSendingDomain func(t *testing.T, tenantID string) string
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
		pages: func(t *testing.T) domain.SubscriptionPageRepository {
			return adapters.NewSubscriptionPages(dbtest.AppPool(t))
		},
		pending: func(t *testing.T) domain.PendingSubscriptionRepository {
			return adapters.NewPendingSubscriptions(dbtest.AppPool(t))
		},
		newTenant: func(t *testing.T) string { return seedTenant(t, dbtest.AppPool(t)) },
		newSendingDomain: func(t *testing.T, tenantID string) string {
			return seedSendingDomain(t, dbtest.AppPool(t), tenantID)
		},
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
		pages: func(t *testing.T) domain.SubscriptionPageRepository {
			return adapters.NewYDBSubscriptionPages(dbtest.YDBMigrated(t))
		},
		pending: func(t *testing.T) domain.PendingSubscriptionRepository {
			return adapters.NewYDBPendingSubscriptions(dbtest.YDBMigrated(t))
		},
		newTenant:        func(*testing.T) string { return uuid.NewString() },
		newSendingDomain: func(*testing.T, string) string { return uuid.NewString() },
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

- [ ] **Step 3: Convert the suites**

Rename the opt-in suite, keeping its history:

```bash
git mv internal/audience/adapters/optin_pg_test.go internal/audience/adapters/optin_test.go
```

Replace `internal/audience/adapters/optin_test.go` with the code below. Every test now runs on both backends. Beyond the old pg tests it pins: every page field round-trips; an `Update` onto a taken slug conflicts and changes nothing; `All` is newest first; unknown ids are not found; pages are tenant-scoped; a refreshing upsert retires the old token; `RefreshToken` on an unknown id; and concurrent upserts of one key return one id.

```go
package adapters_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	audiencecommand "github.com/nikolaymatrosov/nvelope/internal/audience/app/command"
	"github.com/nikolaymatrosov/nvelope/internal/audience/domain"
	"github.com/nikolaymatrosov/nvelope/internal/token"
)

// fakeSuppression is a test double for domain.SuppressionLookup: it reports the
// addresses it was built with as suppressed.
type fakeSuppression struct {
	suppressed map[string]string
}

func (f fakeSuppression) Suppressed(_ context.Context, _ string, emails []string) (map[string]string, error) {
	out := map[string]string{}
	for _, e := range emails {
		if reason, ok := f.suppressed[e]; ok {
			out[e] = reason
		}
	}
	return out, nil
}

// seedSendingDomain inserts a verified sending domain for the tenant so a
// subscription page can satisfy its Postgres foreign key.
func seedSendingDomain(t *testing.T, pool *pgxpool.Pool, tenantID string) string {
	t.Helper()
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback(ctx) }()
	_, err = tx.Exec(ctx, "SELECT set_config('app.tenant_id', $1, true)", tenantID)
	require.NoError(t, err)
	var id string
	require.NoError(t, tx.QueryRow(ctx,
		`INSERT INTO sending_domains (tenant_id, domain, status)
		 VALUES ($1, 'mail.example.com', 'verified') RETURNING id`, tenantID).Scan(&id))
	require.NoError(t, tx.Commit(ctx))
	return id
}

func newSubscriptionPage(t *testing.T, tenantID, slug, listID, domainID string) *domain.SubscriptionPage {
	t.Helper()
	p, err := domain.NewSubscriptionPage(tenantID, slug, "Join us", []string{listID},
		[]domain.FormField{{Key: "name", Label: "Name", Required: false}},
		domainID, "Newsletter", "hello")
	require.NoError(t, err)
	return p
}

// addSubscriptionPage persists a page with the given slug, targeting one new
// list, and returns the page id and the list id.
func addSubscriptionPage(t *testing.T, b backend, tenantID, slug string) (pageID, listID string) {
	t.Helper()
	listID = addList(t, b.lists(t), tenantID, "List "+slug)
	pageID, err := b.pages(t).Add(context.Background(), tenantID,
		newSubscriptionPage(t, tenantID, slug, listID, b.newSendingDomain(t, tenantID)))
	require.NoError(t, err)
	return pageID, listID
}

// newPending builds a pending subscription of email to the page, targeting
// listID, confirmed by rawToken.
func newPending(t *testing.T, tenantID, pageID, email, listID, rawToken string,
	expiresAt time.Time) *domain.PendingSubscription {
	t.Helper()
	p, err := domain.NewPendingSubscription(tenantID, pageID, email,
		domain.HydrateAttributes(map[string]any{"name": "Pat"}), []string{listID},
		token.Hash(rawToken), expiresAt)
	require.NoError(t, err)
	return p
}

func TestSubscriptionPagesAddGetUpdate(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.pages(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		listID := addList(t, b.lists(t), tenantID, "Newsletter")
		domainID := b.newSendingDomain(t, tenantID)

		id, err := repo.Add(ctx, tenantID, newSubscriptionPage(t, tenantID, "join", listID, domainID))
		require.NoError(t, err)
		require.NotEmpty(t, id)

		got, err := repo.GetBySlug(ctx, tenantID, "join")
		require.NoError(t, err)
		require.Equal(t, id, got.ID())
		require.Equal(t, "Join us", got.Title())
		require.Equal(t, []string{listID}, got.TargetListIDs())
		require.Equal(t, []domain.FormField{{Key: "name", Label: "Name"}}, got.Fields())
		require.Equal(t, domainID, got.SendingDomainID())
		require.Equal(t, "hello", got.FromLocalPart())
		require.True(t, got.Active())

		// A duplicate slug within the tenant is a conflict.
		_, err = repo.Add(ctx, tenantID, newSubscriptionPage(t, tenantID, "join", listID, domainID))
		require.ErrorIs(t, err, domain.ErrSubscriptionPageSlugTaken)

		// Update deactivates the page.
		require.NoError(t, repo.Update(ctx, tenantID, id,
			func(p *domain.SubscriptionPage) (*domain.SubscriptionPage, error) {
				return p, p.Reconfigure("join", "Join us", []string{listID}, p.Fields(),
					domainID, "Newsletter", "hello", false)
			}))
		got, err = repo.Get(ctx, tenantID, id)
		require.NoError(t, err)
		require.False(t, got.Active())

		// Renaming another page onto the taken slug is a conflict too.
		otherID, err := repo.Add(ctx, tenantID, newSubscriptionPage(t, tenantID, "other", listID, domainID))
		require.NoError(t, err)
		err = repo.Update(ctx, tenantID, otherID,
			func(p *domain.SubscriptionPage) (*domain.SubscriptionPage, error) {
				return p, p.Reconfigure("join", "Join us", []string{listID}, p.Fields(),
					domainID, "Newsletter", "hello", true)
			})
		require.ErrorIs(t, err, domain.ErrSubscriptionPageSlugTaken)

		all, err := repo.All(ctx, tenantID)
		require.NoError(t, err)
		require.Len(t, all, 2)
		require.Equal(t, otherID, all[0].ID(), "newest first")
		require.Equal(t, "other", all[0].Slug(), "the failed rename changed nothing")

		_, err = repo.Get(ctx, tenantID, "00000000-0000-0000-0000-000000000000")
		require.ErrorIs(t, err, domain.ErrSubscriptionPageNotFound)
		_, err = repo.GetBySlug(ctx, tenantID, "missing")
		require.ErrorIs(t, err, domain.ErrSubscriptionPageNotFound)
		err = repo.Update(ctx, tenantID, "00000000-0000-0000-0000-000000000000",
			func(p *domain.SubscriptionPage) (*domain.SubscriptionPage, error) { return p, nil })
		require.ErrorIs(t, err, domain.ErrSubscriptionPageNotFound)
	})
}

func TestSubscriptionPagesAreTenantScoped(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.pages(t)
		ctx := context.Background()
		tenantA, tenantB := b.newTenant(t), b.newTenant(t)
		idA, _ := addSubscriptionPage(t, b, tenantA, "join")

		// Another tenant may use the same slug and sees nothing of tenant A.
		addSubscriptionPage(t, b, tenantB, "join")
		_, err := repo.Get(ctx, tenantB, idA)
		require.ErrorIs(t, err, domain.ErrSubscriptionPageNotFound)
		gotB, err := repo.GetBySlug(ctx, tenantB, "join")
		require.NoError(t, err)
		require.NotEqual(t, idA, gotB.ID())
		all, err := repo.All(ctx, tenantB)
		require.NoError(t, err)
		require.Len(t, all, 1)
	})
}

func TestPendingSubscriptionsUpsertGetRefreshDelete(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.pending(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		pageID, listID := addSubscriptionPage(t, b, tenantID, "join")

		rawToken, _ := token.New()
		expires := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
		id, err := repo.Upsert(ctx, tenantID,
			newPending(t, tenantID, pageID, "P@Example.com", listID, rawToken, expires))
		require.NoError(t, err)

		got, err := repo.GetByTokenHash(ctx, tenantID, token.Hash(rawToken))
		require.NoError(t, err)
		require.Equal(t, id, got.ID())
		require.Equal(t, "p@example.com", got.Email())
		require.Equal(t, pageID, got.SubscriptionPageID())
		require.Equal(t, []string{listID}, got.TargetListIDs())
		require.Equal(t, map[string]any{"name": "Pat"}, got.Attributes().Values())
		require.True(t, expires.Equal(got.ExpiresAt()))

		// Upserting the same address/page refreshes the existing row rather
		// than stacking a duplicate, and the old token stops resolving.
		newRaw, _ := token.New()
		id2, err := repo.Upsert(ctx, tenantID,
			newPending(t, tenantID, pageID, "p@example.com", listID, newRaw, expires))
		require.NoError(t, err)
		require.Equal(t, id, id2, "a repeat submission refreshes the same pending row")
		_, err = repo.GetByTokenHash(ctx, tenantID, token.Hash(rawToken))
		require.ErrorIs(t, err, domain.ErrPendingSubscriptionNotFound)

		// RefreshToken swaps the token; the old hash no longer resolves.
		rotated, _ := token.New()
		require.NoError(t, repo.RefreshToken(ctx, tenantID, id, token.Hash(rotated),
			time.Now().Add(2*time.Hour)))
		_, err = repo.GetByTokenHash(ctx, tenantID, token.Hash(newRaw))
		require.ErrorIs(t, err, domain.ErrPendingSubscriptionNotFound)
		_, err = repo.GetByTokenHash(ctx, tenantID, token.Hash(rotated))
		require.NoError(t, err)
		require.ErrorIs(t, repo.RefreshToken(ctx, tenantID, "00000000-0000-0000-0000-000000000000",
			token.Hash(rotated), time.Now()), domain.ErrPendingSubscriptionNotFound)

		require.NoError(t, repo.Delete(ctx, tenantID, id))
		_, err = repo.Get(ctx, tenantID, id)
		require.ErrorIs(t, err, domain.ErrPendingSubscriptionNotFound)
		// Deleting a missing row is not an error — confirmation is idempotent.
		require.NoError(t, repo.Delete(ctx, tenantID, id))
	})
}

// TestPendingSubscriptionsUpsertIsRaceFree pins the atomicity of Postgres
// ON CONFLICT DO UPDATE: concurrent submissions of one address to one page
// (a double click) all return the one pending row.
func TestPendingSubscriptionsUpsertIsRaceFree(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.pending(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		pageID, listID := addSubscriptionPage(t, b, tenantID, "join")

		const n = 8
		subs := make([]*domain.PendingSubscription, n)
		for i := range n {
			raw, _ := token.New()
			subs[i] = newPending(t, tenantID, pageID, "race@example.com", listID, raw, time.Now().Add(time.Hour))
		}
		ids := make([]string, n)
		errs := make([]error, n)
		var wg sync.WaitGroup
		for i := range n {
			wg.Go(func() { ids[i], errs[i] = repo.Upsert(ctx, tenantID, subs[i]) })
		}
		wg.Wait()
		for i := range n {
			require.NoError(t, errs[i])
			require.Equal(t, ids[0], ids[i], "every upsert returns the one pending row")
		}
	})
}

func TestConfirmSubscriptionLifecycle(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		ctx := context.Background()
		tenantID := b.newTenant(t)
		pageID, listID := addSubscriptionPage(t, b, tenantID, "join")
		pending, subscribers, memberships := b.pending(t), b.subscribers(t), b.memberships(t)

		confirm := audiencecommand.NewConfirmSubscriptionHandler(pending, subscribers, memberships,
			fakeSuppression{})

		// Submit → pending row, then confirm → subscriber with a confirmed membership.
		rawToken, _ := token.New()
		ps, err := domain.NewPendingSubscription(tenantID, pageID, "alice@example.com",
			domain.HydrateAttributes(map[string]any{"name": "Alice"}), []string{listID},
			token.Hash(rawToken), time.Now().Add(time.Hour))
		require.NoError(t, err)
		_, err = pending.Upsert(ctx, tenantID, ps)
		require.NoError(t, err)

		res, err := confirm.Handle(ctx, audiencecommand.ConfirmSubscription{TenantID: tenantID, Token: rawToken})
		require.NoError(t, err)
		require.False(t, res.AlreadyConfirmed)

		subs, total, err := subscribers.Search(ctx, tenantID, "alice@example.com", domain.Page{Limit: 1})
		require.NoError(t, err)
		require.Equal(t, 1, total)
		require.Equal(t, "Alice", subs[0].Name())
		mships, err := memberships.ForSubscriber(ctx, tenantID, subs[0].ID())
		require.NoError(t, err)
		require.Len(t, mships, 1)
		require.Equal(t, domain.SubscriptionConfirmed, mships[0].Status())

		// The pending row is consumed; a repeat visit is benign.
		res, err = confirm.Handle(ctx, audiencecommand.ConfirmSubscription{TenantID: tenantID, Token: rawToken})
		require.NoError(t, err)
		require.True(t, res.AlreadyConfirmed)
	})
}

func TestConfirmSubscriptionRejectsExpiredAndSuppressed(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		ctx := context.Background()
		tenantID := b.newTenant(t)
		pageID, listID := addSubscriptionPage(t, b, tenantID, "join")
		pending := b.pending(t)

		// An expired confirmation link is rejected.
		expiredRaw, _ := token.New()
		_, err := pending.Upsert(ctx, tenantID, newPending(t, tenantID, pageID, "old@example.com",
			listID, expiredRaw, time.Now().Add(-time.Hour)))
		require.NoError(t, err)
		confirm := audiencecommand.NewConfirmSubscriptionHandler(pending,
			b.subscribers(t), b.memberships(t), fakeSuppression{})
		_, err = confirm.Handle(ctx, audiencecommand.ConfirmSubscription{TenantID: tenantID, Token: expiredRaw})
		require.ErrorIs(t, err, domain.ErrConfirmationExpired)

		// A suppressed address is not silently re-subscribed.
		suppRaw, _ := token.New()
		_, err = pending.Upsert(ctx, tenantID, newPending(t, tenantID, pageID, "blocked@example.com",
			listID, suppRaw, time.Now().Add(time.Hour)))
		require.NoError(t, err)
		confirmSupp := audiencecommand.NewConfirmSubscriptionHandler(pending,
			b.subscribers(t), b.memberships(t),
			fakeSuppression{suppressed: map[string]string{"blocked@example.com": "complaint"}})
		_, err = confirmSupp.Handle(ctx, audiencecommand.ConfirmSubscription{TenantID: tenantID, Token: suppRaw})
		require.ErrorIs(t, err, domain.ErrAddressSuppressed)
	})
}
```

Replace the two test functions in `internal/audience/adapters/optin_worker_test.go`, and drop its `dbtest` import. `recordingMailer` and `stubSendingDomainResolver` above them stay as they are. The file becomes:

```go
package adapters_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/audience/adapters"
	"github.com/nikolaymatrosov/nvelope/internal/audience/domain"
	"github.com/nikolaymatrosov/nvelope/internal/platform/jobs"
	"github.com/nikolaymatrosov/nvelope/internal/token"
)

// recordingMailer is a test double for domain.ConfirmationMailer.
type recordingMailer struct {
	last domain.ConfirmationEmail
	sent int
}

func (m *recordingMailer) Send(_ context.Context, msg domain.ConfirmationEmail) error {
	m.last = msg
	m.sent++
	return nil
}

// stubSendingDomainResolver is a test double for domain.SendingDomainResolver
// that returns one fixed sending domain regardless of the id.
type stubSendingDomainResolver struct {
	info domain.SendingDomainInfo
}

func (s stubSendingDomainResolver) Resolve(context.Context, string, string) (
	domain.SendingDomainInfo, error) {
	return s.info, nil
}

func TestOptinWorkerSendsConfirmationEmail(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		ctx := context.Background()
		tenantID := b.newTenant(t)
		pageID, listID := addSubscriptionPage(t, b, tenantID, "join")
		pages, pending := b.pages(t), b.pending(t)

		rawToken, _ := token.New()
		ps, err := domain.NewPendingSubscription(tenantID, pageID, "alice@example.com",
			domain.HydrateAttributes(nil), []string{listID}, token.Hash(rawToken),
			time.Now().Add(time.Hour))
		require.NoError(t, err)
		pendingID, err := pending.Upsert(ctx, tenantID, ps)
		require.NoError(t, err)

		mailer := &recordingMailer{}
		resolver := stubSendingDomainResolver{
			info: domain.SendingDomainInfo{Domain: "mail.example.com", Verified: true},
		}
		worker := adapters.NewOptinWorker(pending, pages, resolver, mailer, "https://pages.example.com")

		require.NoError(t, worker.Work(ctx, jobs.OptinSendArgs{
			TenantID:              tenantID,
			TenantSlug:            "acme",
			PendingSubscriptionID: pendingID,
			ConfirmationToken:     rawToken,
		}))

		require.Equal(t, 1, mailer.sent)
		require.Equal(t, "alice@example.com", mailer.last.To)
		require.Equal(t, "hello@mail.example.com", mailer.last.FromAddress)
		require.Contains(t, mailer.last.HTMLBody,
			"https://pages.example.com/t/acme/confirm/"+rawToken)
		require.True(t, strings.Contains(mailer.last.TextBody, rawToken))
	})
}

func TestOptinWorkerIgnoresMissingPending(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		mailer := &recordingMailer{}
		worker := adapters.NewOptinWorker(b.pending(t), b.pages(t), stubSendingDomainResolver{},
			mailer, "https://pages.example.com")

		// A confirmed-and-deleted pending subscription must not fail a redelivery.
		require.NoError(t, worker.Work(context.Background(), jobs.OptinSendArgs{
			TenantID:              b.newTenant(t),
			PendingSubscriptionID: "00000000-0000-0000-0000-000000000000",
			ConfirmationToken:     "irrelevant",
		}))
		require.Equal(t, 0, mailer.sent)
	})
}
```

- [ ] **Step 4: Run the suites to verify they fail**

Run: `go vet ./internal/audience/adapters/`

Expected: FAIL with `undefined: adapters.NewYDBSubscriptionPages` and `undefined: adapters.NewYDBPendingSubscriptions`.

- [ ] **Step 5: Write the subscription page repository**

Create `internal/audience/adapters/subscription_pages_ydb.go`:

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

// YDBSubscriptionPages is the YDB-backed implementation of
// domain.SubscriptionPageRepository, for the serverless runtime. The target
// list ids and the form fields are JSON in Utf8 columns.
type YDBSubscriptionPages struct {
	driver *ydb.Driver
}

var _ domain.SubscriptionPageRepository = (*YDBSubscriptionPages)(nil)

// NewYDBSubscriptionPages builds a YDBSubscriptionPages repository over the
// driver.
func NewYDBSubscriptionPages(d *ydb.Driver) *YDBSubscriptionPages {
	return &YDBSubscriptionPages{driver: d}
}

// ydbSubscriptionPageColumns is the projection scanYDBSubscriptionPage reads,
// in order.
const ydbSubscriptionPageColumns = `id, tenant_id, slug, title, target_list_ids, fields,
    sending_domain_id, from_name, from_local_part, active, created_at, updated_at`

const ydbSelectSubscriptionPage = `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
SELECT ` + ydbSubscriptionPageColumns + `
FROM subscription_pages
WHERE tenant_id = $tenant_id AND id = $id;`

// ydbSubscriptionPageWrite declares the parameters Add and Update bind from a
// page, in ydbSubscriptionPageParams order.
const ydbSubscriptionPageWrite = `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DECLARE $slug AS Utf8;
DECLARE $title AS Utf8;
DECLARE $target_list_ids AS Utf8;
DECLARE $fields AS Utf8;
DECLARE $sending_domain_id AS Utf8;
DECLARE $from_name AS Utf8;
DECLARE $from_local_part AS Utf8;
DECLARE $active AS Bool;
`

// Add persists a new page under a fresh UUID and returns the id. It returns
// domain.ErrSubscriptionPageSlugTaken when the tenant already uses the slug.
func (r *YDBSubscriptionPages) Add(ctx context.Context, tenantID string,
	p *domain.SubscriptionPage) (string, error) {

	id := uuid.NewString()
	params, err := ydbSubscriptionPageParams(tenantID, id, p)
	if err != nil {
		return "", err
	}
	err = r.driver.Query().Exec(ctx, ydbSubscriptionPageWrite+`INSERT INTO subscription_pages
    (tenant_id, id, slug, title, target_list_ids, fields, sending_domain_id,
     from_name, from_local_part, active, created_at, updated_at)
VALUES ($tenant_id, $id, $slug, $title, $target_list_ids, $fields, $sending_domain_id,
    $from_name, $from_local_part, $active, CurrentUtcTimestamp(), CurrentUtcTimestamp());`,
		params)
	// Only the slug index can collide: the id is fresh.
	if db.IsYDBConflict(err) {
		return "", domain.ErrSubscriptionPageSlugTaken
	}
	if err != nil {
		return "", fmt.Errorf("inserting subscription page: %w", err)
	}
	return id, nil
}

// Update loads the page, runs fn, and persists the result in one serializable
// transaction. It returns domain.ErrSubscriptionPageSlugTaken when the new
// slug belongs to another page of the tenant. The SDK retries the transaction
// on conflict, so fn may run more than once and must be retry-safe.
func (r *YDBSubscriptionPages) Update(ctx context.Context, tenantID, id string,
	fn func(*domain.SubscriptionPage) (*domain.SubscriptionPage, error)) error {

	err := r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		loaded, err := ydbSubscriptionPageFromRow(tx.QueryRow(ctx, ydbSelectSubscriptionPage, ydbTenantAndID(tenantID, id)))
		if err != nil {
			return err
		}
		updated, err := fn(loaded)
		if err != nil {
			return err
		}
		params, err := ydbSubscriptionPageParams(tenantID, id, updated)
		if err != nil {
			return err
		}
		err = tx.Exec(ctx, ydbSubscriptionPageWrite+`UPDATE subscription_pages SET
    slug = $slug, title = $title, target_list_ids = $target_list_ids, fields = $fields,
    sending_domain_id = $sending_domain_id, from_name = $from_name,
    from_local_part = $from_local_part, active = $active, updated_at = CurrentUtcTimestamp()
WHERE tenant_id = $tenant_id AND id = $id;`, params)
		// An UPDATE onto another page's slug hits the unique slug index at
		// the statement (spec "Plan B4 planning spike").
		if db.IsYDBConflict(err) {
			return domain.ErrSubscriptionPageSlugTaken
		}
		if err != nil {
			return fmt.Errorf("updating subscription page: %w", err)
		}
		return nil
	})
	// This covers a YDB that reports the collision at commit.
	if db.IsYDBConflict(err) {
		return domain.ErrSubscriptionPageSlugTaken
	}
	return err
}

// Get returns the page by id, or domain.ErrSubscriptionPageNotFound.
func (r *YDBSubscriptionPages) Get(ctx context.Context, tenantID, id string) (*domain.SubscriptionPage, error) {
	return ydbSubscriptionPageFromRow(r.driver.Query().QueryRow(ctx, ydbSelectSubscriptionPage, ydbTenantAndID(tenantID, id)))
}

// GetBySlug returns the page by slug, or domain.ErrSubscriptionPageNotFound.
func (r *YDBSubscriptionPages) GetBySlug(ctx context.Context, tenantID, slug string) (*domain.SubscriptionPage, error) {
	return ydbSubscriptionPageFromRow(r.driver.Query().QueryRow(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $slug AS Utf8;
SELECT `+ydbSubscriptionPageColumns+`
FROM subscription_pages VIEW idx_subscription_pages_slug
WHERE tenant_id = $tenant_id AND slug = $slug;`,
		query.WithParameters(ydb.ParamsBuilder().
			Param("$tenant_id").Text(tenantID).
			Param("$slug").Text(slug).
			Build())))
}

// All returns every subscription page of the tenant, newest first. It is one
// statement outside a transaction, so nothing re-runs.
func (r *YDBSubscriptionPages) All(ctx context.Context, tenantID string) ([]*domain.SubscriptionPage, error) {
	rs, err := r.driver.Query().QueryResultSet(ctx, `DECLARE $tenant_id AS Utf8;
SELECT `+ydbSubscriptionPageColumns+`
FROM subscription_pages
WHERE tenant_id = $tenant_id
ORDER BY created_at DESC;`, ydbTenantParam(tenantID))
	if err != nil {
		return nil, fmt.Errorf("listing subscription pages: %w", err)
	}
	defer func() { _ = rs.Close(ctx) }()
	var pages []*domain.SubscriptionPage
	for row, err := range rs.Rows(ctx) {
		if err != nil {
			return nil, fmt.Errorf("listing subscription pages: %w", err)
		}
		p, err := scanYDBSubscriptionPage(row)
		if err != nil {
			return nil, err
		}
		pages = append(pages, p)
	}
	return pages, nil
}

// ydbSubscriptionPageParams binds every ydbSubscriptionPageWrite parameter.
func ydbSubscriptionPageParams(tenantID, id string, p *domain.SubscriptionPage) (query.ExecuteOption, error) {
	fields, err := json.Marshal(p.Fields())
	if err != nil {
		return nil, fmt.Errorf("encoding subscription page fields: %w", err)
	}
	return query.WithParameters(ydb.ParamsBuilder().
		Param("$tenant_id").Text(tenantID).
		Param("$id").Text(id).
		Param("$slug").Text(p.Slug()).
		Param("$title").Text(p.Title()).
		Param("$target_list_ids").Text(ydbStrings(p.TargetListIDs())).
		Param("$fields").Text(string(fields)).
		Param("$sending_domain_id").Text(p.SendingDomainID()).
		Param("$from_name").Text(p.FromName()).
		Param("$from_local_part").Text(p.FromLocalPart()).
		Param("$active").Bool(p.Active()).
		Build()), nil
}

// ydbSubscriptionPageFromRow maps a single-row read to the page, translating
// "no row" into domain.ErrSubscriptionPageNotFound.
func ydbSubscriptionPageFromRow(row query.Row, err error) (*domain.SubscriptionPage, error) {
	if errors.Is(err, query.ErrNoRows) {
		return nil, domain.ErrSubscriptionPageNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("loading subscription page: %w", err)
	}
	return scanYDBSubscriptionPage(row)
}

// scanYDBSubscriptionPage reads one row in ydbSubscriptionPageColumns order.
func scanYDBSubscriptionPage(row query.Row) (*domain.SubscriptionPage, error) {
	var id, tenantID, slug, title, rawListIDs, rawFields, sendingDomainID, fromName, fromLocalPart string
	var active bool
	var createdAt, updatedAt time.Time
	if err := row.Scan(&id, &tenantID, &slug, &title, &rawListIDs, &rawFields,
		&sendingDomainID, &fromName, &fromLocalPart, &active, &createdAt, &updatedAt); err != nil {
		return nil, fmt.Errorf("scanning subscription page: %w", err)
	}
	listIDs, err := ydbStringsFrom(rawListIDs)
	if err != nil {
		return nil, fmt.Errorf("decoding subscription page lists: %w", err)
	}
	var fields []domain.FormField
	if err := json.Unmarshal([]byte(rawFields), &fields); err != nil {
		return nil, fmt.Errorf("decoding subscription page fields: %w", err)
	}
	return domain.HydrateSubscriptionPage(id, tenantID, slug, title, listIDs, fields,
		sendingDomainID, fromName, fromLocalPart, active, createdAt, updatedAt), nil
}
```

- [ ] **Step 6: Write the pending subscription repository**

Create `internal/audience/adapters/pending_subscriptions_ydb.go`. `Upsert` reads the existing id through the unique index, then `UPSERT`s every column under that id or a fresh one. That keeps the row's id, as `ON CONFLICT DO UPDATE ... RETURNING id` does, and refreshes `created_at` as the Postgres `SET created_at = now()` does.

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
)

// YDBPendingSubscriptions is the YDB-backed implementation of
// domain.PendingSubscriptionRepository, for the serverless runtime. The
// attributes and target list ids are JSON in Utf8 columns.
type YDBPendingSubscriptions struct {
	driver *ydb.Driver
}

var _ domain.PendingSubscriptionRepository = (*YDBPendingSubscriptions)(nil)

// NewYDBPendingSubscriptions builds a YDBPendingSubscriptions repository over
// the driver.
func NewYDBPendingSubscriptions(d *ydb.Driver) *YDBPendingSubscriptions {
	return &YDBPendingSubscriptions{driver: d}
}

// ydbPendingSubscriptionColumns is the projection scanYDBPendingSubscription
// reads, in order.
const ydbPendingSubscriptionColumns = `id, tenant_id, subscription_page_id, email, attributes,
    target_list_ids, confirmation_token_hash, expires_at, created_at`

// Upsert creates the pending subscription, or refreshes the existing one for
// the same (tenant, email, page), and returns the row's id. It is the
// Postgres ON CONFLICT DO UPDATE: a read through the unique index, then an
// UPDATE or INSERT in one transaction. Concurrent upserts of one key abort
// each other, and the SDK retries the loser, which then sees the winner's row.
func (r *YDBPendingSubscriptions) Upsert(ctx context.Context, tenantID string,
	p *domain.PendingSubscription) (string, error) {

	attrs, err := marshalAttributes(p.Attributes())
	if err != nil {
		return "", err
	}
	newID := uuid.NewString()
	var id string
	err = r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		row, err := tx.QueryRow(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $email AS Utf8;
DECLARE $subscription_page_id AS Utf8;
SELECT id FROM pending_subscriptions VIEW idx_pending_subscriptions_email
WHERE tenant_id = $tenant_id AND email = $email AND subscription_page_id = $subscription_page_id;`,
			query.WithParameters(ydb.ParamsBuilder().
				Param("$tenant_id").Text(tenantID).
				Param("$email").Text(p.Email()).
				Param("$subscription_page_id").Text(p.SubscriptionPageID()).
				Build()))
		target := newID
		switch {
		case errors.Is(err, query.ErrNoRows):
		case err != nil:
			return fmt.Errorf("looking up pending subscription: %w", err)
		default:
			if err := row.Scan(&target); err != nil {
				return fmt.Errorf("scanning pending subscription id: %w", err)
			}
		}
		// UPSERT on the primary key writes every column: a new row under
		// newID, or the existing row with its key and email unchanged and a
		// fresh token, expiry and created_at, as ON CONFLICT DO UPDATE does.
		if err := tx.Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DECLARE $subscription_page_id AS Utf8;
DECLARE $email AS Utf8;
DECLARE $attributes AS Utf8;
DECLARE $target_list_ids AS Utf8;
DECLARE $confirmation_token_hash AS Utf8;
DECLARE $expires_at AS Timestamp;
UPSERT INTO pending_subscriptions
    (tenant_id, id, subscription_page_id, email, attributes, target_list_ids,
     confirmation_token_hash, expires_at, created_at)
VALUES ($tenant_id, $id, $subscription_page_id, $email, $attributes, $target_list_ids,
    $confirmation_token_hash, $expires_at, CurrentUtcTimestamp());`,
			query.WithParameters(ydb.ParamsBuilder().
				Param("$tenant_id").Text(tenantID).
				Param("$id").Text(target).
				Param("$subscription_page_id").Text(p.SubscriptionPageID()).
				Param("$email").Text(p.Email()).
				Param("$attributes").Text(string(attrs)).
				Param("$target_list_ids").Text(ydbStrings(p.TargetListIDs())).
				Param("$confirmation_token_hash").Text(p.ConfirmationTokenHash()).
				Param("$expires_at").Timestamp(p.ExpiresAt()).
				Build())); err != nil {
			return fmt.Errorf("upserting pending subscription: %w", err)
		}
		id = target
		return nil
	})
	if err != nil {
		return "", err
	}
	return id, nil
}

// Get returns the pending subscription by id, or
// domain.ErrPendingSubscriptionNotFound.
func (r *YDBPendingSubscriptions) Get(ctx context.Context, tenantID, id string) (*domain.PendingSubscription, error) {
	return ydbPendingSubscriptionFromRow(r.driver.Query().QueryRow(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
SELECT `+ydbPendingSubscriptionColumns+`
FROM pending_subscriptions
WHERE tenant_id = $tenant_id AND id = $id;`, ydbTenantAndID(tenantID, id)))
}

// GetByTokenHash returns the pending subscription whose confirmation token
// hashes to tokenHash, or domain.ErrPendingSubscriptionNotFound.
func (r *YDBPendingSubscriptions) GetByTokenHash(ctx context.Context, tenantID,
	tokenHash string) (*domain.PendingSubscription, error) {

	return ydbPendingSubscriptionFromRow(r.driver.Query().QueryRow(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $token_hash AS Utf8;
SELECT `+ydbPendingSubscriptionColumns+`
FROM pending_subscriptions VIEW idx_pending_subscriptions_token
WHERE tenant_id = $tenant_id AND confirmation_token_hash = $token_hash;`,
		query.WithParameters(ydb.ParamsBuilder().
			Param("$tenant_id").Text(tenantID).
			Param("$token_hash").Text(tokenHash).
			Build())))
}

// RefreshToken replaces the confirmation token hash and expiry. It returns
// domain.ErrPendingSubscriptionNotFound when absent.
func (r *YDBPendingSubscriptions) RefreshToken(ctx context.Context, tenantID, id, tokenHash string,
	expiresAt time.Time) error {

	_, err := r.driver.Query().QueryRow(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DECLARE $token_hash AS Utf8;
DECLARE $expires_at AS Timestamp;
UPDATE pending_subscriptions
SET confirmation_token_hash = $token_hash, expires_at = $expires_at
WHERE tenant_id = $tenant_id AND id = $id
RETURNING id;`,
		query.WithParameters(ydb.ParamsBuilder().
			Param("$tenant_id").Text(tenantID).
			Param("$id").Text(id).
			Param("$token_hash").Text(tokenHash).
			Param("$expires_at").Timestamp(expiresAt).
			Build()))
	if errors.Is(err, query.ErrNoRows) {
		return domain.ErrPendingSubscriptionNotFound
	}
	if err != nil {
		return fmt.Errorf("refreshing pending subscription token: %w", err)
	}
	return nil
}

// Delete removes the pending subscription. A missing row is not an error:
// confirmation is idempotent.
func (r *YDBPendingSubscriptions) Delete(ctx context.Context, tenantID, id string) error {
	if err := r.driver.Query().Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DELETE FROM pending_subscriptions
WHERE tenant_id = $tenant_id AND id = $id;`, ydbTenantAndID(tenantID, id)); err != nil {
		return fmt.Errorf("deleting pending subscription: %w", err)
	}
	return nil
}

// ydbPendingSubscriptionFromRow maps a single-row read to the pending
// subscription, translating "no row" into
// domain.ErrPendingSubscriptionNotFound.
func ydbPendingSubscriptionFromRow(row query.Row, err error) (*domain.PendingSubscription, error) {
	if errors.Is(err, query.ErrNoRows) {
		return nil, domain.ErrPendingSubscriptionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("loading pending subscription: %w", err)
	}
	var id, tenantID, pageID, email, rawAttrs, rawListIDs, tokenHash string
	var expiresAt, createdAt time.Time
	if err := row.Scan(&id, &tenantID, &pageID, &email, &rawAttrs, &rawListIDs,
		&tokenHash, &expiresAt, &createdAt); err != nil {
		return nil, fmt.Errorf("scanning pending subscription: %w", err)
	}
	raw := map[string]any{}
	if err := json.Unmarshal([]byte(rawAttrs), &raw); err != nil {
		return nil, fmt.Errorf("decoding pending subscription attributes: %w", err)
	}
	listIDs, err := ydbStringsFrom(rawListIDs)
	if err != nil {
		return nil, fmt.Errorf("decoding pending subscription lists: %w", err)
	}
	return domain.HydratePendingSubscription(id, tenantID, pageID, email,
		domain.HydrateAttributes(raw), listIDs, tokenHash, expiresAt, createdAt), nil
}
```

- [ ] **Step 7: Run the suites to verify they pass**

Run: `export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock" && go test ./internal/audience/adapters/ -run 'TestSubscriptionPages|TestPendingSubscriptions|TestConfirmSubscription|TestOptinWorker' -count=1 -v -timeout 8m 2>&1 | grep -E '^\s+--- (PASS|FAIL)'`

Expected: 16 `--- PASS` lines, one `/pg` and one `/ydb` for each of the eight tests, and no `--- FAIL`.

Run: `export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock" && go test ./internal/audience/adapters/ -run 'TestPendingSubscriptionsUpsertIsRaceFree' -count=10 -timeout 8m`

Expected: `ok`. On a failure, re-run once before investigating (see "Known environmental flakes").

Any other `/ydb` failure: fix only `subscription_pages_ydb.go` or `pending_subscriptions_ydb.go`, and record the verbatim error and the fix in the report.

- [ ] **Step 8: Verify and commit**

Run: `export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock" && go build ./... && go vet ./... && go test ./internal/audience/... -count=1 -timeout 8m`

Expected: every package `ok`.

Run: `golangci-lint run ./internal/audience/...`

Expected: `0 issues.`

```bash
gofmt -l internal/audience/
git add internal/db/ydbmigrations/000007_audience_optin_jobs.up.sql
git add internal/audience/adapters/subscription_pages_ydb.go internal/audience/adapters/pending_subscriptions_ydb.go
git add internal/audience/adapters/helpers_test.go internal/audience/adapters/optin_test.go internal/audience/adapters/optin_worker_test.go
git commit -m "feat(audience): add the YDB subscription page and pending subscription repositories under pg+ydb contract suites"
```

`gofmt -l` must print nothing. Afterwards, `git status --short` must print nothing.

---

### Task 2: Jobs on YDB, with job files in Object Storage

**Files:**
- Create: `internal/dbtest/s3.go`
- Modify: `internal/media/adapters/blobstore_s3_test.go`
- Modify: `internal/audience/adapters/jobs_pg.go`
- Create: `internal/audience/adapters/jobs_ydb.go`
- Modify: `internal/audience/adapters/helpers_test.go`
- Rename: `internal/audience/adapters/jobs_pg_test.go` → `internal/audience/adapters/jobs_test.go` (rewritten)

**Interfaces:**
- Consumes:
  - The port `domain.JobRepository` (`internal/audience/domain/repository.go`), unchanged.
  - From `jobs_pg.go`: `importParams`, `jobRow`, `importJobFromRow(tenantID string, j jobRow) (*domain.ImportJob, error)`, `exportJobFromRow(tenantID string, j jobRow) (*domain.ExportJob, error)`.
  - `fakecloudImage` (`internal/dbtest/sqs.go`, from C1).
  - The workers `NewImportWorker`, `NewExportWorker` and their `Work(ctx, args)` methods, unchanged.
  - From the harness: Task 1's `backend`, plus `addList`, `addSubscriber`, `emails`, `mustSegment` (B5).
- Produces:
  ```go
  // package dbtest
  const S3AccessKey, S3SecretKey, S3Region, S3Bucket = "test", "test", "us-east-1", "nvelope-media"
  func S3Endpoint(t *testing.T) string
  func S3Client(t *testing.T) *s3.Client
  // package adapters
  func NewYDBJobs(d *ydb.Driver, files *s3.Client, bucket string) *YDBJobs
  func exportParamsJSON(j *domain.ExportJob) ([]byte, error)   // jobs_pg.go
  func jobSummaryFromRow(j jobRow) domain.JobSummary           // jobs_pg.go
  // package adapters_test
  type backend struct {
      // Task 1 fields, plus:
      jobs func(t *testing.T) domain.JobRepository
  }
  ```

- [ ] **Step 1: Add the shared S3 test helper**

Create `internal/dbtest/s3.go`. It starts the same container the media test starts today (same name, image and environment), so both share one container. The bucket is created once per test binary, with the production policy: anonymous read on `published/*` only.

```go
package dbtest

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	// s3ContainerName is the fixed name of the shared fakecloud container
	// that stands in for Object Storage. It is distinct from the old
	// "nvelope-test-s3" so a reused container never comes up without IAM
	// enforcement, which bucket policies need.
	s3ContainerName = "nvelope-test-s3-strict"

	// S3AccessKey and S3SecretKey are fakecloud's root identity. Strict IAM
	// rejects any other key.
	S3AccessKey = "test"
	S3SecretKey = "test"
	// S3Region is the region the test bucket lives in.
	S3Region = "us-east-1"
	// S3Bucket is the test bucket. It has the production media bucket's
	// shape: objects are private, and anonymous read is granted on the
	// published/ prefix only.
	S3Bucket = "nvelope-media"
)

var (
	s3Once     sync.Once
	s3Endpoint string
	s3Err      error
)

// S3Endpoint resolves an S3 endpoint holding S3Bucket for integration tests.
// If NVELOPE_OBJECT_STORAGE_ENDPOINT is set it is used verbatim; otherwise a
// fakecloud container is started — or an existing one reused — via
// testcontainers-go. Tests write under their own unique keys, so they never
// share state.
func S3Endpoint(t *testing.T) string {
	t.Helper()
	s3Once.Do(func() {
		if env := os.Getenv("NVELOPE_OBJECT_STORAGE_ENDPOINT"); env != "" {
			s3Endpoint = env
		} else {
			s3Endpoint, s3Err = startS3Container()
		}
		if s3Err == nil {
			s3Err = ensureS3Bucket(s3Endpoint)
		}
	})
	require.NoError(t, s3Err)
	return s3Endpoint
}

// S3Client returns a path-style client for S3Endpoint, signed with the test
// credentials.
func S3Client(t *testing.T) *s3.Client {
	t.Helper()
	return newS3Client(S3Endpoint(t))
}

func newS3Client(endpoint string) *s3.Client {
	return s3.New(s3.Options{
		Region:       S3Region,
		BaseEndpoint: aws.String(endpoint),
		Credentials:  credentials.NewStaticCredentialsProvider(S3AccessKey, S3SecretKey, ""),
		UsePathStyle: true,
	})
}

func startS3Container() (string, error) {
	ctx := context.Background()
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        fakecloudImage,
			ExposedPorts: []string{"4566/tcp"},
			Env: map[string]string{
				// The default IPv4-only bind breaks testcontainers' "localhost"
				// through the IPv6 port mapping.
				"FAKECLOUD_ADDR": "[::]:4566",
				// fakecloud ignores bucket policies unless IAM enforcement is
				// on; with strict IAM an anonymous GET is checked against the
				// policy ensureS3Bucket sets.
				"FAKECLOUD_IAM": "strict",
			},
			WaitingFor: wait.ForHTTP("/_fakecloud/health").
				WithPort("4566/tcp").
				WithStartupTimeout(90 * time.Second),
			Name: s3ContainerName,
		},
		Started: true,
		Reuse:   true,
	})
	if err != nil {
		return "", fmt.Errorf("S3 integration tests require Docker: %w", err)
	}
	host, err := container.Host(ctx)
	if err != nil {
		return "", fmt.Errorf("resolving fakecloud host: %w", err)
	}
	port, err := container.MappedPort(ctx, "4566/tcp")
	if err != nil {
		return "", fmt.Errorf("resolving fakecloud port: %w", err)
	}
	return fmt.Sprintf("http://%s:%s", host, port.Port()), nil
}

// ensureS3Bucket creates S3Bucket if needed and grants anonymous read on the
// published/ prefix only.
func ensureS3Bucket(endpoint string) error {
	client := newS3Client(endpoint)
	ctx := context.Background()
	if _, err := client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(S3Bucket)}); err != nil {
		_, err := client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(S3Bucket)})
		// Another test binary may have created it in between.
		if _, headErr := client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(S3Bucket)}); headErr != nil {
			return fmt.Errorf("creating test bucket: %w", err)
		}
	}
	policy := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":["*"]},` +
		`"Action":["s3:GetObject"],"Resource":["arn:aws:s3:::` + S3Bucket + `/published/*"]}]}`
	if _, err := client.PutBucketPolicy(ctx, &s3.PutBucketPolicyInput{
		Bucket: aws.String(S3Bucket),
		Policy: aws.String(policy),
	}); err != nil {
		return fmt.Errorf("setting test bucket policy: %w", err)
	}
	return nil
}
```

- [ ] **Step 2: Move the media test onto it**

In `internal/media/adapters/blobstore_s3_test.go`, delete everything from the `const (` block (`s3ContainerName`, `s3AccessKey`, `s3SecretKey`, `s3TestBucket`) down to just before `func newTestStore`: the constants, the `s3Once`/`s3Endpoint`/`s3Err` variables, `startS3` and `ensureBucket`. Then replace the imports and `newTestStore` with the code below. The test functions below `newTestStore` stay as they are.

```go
package adapters_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
	"github.com/nikolaymatrosov/nvelope/internal/media/adapters"
)

func newTestStore(t *testing.T) *adapters.S3BlobStore {
	t.Helper()
	endpoint := dbtest.S3Endpoint(t)
	store, err := adapters.NewS3BlobStore(adapters.S3Config{
		Endpoint:        endpoint,
		Region:          dbtest.S3Region,
		Bucket:          dbtest.S3Bucket,
		AccessKeyID:     dbtest.S3AccessKey,
		SecretAccessKey: dbtest.S3SecretKey,
		PublicBaseURL:   endpoint + "/" + dbtest.S3Bucket,
	})
	require.NoError(t, err)
	return store
}
```

Run: `export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock" && go test ./internal/media/adapters/ -count=1 -timeout 8m`

Expected: `ok`.

- [ ] **Step 3: Share two helpers from the pg jobs adapter**

In `internal/audience/adapters/jobs_pg.go`, move the export-params encoding and the summary mapping into functions that `YDBJobs` also calls. Behaviour does not change.

In `AddExport`, replace:

```go
	p := exportParams{Selection: string(j.Selection()), ListID: j.ListID()}
	if j.Segment() != nil {
		root := j.Segment().Root()
		p.Segment = &root
	}
	params, err := json.Marshal(p)
	if err != nil {
		return "", fmt.Errorf("encoding export params: %w", err)
	}
```

with:

```go
	params, err := exportParamsJSON(j)
	if err != nil {
		return "", err
	}
```

Replace the comment `// importRow is the scanned shape of an import job row.` above `type jobRow` with:

```go
// exportParamsJSON encodes the export job's selection as its params column.
func exportParamsJSON(j *domain.ExportJob) ([]byte, error) {
	p := exportParams{Selection: string(j.Selection()), ListID: j.ListID()}
	if j.Segment() != nil {
		root := j.Segment().Root()
		p.Segment = &root
	}
	params, err := json.Marshal(p)
	if err != nil {
		return nil, fmt.Errorf("encoding export params: %w", err)
	}
	return params, nil
}

// jobRow is the scanned shape of an import or export job row.
```

Insert above `// GetImport returns the import job, or domain.ErrJobNotFound.`:

```go
// jobSummaryFromRow builds the kind-agnostic status of a scanned row.
func jobSummaryFromRow(j jobRow) domain.JobSummary {
	var failures []domain.RowFailure
	if len(j.failures) > 0 {
		_ = json.Unmarshal(j.failures, &failures)
	}
	return domain.JobSummary{
		ID: j.id, Kind: j.kind, Status: domain.JobStatus(j.status), FileName: j.fileName,
		CreatedCount: j.created, UpdatedCount: j.updated, FailedCount: j.failed,
		RowCount: j.rowCount, Failures: failures,
	}
}

```

In `Summary`, replace:

```go
		var failures []domain.RowFailure
		if len(j.failures) > 0 {
			_ = json.Unmarshal(j.failures, &failures)
		}
		out = domain.JobSummary{
			ID: j.id, Kind: j.kind, Status: domain.JobStatus(j.status), FileName: j.fileName,
			CreatedCount: j.created, UpdatedCount: j.updated, FailedCount: j.failed,
			RowCount: j.rowCount, Failures: failures,
		}
		return nil
```

with:

```go
		out = jobSummaryFromRow(j)
		return nil
```

Run: `export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock" && go test ./internal/audience/adapters/ -run 'TestJobRepository|TestImportWorkerAgainstRealQueue|TestExportWorkerAgainstRealQueue' -count=1 -timeout 8m`

Expected: `ok` (the old pg suite still passes).

- [ ] **Step 4: Extend the harness**

In `internal/audience/adapters/helpers_test.go`, add the `jobs` field after `pending`:

```go
	jobs        func(t *testing.T) domain.JobRepository
```

In the `"pg"` backend, after `pending`:

```go
		jobs: func(t *testing.T) domain.JobRepository {
			return adapters.NewJobs(dbtest.AppPool(t))
		},
```

In the `"ydb"` backend, after `pending`:

```go
		jobs: func(t *testing.T) domain.JobRepository {
			return adapters.NewYDBJobs(dbtest.YDBMigrated(t), dbtest.S3Client(t), dbtest.S3Bucket)
		},
```

- [ ] **Step 5: Convert the jobs suite**

```bash
git mv internal/audience/adapters/jobs_pg_test.go internal/audience/adapters/jobs_test.go
```

Replace `internal/audience/adapters/jobs_test.go` with the code below. The River-backed worker tests become `TestImportWorker` and `TestExportWorker`, which call `Work` directly on both backends; `setupRiverQueue` and `waitForJob` go away. New coverage: every job field round-trips, including failures, timestamps, the list selection and a segment; an export has no file until staged; unknown and cross-tenant ids are not found; an import attaches its rows to the target list and a redelivery changes nothing. `TestYDBJobsFilesArePrivate` runs on YDB only, because only YDB keeps the file in Object Storage.

```go
package adapters_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/audience/adapters"
	"github.com/nikolaymatrosov/nvelope/internal/audience/domain"
	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
	"github.com/nikolaymatrosov/nvelope/internal/platform/jobs"
)

const requester = "00000000-0000-0000-0000-000000000001"

const unknownJob = "00000000-0000-0000-0000-000000000000"

func TestJobRepositoryImportLifecycle(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.jobs(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		listID := addList(t, b.lists(t), tenantID, "Newsletter")

		job, err := domain.NewImportJob(tenantID, requester, "subs.csv", []string{listID})
		require.NoError(t, err)
		id, err := repo.AddImport(ctx, tenantID, job, []byte("email\na@example.com\n"))
		require.NoError(t, err)

		staged, err := repo.StagedFile(ctx, tenantID, id)
		require.NoError(t, err)
		require.Equal(t, "email\na@example.com\n", string(staged))

		got, err := repo.GetImport(ctx, tenantID, id)
		require.NoError(t, err)
		require.Equal(t, domain.JobPending, got.Status())
		require.Equal(t, "subs.csv", got.FileName())
		require.Equal(t, []string{listID}, got.TargetListIDs())
		require.Nil(t, got.StartedAt())

		failures := []domain.RowFailure{{Row: 3, Reason: "email is not valid"}}
		require.NoError(t, repo.UpdateImport(ctx, tenantID, id, func(j *domain.ImportJob) (*domain.ImportJob, error) {
			if err := j.Start(time.Now()); err != nil {
				return nil, err
			}
			return j, j.Complete(2, 1, 1, failures, time.Now())
		}))

		got, err = repo.GetImport(ctx, tenantID, id)
		require.NoError(t, err)
		require.NotNil(t, got.StartedAt())
		require.NotNil(t, got.FinishedAt())
		require.Equal(t, failures, got.Failures())

		summary, err := repo.Summary(ctx, tenantID, id)
		require.NoError(t, err)
		require.Equal(t, "import", summary.Kind)
		require.Equal(t, domain.JobCompleted, summary.Status)
		require.Equal(t, "subs.csv", summary.FileName)
		require.Equal(t, 2, summary.CreatedCount)
		require.Equal(t, 1, summary.UpdatedCount)
		require.Equal(t, 1, summary.FailedCount)
		require.Equal(t, failures, summary.Failures)

		// An import id is not an export job.
		_, err = repo.GetExport(ctx, tenantID, id)
		require.ErrorIs(t, err, domain.ErrJobNotFound)
	})
}

func TestJobRepositoryExportLifecycle(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.jobs(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		listID := addList(t, b.lists(t), tenantID, "Newsletter")

		job, err := domain.NewExportJob(tenantID, requester, domain.ExportList, listID, nil)
		require.NoError(t, err)
		id, err := repo.AddExport(ctx, tenantID, job)
		require.NoError(t, err)

		// An export has no file until its result is staged.
		staged, err := repo.StagedFile(ctx, tenantID, id)
		require.NoError(t, err)
		require.Nil(t, staged)

		require.NoError(t, repo.StageResult(ctx, tenantID, id, "export.csv", []byte("email\n")))
		require.NoError(t, repo.UpdateExport(ctx, tenantID, id, func(j *domain.ExportJob) (*domain.ExportJob, error) {
			if err := j.Start(time.Now()); err != nil {
				return nil, err
			}
			return j, j.Complete(1, time.Now())
		}))

		got, err := repo.GetExport(ctx, tenantID, id)
		require.NoError(t, err)
		require.Equal(t, domain.JobCompleted, got.Status())
		require.Equal(t, domain.ExportList, got.Selection())
		require.Equal(t, listID, got.ListID())
		require.Equal(t, 1, got.RowCount())
		staged, err = repo.StagedFile(ctx, tenantID, id)
		require.NoError(t, err)
		require.Equal(t, "email\n", string(staged))
		summary, err := repo.Summary(ctx, tenantID, id)
		require.NoError(t, err)
		require.Equal(t, "export", summary.Kind)
		require.Equal(t, "export.csv", summary.FileName)
		require.Equal(t, 1, summary.RowCount)

		// An export keeps its segment.
		seg := mustSegment(t, domain.Node{Field: &domain.FieldCondition{Field: "email", Op: domain.OpContains, Value: "a"}})
		segJob, err := domain.NewExportJob(tenantID, requester, domain.ExportSegment, "", &seg)
		require.NoError(t, err)
		segID, err := repo.AddExport(ctx, tenantID, segJob)
		require.NoError(t, err)
		gotSeg, err := repo.GetExport(ctx, tenantID, segID)
		require.NoError(t, err)
		require.NotNil(t, gotSeg.Segment())
		require.Equal(t, seg.Root(), gotSeg.Segment().Root())
	})
}

func TestJobRepositoryUnknownJob(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.jobs(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		_, err := repo.GetImport(ctx, tenantID, unknownJob)
		require.ErrorIs(t, err, domain.ErrJobNotFound)
		_, err = repo.GetExport(ctx, tenantID, unknownJob)
		require.ErrorIs(t, err, domain.ErrJobNotFound)
		_, err = repo.Summary(ctx, tenantID, unknownJob)
		require.ErrorIs(t, err, domain.ErrJobNotFound)
		_, err = repo.StagedFile(ctx, tenantID, unknownJob)
		require.ErrorIs(t, err, domain.ErrJobNotFound)
		require.ErrorIs(t, repo.StageResult(ctx, tenantID, unknownJob, "x.csv", []byte("x")),
			domain.ErrJobNotFound)
		noop := func(j *domain.ImportJob) (*domain.ImportJob, error) { return j, nil }
		require.ErrorIs(t, repo.UpdateImport(ctx, tenantID, unknownJob, noop), domain.ErrJobNotFound)

		// Another tenant's job is not found either.
		job, err := domain.NewImportJob(tenantID, requester, "subs.csv", nil)
		require.NoError(t, err)
		id, err := repo.AddImport(ctx, tenantID, job, []byte("email\n"))
		require.NoError(t, err)
		_, err = repo.Summary(ctx, b.newTenant(t), id)
		require.ErrorIs(t, err, domain.ErrJobNotFound)
	})
}

// TestYDBJobsFilesArePrivate pins where the YDB backend keeps a job's file:
// a private Object Storage object that the row references.
func TestYDBJobsFilesArePrivate(t *testing.T) {
	t.Parallel()
	files := dbtest.S3Client(t)
	repo := adapters.NewYDBJobs(dbtest.YDBMigrated(t), files, dbtest.S3Bucket)
	ctx := context.Background()
	tenantID := uuid.NewString()

	job, err := domain.NewImportJob(tenantID, requester, "subs.csv", nil)
	require.NoError(t, err)
	id, err := repo.AddImport(ctx, tenantID, job, []byte("email\na@example.com\n"))
	require.NoError(t, err)
	key := "jobs/" + tenantID + "/" + id

	// Subscriber data is never anonymously readable.
	resp, err := http.Get(dbtest.S3Endpoint(t) + "/" + dbtest.S3Bucket + "/" + key)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusForbidden, resp.StatusCode)

	// A row whose object is gone fails loudly instead of reading as empty.
	_, err = files.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(dbtest.S3Bucket), Key: aws.String(key)})
	require.NoError(t, err)
	_, err = repo.StagedFile(ctx, tenantID, id)
	require.Error(t, err)
}

func TestImportWorker(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		ctx := context.Background()
		tenantID := b.newTenant(t)
		jobRepo, subscribers, memberships := b.jobs(t), b.subscribers(t), b.memberships(t)
		listID := addList(t, b.lists(t), tenantID, "Imported")

		// Seed an existing subscriber so the import exercises an update.
		existing := addSubscriber(t, subscribers, tenantID, "existing@example.com")

		// Stage a CSV with a new row, an existing row, and an invalid row.
		csv := "email,name\nnew@example.com,New\nexisting@example.com,Updated\nnot-an-email,Bad\n"
		job, err := domain.NewImportJob(tenantID, requester, "subs.csv", []string{listID})
		require.NoError(t, err)
		jobID, err := jobRepo.AddImport(ctx, tenantID, job, []byte(csv))
		require.NoError(t, err)

		worker := adapters.NewImportWorker(jobRepo, subscribers, memberships)
		require.NoError(t, worker.Work(ctx, jobs.ImportArgs{TenantID: tenantID, JobID: jobID}))

		summary, err := jobRepo.Summary(ctx, tenantID, jobID)
		require.NoError(t, err)
		require.Equal(t, domain.JobCompleted, summary.Status)
		require.Equal(t, 1, summary.CreatedCount, "the new row is created")
		require.Equal(t, 1, summary.UpdatedCount, "the existing row is updated")
		require.Equal(t, 1, summary.FailedCount, "the invalid-email row is skipped")

		updated, err := subscribers.Get(ctx, tenantID, existing)
		require.NoError(t, err)
		require.Equal(t, "Updated", updated.Name())
		members, total, err := subscribers.InList(ctx, tenantID, listID, domain.Page{Limit: 10})
		require.NoError(t, err)
		require.Equal(t, 2, total)
		require.Equal(t, []string{"existing@example.com", "new@example.com"}, emails(members))

		// A redelivery of a finished job changes nothing.
		require.NoError(t, worker.Work(ctx, jobs.ImportArgs{TenantID: tenantID, JobID: jobID}))
		again, err := jobRepo.Summary(ctx, tenantID, jobID)
		require.NoError(t, err)
		require.Equal(t, summary, again)
	})
}

func TestExportWorker(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		ctx := context.Background()
		tenantID := b.newTenant(t)
		jobRepo, subscribers := b.jobs(t), b.subscribers(t)
		addSubscriber(t, subscribers, tenantID, "exp1@example.com")
		addSubscriber(t, subscribers, tenantID, "exp2@example.com")

		job, err := domain.NewExportJob(tenantID, requester, domain.ExportAll, "", nil)
		require.NoError(t, err)
		jobID, err := jobRepo.AddExport(ctx, tenantID, job)
		require.NoError(t, err)

		worker := adapters.NewExportWorker(jobRepo, subscribers)
		require.NoError(t, worker.Work(ctx, jobs.ExportArgs{TenantID: tenantID, JobID: jobID}))

		summary, err := jobRepo.Summary(ctx, tenantID, jobID)
		require.NoError(t, err)
		require.Equal(t, domain.JobCompleted, summary.Status)
		require.Equal(t, 2, summary.RowCount)

		data, err := jobRepo.StagedFile(ctx, tenantID, jobID)
		require.NoError(t, err)
		require.Contains(t, string(data), "exp1@example.com")
		require.Contains(t, string(data), "exp2@example.com")
	})
}
```

- [ ] **Step 6: Run the suite to verify it fails**

Run: `go vet ./internal/audience/adapters/`

Expected: FAIL with `undefined: adapters.NewYDBJobs`.

- [ ] **Step 7: Write the job repository**

Create `internal/audience/adapters/jobs_ydb.go`. It scans each row into the pg adapter's `jobRow`, so `importJobFromRow`, `exportJobFromRow` and `jobSummaryFromRow` rebuild the domain values on both backends. `UpdateImport` and `UpdateExport` never read `file_key` and never touch Object Storage.

```go
package adapters

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
	ydb "github.com/ydb-platform/ydb-go-sdk/v3"
	"github.com/ydb-platform/ydb-go-sdk/v3/query"

	"github.com/nikolaymatrosov/nvelope/internal/audience/domain"
)

// YDBJobs is the YDB-backed implementation of domain.JobRepository, for the
// serverless runtime. The job record is a YDB row; the staged upload (import)
// or generated file (export) is a private Object Storage object, because a
// YDB cell holds at most 16 MiB and an upload may be 32 MB. The object is
// written before the row that references it, so a row never points at a
// missing object.
type YDBJobs struct {
	driver *ydb.Driver
	files  *s3.Client
	bucket string
}

var _ domain.JobRepository = (*YDBJobs)(nil)

// NewYDBJobs builds a YDBJobs repository over the driver, keeping job files in
// bucket through files.
func NewYDBJobs(d *ydb.Driver, files *s3.Client, bucket string) *YDBJobs {
	return &YDBJobs{driver: d, files: files, bucket: bucket}
}

// ydbJobColumns is the projection scanYDBJob reads, in order. It leaves out
// file_key, which only StagedFile needs.
const ydbJobColumns = `id, kind, requested_by, status, params, file_name,
    created_count, updated_count, failed_count, row_count, failures,
    created_at, started_at, finished_at`

// ydbJobFileKey is the object key of a job's file. Import and export jobs
// never share an id, so one key shape serves both.
func ydbJobFileKey(tenantID, id string) string {
	return "jobs/" + tenantID + "/" + id
}

// AddImport stores the staged upload, then persists a new import job that
// references it, and returns the job id.
func (r *YDBJobs) AddImport(ctx context.Context, tenantID string, j *domain.ImportJob,
	fileBytes []byte) (string, error) {

	params, err := json.Marshal(importParams{TargetListIDs: j.TargetListIDs()})
	if err != nil {
		return "", fmt.Errorf("encoding import params: %w", err)
	}
	id := uuid.NewString()
	key := ydbJobFileKey(tenantID, id)
	if err := r.putFile(ctx, key, fileBytes); err != nil {
		return "", err
	}
	if err := r.insert(ctx, tenantID, id, "import", j.RequestedBy(), j.Status(), params,
		j.FileName(), &key); err != nil {
		r.removeFile(ctx, key)
		return "", fmt.Errorf("inserting import job: %w", err)
	}
	return id, nil
}

// AddExport persists a new export job, which has no file until StageResult.
func (r *YDBJobs) AddExport(ctx context.Context, tenantID string, j *domain.ExportJob) (string, error) {
	params, err := exportParamsJSON(j)
	if err != nil {
		return "", err
	}
	id := uuid.NewString()
	if err := r.insert(ctx, tenantID, id, "export", j.RequestedBy(), j.Status(), params,
		"", nil); err != nil {
		return "", fmt.Errorf("inserting export job: %w", err)
	}
	return id, nil
}

// insert writes a new job row with zero counts and no failures, as the
// Postgres column defaults do.
func (r *YDBJobs) insert(ctx context.Context, tenantID, id, kind, requestedBy string,
	status domain.JobStatus, params []byte, fileName string, fileKey *string) error {

	return r.driver.Query().Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DECLARE $kind AS Utf8;
DECLARE $requested_by AS Utf8;
DECLARE $status AS Utf8;
DECLARE $params AS Utf8;
DECLARE $file_name AS Utf8;
DECLARE $file_key AS Optional<Utf8>;
INSERT INTO import_export_jobs
    (tenant_id, id, kind, requested_by, status, params, file_name, file_key,
     created_count, updated_count, failed_count, row_count, failures, created_at)
VALUES ($tenant_id, $id, $kind, $requested_by, $status, $params, $file_name, $file_key,
    0l, 0l, 0l, 0l, "[]"u, CurrentUtcTimestamp());`,
		query.WithParameters(ydb.ParamsBuilder().
			Param("$tenant_id").Text(tenantID).
			Param("$id").Text(id).
			Param("$kind").Text(kind).
			Param("$requested_by").Text(requestedBy).
			Param("$status").Text(string(status)).
			Param("$params").Text(string(params)).
			Param("$file_name").Text(fileName).
			Param("$file_key").BeginOptional().Text(fileKey).EndOptional().
			Build()))
}

// ydbSelectJob reads one job of the given kind.
const ydbSelectJob = `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DECLARE $kind AS Utf8;
SELECT ` + ydbJobColumns + `
FROM import_export_jobs
WHERE tenant_id = $tenant_id AND id = $id AND kind = $kind;`

// ydbJobKey binds the job's primary key and the kind it must have.
func ydbJobKey(tenantID, id, kind string) query.ExecuteOption {
	return query.WithParameters(ydb.ParamsBuilder().
		Param("$tenant_id").Text(tenantID).
		Param("$id").Text(id).
		Param("$kind").Text(kind).
		Build())
}

// GetImport returns the import job, or domain.ErrJobNotFound.
func (r *YDBJobs) GetImport(ctx context.Context, tenantID, id string) (*domain.ImportJob, error) {
	row, err := ydbJobFromRow(r.driver.Query().QueryRow(ctx, ydbSelectJob, ydbJobKey(tenantID, id, "import")))
	if err != nil {
		return nil, err
	}
	return importJobFromRow(tenantID, row)
}

// GetExport returns the export job, or domain.ErrJobNotFound.
func (r *YDBJobs) GetExport(ctx context.Context, tenantID, id string) (*domain.ExportJob, error) {
	row, err := ydbJobFromRow(r.driver.Query().QueryRow(ctx, ydbSelectJob, ydbJobKey(tenantID, id, "export")))
	if err != nil {
		return nil, err
	}
	return exportJobFromRow(tenantID, row)
}

// UpdateImport loads the import job, runs fn, and persists the result in one
// serializable transaction. The SDK retries the transaction on conflict, so
// fn may run more than once and must be retry-safe.
func (r *YDBJobs) UpdateImport(ctx context.Context, tenantID, id string,
	fn func(*domain.ImportJob) (*domain.ImportJob, error)) error {

	return r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		row, err := ydbJobFromRow(tx.QueryRow(ctx, ydbSelectJob, ydbJobKey(tenantID, id, "import")))
		if err != nil {
			return err
		}
		loaded, err := importJobFromRow(tenantID, row)
		if err != nil {
			return err
		}
		updated, err := fn(loaded)
		if err != nil {
			return err
		}
		created, upd, failed := updated.Counts()
		failures, err := json.Marshal(updated.Failures())
		if err != nil {
			return fmt.Errorf("encoding failures: %w", err)
		}
		if err := tx.Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DECLARE $status AS Utf8;
DECLARE $created_count AS Int64;
DECLARE $updated_count AS Int64;
DECLARE $failed_count AS Int64;
DECLARE $failures AS Utf8;
DECLARE $started_at AS Optional<Timestamp>;
DECLARE $finished_at AS Optional<Timestamp>;
UPDATE import_export_jobs SET
    status = $status, created_count = $created_count, updated_count = $updated_count,
    failed_count = $failed_count, failures = $failures,
    started_at = $started_at, finished_at = $finished_at
WHERE tenant_id = $tenant_id AND id = $id;`,
			query.WithParameters(ydb.ParamsBuilder().
				Param("$tenant_id").Text(tenantID).
				Param("$id").Text(id).
				Param("$status").Text(string(updated.Status())).
				Param("$created_count").Int64(int64(created)).
				Param("$updated_count").Int64(int64(upd)).
				Param("$failed_count").Int64(int64(failed)).
				Param("$failures").Text(string(failures)).
				Param("$started_at").BeginOptional().Timestamp(updated.StartedAt()).EndOptional().
				Param("$finished_at").BeginOptional().Timestamp(updated.FinishedAt()).EndOptional().
				Build())); err != nil {
			return fmt.Errorf("updating import job: %w", err)
		}
		return nil
	})
}

// UpdateExport loads the export job, runs fn, and persists the result in one
// serializable transaction. The SDK retries the transaction on conflict, so
// fn may run more than once and must be retry-safe.
func (r *YDBJobs) UpdateExport(ctx context.Context, tenantID, id string,
	fn func(*domain.ExportJob) (*domain.ExportJob, error)) error {

	return r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		row, err := ydbJobFromRow(tx.QueryRow(ctx, ydbSelectJob, ydbJobKey(tenantID, id, "export")))
		if err != nil {
			return err
		}
		loaded, err := exportJobFromRow(tenantID, row)
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
DECLARE $row_count AS Int64;
DECLARE $started_at AS Optional<Timestamp>;
DECLARE $finished_at AS Optional<Timestamp>;
UPDATE import_export_jobs SET
    status = $status, row_count = $row_count,
    started_at = $started_at, finished_at = $finished_at
WHERE tenant_id = $tenant_id AND id = $id;`,
			query.WithParameters(ydb.ParamsBuilder().
				Param("$tenant_id").Text(tenantID).
				Param("$id").Text(id).
				Param("$status").Text(string(updated.Status())).
				Param("$row_count").Int64(int64(updated.RowCount())).
				Param("$started_at").BeginOptional().Timestamp(updated.StartedAt()).EndOptional().
				Param("$finished_at").BeginOptional().Timestamp(updated.FinishedAt()).EndOptional().
				Build())); err != nil {
			return fmt.Errorf("updating export job: %w", err)
		}
		return nil
	})
}

// Summary returns the kind-agnostic status of any job.
func (r *YDBJobs) Summary(ctx context.Context, tenantID, id string) (domain.JobSummary, error) {
	row, err := ydbJobFromRow(r.driver.Query().QueryRow(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
SELECT `+ydbJobColumns+`
FROM import_export_jobs
WHERE tenant_id = $tenant_id AND id = $id;`, ydbTenantAndID(tenantID, id)))
	if err != nil {
		return domain.JobSummary{}, err
	}
	return jobSummaryFromRow(row), nil
}

// StagedFile returns the job's file, or nil when the job has none yet. A
// job whose object is missing is an error, never an empty file.
func (r *YDBJobs) StagedFile(ctx context.Context, tenantID, id string) ([]byte, error) {
	row, err := r.driver.Query().QueryRow(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
SELECT file_key FROM import_export_jobs
WHERE tenant_id = $tenant_id AND id = $id;`, ydbTenantAndID(tenantID, id))
	if errors.Is(err, query.ErrNoRows) {
		return nil, domain.ErrJobNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("loading staged file: %w", err)
	}
	var key *string
	if err := row.Scan(&key); err != nil {
		return nil, fmt.Errorf("scanning staged file key: %w", err)
	}
	if key == nil {
		return nil, nil
	}
	return r.getFile(ctx, *key)
}

// StageResult stores a generated file, then points the job at it. It returns
// domain.ErrJobNotFound when the job is absent.
func (r *YDBJobs) StageResult(ctx context.Context, tenantID, id, fileName string, data []byte) error {
	key := ydbJobFileKey(tenantID, id)
	if err := r.putFile(ctx, key, data); err != nil {
		return err
	}
	_, err := r.driver.Query().QueryRow(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DECLARE $file_name AS Utf8;
DECLARE $file_key AS Utf8;
UPDATE import_export_jobs SET file_name = $file_name, file_key = $file_key
WHERE tenant_id = $tenant_id AND id = $id
RETURNING id;`,
		query.WithParameters(ydb.ParamsBuilder().
			Param("$tenant_id").Text(tenantID).
			Param("$id").Text(id).
			Param("$file_name").Text(fileName).
			Param("$file_key").Text(key).
			Build()))
	if errors.Is(err, query.ErrNoRows) {
		r.removeFile(ctx, key)
		return domain.ErrJobNotFound
	}
	if err != nil {
		return fmt.Errorf("staging job result: %w", err)
	}
	return nil
}

// putFile writes data to key as a private object (no ACL: the bucket grants
// anonymous read on published/ only).
func (r *YDBJobs) putFile(ctx context.Context, key string, data []byte) error {
	if _, err := r.files.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(r.bucket),
		Key:           aws.String(key),
		Body:          bytes.NewReader(data),
		ContentLength: aws.Int64(int64(len(data))),
	}); err != nil {
		return fmt.Errorf("storing job file %s: %w", key, err)
	}
	return nil
}

// getFile reads the object at key.
func (r *YDBJobs) getFile(ctx context.Context, key string) ([]byte, error) {
	out, err := r.files.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(r.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("loading job file %s: %w", key, err)
	}
	defer func() { _ = out.Body.Close() }()
	data, err := io.ReadAll(out.Body)
	if err != nil {
		return nil, fmt.Errorf("reading job file %s: %w", key, err)
	}
	return data, nil
}

// removeFile deletes an object no row references. It is best-effort: an
// orphan is a private object that nothing reads.
func (r *YDBJobs) removeFile(ctx context.Context, key string) {
	_, _ = r.files.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(r.bucket),
		Key:    aws.String(key),
	})
}

// ydbJobFromRow maps a single-row read in ydbJobColumns order to the shared
// jobRow, translating "no row" into domain.ErrJobNotFound.
func ydbJobFromRow(row query.Row, err error) (jobRow, error) {
	if errors.Is(err, query.ErrNoRows) {
		return jobRow{}, domain.ErrJobNotFound
	}
	if err != nil {
		return jobRow{}, fmt.Errorf("loading job: %w", err)
	}
	var j jobRow
	var params, failures string
	var created, updated, failed, rowCount int64
	if err := row.Scan(&j.id, &j.kind, &j.requestedBy, &j.status, &params, &j.fileName,
		&created, &updated, &failed, &rowCount, &failures,
		&j.createdAt, &j.startedAt, &j.finishedAt); err != nil {
		return jobRow{}, fmt.Errorf("scanning job: %w", err)
	}
	j.params, j.failures = []byte(params), []byte(failures)
	j.created, j.updated, j.failed, j.rowCount = int(created), int(updated), int(failed), int(rowCount)
	return j, nil
}
```

- [ ] **Step 8: Run the suites to verify they pass**

Run: `export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock" && go test ./internal/audience/adapters/ -run 'TestJobRepository|TestYDBJobs|TestImportWorker|TestExportWorker' -count=1 -v -timeout 8m 2>&1 | grep -E '^\s*--- (PASS|FAIL)'`

Expected: no `--- FAIL`. Each of the five eachBackend tests has a `/pg` and a `/ydb` `--- PASS`, and `TestYDBJobsFilesArePrivate` passes.

Any other `/ydb` failure: fix only `jobs_ydb.go`, and record the verbatim error and the fix in the report.

- [ ] **Step 9: Verify and commit**

Run: `export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock" && go build ./... && go vet ./... && go test ./internal/audience/... ./internal/media/... ./internal/dbtest/... -count=1 -timeout 8m`

Expected: every package `ok`.

Run: `golangci-lint run ./internal/audience/... ./internal/media/... ./internal/dbtest/...`

Expected: `0 issues.`

```bash
gofmt -l internal/audience/ internal/media/ internal/dbtest/
git add internal/dbtest/s3.go internal/media/adapters/blobstore_s3_test.go
git add internal/audience/adapters/jobs_pg.go internal/audience/adapters/jobs_ydb.go
git add internal/audience/adapters/helpers_test.go internal/audience/adapters/jobs_test.go
git commit -m "feat(audience): add the YDB job repository with job files in Object Storage under a pg+ydb contract suite"
```

`gofmt -l` must print nothing. Afterwards, `git status --short` must print nothing.

---

### Task 3: Find the subscriber by exact email

`ConfirmSubscriptionHandler` and `ImportWorker.attachToLists` re-load an upserted subscriber with `Search(email, Limit 1)`. `Search` matches a substring and returns the newest match, so for `alice@example.com` they act on a newer `malice@example.com`. A double-opt-in confirmation then confirms the wrong subscriber's memberships, and an import attaches the wrong subscriber to its lists. This happens on both backends.

**Files:**
- Modify: `internal/audience/domain/repository.go`
- Modify: `internal/audience/adapters/subscribers_pg.go`
- Modify: `internal/audience/adapters/subscribers_ydb.go`
- Modify: `internal/audience/app/command/fakes_test.go`
- Modify: `internal/audience/app/query/query_test.go`
- Modify: `internal/audience/app/command/confirm_subscription.go`
- Modify: `internal/audience/adapters/import_worker.go`
- Test: `internal/audience/adapters/subscribers_test.go`, `jobs_test.go`, `optin_test.go`

**Interfaces:**
- Consumes: Tasks 1–2's suites; `scanSubscriberRow`, `subscriberColumns` (`subscribers_pg.go`); `ydbSubscriberFromRow`, `ydbSubscriberColumns` (`subscribers_ydb.go`); the index `idx_subscribers_email` (`000006_audience`).
- Produces:
  ```go
  // package domain, on SubscriberRepository
  GetByEmail(ctx context.Context, tenantID, email string) (*Subscriber, error)
  ```

- [ ] **Step 1: Write the failing tests**

Append to `internal/audience/adapters/subscribers_test.go`:

```go
// TestSubscribersGetByEmail pins an exact, case-insensitive email lookup: it
// never returns a subscriber whose email merely contains the address, as a
// Search for it would.
func TestSubscribersGetByEmail(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.subscribers(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		alice := addSubscriber(t, repo, tenantID, "alice@example.com")
		addSubscriber(t, repo, tenantID, "malice@example.com") // newer, and contains "alice@example.com"

		got, err := repo.GetByEmail(ctx, tenantID, "alice@example.com")
		require.NoError(t, err)
		require.Equal(t, alice, got.ID())
		got, err = repo.GetByEmail(ctx, tenantID, "Alice@Example.COM")
		require.NoError(t, err)
		require.Equal(t, alice, got.ID(), "the lookup ignores case, as citext does")

		_, err = repo.GetByEmail(ctx, tenantID, "lice@example.com")
		require.ErrorIs(t, err, domain.ErrSubscriberNotFound)
		_, err = repo.GetByEmail(ctx, b.newTenant(t), "alice@example.com")
		require.ErrorIs(t, err, domain.ErrSubscriberNotFound)
	})
}
```

In `internal/audience/adapters/jobs_test.go` (`TestImportWorker`), replace:

```go
		// Seed an existing subscriber so the import exercises an update.
		existing := addSubscriber(t, subscribers, tenantID, "existing@example.com")
```

with:

```go
		// Seed an existing subscriber so the import exercises an update, and a
		// newer one whose email contains it, which the import must not attach.
		existing := addSubscriber(t, subscribers, tenantID, "existing@example.com")
		addSubscriber(t, subscribers, tenantID, "preexisting@example.com")
```

In `internal/audience/adapters/optin_test.go` (`TestConfirmSubscriptionLifecycle`), insert before `// Submit → pending row, then confirm → subscriber with a confirmed membership.`:

```go
		// alice is already a subscriber, and a newer subscriber's email
		// contains hers: the confirmation must reach alice, not malice.
		addSubscriber(t, subscribers, tenantID, "alice@example.com")
		malice := addSubscriber(t, subscribers, tenantID, "malice@example.com")

```

and replace:

```go
		subs, total, err := subscribers.Search(ctx, tenantID, "alice@example.com", domain.Page{Limit: 1})
		require.NoError(t, err)
		require.Equal(t, 1, total)
		require.Equal(t, "Alice", subs[0].Name())
		mships, err := memberships.ForSubscriber(ctx, tenantID, subs[0].ID())
		require.NoError(t, err)
		require.Len(t, mships, 1)
		require.Equal(t, domain.SubscriptionConfirmed, mships[0].Status())
```

with:

```go
		alice, err := subscribers.GetByEmail(ctx, tenantID, "alice@example.com")
		require.NoError(t, err)
		require.Equal(t, "Alice", alice.Name())
		mships, err := memberships.ForSubscriber(ctx, tenantID, alice.ID())
		require.NoError(t, err)
		require.Len(t, mships, 1)
		require.Equal(t, domain.SubscriptionConfirmed, mships[0].Status())
		mships, err = memberships.ForSubscriber(ctx, tenantID, malice)
		require.NoError(t, err)
		require.Empty(t, mships)
```

Run: `go vet ./internal/audience/adapters/`

Expected: FAIL with `repo.GetByEmail undefined` (the port has no such method yet).

- [ ] **Step 2: Add the port method and both implementations**

In `internal/audience/domain/repository.go`, after `Get` in `SubscriberRepository`:

```go
	// GetByEmail returns the subscriber with exactly this email, compared
	// case-insensitively, or ErrSubscriberNotFound.
	GetByEmail(ctx context.Context, tenantID, email string) (*Subscriber, error)
```

In `internal/audience/adapters/subscribers_pg.go`, insert above `func (r *Subscribers) getTx(`:

```go
// GetByEmail returns the subscriber with exactly this email, or
// domain.ErrSubscriberNotFound. The citext column compares case-insensitively.
func (r *Subscribers) GetByEmail(ctx context.Context, tenantID, email string) (*domain.Subscriber, error) {
	var out *domain.Subscriber
	err := tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		s, err := scanSubscriberRow(tx.QueryRow(ctx,
			"SELECT "+subscriberColumns+" FROM subscribers WHERE email = $1", email))
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrSubscriberNotFound
		}
		if err != nil {
			return fmt.Errorf("loading subscriber by email: %w", err)
		}
		out = s
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

```

In `internal/audience/adapters/subscribers_ydb.go`, insert above the `// Search returns a page of subscribers whose email or name contains q, case` comment:

```go
// GetByEmail returns the subscriber with exactly this email, or
// domain.ErrSubscriberNotFound. The column holds the email lower-cased, so
// lower-casing the argument matches the Postgres citext comparison.
func (r *YDBSubscribers) GetByEmail(ctx context.Context, tenantID, email string) (*domain.Subscriber, error) {
	return ydbSubscriberFromRow(r.driver.Query().QueryRow(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $email AS Utf8;
SELECT `+ydbSubscriberColumns+`
FROM subscribers VIEW idx_subscribers_email AS s
WHERE s.tenant_id = $tenant_id AND s.email = $email;`,
		query.WithParameters(ydb.ParamsBuilder().
			Param("$tenant_id").Text(tenantID).
			Param("$email").Text(strings.ToLower(email)).
			Build())))
}

```

In `internal/audience/app/command/fakes_test.go`, add `"strings"` to the imports (after `"strconv"`) and insert above `func (f *fakeSubscribers) Get(`:

```go
func (f *fakeSubscribers) GetByEmail(_ context.Context, tenantID, email string) (*domain.Subscriber, error) {
	for _, s := range f.byID {
		if s.TenantID() == tenantID && strings.EqualFold(s.Email(), email) {
			return s, nil
		}
	}
	return nil, domain.ErrSubscriberNotFound
}

```

In `internal/audience/app/query/query_test.go`, insert above `func (f *fakeSubscribers) Search(`:

```go
func (f *fakeSubscribers) GetByEmail(context.Context, string, string) (*domain.Subscriber, error) {
	return nil, domain.ErrSubscriberNotFound
}
```

- [ ] **Step 3: Run the tests to verify the regressions fail**

Run: `export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock" && go test ./internal/audience/adapters/ -run 'TestSubscribersGetByEmail|TestImportWorker|TestConfirmSubscriptionLifecycle' -count=1 -timeout 8m 2>&1 | grep -E '^\s*--- FAIL'`

Expected: `TestImportWorker` and `TestConfirmSubscriptionLifecycle` fail on `/pg` and `/ydb` (the callers still use `Search`). `TestSubscribersGetByEmail` passes.

- [ ] **Step 4: Switch both callers to GetByEmail**

In `internal/audience/app/command/confirm_subscription.go`, replace:

```go
	subscriberID, err := h.resolveID(ctx, cmd.TenantID, pending.Email())
	if err != nil {
		return ConfirmSubscriptionResult{}, err
	}
	for _, listID := range pending.TargetListIDs() {
		if err := h.confirmMembership(ctx, cmd.TenantID, subscriberID, listID); err != nil {
```

with:

```go
	// Re-load the upserted subscriber by its exact email to obtain its id.
	subscriber, err := h.subscribers.GetByEmail(ctx, cmd.TenantID, pending.Email())
	if err != nil {
		return ConfirmSubscriptionResult{}, err
	}
	for _, listID := range pending.TargetListIDs() {
		if err := h.confirmMembership(ctx, cmd.TenantID, subscriber.ID(), listID); err != nil {
```

and delete the `resolveID` method with its comment (`// resolveID re-loads an upserted subscriber by email to obtain its id.` through its closing brace).

In `internal/audience/adapters/import_worker.go` (`attachToLists`), change the doc comment's `subscriber is re-loaded by email to obtain its id.` to `subscriber is re-loaded by its exact email to obtain its id.`, and replace:

```go
	subs, _, err := w.subscribers.Search(ctx, tenantID, email, domain.Page{Limit: 1})
	if err != nil || len(subs) == 0 {
		return
	}
```

with:

```go
	sub, err := w.subscribers.GetByEmail(ctx, tenantID, email)
	if err != nil {
		return
	}
```

and, in the loop, `subs[0].ID()` with `sub.ID()`.

- [ ] **Step 5: Run the tests to verify they pass, and commit**

Run: `export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock" && go build ./... && go vet ./... && go test ./internal/audience/... -count=1 -timeout 8m`

Expected: every package `ok`.

Run: `golangci-lint run ./internal/audience/...`

Expected: `0 issues.`

```bash
gofmt -l internal/audience/
git add internal/audience/domain/repository.go internal/audience/adapters/subscribers_pg.go internal/audience/adapters/subscribers_ydb.go
git add internal/audience/app/command/fakes_test.go internal/audience/app/query/query_test.go
git add internal/audience/app/command/confirm_subscription.go internal/audience/adapters/import_worker.go
git add internal/audience/adapters/subscribers_test.go internal/audience/adapters/jobs_test.go internal/audience/adapters/optin_test.go
git commit -m "fix(audience): find the subscriber by exact email when confirming a subscription or importing"
```

`gofmt -l` must print nothing. Afterwards, `git status --short` must print nothing.

---

### Task 4: Format large numbers in segment conditions

Both segment translators build an attribute operand with `fmt.Sprintf("%v", c.Value)`. A JSON number decodes to a `float64`, and `%v` prints 1,000,000 as `1e+06`, while Postgres `->>` and YDB `JSON_VALUE` read the stored number back as `1000000`. So `seats eq 1000000` never matches on either backend.

**Files:**
- Modify: `internal/audience/adapters/segment_sql.go`
- Modify: `internal/audience/adapters/segment_yql.go`
- Test: `internal/audience/adapters/subscribers_test.go`

**Interfaces:**
- Produces: `func segmentOperand(v any) string` (`segment_sql.go`), used by both translators.

- [ ] **Step 1: Write the failing test rows**

In `internal/audience/adapters/subscribers_test.go` (`TestSubscribersRunSegmentOperators`), give bob a `seats` attribute. Replace:

```go
			map[string]any{"plan": "free", "n": 7})
```

with:

```go
			map[string]any{"plan": "free", "n": 7, "seats": 1000000})
```

and add after the `{"attr eq on a boolean", ...}` row:

```go
			{"attr eq on a number of a million", attr("seats", domain.OpEq, float64(1000000)), []string{o}},
			{"attr gte on a number of a million", attr("seats", domain.OpGte, float64(1000000)), []string{o}},
```

Run: `export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock" && go test ./internal/audience/adapters/ -run 'TestSubscribersRunSegmentOperators' -count=1 -timeout 8m 2>&1 | grep -E '^\s*--- FAIL|eq on a number of a million'`

Expected: FAIL on `/pg` and `/ydb`, in `attr eq on a number of a million`. The `gte` row already passes: both backends cast `1e+06` to a number.

- [ ] **Step 2: Add segmentOperand**

In `internal/audience/adapters/segment_sql.go`, add `"strconv"` to the imports (after `"fmt"`), and insert above `// attr translates a custom-attribute condition over the jsonb column.`:

```go
// segmentOperand renders an attribute condition's value as the text that
// Postgres ->> and YDB JSON_VALUE read a stored scalar back as. A JSON number
// decodes to a float64, which %v would print as 1e+06 for 1000000.
func segmentOperand(v any) string {
	if f, ok := v.(float64); ok {
		return strconv.FormatFloat(f, 'f', -1, 64)
	}
	return fmt.Sprintf("%v", v)
}

```

Replace all four `t.placeholder(fmt.Sprintf("%v", c.Value))` in `segment_sql.go` with `t.placeholder(segmentOperand(c.Value))`, and the one `t.param(fmt.Sprintf("%v", c.Value))` in `segment_yql.go` with `t.param(segmentOperand(c.Value))`.

- [ ] **Step 3: Run the tests to verify they pass, and commit**

Run: `export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock" && go vet ./internal/audience/adapters/ && go test ./internal/audience/adapters/ -run 'TestSubscribersRunSegment|TestSegment' -count=1 -timeout 8m`

Expected: `ok`.

Run: `golangci-lint run ./internal/audience/...`

Expected: `0 issues.`

```bash
gofmt -l internal/audience/
git add internal/audience/adapters/segment_sql.go internal/audience/adapters/segment_yql.go internal/audience/adapters/subscribers_test.go
git commit -m "fix(audience): format large numbers in segment conditions as Postgres and YDB read them back"
```

`gofmt -l` must print nothing. Afterwards, `git status --short` must print nothing.

---

### Task 5: Check the list when assigning a per-list role on YDB

`YDBRoles.AssignListRole` (`internal/iam/adapters/roles_ydb.go`) reads the role but not the list. B4 accepted that while `lists` was not on YDB; since B5 it is. Postgres rejects an assignment to an unknown list through the `user_list_roles` foreign key. On YDB the assignment succeeds, and so does one that commits after the list's delete. The dangling row keeps its role in use, so `YDBRoles.Delete` returns `ErrRoleInUse` until someone removes the assignment by the dead list id.

**Files:**
- Modify: `internal/iam/domain/errors.go`
- Modify: `internal/iam/adapters/roles_ydb.go`
- Modify: `internal/iam/adapters/helpers_test.go`
- Test: `internal/iam/adapters/roles_test.go`

**Interfaces:**
- Consumes: the audience `lists` table (`000006_audience`), read by key; `ydbRoleExists`, `ydbTenantAndID` (iam adapters); the iam harness (`eachBackend`, `newTenantUser`, `newRole`).
- Produces: `domain.ErrListNotFound` (iam), a `NotFound` apperr with slug `list_not_found`.

- [ ] **Step 1: Add the error and seed real lists in the iam harness**

In `internal/iam/domain/errors.go`, after `ErrRoleNotFound`:

```go

	// ErrListNotFound is returned when a per-list role is assigned on a list
	// that does not exist.
	ErrListNotFound = apperr.NewNotFound("list_not_found", "no such list")
```

In `internal/iam/adapters/helpers_test.go`:

1. Add to the imports, after `"github.com/stretchr/testify/require"`:

```go
	ydb "github.com/ydb-platform/ydb-go-sdk/v3"
	"github.com/ydb-platform/ydb-go-sdk/v3/query"
```

2. Replace the comment above `newTenant` in `backend` with:

```go
	// newTenant, newPlatformUser and newList return ids of rows other contexts
	// own. On Postgres the iam tables carry foreign keys to them, so the pg
	// backend inserts real rows. YDB has no foreign keys. No iam query reads
	// tenants or platform_users, so a bare UUID stands in for those; a per-list
	// role assignment reads lists, so newList inserts a real row there too.
```

3. In the `"ydb"` backend, replace `newList:         func(*testing.T, string) string { return uuid.NewString() },` with `newList:         seedYDBList,`.

4. Insert above `// seedTenant inserts a tenant row directly and returns its id.`:

```go
// seedYDBList inserts a row into the audience context's lists table, which a
// per-list role assignment reads on YDB in place of the Postgres foreign key.
// Raw YQL keeps the iam tests from importing another context.
func seedYDBList(t *testing.T, tenantID string) string {
	t.Helper()
	id := uuid.NewString()
	require.NoError(t, dbtest.YDBMigrated(t).Query().Exec(context.Background(), `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DECLARE $name AS Utf8;
INSERT INTO lists (tenant_id, id, name, description, visibility, optin, tags, created_at, updated_at)
VALUES ($tenant_id, $id, $name, ""u, "private"u, "single"u, "[]"u,
    CurrentUtcTimestamp(), CurrentUtcTimestamp());`,
		query.WithParameters(ydb.ParamsBuilder().
			Param("$tenant_id").Text(tenantID).
			Param("$id").Text(id).
			Param("$name").Text("L-"+dbtest.RandString()).
			Build())))
	return id
}

```

- [ ] **Step 2: Write the failing test**

In `internal/iam/adapters/roles_test.go`, insert above `// TestRolesAssignUnknownRoleFails pins`:

```go
// TestRolesAssignUnknownListFails pins the Postgres foreign key from a per-list
// assignment to its list: assigning a role on a list that does not exist
// fails and grants nothing. Postgres returns a wrapped foreign-key error; YDB
// returns domain.ErrListNotFound.
func TestRolesAssignUnknownListFails(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.roles(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		userID := newTenantUser(t, b, tenantID)
		roleID, err := repo.Add(ctx, tenantID, newRole(t, tenantID, "List editor"))
		require.NoError(t, err)

		err = repo.AssignListRole(ctx, tenantID, userID, uuid.NewString(), roleID)
		require.Error(t, err)
		if b.name == "ydb" {
			require.ErrorIs(t, err, domain.ErrListNotFound)
		}

		_, listPerms, err := repo.EffectiveFor(ctx, tenantID, userID)
		require.NoError(t, err)
		require.Empty(t, listPerms)
		// The role is not held, so it can be deleted.
		require.NoError(t, repo.Delete(ctx, tenantID, roleID))
	})
}
```

Run: `export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock" && go test ./internal/iam/adapters/ -run 'TestRolesAssignUnknownListFails' -count=1 -timeout 8m 2>&1 | grep -E '^\s*--- (PASS|FAIL)'`

Expected: `/pg` passes; `/ydb` FAILS (the assignment succeeds).

- [ ] **Step 3: Read the list in AssignListRole**

In `internal/iam/adapters/roles_ydb.go` (`AssignListRole`), replace the doc comment's last two sentences with:

```go
// on that list. It returns domain.ErrRoleNotFound when the tenant has no such
// role and domain.ErrListNotFound when it has no such list, standing in for
// the Postgres foreign keys.
```

and insert after the `ydbRoleExists` check, before the `UPSERT INTO user_list_roles` statement:

```go
		// lists belongs to audience. Reading it stands in for the Postgres
		// foreign key, and makes a concurrent YDBLists.Delete, which removes
		// the list's assignments, abort one of the two transactions.
		_, err := tx.QueryRow(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
SELECT id FROM lists
WHERE tenant_id = $tenant_id AND id = $id;`, ydbTenantAndID(tenantID, listID))
		if errors.Is(err, query.ErrNoRows) {
			return domain.ErrListNotFound
		}
		if err != nil {
			return fmt.Errorf("checking list: %w", err)
		}
```

- [ ] **Step 4: Run the tests to verify they pass, and commit**

Run: `export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock" && go vet ./internal/iam/... && go test ./internal/iam/... ./internal/audience/adapters/ -count=1 -timeout 8m`

Expected: every package `ok`. The other role tests that assign list roles now run against real YDB list rows.

Run: `golangci-lint run ./internal/iam/...`

Expected: `0 issues.`

```bash
gofmt -l internal/iam/
git add internal/iam/domain/errors.go internal/iam/adapters/roles_ydb.go
git add internal/iam/adapters/helpers_test.go internal/iam/adapters/roles_test.go
git commit -m "fix(iam): check the list when assigning a per-list role on YDB"
```

`gofmt -l` must print nothing. Afterwards, `git status --short` must print nothing.

- [ ] **Step 5: Full verification**

Run: `export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock" && go build ./... && go vet ./... && go test ./... -timeout 15m 2>&1 | grep -vE '^(ok|\?)\s'`

Expected: no output. In other words, no failing package.

Run: `golangci-lint run`

Expected: `0 issues.`

Run: `PATH="$HOME/go/bin:$PATH" make lint-arch`

Expected: the last line is `[arch] clean`.

Run: `git diff --stat 6e714f1 -- go.mod go.sum`

Expected: no output.

Run: `ls internal/audience/adapters/*_pg_test.go`

Expected: `No such file or directory`. Every audience suite is a pg+ydb contract suite now.

---

## Done criteria for Plan B6

- `go build ./... && go vet ./... && go test ./...` passes. Every eachBackend test in `internal/audience/adapters` and `internal/iam/adapters` passes on both `/pg` and `/ydb`, and `TestYDBJobsFilesArePrivate` passes.
- `golangci-lint run` reports `0 issues.`, and `make lint-arch` reports `[arch] clean`.
- No `go.mod` or `go.sum` change.
- No `*_pg_test.go` file is left in `internal/audience/adapters`.
- These tests pass on both backends, so YDB reproduces the Postgres behaviour that has no YDB construct:
  - `TestSubscriptionPagesAddGetUpdate`: the unique slug on insert and on rename.
  - `TestPendingSubscriptionsUpsertGetRefreshDelete` and `TestPendingSubscriptionsUpsertIsRaceFree`: `ON CONFLICT DO UPDATE ... RETURNING id`, and its atomicity.
  - `TestJobRepositoryExportLifecycle`: a NULL `file_bytes` reads back as no file.
  - `TestSubscribersGetByEmail`: the `citext` comparison of an exact lookup.
  - `TestRolesAssignUnknownListFails`: the `user_list_roles` foreign key to `lists`.
- The final report lists what stays open for the next plans:
  - **Plan C4 wiring.** The serverless composition root:
    - wires `NewYDBSubscriptionPages`, `NewYDBPendingSubscriptions` and `NewYDBJobs` in place of `NewSubscriptionPages`, `NewPendingSubscriptions` and `NewJobs` (`internal/service/application.go`, the audience block; `cmd/worker/main.go` today);
    - builds the `*s3.Client` for `NewYDBJobs` with the options `mediaadapters.NewS3BlobStore` uses (path-style, static keys from `cfg.ObjectStorageAccessKeyID` / `SecretAccessKey`, `cfg.ObjectStorageRegion`, `cfg.ObjectStorageEndpoint`), and passes `cfg.ObjectStorageBucket`.
  - **Plan C3.** The `api` and `worker` service accounts need read and write on the media bucket (import uploads come through `api`; the workers read and write job files). The bucket policy keeps anonymous read on `published/*` only.
  - **Accepted YDB/pg differences.** No contract test pins these:
    - Upserting a pending subscription for an unknown page: pg fails on the foreign key; YDB accepts it. The only writer loads the page first.
    - The confirmation-token hash is unique per tenant on YDB and globally on pg. A random 256-bit hash makes the difference unobservable.
    - Assigning a role on an unknown list: pg returns a wrapped foreign-key error; YDB returns `ErrListNotFound`.
    - `AddImport` with nil bytes: pg stores NULL and `StagedFile` returns nil; YDB stores an empty object and returns an empty slice. No caller passes nil.
    - An import stores at most its first 10000 row failures on YDB (a cell holds 16 MiB); `failed_count` stays exact. Postgres stores them all.
    - YDB accepts values the Postgres `CHECK` constraints forbid (`import_export_jobs.kind` and `status`). None is reachable through the domain constructors.
  - **Ceilings.** Listed under the spec's "Ceilings": job files in Object Storage are never deleted, as Postgres keeps `file_bytes`; a failed best-effort delete leaves an unreferenced private object.
