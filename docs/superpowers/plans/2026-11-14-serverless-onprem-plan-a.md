# Serverless On-Prem — Plan A (SaaS-safe groundwork) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:**
- Decouple every job worker from River's job envelope, so the serverless
  dispatcher (Plan C) can drive the same workers.
- Add a reusable local-YDB test container.
- Resolve the open questions V1, V2, V4, V5 and V6 of the design.

SaaS behavior does not change.

**Architecture:**
- **Worker signature.** Workers change from `Work(ctx, *river.Job[T])` to
  `Work(ctx, args T)`. One generic adapter, `jobs.AddWorker`, registers them
  with River. Snooze stays `river.JobSnooze`.
- **Local YDB in tests.** `dbtest.YDB(t)` starts or reuses one
  `ydbplatform/local-ydb` container, like `nvelope-test-pg`.
- **Spikes and audits.** Each records its findings in the design spec. No
  spike code is committed.

**Tech Stack:**
- Go 1.26
- River v0.37.1
- `ydb-go-sdk/v3` v3.136.4
- testcontainers-go v0.42.0
- testify

**Design spec:** [docs/superpowers/specs/2026-11-14-serverless-onprem-design.md](../specs/2026-11-14-serverless-onprem-design.md)

## Global Constraints

- **Branch.** Work on `feat/serverless-onprem`.
- **Dependencies.** Plan A adds no new Go module dependencies. `go.mod` changes
  only if `go mod tidy` moves an already-present module from indirect to direct.
- **No behavior change.** SaaS behavior is unchanged, and `go test ./...` must
  pass before and after every task.
- **Docker.** Integration tests need a running Docker daemon. Postgres comes
  from the reused `nvelope-test-pg` container.
- **Docker on this machine is OrbStack.** There is no `/var/run/docker.sock`,
  so testcontainers fails until you prefix every `go test`, `make test` and
  `make ci` with:
  `export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock" &&`.
  Ryuk reaps the test containers between runs, so each `go test` run starts
  fresh containers. Reuse by name works only within one run.
- **Local YDB image.** Pin `ydbplatform/local-ydb:26.3.1.23`. It is
  amd64-only: on Apple Silicon, OrbStack runs it under Rosetta, so startup is
  slower than native.
- **Paths.** Pass paths relative to the repo root. Never prefix a command
  with `cd <repo-root> &&`.
- **Commits.**
  - Use conventional commit messages (`refactor(jobs): ...`, `test(dbtest): ...`,
    `docs: ...`).
  - **No `Co-Authored-By` trailer** of any kind.
- **Tooling.**
  - `goimports` is not installed. Use `gofmt -w` and remove unused imports
    explicitly, as each step shows.
  - `golangci-lint` and `go-cleanarch` (`make lint-arch`) are on PATH.

---

### Task 1: `jobs.AddWorker` and the pilot conversion of `VerifyWorker`

**Files:**
- Create: `internal/platform/jobs/worker.go`
- Modify: `internal/sending/adapters/verify_worker.go:18-19,36-37`
- Modify: `internal/sending/adapters/verify_worker_test.go:38-42` and the snooze assertion in `TestVerifyWorkerSnoozesWhilePending` (around line 95)
- Modify: `cmd/worker/main.go` (the `NewVerifyWorker` registration line)

**Interfaces:**
- Produces (used by Task 2 and by Plan C):
  ```go
  // package jobs (internal/platform/jobs)
  type Worker[T river.JobArgs] interface {
      Work(ctx context.Context, args T) error
  }
  func AddWorker[T river.JobArgs](workers *river.Workers, w Worker[T])
  ```
- Produces: `(*sendingadapters.VerifyWorker).Work(ctx context.Context, args jobs.DomainVerifyArgs) error`

- [ ] **Step 1: Write the failing test**

In `internal/sending/adapters/verify_worker_test.go`, replace the helper at
lines 38–42:

```go
func verifyJob(tenantID, domainID string) *river.Job[jobs.DomainVerifyArgs] {
	return &river.Job[jobs.DomainVerifyArgs]{
		Args: jobs.DomainVerifyArgs{TenantID: tenantID, DomainID: domainID},
	}
}
```

with:

```go
func verifyJob(tenantID, domainID string) jobs.DomainVerifyArgs {
	return jobs.DomainVerifyArgs{TenantID: tenantID, DomainID: domainID}
}
```

In `TestVerifyWorkerSnoozesWhilePending`, tighten the assertion. Replace:

```go
	err = w.Work(ctx, verifyJob(tenantID, id))
	require.Error(t, err, "an unverified domain within the window snoozes")
```

with:

```go
	err = w.Work(ctx, verifyJob(tenantID, id))
	require.Equal(t, river.JobSnooze(time.Minute), err,
		"an unverified domain within the window snoozes for one interval")
```

The `river` import stays, because the test still uses `river.JobSnooze`.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/sending/adapters/ -run TestVerifyWorker`

Expected: a build failure like `cannot use verifyJob(tenantID, id) (value of
struct type jobs.DomainVerifyArgs) as *river.Job[jobs.DomainVerifyArgs] value`.

- [ ] **Step 3: Create the generic adapter**

Create `internal/platform/jobs/worker.go`:

```go
package jobs

import (
	"context"

	"github.com/riverqueue/river"
)

// Worker processes one job given only its decoded arguments. It does not know
// which queue runtime delivered the job: River drives it in SaaS mode (through
// AddWorker), and the serverless dispatcher drives it from a Message Queue
// trigger. To retry later, return river.JobSnooze(d) — both runtimes
// understand it.
type Worker[T river.JobArgs] interface {
	Work(ctx context.Context, args T) error
}

// AddWorker registers w with River under T's kind. Like river.AddWorker, it
// panics when the kind is already registered.
func AddWorker[T river.JobArgs](workers *river.Workers, w Worker[T]) {
	river.AddWorker[T](workers, &riverWorker[T]{w: w})
}

// riverWorker adapts a Worker to River's job-envelope signature.
type riverWorker[T river.JobArgs] struct {
	river.WorkerDefaults[T]
	w Worker[T]
}

// Work unwraps the River envelope and hands the args to the wrapped Worker.
func (r *riverWorker[T]) Work(ctx context.Context, job *river.Job[T]) error {
	return r.w.Work(ctx, job.Args)
}
```

- [ ] **Step 4: Convert `VerifyWorker`**

In `internal/sending/adapters/verify_worker.go`:

1. Delete the line `	river.WorkerDefaults[jobs.DomainVerifyArgs]` (line 19).
2. Update the doc comment on line 14. Change
   `// VerifyWorker is the River worker that polls` to
   `// VerifyWorker is the job worker that polls`.
3. Replace the signature and first line of `Work` (lines 36–37):

   ```go
   func (w *VerifyWorker) Work(ctx context.Context, job *river.Job[jobs.DomainVerifyArgs]) error {
   	tenantID, domainID := job.Args.TenantID, job.Args.DomainID
   ```

   with:

   ```go
   func (w *VerifyWorker) Work(ctx context.Context, args jobs.DomainVerifyArgs) error {
   	tenantID, domainID := args.TenantID, args.DomainID
   ```

The `river` import stays, because `river.JobSnooze` is still used on line 77.

In `cmd/worker/main.go`, change the registration:

```go
	river.AddWorker(workers, sendingadapters.NewVerifyWorker(sendingDomains, verifier,
		cfg.SendingDomainVerifyInterval, cfg.SendingDomainVerifyWindow))
```

to:

```go
	jobs.AddWorker(workers, sendingadapters.NewVerifyWorker(sendingDomains, verifier,
		cfg.SendingDomainVerifyInterval, cfg.SendingDomainVerifyWindow))
```

`jobs` is already imported in `cmd/worker/main.go`.

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go build ./... && go test ./internal/sending/... ./internal/platform/jobs/...`

Expected: build OK and all tests PASS, including
`TestVerifyWorkerSnoozesWhilePending`.

- [ ] **Step 6: Commit**

```bash
git add internal/platform/jobs/worker.go internal/sending/adapters/verify_worker.go \
  internal/sending/adapters/verify_worker_test.go cmd/worker/main.go
git commit -m "refactor(jobs): add runtime-agnostic Worker and convert VerifyWorker"
```

---

### Task 2: Convert the remaining 12 workers and their call sites

**Files:**
- Modify (workers):
  - `internal/audience/adapters/import_worker.go`
  - `internal/audience/adapters/export_worker.go`
  - `internal/audience/adapters/optin_worker.go`
  - `internal/auth/adapters/verification_worker.go`
  - `internal/auth/adapters/verification_cleanup_worker.go`
  - `internal/billing/adapters/charge_worker.go`
  - `internal/billing/adapters/rollup_worker.go`
  - `internal/billing/adapters/sweep_worker.go`
  - `internal/campaign/adapters/start_worker.go`
  - `internal/campaign/adapters/batch_worker.go`
  - `internal/deliverability/adapters/analytics_worker.go`
  - `internal/deliverability/adapters/feedback_worker.go`
- Modify (direct `Work` callers in tests):
  - `internal/audience/adapters/optin_worker_test.go`
  - `internal/auth/adapters/verification_worker_test.go`
  - `internal/auth/adapters/verification_cleanup_worker_test.go`
  - `internal/campaign/adapters/start_worker_test.go`
  - `internal/campaign/adapters/send_pipeline_test.go`
  - `internal/campaign/adapters/resumability_test.go`
  - `internal/deliverability/adapters/feedback_worker_test.go`
- Modify (River registration):
  - `cmd/worker/main.go`
  - `internal/audience/adapters/jobs_pg_test.go`
  - `internal/api/campaign_handlers_test.go`
  - `internal/api/jobs_handlers_test.go`

**Interfaces:**
- Consumes: `jobs.AddWorker` and `jobs.Worker[T]` from Task 1.
- Produces: every worker below exposes `Work(ctx context.Context, args jobs.<Kind>Args) error`:

  | Worker | Args type |
  |--------|-----------|
  | `ImportWorker` | `jobs.ImportArgs` |
  | `ExportWorker` | `jobs.ExportArgs` |
  | `OptinWorker` | `jobs.OptinSendArgs` |
  | `VerificationWorker` | `jobs.VerificationSendArgs` |
  | `VerificationCleanupWorker` | `jobs.VerificationCleanupArgs`, ignored as `_` |
  | `ChargeWorker` | `jobs.BillingChargeArgs` |
  | `RollupWorker` | `jobs.UsageRollupArgs` |
  | `SweepWorker` | `jobs.BillingSweepArgs`, ignored as `_` |
  | `StartWorker` | `jobs.CampaignStartArgs` |
  | `BatchWorker` | `jobs.CampaignBatchArgs` |
  | `AnalyticsWorker` | `jobs.AnalyticsRefreshArgs` |
  | `FeedbackWorker` | `jobs.FeedbackProcessArgs` |

  Plan C's dispatcher calls these exact signatures.

The change is the same three-part rewrite in every worker file. Every file has
exactly one `river.WorkerDefaults[jobs.X]` line, one `Work` signature taking
`job *river.Job[jobs.X]` (or `_ *river.Job[jobs.X]`), and reads arguments only
through `job.Args.`. Other identifiers named `job` in those files are local
domain objects, such as `importJob`, and never match `job.Args.`.

- [ ] **Step 1: Write the failing tests**

Rewrite every direct `Work` call in the seven test files, so that it passes the
args struct instead of a `*river.Job`:

```bash
perl -0777 -pi -e 's/&river\.Job\[jobs\.\w+\]\{\s*Args:\s*(jobs\.\w+\{.*?\}),\s*\}/$1/gs; s/\*river\.Job\[(jobs\.\w+)\]/$1/g' \
  internal/audience/adapters/optin_worker_test.go \
  internal/auth/adapters/verification_worker_test.go \
  internal/auth/adapters/verification_cleanup_worker_test.go \
  internal/campaign/adapters/start_worker_test.go \
  internal/campaign/adapters/send_pipeline_test.go \
  internal/campaign/adapters/resumability_test.go \
  internal/deliverability/adapters/feedback_worker_test.go
```

What the script does:
- The first substitution turns `&river.Job[jobs.X]{ Args: jobs.X{...}, }`
  into `jobs.X{...}`. None of these `Args` literals contain nested braces.
- The second substitution turns the helper return type
  `*river.Job[jobs.FeedbackProcessArgs]` in `feedback_worker_test.go` into
  `jobs.FeedbackProcessArgs`.

Two results to check by eye:

- `resumability_test.go` around line 81 now reads:

  ```go
  	job := jobs.CampaignBatchArgs{TenantID: f.tenantID, CampaignID: f.campaignID, Offset: 0, Limit: 100}
  ```

- `feedback_worker_test.go` lines 18–22 now read:

  ```go
  func feedbackJob(inboundEventID string) jobs.FeedbackProcessArgs {
  	return jobs.FeedbackProcessArgs{InboundEventID: inboundEventID}
  }
  ```

Confirm that no envelope is left in these files:

Run: `grep -n 'river\.Job\[' internal/*/adapters/*_test.go`

Expected: no output.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go vet ./internal/audience/adapters/ ./internal/auth/adapters/ ./internal/campaign/adapters/ ./internal/deliverability/adapters/`

Expected: build errors like `cannot use jobs.OptinSendArgs{…} … as
*river.Job[jobs.OptinSendArgs] value in argument to worker.Work`, plus
`"github.com/riverqueue/river" imported and not used` in test files that no
longer reference `river.`.

- [ ] **Step 3: Rewrite the 12 worker sources**

```bash
WORKERS="internal/audience/adapters/import_worker.go
internal/audience/adapters/export_worker.go
internal/audience/adapters/optin_worker.go
internal/auth/adapters/verification_worker.go
internal/auth/adapters/verification_cleanup_worker.go
internal/billing/adapters/charge_worker.go
internal/billing/adapters/rollup_worker.go
internal/billing/adapters/sweep_worker.go
internal/campaign/adapters/start_worker.go
internal/campaign/adapters/batch_worker.go
internal/deliverability/adapters/analytics_worker.go
internal/deliverability/adapters/feedback_worker.go"
perl -pi -e 's/^\s*river\.WorkerDefaults\[jobs\.\w+\]\n//; s/\bjob \*river\.Job\[(jobs\.\w+)\]/args $1/; s/\b_ \*river\.Job\[(jobs\.\w+)\]/_ $1/; s/\bjob\.Args\./args./g' $WORKERS
```

Spot-check one result.
`internal/campaign/adapters/batch_worker.go` lines 80–82 must read:

```go
func (w *BatchWorker) Work(ctx context.Context, args jobs.CampaignBatchArgs) error {
	tenantID, campaignID := args.TenantID, args.CampaignID
```

The `Pending` call further down must read
`w.recipients.Pending(ctx, tenantID, campaignID, args.Offset, args.Limit)`.

The cleanup worker signature in
`internal/auth/adapters/verification_cleanup_worker.go` spans two lines and
must now read:

```go
func (w *VerificationCleanupWorker) Work(ctx context.Context,
	_ jobs.VerificationCleanupArgs) error {
```

- [ ] **Step 4: Switch River registration to `jobs.AddWorker`**

```bash
perl -pi -e 's/\briver\.AddWorker\(/jobs.AddWorker(/g' \
  cmd/worker/main.go \
  internal/audience/adapters/jobs_pg_test.go \
  internal/api/campaign_handlers_test.go \
  internal/api/jobs_handlers_test.go
```

These four files keep their `river` import: they still call
`river.NewWorkers()`, and some call `river.InsertOpts`. They already import
`internal/platform/jobs`.

- [ ] **Step 5: Drop now-unused `river` imports and format**

```bash
for f in $WORKERS \
  internal/audience/adapters/optin_worker_test.go \
  internal/auth/adapters/verification_worker_test.go \
  internal/auth/adapters/verification_cleanup_worker_test.go \
  internal/campaign/adapters/start_worker_test.go \
  internal/campaign/adapters/send_pipeline_test.go \
  internal/campaign/adapters/resumability_test.go \
  internal/deliverability/adapters/feedback_worker_test.go; do
  grep -q 'river\.' "$f" || perl -ni -e 'print unless /^\s*"github.com\/riverqueue\/river"\s*$/' "$f"
done
gofmt -w $WORKERS cmd/worker/main.go internal/*/adapters/*_test.go internal/api/*_test.go
```

`WORKERS` comes from Step 3. Re-run that assignment first if this is a new
shell.

Two worker files must keep the import, because they still call
`river.JobSnooze`: `batch_worker.go` and, from Task 1, `verify_worker.go`.

Run: `grep -ln '"github.com/riverqueue/river"' internal/*/adapters/*_worker.go`

Expected: exactly `internal/campaign/adapters/batch_worker.go` and
`internal/sending/adapters/verify_worker.go`.

- [ ] **Step 6: Fix stale doc comments**

Run: `grep -n 'River worker' internal/*/adapters/*_worker.go`

On each hit, change `River worker` to `job worker`. These are doc-comment
wording only. Leave comments that describe River redelivery semantics, such as
"River redelivery is harmless", unchanged: SaaS still runs on River, and Plan C
broadens them to "redelivery".

- [ ] **Step 7: Run the full verification**

Run: `go build ./... && go vet ./... && go test ./...`

Expected: everything PASSES. The River-driven integration tests are
`internal/audience/adapters/jobs_pg_test.go` (import and export through a real
River client) and `internal/api/campaign_handlers_test.go` (start and batch
through a real River client). They pass, which proves that `jobs.AddWorker`
works end to end.

Run: `golangci-lint run && make lint-arch`

Expected: no new findings.

- [ ] **Step 8: Commit**

```bash
git add cmd/worker/main.go internal/
git commit -m "refactor(jobs): decouple all workers from the River job envelope"
```

---

### Task 3: `dbtest.YDB`, a reusable local-YDB test container

**Files:**
- Create: `internal/dbtest/ydb.go`
- Create: `internal/dbtest/ydb_test.go`
- Modify: `Makefile` (the `test-db-clean` target, lines 35–38)

**Interfaces:**
- Produces (used by Task 4 and by every YDB adapter test in Plan B):
  ```go
  // package dbtest
  func YDBDSN(t *testing.T) string    // grpc://host:port/local
  func YDB(t *testing.T) *ydb.Driver  // closed on t.Cleanup
  ```
- Env override: `NVELOPE_TEST_YDB_DSN`. When set, the helper uses it and
  starts no container.

- [ ] **Step 1: Write the failing test**

Create `internal/dbtest/ydb_test.go`:

```go
package dbtest_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
)

func TestYDBRoundTrip(t *testing.T) {
	t.Parallel()
	db := dbtest.YDB(t)

	row, err := db.Query().QueryRow(context.Background(), "SELECT 42 AS answer")
	require.NoError(t, err)

	var answer int32
	require.NoError(t, row.Scan(&answer))
	require.Equal(t, int32(42), answer)
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/dbtest/ -run TestYDBRoundTrip`

Expected: build failure `undefined: dbtest.YDB`.

- [ ] **Step 3: Implement the helper**

Create `internal/dbtest/ydb.go`:

```go
package dbtest

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	ydb "github.com/ydb-platform/ydb-go-sdk/v3"
	"github.com/ydb-platform/ydb-go-sdk/v3/balancers"
)

// ydbContainerName is the fixed name of the shared local-YDB test container.
// Like nvelope-test-pg, testcontainers' reuse feature keys on this name, so
// every test binary in a run attaches to the same container. Remove a leftover
// one with `make test-db-clean`.
const ydbContainerName = "nvelope-test-ydb"

// ydbImage pins the local-YDB image. It is amd64-only, so on Apple Silicon
// Docker runs it under emulation and startup takes noticeably longer.
const ydbImage = "ydbplatform/local-ydb:26.3.1.23"

// ydbReadyTimeout bounds how long startup may take before the gRPC endpoint
// serves queries. Emulated amd64 on Apple Silicon is the slow case.
const ydbReadyTimeout = 3 * time.Minute

var (
	ydbOnce sync.Once
	ydbDSN  string
	ydbErr  error
)

// YDBDSN resolves a YDB DSN for integration tests. If NVELOPE_TEST_YDB_DSN is
// set it is used verbatim; otherwise a local-ydb container is started — or an
// existing one reused — via testcontainers-go, which requires a running Docker
// daemon. The test fails when no YDB can be obtained.
func YDBDSN(t *testing.T) string {
	t.Helper()
	ydbOnce.Do(func() {
		if env := os.Getenv("NVELOPE_TEST_YDB_DSN"); env != "" {
			ydbDSN = env
			return
		}
		ydbDSN, ydbErr = startYDBContainer()
	})
	require.NoError(t, ydbErr)
	return ydbDSN
}

// YDB opens a driver against the test YDB and closes it when the test ends.
func YDB(t *testing.T) *ydb.Driver {
	t.Helper()
	db, err := openYDB(context.Background(), YDBDSN(t))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close(context.Background()) })
	return db
}

// openYDB connects with the SingleConn balancer. It skips endpoint
// discovery: local-ydb advertises its in-container address, which is not
// reachable through the mapped host port.
func openYDB(ctx context.Context, dsn string) (*ydb.Driver, error) {
	return ydb.Open(ctx, dsn, ydb.WithBalancer(balancers.SingleConn()))
}

// startYDBContainer starts the shared local-YDB container, reusing an existing
// one if it is already running, and waits until it serves queries.
func startYDBContainer() (string, error) {
	ctx := context.Background()
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:         ydbImage,
			ImagePlatform: "linux/amd64",
			ExposedPorts:  []string{"2136/tcp"},
			Env: map[string]string{
				"GRPC_PORT":                "2136",
				"YDB_USE_IN_MEMORY_PDISKS": "true",
			},
			WaitingFor: wait.ForListeningPort("2136/tcp").WithStartupTimeout(ydbReadyTimeout),
			Name:       ydbContainerName,
		},
		Started: true,
		Reuse:   true,
	})
	if err != nil {
		return "", fmt.Errorf("YDB integration tests require Docker: %w", err)
	}
	host, err := container.Host(ctx)
	if err != nil {
		return "", fmt.Errorf("resolving ydb host: %w", err)
	}
	port, err := container.MappedPort(ctx, "2136/tcp")
	if err != nil {
		return "", fmt.Errorf("resolving ydb port: %w", err)
	}
	dsn := fmt.Sprintf("grpc://%s:%s/local", host, port.Port())
	if err := waitYDBReady(ctx, dsn); err != nil {
		return "", err
	}
	return dsn, nil
}

// waitYDBReady polls until a trivial query succeeds. The gRPC port opens
// before the database finishes initialising, so a listening port alone is not
// readiness.
func waitYDBReady(ctx context.Context, dsn string) error {
	deadline := time.Now().Add(ydbReadyTimeout)
	var lastErr error
	for time.Now().Before(deadline) {
		attemptCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		db, err := openYDB(attemptCtx, dsn)
		if err == nil {
			err = db.Query().Exec(attemptCtx, "SELECT 1")
			_ = db.Close(context.Background())
		}
		cancel()
		if err == nil {
			return nil
		}
		lastErr = err
		time.Sleep(time.Second)
	}
	return fmt.Errorf("local YDB not ready after %s: %w", ydbReadyTimeout, lastErr)
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/dbtest/ -run TestYDBRoundTrip -v -timeout 5m`

Expected: PASS. The first run pulls the image and may take a few minutes.
Later runs skip the pull but still start a fresh container: Ryuk reaps it
between runs on this machine.

If startup fails with an emulation or CPU-instruction error, stop. Report the
error verbatim instead of changing the image: picking another image or tag is
a decision for the user.

- [ ] **Step 5: Extend `make test-db-clean`**

In `Makefile`, replace:

```make
# Integration tests reuse a single testcontainers-managed Postgres container
# that persists between runs for speed. test-db-clean removes it.
test-db-clean:
	docker rm -f nvelope-test-pg
```

with:

```make
# Integration tests share testcontainers-managed Postgres and local-YDB
# containers, reused by name. test-db-clean removes any that are left over.
test-db-clean:
	docker rm -f nvelope-test-pg nvelope-test-ydb 2>/dev/null || true
```

Run: `make test-db-clean && go test ./internal/dbtest/ -run TestYDBRoundTrip -timeout 5m`

Expected: `test-db-clean` exits 0 even when no container exists, then the test
PASSES on a fresh container.

- [ ] **Step 6: Tidy and commit**

Run: `go mod tidy && git diff --stat go.mod go.sum`

Expected: no new modules. At most, an existing module moves from indirect to
direct.

```bash
git add internal/dbtest/ydb.go internal/dbtest/ydb_test.go Makefile go.mod go.sum
git commit -m "test(dbtest): add a reusable local-YDB test container"
```

---

### Task 4: Spike V1 and V4, YQL capabilities against local YDB

**Files:**
- Create, then delete (never commit): `internal/dbtest/ydb_spike_test.go`
- Modify: `docs/superpowers/specs/2026-11-14-serverless-onprem-design.md`. Append a `## Spike findings (Plan A)` section before `## Ceilings`.

**Interfaces:**
- Consumes: `dbtest.YDB(t)` from Task 3.
- Produces: written findings that Plan B depends on:
  - V1: is `ILIKE` usable?
  - V4: can one query-service `Exec` run a multi-statement DDL file with a
    TTL table and a `GLOBAL UNIQUE SYNC` index? Does a duplicate violate the
    unique index? Do `UPSERT ... RETURNING` and `CurrentUtcTimestamp()` work?

- [ ] **Step 1: Write the spike test**

Create `internal/dbtest/ydb_spike_test.go`:

```go
package dbtest_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
)

// Throwaway spike for design questions V1 and V4. Never committed.
func TestSpikeYQL(t *testing.T) {
	db := dbtest.YDB(t)
	ctx := context.Background()
	q := db.Query()

	// YQL string literals: a `u` suffix makes them Utf8 to match the column type.
	_ = q.Exec(ctx, "DROP TABLE IF EXISTS spike_users;")
	_ = q.Exec(ctx, "DROP TABLE IF EXISTS spike_counters;")

	t.Run("V4 multi-statement DDL with TTL and unique index", func(t *testing.T) {
		err := q.Exec(ctx, `
			CREATE TABLE spike_users (
				id Utf8 NOT NULL,
				email Utf8 NOT NULL,
				expire_at Timestamp NOT NULL,
				PRIMARY KEY (id),
				INDEX idx_email GLOBAL UNIQUE SYNC ON (email)
			) WITH (TTL = Interval("PT0S") ON expire_at);
			CREATE TABLE spike_counters (
				key Utf8 NOT NULL,
				count Uint64 NOT NULL,
				PRIMARY KEY (key)
			);`)
		require.NoError(t, err)
	})

	t.Run("V4 unique index rejects a duplicate email", func(t *testing.T) {
		require.NoError(t, q.Exec(ctx, `INSERT INTO spike_users (id, email, expire_at)
			VALUES ("1"u, "a@example.com"u, CurrentUtcTimestamp() + Interval("P1D"));`))
		err := q.Exec(ctx, `INSERT INTO spike_users (id, email, expire_at)
			VALUES ("2"u, "a@example.com"u, CurrentUtcTimestamp() + Interval("P1D"));`)
		require.Error(t, err)
		t.Logf("duplicate-email error (record its status code): %v", err)
	})

	t.Run("V4 INSERT of an existing primary key fails", func(t *testing.T) {
		err := q.Exec(ctx, `INSERT INTO spike_users (id, email, expire_at)
			VALUES ("1"u, "b@example.com"u, CurrentUtcTimestamp() + Interval("P1D"));`)
		require.Error(t, err)
		t.Logf("duplicate-key error (record its status code): %v", err)
	})

	t.Run("V4 UPSERT RETURNING", func(t *testing.T) {
		row, err := q.QueryRow(ctx, `UPSERT INTO spike_counters (key, count)
			VALUES ("k"u, 7ul) RETURNING count;`)
		require.NoError(t, err)
		var n uint64
		require.NoError(t, row.Scan(&n))
		require.Equal(t, uint64(7), n)
	})

	t.Run("V4 CurrentUtcTimestamp", func(t *testing.T) {
		row, err := q.QueryRow(ctx, `SELECT CurrentUtcTimestamp() AS now;`)
		require.NoError(t, err)
		var now time.Time
		require.NoError(t, row.Scan(&now))
		require.WithinDuration(t, time.Now(), now, time.Minute)
	})

	t.Run("V1 ILIKE", func(t *testing.T) {
		row, err := q.QueryRow(ctx, `SELECT "Alice@Example.com" ILIKE "alice%" AS hit;`)
		require.NoError(t, err)
		var hit bool
		require.NoError(t, row.Scan(&hit))
		require.True(t, hit)
	})
}
```

- [ ] **Step 2: Run the spike**

Run: `go test ./internal/dbtest/ -run TestSpikeYQL -v -timeout 5m`

Record each subtest's outcome: PASS, or FAIL with the exact error text. Also
record the two logged error messages, which carry the status codes Plan B maps
to "already exists".

**Fallback A.** If the multi-statement DDL subtest fails because of the
statement count, retry it once, splitting it into two `q.Exec` calls, one per
`CREATE TABLE`. Record whether single-statement DDL works.

**Fallback B.** If `ILIKE` fails, run
`SELECT String::AsciiToLower("Alice@Example.com") LIKE "alice%" AS hit;` and
record whether that works instead. It is the `email_lower` fallback written as
a function.

- [ ] **Step 3: Record the findings in the spec**

In `docs/superpowers/specs/2026-11-14-serverless-onprem-design.md`, insert this
section immediately before `## Ceilings`. Fill each `<result>` with what Step 2
produced, verbatim error text included; never leave a `<result>` in the file.

```markdown
## Spike findings (Plan A)

Run against `ydbplatform/local-ydb:26.3.1.23`.

### V1 — `ILIKE`

<result: PASS, or FAIL with the error; if it failed, the String::AsciiToLower result>

Decision: <subscriber search uses ILIKE | subscriber search uses String::AsciiToLower(email) LIKE … | an email_lower column is needed>

### V4 — DDL, uniqueness, RETURNING, CurrentUtcTimestamp

- Multi-statement DDL in one `Exec`: <result>
- TTL + inline `GLOBAL UNIQUE SYNC` index in `CREATE TABLE`: <result>
- Duplicate unique-index value error: `<logged error text>`
- Duplicate primary key on `INSERT` error: `<logged error text>`
- `UPSERT ... RETURNING`: <result>
- `CurrentUtcTimestamp()` scan into `time.Time`: <result>

Decision: <in-repo runner, one Exec per file | in-repo runner, one Exec per statement | goose v3.30.1 ydb dialect>
```

Choose the V4 decision this way:
- If multi-statement `Exec` works: in-repo runner, one `Exec` per file.
- If only single-statement `Exec` works: in-repo runner, one `Exec` per
  statement, with files split on lines that end in `;`.
- If DDL through the query service fails entirely: `goose`.

- [ ] **Step 4: Delete the spike and commit the findings**

```bash
rm internal/dbtest/ydb_spike_test.go
git status --short
```

Expected: only the spec is modified.

```bash
git add docs/superpowers/specs/2026-11-14-serverless-onprem-design.md
git commit -m "docs: record YQL spike findings (V1, V4) for serverless on-prem"
```

---

### Task 5: Spike V2 and V5, Terraform provider coverage and serverless YDB limits

**Files:**
- Modify: `docs/superpowers/specs/2026-11-14-serverless-onprem-design.md`. Append two subsections to `## Spike findings (Plan A)`.

**Interfaces:**
- Produces: written findings that Plan C (Terraform) and Plan B (paging)
  depend on.

- [ ] **Step 1: Check provider resources (V2)**

The yandex provider docs live in the provider repo:
`https://raw.githubusercontent.com/yandex-cloud/terraform-provider-yandex/master/docs/resources/<name>.md`.
Fetch each of these and record whether it exists:

| Resource | What to confirm |
|----------|-----------------|
| `function_trigger` | Can it target a Serverless Container (a `container { id, service_account_id }` block)? Which of `message_queue`, `timer` and `data_streams` work with a container target? For `data_streams`, does it accept a YDB-database topic (stream name plus `database`)? |
| `ydb_database_serverless` | exists; attribute holding the database path and endpoint |
| `ydb_topic` | exists; can it create a consumer? |
| `message_queue` | exists; `redrive_policy` (DLQ) and `visibility_timeout_seconds` attributes |
| `serverless_container` | exists; `concurrency`, `execution_timeout`, `provision_policy`, secret mounts from Lockbox |
| `api_gateway` | exists; inline `spec` with `x-yc-apigateway-integration: serverless_containers` |
| `cm_certificate` | exists; managed Let's Encrypt with DNS challenge |
| `lockbox_secret`, `lockbox_secret_version` | exist |
| anything matching `postbox` | search the `docs/resources/` index page: `https://github.com/yandex-cloud/terraform-provider-yandex/tree/master/docs/resources` |

Run: none (documentation lookup).

Expected: a filled table for each row.

- [ ] **Step 2: Check serverless YDB limits (V5)**

Fetch the YDB limits documentation. Start at
`https://yandex.cloud/en/docs/ydb/concepts/limits`. If that page has moved,
follow links from `https://yandex.cloud/en/docs/ydb/` to "Quotas and limits"
and "Serverless". Record, for serverless mode:
- the maximum rows or bytes returned per query result;
- the maximum query or transaction execution time;
- the maximum transaction size, or number of changed rows;
- request-unit throttling defaults: provisioned RU/s, and what happens when it
  is exceeded.

Run: none (documentation lookup).

Expected: each limit with its number and the source URL.

- [ ] **Step 3: Record the findings in the spec**

Append to `## Spike findings (Plan A)`, filling every cell from Steps 1–2 with
no placeholders left:

```markdown
### V2 — Terraform provider coverage

| Resource | Exists | Notes (exact attribute names, gaps) |
|----------|--------|-------------------------------------|
| … one row per resource from Step 1 … |

Decision: <Postbox address and configuration set in Terraform | Postbox is a documented manual README step>. Triggers that cannot target a container: <none | list, with the workaround>.

### V5 — Serverless YDB limits

| Limit | Value | Source |
|-------|-------|--------|
| … one row per limit from Step 2 … |

Decision for Plan B: <export and recipient resolution page through results with page size N | streaming via Query().Query is sufficient>.
```

- [ ] **Step 4: Commit**

```bash
git add docs/superpowers/specs/2026-11-14-serverless-onprem-design.md
git commit -m "docs: record Terraform provider and serverless YDB limit findings (V2, V5)"
```

---

### Task 6: Audit V6, retry-safety of transaction closures

**Files:**
- Modify: `docs/superpowers/specs/2026-11-14-serverless-onprem-design.md`. Append `### V6 — retry-safety audit` to `## Spike findings (Plan A)`.

**Interfaces:**
- Produces: a list of closures that are unsafe under YDB's automatic
  transaction retry, each with the fix Plan B must apply.

Background: `ydb-go-sdk` `DoTx` re-runs the whole closure on `ABORTED` or
`UNAVAILABLE`. A closure is **retry-safe** when running it twice leaves the
same outcome as running it once. That rules out two things inside a closure:

- **(a) External I/O.** A call on an enqueuer, mailer, messenger, Postbox
  client, verifier, blob store, HTTP client, or `time.Sleep`.
- **(b) Non-idempotent mutation of captured state.** `append` to an outer
  slice, `+=` or `++` on an outer counter, sending on a channel, or closing a
  channel.

Plain assignment to an outer variable (`snooze = true`, `created = x`) is safe:
the last run wins. A `time.Now()` captured **outside** the closure is safe.
Calling `time.Now()` **inside** is safe for correctness, but the plan must note
it, because a retry produces a later timestamp.

- [ ] **Step 1: List the call sites**

Run:

```bash
grep -rnE '\.(Update|Mutate|With|Modify)[A-Za-z]*\(ctx,' --include='*.go' internal \
  | grep -vE '_test.go|_pg.go|tenantdb|\.gen\.go|internal/api/problem.go'
```

Expected: about 40 lines across `*/app/command/*.go` and `*/adapters/*_worker.go`.
These are the UpdateFn closures the app hands to repositories. Under YDB they
run inside `DoTx`.

Run:

```bash
grep -nE 'tenantdb\.WithTenant|pool\.Begin|BeginFunc|BeginTx' internal/*/adapters/*_pg.go | wc -l
grep -nE 'tenantdb\.WithTenant|pool\.Begin|BeginFunc|BeginTx' internal/*/adapters/*_pg.go
```

Expected: about 129 lines. These are the adapters' own transaction bodies.

- [ ] **Step 2: Classify each UpdateFn closure**

For each site from the first command, open the file at the line. Read the
closure literal passed as the last argument, and classify it as `safe`,
`unsafe-io`, `unsafe-mutation`, or `safe-time-inside`, using the rules above.
`billing` sites are out of serverless scope: classify them anyway, but mark
them `billing (not ported)`.

- [ ] **Step 3: Scan the pg adapter transaction bodies**

For each site from the second command, read the transaction closure. Flag any
statement that uses something other than `tx`, local variables, or pure
functions. For example: a struct field that is a client, a call to an injected
port, or a `log`/`slog` call. A log line is harmless, but note it because it
will repeat on retry. These closures are rewritten in Plan B anyway. The
point is to catch any that **embed** a side effect the rewrite must move out
of the transaction.

- [ ] **Step 4: Record the findings in the spec**

Append to `## Spike findings (Plan A)`, listing every non-`safe` site
explicitly, with no placeholders left:

```markdown
### V6 — retry-safety audit

Scanned: <N> UpdateFn closures outside pg adapters; <M> transaction bodies in pg adapters.

| Site (file:line) | Class | What repeats on retry | Fix in Plan B |
|------------------|-------|-----------------------|---------------|
| … one row per non-safe site … |

All other sites are `safe`.
```

If every site is `safe`, the table is replaced with the single line
`No unsafe sites found.`. Keep the `Scanned:` line in either case.

- [ ] **Step 5: Commit**

```bash
git add docs/superpowers/specs/2026-11-14-serverless-onprem-design.md
git commit -m "docs: record transaction-closure retry-safety audit (V6)"
```

---

## Done criteria for Plan A

- `go build ./... && go vet ./... && go test ./...` passes.
- `golangci-lint run` and `make lint-arch` report nothing new.
- No `*.go` file outside `internal/platform/jobs/` references `river.Job[`:

  Run: `grep -rn 'river\.Job\[' --include='*.go' internal cmd | grep -v internal/platform/jobs/`

  Expected: no output.
- The spec has a `## Spike findings (Plan A)` section covering V1, V2, V4, V5
  and V6, with no `<...>` placeholders. Every decision line is filled in.
- Plans B and C are written next, from the updated spec.
