# Serverless On-Prem — Plan B7 (campaign on YDB) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:**
- Port the `campaign` context to YDB: `YDBTemplates`, `YDBCampaigns`, `YDBRecipients`, `YDBTracking` and `YDBTransactionalMessages` behind the existing domain ports, held to the same contract tests as their Postgres counterparts.
- Write the two Postgres cascades into campaign tables that the audience YDB adapters now owe: a list delete removes the list's campaign targets, and a subscriber delete removes the subscriber's campaign recipients.
- Answer the open B2/V5 question: how `Recipients.BulkInsert` works on YDB, where a conflicting `INSERT` aborts the transaction.

**Architecture:**
- **Migration.** One file, `internal/db/ydbmigrations/000008_campaign.up.sql`, creates `templates`, `campaigns`, `campaign_lists`, `campaign_recipients`, `links`, `link_clicks`, `campaign_views` and `transactional_messages`. Every one is tenant-scoped. It also creates the two provider-message-id indexes that Plan B8 reads.
- **Adapters.** The YDB adapters sit next to the pg adapters in `internal/campaign/adapters` and call `db.Query()` / `DoTx` directly, like the B5 and B6 adapters. `ydb_common.go` holds the small parameter helpers. The pg helpers `nullableString` and `deref` (`campaigns_pg.go`) are reused.
- **BulkInsert.** Per chunk of 1000 recipients, one `DoTx` reads the emails the campaign already has, through the unique email index, and inserts the rest with `INSERT ... SELECT ... FROM AS_TABLE($rows)` (spec "Plan B7 planning spike").
- **Recipient writes.** `campaign_recipients` is keyed `(tenant_id, campaign_id, id)`, and the worker and the tracking endpoints know a recipient by id alone, so `MarkSent`, `MarkFailed` and `MarkSkipped` are `UPDATE ... ON SELECT ... FROM ... VIEW idx_campaign_recipients_id`.
- **Contract tests.** The pg tests of templates, campaigns, archive, recipients, tracking and transactional messages become pg+ydb suites through an `eachBackend` harness in `helpers_test.go`, built up over Tasks 1–3. The send-pipeline and resumability tests (the start and batch workers) run on both backends (Task 4). The two start-worker quota tests stay on Postgres, because billing is not ported.

**Tech Stack:**
- Go 1.26
- `github.com/ydb-platform/ydb-go-sdk/v3` v3.136.4 (query service, `table/types` for the bulk-insert parameter)
- testcontainers-go, local-ydb `26.3.1.23`
- testify, `github.com/google/uuid`

**Design spec:** [docs/superpowers/specs/2026-11-14-serverless-onprem-design.md](../specs/2026-11-14-serverless-onprem-design.md).
- Binding sections:
  - "Storage layer (YDB)": Transactions; Schema and migrations; SQL translation rules, including the rows this plan adds or settles.
  - "Spike findings (Plan A)": V5 and V6.
  - "Plan B2 planning spike" to "Plan B7 planning spike".
  - "Ceilings".
- The B7 spike found the one construct that decides the design: a conflicting `INSERT` rolls the transaction back, so the next statement fails with `Transaction not found`. Every other YQL construct is in the spike's list of what passed.
- **Dry run.** During planning, every code block in this plan was applied stage by stage to the worktree, then reverted. Stage N is Task N below.
  - After each stage, `go vet`, `golangci-lint run` and the campaign and audience adapter suites passed. Stage 1 also ran `internal/db`.
  - Each cascade and the BulkInsert dedup was sabotage-checked: with the code reverted, its test fails (Task 1: the list-target delete and the template unlink; Task 2: the recipient delete and the existence read).
  - `TestRecipientsBulkInsertIsRaceFree` and `TestTrackingUpsertLinksIsRaceFree` passed three repeated runs.
  - On the final state:
    - `go build ./... && go vet ./... && go test ./...` had no failing package (49 `ok` lines);
    - `golangci-lint run` reported `0 issues.`;
    - `make lint-arch` reported `[arch] clean`;
    - `go.mod` and `go.sum` were unchanged.
  - A run that starts while Ryuk is still reaping the previous run's containers can hang. It happened twice during the dry run; killing it and re-running passed.

**Scope:**
- This is the seventh Plan B plan. The port order continues with deliverability and media.
- Not in scope:
  - Wiring the YDB adapters into the composition root. That is Plan C4 (see "Done criteria").
  - The workers' logic. `start_worker.go` and `batch_worker.go` do not change; Task 4 changes only their tests.
  - The Postgres adapters. The only change is one line in `recipients_pg.go` (Task 2).
  - `campaign_analytics` and `delivery_events`: Plan B8.

## Global Constraints

- **Branch.**
  - Work on `feat/serverless-onprem` in the worktree `/Users/nikthespirit/experiments/nvelope-wt/serverless-onprem`. Both already exist, and the branch starts at `main` (`c59536b`) plus the commit that adds this plan.
  - Do not create branches or worktrees.
  - The main checkout belongs to other sessions.
  - Never use bare `git stash` or `git stash pop`; the stash is shared.
- **Docker.**
  - Docker on this machine is OrbStack. Prefix every `go test` / `make test` / `make ci` with `export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock" && `.
  - Ryuk reaps test containers shortly after a run ends. A run that starts within that window can hang or fail with `No such container`; kill it (`pkill -f adapters.test`) and re-run.
  - The shared containers `nvelope-test-pg`, `nvelope-test-ydb`, `nvelope-test-s3-strict` and `nvelope-test-sqs` may belong to another session's live run. Never `docker rm` them.
- **YDB container.** It is `ydbplatform/local-ydb:26.3.1.23`, amd64 under Rosetta. A fresh container can take about 45 s to start, so use `-timeout 8m` on runs that touch YDB, and `-timeout 15m` for the full suite.
- **Known environmental flakes. None of these is caused by this plan:**
  - `internal/platform/ratelimit` `TestAllowIsConsistentUnderConcurrency` can fail with `redis eval failed: i/o timeout` under full-suite load. Re-run that package alone.
  - pg-backed packages can all fail at once with `no migration found for version N`, where N is above `24`, or with `unexpected postmaster exit`. That means another worktree's run is using the shared `nvelope-test-pg`. Re-run once it is gone.
  - `internal/api` `TestPublicSubscriptionCrossTenantIsolation` can fail on a reused pg database (a fixed `hash-a` token), and `TestEmailVerificationsDeleteExpiredBefore` on a duplicate token hash. Re-run once.
- **Dependencies.** No new Go modules and no `go.mod` change.
- **SaaS behaviour.** Postgres adapters, River, Redis and every existing test keep their behaviour. `go test ./...` passes after every task.
- **YQL conventions** (spec "SQL translation rules" and V4):
  - Every query `DECLARE`s each parameter: `DECLARE $id AS Utf8;`. A nullable column's parameter is `Optional<...>` and is bound with `BeginOptional()...EndOptional()` from a pointer.
  - Literals that target a `Utf8` column take the `u` suffix (`"pending"u`); a bare `"..."` is `String`.
  - Every DDL statement is idempotent (`CREATE TABLE IF NOT EXISTS`). A statement ends on a line ending in `;`. Never end a line with `;` inside a string literal or a `--` comment.
  - A secondary index is read only through an explicit `VIEW <index_name>`.
- **Tenant scoping.** Every table's primary key starts with `tenant_id`, and every query filters by the `tenantID` the port receives. The only queries without it are the two tracking resolvers, which read a global index by id on purpose.
- **IDs** are UUIDv4 strings generated in Go (`uuid.NewString()`) before any `DoTx`, and stored as `Utf8`.
- **No column defaults, no CHECK constraints.** Every `INSERT` writes every `NOT NULL` column. Only the `created_at`, `updated_at`, `sent_at`, `clicked_at` and `viewed_at` values come from `CurrentUtcTimestamp()`.
- **Foreign keys.** YDB has none. The cascades are explicit: `YDBLists.Delete`, `YDBSubscribers.Delete` and `YDBTemplates.Delete` (spec "Plan B7 planning spike").
- **`DoTx` closures must be retry-safe** (V6):
  - only database operations inside, plus the port's `fn` callbacks;
  - ids, encoded JSON and domain values are built before `DoTx`;
  - values the caller reads are assigned at the end of the closure, never accumulated across runs;
  - never `append` to a slice, or write to a map, declared outside the closure.
- **Duplicate key mapping.**
  - "Already exists" comes from `db.IsYDBConflict(err)`. Never match on the message text.
  - Inside a `DoTx`, check it on the statement and also on the error `DoTx` returns, for a YDB that reports the conflict at commit.
  - Never add `query.WithIdempotent()` to an insert path.
- **Commits.** Use conventional messages. **No `Co-Authored-By` trailer** of any kind.
- **Tooling.**
  - `goimports` is not installed, so use `gofmt -w`.
  - `golangci-lint` is on PATH. It lints test files too.
  - Run arch lint as `PATH="$HOME/go/bin:$PATH" make lint-arch`, with PATH set inline. A worktree guard refuses `export PATH` combined with git commands, and it refuses complex compound git commands. Run git commands one per line, as the commit steps show.

## File structure

| File | Responsibility |
|------|----------------|
| `internal/db/ydbmigrations/000008_campaign.up.sql` | the eight tables and their indexes |
| `internal/campaign/adapters/ydb_common.go` | parameter helpers shared by the YDB adapters |
| `internal/campaign/adapters/templates_ydb.go` | `YDBTemplates`, with the unlink of campaigns on delete |
| `internal/campaign/adapters/campaigns_ydb.go` | `YDBCampaigns`: campaigns, the archive, targets |
| `internal/campaign/adapters/transactional_ydb.go` | `YDBTransactionalMessages` |
| `internal/campaign/adapters/recipients_ydb.go` | `YDBRecipients`: chunked `BulkInsert`, the batch window, the marks, the counts |
| `internal/campaign/adapters/tracking_ydb.go` | `YDBTracking`: links, clicks, views, the two tenant resolvers |
| `internal/campaign/adapters/recipients_pg.go` | resets the `BulkInsert` counter inside its closure (V6) |
| `internal/audience/adapters/lists_ydb.go`, `subscribers_ydb.go` | the two cascades into campaign tables |
| `internal/campaign/adapters/helpers_test.go` | the `backend` harness, built up over Tasks 1–3 |
| `internal/campaign/adapters/{templates,campaigns,archive,transactional,recipients,tracking}_test.go` | were `*_pg_test.go`; pg+ydb contract suites |
| `internal/campaign/adapters/{send_pipeline,resumability}_test.go` | the worker tests on both backends |
| `internal/audience/adapters/lists_ydb_test.go`, `subscribers_ydb_test.go` | the cascades, YDB only |

---
### Task 1: Migration, templates, campaigns and transactional messages on YDB

**Files:**
- Create: `internal/db/ydbmigrations/000008_campaign.up.sql`
- Create: `internal/campaign/adapters/ydb_common.go`, `templates_ydb.go`, `campaigns_ydb.go`, `transactional_ydb.go`
- Modify: `internal/campaign/adapters/helpers_test.go`
- Rename and rewrite: `templates_pg_test.go` → `templates_test.go`, `campaigns_pg_test.go` → `campaigns_test.go`, `archive_pg_test.go` → `archive_test.go`, `transactional_pg_test.go` → `transactional_test.go`
- Modify: `internal/audience/adapters/lists_ydb.go`, `internal/audience/adapters/lists_ydb_test.go`

**Interfaces:**
- Consumes:
  - The ports `domain.TemplateRepository`, `domain.CampaignRepository` and `domain.TransactionalMessageRepository` (`internal/campaign/domain/repository.go`), unchanged.
  - `domain.HydrateTemplate`, `domain.HydrateCampaign`, unchanged.
  - From the pg adapters: `nullableString(string) *string` and `deref(*string) string` (`campaigns_pg.go`).
  - `db.IsYDBConflict`, `dbtest.YDBMigrated`.
- Produces:
  ```go
  // package adapters
  func NewYDBTemplates(d *ydb.Driver) *YDBTemplates
  func NewYDBCampaigns(d *ydb.Driver) *YDBCampaigns
  func NewYDBTransactionalMessages(d *ydb.Driver) *YDBTransactionalMessages
  ```

- [ ] **Step 1: Create the migration.** It has no `DEFAULT`, no `CHECK` and no foreign key. The comments explain each index.

```sql
-- Campaign context (internal/campaign): templates, campaigns and their send
-- targets, per-recipient send progress, tracked links with their click and
-- view events, and the record of transactional sends. Every table is
-- tenant-scoped: its primary key starts with tenant_id and every query filters
-- by it. No column has a DEFAULT, so the adapters write the values Postgres
-- takes from its column defaults. YDB has no foreign keys and no CHECK
-- constraints: the domain keeps statuses and kinds valid, and the adapters
-- write the cascades the port needs (see the spec, "Plan B7 planning spike").

-- name is unique per tenant, as UNIQUE (tenant_id, name) is on Postgres.
-- body_doc and theme are JSON only stored and read back whole.
CREATE TABLE IF NOT EXISTS templates (
    tenant_id  Utf8      NOT NULL,
    id         Utf8      NOT NULL,
    name       Utf8      NOT NULL,
    kind       Utf8      NOT NULL,
    subject    Utf8      NOT NULL,
    body_html  Utf8      NOT NULL,
    body_text  Utf8      NOT NULL,
    body_doc   Utf8,
    theme      Utf8,
    created_at Timestamp NOT NULL,
    updated_at Timestamp NOT NULL,
    PRIMARY KEY (tenant_id, id),
    INDEX idx_templates_name GLOBAL UNIQUE SYNC ON (tenant_id, name)
);

-- idx_campaigns_id lets the public open-tracking endpoint find the owning
-- tenant of a campaign id, which Postgres does through a SECURITY DEFINER
-- function. sending_domain_id and template_id are NULL when unset.
CREATE TABLE IF NOT EXISTS campaigns (
    tenant_id         Utf8      NOT NULL,
    id                Utf8      NOT NULL,
    name              Utf8      NOT NULL,
    subject           Utf8      NOT NULL,
    body_html         Utf8      NOT NULL,
    body_text         Utf8      NOT NULL,
    from_name         Utf8      NOT NULL,
    from_local_part   Utf8      NOT NULL,
    sending_domain_id Utf8,
    template_id       Utf8,
    status            Utf8      NOT NULL,
    max_send_errors   Int64     NOT NULL,
    sent_count        Int64     NOT NULL,
    failed_count      Int64     NOT NULL,
    recipient_count   Int64     NOT NULL,
    body_doc          Utf8,
    theme             Utf8,
    created_at        Timestamp NOT NULL,
    updated_at        Timestamp NOT NULL,
    started_at        Timestamp,
    finished_at       Timestamp,
    archive_visible   Bool      NOT NULL,
    archived_at       Timestamp,
    PRIMARY KEY (tenant_id, id),
    INDEX idx_campaigns_id GLOBAL UNIQUE SYNC ON (id)
);

-- A target is a list (list_id) or a segment (segment_query), never both.
-- idx_campaign_lists_unique reproduces UNIQUE (campaign_id, list_id): YDB
-- admits many NULL list_ids, as Postgres does. idx_campaign_lists_list serves
-- the delete of a list, which removes its targets.
CREATE TABLE IF NOT EXISTS campaign_lists (
    tenant_id     Utf8 NOT NULL,
    campaign_id   Utf8 NOT NULL,
    id            Utf8 NOT NULL,
    list_id       Utf8,
    segment_query Utf8,
    PRIMARY KEY (tenant_id, campaign_id, id),
    INDEX idx_campaign_lists_list GLOBAL SYNC ON (tenant_id, list_id),
    INDEX idx_campaign_lists_unique GLOBAL UNIQUE SYNC ON (tenant_id, campaign_id, list_id)
);

-- The primary key makes a campaign's recipients one key range, in id order,
-- which the batch window reads. idx_campaign_recipients_id finds a recipient
-- from its id alone (the send worker and the tracking endpoints know only the
-- id). The email index reproduces UNIQUE (campaign_id, email). The subscriber
-- index serves the delete of a subscriber, which removes its recipient rows.
-- The provider index lets delivery feedback find the recipient of a provider
-- message id, with or without the tenant (Plan B8).
CREATE TABLE IF NOT EXISTS campaign_recipients (
    tenant_id           Utf8      NOT NULL,
    campaign_id         Utf8      NOT NULL,
    id                  Utf8      NOT NULL,
    subscriber_id       Utf8      NOT NULL,
    email               Utf8      NOT NULL,
    status              Utf8      NOT NULL,
    failure_reason      Utf8      NOT NULL,
    sent_at             Timestamp,
    provider_message_id Utf8,
    PRIMARY KEY (tenant_id, campaign_id, id),
    INDEX idx_campaign_recipients_id GLOBAL SYNC ON (tenant_id, id),
    INDEX idx_campaign_recipients_email GLOBAL UNIQUE SYNC ON (tenant_id, campaign_id, email),
    INDEX idx_campaign_recipients_subscriber GLOBAL SYNC ON (tenant_id, subscriber_id),
    INDEX idx_campaign_recipients_provider GLOBAL SYNC ON (provider_message_id)
);

-- idx_links_id lets the public click-tracking endpoint find the owning tenant
-- of a link id. idx_links_url reproduces UNIQUE (campaign_id, url).
CREATE TABLE IF NOT EXISTS links (
    tenant_id   Utf8      NOT NULL,
    id          Utf8      NOT NULL,
    campaign_id Utf8      NOT NULL,
    url         Utf8      NOT NULL,
    created_at  Timestamp NOT NULL,
    PRIMARY KEY (tenant_id, id),
    INDEX idx_links_id GLOBAL UNIQUE SYNC ON (id),
    INDEX idx_links_url GLOBAL UNIQUE SYNC ON (tenant_id, campaign_id, url)
);

-- Click and view events are only written: no query reads them, so they carry
-- no index and need no cascade from a deleted recipient.
CREATE TABLE IF NOT EXISTS link_clicks (
    tenant_id    Utf8      NOT NULL,
    id           Utf8      NOT NULL,
    link_id      Utf8      NOT NULL,
    campaign_id  Utf8      NOT NULL,
    recipient_id Utf8      NOT NULL,
    clicked_at   Timestamp NOT NULL,
    PRIMARY KEY (tenant_id, id)
);

CREATE TABLE IF NOT EXISTS campaign_views (
    tenant_id    Utf8      NOT NULL,
    id           Utf8      NOT NULL,
    campaign_id  Utf8      NOT NULL,
    recipient_id Utf8      NOT NULL,
    viewed_at    Timestamp NOT NULL,
    PRIMARY KEY (tenant_id, id)
);

-- provider_message_id is unique across tenants, as on Postgres: delivery
-- feedback resolves the owning tenant from it (Plan B8). template_id is only
-- written, so a deleted template leaves it dangling.
CREATE TABLE IF NOT EXISTS transactional_messages (
    tenant_id           Utf8      NOT NULL,
    id                  Utf8      NOT NULL,
    template_id         Utf8,
    provider_message_id Utf8      NOT NULL,
    recipient_email     Utf8      NOT NULL,
    sent_at             Timestamp NOT NULL,
    PRIMARY KEY (tenant_id, id),
    INDEX idx_transactional_messages_provider GLOBAL UNIQUE SYNC ON (provider_message_id)
);
```

- [ ] **Step 2: Create the helpers and the three adapters.**

**`internal/campaign/adapters/ydb_common.go`** (create):

```go
package adapters

import (
	"context"

	ydb "github.com/ydb-platform/ydb-go-sdk/v3"
	"github.com/ydb-platform/ydb-go-sdk/v3/query"
)

// ydbTenantParam binds the $tenant_id parameter.
func ydbTenantParam(tenantID string) query.ExecuteOption {
	return query.WithParameters(ydb.ParamsBuilder().Param("$tenant_id").Text(tenantID).Build())
}

// ydbTenantAndID binds the (tenant_id, id) primary key of an unscoped table.
func ydbTenantAndID(tenantID, id string) query.ExecuteOption {
	return query.WithParameters(ydb.ParamsBuilder().
		Param("$tenant_id").Text(tenantID).
		Param("$id").Text(id).
		Build())
}

// ydbTenantAndPage binds $tenant_id, $limit and $offset.
func ydbTenantAndPage(tenantID string, limit, offset int) query.ExecuteOption {
	return query.WithParameters(ydb.ParamsBuilder().
		Param("$tenant_id").Text(tenantID).
		Param("$limit").Uint64(uint64(limit)).
		Param("$offset").Uint64(uint64(offset)).
		Build())
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

// nullableJSONString maps an empty JSON document to nil for a nullable Utf8
// column that stores it whole.
func nullableJSONString(b []byte) *string {
	if len(b) == 0 {
		return nil
	}
	s := string(b)
	return &s
}

// jsonBytes returns the stored JSON document as bytes, nil when the column is
// NULL.
func jsonBytes(s *string) []byte {
	if s == nil {
		return nil
	}
	return []byte(*s)
}
```

**`internal/campaign/adapters/templates_ydb.go`** (create):

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

	"github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
	"github.com/nikolaymatrosov/nvelope/internal/db"
)

// YDBTemplates is the YDB-backed implementation of domain.TemplateRepository,
// for the serverless runtime. YDB has no row-level security, so every
// statement filters by tenant_id explicitly.
type YDBTemplates struct {
	driver *ydb.Driver
}

var _ domain.TemplateRepository = (*YDBTemplates)(nil)

// NewYDBTemplates builds a YDBTemplates repository over the driver.
func NewYDBTemplates(d *ydb.Driver) *YDBTemplates {
	return &YDBTemplates{driver: d}
}

// ydbTemplateColumns is the projection scanYDBTemplate reads, in order.
const ydbTemplateColumns = `id, tenant_id, name, kind, subject, body_html, body_text, body_doc, theme, created_at, updated_at`

const ydbSelectTemplate = `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
SELECT ` + ydbTemplateColumns + `
FROM templates
WHERE tenant_id = $tenant_id AND id = $id;`

// ydbTemplateWrite declares the parameters Add and Update bind from a
// template, in ydbTemplateParams order.
const ydbTemplateWrite = `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DECLARE $name AS Utf8;
DECLARE $kind AS Utf8;
DECLARE $subject AS Utf8;
DECLARE $body_html AS Utf8;
DECLARE $body_text AS Utf8;
DECLARE $body_doc AS Optional<Utf8>;
DECLARE $theme AS Optional<Utf8>;
`

func ydbTemplateParams(tenantID, id string, t *domain.Template) query.ExecuteOption {
	return query.WithParameters(ydb.ParamsBuilder().
		Param("$tenant_id").Text(tenantID).
		Param("$id").Text(id).
		Param("$name").Text(t.Name()).
		Param("$kind").Text(string(t.Kind())).
		Param("$subject").Text(t.Subject()).
		Param("$body_html").Text(t.BodyHTML()).
		Param("$body_text").Text(t.BodyText()).
		Param("$body_doc").BeginOptional().Text(nullableJSONString(t.BodyDocJSON())).EndOptional().
		Param("$theme").BeginOptional().Text(nullableJSONString(t.ThemeJSON())).EndOptional().
		Build())
}

// Add persists a new template under a fresh UUID and returns the id. It
// returns domain.ErrTemplateNameTaken when the tenant already has a template
// with that name.
func (r *YDBTemplates) Add(ctx context.Context, tenantID string, t *domain.Template) (string, error) {
	id := uuid.NewString()
	err := r.driver.Query().Exec(ctx, ydbTemplateWrite+`INSERT INTO templates
    (tenant_id, id, name, kind, subject, body_html, body_text, body_doc, theme, created_at, updated_at)
VALUES ($tenant_id, $id, $name, $kind, $subject, $body_html, $body_text, $body_doc, $theme,
    CurrentUtcTimestamp(), CurrentUtcTimestamp());`, ydbTemplateParams(tenantID, id, t))
	// Only the name index can collide: the id is fresh.
	if db.IsYDBConflict(err) {
		return "", domain.ErrTemplateNameTaken
	}
	if err != nil {
		return "", fmt.Errorf("inserting template: %w", err)
	}
	return id, nil
}

// Get returns the template, or domain.ErrTemplateNotFound.
func (r *YDBTemplates) Get(ctx context.Context, tenantID, id string) (*domain.Template, error) {
	return ydbTemplateFromRow(r.driver.Query().QueryRow(ctx, ydbSelectTemplate, ydbTenantAndID(tenantID, id)))
}

// Update loads the template, runs fn, and persists the result in one
// serializable transaction. The SDK retries the transaction on conflict, so fn
// may run more than once and must be retry-safe.
func (r *YDBTemplates) Update(ctx context.Context, tenantID, id string,
	fn func(*domain.Template) (*domain.Template, error)) error {

	err := r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		loaded, err := ydbTemplateFromRow(tx.QueryRow(ctx, ydbSelectTemplate, ydbTenantAndID(tenantID, id)))
		if err != nil {
			return err
		}
		updated, err := fn(loaded)
		if err != nil {
			return err
		}
		err = tx.Exec(ctx, ydbTemplateWrite+`UPDATE templates SET
    name = $name, subject = $subject, body_html = $body_html, body_text = $body_text,
    body_doc = $body_doc, theme = $theme, updated_at = CurrentUtcTimestamp()
WHERE tenant_id = $tenant_id AND id = $id;`, ydbTemplateParams(tenantID, id, updated))
		if db.IsYDBConflict(err) {
			return domain.ErrTemplateNameTaken
		}
		if err != nil {
			return fmt.Errorf("updating template: %w", err)
		}
		return nil
	})
	// This covers a YDB that reports the collision at commit.
	if db.IsYDBConflict(err) {
		return domain.ErrTemplateNameTaken
	}
	return err
}

// All returns a page of the tenant's templates, ordered by name, and the total
// count, read in one transaction.
func (r *YDBTemplates) All(ctx context.Context, tenantID string, page domain.Page) ([]*domain.Template, int, error) {
	page = page.Normalize()
	var templates []*domain.Template
	var total int
	err := r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		count, err := ydbCount(ctx, tx, `DECLARE $tenant_id AS Utf8;
SELECT COUNT(*) FROM templates WHERE tenant_id = $tenant_id;`, ydbTenantParam(tenantID))
		if err != nil {
			return fmt.Errorf("counting templates: %w", err)
		}
		// ponytail: OFFSET paging reads every skipped row (spec "Ceilings").
		rs, err := tx.QueryResultSet(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $limit AS Uint64;
DECLARE $offset AS Uint64;
SELECT `+ydbTemplateColumns+`
FROM templates
WHERE tenant_id = $tenant_id
ORDER BY name
LIMIT $limit OFFSET $offset;`, ydbTenantAndPage(tenantID, page.Limit, page.Offset))
		if err != nil {
			return fmt.Errorf("listing templates: %w", err)
		}
		// Collected into a local and assigned at the end: the SDK may re-run
		// the closure, and a retry must not append to rows a failed run read.
		var local []*domain.Template
		for row, err := range rs.Rows(ctx) {
			if err != nil {
				return fmt.Errorf("listing templates: %w", err)
			}
			t, err := scanYDBTemplate(row)
			if err != nil {
				return err
			}
			local = append(local, t)
		}
		templates, total = local, count
		return nil
	})
	if err != nil {
		return nil, 0, err
	}
	return templates, total, nil
}

// Delete removes a template, or returns domain.ErrTemplateNotFound when no
// such template exists for the tenant. Postgres sets campaigns.template_id to
// NULL by foreign-key action, so the same transaction does it here: a campaign
// keeps its copied content and loses the link. transactional_messages keeps
// its template_id, which nothing reads.
func (r *YDBTemplates) Delete(ctx context.Context, tenantID, id string) error {
	return r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		_, err := tx.QueryRow(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DELETE FROM templates
WHERE tenant_id = $tenant_id AND id = $id
RETURNING id;`, ydbTenantAndID(tenantID, id))
		if errors.Is(err, query.ErrNoRows) {
			return domain.ErrTemplateNotFound
		}
		if err != nil {
			return fmt.Errorf("deleting template: %w", err)
		}
		// ponytail: no index on campaigns.template_id, so this scans the
		// tenant's campaigns. Add a GLOBAL SYNC index on (tenant_id,
		// template_id) if a tenant outgrows that.
		if err := tx.Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
UPDATE campaigns SET template_id = NULL
WHERE tenant_id = $tenant_id AND template_id = $id;`, ydbTenantAndID(tenantID, id)); err != nil {
			return fmt.Errorf("unlinking template from campaigns: %w", err)
		}
		return nil
	})
}

// ydbTemplateFromRow maps a single-row read to the template, translating "no
// row" into domain.ErrTemplateNotFound.
func ydbTemplateFromRow(row query.Row, err error) (*domain.Template, error) {
	if errors.Is(err, query.ErrNoRows) {
		return nil, domain.ErrTemplateNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("loading template: %w", err)
	}
	return scanYDBTemplate(row)
}

// scanYDBTemplate reads one row in ydbTemplateColumns order.
func scanYDBTemplate(row query.Row) (*domain.Template, error) {
	var id, tenantID, name, kind, subject, bodyHTML, bodyText string
	var bodyDoc, theme *string
	var createdAt, updatedAt time.Time
	if err := row.Scan(&id, &tenantID, &name, &kind, &subject, &bodyHTML, &bodyText,
		&bodyDoc, &theme, &createdAt, &updatedAt); err != nil {
		return nil, fmt.Errorf("scanning template: %w", err)
	}
	return domain.HydrateTemplate(id, tenantID, name, domain.Kind(kind), subject,
		bodyHTML, bodyText, jsonBytes(bodyDoc), jsonBytes(theme), createdAt, updatedAt), nil
}
```

**`internal/campaign/adapters/campaigns_ydb.go`** (create):

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

	"github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
)

// YDBCampaigns is the YDB-backed implementation of domain.CampaignRepository,
// for the serverless runtime. YDB has no row-level security, so every
// statement filters by tenant_id explicitly.
type YDBCampaigns struct {
	driver *ydb.Driver
}

var _ domain.CampaignRepository = (*YDBCampaigns)(nil)

// NewYDBCampaigns builds a YDBCampaigns repository over the driver.
func NewYDBCampaigns(d *ydb.Driver) *YDBCampaigns {
	return &YDBCampaigns{driver: d}
}

// ydbCampaignColumns is the projection scanYDBCampaign reads, in order.
const ydbCampaignColumns = `id, tenant_id, name, subject, body_html, body_text, from_name,
    from_local_part, sending_domain_id, template_id, status, max_send_errors,
    sent_count, failed_count, recipient_count, body_doc, theme,
    created_at, updated_at, started_at, finished_at, archive_visible, archived_at`

const ydbSelectCampaign = `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
SELECT ` + ydbCampaignColumns + `
FROM campaigns
WHERE tenant_id = $tenant_id AND id = $id;`

// ydbCampaignWrite declares the parameters Update binds from a campaign.
const ydbCampaignWrite = `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DECLARE $name AS Utf8;
DECLARE $subject AS Utf8;
DECLARE $body_html AS Utf8;
DECLARE $body_text AS Utf8;
DECLARE $from_name AS Utf8;
DECLARE $from_local_part AS Utf8;
DECLARE $sending_domain_id AS Optional<Utf8>;
DECLARE $status AS Utf8;
DECLARE $max_send_errors AS Int64;
DECLARE $sent_count AS Int64;
DECLARE $failed_count AS Int64;
DECLARE $recipient_count AS Int64;
DECLARE $started_at AS Optional<Timestamp>;
DECLARE $finished_at AS Optional<Timestamp>;
DECLARE $archive_visible AS Bool;
DECLARE $archived_at AS Optional<Timestamp>;
DECLARE $body_doc AS Optional<Utf8>;
DECLARE $theme AS Optional<Utf8>;
`

// ydbCampaignParams binds every column Update writes. withTemplate adds
// template_id for Add: Update never changes it, as the Postgres UPDATE leaves
// it alone.
func ydbCampaignParams(tenantID, id string, c *domain.Campaign, withTemplate bool) query.ExecuteOption {
	b := ydb.ParamsBuilder().
		Param("$tenant_id").Text(tenantID).
		Param("$id").Text(id).
		Param("$name").Text(c.Name()).
		Param("$subject").Text(c.Subject()).
		Param("$body_html").Text(c.BodyHTML()).
		Param("$body_text").Text(c.BodyText()).
		Param("$from_name").Text(c.FromName()).
		Param("$from_local_part").Text(c.FromLocalPart()).
		Param("$sending_domain_id").BeginOptional().Text(nullableString(c.SendingDomainID())).EndOptional().
		Param("$status").Text(string(c.Status())).
		Param("$max_send_errors").Int64(int64(c.MaxSendErrors())).
		Param("$sent_count").Int64(int64(c.SentCount())).
		Param("$failed_count").Int64(int64(c.FailedCount())).
		Param("$recipient_count").Int64(int64(c.RecipientCount())).
		Param("$started_at").BeginOptional().Timestamp(c.StartedAt()).EndOptional().
		Param("$finished_at").BeginOptional().Timestamp(c.FinishedAt()).EndOptional().
		Param("$archive_visible").Bool(c.ArchiveVisible()).
		Param("$archived_at").BeginOptional().Timestamp(c.ArchivedAt()).EndOptional().
		Param("$body_doc").BeginOptional().Text(nullableJSONString(c.BodyDocJSON())).EndOptional().
		Param("$theme").BeginOptional().Text(nullableJSONString(c.ThemeJSON())).EndOptional()
	if withTemplate {
		b = b.Param("$template_id").BeginOptional().Text(nullableString(c.TemplateID())).EndOptional()
	}
	return query.WithParameters(b.Build())
}

// Add persists a new campaign under a fresh UUID and returns the id.
func (r *YDBCampaigns) Add(ctx context.Context, tenantID string, c *domain.Campaign) (string, error) {
	id := uuid.NewString()
	err := r.driver.Query().Exec(ctx, ydbCampaignWrite+`DECLARE $template_id AS Optional<Utf8>;
INSERT INTO campaigns
    (tenant_id, id, name, subject, body_html, body_text, from_name, from_local_part,
     sending_domain_id, template_id, status, max_send_errors, sent_count, failed_count,
     recipient_count, body_doc, theme, created_at, updated_at, started_at, finished_at,
     archive_visible, archived_at)
VALUES ($tenant_id, $id, $name, $subject, $body_html, $body_text, $from_name, $from_local_part,
    $sending_domain_id, $template_id, $status, $max_send_errors, $sent_count, $failed_count,
    $recipient_count, $body_doc, $theme, CurrentUtcTimestamp(), CurrentUtcTimestamp(),
    $started_at, $finished_at, $archive_visible, $archived_at);`, ydbCampaignParams(tenantID, id, c, true))
	if err != nil {
		return "", fmt.Errorf("inserting campaign: %w", err)
	}
	return id, nil
}

// Get returns the campaign, or domain.ErrCampaignNotFound.
func (r *YDBCampaigns) Get(ctx context.Context, tenantID, id string) (*domain.Campaign, error) {
	return ydbCampaignFromRow(r.driver.Query().QueryRow(ctx, ydbSelectCampaign, ydbTenantAndID(tenantID, id)))
}

// Update loads the campaign, runs fn, and persists the result in one
// serializable transaction. The SDK retries the transaction on conflict, so fn
// may run more than once and must be retry-safe.
func (r *YDBCampaigns) Update(ctx context.Context, tenantID, id string,
	fn func(*domain.Campaign) (*domain.Campaign, error)) error {

	return r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		loaded, err := ydbCampaignFromRow(tx.QueryRow(ctx, ydbSelectCampaign, ydbTenantAndID(tenantID, id)))
		if err != nil {
			return err
		}
		updated, err := fn(loaded)
		if err != nil {
			return err
		}
		err = tx.Exec(ctx, ydbCampaignWrite+`UPDATE campaigns SET
    name = $name, subject = $subject, body_html = $body_html, body_text = $body_text,
    from_name = $from_name, from_local_part = $from_local_part,
    sending_domain_id = $sending_domain_id, status = $status,
    max_send_errors = $max_send_errors, sent_count = $sent_count,
    failed_count = $failed_count, recipient_count = $recipient_count,
    started_at = $started_at, finished_at = $finished_at,
    archive_visible = $archive_visible, archived_at = $archived_at,
    body_doc = $body_doc, theme = $theme, updated_at = CurrentUtcTimestamp()
WHERE tenant_id = $tenant_id AND id = $id;`, ydbCampaignParams(tenantID, id, updated, false))
		if err != nil {
			return fmt.Errorf("updating campaign: %w", err)
		}
		return nil
	})
}

// All returns a page of the tenant's campaigns, newest first, and the total
// count, read in one transaction.
func (r *YDBCampaigns) All(ctx context.Context, tenantID string, page domain.Page) ([]*domain.Campaign, int, error) {
	return r.page(ctx, "campaigns", "", "created_at DESC", tenantID, page)
}

// Archived returns a page of the tenant's archive-visible campaigns,
// newest-first by archived_at, and the total count.
func (r *YDBCampaigns) Archived(ctx context.Context, tenantID string, page domain.Page) ([]*domain.Campaign, int, error) {
	return r.page(ctx, "archived campaigns", " AND archive_visible = true", "archived_at DESC", tenantID, page)
}

// page runs a COUNT(*) query and a page query over the tenant's campaigns in
// one transaction. where is an extra predicate, order the ORDER BY.
// ponytail: no index serves the archive filter or the order, so both scan the
// tenant's campaigns, and COUNT(*) plus OFFSET reads every matching row (spec
// "Ceilings"). Fine for one organisation's campaign history; keyset paging and
// an index on (tenant_id, archive_visible, archived_at) are the upgrade path.
func (r *YDBCampaigns) page(ctx context.Context, what, where, order, tenantID string,
	page domain.Page) ([]*domain.Campaign, int, error) {

	page = page.Normalize()
	var campaigns []*domain.Campaign
	var total int
	err := r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		count, err := ydbCount(ctx, tx, `DECLARE $tenant_id AS Utf8;
SELECT COUNT(*) FROM campaigns WHERE tenant_id = $tenant_id`+where+`;`, ydbTenantParam(tenantID))
		if err != nil {
			return fmt.Errorf("counting %s: %w", what, err)
		}
		rs, err := tx.QueryResultSet(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $limit AS Uint64;
DECLARE $offset AS Uint64;
SELECT `+ydbCampaignColumns+`
FROM campaigns
WHERE tenant_id = $tenant_id`+where+`
ORDER BY `+order+`
LIMIT $limit OFFSET $offset;`, ydbTenantAndPage(tenantID, page.Limit, page.Offset))
		if err != nil {
			return fmt.Errorf("listing %s: %w", what, err)
		}
		// Collected into a local and assigned at the end: the SDK may re-run
		// the closure, and a retry must not append to rows a failed run read.
		var local []*domain.Campaign
		for row, err := range rs.Rows(ctx) {
			if err != nil {
				return fmt.Errorf("listing %s: %w", what, err)
			}
			c, err := scanYDBCampaign(row)
			if err != nil {
				return err
			}
			local = append(local, c)
		}
		campaigns, total = local, count
		return nil
	})
	if err != nil {
		return nil, 0, err
	}
	return campaigns, total, nil
}

// SaveTargets replaces a campaign's targeted lists and segments. Every target
// gets a fresh UUID, and a list targeted twice collides on the unique index,
// as it does on Postgres.
func (r *YDBCampaigns) SaveTargets(ctx context.Context, tenantID, campaignID string,
	targets []domain.Target) error {

	ids := make([]string, len(targets))
	for i := range ids {
		ids[i] = uuid.NewString()
	}
	return r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		if err := tx.Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $campaign_id AS Utf8;
DELETE FROM campaign_lists WHERE tenant_id = $tenant_id AND campaign_id = $campaign_id;`,
			query.WithParameters(ydb.ParamsBuilder().
				Param("$tenant_id").Text(tenantID).
				Param("$campaign_id").Text(campaignID).
				Build())); err != nil {
			return fmt.Errorf("clearing campaign targets: %w", err)
		}
		for i, t := range targets {
			err := tx.Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $campaign_id AS Utf8;
DECLARE $id AS Utf8;
DECLARE $list_id AS Optional<Utf8>;
DECLARE $segment_query AS Optional<Utf8>;
INSERT INTO campaign_lists (tenant_id, campaign_id, id, list_id, segment_query)
VALUES ($tenant_id, $campaign_id, $id, $list_id, $segment_query);`,
				query.WithParameters(ydb.ParamsBuilder().
					Param("$tenant_id").Text(tenantID).
					Param("$campaign_id").Text(campaignID).
					Param("$id").Text(ids[i]).
					Param("$list_id").BeginOptional().Text(nullableString(t.ListID)).EndOptional().
					Param("$segment_query").BeginOptional().Text(nullableJSONString(t.SegmentQuery)).EndOptional().
					Build()))
			if err != nil {
				return fmt.Errorf("inserting campaign target: %w", err)
			}
		}
		return nil
	})
}

// Targets returns a campaign's targeted lists and segments.
func (r *YDBCampaigns) Targets(ctx context.Context, tenantID, campaignID string) ([]domain.Target, error) {
	var targets []domain.Target
	err := r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		rs, err := tx.QueryResultSet(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $campaign_id AS Utf8;
SELECT list_id, segment_query FROM campaign_lists
WHERE tenant_id = $tenant_id AND campaign_id = $campaign_id;`,
			query.WithParameters(ydb.ParamsBuilder().
				Param("$tenant_id").Text(tenantID).
				Param("$campaign_id").Text(campaignID).
				Build()))
		if err != nil {
			return fmt.Errorf("listing campaign targets: %w", err)
		}
		// Collected into a local and assigned at the end (V6).
		var local []domain.Target
		for row, err := range rs.Rows(ctx) {
			if err != nil {
				return fmt.Errorf("listing campaign targets: %w", err)
			}
			var listID, segment *string
			if err := row.Scan(&listID, &segment); err != nil {
				return fmt.Errorf("scanning campaign target: %w", err)
			}
			local = append(local, domain.Target{ListID: deref(listID), SegmentQuery: jsonBytes(segment)})
		}
		targets = local
		return nil
	})
	if err != nil {
		return nil, err
	}
	return targets, nil
}

// ydbCampaignFromRow maps a single-row read to the campaign, translating "no
// row" into domain.ErrCampaignNotFound.
func ydbCampaignFromRow(row query.Row, err error) (*domain.Campaign, error) {
	if errors.Is(err, query.ErrNoRows) {
		return nil, domain.ErrCampaignNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("loading campaign: %w", err)
	}
	return scanYDBCampaign(row)
}

// scanYDBCampaign reads one row in ydbCampaignColumns order.
func scanYDBCampaign(row query.Row) (*domain.Campaign, error) {
	var id, tenantID, name, subject, bodyHTML, bodyText, fromName, fromLocalPart, status string
	var sendingDomainID, templateID, bodyDoc, theme *string
	var maxSendErrors, sentCount, failedCount, recipientCount int64
	var createdAt, updatedAt time.Time
	var startedAt, finishedAt, archivedAt *time.Time
	var archiveVisible bool
	if err := row.Scan(&id, &tenantID, &name, &subject, &bodyHTML, &bodyText, &fromName,
		&fromLocalPart, &sendingDomainID, &templateID, &status, &maxSendErrors,
		&sentCount, &failedCount, &recipientCount, &bodyDoc, &theme,
		&createdAt, &updatedAt, &startedAt, &finishedAt, &archiveVisible, &archivedAt); err != nil {
		return nil, fmt.Errorf("scanning campaign: %w", err)
	}
	return domain.HydrateCampaign(id, tenantID, name, subject, bodyHTML, bodyText, fromName,
		fromLocalPart, deref(sendingDomainID), deref(templateID), domain.CampaignStatus(status),
		int(maxSendErrors), int(sentCount), int(failedCount), int(recipientCount),
		jsonBytes(bodyDoc), jsonBytes(theme),
		createdAt, updatedAt, startedAt, finishedAt, archiveVisible, archivedAt), nil
}
```

**`internal/campaign/adapters/transactional_ydb.go`** (create):

```go
package adapters

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	ydb "github.com/ydb-platform/ydb-go-sdk/v3"
	"github.com/ydb-platform/ydb-go-sdk/v3/query"

	"github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
)

// YDBTransactionalMessages is the YDB-backed implementation of
// domain.TransactionalMessageRepository, for the serverless runtime.
type YDBTransactionalMessages struct {
	driver *ydb.Driver
}

var _ domain.TransactionalMessageRepository = (*YDBTransactionalMessages)(nil)

// NewYDBTransactionalMessages builds a YDBTransactionalMessages repository
// over the driver.
func NewYDBTransactionalMessages(d *ydb.Driver) *YDBTransactionalMessages {
	return &YDBTransactionalMessages{driver: d}
}

// Record persists one transactional send under a fresh UUID. A provider
// message id that is already recorded is an error, as on Postgres.
func (r *YDBTransactionalMessages) Record(ctx context.Context, tenantID, templateID,
	providerMessageID, recipientEmail string) error {

	err := r.driver.Query().Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DECLARE $template_id AS Optional<Utf8>;
DECLARE $provider_message_id AS Utf8;
DECLARE $recipient_email AS Utf8;
INSERT INTO transactional_messages
    (tenant_id, id, template_id, provider_message_id, recipient_email, sent_at)
VALUES ($tenant_id, $id, $template_id, $provider_message_id, $recipient_email,
    CurrentUtcTimestamp());`,
		query.WithParameters(ydb.ParamsBuilder().
			Param("$tenant_id").Text(tenantID).
			Param("$id").Text(uuid.NewString()).
			Param("$template_id").BeginOptional().Text(nullableString(templateID)).EndOptional().
			Param("$provider_message_id").Text(providerMessageID).
			Param("$recipient_email").Text(recipientEmail).
			Build()))
	if err != nil {
		return fmt.Errorf("recording transactional message: %w", err)
	}
	return nil
}
```

- [ ] **Step 3: Delete cascade from a list into its campaign targets.**

**`internal/audience/adapters/lists_ydb.go`** (modify):

```diff
--- a/internal/audience/adapters/lists_ydb.go
+++ b/internal/audience/adapters/lists_ydb.go
@@ -125,8 +125,8 @@
 }
 
 // Delete removes the list and, in the same transaction, the rows Postgres
-// deletes by foreign-key cascade: the list's memberships and the iam per-list
-// role assignments on it. It returns domain.ErrListNotFound when absent.
+// deletes by foreign-key cascade: the list's memberships, the iam per-list
+// role assignments on it and the campaign targets that name it. It returns domain.ErrListNotFound when absent.
 func (r *YDBLists) Delete(ctx context.Context, tenantID, id string) error {
 	return r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
 		_, err := tx.QueryRow(ctx, `DECLARE $tenant_id AS Utf8;
@@ -160,6 +160,17 @@
 WHERE tenant_id = $tenant_id AND list_id = $id;`, ydbTenantAndID(tenantID, id)); err != nil {
 			return fmt.Errorf("deleting list roles: %w", err)
 		}
+		// campaign_lists belongs to campaign. Postgres cascades into it
+		// through its foreign key to lists; a dangling target would make
+		// every send of that campaign resolve a list that no longer exists.
+		if err := tx.Exec(ctx, `DECLARE $tenant_id AS Utf8;
+DECLARE $id AS Utf8;
+DELETE FROM campaign_lists ON
+SELECT tenant_id, campaign_id, id
+FROM campaign_lists VIEW idx_campaign_lists_list
+WHERE tenant_id = $tenant_id AND list_id = $id;`, ydbTenantAndID(tenantID, id)); err != nil {
+			return fmt.Errorf("deleting campaign targets: %w", err)
+		}
 		return nil
 	})
 }
```

- [ ] **Step 4: Build the harness.** Run `git mv` for the four tests first, then write `helpers_test.go` whole. `eachBackend` runs a body once per backend, as parallel subtests. Tasks 2 and 3 add the recipient and tracking fields. `withRecipientProviderMessageID` stays until Task 2, because the pg recipients test still uses it.

```bash
git mv internal/campaign/adapters/templates_pg_test.go internal/campaign/adapters/templates_test.go
git mv internal/campaign/adapters/campaigns_pg_test.go internal/campaign/adapters/campaigns_test.go
git mv internal/campaign/adapters/archive_pg_test.go internal/campaign/adapters/archive_test.go
git mv internal/campaign/adapters/transactional_pg_test.go internal/campaign/adapters/transactional_test.go
```

**`internal/campaign/adapters/helpers_test.go`** (replace the whole file):

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

	"github.com/nikolaymatrosov/nvelope/internal/campaign/adapters"
	"github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
	"github.com/nikolaymatrosov/nvelope/internal/platform/tenantdb"
)

// backend is one storage implementation of the campaign repositories under
// contract test. Every contract test body runs once per backend, so the
// Postgres and YDB adapters are held to the same behaviour.
type backend struct {
	name          string
	templates     func(t *testing.T) domain.TemplateRepository
	campaigns     func(t *testing.T) domain.CampaignRepository
	transactional func(t *testing.T) domain.TransactionalMessageRepository
	// The seeds below create rows owned by other contexts. On Postgres they
	// are real rows, because the campaign tables carry foreign keys to them.
	// YDB has no foreign keys and no campaign query reads these tables, so a
	// bare UUID stands in.
	newTenant        func(t *testing.T) string
	newSendingDomain func(t *testing.T, tenantID string) string
	newList          func(t *testing.T, tenantID string) string
	newSubscriber    func(t *testing.T, tenantID, email string) string
	// The probes read what no port exposes.
	transactionalCount func(t *testing.T, tenantID string) int
}

var backends = []backend{
	{
		name: "pg",
		templates: func(t *testing.T) domain.TemplateRepository {
			return adapters.NewTemplates(dbtest.AppPool(t))
		},
		campaigns: func(t *testing.T) domain.CampaignRepository {
			return adapters.NewCampaigns(dbtest.AppPool(t))
		},
		transactional: func(t *testing.T) domain.TransactionalMessageRepository {
			return adapters.NewTransactionalMessages(dbtest.AppPool(t))
		},
		newTenant: func(t *testing.T) string { return seedTenant(t, dbtest.AppPool(t)) },
		newSendingDomain: func(t *testing.T, tenantID string) string {
			return seedSendingDomain(t, dbtest.AppPool(t), tenantID, "mail."+dbtest.RandString()+".com")
		},
		newList: func(t *testing.T, tenantID string) string {
			return seedList(t, dbtest.AppPool(t), tenantID, "L-"+dbtest.RandString())
		},
		newSubscriber: func(t *testing.T, tenantID, email string) string {
			return seedSubscriber(t, dbtest.AppPool(t), tenantID, email)
		},
		transactionalCount: func(t *testing.T, tenantID string) int {
			return pgCount(t, tenantID, "SELECT count(*) FROM transactional_messages", nil)
		},
	},
	{
		name: "ydb",
		templates: func(t *testing.T) domain.TemplateRepository {
			return adapters.NewYDBTemplates(dbtest.YDBMigrated(t))
		},
		campaigns: func(t *testing.T) domain.CampaignRepository {
			return adapters.NewYDBCampaigns(dbtest.YDBMigrated(t))
		},
		transactional: func(t *testing.T) domain.TransactionalMessageRepository {
			return adapters.NewYDBTransactionalMessages(dbtest.YDBMigrated(t))
		},
		newTenant:        func(*testing.T) string { return uuid.NewString() },
		newSendingDomain: func(*testing.T, string) string { return uuid.NewString() },
		newList:          func(*testing.T, string) string { return uuid.NewString() },
		newSubscriber:    func(*testing.T, string, string) string { return uuid.NewString() },
		transactionalCount: func(t *testing.T, tenantID string) int {
			return ydbCount(t, `DECLARE $tenant_id AS Utf8;
SELECT COUNT(*) FROM transactional_messages WHERE tenant_id = $tenant_id;`, tenantID, "", "")
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

// pgCount reads one count inside the tenant-bound transaction. A nil arg runs
// the statement with no argument.
func pgCount(t *testing.T, tenantID, sql string, arg any) int {
	t.Helper()
	var n int
	require.NoError(t, tenantdb.WithTenant(context.Background(), dbtest.AppPool(t), tenantID,
		func(ctx context.Context, tx pgx.Tx) error {
			if arg == nil {
				return tx.QueryRow(ctx, sql).Scan(&n)
			}
			return tx.QueryRow(ctx, sql, arg).Scan(&n)
		}))
	return n
}

// ydbParams binds $tenant_id and, when name is set, one more Utf8 parameter.
func ydbParams(tenantID, name, value string) query.ExecuteOption {
	b := ydb.ParamsBuilder().Param("$tenant_id").Text(tenantID)
	if name != "" {
		b = b.Param(name).Text(value)
	}
	return query.WithParameters(b.Build())
}

// ydbCount reads one COUNT(*).
func ydbCount(t *testing.T, q, tenantID, name, value string) int {
	t.Helper()
	row, err := dbtest.YDBMigrated(t).Query().QueryRow(context.Background(), q, ydbParams(tenantID, name, value))
	require.NoError(t, err)
	var n uint64
	require.NoError(t, row.Scan(&n))
	return int(n)
}

// seedTenant inserts a tenant row directly and returns its id.
func seedTenant(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var id string
	require.NoError(t, pool.QueryRow(context.Background(),
		`INSERT INTO tenants (name, slug, status) VALUES ('Workspace', $1, 'active') RETURNING id`,
		"cmp-"+dbtest.RandString()).Scan(&id))
	return id
}

// seedSubscriber inserts a subscriber inside the tenant-bound transaction and
// returns its id.
func seedSubscriber(t *testing.T, pool *pgxpool.Pool, tenantID, email string) string {
	t.Helper()
	var id string
	require.NoError(t, tenantdb.WithTenant(context.Background(), pool, tenantID,
		func(ctx context.Context, tx pgx.Tx) error {
			return tx.QueryRow(ctx,
				`INSERT INTO subscribers (tenant_id, email, name) VALUES ($1, $2, 'Sub')
				 RETURNING id`, tenantID, email).Scan(&id)
		}))
	return id
}

// seedSendingDomain inserts a verified sending domain and returns its id.
func seedSendingDomain(t *testing.T, pool *pgxpool.Pool, tenantID, name string) string {
	t.Helper()
	var id string
	require.NoError(t, tenantdb.WithTenant(context.Background(), pool, tenantID,
		func(ctx context.Context, tx pgx.Tx) error {
			return tx.QueryRow(ctx,
				`INSERT INTO sending_domains (tenant_id, domain, status, verified_at)
				 VALUES ($1, $2, 'verified', now()) RETURNING id`, tenantID, name).Scan(&id)
		}))
	return id
}

// seedList inserts a list inside the tenant-bound transaction and returns its
// id.
func seedList(t *testing.T, pool *pgxpool.Pool, tenantID, name string) string {
	t.Helper()
	var id string
	require.NoError(t, tenantdb.WithTenant(context.Background(), pool, tenantID,
		func(ctx context.Context, tx pgx.Tx) error {
			return tx.QueryRow(ctx,
				`INSERT INTO lists (tenant_id, name) VALUES ($1, $2) RETURNING id`,
				tenantID, name).Scan(&id)
		}))
	return id
}

// newCampaign builds a draft campaign for the given tenant.
func newCampaign(t *testing.T, tenantID, name, sendingDomainID string) *domain.Campaign {
	t.Helper()
	c, err := domain.NewCampaign(tenantID, name, "Subject", `<p>hi</p>`, "Hi",
		"Acme", "news", sendingDomainID, "", 100)
	require.NoError(t, err)
	return c
}

func newTemplate(t *testing.T, tenantID, name string) *domain.Template {
	t.Helper()
	tpl, err := domain.NewTemplate(tenantID, name, domain.KindCampaign, "Subject", "<p>b</p>", "")
	require.NoError(t, err)
	return tpl
}

// withRecipientProviderMessageID reads a campaign recipient's persisted
// provider_message_id inside the tenant-bound transaction.
func withRecipientProviderMessageID(t *testing.T, pool *pgxpool.Pool, tenantID, recipientID string,
	dst *string) error {

	t.Helper()
	return tenantdb.WithTenant(context.Background(), pool, tenantID,
		func(ctx context.Context, tx pgx.Tx) error {
			return tx.QueryRow(ctx,
				"SELECT coalesce(provider_message_id, '') FROM campaign_recipients WHERE id = $1",
				recipientID).Scan(dst)
		})
}
```

- [ ] **Step 5: Write the contract tests.** Each body runs on pg and ydb. `TestTemplatesDeleteUnlinksCampaigns` pins the `ON DELETE SET NULL` the YDB adapter writes by hand. `TestCampaignsSaveAndReadTargets` pins the list-uniqueness collision and that a failed `SaveTargets` leaves the old targets. Replace each file whole.

**`internal/campaign/adapters/templates_test.go`** (create):

```go
package adapters_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
)

func TestTemplatesAddGetAll(t *testing.T) {
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.templates(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		id, err := repo.Add(ctx, tenantID, newTemplate(t, tenantID, "Welcome"))
		require.NoError(t, err)

		got, err := repo.Get(ctx, tenantID, id)
		require.NoError(t, err)
		require.Equal(t, "Welcome", got.Name())
		require.Equal(t, domain.KindCampaign, got.Kind())

		all, total, err := repo.All(ctx, tenantID, domain.DefaultPage)
		require.NoError(t, err)
		require.Equal(t, 1, total)
		require.Len(t, all, 1)
	})
}

func TestTemplatesDuplicateNameConflicts(t *testing.T) {
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.templates(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		_, err := repo.Add(ctx, tenantID, newTemplate(t, tenantID, "Dup"))
		require.NoError(t, err)
		_, err = repo.Add(ctx, tenantID, newTemplate(t, tenantID, "Dup"))
		require.ErrorIs(t, err, domain.ErrTemplateNameTaken)

		other, err := repo.Add(ctx, tenantID, newTemplate(t, tenantID, "Other"))
		require.NoError(t, err)
		err = repo.Update(ctx, tenantID, other, func(tpl *domain.Template) (*domain.Template, error) {
			return tpl, tpl.Recompose("Dup", "Subject", "<p>b</p>", "")
		})
		require.ErrorIs(t, err, domain.ErrTemplateNameTaken, "renaming onto a taken name conflicts too")
	})
}

func TestTemplatesUpdate(t *testing.T) {
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.templates(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		id, err := repo.Add(ctx, tenantID, newTemplate(t, tenantID, "Old"))
		require.NoError(t, err)
		require.NoError(t, repo.Update(ctx, tenantID, id,
			func(tpl *domain.Template) (*domain.Template, error) {
				return tpl, tpl.Recompose("New", "New subject", "<p>new</p>", "")
			}))

		got, err := repo.Get(ctx, tenantID, id)
		require.NoError(t, err)
		require.Equal(t, "New", got.Name())
		require.Equal(t, "New subject", got.Subject())

		err = repo.Update(ctx, tenantID, "00000000-0000-4000-8000-000000000000",
			func(tpl *domain.Template) (*domain.Template, error) { return tpl, nil })
		require.ErrorIs(t, err, domain.ErrTemplateNotFound)
	})
}

func TestTemplatesGetMissing(t *testing.T) {
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.templates(t)
		tenantID := b.newTenant(t)

		_, err := repo.Get(context.Background(), tenantID, "not-a-uuid")
		require.ErrorIs(t, err, domain.ErrTemplateNotFound)
	})
}

func TestTemplatesDelete(t *testing.T) {
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.templates(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		id, err := repo.Add(ctx, tenantID, newTemplate(t, tenantID, "Gone"))
		require.NoError(t, err)
		require.NoError(t, repo.Delete(ctx, tenantID, id))
		_, err = repo.Get(ctx, tenantID, id)
		require.ErrorIs(t, err, domain.ErrTemplateNotFound)
		require.ErrorIs(t, repo.Delete(ctx, tenantID, id), domain.ErrTemplateNotFound)
		require.ErrorIs(t, repo.Delete(ctx, tenantID, "not-a-uuid"), domain.ErrTemplateNotFound)
	})
}

// TestTemplatesDeleteUnlinksCampaigns pins the Postgres ON DELETE SET NULL on
// campaigns.template_id: a campaign keeps its content and loses the link.
func TestTemplatesDeleteUnlinksCampaigns(t *testing.T) {
	eachBackend(t, func(t *testing.T, b backend) {
		templates, campaigns := b.templates(t), b.campaigns(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		domainID := b.newSendingDomain(t, tenantID)

		tplID, err := templates.Add(ctx, tenantID, newTemplate(t, tenantID, "T"))
		require.NoError(t, err)
		c, err := domain.NewCampaign(tenantID, "From template", "Subject", "<p>hi</p>", "Hi",
			"Acme", "news", domainID, tplID, 100)
		require.NoError(t, err)
		campaignID, err := campaigns.Add(ctx, tenantID, c)
		require.NoError(t, err)
		other, err := campaigns.Add(ctx, tenantID, newCampaign(t, tenantID, "Plain", domainID))
		require.NoError(t, err)

		got, err := campaigns.Get(ctx, tenantID, campaignID)
		require.NoError(t, err)
		require.Equal(t, tplID, got.TemplateID())

		require.NoError(t, templates.Delete(ctx, tenantID, tplID))

		got, err = campaigns.Get(ctx, tenantID, campaignID)
		require.NoError(t, err)
		require.Empty(t, got.TemplateID(), "the campaign is unlinked")
		require.Equal(t, "From template", got.Name(), "and keeps its content")
		_, err = campaigns.Get(ctx, tenantID, other)
		require.NoError(t, err)
	})
}

func TestTemplatesCrossTenantIsolation(t *testing.T) {
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.templates(t)
		ctx := context.Background()
		tenantA, tenantB := b.newTenant(t), b.newTenant(t)

		idA, err := repo.Add(ctx, tenantA, newTemplate(t, tenantA, "Welcome"))
		require.NoError(t, err)

		_, err = repo.Get(ctx, tenantB, idA)
		require.ErrorIs(t, err, domain.ErrTemplateNotFound)
		require.ErrorIs(t, repo.Delete(ctx, tenantB, idA), domain.ErrTemplateNotFound)

		all, _, err := repo.All(ctx, tenantB, domain.DefaultPage)
		require.NoError(t, err)
		require.Empty(t, all)

		// The same name is free in another tenant.
		_, err = repo.Add(ctx, tenantB, newTemplate(t, tenantB, "Welcome"))
		require.NoError(t, err)
	})
}
```

**`internal/campaign/adapters/campaigns_test.go`** (create):

```go
package adapters_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
)

func TestCampaignsAddGetUpdate(t *testing.T) {
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.campaigns(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		domainID := b.newSendingDomain(t, tenantID)

		id, err := repo.Add(ctx, tenantID, newCampaign(t, tenantID, "Spring", domainID))
		require.NoError(t, err)

		got, err := repo.Get(ctx, tenantID, id)
		require.NoError(t, err)
		require.Equal(t, "Spring", got.Name())
		require.Equal(t, domain.CampaignDraft, got.Status())
		require.Equal(t, domainID, got.SendingDomainID())
		require.Nil(t, got.StartedAt())

		require.NoError(t, repo.Update(ctx, tenantID, id,
			func(c *domain.Campaign) (*domain.Campaign, error) {
				return c, c.Start(time.Now())
			}))
		got, err = repo.Get(ctx, tenantID, id)
		require.NoError(t, err)
		require.Equal(t, domain.CampaignRunning, got.Status())
		require.NotNil(t, got.StartedAt())

		err = repo.Update(ctx, tenantID, "00000000-0000-4000-8000-000000000000",
			func(c *domain.Campaign) (*domain.Campaign, error) { return c, nil })
		require.ErrorIs(t, err, domain.ErrCampaignNotFound)
		_, err = repo.Get(ctx, tenantID, "not-a-uuid")
		require.ErrorIs(t, err, domain.ErrCampaignNotFound)
	})
}

func TestCampaignsAllNewestFirst(t *testing.T) {
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.campaigns(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		domainID := b.newSendingDomain(t, tenantID)

		first, err := repo.Add(ctx, tenantID, newCampaign(t, tenantID, "First", domainID))
		require.NoError(t, err)
		time.Sleep(20 * time.Millisecond)
		second, err := repo.Add(ctx, tenantID, newCampaign(t, tenantID, "Second", domainID))
		require.NoError(t, err)

		all, total, err := repo.All(ctx, tenantID, domain.DefaultPage)
		require.NoError(t, err)
		require.Equal(t, 2, total)
		require.Equal(t, []string{second, first}, []string{all[0].ID(), all[1].ID()})

		page, total, err := repo.All(ctx, tenantID, domain.Page{Offset: 1, Limit: 1})
		require.NoError(t, err)
		require.Equal(t, 2, total)
		require.Len(t, page, 1)
		require.Equal(t, first, page[0].ID())
	})
}

func TestCampaignsSaveAndReadTargets(t *testing.T) {
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.campaigns(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		domainID := b.newSendingDomain(t, tenantID)

		id, err := repo.Add(ctx, tenantID, newCampaign(t, tenantID, "Spring", domainID))
		require.NoError(t, err)

		listID := b.newList(t, tenantID)
		segment := json.RawMessage(`{"op":"and","conditions":[]}`)
		require.NoError(t, repo.SaveTargets(ctx, tenantID, id, []domain.Target{
			{ListID: listID},
			{SegmentQuery: segment},
		}))

		targets, err := repo.Targets(ctx, tenantID, id)
		require.NoError(t, err)
		require.Len(t, targets, 2)
		var lists, segments int
		for _, tg := range targets {
			if tg.ListID != "" {
				require.Equal(t, listID, tg.ListID)
				lists++
			} else {
				require.JSONEq(t, string(segment), string(tg.SegmentQuery))
				segments++
			}
		}
		require.Equal(t, 1, lists)
		require.Equal(t, 1, segments)

		// SaveTargets replaces — a second save does not accumulate.
		require.NoError(t, repo.SaveTargets(ctx, tenantID, id, []domain.Target{{ListID: listID}}))
		targets, err = repo.Targets(ctx, tenantID, id)
		require.NoError(t, err)
		require.Len(t, targets, 1)

		// Targeting a list twice collides on UNIQUE (campaign_id, list_id),
		// and the failed save leaves the previous targets in place.
		require.Error(t, repo.SaveTargets(ctx, tenantID, id,
			[]domain.Target{{ListID: listID}, {ListID: listID}}))
		targets, err = repo.Targets(ctx, tenantID, id)
		require.NoError(t, err)
		require.Len(t, targets, 1)

		// Two segments are not a collision: only a list is unique.
		require.NoError(t, repo.SaveTargets(ctx, tenantID, id,
			[]domain.Target{{SegmentQuery: segment}, {SegmentQuery: segment}}))
		targets, err = repo.Targets(ctx, tenantID, id)
		require.NoError(t, err)
		require.Len(t, targets, 2)
	})
}

func TestCampaignsCrossTenantIsolation(t *testing.T) {
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.campaigns(t)
		ctx := context.Background()
		tenantA, tenantB := b.newTenant(t), b.newTenant(t)
		domainA := b.newSendingDomain(t, tenantA)

		idA, err := repo.Add(ctx, tenantA, newCampaign(t, tenantA, "Spring", domainA))
		require.NoError(t, err)

		_, err = repo.Get(ctx, tenantB, idA)
		require.ErrorIs(t, err, domain.ErrCampaignNotFound)

		err = repo.Update(ctx, tenantB, idA, func(c *domain.Campaign) (*domain.Campaign, error) {
			return c, nil
		})
		require.ErrorIs(t, err, domain.ErrCampaignNotFound)

		all, _, err := repo.All(ctx, tenantB, domain.DefaultPage)
		require.NoError(t, err)
		require.Empty(t, all)
	})
}
```

**`internal/campaign/adapters/archive_test.go`** (create):

```go
package adapters_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
)

func TestCampaignsArchivedListsNewestFirst(t *testing.T) {
	eachBackend(t, func(t *testing.T, b backend) {
		ctx := context.Background()
		tenantID := b.newTenant(t)
		domainID := b.newSendingDomain(t, tenantID)
		repo := b.campaigns(t)

		// publish starts a campaign and puts it in the archive at the given
		// time, or leaves it out of the archive when visible is false.
		publish := func(name string, visible bool, at time.Time) string {
			id, err := repo.Add(ctx, tenantID, newCampaign(t, tenantID, name, domainID))
			require.NoError(t, err)
			require.NoError(t, repo.Update(ctx, tenantID, id,
				func(c *domain.Campaign) (*domain.Campaign, error) {
					if err := c.Start(time.Now()); err != nil {
						return nil, err
					}
					return c, c.SetArchiveVisible(visible, at)
				}))
			return id
		}
		older := publish("Old", true, time.Now().Add(-48*time.Hour))
		newer := publish("New", true, time.Now().Add(-1*time.Hour))
		hidden := publish("Hidden", false, time.Time{})

		list, total, err := repo.Archived(ctx, tenantID, domain.Page{Limit: 50})
		require.NoError(t, err)
		require.Equal(t, 2, total, "hidden campaign is not counted in the archive")
		require.Len(t, list, 2)
		require.Equal(t, newer, list[0].ID(), "archive is ordered newest-first")
		require.Equal(t, older, list[1].ID())
		require.NotNil(t, list[0].ArchivedAt())

		// A hidden campaign is still retrievable by id, but the archive query
		// layer filters it out.
		got, err := repo.Get(ctx, tenantID, hidden)
		require.NoError(t, err)
		require.False(t, got.ArchiveVisible())
	})
}
```

**`internal/campaign/adapters/transactional_test.go`** (create):

```go
package adapters_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
)

func TestTransactionalMessagesRecordRoundTrips(t *testing.T) {
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.transactional(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		pm := "pm-" + dbtest.RandString()

		require.NoError(t, repo.Record(ctx, tenantID, "", pm, "user@acme.com"))
		require.Equal(t, 1, b.transactionalCount(t, tenantID))

		// A provider message id is recorded once, across tenants.
		require.Error(t, repo.Record(ctx, tenantID, "", pm, "again@acme.com"))
		require.Equal(t, 1, b.transactionalCount(t, tenantID))
	})
}

func TestTransactionalMessagesCrossTenantIsolation(t *testing.T) {
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.transactional(t)
		ctx := context.Background()
		tenantA, tenantB := b.newTenant(t), b.newTenant(t)

		require.NoError(t, repo.Record(ctx, tenantA, "", "pm-"+dbtest.RandString(), "a@acme.com"))

		require.Equal(t, 1, b.transactionalCount(t, tenantA))
		require.Equal(t, 0, b.transactionalCount(t, tenantB),
			"tenant B sees none of tenant A's transactional messages")
	})
}
```

- [ ] **Step 6: Test the list cascade, on YDB only.** No audience port can see `campaign_lists`, so the test inserts rows with raw YQL, as `TestYDBListsDeleteRemovesListRoles` does for iam. Append to `lists_ydb_test.go`:

```diff
--- a/internal/audience/adapters/lists_ydb_test.go
+++ b/internal/audience/adapters/lists_ydb_test.go
@@ -56,3 +56,55 @@
 	}
 	require.Equal(t, []string{kept}, left, "only the deleted list's role assignments go")
 }
+
+// TestYDBListsDeleteRemovesCampaignTargets covers the stand-in for the
+// Postgres cascade from lists into campaign_lists, which no audience port can
+// observe. Segment targets and targets of other lists stay.
+func TestYDBListsDeleteRemovesCampaignTargets(t *testing.T) {
+	t.Parallel()
+	d := dbtest.YDBMigrated(t)
+	ctx := context.Background()
+	tenantID, campaignID := uuid.NewString(), uuid.NewString()
+	lists := adapters.NewYDBLists(d)
+	gone := addList(t, lists, tenantID, "Gone")
+	kept := addList(t, lists, tenantID, "Kept")
+	targets := map[string]*string{uuid.NewString(): &gone, uuid.NewString(): &kept, uuid.NewString(): nil}
+	for id, listID := range targets {
+		require.NoError(t, d.Query().Exec(ctx, `DECLARE $tenant_id AS Utf8;
+DECLARE $campaign_id AS Utf8;
+DECLARE $id AS Utf8;
+DECLARE $list_id AS Optional<Utf8>;
+INSERT INTO campaign_lists (tenant_id, campaign_id, id, list_id, segment_query)
+VALUES ($tenant_id, $campaign_id, $id, $list_id, NULL);`,
+			query.WithParameters(ydb.ParamsBuilder().
+				Param("$tenant_id").Text(tenantID).
+				Param("$campaign_id").Text(campaignID).
+				Param("$id").Text(id).
+				Param("$list_id").BeginOptional().Text(listID).EndOptional().
+				Build())))
+	}
+
+	require.NoError(t, lists.Delete(ctx, tenantID, gone))
+
+	rs, err := d.Query().QueryResultSet(ctx, `DECLARE $tenant_id AS Utf8;
+SELECT id FROM campaign_lists WHERE tenant_id = $tenant_id;`,
+		query.WithParameters(ydb.ParamsBuilder().Param("$tenant_id").Text(tenantID).Build()))
+	require.NoError(t, err)
+	defer func() { _ = rs.Close(ctx) }()
+	var left int
+	for row, err := range rs.Rows(ctx) {
+		require.NoError(t, err)
+		var id string
+		require.NoError(t, row.Scan(&id))
+		require.NotEqual(t, gone, deref(targets[id]), "the deleted list's target is gone")
+		left++
+	}
+	require.Equal(t, 2, left, "only the deleted list's target goes")
+}
+
+func deref(s *string) string {
+	if s == nil {
+		return ""
+	}
+	return *s
+}
```

- [ ] **Step 7: Verify.**

```bash
gofmt -l internal
go vet ./internal/campaign/... ./internal/audience/...
golangci-lint run ./internal/campaign/... ./internal/audience/...
export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock" && go test ./internal/campaign/adapters ./internal/audience/adapters ./internal/db -count=1 -timeout 8m
```

Expected: `gofmt` prints nothing, `0 issues.`, and every package `ok`.

- [ ] **Step 8: Sabotage check.** Temporarily make each change below, confirm the named test fails, then restore the file (`git diff` must show no sabotage left):
  - in `lists_ydb.go`, remove the `tx.Exec` that deletes `campaign_lists`: `TestYDBListsDeleteRemovesCampaignTargets` fails;
  - in `templates_ydb.go`, change `SET template_id = NULL` to `SET template_id = template_id`: `TestTemplatesDeleteUnlinksCampaigns/ydb` fails.

- [ ] **Step 9: Commit.**

```bash
git add internal/db/ydbmigrations/000008_campaign.up.sql internal/campaign/adapters internal/audience/adapters
git commit -m "feat(campaign): templates, campaigns and transactional messages on YDB"
```

---
### Task 2: Recipients on YDB, and the subscriber-delete cascade

**Files:**
- Create: `internal/campaign/adapters/recipients_ydb.go`
- Modify: `internal/campaign/adapters/recipients_pg.go`, `helpers_test.go`
- Rename and rewrite: `recipients_pg_test.go` → `recipients_test.go`
- Modify: `internal/audience/adapters/subscribers_ydb.go`
- Create: `internal/audience/adapters/subscribers_ydb_test.go`

**Interfaces:**
- Consumes: `domain.RecipientRepository`, `domain.NewRecipient`, `domain.HydrateRecipient` (unchanged); Task 1's `ydb_common.go`, `nullableString`.
- Produces:
  ```go
  func NewYDBRecipients(d *ydb.Driver) *YDBRecipients
  ```

- [ ] **Step 1: Create `YDBRecipients`.** `BulkInsert` dedupes in Go, then writes chunks of 1000 (`ydbRecipientChunk`). `Pending` reads the id-ordered window and filters pending rows in Go, in chunks of 1000. The marks are `UPDATE ... ON` through the id index.

**`internal/campaign/adapters/recipients_ydb.go`** (create):

```go
package adapters

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	ydb "github.com/ydb-platform/ydb-go-sdk/v3"
	"github.com/ydb-platform/ydb-go-sdk/v3/query"
	"github.com/ydb-platform/ydb-go-sdk/v3/table/types"

	"github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
)

// ydbRecipientChunk is how many recipients one transaction writes or one
// query reads. V5 found no documented transaction-size limit, and a query
// returns at most 1000 rows.
const ydbRecipientChunk = 1000

// YDBRecipients is the YDB-backed implementation of domain.RecipientRepository,
// for the serverless runtime. A campaign's recipients are one primary-key
// range, in id order. A recipient is found from its id alone through
// idx_campaign_recipients_id.
type YDBRecipients struct {
	driver *ydb.Driver
}

var _ domain.RecipientRepository = (*YDBRecipients)(nil)

// NewYDBRecipients builds a YDBRecipients repository over the driver.
func NewYDBRecipients(d *ydb.Driver) *YDBRecipients {
	return &YDBRecipients{driver: d}
}

// BulkInsert persists one recipient row per unique (campaign_id, email) and
// returns how many rows were newly inserted. An INSERT that hits an existing
// key aborts the whole transaction on YDB (spec "Plan B7 planning spike"), so
// each chunk of up to 1000 recipients reads the emails the campaign already
// has and inserts only the others, in one transaction. A chunk is idempotent
// on retry, so the SDK may re-run it. A first attempt that committed without
// the response arriving makes the retry count its rows as already present, so
// the returned count can then fall short.
func (r *YDBRecipients) BulkInsert(ctx context.Context, tenantID, campaignID string,
	rs []*domain.Recipient) (int, error) {

	// The first occurrence of an email wins, as ON CONFLICT DO NOTHING keeps
	// the first row. YDB rejects a batch that repeats a key.
	seen := make(map[string]bool, len(rs))
	unique := make([]*domain.Recipient, 0, len(rs))
	for _, rec := range rs {
		if !seen[rec.Email()] {
			seen[rec.Email()] = true
			unique = append(unique, rec)
		}
	}

	inserted := 0
	for start := 0; start < len(unique); start += ydbRecipientChunk {
		chunk := unique[start:min(start+ydbRecipientChunk, len(unique))]
		n, err := r.insertChunk(ctx, tenantID, campaignID, chunk)
		if err != nil {
			return inserted, err
		}
		inserted += n
	}
	return inserted, nil
}

// insertChunk inserts the recipients of chunk whose email the campaign does
// not have yet, and returns how many it inserted.
func (r *YDBRecipients) insertChunk(ctx context.Context, tenantID, campaignID string,
	chunk []*domain.Recipient) (int, error) {

	// Ids are fixed before the transaction, so a retry writes the same rows.
	ids := make([]string, len(chunk))
	emails := make([]types.Value, len(chunk))
	for i, rec := range chunk {
		ids[i] = uuid.NewString()
		emails[i] = types.TextValue(rec.Email())
	}
	idOpt := query.WithParameters(ydb.ParamsBuilder().
		Param("$tenant_id").Text(tenantID).
		Param("$campaign_id").Text(campaignID).
		Param("$emails").Any(types.ListValue(emails...)).
		Build())

	var inserted int
	err := r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		rs, err := tx.QueryResultSet(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $campaign_id AS Utf8;
DECLARE $emails AS List<Utf8>;
SELECT email FROM campaign_recipients VIEW idx_campaign_recipients_email
WHERE tenant_id = $tenant_id AND campaign_id = $campaign_id AND email IN $emails;`, idOpt)
		if err != nil {
			return fmt.Errorf("reading existing recipients: %w", err)
		}
		existing := map[string]bool{}
		for row, err := range rs.Rows(ctx) {
			if err != nil {
				return fmt.Errorf("reading existing recipients: %w", err)
			}
			var email string
			if err := row.Scan(&email); err != nil {
				return fmt.Errorf("scanning existing recipient: %w", err)
			}
			existing[email] = true
		}

		var rows []types.Value
		for i, rec := range chunk {
			if existing[rec.Email()] {
				continue
			}
			rows = append(rows, types.StructValue(
				types.StructFieldValue("id", types.TextValue(ids[i])),
				types.StructFieldValue("subscriber_id", types.TextValue(rec.SubscriberID())),
				types.StructFieldValue("email", types.TextValue(rec.Email()))))
		}
		if len(rows) > 0 {
			err = tx.Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $campaign_id AS Utf8;
DECLARE $rows AS List<Struct<id: Utf8, subscriber_id: Utf8, email: Utf8>>;
INSERT INTO campaign_recipients
    (tenant_id, campaign_id, id, subscriber_id, email, status, failure_reason)
SELECT $tenant_id, $campaign_id, id, subscriber_id, email, "pending"u, ""u
FROM AS_TABLE($rows);`, query.WithParameters(ydb.ParamsBuilder().
				Param("$tenant_id").Text(tenantID).
				Param("$campaign_id").Text(campaignID).
				Param("$rows").Any(types.ListValue(rows...)).
				Build()))
			if err != nil {
				return fmt.Errorf("inserting recipients: %w", err)
			}
		}
		// Assigned at the end: a retry must not add to a failed run's count.
		inserted = len(rows)
		return nil
	})
	if err != nil {
		return 0, err
	}
	return inserted, nil
}

// Pending returns the still-unsent recipients within the stable [offset,
// offset+limit) window of the campaign's id-ordered recipient set, as the
// Postgres adapter does: the window covers every recipient, so a batch's slice
// never shifts as rows are sent.
// ponytail: OFFSET reads every skipped row, so batch N of a campaign reads N
// windows of rows. Fine for the recipient counts of one organisation; store a
// per-campaign ordinal to seek on if a campaign outgrows it (spec "Ceilings").
func (r *YDBRecipients) Pending(ctx context.Context, tenantID, campaignID string,
	offset, limit int) ([]*domain.Recipient, error) {

	var out []*domain.Recipient
	err := r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		// Collected into a local and assigned at the end (V6).
		var local []*domain.Recipient
		for done := 0; done < limit; done += ydbRecipientChunk {
			rs, err := tx.QueryResultSet(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $campaign_id AS Utf8;
DECLARE $limit AS Uint64;
DECLARE $offset AS Uint64;
SELECT id, subscriber_id, email, status, failure_reason, sent_at
FROM campaign_recipients
WHERE tenant_id = $tenant_id AND campaign_id = $campaign_id
ORDER BY id
LIMIT $limit OFFSET $offset;`, query.WithParameters(ydb.ParamsBuilder().
				Param("$tenant_id").Text(tenantID).
				Param("$campaign_id").Text(campaignID).
				Param("$limit").Uint64(uint64(min(ydbRecipientChunk, limit-done))).
				Param("$offset").Uint64(uint64(offset+done)).
				Build()))
			if err != nil {
				return fmt.Errorf("listing pending recipients: %w", err)
			}
			read := 0
			for row, err := range rs.Rows(ctx) {
				if err != nil {
					return fmt.Errorf("listing pending recipients: %w", err)
				}
				read++
				var id, subscriberID, email, status, failureReason string
				var sentAt *time.Time
				if err := row.Scan(&id, &subscriberID, &email, &status, &failureReason, &sentAt); err != nil {
					return fmt.Errorf("scanning recipient: %w", err)
				}
				if domain.RecipientStatus(status) != domain.RecipientPending {
					continue
				}
				local = append(local, domain.HydrateRecipient(id, tenantID, campaignID, subscriberID,
					email, domain.RecipientPending, failureReason, sentAt))
			}
			if read < min(ydbRecipientChunk, limit-done) {
				break // the campaign has no rows past this window
			}
		}
		out = local
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// MarkSent records a successful send for one recipient, persisting the
// provider message id so a later bounce or complaint can be attributed to it.
// A recipient id that does not exist records nothing, as the Postgres UPDATE
// matches no row.
func (r *YDBRecipients) MarkSent(ctx context.Context, tenantID, recipientID, providerMessageID string,
	at time.Time) error {

	err := r.driver.Query().Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DECLARE $sent_at AS Timestamp;
DECLARE $provider_message_id AS Optional<Utf8>;
UPDATE campaign_recipients ON
SELECT tenant_id, campaign_id, id, "sent"u AS status, ""u AS failure_reason,
    $sent_at AS sent_at, $provider_message_id AS provider_message_id
FROM campaign_recipients VIEW idx_campaign_recipients_id
WHERE tenant_id = $tenant_id AND id = $id;`, query.WithParameters(ydb.ParamsBuilder().
		Param("$tenant_id").Text(tenantID).
		Param("$id").Text(recipientID).
		Param("$sent_at").Timestamp(at.UTC()).
		Param("$provider_message_id").BeginOptional().Text(nullableString(providerMessageID)).EndOptional().
		Build()))
	if err != nil {
		return fmt.Errorf("marking recipient sent: %w", err)
	}
	return nil
}

// MarkSkipped records a recipient skipped by the pre-send suppression check,
// storing the suppression reason in failure_reason.
func (r *YDBRecipients) MarkSkipped(ctx context.Context, tenantID, recipientID, reason string) error {
	return r.markStatus(ctx, tenantID, recipientID, domain.RecipientSkipped, reason)
}

// MarkFailed records a failed send for one recipient.
func (r *YDBRecipients) MarkFailed(ctx context.Context, tenantID, recipientID, reason string) error {
	return r.markStatus(ctx, tenantID, recipientID, domain.RecipientFailed, reason)
}

func (r *YDBRecipients) markStatus(ctx context.Context, tenantID, recipientID string,
	status domain.RecipientStatus, reason string) error {

	err := r.driver.Query().Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DECLARE $status AS Utf8;
DECLARE $failure_reason AS Utf8;
UPDATE campaign_recipients ON
SELECT tenant_id, campaign_id, id, $status AS status, $failure_reason AS failure_reason
FROM campaign_recipients VIEW idx_campaign_recipients_id
WHERE tenant_id = $tenant_id AND id = $id;`, query.WithParameters(ydb.ParamsBuilder().
		Param("$tenant_id").Text(tenantID).
		Param("$id").Text(recipientID).
		Param("$status").Text(string(status)).
		Param("$failure_reason").Text(reason).
		Build()))
	if err != nil {
		return fmt.Errorf("marking recipient %s: %w", status, err)
	}
	return nil
}

// Counts returns the campaign's sent, failed, and pending recipient counts.
// ponytail: it reads every recipient row of the campaign on each call.
func (r *YDBRecipients) Counts(ctx context.Context, tenantID, campaignID string) (sent, failed, pending int, err error) {
	err = r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		rs, err := tx.QueryResultSet(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $campaign_id AS Utf8;
SELECT status, COUNT(*) FROM campaign_recipients
WHERE tenant_id = $tenant_id AND campaign_id = $campaign_id
GROUP BY status;`, query.WithParameters(ydb.ParamsBuilder().
			Param("$tenant_id").Text(tenantID).
			Param("$campaign_id").Text(campaignID).
			Build()))
		if err != nil {
			return fmt.Errorf("counting recipients: %w", err)
		}
		// Assigned at the end of a complete read: a retry starts from zero.
		var s, f, p int
		for row, err := range rs.Rows(ctx) {
			if err != nil {
				return fmt.Errorf("counting recipients: %w", err)
			}
			var status string
			var n uint64
			if err := row.Scan(&status, &n); err != nil {
				return fmt.Errorf("scanning recipient count: %w", err)
			}
			switch domain.RecipientStatus(status) {
			case domain.RecipientSent:
				s = int(n)
			case domain.RecipientFailed:
				f = int(n)
			case domain.RecipientPending:
				p = int(n)
			}
		}
		sent, failed, pending = s, f, p
		return nil
	})
	return sent, failed, pending, err
}
```

- [ ] **Step 2: Reset the Postgres `BulkInsert` counter inside its closure** (the V6 table lists it).

```diff
--- a/internal/campaign/adapters/recipients_pg.go
+++ b/internal/campaign/adapters/recipients_pg.go
@@ -32,6 +32,7 @@
 
 	var inserted int
 	err := tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
+		inserted = 0 // a retried run starts from zero (V6)
 		for _, rec := range rs {
 			tag, err := tx.Exec(ctx,
 				`INSERT INTO campaign_recipients (tenant_id, campaign_id, subscriber_id, email)
```
- [ ] **Step 3: Extend the harness** with the `recipients` field, the `providerMessageID` probe and `ydbScalar`/`pgScalar`, and drop `withRecipientProviderMessageID`.

```diff
--- a/internal/campaign/adapters/helpers_test.go
+++ b/internal/campaign/adapters/helpers_test.go
@@ -24,6 +24,7 @@
 	name          string
 	templates     func(t *testing.T) domain.TemplateRepository
 	campaigns     func(t *testing.T) domain.CampaignRepository
+	recipients    func(t *testing.T) domain.RecipientRepository
 	transactional func(t *testing.T) domain.TransactionalMessageRepository
 	// The seeds below create rows owned by other contexts. On Postgres they
 	// are real rows, because the campaign tables carry foreign keys to them.
@@ -34,6 +35,7 @@
 	newList          func(t *testing.T, tenantID string) string
 	newSubscriber    func(t *testing.T, tenantID, email string) string
 	// The probes read what no port exposes.
+	providerMessageID  func(t *testing.T, tenantID, recipientID string) string
 	transactionalCount func(t *testing.T, tenantID string) int
 }
 
@@ -46,6 +48,9 @@
 		campaigns: func(t *testing.T) domain.CampaignRepository {
 			return adapters.NewCampaigns(dbtest.AppPool(t))
 		},
+		recipients: func(t *testing.T) domain.RecipientRepository {
+			return adapters.NewRecipients(dbtest.AppPool(t))
+		},
 		transactional: func(t *testing.T) domain.TransactionalMessageRepository {
 			return adapters.NewTransactionalMessages(dbtest.AppPool(t))
 		},
@@ -59,6 +64,9 @@
 		newSubscriber: func(t *testing.T, tenantID, email string) string {
 			return seedSubscriber(t, dbtest.AppPool(t), tenantID, email)
 		},
+		providerMessageID: func(t *testing.T, tenantID, recipientID string) string {
+			return pgScalar(t, tenantID, "SELECT coalesce(provider_message_id, '') FROM campaign_recipients WHERE id = $1", recipientID)
+		},
 		transactionalCount: func(t *testing.T, tenantID string) int {
 			return pgCount(t, tenantID, "SELECT count(*) FROM transactional_messages", nil)
 		},
@@ -71,6 +79,9 @@
 		campaigns: func(t *testing.T) domain.CampaignRepository {
 			return adapters.NewYDBCampaigns(dbtest.YDBMigrated(t))
 		},
+		recipients: func(t *testing.T) domain.RecipientRepository {
+			return adapters.NewYDBRecipients(dbtest.YDBMigrated(t))
+		},
 		transactional: func(t *testing.T) domain.TransactionalMessageRepository {
 			return adapters.NewYDBTransactionalMessages(dbtest.YDBMigrated(t))
 		},
@@ -78,6 +89,12 @@
 		newSendingDomain: func(*testing.T, string) string { return uuid.NewString() },
 		newList:          func(*testing.T, string) string { return uuid.NewString() },
 		newSubscriber:    func(*testing.T, string, string) string { return uuid.NewString() },
+		providerMessageID: func(t *testing.T, tenantID, recipientID string) string {
+			return ydbScalar(t, `DECLARE $tenant_id AS Utf8;
+DECLARE $id AS Utf8;
+SELECT provider_message_id FROM campaign_recipients VIEW idx_campaign_recipients_id
+WHERE tenant_id = $tenant_id AND id = $id;`, tenantID, "$id", recipientID)
+		},
 		transactionalCount: func(t *testing.T, tenantID string) int {
 			return ydbCount(t, `DECLARE $tenant_id AS Utf8;
 SELECT COUNT(*) FROM transactional_messages WHERE tenant_id = $tenant_id;`, tenantID, "", "")
@@ -96,6 +113,17 @@
 	}
 }
 
+// pgScalar reads one text value inside the tenant-bound transaction.
+func pgScalar(t *testing.T, tenantID, sql, arg string) string {
+	t.Helper()
+	var out string
+	require.NoError(t, tenantdb.WithTenant(context.Background(), dbtest.AppPool(t), tenantID,
+		func(ctx context.Context, tx pgx.Tx) error {
+			return tx.QueryRow(ctx, sql, arg).Scan(&out)
+		}))
+	return out
+}
+
 // pgCount reads one count inside the tenant-bound transaction. A nil arg runs
 // the statement with no argument.
 func pgCount(t *testing.T, tenantID, sql string, arg any) int {
@@ -120,6 +148,19 @@
 	return query.WithParameters(b.Build())
 }
 
+// ydbScalar reads one nullable text value, "" for NULL.
+func ydbScalar(t *testing.T, q, tenantID, name, value string) string {
+	t.Helper()
+	row, err := dbtest.YDBMigrated(t).Query().QueryRow(context.Background(), q, ydbParams(tenantID, name, value))
+	require.NoError(t, err)
+	var out *string
+	require.NoError(t, row.Scan(&out))
+	if out == nil {
+		return ""
+	}
+	return *out
+}
+
 // ydbCount reads one COUNT(*).
 func ydbCount(t *testing.T, q, tenantID, name, value string) int {
 	t.Helper()
@@ -196,17 +237,3 @@
 	require.NoError(t, err)
 	return tpl
 }
-
-// withRecipientProviderMessageID reads a campaign recipient's persisted
-// provider_message_id inside the tenant-bound transaction.
-func withRecipientProviderMessageID(t *testing.T, pool *pgxpool.Pool, tenantID, recipientID string,
-	dst *string) error {
-
-	t.Helper()
-	return tenantdb.WithTenant(context.Background(), pool, tenantID,
-		func(ctx context.Context, tx pgx.Tx) error {
-			return tx.QueryRow(ctx,
-				"SELECT coalesce(provider_message_id, '') FROM campaign_recipients WHERE id = $1",
-				recipientID).Scan(dst)
-		})
-}
```
- [ ] **Step 4: Rewrite the recipients tests.** Run `git mv internal/campaign/adapters/recipients_pg_test.go internal/campaign/adapters/recipients_test.go`, then replace the file whole. The new tests: a 2300-row insert across chunk boundaries with repeats, six concurrent `BulkInsert`s of one set, and the batch window staying stable while the first batch is sent.

```go
package adapters_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
)

// newCampaignID persists a draft campaign of the tenant and returns its id.
func newCampaignID(t *testing.T, b backend, tenantID string) string {
	t.Helper()
	id, err := b.campaigns(t).Add(context.Background(), tenantID,
		newCampaign(t, tenantID, "C-"+fmt.Sprint(time.Now().UnixNano()), b.newSendingDomain(t, tenantID)))
	require.NoError(t, err)
	return id
}

func TestRecipientsBulkInsertDedupesByEmail(t *testing.T) {
	eachBackend(t, func(t *testing.T, b backend) {
		recipients := b.recipients(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		campaignID := newCampaignID(t, b, tenantID)

		sub1 := b.newSubscriber(t, tenantID, "a@acme.com")
		sub2 := b.newSubscriber(t, tenantID, "b@acme.com")

		rs := []*domain.Recipient{
			domain.NewRecipient(tenantID, campaignID, sub1, "a@acme.com"),
			domain.NewRecipient(tenantID, campaignID, sub2, "b@acme.com"),
			domain.NewRecipient(tenantID, campaignID, sub1, "a@acme.com"), // duplicate email
		}
		inserted, err := recipients.BulkInsert(ctx, tenantID, campaignID, rs)
		require.NoError(t, err)
		require.Equal(t, 2, inserted, "the UNIQUE (campaign_id, email) constraint dedupes")

		// A second BulkInsert of the same set inserts nothing.
		inserted, err = recipients.BulkInsert(ctx, tenantID, campaignID, rs)
		require.NoError(t, err)
		require.Equal(t, 0, inserted)

		// A set that overlaps the persisted one inserts only the new rows.
		sub3 := b.newSubscriber(t, tenantID, "c@acme.com")
		inserted, err = recipients.BulkInsert(ctx, tenantID, campaignID, append(rs,
			domain.NewRecipient(tenantID, campaignID, sub3, "c@acme.com")))
		require.NoError(t, err)
		require.Equal(t, 1, inserted)

		// The same email is free in another campaign.
		other := newCampaignID(t, b, tenantID)
		inserted, err = recipients.BulkInsert(ctx, tenantID, other, rs[:1])
		require.NoError(t, err)
		require.Equal(t, 1, inserted)
	})
}

// TestRecipientsBulkInsertSpansChunks writes more recipients than one YDB
// transaction chunk holds, with duplicates across the chunk boundary.
func TestRecipientsBulkInsertSpansChunks(t *testing.T) {
	eachBackend(t, func(t *testing.T, b backend) {
		recipients := b.recipients(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		campaignID := newCampaignID(t, b, tenantID)

		const n = 2300
		subID := b.newSubscriber(t, tenantID, "s@acme.com")
		var rs []*domain.Recipient
		for i := 0; i < n; i++ {
			rs = append(rs, domain.NewRecipient(tenantID, campaignID, subID, fmt.Sprintf("u%d@acme.com", i)))
		}
		rs = append(rs, rs[0], rs[1500]) // repeats of a first-chunk and a later row

		inserted, err := recipients.BulkInsert(ctx, tenantID, campaignID, rs)
		require.NoError(t, err)
		require.Equal(t, n, inserted)

		sent, failed, pending, err := recipients.Counts(ctx, tenantID, campaignID)
		require.NoError(t, err)
		require.Equal(t, []int{0, 0, n}, []int{sent, failed, pending})

		// A window past 1000 rows comes back whole.
		window, err := recipients.Pending(ctx, tenantID, campaignID, 100, 1500)
		require.NoError(t, err)
		require.Len(t, window, 1500)
		for i := 1; i < len(window); i++ {
			require.Less(t, window[i-1].ID(), window[i].ID(), "the window is in id order")
		}
	})
}

// TestRecipientsBulkInsertIsRaceFree runs overlapping BulkInserts of one
// campaign at once: every email lands once and none of the calls fails.
func TestRecipientsBulkInsertIsRaceFree(t *testing.T) {
	eachBackend(t, func(t *testing.T, b backend) {
		recipients := b.recipients(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		campaignID := newCampaignID(t, b, tenantID)
		subID := b.newSubscriber(t, tenantID, "s@acme.com")

		const workers, emails = 6, 40
		var rs []*domain.Recipient
		for i := 0; i < emails; i++ {
			rs = append(rs, domain.NewRecipient(tenantID, campaignID, subID, fmt.Sprintf("r%d@acme.com", i)))
		}
		var wg sync.WaitGroup
		errs := make([]error, workers)
		counts := make([]int, workers)
		for w := 0; w < workers; w++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				counts[w], errs[w] = recipients.BulkInsert(ctx, tenantID, campaignID, rs)
			}()
		}
		wg.Wait()

		total := 0
		for w := range errs {
			require.NoError(t, errs[w])
			total += counts[w]
		}
		require.Equal(t, emails, total, "each email is inserted by exactly one call")
		_, _, pending, err := recipients.Counts(ctx, tenantID, campaignID)
		require.NoError(t, err)
		require.Equal(t, emails, pending)
	})
}

func TestRecipientsPendingAndProgress(t *testing.T) {
	eachBackend(t, func(t *testing.T, b backend) {
		recipients := b.recipients(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		campaignID := newCampaignID(t, b, tenantID)

		sub1 := b.newSubscriber(t, tenantID, "a@acme.com")
		sub2 := b.newSubscriber(t, tenantID, "b@acme.com")
		_, err := recipients.BulkInsert(ctx, tenantID, campaignID, []*domain.Recipient{
			domain.NewRecipient(tenantID, campaignID, sub1, "a@acme.com"),
			domain.NewRecipient(tenantID, campaignID, sub2, "b@acme.com"),
		})
		require.NoError(t, err)

		pending, err := recipients.Pending(ctx, tenantID, campaignID, 0, 10)
		require.NoError(t, err)
		require.Len(t, pending, 2)
		require.Equal(t, domain.RecipientPending, pending[0].Status())
		require.Equal(t, campaignID, pending[0].CampaignID())

		require.NoError(t, recipients.MarkSent(ctx, tenantID, pending[0].ID(), "pm-abc", time.Now()))
		require.NoError(t, recipients.MarkFailed(ctx, tenantID, pending[1].ID(), "bounced"))

		sent, failed, stillPending, err := recipients.Counts(ctx, tenantID, campaignID)
		require.NoError(t, err)
		require.Equal(t, 1, sent)
		require.Equal(t, 1, failed)
		require.Equal(t, 0, stillPending)

		remaining, err := recipients.Pending(ctx, tenantID, campaignID, 0, 10)
		require.NoError(t, err)
		require.Empty(t, remaining, "sent and failed recipients are not re-selected")

		// The provider message id is persisted for the sent recipient so a
		// later bounce/complaint can be attributed to it.
		require.Equal(t, "pm-abc", b.providerMessageID(t, tenantID, pending[0].ID()))
		require.Empty(t, b.providerMessageID(t, tenantID, pending[1].ID()))

		// A recipient id that does not exist records nothing.
		missing := "00000000-0000-4000-8000-000000000000"
		require.NoError(t, recipients.MarkSent(ctx, tenantID, missing, "pm-x", time.Now()))
		require.NoError(t, recipients.MarkFailed(ctx, tenantID, missing, "x"))
	})
}

// TestRecipientsPendingWindowIsStable pins the batch contract: the window is
// taken over every recipient, so sending the first batch does not shift the
// second one.
func TestRecipientsPendingWindowIsStable(t *testing.T) {
	eachBackend(t, func(t *testing.T, b backend) {
		recipients := b.recipients(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		campaignID := newCampaignID(t, b, tenantID)
		subID := b.newSubscriber(t, tenantID, "s@acme.com")

		var rs []*domain.Recipient
		for i := 0; i < 6; i++ {
			rs = append(rs, domain.NewRecipient(tenantID, campaignID, subID, fmt.Sprintf("w%d@acme.com", i)))
		}
		_, err := recipients.BulkInsert(ctx, tenantID, campaignID, rs)
		require.NoError(t, err)

		all, err := recipients.Pending(ctx, tenantID, campaignID, 0, 6)
		require.NoError(t, err)
		require.Len(t, all, 6)
		secondBefore, err := recipients.Pending(ctx, tenantID, campaignID, 3, 3)
		require.NoError(t, err)
		require.Equal(t, ids(all[3:]), ids(secondBefore))

		for _, r := range all[:3] {
			require.NoError(t, recipients.MarkSent(ctx, tenantID, r.ID(), "pm", time.Now()))
		}
		secondAfter, err := recipients.Pending(ctx, tenantID, campaignID, 3, 3)
		require.NoError(t, err)
		require.Equal(t, ids(all[3:]), ids(secondAfter), "sending the first batch leaves the second where it was")
		firstAfter, err := recipients.Pending(ctx, tenantID, campaignID, 0, 3)
		require.NoError(t, err)
		require.Empty(t, firstAfter, "a redelivered first batch finds nothing left to send")
	})
}

func ids(rs []*domain.Recipient) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.ID()
	}
	return out
}

func TestRecipientsMarkSkipped(t *testing.T) {
	eachBackend(t, func(t *testing.T, b backend) {
		recipients := b.recipients(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		campaignID := newCampaignID(t, b, tenantID)
		sub := b.newSubscriber(t, tenantID, "skip@acme.com")
		_, err := recipients.BulkInsert(ctx, tenantID, campaignID, []*domain.Recipient{
			domain.NewRecipient(tenantID, campaignID, sub, "skip@acme.com"),
		})
		require.NoError(t, err)
		pending, err := recipients.Pending(ctx, tenantID, campaignID, 0, 10)
		require.NoError(t, err)
		require.Len(t, pending, 1)

		require.NoError(t, recipients.MarkSkipped(ctx, tenantID, pending[0].ID(), "hard_bounce"))

		// A skipped recipient is neither sent, failed, nor pending.
		sent, failed, stillPending, err := recipients.Counts(ctx, tenantID, campaignID)
		require.NoError(t, err)
		require.Equal(t, 0, sent+failed+stillPending)

		remaining, err := recipients.Pending(ctx, tenantID, campaignID, 0, 10)
		require.NoError(t, err)
		require.Empty(t, remaining, "a skipped recipient is not re-selected for sending")
	})
}

func TestRecipientsCrossTenantIsolation(t *testing.T) {
	eachBackend(t, func(t *testing.T, b backend) {
		recipients := b.recipients(t)
		ctx := context.Background()
		tenantA, tenantB := b.newTenant(t), b.newTenant(t)
		campaignA := newCampaignID(t, b, tenantA)

		subA := b.newSubscriber(t, tenantA, "a@acme.com")
		_, err := recipients.BulkInsert(ctx, tenantA, campaignA, []*domain.Recipient{
			domain.NewRecipient(tenantA, campaignA, subA, "a@acme.com"),
		})
		require.NoError(t, err)
		own, err := recipients.Pending(ctx, tenantA, campaignA, 0, 10)
		require.NoError(t, err)
		require.Len(t, own, 1)

		// Tenant B sees none of tenant A's recipients, and cannot mark them.
		pending, err := recipients.Pending(ctx, tenantB, campaignA, 0, 10)
		require.NoError(t, err)
		require.Empty(t, pending)

		require.NoError(t, recipients.MarkFailed(ctx, tenantB, own[0].ID(), "not yours"))
		sent, failed, stillPending, err := recipients.Counts(ctx, tenantB, campaignA)
		require.NoError(t, err)
		require.Equal(t, 0, sent+failed+stillPending)
		_, failed, stillPending, err = recipients.Counts(ctx, tenantA, campaignA)
		require.NoError(t, err)
		require.Equal(t, []int{0, 1}, []int{failed, stillPending}, "tenant A's recipient is untouched")
	})
}
```
- [ ] **Step 5: Cascade from a subscriber into its campaign recipients.**

**`internal/audience/adapters/subscribers_ydb.go`** (modify):

```diff
--- a/internal/audience/adapters/subscribers_ydb.go
+++ b/internal/audience/adapters/subscribers_ydb.go
@@ -189,7 +189,7 @@
 }
 
 // Delete removes the subscriber and, in the same transaction, its
-// memberships, as the Postgres cascade does. It returns
+// memberships and campaign recipients, as the Postgres cascade does. It returns
 // domain.ErrSubscriberNotFound when absent.
 func (r *YDBSubscribers) Delete(ctx context.Context, tenantID, id string) error {
 	return r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
@@ -210,6 +210,19 @@
 WHERE tenant_id = $tenant_id AND subscriber_id = $id;`, ydbTenantAndID(tenantID, id)); err != nil {
 			return fmt.Errorf("deleting subscriber memberships: %w", err)
 		}
+		// campaign_recipients belongs to campaign. Postgres cascades into it
+		// through its foreign key to subscribers, so an erased subscriber
+		// keeps no address in a campaign and is not mailed by a running one.
+		// The click and view events keep their recipient id: nothing reads
+		// them.
+		if err := tx.Exec(ctx, `DECLARE $tenant_id AS Utf8;
+DECLARE $id AS Utf8;
+DELETE FROM campaign_recipients ON
+SELECT tenant_id, campaign_id, id
+FROM campaign_recipients VIEW idx_campaign_recipients_subscriber
+WHERE tenant_id = $tenant_id AND subscriber_id = $id;`, ydbTenantAndID(tenantID, id)); err != nil {
+			return fmt.Errorf("deleting subscriber campaign recipients: %w", err)
+		}
 		return nil
 	})
 }
```

**`internal/audience/adapters/subscribers_ydb_test.go`** (create):

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

// TestYDBSubscribersDeleteRemovesCampaignRecipients covers the stand-in for
// the Postgres cascade from subscribers into campaign_recipients, which no
// audience port can observe. Other subscribers' recipient rows stay.
func TestYDBSubscribersDeleteRemovesCampaignRecipients(t *testing.T) {
	t.Parallel()
	d := dbtest.YDBMigrated(t)
	ctx := context.Background()
	tenantID, campaignID := uuid.NewString(), uuid.NewString()
	subscribers := adapters.NewYDBSubscribers(d)
	gone := addSubscriber(t, subscribers, tenantID, "gone@acme.com")
	kept := addSubscriber(t, subscribers, tenantID, "kept@acme.com")
	for _, r := range []struct{ subscriberID, email string }{
		{gone, "gone@acme.com"}, {kept, "kept@acme.com"},
	} {
		require.NoError(t, d.Query().Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $campaign_id AS Utf8;
DECLARE $id AS Utf8;
DECLARE $subscriber_id AS Utf8;
DECLARE $email AS Utf8;
INSERT INTO campaign_recipients
    (tenant_id, campaign_id, id, subscriber_id, email, status, failure_reason)
VALUES ($tenant_id, $campaign_id, $id, $subscriber_id, $email, "pending"u, ""u);`,
			query.WithParameters(ydb.ParamsBuilder().
				Param("$tenant_id").Text(tenantID).
				Param("$campaign_id").Text(campaignID).
				Param("$id").Text(uuid.NewString()).
				Param("$subscriber_id").Text(r.subscriberID).
				Param("$email").Text(r.email).
				Build())))
	}

	require.NoError(t, subscribers.Delete(ctx, tenantID, gone))

	rs, err := d.Query().QueryResultSet(ctx, `DECLARE $tenant_id AS Utf8;
SELECT subscriber_id FROM campaign_recipients WHERE tenant_id = $tenant_id;`,
		query.WithParameters(ydb.ParamsBuilder().Param("$tenant_id").Text(tenantID).Build()))
	require.NoError(t, err)
	defer func() { _ = rs.Close(ctx) }()
	var left []string
	for row, err := range rs.Rows(ctx) {
		require.NoError(t, err)
		var subscriberID string
		require.NoError(t, row.Scan(&subscriberID))
		left = append(left, subscriberID)
	}
	require.Equal(t, []string{kept}, left, "only the deleted subscriber's recipient rows go")
}
```

- [ ] **Step 6: Verify.**

```bash
gofmt -l internal
go vet ./internal/campaign/... ./internal/audience/...
golangci-lint run ./internal/campaign/... ./internal/audience/...
export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock" && go test ./internal/campaign/adapters ./internal/audience/adapters -count=1 -timeout 8m
export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock" && go test ./internal/campaign/adapters -count=3 -timeout 8m -run 'RaceFree|SpansChunks|Window'
```

- [ ] **Step 7: Sabotage check.** Restore the file after each:
  - in `recipients_ydb.go`, change `if existing[rec.Email()] {` to `if false {`: `TestRecipientsBulkInsertDedupesByEmail/ydb` fails;
  - in `subscribers_ydb.go`, remove the `tx.Exec` that deletes `campaign_recipients`: `TestYDBSubscribersDeleteRemovesCampaignRecipients` fails.

- [ ] **Step 8: Commit.**

```bash
git add internal/campaign/adapters internal/audience/adapters
git commit -m "feat(campaign): recipients on YDB, and the subscriber-delete cascade"
```

---
### Task 3: Tracking on YDB

**Files:**
- Create: `internal/campaign/adapters/tracking_ydb.go`
- Modify: `internal/campaign/adapters/helpers_test.go`
- Rename and rewrite: `tracking_pg_test.go` → `tracking_test.go`

**Interfaces:**
- Consumes: `domain.TrackingRepository`, `domain.ErrLinkNotFound`, `domain.ErrCampaignNotFound` (unchanged); Task 2's recipients (the tests seed through `b.recipients`).
- Produces:
  ```go
  func NewYDBTracking(d *ydb.Driver) *YDBTracking
  ```

- [ ] **Step 1: Create `YDBTracking`.** `UpsertLinks` reads the campaign's URLs and inserts the missing ones. `RecordClick` and `RecordView` read the recipient through the id index and record nothing unless it belongs to the campaign. The resolvers read the global id indexes.

**`internal/campaign/adapters/tracking_ydb.go`** (create):

```go
package adapters

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	ydb "github.com/ydb-platform/ydb-go-sdk/v3"
	"github.com/ydb-platform/ydb-go-sdk/v3/query"
	"github.com/ydb-platform/ydb-go-sdk/v3/table/types"

	"github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
)

// YDBTracking is the YDB-backed implementation of domain.TrackingRepository,
// for the serverless runtime. The two resolvers read through the global
// indexes idx_links_id and idx_campaigns_id, which stand in for the Postgres
// SECURITY DEFINER functions.
type YDBTracking struct {
	driver *ydb.Driver
}

var _ domain.TrackingRepository = (*YDBTracking)(nil)

// NewYDBTracking builds a YDBTracking repository over the driver.
func NewYDBTracking(d *ydb.Driver) *YDBTracking {
	return &YDBTracking{driver: d}
}

// UpsertLinks ensures one links row per distinct URL and returns the map from
// URL to its links-row id. It reads the URLs the campaign already has and
// inserts the others, in one transaction: an INSERT onto an existing key would
// abort it. A concurrent upsert of the same URL aborts one of the two
// transactions, and the SDK retries it.
func (r *YDBTracking) UpsertLinks(ctx context.Context, tenantID, campaignID string,
	urls []string) (map[string]string, error) {

	seen := make(map[string]bool, len(urls))
	var distinct []string
	for _, u := range urls {
		if !seen[u] {
			seen[u] = true
			distinct = append(distinct, u)
		}
	}
	out := make(map[string]string, len(distinct))
	if len(distinct) == 0 {
		return out, nil
	}

	// Ids are fixed before the transaction, so a retry writes the same rows.
	fresh := make(map[string]string, len(distinct))
	values := make([]types.Value, len(distinct))
	for i, u := range distinct {
		fresh[u] = uuid.NewString()
		values[i] = types.TextValue(u)
	}

	var result map[string]string
	err := r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		rs, err := tx.QueryResultSet(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $campaign_id AS Utf8;
DECLARE $urls AS List<Utf8>;
SELECT url, id FROM links VIEW idx_links_url
WHERE tenant_id = $tenant_id AND campaign_id = $campaign_id AND url IN $urls;`,
			query.WithParameters(ydb.ParamsBuilder().
				Param("$tenant_id").Text(tenantID).
				Param("$campaign_id").Text(campaignID).
				Param("$urls").Any(types.ListValue(values...)).
				Build()))
		if err != nil {
			return fmt.Errorf("reading links: %w", err)
		}
		// Built in a local and assigned at the end (V6).
		local := make(map[string]string, len(distinct))
		for row, err := range rs.Rows(ctx) {
			if err != nil {
				return fmt.Errorf("reading links: %w", err)
			}
			var url, id string
			if err := row.Scan(&url, &id); err != nil {
				return fmt.Errorf("scanning link: %w", err)
			}
			local[url] = id
		}
		var rows []types.Value
		for _, u := range distinct {
			if _, ok := local[u]; ok {
				continue
			}
			local[u] = fresh[u]
			rows = append(rows, types.StructValue(
				types.StructFieldValue("id", types.TextValue(fresh[u])),
				types.StructFieldValue("url", types.TextValue(u))))
		}
		if len(rows) > 0 {
			err = tx.Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $campaign_id AS Utf8;
DECLARE $rows AS List<Struct<id: Utf8, url: Utf8>>;
INSERT INTO links (tenant_id, id, campaign_id, url, created_at)
SELECT $tenant_id, id, $campaign_id, url, CurrentUtcTimestamp()
FROM AS_TABLE($rows);`, query.WithParameters(ydb.ParamsBuilder().
				Param("$tenant_id").Text(tenantID).
				Param("$campaign_id").Text(campaignID).
				Param("$rows").Any(types.ListValue(rows...)).
				Build()))
			if err != nil {
				return fmt.Errorf("inserting links: %w", err)
			}
		}
		result = local
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// ownRecipient reports whether the recipient exists in the tenant and belongs
// to the campaign. A missing or foreign id is false, never an error.
func ownRecipient(ctx context.Context, tx query.TxActor, tenantID, campaignID, recipientID string) (bool, error) {
	row, err := tx.QueryRow(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
SELECT campaign_id FROM campaign_recipients VIEW idx_campaign_recipients_id
WHERE tenant_id = $tenant_id AND id = $id;`, ydbTenantAndID(tenantID, recipientID))
	if errors.Is(err, query.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("loading recipient: %w", err)
	}
	var owner string
	if err := row.Scan(&owner); err != nil {
		return false, fmt.Errorf("scanning recipient: %w", err)
	}
	return owner == campaignID, nil
}

// RecordClick returns the link's original URL and records the click when the
// recipient belongs to the link's campaign. A missing, malformed or foreign
// recipient id records nothing. An unknown link is domain.ErrLinkNotFound with
// an empty URL. When the insert itself fails, the URL is returned alongside
// the error so the caller can still redirect.
func (r *YDBTracking) RecordClick(ctx context.Context, tenantID, linkID, recipientID string) (string, error) {
	clickID := uuid.NewString()
	var url string
	err := r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		row, err := tx.QueryRow(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
SELECT campaign_id, url FROM links WHERE tenant_id = $tenant_id AND id = $id;`,
			ydbTenantAndID(tenantID, linkID))
		if errors.Is(err, query.ErrNoRows) {
			return domain.ErrLinkNotFound
		}
		if err != nil {
			return fmt.Errorf("loading link: %w", err)
		}
		var campaignID, linkURL string
		if err := row.Scan(&campaignID, &linkURL); err != nil {
			return fmt.Errorf("scanning link: %w", err)
		}
		url = linkURL // the last run wins
		own, err := ownRecipient(ctx, tx, tenantID, campaignID, recipientID)
		if err != nil || !own {
			return err
		}
		err = tx.Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DECLARE $link_id AS Utf8;
DECLARE $campaign_id AS Utf8;
DECLARE $recipient_id AS Utf8;
INSERT INTO link_clicks (tenant_id, id, link_id, campaign_id, recipient_id, clicked_at)
VALUES ($tenant_id, $id, $link_id, $campaign_id, $recipient_id, CurrentUtcTimestamp());`,
			query.WithParameters(ydb.ParamsBuilder().
				Param("$tenant_id").Text(tenantID).
				Param("$id").Text(clickID).
				Param("$link_id").Text(linkID).
				Param("$campaign_id").Text(campaignID).
				Param("$recipient_id").Text(recipientID).
				Build()))
		if err != nil {
			return fmt.Errorf("recording click: %w", err)
		}
		return nil
	})
	return url, err
}

// RecordView records an open when the recipient belongs to the campaign. A
// missing or foreign recipient id records nothing.
func (r *YDBTracking) RecordView(ctx context.Context, tenantID, campaignID, recipientID string) error {
	viewID := uuid.NewString()
	return r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		own, err := ownRecipient(ctx, tx, tenantID, campaignID, recipientID)
		if err != nil || !own {
			return err
		}
		err = tx.Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DECLARE $campaign_id AS Utf8;
DECLARE $recipient_id AS Utf8;
INSERT INTO campaign_views (tenant_id, id, campaign_id, recipient_id, viewed_at)
VALUES ($tenant_id, $id, $campaign_id, $recipient_id, CurrentUtcTimestamp());`,
			query.WithParameters(ydb.ParamsBuilder().
				Param("$tenant_id").Text(tenantID).
				Param("$id").Text(viewID).
				Param("$campaign_id").Text(campaignID).
				Param("$recipient_id").Text(recipientID).
				Build()))
		if err != nil {
			return fmt.Errorf("recording view: %w", err)
		}
		return nil
	})
}

// ResolveTenantForLink returns the tenant that owns a links row.
func (r *YDBTracking) ResolveTenantForLink(ctx context.Context, linkID string) (string, error) {
	return r.resolveTenant(ctx, `DECLARE $id AS Utf8;
SELECT tenant_id FROM links VIEW idx_links_id WHERE id = $id;`, linkID, domain.ErrLinkNotFound)
}

// ResolveTenantForCampaign returns the tenant that owns a campaign.
func (r *YDBTracking) ResolveTenantForCampaign(ctx context.Context, campaignID string) (string, error) {
	return r.resolveTenant(ctx, `DECLARE $id AS Utf8;
SELECT tenant_id FROM campaigns VIEW idx_campaigns_id WHERE id = $id;`, campaignID, domain.ErrCampaignNotFound)
}

func (r *YDBTracking) resolveTenant(ctx context.Context, q, id string, notFound error) (string, error) {
	row, err := r.driver.Query().QueryRow(ctx, q,
		query.WithParameters(ydb.ParamsBuilder().Param("$id").Text(id).Build()))
	if errors.Is(err, query.ErrNoRows) {
		return "", notFound
	}
	if err != nil {
		return "", fmt.Errorf("resolving tenant: %w", err)
	}
	var tenantID string
	if err := row.Scan(&tenantID); err != nil {
		return "", fmt.Errorf("scanning tenant: %w", err)
	}
	return tenantID, nil
}
```

- [ ] **Step 2: Extend the harness** with `tracking`, `clickCount` and `viewCount`.

```diff
--- a/internal/campaign/adapters/helpers_test.go
+++ b/internal/campaign/adapters/helpers_test.go
@@ -25,6 +25,7 @@
 	templates     func(t *testing.T) domain.TemplateRepository
 	campaigns     func(t *testing.T) domain.CampaignRepository
 	recipients    func(t *testing.T) domain.RecipientRepository
+	tracking      func(t *testing.T) domain.TrackingRepository
 	transactional func(t *testing.T) domain.TransactionalMessageRepository
 	// The seeds below create rows owned by other contexts. On Postgres they
 	// are real rows, because the campaign tables carry foreign keys to them.
@@ -37,6 +38,8 @@
 	// The probes read what no port exposes.
 	providerMessageID  func(t *testing.T, tenantID, recipientID string) string
 	transactionalCount func(t *testing.T, tenantID string) int
+	clickCount         func(t *testing.T, tenantID, linkID string) int
+	viewCount          func(t *testing.T, tenantID, campaignID string) int
 }
 
 var backends = []backend{
@@ -51,6 +54,9 @@
 		recipients: func(t *testing.T) domain.RecipientRepository {
 			return adapters.NewRecipients(dbtest.AppPool(t))
 		},
+		tracking: func(t *testing.T) domain.TrackingRepository {
+			return adapters.NewTracking(dbtest.AppPool(t))
+		},
 		transactional: func(t *testing.T) domain.TransactionalMessageRepository {
 			return adapters.NewTransactionalMessages(dbtest.AppPool(t))
 		},
@@ -70,6 +76,12 @@
 		transactionalCount: func(t *testing.T, tenantID string) int {
 			return pgCount(t, tenantID, "SELECT count(*) FROM transactional_messages", nil)
 		},
+		clickCount: func(t *testing.T, tenantID, linkID string) int {
+			return pgCount(t, tenantID, "SELECT count(*) FROM link_clicks WHERE link_id = $1", linkID)
+		},
+		viewCount: func(t *testing.T, tenantID, campaignID string) int {
+			return pgCount(t, tenantID, "SELECT count(*) FROM campaign_views WHERE campaign_id = $1", campaignID)
+		},
 	},
 	{
 		name: "ydb",
@@ -82,6 +94,9 @@
 		recipients: func(t *testing.T) domain.RecipientRepository {
 			return adapters.NewYDBRecipients(dbtest.YDBMigrated(t))
 		},
+		tracking: func(t *testing.T) domain.TrackingRepository {
+			return adapters.NewYDBTracking(dbtest.YDBMigrated(t))
+		},
 		transactional: func(t *testing.T) domain.TransactionalMessageRepository {
 			return adapters.NewYDBTransactionalMessages(dbtest.YDBMigrated(t))
 		},
@@ -99,6 +114,16 @@
 			return ydbCount(t, `DECLARE $tenant_id AS Utf8;
 SELECT COUNT(*) FROM transactional_messages WHERE tenant_id = $tenant_id;`, tenantID, "", "")
 		},
+		clickCount: func(t *testing.T, tenantID, linkID string) int {
+			return ydbCount(t, `DECLARE $tenant_id AS Utf8;
+DECLARE $k AS Utf8;
+SELECT COUNT(*) FROM link_clicks WHERE tenant_id = $tenant_id AND link_id = $k;`, tenantID, "$k", linkID)
+		},
+		viewCount: func(t *testing.T, tenantID, campaignID string) int {
+			return ydbCount(t, `DECLARE $tenant_id AS Utf8;
+DECLARE $k AS Utf8;
+SELECT COUNT(*) FROM campaign_views WHERE tenant_id = $tenant_id AND campaign_id = $k;`, tenantID, "$k", campaignID)
+		},
 	},
 }
 
```
- [ ] **Step 3: Rewrite the tracking tests.** Run `git mv internal/campaign/adapters/tracking_pg_test.go internal/campaign/adapters/tracking_test.go`, then replace the file whole.

```go
package adapters_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
)

// seedCampaignWithRecipient persists a campaign and one recipient, returning
// both ids.
func seedCampaignWithRecipient(t *testing.T, b backend, tenantID string) (campaignID, recipientID string) {
	t.Helper()
	ctx := context.Background()
	campaignID = newCampaignID(t, b, tenantID)
	subID := b.newSubscriber(t, tenantID, "s-"+campaignID+"@acme.com")
	recipients := b.recipients(t)
	_, err := recipients.BulkInsert(ctx, tenantID, campaignID, []*domain.Recipient{
		domain.NewRecipient(tenantID, campaignID, subID, "rcpt@acme.com"),
	})
	require.NoError(t, err)
	pending, err := recipients.Pending(ctx, tenantID, campaignID, 0, 1)
	require.NoError(t, err)
	require.Len(t, pending, 1)
	return campaignID, pending[0].ID()
}

func TestTrackingUpsertLinksIsIdempotent(t *testing.T) {
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.tracking(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		campaignID, _ := seedCampaignWithRecipient(t, b, tenantID)

		urls := []string{"https://acme.com/a", "https://acme.com/b", "https://acme.com/a"}
		first, err := repo.UpsertLinks(ctx, tenantID, campaignID, urls)
		require.NoError(t, err)
		require.Len(t, first, 2)

		second, err := repo.UpsertLinks(ctx, tenantID, campaignID, urls)
		require.NoError(t, err)
		require.Equal(t, first, second, "re-upserting yields the same link ids")

		// A new URL joins the existing ones without changing their ids.
		third, err := repo.UpsertLinks(ctx, tenantID, campaignID,
			[]string{"https://acme.com/a", "https://acme.com/c"})
		require.NoError(t, err)
		require.Equal(t, first["https://acme.com/a"], third["https://acme.com/a"])
		require.NotEmpty(t, third["https://acme.com/c"])

		empty, err := repo.UpsertLinks(ctx, tenantID, campaignID, nil)
		require.NoError(t, err)
		require.Empty(t, empty)
	})
}

// TestTrackingUpsertLinksIsRaceFree upserts the same URLs from several
// goroutines: every caller gets the same ids.
func TestTrackingUpsertLinksIsRaceFree(t *testing.T) {
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.tracking(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		campaignID, _ := seedCampaignWithRecipient(t, b, tenantID)

		urls := []string{"https://acme.com/a", "https://acme.com/b"}
		const workers = 6
		results := make([]map[string]string, workers)
		errs := make([]error, workers)
		done := make(chan int)
		for w := 0; w < workers; w++ {
			go func() {
				results[w], errs[w] = repo.UpsertLinks(ctx, tenantID, campaignID, urls)
				done <- w
			}()
		}
		for range results {
			<-done
		}
		for w := range results {
			require.NoError(t, errs[w])
			require.Equal(t, results[0], results[w])
		}
	})
}

func TestTrackingRecordClickAndView(t *testing.T) {
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.tracking(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		campaignID, recipientID := seedCampaignWithRecipient(t, b, tenantID)

		links, err := repo.UpsertLinks(ctx, tenantID, campaignID, []string{"https://acme.com/sale"})
		require.NoError(t, err)
		linkID := links["https://acme.com/sale"]

		url, err := repo.RecordClick(ctx, tenantID, linkID, recipientID)
		require.NoError(t, err)
		require.Equal(t, "https://acme.com/sale", url)
		require.Equal(t, 1, b.clickCount(t, tenantID, linkID))

		require.NoError(t, repo.RecordView(ctx, tenantID, campaignID, recipientID))
		require.Equal(t, 1, b.viewCount(t, tenantID, campaignID))
	})
}

func TestTrackingRecordClickUnknownLink(t *testing.T) {
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.tracking(t)
		tenantID := b.newTenant(t)
		_, recipientID := seedCampaignWithRecipient(t, b, tenantID)

		url, err := repo.RecordClick(context.Background(), tenantID, "not-a-uuid", recipientID)
		require.ErrorIs(t, err, domain.ErrLinkNotFound)
		require.Empty(t, url)
	})
}

func TestTrackingResolveTenant(t *testing.T) {
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.tracking(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		campaignID, _ := seedCampaignWithRecipient(t, b, tenantID)

		links, err := repo.UpsertLinks(ctx, tenantID, campaignID, []string{"https://acme.com/x"})
		require.NoError(t, err)
		linkID := links["https://acme.com/x"]

		// The resolvers run with no tenant bound.
		resolved, err := repo.ResolveTenantForLink(ctx, linkID)
		require.NoError(t, err)
		require.Equal(t, tenantID, resolved)

		resolved, err = repo.ResolveTenantForCampaign(ctx, campaignID)
		require.NoError(t, err)
		require.Equal(t, tenantID, resolved)

		_, err = repo.ResolveTenantForLink(ctx, "not-a-uuid")
		require.ErrorIs(t, err, domain.ErrLinkNotFound)
		_, err = repo.ResolveTenantForCampaign(ctx, "not-a-uuid")
		require.ErrorIs(t, err, domain.ErrCampaignNotFound)
		_, err = repo.ResolveTenantForLink(ctx, "00000000-0000-4000-8000-000000000000")
		require.ErrorIs(t, err, domain.ErrLinkNotFound)
	})
}

func TestTrackingCrossTenantIsolation(t *testing.T) {
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.tracking(t)
		ctx := context.Background()
		tenantA, tenantB := b.newTenant(t), b.newTenant(t)
		campaignA, recipientA := seedCampaignWithRecipient(t, b, tenantA)

		links, err := repo.UpsertLinks(ctx, tenantA, campaignA, []string{"https://acme.com/x"})
		require.NoError(t, err)
		linkA := links["https://acme.com/x"]

		// Tenant B cannot record against tenant A's link or campaign.
		_, err = repo.RecordClick(ctx, tenantB, linkA, recipientA)
		require.ErrorIs(t, err, domain.ErrLinkNotFound)
		require.NoError(t, repo.RecordView(ctx, tenantB, campaignA, recipientA))
		require.Zero(t, b.viewCount(t, tenantA, campaignA))
	})
}

func TestTrackingRecordClickAttributesOnlyOwnRecipients(t *testing.T) {
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.tracking(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		campaignID, ownRecipient := seedCampaignWithRecipient(t, b, tenantID)
		_, otherRecipient := seedCampaignWithRecipient(t, b, tenantID)

		links, err := repo.UpsertLinks(ctx, tenantID, campaignID, []string{"https://acme.com/sale"})
		require.NoError(t, err)
		linkID := links["https://acme.com/sale"]

		for _, s := range []string{otherRecipient, "", "not-a-uuid"} {
			url, err := repo.RecordClick(ctx, tenantID, linkID, s)
			require.NoError(t, err, "recipient %q", s)
			require.Equal(t, "https://acme.com/sale", url, "the redirect target is returned regardless")
		}
		require.Zero(t, b.clickCount(t, tenantID, linkID),
			"no click is attributed to a foreign, missing, or malformed recipient")

		_, err = repo.RecordClick(ctx, tenantID, linkID, ownRecipient)
		require.NoError(t, err)
		require.Equal(t, 1, b.clickCount(t, tenantID, linkID), "the campaign's own recipient is attributed")
	})
}

func TestTrackingRecordViewIgnoresForeignRecipients(t *testing.T) {
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.tracking(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		campaignID, ownRecipient := seedCampaignWithRecipient(t, b, tenantID)
		_, otherRecipient := seedCampaignWithRecipient(t, b, tenantID)

		require.NoError(t, repo.RecordView(ctx, tenantID, campaignID, otherRecipient))
		require.NoError(t, repo.RecordView(ctx, tenantID, campaignID, "not-a-uuid"))
		require.Zero(t, b.viewCount(t, tenantID, campaignID))

		require.NoError(t, repo.RecordView(ctx, tenantID, campaignID, ownRecipient))
		require.Equal(t, 1, b.viewCount(t, tenantID, campaignID))
	})
}
```
- [ ] **Step 4: Verify.**

```bash
gofmt -l internal
go vet ./internal/campaign/...
golangci-lint run ./internal/campaign/...
export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock" && go test ./internal/campaign/adapters -count=1 -timeout 8m
```

- [ ] **Step 5: Commit.**

```bash
git add internal/campaign/adapters
git commit -m "feat(campaign): tracking on YDB"
```

---

### Task 4: The send pipeline on both backends

**Files:**
- Modify: `internal/campaign/adapters/send_pipeline_test.go`, `resumability_test.go`

The fixture builds its repositories and seeds from the `backend`, so the start worker and the batch worker run over the YDB repositories too. The Redis limiter stays: it is a `domain.RateLimiter` either way. `start_worker_test.go` is unchanged: its quota tests use billing, which stays on Postgres.

- [ ] **Step 1: Apply the diffs.**

**`internal/campaign/adapters/send_pipeline_test.go`** (modify):

```diff
--- a/internal/campaign/adapters/send_pipeline_test.go
+++ b/internal/campaign/adapters/send_pipeline_test.go
@@ -96,9 +96,9 @@
 // pipelineFixture holds the wiring of one send-pipeline test.
 type pipelineFixture struct {
 	tenantID, campaignID string
-	campaigns            *adapters.Campaigns
-	recipients           *adapters.Recipients
-	tracking             *adapters.Tracking
+	campaigns            domain.CampaignRepository
+	recipients           domain.RecipientRepository
+	tracking             domain.TrackingRepository
 	messenger            *fakeMessenger
 	limiter              *ratelimit.Limiter
 	source               fakeSource
@@ -107,17 +107,16 @@
 
 // newPipelineFixture seeds a running campaign targeting recipientCount
 // subscribers and returns the wired pipeline.
-func newPipelineFixture(t *testing.T, recipientCount, maxSendErrors int,
+func newPipelineFixture(t *testing.T, b backend, recipientCount, maxSendErrors int,
 	perTenant, global ratelimit.Limit) *pipelineFixture {
 	t.Helper()
-	pool := dbtest.AppPool(t)
 	redisURL := dbtest.RedisURL(t)
 	dbtest.FlushRedis(t, redisURL)
 	ctx := context.Background()
 
-	tenantID := seedTenant(t, pool)
-	domainID := seedSendingDomain(t, pool, tenantID, "mail.acme.com")
-	campaigns := adapters.NewCampaigns(pool)
+	tenantID := b.newTenant(t)
+	domainID := b.newSendingDomain(t, tenantID)
+	campaigns := b.campaigns(t)
 
 	c, err := domain.NewCampaign(tenantID, "Pipeline", "Subject",
 		`<p>Hi <a href="https://acme.com/x">link</a></p>`, "Hi", "Acme", "news",
@@ -126,7 +125,7 @@
 	campaignID, err := campaigns.Add(ctx, tenantID, c)
 	require.NoError(t, err)
 
-	listID := seedList(t, pool, tenantID, "L")
+	listID := b.newList(t, tenantID)
 	require.NoError(t, campaigns.SaveTargets(ctx, tenantID, campaignID,
 		[]domain.Target{{ListID: listID}}))
 	require.NoError(t, campaigns.Update(ctx, tenantID, campaignID,
@@ -138,7 +137,7 @@
 	for i := 0; i < recipientCount; i++ {
 		email := dbtest.RandString() + "@acme.com"
 		members = append(members, domain.AudienceMember{
-			SubscriberID: seedSubscriber(t, pool, tenantID, email),
+			SubscriberID: b.newSubscriber(t, tenantID, email),
 			Email:        email,
 		})
 	}
@@ -150,8 +149,8 @@
 	return &pipelineFixture{
 		tenantID: tenantID, campaignID: campaignID,
 		campaigns:  campaigns,
-		recipients: adapters.NewRecipients(pool),
-		tracking:   adapters.NewTracking(pool),
+		recipients: b.recipients(t),
+		tracking:   b.tracking(t),
 		messenger:  newFakeMessenger(),
 		limiter:    limiter,
 		source:     fakeSource{members: members},
@@ -191,45 +190,48 @@
 }
 
 func TestSendPipelineDeliversEveryRecipientOnce(t *testing.T) {
-	t.Parallel()
-	limit := ratelimit.Limit{Max: 1000, Window: time.Second}
-	f := newPipelineFixture(t, 12, 100, limit, limit)
-	f.run(t, 5, limit)
+	eachBackend(t, func(t *testing.T, b backend) {
+		limit := ratelimit.Limit{Max: 1000, Window: time.Second}
+		f := newPipelineFixture(t, b, 12, 100, limit, limit)
+		f.run(t, 5, limit)
 
-	require.Equal(t, 12, f.messenger.total(), "every recipient received exactly one message")
-	for _, m := range f.source.members {
-		require.Equal(t, 1, f.messenger.count(m.Email))
-	}
+		require.Equal(t, 12, f.messenger.total(), "every recipient received exactly one message")
+		for _, m := range f.source.members {
+			require.Equal(t, 1, f.messenger.count(m.Email))
+		}
 
-	c, err := f.campaigns.Get(context.Background(), f.tenantID, f.campaignID)
-	require.NoError(t, err)
-	require.Equal(t, domain.CampaignFinished, c.Status(), "the campaign finishes once all are sent")
-	require.Equal(t, 12, c.SentCount())
+		c, err := f.campaigns.Get(context.Background(), f.tenantID, f.campaignID)
+		require.NoError(t, err)
+		require.Equal(t, domain.CampaignFinished, c.Status(), "the campaign finishes once all are sent")
+		require.Equal(t, 12, c.SentCount())
+	})
 }
 
 func TestSendPipelineRateLimitPacing(t *testing.T) {
-	t.Parallel()
-	// A tiny per-tenant window forces the batch worker to snooze and resume.
-	limit := ratelimit.Limit{Max: 3, Window: 200 * time.Millisecond}
-	global := ratelimit.Limit{Max: 1000, Window: time.Second}
-	f := newPipelineFixture(t, 9, 100, limit, global)
-	f.run(t, 9, limit)
+	eachBackend(t, func(t *testing.T, b backend) {
+		// A tiny per-tenant window forces the batch worker to snooze and resume.
+		limit := ratelimit.Limit{Max: 3, Window: 200 * time.Millisecond}
+		global := ratelimit.Limit{Max: 1000, Window: time.Second}
+		f := newPipelineFixture(t, b, 9, 100, limit, global)
+		f.run(t, 9, limit)
 
-	require.Equal(t, 9, f.messenger.total(), "pacing drops no recipient and duplicates none")
-	c, err := f.campaigns.Get(context.Background(), f.tenantID, f.campaignID)
-	require.NoError(t, err)
-	require.Equal(t, domain.CampaignFinished, c.Status())
+		require.Equal(t, 9, f.messenger.total(), "pacing drops no recipient and duplicates none")
+		c, err := f.campaigns.Get(context.Background(), f.tenantID, f.campaignID)
+		require.NoError(t, err)
+		require.Equal(t, domain.CampaignFinished, c.Status())
+	})
 }
 
 func TestSendPipelineAutoPausesPastErrorThreshold(t *testing.T) {
-	t.Parallel()
-	limit := ratelimit.Limit{Max: 1000, Window: time.Second}
-	f := newPipelineFixture(t, 10, 3, limit, limit)
-	f.messenger.failAll = true
-	f.run(t, 10, limit)
+	eachBackend(t, func(t *testing.T, b backend) {
+		limit := ratelimit.Limit{Max: 1000, Window: time.Second}
+		f := newPipelineFixture(t, b, 10, 3, limit, limit)
+		f.messenger.failAll = true
+		f.run(t, 10, limit)
 
-	c, err := f.campaigns.Get(context.Background(), f.tenantID, f.campaignID)
-	require.NoError(t, err)
-	require.Equal(t, domain.CampaignPaused, c.Status(),
-		"accumulated failures past max_send_errors auto-pause the campaign")
+		c, err := f.campaigns.Get(context.Background(), f.tenantID, f.campaignID)
+		require.NoError(t, err)
+		require.Equal(t, domain.CampaignPaused, c.Status(),
+			"accumulated failures past max_send_errors auto-pause the campaign")
+	})
 }
```

**`internal/campaign/adapters/resumability_test.go`** (modify):

```diff
--- a/internal/campaign/adapters/resumability_test.go
+++ b/internal/campaign/adapters/resumability_test.go
@@ -58,40 +58,41 @@
 }
 
 func TestSendPipelineResumesAfterWorkerCrash(t *testing.T) {
-	t.Parallel()
-	limit := ratelimit.Limit{Max: 1000, Window: time.Second}
-	f := newPipelineFixture(t, 10, 100, limit, limit)
+	eachBackend(t, func(t *testing.T, b backend) {
+		limit := ratelimit.Limit{Max: 1000, Window: time.Second}
+		f := newPipelineFixture(t, b, 10, 100, limit, limit)
 
-	// Materialise recipients and the single batch via the start worker.
-	start := adapters.NewStartWorker(f.campaigns, f.recipients, f.tracking,
-		f.source, f.enqueuer, nil, 100)
-	require.NoError(t, start.Work(context.Background(), jobs.CampaignStartArgs{TenantID: f.tenantID, CampaignID: f.campaignID}))
-	require.Len(t, f.enqueuer.batches, 1)
+		// Materialise recipients and the single batch via the start worker.
+		start := adapters.NewStartWorker(f.campaigns, f.recipients, f.tracking,
+			f.source, f.enqueuer, nil, 100)
+		require.NoError(t, start.Work(context.Background(), jobs.CampaignStartArgs{TenantID: f.tenantID, CampaignID: f.campaignID}))
+		require.Len(t, f.enqueuer.batches, 1)
 
-	messenger := &crashingMessenger{sent: map[string]int{}, crashAt: 4}
-	newBatch := func() *adapters.BatchWorker {
-		return adapters.NewBatchWorker(f.campaigns, f.recipients, f.tracking,
-			messenger, adapters.NewRateLimiter(f.limiter), fakeDomainLookup{name: "mail.acme.com"},
-			noopSuppression{}, nil, nil, nil, nil,
-			domain.Limit{Max: limit.Max, Window: limit.Window}, "https://track.test")
-	}
-	job := jobs.CampaignBatchArgs{TenantID: f.tenantID, CampaignID: f.campaignID, Offset: 0, Limit: 100}
+		messenger := &crashingMessenger{sent: map[string]int{}, crashAt: 4}
+		newBatch := func() *adapters.BatchWorker {
+			return adapters.NewBatchWorker(f.campaigns, f.recipients, f.tracking,
+				messenger, adapters.NewRateLimiter(f.limiter), fakeDomainLookup{name: "mail.acme.com"},
+				noopSuppression{}, nil, nil, nil, nil,
+				domain.Limit{Max: limit.Max, Window: limit.Window}, "https://track.test")
+		}
+		job := jobs.CampaignBatchArgs{TenantID: f.tenantID, CampaignID: f.campaignID, Offset: 0, Limit: 100}
 
-	// First run: the worker is "killed" after crashAt sends.
-	crashCtx, cancel := context.WithCancel(context.Background())
-	messenger.cancel = cancel
-	err := newBatch().Work(crashCtx, job)
-	require.Error(t, err, "the cancelled context aborts the batch")
-	require.Less(t, messenger.total(), 10, "the crash left recipients unsent")
+		// First run: the worker is "killed" after crashAt sends.
+		crashCtx, cancel := context.WithCancel(context.Background())
+		messenger.cancel = cancel
+		err := newBatch().Work(crashCtx, job)
+		require.Error(t, err, "the cancelled context aborts the batch")
+		require.Less(t, messenger.total(), 10, "the crash left recipients unsent")
 
-	// Restart: River redelivers the same batch with a fresh context.
-	require.NoError(t, newBatch().Work(context.Background(), job))
+		// Restart: River redelivers the same batch with a fresh context.
+		require.NoError(t, newBatch().Work(context.Background(), job))
 
-	require.Equal(t, 10, messenger.total(), "every recipient is eventually sent")
-	require.Empty(t, messenger.duplicates(), "no recipient is sent twice across the restart")
+		require.Equal(t, 10, messenger.total(), "every recipient is eventually sent")
+		require.Empty(t, messenger.duplicates(), "no recipient is sent twice across the restart")
 
-	c, err := f.campaigns.Get(context.Background(), f.tenantID, f.campaignID)
-	require.NoError(t, err)
-	require.Equal(t, domain.CampaignFinished, c.Status())
-	require.Equal(t, 10, c.SentCount())
+		c, err := f.campaigns.Get(context.Background(), f.tenantID, f.campaignID)
+		require.NoError(t, err)
+		require.Equal(t, domain.CampaignFinished, c.Status())
+		require.Equal(t, 10, c.SentCount())
+	})
 }
```

- [ ] **Step 2: Verify the whole branch.**

```bash
gofmt -l internal
go build ./... && go vet ./...
golangci-lint run
PATH="$HOME/go/bin:$PATH" make lint-arch
export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock" && go test ./... -timeout 15m
```

Expected: `0 issues.`, `[arch] clean`, and no failing package other than the known flakes.

- [ ] **Step 3: Commit.**

```bash
git add internal/campaign/adapters
git commit -m "test(campaign): run the send pipeline on both backends"
```

---

## Done criteria for Plan B7

- The five YDB adapters satisfy their ports, and `var _ domain.X = (*YDBX)(nil)` assertions compile.
- The pg+ydb contract suites pass; the cascade tests pass on YDB; the worker tests pass on both backends.
- `go build ./... && go vet ./... && go test ./...`, `golangci-lint run` and `make lint-arch` are clean.
- Nothing wires the YDB adapters yet. **Plan C4** must:
  - build `NewYDBTemplates`, `NewYDBCampaigns`, `NewYDBRecipients`, `NewYDBTracking` and `NewYDBTransactionalMessages` over the `*ydb.Driver`, and pass them where the pg ones go;
  - pass the already-ported `NewYDBRateLimiter` to the batch worker.
- **Plan B8** reads `idx_campaign_recipients_provider` and `idx_transactional_messages_provider` (created here) and must tolerate a dangling `campaign_recipient_id` (spec "Plan B7 planning spike", "For B8, B9 and C4").
