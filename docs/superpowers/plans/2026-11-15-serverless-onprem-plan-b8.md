# Serverless On-Prem — Plan B8 (deliverability on YDB) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:**
- Port the `deliverability` context to YDB: `YDBEvents`, `YDBSuppressions`, `YDBSettings`, `YDBAnalytics`, `YDBSuppressionChecker` and `YDBAuditLog` behind the existing domain and app ports, held to the same contract tests as their Postgres counterparts.
- Add the one port method the feedback recovery sweep needs, `EventRepository.PendingInbound`, on both backends, and the use case that calls it.

**Architecture:**
- **Migration.** One file, `internal/db/ydbmigrations/000009_deliverability.up.sql`, creates `inbound_feedback_events` (the one table without a tenant), `delivery_events`, `suppression_list`, `bounce_settings` and `campaign_analytics`. B7 already created the provider-message-id indexes that attribution reads.
- **Adapters.** The YDB adapters sit next to the pg adapters in `internal/deliverability/adapters` and call `db.Query()` / `DoTx` directly, like the B5 to B7 adapters. `ydb_common.go` holds two small helpers. The pg helper `nullString` (`events_pg.go`) is reused.
- **Idempotent writes.** `StageInbound`, `RecordEvent` and `Upsert` read through a unique index first and insert only when the key is absent, in one `DoTx`: a conflicting `INSERT` aborts the transaction (spec "Plan B7 planning spike").
- **Analytics refresh.** One `UPSERT ... SELECT` over two named `GROUP BY` subqueries and two `LEFT JOIN`s (spec "Plan B8 planning spike").
- **Feedback recovery.** `EventRepository.PendingInbound` plus `command.SweepPendingFeedbackHandler`. C4 wires the cron route.
- **Contract tests.** The deliverability adapter tests become pg+ydb suites through an `eachBackend` harness in `backend_test.go`, built up over Tasks 1 to 3, as in campaign.

**Tech Stack:**
- Go 1.26
- `github.com/ydb-platform/ydb-go-sdk/v3` v3.136.4 (query service, `table/types` for list parameters)
- testcontainers-go, local-ydb `26.3.1.23`
- testify, `github.com/google/uuid`

**Design spec:** [docs/superpowers/specs/2026-11-14-serverless-onprem-design.md](../specs/2026-11-14-serverless-onprem-design.md).
- Binding sections:
  - "Storage layer (YDB)": Transactions; Schema and migrations; SQL translation rules, including the rows Plan B8 adds.
  - "Spike findings (Plan A)": V5 and V6.
  - "Plan B2 planning spike" to "Plan B8 planning spike".
  - "Ceilings".
- **Dry run.** During planning, every file in this plan was applied stage by stage to the worktree, then reverted. Stage N is Task N below.
  - After each stage, `go build ./...`, `go vet`, `golangci-lint run` and the deliverability suites passed. Stage 1 also ran `internal/db`.
  - `StageInbound` and `RecordEvent` were sabotage-checked: with the existence read removed, their idempotence and race tests fail on YDB.
  - `TestEventsStageInboundIsRaceFree` and `TestEventsRecordEventIsRaceFree` passed repeated runs.
  - On the final state:
    - `go build ./... && go vet ./... && go test ./...` had no failing package (49 `ok` lines);
    - `golangci-lint run` reported `0 issues.`;
    - `make lint-arch` reported `[arch] clean`;
    - `go.mod` and `go.sum` were unchanged.

**Scope:**
- This is the eighth Plan B plan. Media (B9) follows.
- Not in scope:
  - Wiring the YDB adapters or the sweep into the composition root and the cron routes. That is Plan C4 (see "Done criteria").
  - `IngestNotificationHandler` and the workers: unchanged. `ProcessFeedback` was changed by one guard in the final review (see "Review follow-ups").
  - The Postgres adapters. The only change is the new `PendingInbound` method in `events_pg.go` (Task 2).
  - `suppression_list.source_event_id` always being NULL (see "Found, not fixed" in the spec's B8 section).

## Global Constraints

- **Branch.**
  - Work on `feat/serverless-onprem` in the worktree `/Users/nikthespirit/experiments/nvelope-wt/serverless-onprem`. Both already exist, and the branch starts at `5c296ad` plus the commit that adds this plan.
  - Do not create branches or worktrees. Do not push or merge.
  - The main checkout belongs to other sessions.
  - Never use bare `git stash` or `git stash pop`; the stash is shared.
- **Docker.**
  - Docker on this machine is OrbStack. Prefix every `go test` / `make test` / `make ci` with `export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock" && `.
  - Ryuk reaps test containers shortly after a run ends. A run that starts within that window can hang or fail with `No such container`; kill it (`pkill -f adapters.test`) and re-run.
  - The shared containers `nvelope-test-pg`, `nvelope-test-ydb`, `nvelope-test-s3-strict` and `nvelope-test-sqs` may belong to another session's live run. Never `docker rm` them.
- **YDB container.** It is `ydbplatform/local-ydb:26.3.1.23`, amd64 under Rosetta. A fresh container can take about 45 s to start, so use `-timeout 8m` on runs that touch YDB, and `-timeout 15m` for the full suite.
- **Known environmental flakes. None of these is caused by this plan:**
  - `internal/platform/ratelimit` `TestAllowIsConsistentUnderConcurrency` can fail with `redis eval failed: i/o timeout` under full-suite load. Re-run that package alone.
  - pg-backed packages can all fail at once with `no migration found for version N`, or with `unexpected postmaster exit`. That means another worktree's run is using the shared `nvelope-test-pg`. Re-run once it is gone.
  - `internal/api` `TestPublicSubscriptionCrossTenantIsolation` can fail on a reused pg database (a fixed `hash-a` token), `TestEmailVerificationsDeleteExpiredBefore` on a duplicate token hash, and `TestTransactionalSendWithValidKey` with a 500 (the fake messenger's constant `"msg-ref"` collides on the unique `provider_message_id`). Re-run once.
- **Dependencies.** No new Go modules and no `go.mod` change.
- **SaaS behaviour.** Postgres adapters, River, Redis and every existing test keep their behaviour. `go test ./...` passes after every task.
- **YQL conventions** (spec "SQL translation rules" and V4):
  - Every query `DECLARE`s each parameter: `DECLARE $id AS Utf8;`. A nullable column's parameter is `Optional<...>` and is bound with `BeginOptional()...EndOptional()` from a pointer.
  - Literals that target a `Utf8` column take the `u` suffix (`"pending"u`); a bare `"..."` is `String`.
  - Every DDL statement is idempotent (`CREATE TABLE IF NOT EXISTS`). A statement ends on a line ending in `;`. Never end a line with `;` inside a string literal or a `--` comment.
  - A secondary index is read only through an explicit `VIEW <index_name>`.
  - An `Int64` column scans into `int64`, not `int`.
- **Tenant scoping.** Every tenant-scoped table's primary key starts with `tenant_id`, and every query filters by the `tenantID` the port receives. The exceptions are the staging table `inbound_feedback_events` (no tenant yet) and `TenantForMessage`, which reads the global provider-message-id indexes on purpose.
- **IDs** are UUIDv4 strings generated in Go (`uuid.NewString()`) before any `DoTx`, and stored as `Utf8`.
- **No column defaults, no CHECK constraints, no foreign keys.** Every `INSERT` writes every `NOT NULL` column.
- **`DoTx` closures must be retry-safe** (V6):
  - only database operations inside;
  - ids and domain values are built before `DoTx`;
  - values the caller reads are assigned at the end of the closure, never accumulated across runs;
  - never `append` to a slice, or write to a map, declared outside the closure.
- **Commits.** Use conventional messages. **No `Co-Authored-By` trailer** of any kind.
- **Tooling.**
  - `goimports` is not installed, so use `gofmt -w`.
  - `golangci-lint` is on PATH. It lints test files too.
  - Run arch lint as `PATH="$HOME/go/bin:$PATH" make lint-arch`, with PATH set inline. Run git commands one per line, as the commit steps show.

## File structure

| File | Responsibility |
|------|----------------|
| `internal/db/ydbmigrations/000009_deliverability.up.sql` | the five tables and their indexes |
| `internal/deliverability/adapters/ydb_common.go` | `ydbTenantParam`, `deref` |
| `internal/deliverability/adapters/suppression_ydb.go` | `YDBSuppressions` and `YDBSuppressionChecker` |
| `internal/deliverability/adapters/settings_ydb.go` | `YDBSettings` |
| `internal/deliverability/adapters/audit_ydb.go` | `YDBAuditLog` |
| `internal/deliverability/adapters/events_ydb.go` | `YDBEvents`: staging, attribution, `RecordEvent`, `PendingInbound` |
| `internal/deliverability/adapters/analytics_ydb.go` | `YDBAnalytics` |
| `internal/deliverability/domain/repository.go`, `adapters/events_pg.go` | the `PendingInbound` port method and its pg implementation |
| `internal/deliverability/app/command/feedback.go` | `SweepPendingFeedbackHandler` |
| `internal/deliverability/adapters/backend_test.go` | the `backend` harness, built up over Tasks 1 to 3 |
| `internal/deliverability/adapters/helpers_test.go` | `seedTenant`, `newInbound`, `stageInbound` |
| `internal/deliverability/adapters/{suppression,events,analytics}_test.go`, `feedback_worker_test.go` | pg+ydb contract suites (the first three were `*_pg_test.go`) |

---

### Task 1: Migration, suppressions, settings, the pre-send checker and the audit log on YDB

**Files:**
- Create: `internal/db/ydbmigrations/000009_deliverability.up.sql`
- Create: `internal/deliverability/adapters/ydb_common.go`, `suppression_ydb.go`, `settings_ydb.go`, `audit_ydb.go`, `backend_test.go`
- Rename and rewrite: `suppression_pg_test.go` -> `suppression_test.go`

**Interfaces:**
- Consumes:
  - The ports `domain.SuppressionRepository`, `domain.SettingsRepository` (`internal/deliverability/domain/repository.go`), `command.AuditWriter` (`app/command/suppression.go`) and `campaigndomain.SuppressionChecker`, unchanged.
  - `domain.HydrateSuppressionEntry`, `domain.HydrateBounceSettings`, `domain.DefaultBounceSettings`.
  - From the pg adapters: `nullString(string) *string` (`events_pg.go`).
  - `dbtest.YDBMigrated`, `dbtest.AppPool`.
- Produces:
  ```go
  // package adapters
  func NewYDBSuppressions(d *ydb.Driver) *YDBSuppressions
  func NewYDBSuppressionChecker(d *ydb.Driver) *YDBSuppressionChecker
  func NewYDBSettings(d *ydb.Driver) *YDBSettings
  func NewYDBAuditLog(d *ydb.Driver) *YDBAuditLog
  ```
  and the `backend` harness with `suppressions`, `settings`, `checker`, `audit`, `newTenant` and `auditCount`.


- [ ] **Step 1: Create the migration.** It has no `DEFAULT`, no `CHECK` and no foreign key. All five tables are created here so that the later tasks add no migration.

```sql
-- Deliverability context (internal/deliverability): the staged inbound
-- delivery-feedback notifications, the attributed delivery events, the
-- tenant's suppression list and bounce settings, and the pre-computed campaign
-- analytics. No column has a DEFAULT, no CHECK and no foreign key: the adapters
-- write every value and the domain keeps them valid (see 000008_campaign).

-- inbound_feedback_events stages every notification before its tenant is
-- known, so it is the one table here without a tenant_id. raw_payload is JSON
-- that is only stored and read back whole, so it is a plain Utf8 column.
-- idx_inbound_dedupe is the idempotency key the consumer stages by.
-- idx_inbound_status serves the recovery sweep, which lists the notifications
-- still pending after a threshold.
CREATE TABLE IF NOT EXISTS inbound_feedback_events (
    id                  Utf8      NOT NULL,
    dedupe_key          Utf8      NOT NULL,
    event_kind          Utf8      NOT NULL,
    provider_message_id Utf8      NOT NULL,
    recipient_email     Utf8      NOT NULL,
    occurred_at         Timestamp NOT NULL,
    raw_payload         Utf8      NOT NULL,
    status              Utf8      NOT NULL,
    received_at         Timestamp NOT NULL,
    processed_at        Timestamp,
    PRIMARY KEY (id),
    INDEX idx_inbound_dedupe GLOBAL UNIQUE SYNC ON (dedupe_key),
    INDEX idx_inbound_status GLOBAL SYNC ON (status, received_at)
);

-- delivery_events holds the attributed feedback events. Exactly one of
-- campaign_recipient_id and transactional_message_id is set, which the domain
-- guarantees. A campaign's recipients go when their subscriber is deleted, so
-- campaign_recipient_id can dangle; nothing reads it. idx_delivery_events_inbound
-- makes RecordEvent idempotent on the inbound event.
CREATE TABLE IF NOT EXISTS delivery_events (
    tenant_id                Utf8      NOT NULL,
    id                       Utf8      NOT NULL,
    inbound_event_id         Utf8      NOT NULL,
    event_kind               Utf8      NOT NULL,
    recipient_email          Utf8      NOT NULL,
    campaign_id              Utf8,
    campaign_recipient_id    Utf8,
    transactional_message_id Utf8,
    provider_message_id      Utf8      NOT NULL,
    occurred_at              Timestamp NOT NULL,
    created_at               Timestamp NOT NULL,
    PRIMARY KEY (tenant_id, id),
    INDEX idx_delivery_events_inbound GLOBAL UNIQUE SYNC ON (tenant_id, inbound_event_id)
);

-- suppression_list: one row per suppressed address and tenant, which
-- idx_suppression_email enforces and the pre-send check reads. The List port
-- pages in id order, so the primary key is (tenant_id, id).
CREATE TABLE IF NOT EXISTS suppression_list (
    tenant_id       Utf8      NOT NULL,
    id              Utf8      NOT NULL,
    email           Utf8      NOT NULL,
    reason          Utf8      NOT NULL,
    source_event_id Utf8,
    suppressed_at   Timestamp NOT NULL,
    note            Utf8      NOT NULL,
    PRIMARY KEY (tenant_id, id),
    INDEX idx_suppression_email GLOBAL UNIQUE SYNC ON (tenant_id, email)
);

-- bounce_settings: a row is created lazily; until then the domain defaults
-- apply.
CREATE TABLE IF NOT EXISTS bounce_settings (
    tenant_id               Utf8      NOT NULL,
    suppress_on_hard_bounce Bool      NOT NULL,
    suppress_on_complaint   Bool      NOT NULL,
    updated_at              Timestamp NOT NULL,
    PRIMARY KEY (tenant_id)
);

-- campaign_analytics: the per-campaign roll-up the refresh job recomputes.
CREATE TABLE IF NOT EXISTS campaign_analytics (
    tenant_id        Utf8      NOT NULL,
    campaign_id      Utf8      NOT NULL,
    sent_count       Int64     NOT NULL,
    delivered_count  Int64     NOT NULL,
    opened_count     Int64     NOT NULL,
    clicked_count    Int64     NOT NULL,
    bounced_count    Int64     NOT NULL,
    complained_count Int64     NOT NULL,
    refreshed_at     Timestamp NOT NULL,
    PRIMARY KEY (tenant_id, campaign_id)
);
```


- [ ] **Step 2: Create the adapters.**


`internal/deliverability/adapters/ydb_common.go`:

```go
package adapters

import (
	ydb "github.com/ydb-platform/ydb-go-sdk/v3"
	"github.com/ydb-platform/ydb-go-sdk/v3/query"
)

// ydbTenantParam binds the $tenant_id parameter.
func ydbTenantParam(tenantID string) query.ExecuteOption {
	return query.WithParameters(ydb.ParamsBuilder().Param("$tenant_id").Text(tenantID).Build())
}

// deref returns the value of a nullable text column, "" for NULL.
func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
```


`internal/deliverability/adapters/suppression_ydb.go`:

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
	"github.com/ydb-platform/ydb-go-sdk/v3/table/types"

	campaigndomain "github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
	"github.com/nikolaymatrosov/nvelope/internal/deliverability/domain"
)

// YDBSuppressions is the YDB-backed implementation of
// domain.SuppressionRepository, for the serverless runtime. The address is
// unique per tenant through idx_suppression_email.
type YDBSuppressions struct {
	driver *ydb.Driver
}

var _ domain.SuppressionRepository = (*YDBSuppressions)(nil)

// NewYDBSuppressions builds a YDBSuppressions repository over the driver.
func NewYDBSuppressions(d *ydb.Driver) *YDBSuppressions {
	return &YDBSuppressions{driver: d}
}

// ydbSuppressionIDByEmail reads the id of the tenant's entry for an address.
func ydbSuppressionIDByEmail(ctx context.Context, tx query.TxActor, tenantID, email string) (string, bool, error) {
	row, err := tx.QueryRow(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $email AS Utf8;
SELECT id FROM suppression_list VIEW idx_suppression_email
WHERE tenant_id = $tenant_id AND email = $email;`,
		query.WithParameters(ydb.ParamsBuilder().
			Param("$tenant_id").Text(tenantID).
			Param("$email").Text(email).
			Build()))
	if errors.Is(err, query.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("reading suppression entry: %w", err)
	}
	var id string
	if err := row.Scan(&id); err != nil {
		return "", false, fmt.Errorf("scanning suppression entry: %w", err)
	}
	return id, true, nil
}

// Upsert adds a suppression entry; an address already suppressed for the
// tenant is a no-op. The transaction reads the address first, because a
// conflicting INSERT would abort it.
func (r *YDBSuppressions) Upsert(ctx context.Context, e *domain.SuppressionEntry) error {
	id := uuid.NewString()
	return r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		if _, found, err := ydbSuppressionIDByEmail(ctx, tx, e.TenantID(), e.Email()); err != nil || found {
			return err
		}
		if err := tx.Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DECLARE $email AS Utf8;
DECLARE $reason AS Utf8;
DECLARE $source_event_id AS Optional<Utf8>;
DECLARE $note AS Utf8;
INSERT INTO suppression_list (tenant_id, id, email, reason, source_event_id, suppressed_at, note)
VALUES ($tenant_id, $id, $email, $reason, $source_event_id, CurrentUtcTimestamp(), $note);`,
			query.WithParameters(ydb.ParamsBuilder().
				Param("$tenant_id").Text(e.TenantID()).
				Param("$id").Text(id).
				Param("$email").Text(e.Email()).
				Param("$reason").Text(string(e.Reason())).
				Param("$source_event_id").BeginOptional().Text(nullString(e.SourceEventID())).EndOptional().
				Param("$note").Text(e.Note()).
				Build())); err != nil {
			return fmt.Errorf("upserting suppression entry: %w", err)
		}
		return nil
	})
}

// Remove deletes a suppression entry, returning ErrSuppressionNotFound when no
// entry matches the address.
func (r *YDBSuppressions) Remove(ctx context.Context, tenantID, email string) error {
	email = strings.ToLower(strings.TrimSpace(email))
	return r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		id, found, err := ydbSuppressionIDByEmail(ctx, tx, tenantID, email)
		if err != nil {
			return err
		}
		if !found {
			return domain.ErrSuppressionNotFound
		}
		if err := tx.Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DELETE FROM suppression_list WHERE tenant_id = $tenant_id AND id = $id;`,
			query.WithParameters(ydb.ParamsBuilder().
				Param("$tenant_id").Text(tenantID).
				Param("$id").Text(id).
				Build())); err != nil {
			return fmt.Errorf("removing suppression entry: %w", err)
		}
		return nil
	})
}

// List returns a page of the tenant's suppression entries, ordered by id, and
// the cursor for the next page (empty when the page is the last).
func (r *YDBSuppressions) List(ctx context.Context, tenantID string, f domain.SuppressionFilter) (
	[]*domain.SuppressionEntry, string, error) {

	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	decl := "DECLARE $tenant_id AS Utf8;\nDECLARE $limit AS Uint64;\n"
	where := ""
	params := ydb.ParamsBuilder().
		Param("$tenant_id").Text(tenantID).
		Param("$limit").Uint64(uint64(limit + 1))
	if f.Reason != "" {
		decl += "DECLARE $reason AS Utf8;\n"
		where += " AND reason = $reason"
		params = params.Param("$reason").Text(string(f.Reason))
	}
	if f.EmailLike != "" {
		decl += "DECLARE $email_like AS Utf8;\n"
		where += ` AND email ILIKE "%"u || $email_like || "%"u`
		params = params.Param("$email_like").Text(f.EmailLike)
	}
	if f.Cursor != "" {
		decl += "DECLARE $cursor AS Utf8;\n"
		where += " AND id > $cursor"
		params = params.Param("$cursor").Text(f.Cursor)
	}
	rs, err := r.driver.Query().QueryResultSet(ctx, decl+
		`SELECT id, tenant_id, email, reason, source_event_id, suppressed_at, note
FROM suppression_list
WHERE tenant_id = $tenant_id`+where+`
ORDER BY id LIMIT $limit;`, query.WithParameters(params.Build()))
	if err != nil {
		return nil, "", fmt.Errorf("listing suppression entries: %w", err)
	}
	var out []*domain.SuppressionEntry
	for row, err := range rs.Rows(ctx) {
		if err != nil {
			return nil, "", fmt.Errorf("listing suppression entries: %w", err)
		}
		var id, tID, email, reason, note string
		var sourceEventID *string
		var suppressedAt time.Time
		if err := row.Scan(&id, &tID, &email, &reason, &sourceEventID, &suppressedAt, &note); err != nil {
			return nil, "", fmt.Errorf("scanning suppression entry: %w", err)
		}
		out = append(out, domain.HydrateSuppressionEntry(id, tID, email,
			domain.SuppressionReason(reason), deref(sourceEventID), suppressedAt, note))
	}

	nextCursor := ""
	if len(out) > limit {
		nextCursor = out[limit-1].ID()
		out = out[:limit]
	}
	return out, nextCursor, nil
}

// YDBSuppressionChecker is the YDB-backed pre-send gate consumed by the
// campaign send paths. It implements the campaign context's SuppressionChecker
// port over idx_suppression_email.
type YDBSuppressionChecker struct {
	driver *ydb.Driver
}

var _ campaigndomain.SuppressionChecker = (*YDBSuppressionChecker)(nil)

// NewYDBSuppressionChecker builds a YDBSuppressionChecker over the driver.
func NewYDBSuppressionChecker(d *ydb.Driver) *YDBSuppressionChecker {
	return &YDBSuppressionChecker{driver: d}
}

// Suppressed returns the subset of emails on the tenant's suppression list,
// mapped to the reason each was suppressed.
func (c *YDBSuppressionChecker) Suppressed(ctx context.Context, tenantID string,
	emails []string) (map[string]string, error) {

	if len(emails) == 0 {
		return map[string]string{}, nil
	}
	values := make([]types.Value, len(emails))
	for i, e := range emails {
		values[i] = types.TextValue(strings.ToLower(strings.TrimSpace(e)))
	}
	rs, err := c.driver.Query().QueryResultSet(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $emails AS List<Utf8>;
SELECT email, reason FROM suppression_list VIEW idx_suppression_email
WHERE tenant_id = $tenant_id AND email IN $emails;`,
		query.WithParameters(ydb.ParamsBuilder().
			Param("$tenant_id").Text(tenantID).
			Param("$emails").Any(types.ListValue(values...)).
			Build()))
	if err != nil {
		return nil, fmt.Errorf("checking suppression list: %w", err)
	}
	out := map[string]string{}
	for row, err := range rs.Rows(ctx) {
		if err != nil {
			return nil, fmt.Errorf("checking suppression list: %w", err)
		}
		var email, reason string
		if err := row.Scan(&email, &reason); err != nil {
			return nil, fmt.Errorf("scanning suppression entry: %w", err)
		}
		out[email] = reason
	}
	return out, nil
}
```


`internal/deliverability/adapters/settings_ydb.go`:

```go
package adapters

import (
	"context"
	"errors"
	"fmt"
	"time"

	ydb "github.com/ydb-platform/ydb-go-sdk/v3"
	"github.com/ydb-platform/ydb-go-sdk/v3/query"

	"github.com/nikolaymatrosov/nvelope/internal/deliverability/domain"
)

// YDBSettings is the YDB-backed implementation of domain.SettingsRepository,
// for the serverless runtime.
type YDBSettings struct {
	driver *ydb.Driver
}

var _ domain.SettingsRepository = (*YDBSettings)(nil)

// NewYDBSettings builds a YDBSettings repository over the driver.
func NewYDBSettings(d *ydb.Driver) *YDBSettings {
	return &YDBSettings{driver: d}
}

// Get returns the tenant's bounce settings, or DefaultBounceSettings when no
// row exists.
func (r *YDBSettings) Get(ctx context.Context, tenantID string) (*domain.BounceSettings, error) {
	row, err := r.driver.Query().QueryRow(ctx, `DECLARE $tenant_id AS Utf8;
SELECT suppress_on_hard_bounce, suppress_on_complaint, updated_at
FROM bounce_settings WHERE tenant_id = $tenant_id;`, ydbTenantParam(tenantID))
	if errors.Is(err, query.ErrNoRows) {
		return domain.DefaultBounceSettings(tenantID), nil
	}
	if err != nil {
		return nil, fmt.Errorf("loading bounce settings: %w", err)
	}
	var suppressHard, suppressComplaint bool
	var updatedAt time.Time
	if err := row.Scan(&suppressHard, &suppressComplaint, &updatedAt); err != nil {
		return nil, fmt.Errorf("scanning bounce settings: %w", err)
	}
	return domain.HydrateBounceSettings(tenantID, suppressHard, suppressComplaint, updatedAt), nil
}

// Put upserts the tenant's bounce settings.
func (r *YDBSettings) Put(ctx context.Context, tenantID string, s *domain.BounceSettings) error {
	if err := r.driver.Query().Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $suppress_on_hard_bounce AS Bool;
DECLARE $suppress_on_complaint AS Bool;
UPSERT INTO bounce_settings (tenant_id, suppress_on_hard_bounce, suppress_on_complaint, updated_at)
VALUES ($tenant_id, $suppress_on_hard_bounce, $suppress_on_complaint, CurrentUtcTimestamp());`,
		query.WithParameters(ydb.ParamsBuilder().
			Param("$tenant_id").Text(tenantID).
			Param("$suppress_on_hard_bounce").Bool(s.SuppressOnHardBounce()).
			Param("$suppress_on_complaint").Bool(s.SuppressOnComplaint()).
			Build())); err != nil {
		return fmt.Errorf("upserting bounce settings: %w", err)
	}
	return nil
}
```


`internal/deliverability/adapters/audit_ydb.go`:

```go
package adapters

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	ydb "github.com/ydb-platform/ydb-go-sdk/v3"
	"github.com/ydb-platform/ydb-go-sdk/v3/query"
)

// YDBAuditLog records privileged deliverability actions in the shared
// audit_log table (created by the iam migration). It implements the command
// layer's AuditWriter port.
type YDBAuditLog struct {
	driver *ydb.Driver
}

// NewYDBAuditLog builds a YDBAuditLog writer over the driver.
func NewYDBAuditLog(d *ydb.Driver) *YDBAuditLog {
	return &YDBAuditLog{driver: d}
}

// Record appends one audit entry. Manual suppression and bounce-setting
// changes are always operator actions made through a workspace session.
func (a *YDBAuditLog) Record(ctx context.Context, tenantID, actorID, action, target string) error {
	if err := a.driver.Query().Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DECLARE $actor_id AS Utf8;
DECLARE $action AS Utf8;
DECLARE $target AS Utf8;
INSERT INTO audit_log (tenant_id, created_at, id, actor_id, actor_kind, action, target, metadata)
VALUES ($tenant_id, CurrentUtcTimestamp(), $id, $actor_id, "session"u, $action, $target, "{}"u);`,
		query.WithParameters(ydb.ParamsBuilder().
			Param("$tenant_id").Text(tenantID).
			Param("$id").Text(uuid.NewString()).
			Param("$actor_id").Text(actorID).
			Param("$action").Text(action).
			Param("$target").Text(target).
			Build())); err != nil {
		return fmt.Errorf("recording audit entry: %w", err)
	}
	return nil
}
```


- [ ] **Step 3: Rename the suppression test and create the harness.** The harness holds the first six fields; Tasks 2 and 3 add the rest.

```bash
git mv internal/deliverability/adapters/suppression_pg_test.go internal/deliverability/adapters/suppression_test.go
```

`internal/deliverability/adapters/backend_test.go`:

```go
package adapters_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	ydb "github.com/ydb-platform/ydb-go-sdk/v3"
	"github.com/ydb-platform/ydb-go-sdk/v3/query"

	campaigndomain "github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
	"github.com/nikolaymatrosov/nvelope/internal/deliverability/adapters"
	"github.com/nikolaymatrosov/nvelope/internal/deliverability/app/command"
	"github.com/nikolaymatrosov/nvelope/internal/deliverability/domain"
	"github.com/nikolaymatrosov/nvelope/internal/platform/tenantdb"
)

// backend is one storage implementation of the deliverability repositories
// under contract test. Every contract test body runs once per backend, so the
// Postgres and YDB adapters are held to the same behaviour.
type backend struct {
	name         string
	suppressions func(t *testing.T) domain.SuppressionRepository
	settings     func(t *testing.T) domain.SettingsRepository
	checker      func(t *testing.T) campaigndomain.SuppressionChecker
	audit        func(t *testing.T) command.AuditWriter
	// The seeds below create rows owned by other contexts. On Postgres they
	// are real rows, because the delivery tables carry foreign keys to them.
	// YDB has no foreign keys, so a bare UUID stands in for a tenant.
	newTenant func(t *testing.T) string
	// The probes read what no port exposes.
	auditCount func(t *testing.T, tenantID string) int
}

var backends = []backend{
	{
		name: "pg",
		suppressions: func(t *testing.T) domain.SuppressionRepository {
			return adapters.NewSuppressions(dbtest.AppPool(t))
		},
		settings: func(t *testing.T) domain.SettingsRepository {
			return adapters.NewSettings(dbtest.AppPool(t))
		},
		checker: func(t *testing.T) campaigndomain.SuppressionChecker {
			return adapters.NewSuppressionChecker(dbtest.AppPool(t))
		},
		audit: func(t *testing.T) command.AuditWriter {
			return adapters.NewAuditLog(dbtest.AppPool(t))
		},
		newTenant: func(t *testing.T) string { return seedTenant(t, dbtest.AppPool(t)) },
		auditCount: func(t *testing.T, tenantID string) int {
			return pgCount(t, dbtest.AppPool(t), tenantID, "SELECT count(*) FROM audit_log")
		},
	},
	{
		name: "ydb",
		suppressions: func(t *testing.T) domain.SuppressionRepository {
			return adapters.NewYDBSuppressions(dbtest.YDBMigrated(t))
		},
		settings: func(t *testing.T) domain.SettingsRepository {
			return adapters.NewYDBSettings(dbtest.YDBMigrated(t))
		},
		checker: func(t *testing.T) campaigndomain.SuppressionChecker {
			return adapters.NewYDBSuppressionChecker(dbtest.YDBMigrated(t))
		},
		audit: func(t *testing.T) command.AuditWriter {
			return adapters.NewYDBAuditLog(dbtest.YDBMigrated(t))
		},
		newTenant: func(*testing.T) string { return uuid.NewString() },
		auditCount: func(t *testing.T, tenantID string) int {
			return ydbCount(t, `DECLARE $tenant_id AS Utf8;
SELECT COUNT(*) FROM audit_log WHERE tenant_id = $tenant_id;`, tenantID)
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

// pgCount reads one count inside the tenant-bound transaction.
func pgCount(t *testing.T, pool *pgxpool.Pool, tenantID, sql string) int {
	t.Helper()
	var n int
	require.NoError(t, tenantdb.WithTenant(context.Background(), pool, tenantID,
		func(ctx context.Context, tx pgx.Tx) error {
			return tx.QueryRow(ctx, sql).Scan(&n)
		}))
	return n
}

// ydbCount runs a single-row COUNT(*) query that takes $tenant_id.
func ydbCount(t *testing.T, q, tenantID string) int {
	t.Helper()
	row, err := dbtest.YDBMigrated(t).Query().QueryRow(context.Background(), q,
		query.WithParameters(ydb.ParamsBuilder().Param("$tenant_id").Text(tenantID).Build()))
	require.NoError(t, err)
	var n uint64
	require.NoError(t, row.Scan(&n))
	return int(n)
}
```


- [ ] **Step 4: Replace `suppression_test.go`** with the pg+ydb contract suite. It covers upsert and list, the filters (reason, case-insensitive email including Cyrillic) and the cursor, remove, tenant isolation, the pre-send checker, the settings, and the audit log.

```go
package adapters_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/deliverability/domain"
)

func TestSuppressionsUpsertAndList(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.suppressions(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		entry, err := domain.NewManualSuppression(tenantID, "blocked@acme.com", "by support")
		require.NoError(t, err)
		require.NoError(t, repo.Upsert(ctx, entry))
		// A second upsert of the same address is a no-op: the first entry stays.
		again, err := domain.NewSuppressionEntry(tenantID, "Blocked@acme.com", domain.ReasonComplaint, "")
		require.NoError(t, err)
		require.NoError(t, repo.Upsert(ctx, again))

		page, next, err := repo.List(ctx, tenantID, domain.SuppressionFilter{})
		require.NoError(t, err)
		require.Empty(t, next)
		require.Len(t, page, 1)
		require.NotEmpty(t, page[0].ID())
		require.Equal(t, tenantID, page[0].TenantID())
		require.Equal(t, "blocked@acme.com", page[0].Email())
		require.Equal(t, domain.ReasonManual, page[0].Reason())
		require.Equal(t, "by support", page[0].Note())
		require.Empty(t, page[0].SourceEventID())
		require.False(t, page[0].SuppressedAt().IsZero())
	})
}

func TestSuppressionsListFiltersAndPages(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.suppressions(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		for _, e := range []struct {
			email  string
			reason domain.SuppressionReason
		}{
			{"alice@acme.com", domain.ReasonHardBounce},
			{"bob@acme.com", domain.ReasonComplaint},
			{"carol@acme.com", domain.ReasonHardBounce},
			{"ёлка@пример.рф", domain.ReasonManual},
		} {
			entry, err := domain.NewSuppressionEntry(tenantID, e.email, e.reason, "")
			require.NoError(t, err)
			require.NoError(t, repo.Upsert(ctx, entry))
		}

		hard, _, err := repo.List(ctx, tenantID, domain.SuppressionFilter{Reason: domain.ReasonHardBounce})
		require.NoError(t, err)
		require.Len(t, hard, 2)

		like, _, err := repo.List(ctx, tenantID, domain.SuppressionFilter{EmailLike: "ALI"})
		require.NoError(t, err)
		require.Len(t, like, 1, "the email filter ignores case")
		require.Equal(t, "alice@acme.com", like[0].Email())

		cyrillic, _, err := repo.List(ctx, tenantID, domain.SuppressionFilter{EmailLike: "ЁЛК"})
		require.NoError(t, err)
		require.Len(t, cyrillic, 1, "the email filter ignores case for non-ASCII too")

		// Three pages of one, in id order, walked by the cursor.
		var seen []string
		cursor := ""
		for range 4 {
			page, next, err := repo.List(ctx, tenantID, domain.SuppressionFilter{Limit: 1, Cursor: cursor})
			require.NoError(t, err)
			for _, e := range page {
				seen = append(seen, e.Email())
			}
			if next == "" {
				break
			}
			cursor = next
		}
		require.Len(t, seen, 4)
		require.ElementsMatch(t,
			[]string{"alice@acme.com", "bob@acme.com", "carol@acme.com", "ёлка@пример.рф"}, seen)
	})
}

func TestSuppressionsRemove(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.suppressions(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		entry, err := domain.NewManualSuppression(tenantID, "gone@acme.com", "")
		require.NoError(t, err)
		require.NoError(t, repo.Upsert(ctx, entry))

		require.NoError(t, repo.Remove(ctx, tenantID, " Gone@acme.com "))
		require.ErrorIs(t, repo.Remove(ctx, tenantID, "gone@acme.com"), domain.ErrSuppressionNotFound)
	})
}

func TestSuppressionsCrossTenantIsolation(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.suppressions(t)
		ctx := context.Background()
		tenantA := b.newTenant(t)
		tenantB := b.newTenant(t)

		entryA, err := domain.NewManualSuppression(tenantA, "shared@acme.com", "")
		require.NoError(t, err)
		require.NoError(t, repo.Upsert(ctx, entryA))

		// The same address is mailable for tenant B — its list is empty.
		pageB, _, err := repo.List(ctx, tenantB, domain.SuppressionFilter{})
		require.NoError(t, err)
		require.Empty(t, pageB)
		// Tenant B cannot remove tenant A's entry.
		require.ErrorIs(t, repo.Remove(ctx, tenantB, "shared@acme.com"), domain.ErrSuppressionNotFound)

		pageA, _, err := repo.List(ctx, tenantA, domain.SuppressionFilter{})
		require.NoError(t, err)
		require.Len(t, pageA, 1)
	})
}

func TestSuppressionCheckerReturnsTheSuppressedSubset(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.suppressions(t)
		checker := b.checker(t)
		ctx := context.Background()
		tenantA := b.newTenant(t)
		tenantB := b.newTenant(t)

		bounced, err := domain.NewSuppressionEntry(tenantA, "bounced@acme.com", domain.ReasonHardBounce, "")
		require.NoError(t, err)
		require.NoError(t, repo.Upsert(ctx, bounced))
		manual, err := domain.NewManualSuppression(tenantA, "manual@acme.com", "")
		require.NoError(t, err)
		require.NoError(t, repo.Upsert(ctx, manual))

		got, err := checker.Suppressed(ctx, tenantA,
			[]string{"Bounced@acme.com", "manual@acme.com", "fine@acme.com"})
		require.NoError(t, err)
		require.Equal(t, map[string]string{
			"bounced@acme.com": string(domain.ReasonHardBounce),
			"manual@acme.com":  string(domain.ReasonManual),
		}, got)

		empty, err := checker.Suppressed(ctx, tenantA, nil)
		require.NoError(t, err)
		require.Empty(t, empty)

		other, err := checker.Suppressed(ctx, tenantB, []string{"bounced@acme.com"})
		require.NoError(t, err)
		require.Empty(t, other, "another tenant's list does not suppress")
	})
}

func TestSettingsGetDefaultAndPut(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.settings(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		// No row yet — the defaults apply (both toggles on).
		got, err := repo.Get(ctx, tenantID)
		require.NoError(t, err)
		require.True(t, got.SuppressOnHardBounce())
		require.True(t, got.SuppressOnComplaint())

		require.NoError(t, repo.Put(ctx, tenantID, domain.NewBounceSettings(tenantID, true, false)))
		got, err = repo.Get(ctx, tenantID)
		require.NoError(t, err)
		require.True(t, got.SuppressOnHardBounce())
		require.False(t, got.SuppressOnComplaint())
		require.False(t, got.UpdatedAt().IsZero())

		// A second put overwrites the first.
		require.NoError(t, repo.Put(ctx, tenantID, domain.NewBounceSettings(tenantID, false, true)))
		got, err = repo.Get(ctx, tenantID)
		require.NoError(t, err)
		require.False(t, got.SuppressOnHardBounce())
		require.True(t, got.SuppressOnComplaint())
	})
}

func TestSettingsCrossTenantIsolation(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.settings(t)
		ctx := context.Background()
		tenantA := b.newTenant(t)
		tenantB := b.newTenant(t)

		require.NoError(t, repo.Put(ctx, tenantA, domain.NewBounceSettings(tenantA, false, false)))

		// Tenant B is unaffected — it still sees the defaults.
		gotB, err := repo.Get(ctx, tenantB)
		require.NoError(t, err)
		require.True(t, gotB.SuppressOnHardBounce())
		require.True(t, gotB.SuppressOnComplaint())
	})
}

func TestAuditLogRecordsAnEntry(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		audit := b.audit(t)
		tenantA := b.newTenant(t)
		tenantB := b.newTenant(t)

		require.NoError(t, audit.Record(context.Background(), tenantA,
			uuid.NewString(), "suppression.add", "blocked@acme.com"))

		require.Equal(t, 1, b.auditCount(t, tenantA))
		require.Equal(t, 0, b.auditCount(t, tenantB))
	})
}
```


- [ ] **Step 5: Format and verify.**

```bash
gofmt -l internal
go build ./... && go vet ./internal/deliverability/... ./internal/db/...
golangci-lint run ./internal/deliverability/... ./internal/db/...
export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock" && go test ./internal/deliverability/... ./internal/db/... -count=1 -timeout 8m
```

Expected: `gofmt -l` prints nothing, `0 issues.`, every package `ok`. The suppression, settings, checker and audit tests run as `/pg` and `/ydb` subtests (`go test ./internal/deliverability/adapters -run 'Suppression|Settings|AuditLog' -v` shows both). `internal/db` `TestMigrateYDBAppliesEmbeddedSchema` applies the new migration.

- [ ] **Step 6: Commit.**

```bash
git add internal/db/ydbmigrations/000009_deliverability.up.sql internal/deliverability
git commit -m "feat(deliverability): port suppressions, bounce settings and the audit log to YDB"
```

---

### Task 2: Inbound events, attribution and the feedback recovery sweep

**Files:**
- Modify: `internal/deliverability/domain/repository.go`, `internal/deliverability/adapters/events_pg.go`, `internal/deliverability/app/command/feedback.go`, `internal/deliverability/app/command/feedback_test.go`
- Create: `internal/deliverability/adapters/events_ydb.go`
- Replace: `internal/deliverability/adapters/backend_test.go`, `helpers_test.go`, `feedback_worker_test.go`
- Rename and rewrite: `events_pg_test.go` -> `events_test.go`

**Interfaces:**
- Consumes: everything Task 1 produced; the B7 tables and the indexes `idx_campaign_recipients_provider` and `idx_transactional_messages_provider`; `domain.EventRepository`.
- Produces:
  ```go
  // package domain
  // EventRepository gains:
  PendingInbound(ctx context.Context, receivedBefore time.Time, limit int) ([]string, error)

  // package command
  type SweepPendingFeedback struct{ OlderThan time.Duration }
  func NewSweepPendingFeedbackHandler(events domain.EventRepository, enqueuer FeedbackEnqueuer) SweepPendingFeedbackHandler
  func (h SweepPendingFeedbackHandler) Handle(ctx context.Context, cmd SweepPendingFeedback) error

  // package adapters
  func NewYDBEvents(d *ydb.Driver) *YDBEvents
  ```
  and the `backend` harness fields `events`, `newCampaign`, `newRecipient`, `newTransactional`, `eventCount`, plus `seedSend`.

- [ ] **Step 1: Add the port method and its pg implementation.**


- [ ] **Modify `internal/deliverability/domain/repository.go`.** Apply this change (a unified diff; the `@@` line numbers are those of the file at `5c296ad`).

```diff
@@ -1,6 +1,9 @@
 package domain
 
-import "context"
+import (
+	"context"
+	"time"
+)
 
 // EventRepository persists inbound notifications and the attributed delivery
 // events. The control-plane staging methods run through the pool; the
@@ -30,6 +33,12 @@
 
 	// MarkInbound sets the staged row's terminal status and processed-at time.
 	MarkInbound(ctx context.Context, eventID string, status InboundStatus) error
+
+	// PendingInbound returns the ids of up to limit staged notifications that
+	// are still pending and were received before receivedBefore, oldest first.
+	// The recovery sweep re-enqueues them: a feedback.process job that was
+	// dead-lettered leaves its notification pending for good.
+	PendingInbound(ctx context.Context, receivedBefore time.Time, limit int) ([]string, error)
 }
 
 // SuppressionRepository persists the tenant's suppression list. Every
```


- [ ] **Modify `internal/deliverability/adapters/events_pg.go`.** Apply this change (a unified diff; the `@@` line numbers are those of the file at `5c296ad`).

```diff
@@ -196,3 +196,20 @@
 	}
 	return nil
 }
+
+// PendingInbound returns the ids of staged notifications still pending that
+// were received before receivedBefore, oldest first.
+func (r *Events) PendingInbound(ctx context.Context, receivedBefore time.Time, limit int) ([]string, error) {
+	rows, err := r.pool.Query(ctx,
+		`SELECT id FROM inbound_feedback_events
+		 WHERE status = 'pending' AND received_at < $1
+		 ORDER BY received_at LIMIT $2`, receivedBefore, limit)
+	if err != nil {
+		return nil, fmt.Errorf("listing pending notifications: %w", err)
+	}
+	ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
+	if err != nil {
+		return nil, fmt.Errorf("listing pending notifications: %w", err)
+	}
+	return ids, nil
+}
```


- [ ] **Step 2: Add the sweep use case and its test.** `ProcessFeedback` is idempotent, so enqueueing a notification whose job is merely slow is safe.


- [ ] **Modify `internal/deliverability/app/command/feedback.go`.** Apply this change (a unified diff; the `@@` line numbers are those of the file at `5c296ad`).

```diff
@@ -5,7 +5,9 @@
 
 import (
 	"context"
+	"errors"
 	"log/slog"
+	"time"
 
 	"github.com/nikolaymatrosov/nvelope/internal/deliverability/domain"
 )
@@ -162,3 +164,50 @@
 		"event_kind", string(n.Kind))
 	return h.events.MarkInbound(ctx, n.ID, domain.InboundUnattributed)
 }
+
+// feedbackSweepLimit bounds the notifications one sweep re-enqueues.
+// ponytail: the next tick takes the rest; a retention job for processed rows
+// if the staging table grows.
+const feedbackSweepLimit = 500
+
+// SweepPendingFeedback is the request to re-enqueue feedback.process for
+// staged notifications still pending after OlderThan.
+type SweepPendingFeedback struct {
+	OlderThan time.Duration
+}
+
+// SweepPendingFeedbackHandler handles SweepPendingFeedback. A feedback.process
+// job that exhausts its retries is dead-lettered, which leaves its notification
+// pending and its bounce or complaint unapplied. The sweep enqueues the job
+// again. A job that is only slow runs twice, which is safe: ProcessFeedback is
+// idempotent.
+type SweepPendingFeedbackHandler struct {
+	events   domain.EventRepository
+	enqueuer FeedbackEnqueuer
+}
+
+// NewSweepPendingFeedbackHandler builds the handler, failing fast on a nil
+// dependency.
+func NewSweepPendingFeedbackHandler(events domain.EventRepository,
+	enqueuer FeedbackEnqueuer) SweepPendingFeedbackHandler {
+	if events == nil || enqueuer == nil {
+		panic("nil dependency")
+	}
+	return SweepPendingFeedbackHandler{events: events, enqueuer: enqueuer}
+}
+
+// Handle enqueues every pending notification older than cmd.OlderThan. One
+// failed enqueue does not stop the rest; the errors are joined.
+func (h SweepPendingFeedbackHandler) Handle(ctx context.Context, cmd SweepPendingFeedback) error {
+	ids, err := h.events.PendingInbound(ctx, time.Now().Add(-cmd.OlderThan), feedbackSweepLimit)
+	if err != nil {
+		return err
+	}
+	var errs []error
+	for _, id := range ids {
+		if err := h.enqueuer.EnqueueFeedbackProcess(ctx, id); err != nil {
+			errs = append(errs, err)
+		}
+	}
+	return errors.Join(errs...)
+}
```


- [ ] **Modify `internal/deliverability/app/command/feedback_test.go`.** Apply this change (a unified diff; the `@@` line numbers are those of the file at `5c296ad`).

```diff
@@ -25,8 +25,12 @@
 // fakeEvents is an in-memory EventRepository for ingestion tests.
 type fakeEvents struct {
 	staged []domain.InboundNotification
-	byKey  map[string]string
-	nextID int
+	// pending is what PendingInbound returns; pendingBefore records the cutoff
+	// it was asked for.
+	pending       []string
+	pendingBefore time.Time
+	byKey         map[string]string
+	nextID        int
 }
 
 func newFakeEvents() *fakeEvents { return &fakeEvents{byKey: map[string]string{}} }
@@ -62,6 +66,11 @@
 	return nil
 }
 
+func (f *fakeEvents) PendingInbound(_ context.Context, before time.Time, _ int) ([]string, error) {
+	f.pendingBefore = before
+	return f.pending, nil
+}
+
 // fakeEnqueuer records enqueued feedback-process jobs.
 type fakeEnqueuer struct{ enqueued []string }
 
@@ -127,3 +136,19 @@
 	err := h.Handle(context.Background(), command.IngestNotification{RawPayload: []byte("not json")})
 	require.ErrorIs(t, err, domain.ErrValidationFailed)
 }
+
+func TestSweepPendingFeedbackEnqueuesEveryPendingNotification(t *testing.T) {
+	t.Parallel()
+	events := newFakeEvents()
+	events.pending = []string{"evt-a", "evt-b"}
+	enqueuer := &fakeEnqueuer{}
+	handler := command.NewSweepPendingFeedbackHandler(events, enqueuer)
+
+	start := time.Now()
+	require.NoError(t, handler.Handle(context.Background(),
+		command.SweepPendingFeedback{OlderThan: 10 * time.Minute}))
+
+	require.Equal(t, []string{"evt-a", "evt-b"}, enqueuer.enqueued)
+	require.WithinDuration(t, start.Add(-10*time.Minute), events.pendingBefore, time.Minute,
+		"the cutoff is now minus OlderThan")
+}
```


- [ ] **Step 3: Create `events_ydb.go`.**

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

	"github.com/nikolaymatrosov/nvelope/internal/deliverability/domain"
)

// YDBEvents is the YDB-backed implementation of domain.EventRepository, for
// the serverless runtime. The staging methods and TenantForMessage read no
// tenant; Attribute and RecordEvent filter by the tenant explicitly. The
// provider-message-id lookups read the indexes Plan B7 created on the campaign
// tables.
type YDBEvents struct {
	driver *ydb.Driver
}

var _ domain.EventRepository = (*YDBEvents)(nil)

// NewYDBEvents builds a YDBEvents repository over the driver.
func NewYDBEvents(d *ydb.Driver) *YDBEvents {
	return &YDBEvents{driver: d}
}

// StageInbound inserts a staged notification, deduplicating on dedupe_key. The
// transaction reads the key first, because a conflicting INSERT would abort
// it. Two concurrent stagings of one key conflict, and the SDK retries the
// loser, which then finds the winner's row.
func (r *YDBEvents) StageInbound(ctx context.Context, n domain.InboundNotification) (string, bool, error) {
	fresh := uuid.NewString()
	var id string
	var staged bool
	err := r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		row, err := tx.QueryRow(ctx, `DECLARE $dedupe_key AS Utf8;
SELECT id FROM inbound_feedback_events VIEW idx_inbound_dedupe WHERE dedupe_key = $dedupe_key;`,
			query.WithParameters(ydb.ParamsBuilder().Param("$dedupe_key").Text(n.DedupeKey).Build()))
		if err == nil {
			var existing string
			if err := row.Scan(&existing); err != nil {
				return fmt.Errorf("scanning staged notification: %w", err)
			}
			id, staged = existing, false
			return nil
		}
		if !errors.Is(err, query.ErrNoRows) {
			return fmt.Errorf("loading staged notification: %w", err)
		}
		if err := tx.Exec(ctx, `DECLARE $id AS Utf8;
DECLARE $dedupe_key AS Utf8;
DECLARE $event_kind AS Utf8;
DECLARE $provider_message_id AS Utf8;
DECLARE $recipient_email AS Utf8;
DECLARE $occurred_at AS Timestamp;
DECLARE $raw_payload AS Utf8;
INSERT INTO inbound_feedback_events
  (id, dedupe_key, event_kind, provider_message_id, recipient_email, occurred_at,
   raw_payload, status, received_at, processed_at)
VALUES ($id, $dedupe_key, $event_kind, $provider_message_id, $recipient_email, $occurred_at,
        $raw_payload, "pending"u, CurrentUtcTimestamp(), NULL);`,
			query.WithParameters(ydb.ParamsBuilder().
				Param("$id").Text(fresh).
				Param("$dedupe_key").Text(n.DedupeKey).
				Param("$event_kind").Text(string(n.Kind)).
				Param("$provider_message_id").Text(n.ProviderMessageID).
				Param("$recipient_email").Text(n.RecipientEmail).
				Param("$occurred_at").Timestamp(n.OccurredAt).
				Param("$raw_payload").Text(string(n.RawPayload)).
				Build())); err != nil {
			return fmt.Errorf("staging inbound notification: %w", err)
		}
		id, staged = fresh, true
		return nil
	})
	if err != nil {
		return "", false, err
	}
	return id, staged, nil
}

// LoadInbound fetches a staged notification by id.
func (r *YDBEvents) LoadInbound(ctx context.Context, eventID string) (domain.InboundNotification, error) {
	row, err := r.driver.Query().QueryRow(ctx, `DECLARE $id AS Utf8;
SELECT id, dedupe_key, event_kind, provider_message_id, recipient_email, occurred_at,
       raw_payload, status
FROM inbound_feedback_events WHERE id = $id;`,
		query.WithParameters(ydb.ParamsBuilder().Param("$id").Text(eventID).Build()))
	if errors.Is(err, query.ErrNoRows) {
		return domain.InboundNotification{}, fmt.Errorf("staged notification %s not found", eventID)
	}
	if err != nil {
		return domain.InboundNotification{}, fmt.Errorf("loading staged notification: %w", err)
	}
	var n domain.InboundNotification
	var kind, status, payload string
	if err := row.Scan(&n.ID, &n.DedupeKey, &kind, &n.ProviderMessageID,
		&n.RecipientEmail, &n.OccurredAt, &payload, &status); err != nil {
		return domain.InboundNotification{}, fmt.Errorf("scanning staged notification: %w", err)
	}
	n.Kind = domain.EventKind(kind)
	n.Status = domain.InboundStatus(status)
	n.RawPayload = []byte(payload)
	return n, nil
}

// TenantForMessage resolves the owning tenant of a provider message id through
// the two provider-message-id indexes. It stands in for the Postgres SECURITY
// DEFINER function and leaks the same thing: the tenant of a caller-supplied
// id.
func (r *YDBEvents) TenantForMessage(ctx context.Context, providerMessageID string) (string, bool, error) {
	for _, q := range []string{
		`DECLARE $pm AS Utf8;
SELECT tenant_id FROM campaign_recipients VIEW idx_campaign_recipients_provider
WHERE provider_message_id = $pm LIMIT 1;`,
		`DECLARE $pm AS Utf8;
SELECT tenant_id FROM transactional_messages VIEW idx_transactional_messages_provider
WHERE provider_message_id = $pm LIMIT 1;`,
	} {
		row, err := r.driver.Query().QueryRow(ctx, q,
			query.WithParameters(ydb.ParamsBuilder().Param("$pm").Text(providerMessageID).Build()))
		if errors.Is(err, query.ErrNoRows) {
			continue
		}
		if err != nil {
			return "", false, fmt.Errorf("resolving tenant for message: %w", err)
		}
		var tenantID string
		if err := row.Scan(&tenantID); err != nil {
			return "", false, fmt.Errorf("scanning tenant for message: %w", err)
		}
		return tenantID, true, nil
	}
	return "", false, nil
}

// Attribute matches a provider message id to a campaign recipient or a
// transactional message of the tenant.
func (r *YDBEvents) Attribute(ctx context.Context, tenantID, providerMessageID string) (
	domain.Attribution, bool, error) {

	params := query.WithParameters(ydb.ParamsBuilder().
		Param("$tenant_id").Text(tenantID).
		Param("$pm").Text(providerMessageID).
		Build())
	row, err := r.driver.Query().QueryRow(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $pm AS Utf8;
SELECT id, campaign_id FROM campaign_recipients VIEW idx_campaign_recipients_provider
WHERE provider_message_id = $pm AND tenant_id = $tenant_id LIMIT 1;`, params)
	if err == nil {
		var recipientID, campaignID string
		if err := row.Scan(&recipientID, &campaignID); err != nil {
			return domain.Attribution{}, false, fmt.Errorf("scanning campaign recipient: %w", err)
		}
		return domain.Attribution{CampaignID: campaignID, CampaignRecipientID: recipientID}, true, nil
	}
	if !errors.Is(err, query.ErrNoRows) {
		return domain.Attribution{}, false, fmt.Errorf("matching campaign recipient: %w", err)
	}
	row, err = r.driver.Query().QueryRow(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $pm AS Utf8;
SELECT id FROM transactional_messages VIEW idx_transactional_messages_provider
WHERE provider_message_id = $pm AND tenant_id = $tenant_id LIMIT 1;`, params)
	if errors.Is(err, query.ErrNoRows) {
		return domain.Attribution{}, false, nil
	}
	if err != nil {
		return domain.Attribution{}, false, fmt.Errorf("matching transactional message: %w", err)
	}
	var txMessageID string
	if err := row.Scan(&txMessageID); err != nil {
		return domain.Attribution{}, false, fmt.Errorf("scanning transactional message: %w", err)
	}
	return domain.Attribution{TransactionalMessageID: txMessageID}, true, nil
}

// RecordEvent inserts the attributed delivery event, idempotent on the inbound
// event id: the transaction reads the id through idx_delivery_events_inbound
// and inserts only when it is absent. recorded reports whether a row was
// written.
func (r *YDBEvents) RecordEvent(ctx context.Context, e *domain.DeliveryEvent) (bool, error) {
	id := uuid.NewString()
	var recorded bool
	err := r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		_, err := tx.QueryRow(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $inbound_event_id AS Utf8;
SELECT id FROM delivery_events VIEW idx_delivery_events_inbound
WHERE tenant_id = $tenant_id AND inbound_event_id = $inbound_event_id;`,
			query.WithParameters(ydb.ParamsBuilder().
				Param("$tenant_id").Text(e.TenantID()).
				Param("$inbound_event_id").Text(e.InboundEventID()).
				Build()))
		if err == nil {
			recorded = false
			return nil
		}
		if !errors.Is(err, query.ErrNoRows) {
			return fmt.Errorf("reading delivery event: %w", err)
		}
		if err := tx.Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DECLARE $inbound_event_id AS Utf8;
DECLARE $event_kind AS Utf8;
DECLARE $recipient_email AS Utf8;
DECLARE $campaign_id AS Optional<Utf8>;
DECLARE $campaign_recipient_id AS Optional<Utf8>;
DECLARE $transactional_message_id AS Optional<Utf8>;
DECLARE $provider_message_id AS Utf8;
DECLARE $occurred_at AS Timestamp;
INSERT INTO delivery_events
  (tenant_id, id, inbound_event_id, event_kind, recipient_email, campaign_id,
   campaign_recipient_id, transactional_message_id, provider_message_id, occurred_at, created_at)
VALUES ($tenant_id, $id, $inbound_event_id, $event_kind, $recipient_email, $campaign_id,
        $campaign_recipient_id, $transactional_message_id, $provider_message_id, $occurred_at,
        CurrentUtcTimestamp());`,
			query.WithParameters(ydb.ParamsBuilder().
				Param("$tenant_id").Text(e.TenantID()).
				Param("$id").Text(id).
				Param("$inbound_event_id").Text(e.InboundEventID()).
				Param("$event_kind").Text(string(e.Kind())).
				Param("$recipient_email").Text(e.RecipientEmail()).
				Param("$campaign_id").BeginOptional().Text(nullString(e.CampaignID())).EndOptional().
				Param("$campaign_recipient_id").BeginOptional().Text(nullString(e.CampaignRecipientID())).EndOptional().
				Param("$transactional_message_id").BeginOptional().Text(nullString(e.TransactionalMessageID())).EndOptional().
				Param("$provider_message_id").Text(e.ProviderMessageID()).
				Param("$occurred_at").Timestamp(e.OccurredAt()).
				Build())); err != nil {
			return fmt.Errorf("recording delivery event: %w", err)
		}
		recorded = true
		return nil
	})
	if err != nil {
		return false, err
	}
	return recorded, nil
}

// MarkInbound sets the staged row's terminal status and processed-at time.
func (r *YDBEvents) MarkInbound(ctx context.Context, eventID string, status domain.InboundStatus) error {
	if err := r.driver.Query().Exec(ctx, `DECLARE $id AS Utf8;
DECLARE $status AS Utf8;
UPDATE inbound_feedback_events
SET status = $status, processed_at = CurrentUtcTimestamp()
WHERE id = $id;`,
		query.WithParameters(ydb.ParamsBuilder().
			Param("$id").Text(eventID).
			Param("$status").Text(string(status)).
			Build())); err != nil {
		return fmt.Errorf("marking inbound notification: %w", err)
	}
	return nil
}

// PendingInbound returns the ids of staged notifications still pending that
// were received before receivedBefore, oldest first, through idx_inbound_status.
func (r *YDBEvents) PendingInbound(ctx context.Context, receivedBefore time.Time, limit int) ([]string, error) {
	rs, err := r.driver.Query().QueryResultSet(ctx, `DECLARE $before AS Timestamp;
DECLARE $limit AS Uint64;
SELECT id FROM inbound_feedback_events VIEW idx_inbound_status
WHERE status = "pending"u AND received_at < $before
ORDER BY received_at LIMIT $limit;`,
		query.WithParameters(ydb.ParamsBuilder().
			Param("$before").Timestamp(receivedBefore).
			Param("$limit").Uint64(uint64(limit)).
			Build()))
	if err != nil {
		return nil, fmt.Errorf("listing pending notifications: %w", err)
	}
	var ids []string
	for row, err := range rs.Rows(ctx) {
		if err != nil {
			return nil, fmt.Errorf("listing pending notifications: %w", err)
		}
		var id string
		if err := row.Scan(&id); err != nil {
			return nil, fmt.Errorf("scanning pending notification: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, nil
}
```


- [ ] **Step 4: Replace `backend_test.go`** with the version that adds events and the seeds. It also adds `seedSend` and the `ydbExec` helper.

```go
package adapters_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	ydb "github.com/ydb-platform/ydb-go-sdk/v3"
	"github.com/ydb-platform/ydb-go-sdk/v3/query"

	campaigndomain "github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
	"github.com/nikolaymatrosov/nvelope/internal/deliverability/adapters"
	"github.com/nikolaymatrosov/nvelope/internal/deliverability/app/command"
	"github.com/nikolaymatrosov/nvelope/internal/deliverability/domain"
	"github.com/nikolaymatrosov/nvelope/internal/platform/tenantdb"
)

// backend is one storage implementation of the deliverability repositories
// under contract test. Every contract test body runs once per backend, so the
// Postgres and YDB adapters are held to the same behaviour.
type backend struct {
	name         string
	events       func(t *testing.T) domain.EventRepository
	suppressions func(t *testing.T) domain.SuppressionRepository
	settings     func(t *testing.T) domain.SettingsRepository
	checker      func(t *testing.T) campaigndomain.SuppressionChecker
	audit        func(t *testing.T) command.AuditWriter
	// The seeds below create rows owned by other contexts. On Postgres they
	// are real rows, because the delivery tables carry foreign keys to them.
	// YDB has no foreign keys, so a bare UUID stands in for a tenant.
	newTenant        func(t *testing.T) string
	newCampaign      func(t *testing.T, tenantID, status string) string
	newRecipient     func(t *testing.T, tenantID, campaignID, status, providerMsgID string) string
	newTransactional func(t *testing.T, tenantID, providerMsgID string) string
	// The probes read what no port exposes.
	eventCount func(t *testing.T, tenantID string) int
	auditCount func(t *testing.T, tenantID string) int
}

var backends = []backend{
	{
		name: "pg",
		events: func(t *testing.T) domain.EventRepository {
			return adapters.NewEvents(dbtest.AppPool(t))
		},
		suppressions: func(t *testing.T) domain.SuppressionRepository {
			return adapters.NewSuppressions(dbtest.AppPool(t))
		},
		settings: func(t *testing.T) domain.SettingsRepository {
			return adapters.NewSettings(dbtest.AppPool(t))
		},
		checker: func(t *testing.T) campaigndomain.SuppressionChecker {
			return adapters.NewSuppressionChecker(dbtest.AppPool(t))
		},
		audit: func(t *testing.T) command.AuditWriter {
			return adapters.NewAuditLog(dbtest.AppPool(t))
		},
		newTenant: func(t *testing.T) string { return seedTenant(t, dbtest.AppPool(t)) },
		newCampaign: func(t *testing.T, tenantID, status string) string {
			var id string
			require.NoError(t, tenantdb.WithTenant(context.Background(), dbtest.AppPool(t), tenantID,
				func(ctx context.Context, tx pgx.Tx) error {
					return tx.QueryRow(ctx,
						`INSERT INTO campaigns
						   (tenant_id, name, subject, body_html, from_name, from_local_part, status)
						 VALUES ($1, $2, 'Subject', '<p>hi</p>', 'Acme', 'news', $3) RETURNING id`,
						tenantID, "C-"+dbtest.RandString(), status).Scan(&id)
				}))
			return id
		},
		newRecipient: func(t *testing.T, tenantID, campaignID, status, providerMsgID string) string {
			var id string
			require.NoError(t, tenantdb.WithTenant(context.Background(), dbtest.AppPool(t), tenantID,
				func(ctx context.Context, tx pgx.Tx) error {
					var subscriberID string
					if err := tx.QueryRow(ctx,
						`INSERT INTO subscribers (tenant_id, email, name) VALUES ($1, $2, 'Sub')
						 RETURNING id`, tenantID, dbtest.RandString()+"@acme.com").Scan(&subscriberID); err != nil {
						return err
					}
					return tx.QueryRow(ctx,
						`INSERT INTO campaign_recipients
						   (tenant_id, campaign_id, subscriber_id, email, status, sent_at, provider_message_id)
						 VALUES (@tenant_id, @campaign_id, @subscriber_id, @email, @status, now(), @provider_message_id)
						 RETURNING id`,
						pgx.NamedArgs{
							"tenant_id":           tenantID,
							"campaign_id":         campaignID,
							"subscriber_id":       subscriberID,
							"email":               dbtest.RandString() + "@acme.com",
							"status":              status,
							"provider_message_id": providerMsgID,
						}).Scan(&id)
				}))
			return id
		},
		newTransactional: func(t *testing.T, tenantID, providerMsgID string) string {
			var id string
			require.NoError(t, tenantdb.WithTenant(context.Background(), dbtest.AppPool(t), tenantID,
				func(ctx context.Context, tx pgx.Tx) error {
					return tx.QueryRow(ctx,
						`INSERT INTO transactional_messages (tenant_id, provider_message_id, recipient_email)
						 VALUES ($1, $2, $3) RETURNING id`,
						tenantID, providerMsgID, "tx@acme.com").Scan(&id)
				}))
			return id
		},
		eventCount: func(t *testing.T, tenantID string) int {
			return pgCount(t, dbtest.AppPool(t), tenantID, "SELECT count(*) FROM delivery_events")
		},
		auditCount: func(t *testing.T, tenantID string) int {
			return pgCount(t, dbtest.AppPool(t), tenantID, "SELECT count(*) FROM audit_log")
		},
	},
	{
		name: "ydb",
		events: func(t *testing.T) domain.EventRepository {
			return adapters.NewYDBEvents(dbtest.YDBMigrated(t))
		},
		suppressions: func(t *testing.T) domain.SuppressionRepository {
			return adapters.NewYDBSuppressions(dbtest.YDBMigrated(t))
		},
		settings: func(t *testing.T) domain.SettingsRepository {
			return adapters.NewYDBSettings(dbtest.YDBMigrated(t))
		},
		checker: func(t *testing.T) campaigndomain.SuppressionChecker {
			return adapters.NewYDBSuppressionChecker(dbtest.YDBMigrated(t))
		},
		audit: func(t *testing.T) command.AuditWriter {
			return adapters.NewYDBAuditLog(dbtest.YDBMigrated(t))
		},
		newTenant: func(*testing.T) string { return uuid.NewString() },
		newCampaign: func(t *testing.T, tenantID, status string) string {
			id := uuid.NewString()
			ydbExec(t, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DECLARE $name AS Utf8;
DECLARE $status AS Utf8;
INSERT INTO campaigns
  (tenant_id, id, name, subject, body_html, body_text, from_name, from_local_part, status,
   max_send_errors, sent_count, failed_count, recipient_count, created_at, updated_at, archive_visible)
VALUES ($tenant_id, $id, $name, "Subject"u, "<p>hi</p>"u, ""u, "Acme"u, "news"u, $status,
        0, 0, 0, 0, CurrentUtcTimestamp(), CurrentUtcTimestamp(), false);`,
				query.WithParameters(ydb.ParamsBuilder().
					Param("$tenant_id").Text(tenantID).
					Param("$id").Text(id).
					Param("$name").Text("C-"+dbtest.RandString()).
					Param("$status").Text(status).Build()))
			return id
		},
		newRecipient: func(t *testing.T, tenantID, campaignID, status, providerMsgID string) string {
			id := uuid.NewString()
			ydbExec(t, `DECLARE $tenant_id AS Utf8;
DECLARE $campaign_id AS Utf8;
DECLARE $id AS Utf8;
DECLARE $email AS Utf8;
DECLARE $status AS Utf8;
DECLARE $provider_message_id AS Utf8;
INSERT INTO campaign_recipients
  (tenant_id, campaign_id, id, subscriber_id, email, status, failure_reason, sent_at, provider_message_id)
VALUES ($tenant_id, $campaign_id, $id, $id, $email, $status, ""u, CurrentUtcTimestamp(), $provider_message_id);`,
				query.WithParameters(ydb.ParamsBuilder().
					Param("$tenant_id").Text(tenantID).
					Param("$campaign_id").Text(campaignID).
					Param("$id").Text(id).
					Param("$email").Text(dbtest.RandString()+"@acme.com").
					Param("$status").Text(status).
					Param("$provider_message_id").Text(providerMsgID).Build()))
			return id
		},
		newTransactional: func(t *testing.T, tenantID, providerMsgID string) string {
			id := uuid.NewString()
			ydbExec(t, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DECLARE $provider_message_id AS Utf8;
INSERT INTO transactional_messages
  (tenant_id, id, template_id, provider_message_id, recipient_email, sent_at)
VALUES ($tenant_id, $id, NULL, $provider_message_id, "tx@acme.com"u, CurrentUtcTimestamp());`,
				query.WithParameters(ydb.ParamsBuilder().
					Param("$tenant_id").Text(tenantID).
					Param("$id").Text(id).
					Param("$provider_message_id").Text(providerMsgID).Build()))
			return id
		},
		eventCount: func(t *testing.T, tenantID string) int {
			return ydbCount(t, `DECLARE $tenant_id AS Utf8;
SELECT COUNT(*) FROM delivery_events WHERE tenant_id = $tenant_id;`, tenantID)
		},
		auditCount: func(t *testing.T, tenantID string) int {
			return ydbCount(t, `DECLARE $tenant_id AS Utf8;
SELECT COUNT(*) FROM audit_log WHERE tenant_id = $tenant_id;`, tenantID)
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

// seedSend creates a finished campaign with one sent recipient carrying
// providerMsgID, returning the campaign and recipient ids.
func seedSend(t *testing.T, b backend, tenantID, providerMsgID string) (campaignID, recipientID string) {
	t.Helper()
	campaignID = b.newCampaign(t, tenantID, "finished")
	return campaignID, b.newRecipient(t, tenantID, campaignID, "sent", providerMsgID)
}

// pgCount reads one count inside the tenant-bound transaction.
func pgCount(t *testing.T, pool *pgxpool.Pool, tenantID, sql string) int {
	t.Helper()
	var n int
	require.NoError(t, tenantdb.WithTenant(context.Background(), pool, tenantID,
		func(ctx context.Context, tx pgx.Tx) error {
			return tx.QueryRow(ctx, sql).Scan(&n)
		}))
	return n
}

// ydbExec runs one write statement with the given parameters.
func ydbExec(t *testing.T, q string, params query.ExecuteOption) {
	t.Helper()
	require.NoError(t, dbtest.YDBMigrated(t).Query().Exec(context.Background(), q, params))
}

// ydbCount runs a single-row COUNT(*) query that takes $tenant_id.
func ydbCount(t *testing.T, q, tenantID string) int {
	t.Helper()
	row, err := dbtest.YDBMigrated(t).Query().QueryRow(context.Background(), q,
		query.WithParameters(ydb.ParamsBuilder().Param("$tenant_id").Text(tenantID).Build()))
	require.NoError(t, err)
	var n uint64
	require.NoError(t, row.Scan(&n))
	return int(n)
}
```


- [ ] **Step 5: Replace `helpers_test.go`.** `seedTransactionalMessage` and `withTenantQuery` go (the harness replaces them), and `stageInbound` arrives. `seedCampaignRecipient` stays for one more task: the Postgres analytics test still uses it.

```go
package adapters_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
	"github.com/nikolaymatrosov/nvelope/internal/deliverability/domain"
	"github.com/nikolaymatrosov/nvelope/internal/platform/tenantdb"
)

// seedTenant inserts a tenant row directly and returns its id.
func seedTenant(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var id string
	require.NoError(t, pool.QueryRow(context.Background(),
		`INSERT INTO tenants (name, slug, status) VALUES ('Workspace', $1, 'active') RETURNING id`,
		"dlv-"+dbtest.RandString()).Scan(&id))
	return id
}

// seedCampaignRecipient inserts a campaign and one recipient carrying
// providerMsgID, returning the campaign and recipient ids.
func seedCampaignRecipient(t *testing.T, pool *pgxpool.Pool, tenantID, providerMsgID string) (
	campaignID, recipientID string) {

	t.Helper()
	require.NoError(t, tenantdb.WithTenant(context.Background(), pool, tenantID,
		func(ctx context.Context, tx pgx.Tx) error {
			var subscriberID string
			if err := tx.QueryRow(ctx,
				`INSERT INTO subscribers (tenant_id, email, name) VALUES ($1, $2, 'Sub')
				 RETURNING id`, tenantID, dbtest.RandString()+"@acme.com").Scan(&subscriberID); err != nil {
				return err
			}
			if err := tx.QueryRow(ctx,
				`INSERT INTO campaigns
				   (tenant_id, name, subject, body_html, from_name, from_local_part, status)
				 VALUES ($1, $2, 'Subject', '<p>hi</p>', 'Acme', 'news', 'finished') RETURNING id`,
				tenantID, "C-"+dbtest.RandString()).Scan(&campaignID); err != nil {
				return err
			}
			return tx.QueryRow(ctx,
				`INSERT INTO campaign_recipients
				   (tenant_id, campaign_id, subscriber_id, email, status, sent_at, provider_message_id)
				 VALUES (@tenant_id, @campaign_id, @subscriber_id, @email, 'sent', now(), @provider_message_id)
				 RETURNING id`,
				pgx.NamedArgs{
					"tenant_id":           tenantID,
					"campaign_id":         campaignID,
					"subscriber_id":       subscriberID,
					"email":               "rcpt@acme.com",
					"provider_message_id": providerMsgID,
				}).Scan(&recipientID)
		}))
	return campaignID, recipientID
}

// newInbound builds an inbound notification for tests.
func newInbound(t *testing.T, kind domain.EventKind, providerMsgID string) domain.InboundNotification {
	t.Helper()
	n, err := domain.NewInboundNotification("dk-"+dbtest.RandString(), kind,
		providerMsgID, "rcpt@acme.com", time.Now(), []byte(`{"k":"v"}`))
	require.NoError(t, err)
	return n
}

// stageInbound stages n and, so that the shared test database does not pile up
// pending notifications for PendingInbound to wade through, marks it failed
// when the test ends if it is still pending.
func stageInbound(t *testing.T, repo domain.EventRepository, n domain.InboundNotification) string {
	t.Helper()
	ctx := context.Background()
	id, _, err := repo.StageInbound(ctx, n)
	require.NoError(t, err)
	t.Cleanup(func() {
		if got, err := repo.LoadInbound(ctx, id); err == nil && got.Status == domain.InboundPending {
			_ = repo.MarkInbound(ctx, id, domain.InboundFailed)
		}
	})
	return id
}
```


- [ ] **Step 6: Rename the events test and replace the two test files.** `events_test.go` covers staging (idempotent and race-free), load, tenant resolution, attribution, `RecordEvent` (idempotent, race-free, isolated), `MarkInbound` and `PendingInbound`. `feedback_worker_test.go` runs the worker paths on both backends.

```bash
git mv internal/deliverability/adapters/events_pg_test.go internal/deliverability/adapters/events_test.go
```

`events_test.go`:

```go
package adapters_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
	"github.com/nikolaymatrosov/nvelope/internal/deliverability/domain"
)

func TestEventsStageInboundIsIdempotent(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.events(t)
		ctx := context.Background()

		n := newInbound(t, domain.KindBounce, "pm-"+dbtest.RandString())

		id1 := stageInbound(t, repo, n)
		require.NotEmpty(t, id1)

		id2, staged2, err := repo.StageInbound(ctx, n)
		require.NoError(t, err)
		require.False(t, staged2, "a duplicate dedupe key inserts nothing")
		require.Equal(t, id1, id2)
	})
}

func TestEventsStageInboundIsRaceFree(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.events(t)
		n := newInbound(t, domain.KindBounce, "pm-"+dbtest.RandString())

		var staged atomic.Int32
		ids := make([]string, 6)
		var wg sync.WaitGroup
		for i := range ids {
			wg.Go(func() {
				id, ok, err := repo.StageInbound(context.Background(), n)
				if err == nil && ok {
					staged.Add(1)
				}
				if err == nil {
					ids[i] = id
				}
			})
		}
		wg.Wait()

		require.EqualValues(t, 1, staged.Load(), "exactly one of the concurrent stagings writes the row")
		for _, id := range ids {
			require.Equal(t, ids[0], id)
			require.NotEmpty(t, id)
		}
		// Retire the row, as stageInbound does for the other tests.
		require.NoError(t, repo.MarkInbound(context.Background(), ids[0], domain.InboundFailed))
	})
}

func TestEventsLoadInbound(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.events(t)
		ctx := context.Background()

		n := newInbound(t, domain.KindComplaint, "pm-"+dbtest.RandString())
		id := stageInbound(t, repo, n)

		loaded, err := repo.LoadInbound(ctx, id)
		require.NoError(t, err)
		require.Equal(t, id, loaded.ID)
		require.Equal(t, n.DedupeKey, loaded.DedupeKey)
		require.Equal(t, domain.KindComplaint, loaded.Kind)
		require.Equal(t, domain.InboundPending, loaded.Status)
		require.Equal(t, n.ProviderMessageID, loaded.ProviderMessageID)
		require.Equal(t, n.RecipientEmail, loaded.RecipientEmail)
		require.JSONEq(t, string(n.RawPayload), string(loaded.RawPayload))
		require.WithinDuration(t, n.OccurredAt, loaded.OccurredAt, time.Millisecond)

		_, err = repo.LoadInbound(ctx, "00000000-0000-0000-0000-000000000000")
		require.Error(t, err, "an unknown id is an error")
	})
}

func TestEventsTenantForMessageResolvesCampaignSend(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.events(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		pm := "pm-" + dbtest.RandString()
		seedSend(t, b, tenantID, pm)

		resolved, ok, err := repo.TenantForMessage(ctx, pm)
		require.NoError(t, err)
		require.True(t, ok)
		require.Equal(t, tenantID, resolved)

		txPM := "pm-" + dbtest.RandString()
		b.newTransactional(t, tenantID, txPM)
		resolved, ok, err = repo.TenantForMessage(ctx, txPM)
		require.NoError(t, err)
		require.True(t, ok, "a transactional send resolves too")
		require.Equal(t, tenantID, resolved)

		_, ok, err = repo.TenantForMessage(ctx, "pm-no-such-message")
		require.NoError(t, err)
		require.False(t, ok)
	})
}

func TestEventsAttributeMatchesCampaignAndTransactional(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.events(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		campaignPM := "pm-" + dbtest.RandString()
		campaignID, recipientID := seedSend(t, b, tenantID, campaignPM)
		attr, ok, err := repo.Attribute(ctx, tenantID, campaignPM)
		require.NoError(t, err)
		require.True(t, ok)
		require.Equal(t, campaignID, attr.CampaignID)
		require.Equal(t, recipientID, attr.CampaignRecipientID)

		txPM := "pm-" + dbtest.RandString()
		txID := b.newTransactional(t, tenantID, txPM)
		attr, ok, err = repo.Attribute(ctx, tenantID, txPM)
		require.NoError(t, err)
		require.True(t, ok)
		require.Equal(t, txID, attr.TransactionalMessageID)
		require.Empty(t, attr.CampaignRecipientID)

		_, ok, err = repo.Attribute(ctx, tenantID, "pm-unmatched")
		require.NoError(t, err)
		require.False(t, ok)

		// Another tenant cannot attribute this tenant's send.
		_, ok, err = repo.Attribute(ctx, b.newTenant(t), campaignPM)
		require.NoError(t, err)
		require.False(t, ok)
	})
}

func TestEventsRecordEventIsIdempotent(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.events(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		pm := "pm-" + dbtest.RandString()
		campaignID, recipientID := seedSend(t, b, tenantID, pm)

		inboundID := stageInbound(t, repo, newInbound(t, domain.KindBounce, pm))

		event, err := domain.NewDeliveryEvent(tenantID, inboundID, domain.KindBounce,
			"rcpt@acme.com", pm, time.Now(),
			domain.Attribution{CampaignID: campaignID, CampaignRecipientID: recipientID})
		require.NoError(t, err)

		recorded, err := repo.RecordEvent(ctx, event)
		require.NoError(t, err)
		require.True(t, recorded)

		recorded, err = repo.RecordEvent(ctx, event)
		require.NoError(t, err)
		require.False(t, recorded, "a second record for the same inbound event is a no-op")
		require.Equal(t, 1, b.eventCount(t, tenantID))
	})
}

func TestEventsRecordEventIsRaceFree(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.events(t)
		tenantID := b.newTenant(t)
		pm := "pm-" + dbtest.RandString()
		seedSend(t, b, tenantID, pm)
		inboundID := stageInbound(t, repo, newInbound(t, domain.KindOpen, pm))
		attr, ok, err := repo.Attribute(context.Background(), tenantID, pm)
		require.NoError(t, err)
		require.True(t, ok)
		event, err := domain.NewDeliveryEvent(tenantID, inboundID, domain.KindOpen,
			"rcpt@acme.com", pm, time.Now(), attr)
		require.NoError(t, err)

		var recorded atomic.Int32
		var failed atomic.Int32
		var wg sync.WaitGroup
		for range 6 {
			wg.Go(func() {
				ok, err := repo.RecordEvent(context.Background(), event)
				switch {
				case err != nil:
					failed.Add(1)
				case ok:
					recorded.Add(1)
				}
			})
		}
		wg.Wait()

		require.Zero(t, failed.Load())
		require.EqualValues(t, 1, recorded.Load())
		require.Equal(t, 1, b.eventCount(t, tenantID))
	})
}

func TestEventsDeliveryEventsCrossTenantIsolation(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.events(t)
		ctx := context.Background()
		tenantA := b.newTenant(t)
		tenantB := b.newTenant(t)
		pm := "pm-" + dbtest.RandString()
		campaignID, recipientID := seedSend(t, b, tenantA, pm)

		inboundID := stageInbound(t, repo, newInbound(t, domain.KindBounce, pm))

		event, err := domain.NewDeliveryEvent(tenantA, inboundID, domain.KindBounce,
			"rcpt@acme.com", pm, time.Now(),
			domain.Attribution{CampaignID: campaignID, CampaignRecipientID: recipientID})
		require.NoError(t, err)
		recorded, err := repo.RecordEvent(ctx, event)
		require.NoError(t, err)
		require.True(t, recorded)

		require.Equal(t, 1, b.eventCount(t, tenantA))
		require.Equal(t, 0, b.eventCount(t, tenantB))
	})
}

func TestEventsMarkInbound(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.events(t)
		ctx := context.Background()

		id := stageInbound(t, repo, newInbound(t, domain.KindBounce, "pm-"+dbtest.RandString()))

		require.NoError(t, repo.MarkInbound(ctx, id, domain.InboundUnattributed))
		loaded, err := repo.LoadInbound(ctx, id)
		require.NoError(t, err)
		require.Equal(t, domain.InboundUnattributed, loaded.Status)
		require.True(t, loaded.IsProcessed())
	})
}

func TestEventsPendingInboundListsOnlyStalePendingNotifications(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.events(t)
		ctx := context.Background()

		pending := stageInbound(t, repo, newInbound(t, domain.KindBounce, "pm-"+dbtest.RandString()))
		done := stageInbound(t, repo, newInbound(t, domain.KindBounce, "pm-"+dbtest.RandString()))
		require.NoError(t, repo.MarkInbound(ctx, done, domain.InboundAttributed))

		ids, err := repo.PendingInbound(ctx, time.Now().Add(time.Minute), 5000)
		require.NoError(t, err)
		require.Contains(t, ids, pending)
		require.NotContains(t, ids, done, "a processed notification is not pending")

		ids, err = repo.PendingInbound(ctx, time.Now().Add(-time.Hour), 5000)
		require.NoError(t, err)
		require.NotContains(t, ids, pending, "a notification newer than the cutoff is not stale")

		ids, err = repo.PendingInbound(ctx, time.Now().Add(time.Minute), 1)
		require.NoError(t, err)
		require.Len(t, ids, 1, "the limit bounds the page")
	})
}
```

`feedback_worker_test.go`:

```go
package adapters_test

import (
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
	"github.com/nikolaymatrosov/nvelope/internal/deliverability/adapters"
	"github.com/nikolaymatrosov/nvelope/internal/deliverability/app/command"
	"github.com/nikolaymatrosov/nvelope/internal/deliverability/domain"
	"github.com/nikolaymatrosov/nvelope/internal/platform/jobs"
)

func feedbackJob(inboundEventID string) jobs.FeedbackProcessArgs {
	return jobs.FeedbackProcessArgs{InboundEventID: inboundEventID}
}

// newFeedbackWorker builds a feedback worker over a ProcessFeedback handler
// with no suppressor — the ingestion-only slice.
func newFeedbackWorker(events domain.EventRepository) *adapters.FeedbackWorker {
	handler := command.NewProcessFeedbackHandler(events, nil, slog.Default())
	return adapters.NewFeedbackWorker(handler)
}

// inboundStatus reads the staged notification's terminal status.
func inboundStatus(t *testing.T, repo domain.EventRepository, id string) domain.InboundStatus {
	t.Helper()
	n, err := repo.LoadInbound(context.Background(), id)
	require.NoError(t, err)
	return n.Status
}

func TestFeedbackWorkerAttributedCampaignPath(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		events := b.events(t)
		worker := newFeedbackWorker(events)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		pm := "pm-" + dbtest.RandString()
		seedSend(t, b, tenantID, pm)

		id := stageInbound(t, events, newInbound(t, domain.KindBounce, pm))

		require.NoError(t, worker.Work(ctx, feedbackJob(id)))
		require.Equal(t, domain.InboundAttributed, inboundStatus(t, events, id))
		require.Equal(t, 1, b.eventCount(t, tenantID))
	})
}

func TestFeedbackWorkerAttributedTransactionalPath(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		events := b.events(t)
		worker := newFeedbackWorker(events)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		pm := "pm-" + dbtest.RandString()
		b.newTransactional(t, tenantID, pm)

		id := stageInbound(t, events, newInbound(t, domain.KindComplaint, pm))

		require.NoError(t, worker.Work(ctx, feedbackJob(id)))
		require.Equal(t, domain.InboundAttributed, inboundStatus(t, events, id))
		require.Equal(t, 1, b.eventCount(t, tenantID))
	})
}

func TestFeedbackWorkerUnattributedPath(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		events := b.events(t)
		worker := newFeedbackWorker(events)
		ctx := context.Background()

		id := stageInbound(t, events, newInbound(t, domain.KindBounce, "pm-orphan-"+dbtest.RandString()))

		require.NoError(t, worker.Work(ctx, feedbackJob(id)))
		require.Equal(t, domain.InboundUnattributed, inboundStatus(t, events, id))
	})
}

func TestFeedbackWorkerIsResumable(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		events := b.events(t)
		worker := newFeedbackWorker(events)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		pm := "pm-" + dbtest.RandString()
		seedSend(t, b, tenantID, pm)

		id := stageInbound(t, events, newInbound(t, domain.KindBounce, pm))

		// Running the job twice — as a redelivery would — records the event
		// exactly once.
		require.NoError(t, worker.Work(ctx, feedbackJob(id)))
		require.NoError(t, worker.Work(ctx, feedbackJob(id)))
		require.Equal(t, 1, b.eventCount(t, tenantID))
	})
}

func TestFeedbackWorkerAppliesSuppressionOnBounce(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		events := b.events(t)
		suppressions := b.suppressions(t)
		settings := b.settings(t)
		applier := adapters.NewSuppressionApplier(suppressions, settings)
		worker := adapters.NewFeedbackWorker(
			command.NewProcessFeedbackHandler(events, applier, slog.Default()))
		ctx := context.Background()
		tenantID := b.newTenant(t)
		pm := "pm-" + dbtest.RandString()
		seedSend(t, b, tenantID, pm)

		id := stageInbound(t, events, newInbound(t, domain.KindBounce, pm))
		require.NoError(t, worker.Work(ctx, feedbackJob(id)))

		page, _, err := suppressions.List(ctx, tenantID, domain.SuppressionFilter{})
		require.NoError(t, err)
		require.Len(t, page, 1, "a hard bounce suppresses the recipient")
		require.Equal(t, domain.ReasonHardBounce, page[0].Reason())
	})
}
```


- [ ] **Step 7: Format and verify.**

```bash
gofmt -l internal
go build ./... && go vet ./internal/deliverability/...
golangci-lint run ./internal/deliverability/...
export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock" && go test ./internal/deliverability/... -count=1 -timeout 8m
go test ./internal/deliverability/adapters -count=3 -run 'RaceFree' -timeout 8m
```

Expected: `gofmt -l` prints nothing, `0 issues.`, every package `ok`.

- [ ] **Step 8: Sabotage check.** In `events_ydb.go`, make `StageInbound` skip its existence read (change `if err == nil {` to `if false && err == nil {` and delete the `ErrNoRows` guard below it): `TestEventsStageInboundIsIdempotent/ydb` and `TestEventsStageInboundIsRaceFree/ydb` must fail. Do the same for the read in `RecordEvent`: `TestEventsRecordEventIsIdempotent/ydb` and `TestEventsRecordEventIsRaceFree/ydb` must fail. Undo both edits by hand (the file is new, so `git checkout` cannot) and re-run Step 7.

- [ ] **Step 9: Commit.**

```bash
git add internal/deliverability
git commit -m "feat(deliverability): port inbound events and attribution to YDB, add the pending-feedback sweep"
```

---

### Task 3: Campaign analytics on YDB

**Files:**
- Create: `internal/deliverability/adapters/analytics_ydb.go`
- Modify: `internal/deliverability/adapters/backend_test.go`
- Replace: `internal/deliverability/adapters/helpers_test.go`
- Rename and rewrite: `analytics_pg_test.go` -> `analytics_test.go`

**Interfaces:**
- Consumes: everything Tasks 1 and 2 produced; `domain.AnalyticsRepository`; the B7 `campaigns` and `campaign_recipients` tables.
- Produces:
  ```go
  // package adapters
  func NewYDBAnalytics(d *ydb.Driver) *YDBAnalytics
  ```
  and the `backend` field `analytics`.

- [ ] **Step 1: Create `analytics_ydb.go`.**

```go
package adapters

import (
	"context"
	"errors"
	"fmt"
	"time"

	ydb "github.com/ydb-platform/ydb-go-sdk/v3"
	"github.com/ydb-platform/ydb-go-sdk/v3/query"

	"github.com/nikolaymatrosov/nvelope/internal/deliverability/domain"
)

// YDBAnalytics is the YDB-backed implementation of domain.AnalyticsRepository,
// for the serverless runtime. Every query filters by the tenant's primary-key
// prefix.
type YDBAnalytics struct {
	driver *ydb.Driver
}

var _ domain.AnalyticsRepository = (*YDBAnalytics)(nil)

// NewYDBAnalytics builds a YDBAnalytics repository over the driver.
func NewYDBAnalytics(d *ydb.Driver) *YDBAnalytics {
	return &YDBAnalytics{driver: d}
}

// GetCampaign returns one campaign's pre-computed roll-up; ok is false when the
// campaign has no analytics row yet.
func (r *YDBAnalytics) GetCampaign(ctx context.Context, tenantID, campaignID string) (
	domain.CampaignAnalytics, bool, error) {

	row, err := r.driver.Query().QueryRow(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $campaign_id AS Utf8;
SELECT sent_count, delivered_count, opened_count, clicked_count, bounced_count,
       complained_count, refreshed_at
FROM campaign_analytics WHERE tenant_id = $tenant_id AND campaign_id = $campaign_id;`,
		query.WithParameters(ydb.ParamsBuilder().
			Param("$tenant_id").Text(tenantID).
			Param("$campaign_id").Text(campaignID).
			Build()))
	if errors.Is(err, query.ErrNoRows) {
		return domain.CampaignAnalytics{}, false, nil
	}
	if err != nil {
		return domain.CampaignAnalytics{}, false, fmt.Errorf("loading campaign analytics: %w", err)
	}
	var sent, delivered, opened, clicked, bounced, complained int64
	var refreshedAt time.Time
	if err := row.Scan(&sent, &delivered, &opened, &clicked, &bounced, &complained, &refreshedAt); err != nil {
		return domain.CampaignAnalytics{}, false, fmt.Errorf("scanning campaign analytics: %w", err)
	}
	return domain.CampaignAnalytics{
		CampaignID:  campaignID,
		Counts:      ydbCounts(sent, delivered, opened, clicked, bounced, complained),
		RefreshedAt: refreshedAt,
	}, true, nil
}

// ydbCounts maps the six Int64 count columns to domain.Counts.
func ydbCounts(sent, delivered, opened, clicked, bounced, complained int64) domain.Counts {
	return domain.Counts{
		Sent: int(sent), Delivered: int(delivered), Opened: int(opened),
		Clicked: int(clicked), Bounced: int(bounced), Complained: int(complained),
	}
}

// GetDashboard returns the tenant totals and the most recently sent campaigns.
// The totals and the list are read in one transaction, so they agree.
func (r *YDBAnalytics) GetDashboard(ctx context.Context, tenantID string) (domain.Dashboard, error) {
	var dash domain.Dashboard
	err := r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		row, err := tx.QueryRow(ctx, `DECLARE $tenant_id AS Utf8;
SELECT COALESCE(SUM(sent_count), 0), COALESCE(SUM(delivered_count), 0),
       COALESCE(SUM(opened_count), 0), COALESCE(SUM(clicked_count), 0),
       COALESCE(SUM(bounced_count), 0), COALESCE(SUM(complained_count), 0)
FROM campaign_analytics WHERE tenant_id = $tenant_id;`, ydbTenantParam(tenantID))
		if err != nil {
			return fmt.Errorf("aggregating dashboard totals: %w", err)
		}
		var sent, delivered, opened, clicked, bounced, complained int64
		if err := row.Scan(&sent, &delivered, &opened, &clicked, &bounced, &complained); err != nil {
			return fmt.Errorf("scanning dashboard totals: %w", err)
		}

		rs, err := tx.QueryResultSet(ctx, `DECLARE $tenant_id AS Utf8;
SELECT ca.campaign_id, c.name, ca.sent_count, ca.delivered_count, ca.opened_count,
       ca.clicked_count, ca.bounced_count, ca.complained_count
FROM campaign_analytics AS ca
JOIN campaigns AS c ON c.tenant_id = ca.tenant_id AND c.id = ca.campaign_id
WHERE ca.tenant_id = $tenant_id
ORDER BY COALESCE(c.started_at, c.created_at) DESC
LIMIT 10;`, ydbTenantParam(tenantID))
		if err != nil {
			return fmt.Errorf("listing recent campaigns: %w", err)
		}
		// Built in a local and assigned at the end (V6).
		var recent []domain.RecentCampaign
		for row, err := range rs.Rows(ctx) {
			if err != nil {
				return fmt.Errorf("listing recent campaigns: %w", err)
			}
			var rc domain.RecentCampaign
			var s, d, o, c, b, cm int64
			if err := row.Scan(&rc.CampaignID, &rc.Name, &s, &d, &o, &c, &b, &cm); err != nil {
				return fmt.Errorf("scanning recent campaign: %w", err)
			}
			rc.Counts = ydbCounts(s, d, o, c, b, cm)
			recent = append(recent, rc)
		}
		dash = domain.Dashboard{
			Totals: ydbCounts(sent, delivered, opened, clicked, bounced, complained),
			Recent: recent,
		}
		return nil
	})
	if err != nil {
		return domain.Dashboard{}, err
	}
	return dash, nil
}

// Refresh recomputes every campaign_analytics row of the tenant in one
// statement. Sent counts come from the campaign's 'sent' recipients, the five
// feedback counts from the distinct recipient addresses per event kind. A
// campaign with no recipient or event gets zeros. The recompute is idempotent.
//
// ponytail: it reads every recipient and delivery event of the tenant on each
// refresh. Fine for one organisation; the upgrade path is counters kept on the
// campaign row, or a refresh that only reads events newer than the last one.
func (r *YDBAnalytics) Refresh(ctx context.Context, tenantID string) error {
	if err := r.driver.Query().Exec(ctx, `DECLARE $tenant_id AS Utf8;
$sent = SELECT campaign_id, COUNT(*) AS n FROM campaign_recipients
        WHERE tenant_id = $tenant_id AND status = "sent"u GROUP BY campaign_id;
$events = SELECT campaign_id,
                 COUNT(DISTINCT IF(event_kind = "delivery"u, recipient_email)) AS delivered,
                 COUNT(DISTINCT IF(event_kind = "open"u, recipient_email)) AS opened,
                 COUNT(DISTINCT IF(event_kind = "click"u, recipient_email)) AS clicked,
                 COUNT(DISTINCT IF(event_kind = "bounce"u, recipient_email)) AS bounced,
                 COUNT(DISTINCT IF(event_kind = "complaint"u, recipient_email)) AS complained
          FROM delivery_events
          WHERE tenant_id = $tenant_id AND campaign_id IS NOT NULL GROUP BY campaign_id;
UPSERT INTO campaign_analytics
  (tenant_id, campaign_id, sent_count, delivered_count, opened_count, clicked_count,
   bounced_count, complained_count, refreshed_at)
SELECT c.tenant_id AS tenant_id, c.id AS campaign_id,
       Unwrap(CAST(COALESCE(s.n, 0ul) AS Int64)) AS sent_count,
       Unwrap(CAST(COALESCE(e.delivered, 0ul) AS Int64)) AS delivered_count,
       Unwrap(CAST(COALESCE(e.opened, 0ul) AS Int64)) AS opened_count,
       Unwrap(CAST(COALESCE(e.clicked, 0ul) AS Int64)) AS clicked_count,
       Unwrap(CAST(COALESCE(e.bounced, 0ul) AS Int64)) AS bounced_count,
       Unwrap(CAST(COALESCE(e.complained, 0ul) AS Int64)) AS complained_count,
       CurrentUtcTimestamp() AS refreshed_at
FROM campaigns AS c
LEFT JOIN $sent AS s ON s.campaign_id = c.id
LEFT JOIN $events AS e ON e.campaign_id = c.id
WHERE c.tenant_id = $tenant_id AND c.status != "draft"u;`, ydbTenantParam(tenantID)); err != nil {
		return fmt.Errorf("refreshing campaign analytics: %w", err)
	}
	return nil
}
```


- [ ] **Step 2: Add `analytics` to the harness.** Apply this change to `backend_test.go`.

```diff
@@ -27,6 +27,7 @@
 	events       func(t *testing.T) domain.EventRepository
 	suppressions func(t *testing.T) domain.SuppressionRepository
 	settings     func(t *testing.T) domain.SettingsRepository
+	analytics    func(t *testing.T) domain.AnalyticsRepository
 	checker      func(t *testing.T) campaigndomain.SuppressionChecker
 	audit        func(t *testing.T) command.AuditWriter
 	// The seeds below create rows owned by other contexts. On Postgres they
@@ -53,6 +54,9 @@
 		settings: func(t *testing.T) domain.SettingsRepository {
 			return adapters.NewSettings(dbtest.AppPool(t))
 		},
+		analytics: func(t *testing.T) domain.AnalyticsRepository {
+			return adapters.NewAnalytics(dbtest.AppPool(t))
+		},
 		checker: func(t *testing.T) campaigndomain.SuppressionChecker {
 			return adapters.NewSuppressionChecker(dbtest.AppPool(t))
 		},
@@ -127,6 +131,9 @@
 		settings: func(t *testing.T) domain.SettingsRepository {
 			return adapters.NewYDBSettings(dbtest.YDBMigrated(t))
 		},
+		analytics: func(t *testing.T) domain.AnalyticsRepository {
+			return adapters.NewYDBAnalytics(dbtest.YDBMigrated(t))
+		},
 		checker: func(t *testing.T) campaigndomain.SuppressionChecker {
 			return adapters.NewYDBSuppressionChecker(dbtest.YDBMigrated(t))
 		},
```


- [ ] **Step 3: Replace `helpers_test.go`.** `seedCampaignRecipient` goes: no test uses it after the next step.

```go
package adapters_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
	"github.com/nikolaymatrosov/nvelope/internal/deliverability/domain"
)

// seedTenant inserts a tenant row directly and returns its id.
func seedTenant(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var id string
	require.NoError(t, pool.QueryRow(context.Background(),
		`INSERT INTO tenants (name, slug, status) VALUES ('Workspace', $1, 'active') RETURNING id`,
		"dlv-"+dbtest.RandString()).Scan(&id))
	return id
}

// newInbound builds an inbound notification for tests.
func newInbound(t *testing.T, kind domain.EventKind, providerMsgID string) domain.InboundNotification {
	t.Helper()
	n, err := domain.NewInboundNotification("dk-"+dbtest.RandString(), kind,
		providerMsgID, "rcpt@acme.com", time.Now(), []byte(`{"k":"v"}`))
	require.NoError(t, err)
	return n
}

// stageInbound stages n and, so that the shared test database does not pile up
// pending notifications for PendingInbound to wade through, marks it failed
// when the test ends if it is still pending.
func stageInbound(t *testing.T, repo domain.EventRepository, n domain.InboundNotification) string {
	t.Helper()
	ctx := context.Background()
	id, _, err := repo.StageInbound(ctx, n)
	require.NoError(t, err)
	t.Cleanup(func() {
		if got, err := repo.LoadInbound(ctx, id); err == nil && got.Status == domain.InboundPending {
			_ = repo.MarkInbound(ctx, id, domain.InboundFailed)
		}
	})
	return id
}
```


- [ ] **Step 4: Rename the analytics test and replace it.** It covers the refresh (distinct-recipient counts, idempotence, drafts skipped, quiet campaigns zero-filled), the missing row, the dashboard with tenant isolation, and the cap of ten recent campaigns.

```bash
git mv internal/deliverability/adapters/analytics_pg_test.go internal/deliverability/adapters/analytics_test.go
```

```go
package adapters_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
	"github.com/nikolaymatrosov/nvelope/internal/deliverability/domain"
)

// recordEvent stages and records one delivery event of the given kind for the
// provider message id, attributing it within the tenant. email is the
// recipient the event names.
func recordEvent(t *testing.T, b backend, tenantID, pm string, kind domain.EventKind, email string) {
	t.Helper()
	ctx := context.Background()
	events := b.events(t)
	n, err := domain.NewInboundNotification("dk-"+dbtest.RandString(), kind, pm, email,
		time.Now(), []byte(`{"k":"v"}`))
	require.NoError(t, err)
	inboundID := stageInbound(t, events, n)
	attr, ok, err := events.Attribute(ctx, tenantID, pm)
	require.NoError(t, err)
	require.True(t, ok)
	ev, err := domain.NewDeliveryEvent(tenantID, inboundID, kind, email, pm, time.Now(), attr)
	require.NoError(t, err)
	recorded, err := events.RecordEvent(ctx, ev)
	require.NoError(t, err)
	require.True(t, recorded)
}

func TestAnalyticsRefreshComputesCounts(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		analytics := b.analytics(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		pm := "pm-" + dbtest.RandString()
		campaignID, _ := seedSend(t, b, tenantID, pm)
		// A failed recipient is not a sent one.
		b.newRecipient(t, tenantID, campaignID, "failed", "pm-"+dbtest.RandString())

		// Counts are distinct recipients per kind: a repeated delivery to one
		// address counts once, and a second address counts again.
		recordEvent(t, b, tenantID, pm, domain.KindDelivery, "a@acme.com")
		recordEvent(t, b, tenantID, pm, domain.KindDelivery, "a@acme.com")
		recordEvent(t, b, tenantID, pm, domain.KindDelivery, "b@acme.com")
		recordEvent(t, b, tenantID, pm, domain.KindOpen, "a@acme.com")
		recordEvent(t, b, tenantID, pm, domain.KindClick, "a@acme.com")
		recordEvent(t, b, tenantID, pm, domain.KindBounce, "b@acme.com")
		recordEvent(t, b, tenantID, pm, domain.KindComplaint, "b@acme.com")

		require.NoError(t, analytics.Refresh(ctx, tenantID))

		got, ok, err := analytics.GetCampaign(ctx, tenantID, campaignID)
		require.NoError(t, err)
		require.True(t, ok)
		require.Equal(t, domain.Counts{
			Sent: 1, Delivered: 2, Opened: 1, Clicked: 1, Bounced: 1, Complained: 1,
		}, got.Counts)
		require.False(t, got.RefreshedAt.IsZero())

		// Refresh is idempotent — a re-run produces the same counts.
		require.NoError(t, analytics.Refresh(ctx, tenantID))
		again, _, err := analytics.GetCampaign(ctx, tenantID, campaignID)
		require.NoError(t, err)
		require.Equal(t, got.Counts, again.Counts)
	})
}

func TestAnalyticsRefreshSkipsDraftsAndZeroFillsQuietCampaigns(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		analytics := b.analytics(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		draft := b.newCampaign(t, tenantID, "draft")
		quiet := b.newCampaign(t, tenantID, "finished")

		require.NoError(t, analytics.Refresh(ctx, tenantID))

		_, ok, err := analytics.GetCampaign(ctx, tenantID, draft)
		require.NoError(t, err)
		require.False(t, ok, "a draft has no analytics row")

		got, ok, err := analytics.GetCampaign(ctx, tenantID, quiet)
		require.NoError(t, err)
		require.True(t, ok, "a campaign with no recipient or event still gets a row")
		require.Equal(t, domain.Counts{}, got.Counts)
	})
}

func TestAnalyticsGetCampaignMissingRow(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		analytics := b.analytics(t)
		tenantID := b.newTenant(t)

		_, ok, err := analytics.GetCampaign(context.Background(), tenantID, "00000000-0000-0000-0000-000000000000")
		require.NoError(t, err)
		require.False(t, ok)
	})
}

func TestAnalyticsDashboardAndCrossTenantIsolation(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		analytics := b.analytics(t)
		ctx := context.Background()
		tenantA := b.newTenant(t)
		tenantB := b.newTenant(t)

		pm := "pm-" + dbtest.RandString()
		seedSend(t, b, tenantA, pm)
		recordEvent(t, b, tenantA, pm, domain.KindBounce, "rcpt@acme.com")
		require.NoError(t, analytics.Refresh(ctx, tenantA))
		require.NoError(t, analytics.Refresh(ctx, tenantB))

		dashA, err := analytics.GetDashboard(ctx, tenantA)
		require.NoError(t, err)
		require.Equal(t, 1, dashA.Totals.Sent)
		require.Equal(t, 1, dashA.Totals.Bounced)
		require.Len(t, dashA.Recent, 1)
		require.NotEmpty(t, dashA.Recent[0].Name)
		require.Equal(t, 1, dashA.Recent[0].Counts.Sent)

		// Tenant B sees none of tenant A's analytics.
		dashB, err := analytics.GetDashboard(ctx, tenantB)
		require.NoError(t, err)
		require.Zero(t, dashB.Totals.Sent)
		require.Empty(t, dashB.Recent)
	})
}

func TestAnalyticsDashboardListsTheTenLatestCampaignsAndTotalsAll(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		analytics := b.analytics(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		for range 12 {
			seedSend(t, b, tenantID, "pm-"+dbtest.RandString())
		}
		require.NoError(t, analytics.Refresh(ctx, tenantID))

		dash, err := analytics.GetDashboard(ctx, tenantID)
		require.NoError(t, err)
		require.Equal(t, 12, dash.Totals.Sent, "totals cover every campaign")
		require.Len(t, dash.Recent, 10, "the recent list is capped")
	})
}
```


- [ ] **Step 5: Format and verify the whole tree.**

```bash
gofmt -l internal
go build ./... && go vet ./...
golangci-lint run
PATH="$HOME/go/bin:$PATH" make lint-arch
export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock" && go test ./... -count=1 -timeout 15m
```

Expected: `gofmt -l` prints nothing, `0 issues.`, `[arch] clean`, and no failing package (49 `ok` lines in the dry run). `git diff --stat 5c296ad -- go.mod go.sum` is empty.

- [ ] **Step 6: Commit.**

```bash
git add internal/deliverability
git commit -m "feat(deliverability): port campaign analytics to YDB"
```

---

## Done criteria for Plan B8

- `000009_deliverability.up.sql` applies on a fresh YDB and again without error.
- Every deliverability repository, the pre-send checker and the audit writer have a YDB implementation held to the same suite as Postgres.
- `EventRepository.PendingInbound` exists on both backends, and `SweepPendingFeedbackHandler` re-enqueues the pending notifications.
- `go build ./... && go vet ./... && go test ./...`, `golangci-lint run` and `make lint-arch` are clean.
- **Not done here, left to Plan C4** (spec "Plan B8 planning spike", "For B9 and C4"):
  - wiring `NewYDBEvents`, `NewYDBSuppressions`, `NewYDBSettings`, `NewYDBAnalytics`, `NewYDBSuppressionChecker` and `NewYDBAuditLog` in the serverless composition root;
  - the `POST /cron/feedback-sweep` route in `WorkerRoutes`, its Terraform timer trigger, and a DLQ-style alert on pending notifications older than the threshold.

## Review follow-ups

Findings of the final review:

- **I1 (fixed, commit `fix(deliverability): apply suppression even when the delivery event was already recorded`).** `ProcessFeedback` skipped `Apply` when `RecordEvent` reported the event as already recorded, so a failure between the two was never retried and the sweep could not recover it. The `recorded &&` guard is dropped; a unit test covers it.
- **I2 (spec corrected, found, not fixed).** On Postgres, deleting a subscriber with any campaign delivery event fails on the `delivery_events` CHECK (an existing SaaS bug); YDB leaves a dangling `campaign_recipient_id`. The spec's "Dangling ids" and "Found, not fixed" now say so.
- **M1.** The spec's `Refresh` ceiling now says "no index on `campaign_id`" and that it scans every recipient of the tenant.
- **M2.** The spec's C4 notes now list `SendTransactional` and `ConfirmSubscription` as `NewYDBSuppressionChecker` consumers, the `WorkerRoutes` widening for the sweep route, and the `OlderThan` latency bound.
- **M3 (ceiling recorded).** `Refresh` as one read-write statement can be retried until the deadline during sends, and an `InboundFailed` that nothing sets can starve the sweep.
- **Deferred minors.** A negative `PendingInbound` limit is unreachable; the dashboard has no tie-breaker on either backend; the Task 3 cap test now asserts which campaigns come back and their order.

