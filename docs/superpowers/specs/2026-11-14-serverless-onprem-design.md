# Serverless On-Prem: nvelope on Yandex Cloud Serverless + YDB

Date: 2026-11-14

## Problem

nvelope is built as a multi-tenant SaaS: four long-running Go services
(`cmd/api`, `cmd/worker`, `cmd/scheduler`, `cmd/consumer`) plus the Node BFF,
backed by PostgreSQL (with RLS for tenant isolation), River (a Postgres job
queue) and Redis (throttling). Customers want to run their own copy in their
own Yandex Cloud folder. The cost model they want is **zero cost when idle**:
they pay only for requests. That rules out anything that bills per hour: VMs,
Managed PostgreSQL, Managed Redis, and long-running processes.

## Goals

- A customer deploys a single-tenant nvelope into their own YC folder with one
  `terraform apply` plus a documented one-time Postbox setup step (V2: the
  provider has no Postbox resource).
- Every runtime piece is pay-per-use: Serverless Containers, API Gateway,
  serverless YDB, Message Queue (YMQ), triggers, Object Storage, Lockbox.
- Feature parity with SaaS, **minus billing and usage metering**.
- SaaS and on-prem share **one codebase on `main`**. The serverless variant is a
  set of alternative adapters behind the existing ports, selected at startup.
  It is not a fork.
- A test suite proves that the Postgres and YDB adapters behave the same.

## Non-goals

These are deferred on purpose:

- **Multiple tenants per on-prem install.** One install = one organization.
  YDB has no RLS, and single-tenant removes the isolation problem completely.
- **Billing, plans, usage metering** in on-prem. The contexts are not wired.
- **Moving SaaS to YDB.** SaaS stays on Postgres + River + Redis.
- **Cloud Marketplace listing.** It can come later on top of the Terraform module.
- **Keyset pagination at the port level.** Existing `count(*) + OFFSET`
  pagination through `domain.Page` is ported as is (see Ceilings). Adapters may
  page internally where a port returns an unbounded result (see V5).
- **Provisioned (warm) instances by default.** Available as a Terraform
  variable, defaulted to 0 to preserve zero idle cost.

## Decisions

| # | Decision | Rationale |
|---|----------|-----------|
| D1 | Serverless YDB is the on-prem database | Only pay-per-request relational store in YC. Managed PG bills per hour. |
| D2 | Same `main` branch, adapters selected via `NVELOPE_RUNTIME=saas\|serverless` | The ports already exist (DDD layout). A long-lived fork would drift from an actively developed `main` within weeks. |
| D3 | Single tenant per install | YDB has no RLS. Today's pg adapters rely on RLS and do not filter by `tenant_id` (e.g. `DELETE FROM subscribers WHERE id = $1`). The YDB adapters filter by `tenant_id` explicitly (see Transactions). The install is still single-tenant: this is defense-in-depth and test isolation, not multi-tenancy. |
| D4 | Background jobs on YMQ + Message Queue triggers, not a YDB topic | YMQ gives per-message retry (visibility timeout), DLQ and delayed delivery (snooze), as River does today. On a topic, a failing message blocks its partition and there is no delay. A YDB job table polled by a timer has minute latency and costs RU while idle. |
| D5 | The YDB topic is used only for Postbox delivery feedback | That is an event stream already (YDS), which fits a Data Streams trigger. |
| D6 | Redis replaced by one YDB fixed-window counter | No serverless Redis exists in YC. All three throttles (send limiter, verification-resend, public-form submission) are fixed or sliding windows. Point upserts cost very little. |
| D7 | Feature parity minus billing | No half-hidden screens. The UI hides only billing and tenant switching. |
| D8 | Deployment via a Terraform module in the repo | Idempotent. An upgrade is a change of the image tag. |

## Architecture

```
               ┌───────────── API Gateway (custom domain, Certificate Manager) ─────────────┐
 browser ─────►│ 3 Phase 7 visual endpoints, SPA      → container `frontend` (Node BFF)      │
               │ (a second `bff` gateway serves /t/:slug/api/* for the frontend's BFF → `api`) │
               │ /t/:slug/api/*, /api/platform/*, tracking, unsubscribe, public pages → `api` │
               └──────────────────────────────────────────────────────────────────────────────┘
 api ── Enqueue ──► YMQ `jobs` / `jobs-long` (each + DLQ) ──MQ trigger──► worker POST /jobs
 Timer triggers (cron) ─────────────────────────────────────────────────► worker POST /cron/:name
 Postbox → YDS topic (in the same YDB db) ──Data Streams trigger────────► worker POST /feedback
 api, worker ──► serverless YDB · Object Storage (media) · Postbox · Lockbox (secrets)
```

### Containers

Each container is built from an existing image. There are no new binaries.

| Container | Image | Role env | Concurrency | Timeout | Routes |
|-----------|-------|----------|-------------|---------|--------|
| `api` | `api` | `NVELOPE_ROLE=api` | high (e.g. 16) | 30 s | existing HTTP API |
| `frontend` | `frontend` | — | high | 30 s | SPA + 3 visual endpoints |
| `worker` | `worker` | `NVELOPE_ROLE=worker` | 1 | up to 1 h | `/jobs`, `/cron/:name`, `/feedback`, `/admin/migrate` |

- All containers are **private**. The `gateway` service account (API Gateway)
  invokes `api` and `frontend`. The `trigger` service account invokes only
  `worker`. Both hold `serverless.containers.invoker`.
- `cmd/worker` in serverless mode starts an HTTP server on `$PORT` instead of a
  River client. The worker-only routes are registered **only** when
  `NVELOPE_ROLE=worker`.
- `cmd/scheduler` and `cmd/consumer` are not deployed in serverless mode. Their
  logic moves to `/cron/:name` and `/feedback`.

### Data flow: sending a campaign

1. `api` enqueues `campaign.start` on `jobs-long`.
2. The trigger calls `worker POST /jobs`. The existing start worker resolves
   recipients and enqueues N `campaign.batch` messages (offset/limit), each its
   own YMQ message.
3. Each batch is a separate invocation. The batch size is configured so that one
   batch completes well inside the container timeout and the queue visibility
   timeout.
4. Redelivery is safe: the batch worker selects only `Pending` recipients and
   stops on `ctx.Err()`. The remaining recipients are picked up on redelivery.

## Storage layer (YDB)

### Layout

- Every `internal/<ctx>/adapters/*_pg.go` gets a sibling `*_ydb.go` that
  implements the same domain/app port. Domain and app layers are unchanged.
- In scope: `audience`, `auth`, `campaign`, `deliverability`, `iam`, `media`,
  `sending`, `tenant`. About 4.9k lines of pg adapter code to port.
- `billing` is not ported and not wired in serverless mode.
- The composition root (`internal/service/application.go`) gets a serverless
  builder that wires the YDB adapters, the YMQ enqueuer and the YDB throttles.

### Transactions

- Adapters call `db.Query().DoTx` from `ydb-go-sdk/v3` (already in `go.mod`)
  directly, in place of `tenantdb.WithTenant`. It is serializable read-write by
  default. A one-line `ydbtx.Do` wrapper was planned and dropped (Plan B1): it
  would add nothing. `DoTx` only runs the transaction. Tenant scoping is the
  adapter's responsibility: every YQL query filters by the `tenantID` the port
  already receives, through an explicit `tenant_id` predicate. The reasons are
  isolation of parallel contract tests (see Testing) and keeping the
  multi-tenant option open.
- The SDK **retries** the closure on `ABORTED`/`UNAVAILABLE`. So every
  transaction closure must be retry-safe: database operations only, no external
  side effects (no enqueue, no email, no HTTP). The plan audits all 129
  transaction call sites. Today enqueue is already outside the transaction
  (`client.Insert`, never `InsertTx`).

### Schema and migrations

- One YQL migration file per ported context in `internal/db/ydbmigrations/`,
  numbered in port order (`000001_sending.up.sql`, `000002_throttle.up.sql`,
  …), covering the in-scope tables (about 38 of the 45) once all contexts are
  ported. Nothing is released yet, so there is no history to keep, and the
  21-migration Postgres history is not replayed.
- Runner: `golang-migrate` v4.19.1 has **no** `ydb` driver (checked during
  planning). The default is a small in-repo runner:
  - embedded `*.up.sql` files are applied in order, one `db.Query().Exec` per
    statement (files are split on lines ending in `;`), because the general YDB
    query-text limit is 10 KB (V5) and the current pg migrations total 59 KB
    with 45 `CREATE TABLE`s;
  - applied versions are recorded in a `schema_migrations (version_id Uint64)`
    table; a version is recorded only after all of its statements succeed.

  If this runner proves unworkable, the fallback is `pressly/goose` v3.30.1,
  which has a `ydb` dialect.
- **Baseline migration rules.** YDB does not run DDL inside a transaction, so a
  failure partway through a file can leave a partial schema. `/admin/migrate`
  stays idempotent only if every DDL statement uses `IF NOT EXISTS` (and
  `IF EXISTS` for drops). Plan B1 tested this failure path
  (`TestMigrateYDBConvergesAfterPartialFailure`): a half-applied file is not
  recorded, and the re-run converges.
- **Non-idempotent statements sit alone in their own file.** YQL
  `ALTER TABLE … ADD/DROP COLUMN` has no `IF NOT EXISTS` form, so the
  idempotency rule cannot hold for schema changes after the first release. A
  file that holds such a statement holds nothing else. Ceiling: if the
  statement succeeds but recording the version fails, the re-run fails on the
  already-applied change and needs a manual fix. The upgrade path is recording
  progress per statement.
- Tenant-scoped tables use a primary key starting with `tenant_id`, e.g.
  `(tenant_id, id)`, so the explicit predicate is a key-prefix lookup.
- **IDs are generated in Go** (`uuid.NewString()`, v4). Today 36 tables default
  to `gen_random_uuid()`, so pg adapters that depend on `RETURNING id` from a DB
  default must pass the ID explicitly in the YDB variant. Random keys spread
  writes across partitions.
- Uniqueness (emails, slugs, tokens) uses `GLOBAL UNIQUE SYNC` secondary
  indexes. Lookups use `GLOBAL SYNC` indexes.
- Sessions, verification tokens and throttle windows use native **table TTL**.
  The verification-cleanup cron job is not needed in serverless mode.

### SQL translation rules

| Postgres | YQL |
|----------|-----|
| `INSERT ... ON CONFLICT DO UPDATE` (18 sites, together with DO NOTHING) | `UPSERT` |
| `INSERT ... ON CONFLICT DO NOTHING` | An INSERT conflict aborts the whole interactive transaction ("Plan B2 planning spike"). Inside a transaction use `SELECT` then `INSERT` (as `AddYDBMembership` does, Plan B3). A standalone single-statement INSERT may treat `db.IsYDBConflict` as a no-op. `UPSERT` is wrong where it would overwrite existing data. `Recipients.BulkInsert` (Plan B7) reads the emails already present through the unique index, then inserts only the others, one transaction per chunk of 1000: the transaction cannot continue after a conflict, and a batch that repeats a key fails whole ("Plan B7 planning spike"). A write keyed by a unique index only (`links`) does the same. |
| `SELECT ... FOR UPDATE` (7) | removed. A serializable conflict aborts the tx and the SDK retries it. |
| `RETURNING` (26) | supported as is |
| `now()` (28) | `CurrentUtcTimestamp()` |
| `jsonb` | `JsonDocument` where YQL queries into the document (audience `subscribers.attributes`, Plan B5: bound with `JSONDocumentFromBytes`, scanned into `string`). JSON that is only stored and read back whole (e.g. `sending_domains.dkim_records`) is a plain `Utf8` column. |
| `bytea` file payloads (`import_export_jobs.file_bytes`: an import upload of up to 32 MB, an export of any size) | a private Object Storage object `jobs/<tenant_id>/<job_id>` in the media bucket; the row keeps a nullable `file_key` (Plan B6). A YDB cell holds at most 16 MiB ("Plan B6 planning spike"). |
| `text[]` read back whole (`roles.permissions`, `api_keys.permissions`, `lists.tags`) | a JSON array in a plain `Utf8` column (Plan B4). An empty slice is stored as `[]`, so it reads back as an empty slice, as Postgres reads back `'{}'`. |
| `citext` (emails, slugs) | Where the domain already lower-cases the value (`tenants.slug`, iam `users.email`, audience `subscribers.email`), the column itself carries the unique index. A query that compares such a column with user input lower-cases the input in Go (the segment `email` condition, Plan B5). Otherwise the value is stored as entered, plus a lower-cased `email_key` column that carries the index (`platform_users`, `invitations`). |
| foreign keys | YDB has none. Where a pg foreign key makes a write fail and the referenced table belongs to the same context, the YDB adapter reads the referenced row in the same `DoTx` (iam role assignments read `roles`, Plan B4). A stand-in read may also target another context's table already on YDB (iam per-list role assignments read audience `lists`, commit `5fadd55`, after Plan B5). A cascade the port must keep becomes an explicit delete in the parent's delete transaction (the same `DoTx` as the parent's delete). The cascade may cross into another context's table when that table is already on YDB: `YDBLists.Delete` deletes iam `user_list_roles` rows (Plan B5). A cascade into a table that is not on YDB yet is the porting plan's job (see "Plan B5 planning spike"). |
| `ORDER BY` on a text column | YDB orders `Utf8` by bytes, pg by the database collation, so mixed-case and non-ASCII values order differently. Acceptable for display lists (iam `roles` by `name`). If a later plan needs collation-equal order, sort in Go or store a sort key. |
| `UPDATE ... WHERE id = $1` on a table whose key is not the id alone (`campaign_recipients`: `(tenant_id, campaign_id, id)`) | `UPDATE t ON SELECT <key columns>, <new values> AS col FROM t VIEW idx WHERE ...` through a `GLOBAL SYNC` index on the id. An id that matches no row updates nothing and is no error, as the Postgres `UPDATE` (Plan B7). |
| a bulk `INSERT` of many rows | `INSERT INTO t (...) SELECT $const, id, ... FROM AS_TABLE($rows)` with `$rows` as `List<Struct<...>>`, bound with `types.ListValue(types.StructValue(...))` through `Param(...).Any(...)`. The ids are fixed in Go before the transaction (Plan B7). |
| `SECURITY DEFINER` lookup of the tenant that owns an id (`tracking_tenant_for_link`, `..._for_campaign`) | a `GLOBAL UNIQUE SYNC` index on `(id)` alone, read with `VIEW`. The index entry carries the primary key, so `tenant_id` comes with it (Plan B7). It leaks what the function leaked: the tenant of a caller-supplied id. |
| bound parameters (`$1`, `@name`) | `$name` parameters, each declared explicitly with `DECLARE $name AS <Type>;`. String literals that target a `Utf8` column take the `u` suffix (`"pending"u`); a bare `"..."` literal is `String`. Identifiers avoid YQL keywords (e.g. `throttle_key`, not `key`). |
| segment SQL translator (`internal/audience/adapters/segment_sql.go`: jsonb `?`, `->>`, `::numeric`) | its own YQL port, `segment_yql.go` (Plan B5). The attribute key is bound, never part of the path: `JSON_VALUE(s.attributes, "$.keyvalue() ? (@.name == $k).value" PASSING $pN AS k)` for `->>`, the same path in `JSON_EXISTS` for `?`, and `CAST(JSON_VALUE(...) AS Double)` for `::numeric`. A list-membership condition is `s.id IN (SELECT subscriber_id FROM subscriber_lists VIEW ... WHERE ...)`, because YQL has no correlated `EXISTS`. |
| `ILIKE` (3 sites: `deliverability/adapters/suppression_pg.go:91` email filter; `audience/adapters/segment_sql.go:68` `col::text ILIKE`; `segment_sql.go:87` `attributes->>key ILIKE`) | YQL `ILIKE`. Plan B5 verified it on `Utf8` columns, with a bound `%...%` pattern and with `"%"u || $v || "%"u`, against Cyrillic data (`ё` included). No lowercase column is needed. Plan B8 ported the `suppression_pg.go` site and tested it on both backends with an ASCII and a Cyrillic (`ЁЛК` against `ёлка@пример.рф`) filter. |
| `SECURITY DEFINER` lookup of the tenant that owns a provider message id (`feedback_tenant_for_message`) | two reads, `campaign_recipients VIEW idx_campaign_recipients_provider` and then `transactional_messages VIEW idx_transactional_messages_provider`, each with `LIMIT 1`. The index entry carries `tenant_id` (Plan B8; the indexes are Plan B7's). |
| `INSERT ... ON CONFLICT DO NOTHING` that must report whether it inserted (`StageInbound`, `RecordEvent`) | `SELECT` through the unique index, then `INSERT` when absent, in one `DoTx`. A concurrent identical write aborts one transaction and the SDK retries it, so exactly one writes (Plan B8; `TestEventsStageInboundIsRaceFree`, `TestEventsRecordEventIsRaceFree`). |
| a per-campaign aggregate that is a correlated `(SELECT count(...) ... WHERE x.campaign_id = c.id)` subquery (`campaign_analytics` refresh) | named subqueries `$name = SELECT campaign_id, COUNT(...) ... GROUP BY campaign_id;` and `LEFT JOIN`s from `campaigns`, written by `UPSERT INTO ... SELECT`. `count(DISTINCT x) FILTER (...)` becomes `COUNT(DISTINCT IF(cond, x))`. The select list must alias each column to its table column name, and a `COALESCE(count, 0ul)` is cast and wrapped: `Unwrap(CAST(COALESCE(n, 0ul) AS Int64))` (Plan B8). |
| a nullable numeric column the domain reads as `0` for "absent" (`media_assets.width`, `height`: `COALESCE(width, 0)`, NULL for a PDF so that `CHECK (> 0)` holds) | a `NOT NULL Int32` column that stores `0`. With no CHECK constraints, the CHECK is the reason Postgres needed NULL, and the adapter scans into `int32` and converts (Plan B9). |
| a read-then-write `UPDATE ... SET c = COALESCE(c, $v)` that must report a missing row (`MarkPublished`) | `SELECT` the row in the `DoTx` (no row: not-found), return when the column is already set, else `UPDATE` by primary key (Plan B9; six concurrent calls on one asset all succeed and leave it published (spike)). |
| `SECURITY DEFINER` functions (6) | ordinary queries. With no RLS, the privilege bypass they existed for is unnecessary. Billing-only functions are dropped. |

### Throttling

`ydbthrottle.Counter` is a fixed-window counter on a table
`throttle_windows(throttle_key Utf8, window_start Timestamp, hits Uint64, expire_at Timestamp)`,
primary key `(throttle_key, window_start)`, with TTL on `expire_at`.
`Allow(ctx, windows...)` runs one read-modify-write transaction over one or
more windows. It admits the hit only when every window has room, and only then
counts it in all of them, matching the Redis limiter's semantics (a denial
never consumes one window's budget while another is full). The SDK retries on
a serializable conflict. When denied it returns `retryAfter` = the longest
wait until a full window rolls over. It backs all three existing ports:

- `campaign/domain.RateLimiter` (send limiter, today `platform/ratelimit` on Redis)
- the auth verification-resend throttle (today `auth/adapters/resend_throttle.go`)
- the public subscription submission throttle (today `audience/adapters/submission_throttle.go`)

SaaS keeps the Redis implementations.

## Job runtime

### Decoupling workers from River

- Every worker changes from `Work(ctx, *river.Job[T]) error` to
  `Work(ctx, args T) error`. Workers read nothing from `river.Job` except
  `Args` (verified).
- `jobs.AddWorker(workers, w)` is one generic wrapper that registers such a
  worker with River for SaaS. It mirrors the signature of `river.AddWorker`.
- Snooze stays `river.JobSnooze(d)`. It returns `*rivertype.JobSnoozeError`,
  which River already understands. The YMQ dispatcher detects it with
  `errors.As` and turns it into a delayed re-send. A project-owned snooze type
  would add a mapping layer and buy nothing.
- This step changes no behavior and lands first, independently.

### YMQ enqueuer

- It implements the same enqueuer interfaces that `platform/jobs.Enqueuer` and
  its siblings implement today.
- Message body: `{"kind": "...", "args": {...}, "not_before": "<RFC3339 or empty>"}`.
- Queue routing by kind:
  - `jobs-long` (visibility timeout ≥ worker container timeout):
    `audience.import`, `audience.export`, `campaign.start`, `campaign.batch`.
  - `jobs` (short visibility timeout): everything else that is wired in
    serverless mode.
- Each queue has a DLQ through a redrive policy with `maxReceiveCount`. C3
  set it to 20 on `jobs` and 10 on `jobs-long` (see "Carried to C3 and C4").
- Client: `aws-sdk-go-v2/service/sqs` against the YMQ endpoint, using static
  access keys from Lockbox. It is pinned to v1.42.27, the release built on the
  `aws-sdk-go-v2` core already in `go.mod` (v1.41.7); `@latest` would upgrade
  the core shared with the S3 and Postbox clients. It speaks the AWS JSON
  protocol, which YMQ accepts (see "Plan C1 planning spike").
- **`UniqueOpts{ByArgs: true}` is not reproduced** (6 sites in `jobs.go`).
  Standard YMQ queues are used. V3 was resolved during planning:

  | Kind | Serverless handling |
  |------|---------------------|
  | `billing.sweep`, `billing.charge`, `usage.rollup` | not wired (billing is out of scope) |
  | `auth.verification_cleanup` | not wired (table TTL replaces it) |
  | `analytics.refresh` | duplicates are harmless: an idempotent recompute, and the cron interval is far longer than one refresh |
  | `domain.verify` via `EnqueueVerifyUnique` | **the sweep re-arms only lost chains** (below) |

  The sweep relies on uniqueness to avoid stacking chains (`cmd/scheduler`
  `sweepPendingDomains`). Without it, every sweep would start one more
  self-snoozing chain per pending domain.

  Plan C1 fixes this in the serverless sweep, not in the worker. The
  `/cron/domain-verify-sweep` handler re-arms a pending domain only when its
  last poll (or its creation, if it was never polled) is older than two poll
  intervals. A chain that is on schedule polls every interval, so the sweep
  never doubles it. If job delivery stalls for more than two intervals, the
  sweep does start one (see Ceilings). `VerifyWorker` and SaaS are unchanged.

  The worker-side debounce planned earlier (`VerifyWorker.Work` returns `nil`
  when `now - LastCheckedAt <= interval/2`) was dropped in Plan C1 planning.
  `RecheckDomain` (the user's "check now") enqueues a plain `EnqueueVerify`, so
  under the debounce a recheck within 7.5 minutes of the last poll (default
  interval 15 min) would not poll at all, in SaaS too. The duplicate chain a
  recheck starts is pre-existing behavior in both runtimes (see Ceilings).

### `POST /jobs` (Message Queue trigger, batch size 1)

The handler decodes the trigger payload and dispatches by `kind` through a
`map[string]func(ctx, json.RawMessage) error` registry. The context has one of
two deadlines:

- Short-queue kinds get a short timeout. It stays below the short queue's
  visibility timeout.
- Long kinds (import, export, campaign start and batch) get the container
  timeout minus a safety margin. It stays below the long queue's visibility
  timeout.

| Outcome | Action | HTTP |
|---------|--------|------|
| `nil` | — (the trigger deletes the message) | 200 |
| `*rivertype.JobSnoozeError{Duration: d}` (from `river.JobSnooze`) | send a copy with `DelaySeconds = min(d, 900s)` and `not_before = now + d` | 200 |
| `not_before` in the future | re-send with the remaining delay, capped at 900 s | 200 |
| any other error, unknown `kind`, undecodable body | log the error (it names the kind) and the YMQ message id. The args are not logged: `optin.send` and `auth.verification_send` carry raw tokens. | 500 → redelivered after the visibility timeout, then DLQ |

### `POST /cron/:name` (timer triggers)

Each route enqueues the same jobs `cmd/scheduler` enqueues today:

- `domain-verify-sweep`
- `analytics-refresh` for the single tenant

`cmd/scheduler` selects pending domains and active tenants with raw SQL on the
pool (`sweepPendingDomains`, `enqueueAnalyticsRefresh`). The cron handlers need
no new queries: they find the single tenant with `TenantRepository.GetBySlug`
(`NVELOPE_TENANT_SLUG`) and its domains with `SendingDomainRepository.All`.
Before the first-run setup has created the tenant, both sweeps do nothing. A
failed sweep is logged and still answered 200, as `cmd/scheduler` logs and
carries on: the next tick is the retry. A trigger-level retry could re-arm a
domain twice before its first poll.

The campaign workers already accept `nil` for the billing ports `quota` and
`usage` ("a deployment without billing"), so serverless wiring passes `nil`.

Billing sweep, usage rollup and verification cleanup are not wired: billing is
out of scope, and TTL replaces the cleanup.

### `POST /feedback` (Data Streams trigger on the Postbox topic)

- Decodes each message in the trigger batch and calls the existing
  `deliverability/app/command.IngestNotificationHandler` (staging row +
  `feedback.process` enqueue). This is the same handler `cmd/consumer` calls.
- A malformed notification (`deliverability/domain.ErrValidationFailed`) is
  logged and skipped, as `cmd/consumer` skips it: it can never succeed. Any
  other failure returns 500. The trigger retries the batch per its retry
  settings, then sends it to the trigger's DLQ. Staging dedupes by
  notification, so a retried batch does not count the messages before the
  failure twice.

## On-prem packaging

### Single tenant and first run

- `NVELOPE_RUNTIME=serverless` implies single-tenant mode. The tenant slug comes
  from config (`NVELOPE_TENANT_SLUG`, set from the Terraform `tenant_slug`).
  Existing `/t/:slug/...` URLs are unchanged.
- Terraform generates a random `setup_token` and stores it in Lockbox. Terraform
  output prints the `yc lockbox payload get ...` command to read it.
- While the workspace at `NVELOPE_TENANT_SLUG` does not exist, `POST /api/platform/signup`
  requires the setup token (`setup_token` in the body, at least 32 characters by config
  validation). A valid token:
  - creates the first user, **skipping email verification**, because nothing can
    send mail before the first login and holding the token already proves the
    right to install. The user is signed in at once (`UserRepository.CreateWithSession`);
  - creates the tenant through the existing `create_workspace` command, with the
    user as owner. Its name starts as the slug; the owner renames it in Settings.

  Without the token: 403 `setup_token_invalid`. Once the workspace exists: 403
  `setup_complete`. Plan C2 keyed "setup is open" on the workspace, not on "no user
  exists": it reuses the `LocateWorkspace` query, it is testable on a shared database,
  and the cron sweeps and invitations need the workspace anyway.
- If the account is created but the workspace is not, the owner is already signed in:
  the landing page offers "Create workspace", and `POST /api/platform/tenants` always
  uses `NVELOPE_TENANT_SLUG` in single-tenant mode.
- After that, people join only by invitation (`invite_teammate` /
  `accept_invitation`).
- `POST /api/platform/tenants` pins the slug to `NVELOPE_TENANT_SLUG`, so it returns 409 `slug_taken` once the tenant exists.
- The setup-token comparison is constant-time: both sides are hashed with SHA-256 first, so the length does not leak either.

### UI feature flags

- A public `GET /api/platform/config` returns
  `{"features": {"billing": bool, "multi_tenant": bool}, "setup_required": bool}`
  (snake_case, like the rest of the platform plane). It is a hand-written route:
  only the tenant plane is spec-first so far, and the route moves into
  `api/openapi/platform.yaml` when the platform plane migrates (Plan C2).
- The frontend hides billing screens and the tenant switcher when the flags are
  false. It shows the setup-token field on the signup page when
  `setup_required` is true.
- The signup links on the landing and login pages are hidden once a single-tenant install is set up. No other frontend changes.

### Terraform module: `deploy/terraform/nvelope-serverless/`

Inputs: `folder_id`, `domain`, `dns_zone_id` or `certificate_id` (exactly one),
`image_tag`, `registry_id`, `tenant_slug`, `system_sender_domain`,
`send_rate_per_sec`, `provisioned_instances` (default 0),
`ydb_throttling_rcu_limit` (default 1000 RU/s; 0 disables the limit) and
`deletion_protection` (default true). Serverless YDB is throttled at 10 RU/s by
default (V5), which would make imports and sends crawl. The provider arguments
were verified in Plan C3 (see "Plan C3 planning spike"):
`serverless_database { enable_throttling_rcu_limit, throttling_rcu_limit }`.

Resources:

- Service accounts (Plan C3):
  - `app`, the runtime identity of `api` and `worker`. Both build the same
    clients, so they need the same roles: YDB editor on the database, YMQ
    writer, Postbox editor, storage editor on the media bucket, Lockbox payload
    viewer on the secret.
  - `gateway`, which invokes `api` and `frontend`.
  - `trigger`, which reads the queues and the topic and invokes `worker`.
  - `queues`, used by Terraform only, to create the queues: their API takes
    only static keys.
- One static access key for `app`, used for YMQ, Postbox and S3, stored in
  Lockbox.
- Serverless YDB database and a YDB topic for Postbox feedback.
- YMQ queues `jobs` and `jobs-long`, each with a DLQ, and a DLQ for the
  feedback trigger.
- An Object Storage bucket for media.
- A Lockbox secret holding the setup token, the TOTP encryption key and the
  static key.
- Serverless Containers `api`, `frontend` and `worker`, with images from the
  project's public Container Registry at `image_tag`. The registry is created
  once by the maintainers, and its ID is a module default.
- Two API gateways (Plan C3):
  - `public` routes paths to `api` and `frontend` on the custom domain, with a
    Let's Encrypt certificate from Certificate Manager. This requires a DNS
    zone in YC; otherwise the user supplies a certificate ID.
  - `bff` routes `/t/{slug}/api/*` to `api` on its default domain. The BFF
    calls Go through it.
- Triggers: 2 Message Queue (`jobs`, `jobs-long`), 2 timer, 1 Data Streams.
- Postbox is **not** created by Terraform: the provider has no Postbox resource
  (V2). Address and configuration set (with the topic as feedback destination)
  are a manual README step.

Migrations:

- A `terraform_data` resource, re-triggered on `image_tag` change, runs a
  local-exec that calls the private `worker POST /admin/migrate` with a
  `yc iam create-token` bearer token. The operator therefore needs `yc`.
- `/admin/migrate` runs the YDB migration runner and is idempotent.

Upgrade: bump `image_tag`, then `terraform apply`.

CI: publish `api`, `worker` and `frontend` images to the public registry, tagged
by release version. A `v*` tag push builds them for `linux/amd64` (Serverless
Containers run only x86_64 images) and pushes them as `cr.yandex/<registry>/<name>:<tag>`.

## Testing

- **Contract suites.** The existing `*_pg_test.go` files (about 3.6k lines) are
  restructured so that each test body runs under `t.Run("pg")` and
  `t.Run("ydb")` against a constructor-parameterized repository. Fixtures are
  created through the repositories wherever possible, not through raw SQL. The
  YDB backend runs in testcontainers with `ydbplatform/local-ydb`: one reused
  container, the same pattern as `nvelope-test-pg`. This is the parity
  guarantee.
  - **Isolation.** YDB adapters keep explicit `tenant_id` predicates, and
    contract tests isolate with the same `seedTenant` pattern as the pg suites,
    so `t.Parallel()` works on the shared `/local` root. The container also
    holds the `dbtest_ready` and `dbtest_ddl_probe` tables, so Plan B tests
    must not assert an exact table set.
- **Job runtime tests use real SQS semantics from fakecloud.** YMQ speaks the
  SQS API, so the YMQ enqueuer and the `/jobs` dispatcher are tested against
  [fakecloud](https://github.com/faiscadev/fakecloud), a local AWS emulator.
  It runs as a `dbtest.SQSEndpoint(t)` testcontainer helper, following the
  `dbtest.YDB` pattern, with `NVELOPE_TEST_SQS_ENDPOINT` as the override.
  - **Verified on 2026-11-14 against `ghcr.io/faiscadev/fakecloud:0.50.0`**
    (multi-arch, native on arm64):
    - per-message `DelaySeconds`, rejecting values above 900 exactly as YMQ does;
    - redelivery after `VisibilityTimeout`;
    - `ApproximateReceiveCount`;
    - `RedrivePolicy` with `maxReceiveCount` moving the message to the DLQ;
    - message attributes.
  - **Container setup:**
    - Run it with `FAKECLOUD_REGION=ru-central1`. Queues are regional, and the
      SDK signs for the YMQ region.
    - Run it with `FAKECLOUD_ADDR=[::]:4566`. The default `0.0.0.0` bind is
      IPv4-only, and `localhost`, which testcontainers returns as the host,
      then fails through the IPv6 port mapping.
    - Readiness is `GET /_fakecloud/health`.
    - Credentials are dummy values (`test`/`test`).
  - **The trigger is not emulated.** fakecloud has no YC Message Queue
    triggers. Tests drive the handler with a small test-only pump: receive
    from fakecloud, wrap the message in the YC trigger payload shape, `POST`
    it to `/jobs`, and delete the message on 200. This exercises the real
    retry and DLQ path: a 500 leaves the message to reappear after the
    visibility timeout.
  - The dispatcher outcome table (`nil`, snooze, `not_before`, error, unknown
    kind, bad body → status and re-send) runs through this pump.
- **Throttle counter.** Window rollover and `retryAfter` against local YDB.
- **Bootstrap.** Signup with no token, a wrong token and the right token on an
  empty DB; a second signup after the first user exists; tenant creation after
  the tenant exists.
- **Smoke test in a real YC folder** before a release: `terraform apply`, setup,
  add a domain, import a CSV, send a campaign to a test list, receive a bounce
  through Postbox feedback, `terraform destroy`.

## Verify at plan start

V1–V6 are resolved; see "Spike findings (Plan A)". The list below is kept as
the original questions.

These are spikes. Each can change one detail of this design, but none changes
its shape.

- **V1** YQL `ILIKE` availability in YDB. Fallback: an `email_lower`
  column. *(Plan A spike against local YDB.)*
- **V2** `yandex` Terraform provider support for Postbox resources, for
  `yandex_ydb_topic`, and for Message Queue, timer and Data Streams triggers
  that target a Serverless Container. *(Plan A docs check.)*
- **V3** Resolved during planning. See the table under "YMQ enqueuer".
- **V4** Whether query-service `Exec` runs one multi-statement DDL file with a
  TTL table and a `GLOBAL UNIQUE SYNC` index, plus `RETURNING` and
  `CurrentUtcTimestamp()`. `golang-migrate` has no `ydb` driver, so this
  decides between the in-repo runner and `goose`. *(Plan A spike against local
  YDB.)*
- **V5** Serverless YDB per-query limits (result rows, duration, transaction
  size) for export and recipient resolution. Page through results where
  needed. *(Plan A docs check.)*
- **V6** Retry-safety audit. Two sets of closures are checked:
  - every closure passed to a repository `Update*` method outside the pg
    adapters (about 40 sites, in `*/app/command` and `*_worker.go`);
  - the bodies of the 129 `WithTenant` closures in the pg adapters, for any
    non-`tx` side effect.

  *(Plan A audit.)*

## Spike findings (Plan A)

Run against `ydbplatform/local-ydb:26.3.1.23`.

### V1 — `ILIKE`

PASS. `SELECT "Alice@Example.com" ILIKE "alice%" AS hit;` returns true. The `String::AsciiToLower` fallback was not needed.

Scope: `grep -rn ILIKE internal --include='*.go'` finds three sites, not one:
`internal/deliverability/adapters/suppression_pg.go:91` (email filter on the
suppression list), `internal/audience/adapters/segment_sql.go:68`
(`col::text ILIKE`) and `segment_sql.go:87` (`attributes->>key ILIKE`). The
spike only tested an ASCII literal against an ASCII literal, never a Utf8
column, a parameter or non-ASCII (Cyrillic) text.

Decision: the three sites use YQL `ILIKE`, provisionally. **To verify in
Plan B:** the first audience or deliverability contract test must check
`ILIKE` on a Utf8 column, with a parameterised `%...%` pattern, against
Cyrillic data. If it fails, fall back to a stored lowercase column with `LIKE`.

### V4 — DDL, uniqueness, RETURNING, CurrentUtcTimestamp

- Multi-statement DDL in one `Exec`: PASS (two `CREATE TABLE` statements in one `Exec`; Fallback A not needed)
- TTL + inline `GLOBAL UNIQUE SYNC` index in `CREATE TABLE`: PASS
- Recorded error texts below are trimmed. The full wrapper is `pool.With failed with 1 attempts: non-retryable error occurred on attempt No.1 (idempotent=false):` followed by the operation error, and the error is marked non-retryable. Matching on message text is fragile.
- Duplicate unique-index value error: `operation/PRECONDITION_FAILED (code = 400120, issues = [{#2012 'Conflict with existing key'}])`
- Duplicate primary key on `INSERT` error: `operation/PRECONDITION_FAILED (code = 400120, issues = [{#2012 'Conflict with existing key'}])` (identical to the unique-index error, so Plan B cannot tell the two apart by status; map `PRECONDITION_FAILED` with "Conflict with existing key" to "already exists"). Recommended matcher, **to verify in Plan B**: `ydb.IsOperationError(err, Ydb.StatusIds_PRECONDITION_FAILED)` plus YQL issue code 2012. `IsOperationErrorAlreadyExistsError` will not fire for this error.
- `UPSERT ... RETURNING`: PASS (`UPSERT INTO ... VALUES (...) RETURNING count` scans a `Uint64` of 7)
- `CurrentUtcTimestamp()` scan into `time.Time`: PASS (within one minute of the host clock)

Notes for Plan B:
- `CurrentUtcTimestamp() + Interval(...)` is `Optional<Timestamp>`; inserting it into a `NOT NULL Timestamp` column fails with `Failed to convert 'expire_at': Optional<Timestamp> to Timestamp`. Wrap it in `Unwrap(...)`.
- Right after a fresh container start, DDL can fail with `database doesn't have storage pools at all to create tablet channels to storage pool binding by profile id` even though `SELECT 1` already succeeds. Fixed in commit 9114101: the `waitYDBReady` probe is now `CREATE TABLE IF NOT EXISTS dbtest_ready`. The race hit 2 of 2 cold starts on the first DDL.

Decision: in-repo runner, one `Exec` per statement, files split on lines ending in `;`. The spike's one-`Exec`-per-file result covered only two small tables, and the general YDB query-text limit is 10 KB (V5), while the current pg migrations are 59 KB with 45 `CREATE TABLE`s. DDL is not transactional in YDB, so the baseline migration must use `IF NOT EXISTS` / `IF EXISTS` on every statement; the spike did not test the partial-failure path.

### V2 — Terraform provider coverage

Source: `https://raw.githubusercontent.com/yandex-cloud/terraform-provider-yandex/master/docs/resources/<name>.md`; resource index from `https://api.github.com/repos/yandex-cloud/terraform-provider-yandex/contents/docs/resources` (270 entries).

| Resource | Exists | Notes (exact attribute names, gaps) |
|----------|--------|-------------------------------------|
| `function_trigger` | yes | `container` block: `id` (required), `service_account_id`, `path`, `retry_attempts`, `retry_interval`. Trigger sections: `container`, `container_registry`, `data_streams`, `dlq`, `function`, `iot`, `logging`, `mail`, `message_queue`, `object_storage`, `timer`, `workflow`. `data_streams`: `stream_name`, `database`, `service_account_id`, `batch_cutoff` (all required), `batch_size`. `message_queue`: `queue_id`, `service_account_id`, `batch_cutoff` (required), `batch_size`, `visibility_timeout`. The page does not state which trigger types work with which targets (not documented; checked: the function_trigger.md URL above). |
| `ydb_database_serverless` | yes | Exported: `database_path`, `ydb_api_endpoint`, `ydb_full_endpoint`, `document_api_endpoint`, `tls_enabled`, `status`, `created_at`. |
| `ydb_topic` | yes | Needs `database_endpoint` and `name`. Has a `consumer` block (`name` required, `important`, `starting_message_timestamp_ms`, `supported_codecs`), so it can create a consumer. |
| `message_queue` | yes | `redrive_policy` (String), `visibility_timeout_seconds` (0 to 43200, default 30), `arn` (read-only), `fifo_queue`, `message_retention_seconds`, `receive_wait_time_seconds`, `access_key`/`secret_key` (fall back to provider config). |
| `serverless_container` | yes | `concurrency`, `execution_timeout` (duration string), `provision_policy { min_instances }`, `memory`, `cores`, `image { url }`, `connectivity { network_id }`, `secrets { id, version_id, key, environment_variable }` (the page does not name Lockbox in the description; its example takes `id` from `yandex_lockbox_secret`). |
| `api_gateway` | yes | `spec` (required string, OpenAPI), `custom_domains { fqdn, certificate_id, domain_id }`, `connectivity`, `log_options`, `canary`, `execution_timeout`. The page example uses only `type: dummy`; no `x-yc-apigateway-integration` with `serverless_containers` example on the page (not documented; checked: api_gateway.md). |
| `cm_certificate` | yes | `managed` block with `challenge_type` of `DNS_CNAME` (documented as "Recommended for fully automated certificate renewal"), `DNS_TXT` or `HTTP`. Exported `challenges` list: `dns_name`, `dns_type`, `dns_value`, `domain`, `http_content`, `http_url`, `type`, `message`, `created_at`, `updated_at`. Let's Encrypt is not named in what was read. |
| `lockbox_secret` | yes | `name`, `kms_key_id`, `deletion_protection`, `password_payload_specification`, `labels`. |
| `lockbox_secret_version` | yes | `secret_id` (required), `entries { key, text_value or command }`, `description`. A `lockbox_secret_version_hashed` resource also exists. |
| anything matching `postbox` | no | No file with `postbox` in its name in the 270-entry `docs/resources` listing. Related names found: none. |

Decision: Postbox is a documented manual README step (no `postbox` resource exists, so neither a sending identity nor a configuration set with an event destination can be created in Terraform). Triggers that cannot target a container: none found, but the docs do not state target compatibility per trigger type, so the `message_queue`, `timer` and `data_streams` triggers with a `container` target are unverified until a Terraform plan or apply in Plan C. `data_streams` has `database` and `stream_name`, so a YDB topic is plausible but not confirmed by the docs.

### V5 — Serverless YDB limits

| Limit | Value | Source |
|-------|-------|--------|
| Max rows per query result | 1,000 rows, returns the rows with the `truncated` flag set, status SUCCESS. Labelled "Legacy Limits", `ExecuteDataQuery` only; general YDB, not serverless-specific. Not documented for the Query service or for serverless specifically (checked: both URLs below). | https://ydb.tech/docs/en/concepts/limits-ydb |
| Max query result size | 50 MB, query fails with `PRECONDITION_FAILED` and returns no data. Legacy Limits, general YDB. | https://ydb.tech/docs/en/concepts/limits-ydb |
| Max query execution time | 2 hours, error `TIMEOUT`. General YDB. | https://ydb.tech/docs/en/concepts/limits-ydb |
| Max transaction execution time | not documented separately (checked: https://ydb.tech/docs/en/concepts/limits-ydb, https://yandex.cloud/en/docs/ydb/concepts/limits) | n/a |
| Max transaction size or changed rows | not documented (checked: https://ydb.tech/docs/en/concepts/limits-ydb, https://yandex.cloud/en/docs/ydb/concepts/limits). Related: 10,000 lock ranges per DataShard, beyond which locks become whole-shard locks. | https://ydb.tech/docs/en/concepts/limits-ydb |
| Max query text, parameter value, row | query text 10 KB (`BAD_REQUEST`), parameter value 50 MB (`BAD_REQUEST`), single row 50 MB (`PRECONDITION_FAILED`). General YDB. | https://ydb.tech/docs/en/concepts/limits-ydb |
| Serverless RU quota | `ydb.serverlessRequestUnitsPerSecond.count` 1,000 RU per cloud; `ydb.serverlessProvisionedCapacity.count` 1,000 billable RU/s. Quotas, raisable via support. | https://yandex.cloud/en/docs/ydb/concepts/limits |
| Serverless throttling default | "10 RU/s by default when creating a database" (settable, can be zero or disabled). | https://yandex.cloud/en/docs/ydb/concepts/serverless_and_dedicated |
| Behavior when throttling limit exceeded | Query rejected with `Throughput limit exceeded` (rejection is delayed to the next query); new queries pause roughly excess RU divided by the limit; safe to retry; about five minutes of burst bandwidth is reserved. | https://yandex.cloud/en/docs/ydb/concepts/serverless_and_dedicated |
| Provisioned capacity | Optional hourly prepaid RU/s ("Provisioned capacity, RU/s"); billing, not a throttle. | https://yandex.cloud/en/docs/ydb/pricing/serverless |

Decision for Plan B (no port changes, so the "no port-level keyset pagination" non-goal holds):

- Unbounded reads (`RecipientSource.MembersOfList` and `MembersOfSegment`, `internal/campaign/domain/repository.go:122-123`) page **inside the adapter**, at most 1000 rows per request, either with keyset paging internal to the adapter on the primary key or with a streaming `Query()` result.
- Export keeps OFFSET pages of 500 (`exportPageSize`, `ExportWorker.pageAll`) through the existing `domain.Page{Offset, Limit}` port.
- `Recipients.BulkInsert` (`internal/campaign/adapters/recipients_pg.go:30-57`) inserts every recipient in one transaction, and V5 found no documented transaction-size limit. In YDB it writes in chunks, one transaction per chunk of at most 1000 rows. This is safe because the unique `(campaign_id, email)` key makes each chunk idempotent on retry.

The 1,000-row truncation applies to the legacy `ExecuteDataQuery` path and its applicability to `Query().Query` streaming is not documented, so paging stays the safe choice. Retry on `Throughput limit exceeded`.

### V6 — retry-safety audit

Scanned: 39 UpdateFn closures outside pg adapters (the grep listed 40 lines; `internal/dbtest/ydb.go:117` is `context.WithTimeout`, not a closure site, and `internal/auth/app/command/set_locale.go:43` passes no closure); 129 transaction bodies in pg adapters.

No closure passed to a repository `Update*` calls an enqueuer, mailer, verifier or Postbox client, sleeps, or uses a channel, with one exception: the `media.Publish` calls below, which reach the blob store (S3). The enqueues in `StartCampaign`, `ResumeCampaign` and `StartWorker` run after `Update` returns, and the verifier check in `VerifyWorker` runs before it. `MediaRefValidator.IsTenantMediaRef(ref string) bool` takes no context and is pure, so `ApplyVisualSave` inside the save-visual closures is safe. Plain assignments to outer variables (`updated = s`, `res.CurrentUpdatedAt = ...`) are safe: the last run wins. The pg adapter bodies reference only `tx`, locals and pure helpers (the adapters hold only `pool`); there is no injected port, no HTTP/S3/Postbox call, no enqueue, no channel op, and no `slog`/`log` call inside any transaction body.

| Site (file:line) | Class | What repeats on retry | Fix in Plan B |
|------------------|-------|-----------------------|---------------|
| `internal/sending/adapters/verify_worker.go:53` | unsafe-mutation | `snooze = true` is only ever set, never reset. A retry whose second run takes the `!d.IsPending()` early return keeps the stale `true` and snoozes a job whose domain is already terminal. | Set `snooze = false` as the first statement of the closure. |
| `internal/billing/app/command/charge_invoice.go:177` (billing, not ported) | unsafe-mutation, safe-time-inside | `exhausted = true` is only ever set. A retry that sees a lower attempt count keeps the stale `true` and suspends the subscription at line 188. `time.Now()` also feeds `NextAttemptAt`. | Not ported. If ever ported: set `exhausted = false` at closure start; compute `now` before the closure. |
| `internal/billing/app/command/charge_invoice.go:86` (billing, not ported) | safe-time-inside | `s.Cancel(time.Now())` timestamp advances on retry. | Not ported. |
| `internal/billing/app/command/charge_invoice.go:259` (billing, not ported) | safe-time-inside | `i.MarkPaid(time.Now())` timestamp advances on retry. | Not ported. |
| `internal/iam/app/command/sessions.go:152` | safe-time-inside | `s.Revoke(time.Now())` timestamp advances on retry. | Accept: timestamp may advance on retry. |
| `internal/campaign/app/command/set_archive_visibility.go:35` | safe-time-inside | `SetArchiveVisible(..., time.Now())` timestamp advances on retry. | Accept: timestamp may advance on retry. |
| `internal/campaign/app/command/campaigns.go:236-247`, `templates.go:105-111`, `save_visual_template.go:88-96` | external-call-idempotent | `media.Publish` copies the tenant's media to a fixed S3 key and skips assets already published, inside the `Update` closure. On YDB a `DoTx` retry re-runs it. | Accept: the publish is idempotent, so a re-run copies nothing new. |
| `internal/campaign/app/command/campaigns.go:214` | safe-time-inside | `c.Start(time.Now())` timestamp advances on retry. The enqueue is outside the closure. | Accept; keep `EnqueueStart` after `Update` returns. |
| `internal/campaign/adapters/batch_worker.go:258` | safe-time-inside | `c.Finish(time.Now())` timestamp advances on retry. | Accept: timestamp may advance on retry. |
| `internal/audience/adapters/import_worker.go:40` | safe-time-inside | `j.Start(time.Now())` timestamp advances on retry. | Accept: timestamp may advance on retry. |
| `internal/audience/adapters/import_worker.go:49` | safe-time-inside | `j.Complete(..., time.Now())` timestamp advances on retry. | Accept: timestamp may advance on retry. |
| `internal/audience/adapters/export_worker.go:44` | safe-time-inside | `j.Start(time.Now())` timestamp advances on retry. | Accept: timestamp may advance on retry. |
| `internal/audience/adapters/export_worker.go:53` | safe-time-inside | `j.Fail(time.Now())` timestamp advances on retry. | Accept: timestamp may advance on retry. |
| `internal/audience/adapters/export_worker.go:66` | safe-time-inside | `j.Complete(..., time.Now())` timestamp advances on retry. | Accept: timestamp may advance on retry. |
| `internal/auth/app/command/signup.go:87-99` (the `issueVerification` callback passed to `CreateWithVerification`; `CreateWithSession` takes the same kind of `issueSession` callback) | safe (last write wins) | `token.New()` runs again and the outer `rawToken` is overwritten by the last run, which is the run whose hash is persisted. | None; documented on the port. |
| `internal/campaign/adapters/recipients_pg.go:49` (`BulkInsert`) | unsafe-mutation (pg body) | `inserted += tag.RowsAffected()` on an outer counter: a retry double-counts rows the first attempt inserted (the rolled-back attempt's count is not undone). | Declare the counter inside the closure and assign it to the outer variable once at the end, or reset `inserted = 0` at closure start. |
| `internal/audience/adapters/lists_pg.go:165`, `memberships_pg.go:119`, `subscribers_pg.go:205`, `subscribers_pg.go:247`, `subscribers_pg.go:285`, `subscription_pages_pg.go:191`, `internal/billing/adapters/invoices_pg.go:179` (billing, not ported), `internal/campaign/adapters/templates_pg.go:152`, `campaigns_pg.go:201`, `campaigns_pg.go:232`, `campaigns_pg.go:285`, `recipients_pg.go:88`, `internal/deliverability/adapters/suppression_pg.go:118`, `analytics_pg.go:100` (`dash.Recent`), `internal/iam/adapters/audit_pg.go:82`, `roles_pg.go:156`, `apikeys_pg.go:132`, `internal/sending/adapters/domains_pg.go:164`, `domains_pg.go:189`, `internal/media/adapters/assets_pg.go:147` | unsafe-mutation (pg body, read paths) | Each does `append` to a slice declared outside the transaction closure. A retry after a partial read appends the rows again, returning duplicates. | Rewritten in Plan B anyway: build the slice inside the closure and assign it to the outer variable at the end (`result = local`), or reset it (`out = nil`) at closure start. Apply to every YDB read helper that collects rows inside a `DoTx` closure. |

All other sites are `safe`.

Plan B9 note: `YDBAssets.List` (the YDB counterpart of `assets_pg.go:147`) builds its rows in a local and assigns them at the end. The Postgres adapter is unchanged.

### Plan B2 planning spike

Run against `ydbplatform/local-ydb:26.3.1.23` while planning the auth port.

- `SELECT ... FROM t VIEW idx WHERE col = $x` through a `GLOBAL UNIQUE SYNC`
  index: PASS.
- A unique-index conflict on `INSERT` inside `DoTx` surfaces **at the
  statement**, not at commit. `db.IsYDBConflict` is true both for the
  statement error and for the error `DoTx` returns, and the transaction is
  rolled back (no row written). Not tested: continuing the transaction after
  the conflict, which `Recipients.BulkInsert` would need (still to verify in
  the campaign plan).
- `DELETE FROM t ON SELECT id FROM t VIEW idx WHERE ...`: PASS.
- `DELETE ... ON SELECT` followed by `INSERT ... RETURNING created_at` on the
  same table in one `DoTx`: PASS (read after write in one transaction works).
- `UPDATE ... WHERE id = $id RETURNING id`: one row for an existing id, zero
  rows and no error for a missing one.
- `QueryRow` on zero rows: `errors.Is(err, query.ErrNoRows)` is true.

### Plan B3 planning spike

Run against `ydbplatform/local-ydb:26.3.1.23` while planning the tenant port,
on tables with the planned `000004_tenant` columns (`role`, `status`, `name`
and `timezone` are accepted as column names). Five passing runs; one more run
hung and one failed with `No such container` while Ryuk was reaping the
previous run's container (environmental).

- `FROM a AS m JOIN b AS u ON u.id = m.user_id WHERE m.tenant_id = $t`:
  PASS. Joined columns (`u.id, u.email, u.name, m.role`) scan by position.
- `JOIN` with a `VIEW` on the left side
  (`FROM a VIEW idx AS m JOIN b AS t ON t.id = m.tenant_id`): PASS.
- `ORDER BY m.created_at` on a left-side column that is not in the
  projection: PASS, in both joins.
- `UPSERT INTO ... VALUES (...)` twice on one key: PASS (one row, holding the
  second write's values).
- `UPDATE ... WHERE <key> AND <guard> RETURNING id` through `tx.QueryRow`
  inside `DoTx`: one row when the guard matches. When it does not match,
  including for another tenant's id, there are zero rows and
  `errors.Is(err, query.ErrNoRows)` is true. Returning an error from the
  closure after an earlier `INSERT` rolls that `INSERT` back, and `errors.Is`
  matches the closure's error on what `DoTx` returns.
- `VIEW` on a `GLOBAL UNIQUE SYNC` `token_hash` index with extra `status` and
  `expires_at > CurrentUtcTimestamp()` predicates: PASS.
- SELECT-then-INSERT inside `DoTx` for a membership that already exists: no
  conflict, and the existing role is kept.
- Pending-invitation uniqueness without a partial index. Eight concurrent
  `DoTx` closures ran, each doing
  `SELECT ... WHERE tenant_id = $t AND email_key = $k AND status = "pending"u LIMIT 1`
  and an `INSERT` when no row came back, on a table keyed `(tenant_id, id)`.
  In every run exactly one inserted. The other seven were aborted by the
  serializable conflict, retried by the SDK (one retry each), and then saw the
  pending row. No `PRECONDITION_FAILED` occurred, and exactly one pending row
  remained.

### Plan B4 planning spike

Run against `ydbplatform/local-ydb:26.3.1.23` while planning the iam port,
on tables with the planned `000005_iam` columns (`state`, `action`, `target`,
`metadata` and `permissions` are accepted as column names). One run, all
passing. A dry run of the whole plan then passed on the real migration.

- `Bool` column: binding with `.Bool(v)` and scanning into `bool`: PASS.
- Nullable `String` (bytes) column: `DECLARE $x AS Optional<String>;`, bound
  with `BeginOptional().Bytes(p).EndOptional()` where `p` is a `*[]byte`
  (`nil` for NULL). Scanning into `*[]byte` gives `nil` for NULL and
  round-trips bytes, including a NUL byte: PASS.
- Two composite `GLOBAL UNIQUE SYNC` indexes on one table,
  `(tenant_id, platform_user_id)` and `(tenant_id, email)`. A duplicate on
  either fails a standalone `INSERT` with `db.IsYDBConflict` true. `VIEW` on a
  composite index with both columns in the predicate: PASS.
- An `UPDATE` inside `DoTx` that moves a row onto another row's unique-index
  value. The conflict surfaces at the statement. `db.IsYDBConflict` is true
  both for the statement error and for the error `DoTx` returns. The closure
  ran once (no retry), and the row is unchanged. An `UPDATE` that writes the
  row's own current value is not a conflict.
- `DELETE ... WHERE <key> RETURNING id` through `QueryRow`: one row for an
  existing key, `query.ErrNoRows` for a missing one.
- A two-column `JOIN` (`ON r.tenant_id = ur.tenant_id AND r.id = ur.role_id`)
  inside `DoTx`, with the left side filtered on its key prefix: PASS. A role
  that exists only in another tenant is not joined.
- `SELECT COUNT(*)` scans into `uint64`. `ORDER BY created_at DESC LIMIT
  $limit OFFSET $offset`, with both declared `Uint64`: PASS. Limit 2 and
  offset 1 over three rows returned the second and first rows.
- A key-prefix `DELETE ... WHERE tenant_id = $t AND user_id = $u`, then an
  `INSERT` of a just-deleted key in the same `DoTx`: no conflict, one attempt.
- `UPDATE ... WHERE <full key> AND used_at IS NULL RETURNING code_hash`
  outside a transaction: one row on the first call, `query.ErrNoRows` on the
  second.

Decisions for Plan B4:

- iam `users.email` has no `email_key` column. `domain.NewTenantUser` already
  lower-cases the email, and no other code writes it. A `GLOBAL UNIQUE SYNC`
  index on `(tenant_id, email)` therefore reproduces the `citext` uniqueness,
  as `tenants.slug` does. This corrects the B3 handover note.
- `audit_log` is keyed `(tenant_id, created_at, id)`, the order `All` pages
  through. The deliverability context writes to the same table.
- Role assignments read the role inside the assigning `DoTx`, standing in for
  the foreign key, and return `ErrRoleNotFound` when it is missing. The read
  also makes a concurrent `Roles.Delete` abort one of the two transactions.
  The user and the list are not checked: users are never deleted, and lists
  belong to audience.
- V6 iam rows:
  - `audit_pg.go:82`: `YDBAudit.All` collects into a closure-local slice.
  - `roles_pg.go:156` and `apikeys_pg.go:132`: `All` is one statement outside
    `DoTx`, so nothing re-runs.
  - `EffectiveFor` writes its map inside the pg closure; the YDB version
    builds the map locally too.
- iam sessions get TTL on `expires_at`, like `platform_sessions`. An expired
  session already authenticates nothing. One difference is visible: once TTL
  has deleted an expired TOTP-pending session, a request answers
  `ErrUnauthenticated` instead of `ErrTOTPRequired`.

### Plan B5 planning spike

Run against `ydbplatform/local-ydb:26.3.1.23` while planning the audience
core port, on throwaway tables shaped like the planned `000006_audience`
`subscribers` and `subscriber_lists`. One run; one case was then re-run with a
fix (below). A dry run of the whole plan then passed on the real migration.

- `JsonDocument` column: bound with `JSONDocument(string)`, scanned into
  `string` and into `[]byte`. It reads back as normalized JSON (keys sorted).
  `Int32` scans into `int32`: PASS.
- `JSON_VALUE(attributes, "$.keyvalue() ? (@.name == $k).value" PASSING $key AS k)`
  with the key bound as `Utf8`: a string reads back as itself, `42` as `"42"`,
  `42.5` as `"42.5"`, `true` as `"true"`. An object, an array, JSON `null` and
  a missing key read back as NULL. Keys `a.b` and `it's` are matched
  literally: PASS. The same path in `JSON_EXISTS` is true for any present
  key, including one whose value is `null` (as Postgres `?` is): PASS.
- Numeric comparison. `JSON_VALUE(... RETURNING Double)` reads the JSON
  string `"42"` as NULL, unlike Postgres `('42')::numeric`: FAIL. Re-run with
  `CAST(JSON_VALUE(...) AS Double) > CAST($val AS Double)`: numbers and
  numeric strings compare, and a non-numeric value or operand is NULL and
  matches nothing, with no error (Postgres fails the whole query): PASS.
- `ILIKE` on `Utf8` columns with a bound pattern: `%алиса%`, `%АЛИСА%`,
  `%ёжик%` (against `ЁЖИК`), `%ALICE@%` and `%%`; and
  `col ILIKE "%"u || $v || "%"u` with `$v = "ПЕТРОВ"`: PASS. This resolves the
  V1 follow-up. `Unicode::ToLower` is also available.
- `s.id IN (SELECT subscriber_id FROM subscriber_lists WHERE ...)` inside an
  `OR`, in a `COUNT(*)` and in an `ORDER BY s.created_at DESC LIMIT ... OFFSET ...`
  page: PASS. The literal `(true)` is a valid predicate: PASS.
- `FROM subscriber_lists VIEW idx_list AS sl JOIN subscribers AS s ON ...`
  with `ORDER BY s.created_at DESC` (a right-side column) and with
  `COUNT(*)`: PASS.
- `DELETE FROM subscriber_lists ON SELECT ... VIEW idx_list WHERE ...`
  inside `DoTx`, then a read through the index in the same transaction finds
  no row: PASS.

The dry run added:

- Eight concurrent `YDBSubscribers.UpsertByEmail` calls for one new email
  (SELECT through the unique email index, then `INSERT` or `UPDATE`, in one
  `DoTx`): exactly one created the row and none failed, in ten repeated runs.
  The losers were aborted and retried by the SDK; no `PRECONDITION_FAILED`
  surfaced.
- On Postgres, `Subscribers.InList` with a malformed list id fails with
  "commit unexpectedly resulted in rollback": it swallows the invalid-input
  error, but the transaction is already aborted. No caller passes a malformed
  id, so the contract suite uses an unknown valid UUID instead.

Decisions for Plan B5:

- Audience is split in two plans. B5 ports the core: `lists`, `subscribers`
  (with the segment translator), `subscriber_lists` and `subscriber_fields`.
  B6 ports `import_export_jobs`, `subscription_pages` and
  `pending_subscriptions`. B6 needs B5's lists, subscribers and memberships
  for its contract tests; B5 needs nothing from B6.
- `subscribers.email` has no `email_key` column: `domain.NewSubscriber`
  lower-cases it, so the unique `(tenant_id, email)` index reproduces
  `citext`. The segment translator lower-cases an `email` operand in Go,
  because Postgres compares the citext column case-insensitively.
- `subscribers.preference_token_hash` is not ported. No code reads or writes
  it.
- `subscriber_lists` is keyed `(tenant_id, subscriber_id, list_id)`, so a
  subscriber's memberships and the subscriber-delete cascade are key-prefix
  operations. A `GLOBAL SYNC` index on `(tenant_id, list_id)` serves
  `InList`, list-membership segment conditions and the list-delete cascade.
- Foreign keys:
  - `YDBMemberships.Attach` reads the subscriber and the list in its `DoTx`
    and returns `ErrSubscriberNotFound` or `ErrListNotFound`. A concurrent
    delete of either aborts one of the two transactions.
  - `YDBSubscribers.Delete` deletes the subscriber's `subscriber_lists` rows
    in the same `DoTx`.
  - `YDBLists.Delete` deletes the list's `subscriber_lists` rows (through the
    list index) and its iam `user_list_roles` rows (a scan of the tenant's
    rows) in the same `DoTx`. This closes the B4 handover note.
  - Not handled in B5, for the campaign plan: Postgres also cascades a
    subscriber delete into `campaign_recipients` and a list delete into
    `campaign_lists`. Those tables are not on YDB yet. The campaign plan must
    either extend `YDBSubscribers.Delete` and `YDBLists.Delete`, or make its
    readers tolerate a dangling id. Plan B7 extends both.
  - Still open, an iam follow-up: `YDBRoles.AssignListRole` does not read
    the list it assigns a role on. With `lists` now on YDB it should, as
    `YDBMemberships.Attach` does; otherwise an assignment to an unknown or
    concurrently deleted list leaves a `user_list_roles` row that keeps its
    role in use.
- `UpsertByEmail` keeps the existing subscriber's state, as Postgres
  `ON CONFLICT DO UPDATE SET name, attributes` does, so an import never
  re-enables a blocklisted address. A contract test now pins this.
- `subscriber_fields.position` is `Int64` (Postgres `integer`), so the Go
  `int` binds without a narrowing conversion.
- V6 audience rows: `lists_pg.go:165` and `subscribers_pg.go:205/247/285`
  become a closure-local slice in `YDBLists.All` and `YDBSubscribers.page`.
  `memberships_pg.go:119` (`ForSubscriber`) is one statement outside `DoTx`.

### Plan C1 planning spike

Run on 2026-11-15 while planning the job runtime. fakecloud runs as
`ghcr.io/faiscadev/fakecloud:0.50.0`, the client is
`aws-sdk-go-v2/service/sqs` v1.42.27. A dry run of the whole plan then passed:
the `ymq` and worker-route tests, `golangci-lint`, `make lint-arch` and
`go test ./...`.

- **YMQ accepts the AWS JSON protocol.** The YMQ API reference documents only
  the query protocol (form-encoded, XML responses), while every current
  `service/sqs` release sends `application/x-amz-json-1.0`. Unauthenticated
  probes of `https://message-queue.api.cloud.yandex.net/` answer a JSON request
  (`X-Amz-Target: AmazonSQS.ListQueues`) in JSON
  (`{"__type":"AccessDeniedException","message":"IAM authentication error."}`)
  and a query request in XML (`<ErrorResponse>`). So the server parses the
  JSON protocol. A real send is confirmed only by the C4 smoke test.
- **The Go SDK against fakecloud:** `DelaySeconds: 901` fails with
  `InvalidParameterValue` ("Must be between 0 and 900"). A message sent with
  `DelaySeconds: 2` is not received at once and is received after 2 s. With
  `VisibilityTimeout 1` and a redrive policy of `maxReceiveCount "2"`, an
  unacknowledged message is received twice (`ApproximateReceiveCount` 1, then
  2), and the third receive finds it in the DLQ. Queue URLs have the form
  `http://<host>:<port>/123456789012/<name>`, and the queue ARN comes from
  `GetQueueAttributes(QueueArn)`.
- **Trigger request formats**, all from the yandex-cloud/docs sources. Every
  trigger invokes a container with `POST` to the trigger's `container.path`.
  - Message Queue: `{"messages":[{"event_metadata":{...},"details":{"queue_id":
    "...","message":{"message_id":"...","body":"...","attributes":{...},
    "message_attributes":{...}}}}]}`. On success the trigger deletes the
    messages. On an error it returns them to the queue with the visibility
    timeout. Only one trigger may read a queue, and only a standard queue.
  - Timer: `{"messages":[{"event_metadata":{"event_type":
    "yandex.cloud.events.serverless.triggers.TimerMessage",...},"details":
    {"trigger_id":"...","payload":"..."}}]}`. The cron fields are
    `Minutes Hours Day-of-month Month Day-of-week Year`. The handlers ignore
    the body; the path names the sweep.
  - Data Streams: `{"messages":[<message>, ...]}`, where each element is the
    stream message's own JSON. Yandex's `yc-postbox-events` example reads
    `message["eventType"]` and `message["mail"]` straight off each element,
    so each element is one Postbox notification, the same bytes
    `cmd/consumer` reads from the topic.

Decisions for Plan C1:

- The `domain.verify` duplicate-chain fix moves from the worker to the
  serverless sweep (see "YMQ enqueuer").
- The worker routes live in `internal/service` (`WorkerRoutes`), because
  `make lint-arch` forbids `internal/api` from importing YDB or adapter code,
  and `/admin/migrate` needs the YDB driver.
- `ymq.AddWorker(d, w)` mirrors `jobs.AddWorker(workers, w)`, so the C4
  serverless worker registers the same worker values River gets.
- The test helper is `dbtest.SQSEndpoint(t) string`, named like
  `dbtest.YDBDSN`. Tests build their client with the production
  `ymq.NewClient`.

Carried to C3 and C4 (final review of Plan C1):

- **Visibility timeouts.** The short queue's `visibility_timeout_seconds` is at
  least the short job timeout plus a margin, and the long queue's is at least
  the long job timeout plus a margin. Each Message Queue trigger's own
  `visibility_timeout` is left unset or set equal to its queue's.
  **Settled in C3**: `jobs` is 120 s, `jobs-long` is 3900 s, and the trigger's
  own `visibility_timeout` stays unset.
- **Retry budget.** `maxReceiveCount=5` with a fixed visibility timeout gives a
  few minutes of retries; River gives 25 attempts with exponential backoff. C3
  decides the short queue's `maxReceiveCount`. C3 also checks in the YC docs
  whether a throttled invocation (container instance limit, HTTP 429) consumes
  a receive count. If it does, a campaign fan-out can dead-letter jobs that
  never ran.
  **Settled in C3**: `maxReceiveCount` is 20 on `jobs` and 10 on `jobs-long`,
  because a 429 consumes a receive.
- **DLQ alerts.** Alert on the depth of both job DLQs and of the Data Streams
  trigger's DLQ.
- **Feedback recovery.** A dead-lettered `feedback.process` leaves a staged
  inbound event that is never processed, so a bounce or complaint never
  suppresses the address. C4 (or the deliverability Plan B) adds a recovery
  path: a cron sweep that re-enqueues `feedback.process` for staged events left
  unprocessed past a threshold, like the domain sweep. This needs a
  deliverability port method.
  **Settled in Plan B8**: B8 adds the port method
  `EventRepository.PendingInbound` on both backends and the use case
  `SweepPendingFeedbackHandler`; C4 only wires the cron route (see "Plan B8
  planning spike").
- **Instance limits.** Size the worker container's instance limit against the
  campaign fan-out, or give `jobs-long` its own container built from the same
  image.
  **Settled in C3**: instance limits cannot be set per container, so `jobs-long`
  keeps the one `worker` container and sizing is a quota request.
- **IAM.** Bind `serverless.containers.invoker` per container, not
  folder-wide. The API Gateway's service account has no binding on `worker`;
  only the triggers' service account and the deploy step invoke it. Anyone who
  can invoke `worker` can start campaigns, send verification and opt-in
  emails, inject bounces that suppress addresses, and run migrations.
  **Settled in C3**: invoker is bound per container. `gateway` gets it on `api`
  and `frontend`, the trigger service account on `worker` only.
- **Config.**
  - The domain-sweep timer fires no more often than `VerifyInterval`, as
    `cmd/scheduler` does today.
  - `WorkerRoutes.VerifyInterval` is the same `cfg.SendingDomainVerifyInterval`
    passed to `NewVerifyWorker`.
  - `NVELOPE_TENANT_SLUG` is required config in serverless mode.
- **Real-YMQ probe.** As soon as C3 can create a queue, run one SendMessage,
  ReceiveMessage and DeleteMessage against real YMQ with the Go SDK, to confirm
  the JSON protocol before C4. If it fails, the fallback is a query-protocol
  `service/sqs` release built on the current core.

### Plan B6 planning spike

Run on 2026-11-15 against `ydbplatform/local-ydb:26.3.1.23` while planning
the second audience port. A dry run of the whole plan then passed on the real
migration, with `go test ./...`, `golangci-lint run` and `make lint-arch`
clean.

- **Cell size.** A nullable `String` cell round-trips 1, 8 and 16 MiB. 32 MiB
  fails at commit with `BAD_REQUEST`: `Row cell size of 33554432 bytes is
  larger than the allowed threshold 16777216`. The B5 handover assumed the
  50 MB row limit (V5); the cell limit is the tighter one. The SDK's gRPC
  message limit is 64 MB in both directions, so transport is not the limit.
- **Transaction write buffer.** 32 MB split into four 8 MiB rows writes in one
  `DoTx` and reads back intact in one streaming query. 64 MB and more fails
  with `PRECONDITION_FAILED`: `Out of buffer memory. Used 67109248 bytes of
  67108864 bytes`. One transaction therefore writes less than 64 MiB.

Decisions for Plan B6:

- **Job files live in Object Storage.** `import_export_jobs.file_bytes` holds
  imports of up to 32 MB (`maxUploadBytes`) and exports of any size, so it
  cannot be a YDB cell. Chunking it across rows was rejected: one transaction
  still caps a file below 64 MiB. `YDBJobs` keeps the job row in YDB and the
  file as a private object `jobs/<tenant_id>/<job_id>` in the media bucket.
  The bucket is private except `published/*` (media library), so subscriber
  data is never public. The port does not change.
  - The object is written before the row that references it (`AddImport`,
    `StageResult`), so a row does not point at a missing object. When the row
    write fails, the object is deleted best-effort. The one exception is an
    ambiguous insert failure: the commit succeeded but the response was lost,
    and the delete removes the object of a row that exists. That row is
    unreachable (`StartImportHandler` returns the error before enqueueing, and
    the port has no List), and `StagedFile` on it fails loudly.
  - `import_export_jobs.failures` is a `Utf8` cell, capped at 16 MiB. `UpdateImport`
    stores at most the first 10000 failures (`ydbMaxStoredFailures`);
    `failed_count` stays exact. Postgres stores them all.
  - `file_key` is NULL until the job has a file: `StagedFile` then returns
    nil, as Postgres does for a NULL `file_bytes`. A row whose object is gone
    is an error, never an empty file.
  - S3 calls run outside `DoTx` (V6).
  - `dbtest.S3Endpoint` and `dbtest.S3Client` serve the fakecloud S3 container
    to every package. The media blob-store test moves onto them.
- **Subscription pages.** `(tenant_id, slug)` is a `GLOBAL UNIQUE SYNC`
  index. `domain.NewSubscriptionPage` lower-cases the slug, so it matches the
  Postgres `UNIQUE (tenant_id, slug)`. `target_list_ids` and `fields` are JSON
  in `Utf8` columns. The `sending_domain_id` foreign key (`ON DELETE
  RESTRICT`) needs no stand-in: no code deletes a sending domain.
- **Pending subscriptions.**
  - `Upsert` (`ON CONFLICT (tenant_id, email, subscription_page_id) DO
    UPDATE`) reads through a `GLOBAL UNIQUE SYNC` index on those columns, then
    `UPSERT`s the row under the found or a fresh id, in one `DoTx`. Eight
    concurrent upserts of one key return one id.
  - `domain.NewPendingSubscription` lower-cases the email, so the index
    reproduces `citext`.
  - The confirmation-token lookup uses a `GLOBAL UNIQUE SYNC` index on
    `(tenant_id, confirmation_token_hash)`. Postgres makes the hash unique
    globally; a random 256-bit hash makes the difference unobservable.
  - No TTL on `expires_at`: a deleted row would answer an expired link with
    "already confirmed" instead of `ErrConfirmationExpired`.
  - The `subscription_page_id` foreign key needs no stand-in: the only writer,
    `SubmitPublicSubscription`, loads the page first, and no code deletes a
    page.
- **Workers.** The audience worker tests call `Work(ctx, args)` on both
  backends. River dispatch stays covered by the `internal/api` tests.
- **Bugs fixed on both backends** (found in the B5 reviews):
  - `ConfirmSubscriptionHandler` and `ImportWorker.attachToLists` found the
    subscriber with `Search`, a substring match, and could act on a newer
    `malice@example.com` for `alice@example.com`. They now use a new
    `SubscriberRepository.GetByEmail`.
  - Both segment translators formatted a number operand with `%v`, so
    `seats eq 1000000` compared against `1e+06`. `segmentOperand` formats a
    `float64` with `strconv.FormatFloat(v, 'f', -1, 64)`.
  - `YDBRoles.AssignListRole` now reads `lists` in its `DoTx` and returns
    `ErrListNotFound` (new in iam), closing the B5 iam follow-up.
- **For C3 and C4.**
  - C4 wires `NewYDBJobs(driver, s3Client, cfg.ObjectStorageBucket)` and
    builds the client with the options `mediaadapters.NewS3BlobStore` uses
    (path-style, static keys, `cfg.ObjectStorageRegion`). It also wires
    `NewYDBSubscriptionPages` and `NewYDBPendingSubscriptions`.
  - The `api` and `worker` service accounts need read and write on the media
    bucket. Anonymous read covers only the `published/` copies, through the
    per-object `public-read` ACL `NewS3BlobStore.Publish` sets. The bucket has
    no bucket policy (Plan C3).
  - **C3.** Bucket-wide anonymous read (the YC bucket's
    `anonymous_access_flags.read`) stays off, because `jobs/` holds subscriber
    PII. The C4 smoke test checks that an anonymous GET on a `jobs/` key
    answers 403.
  - **C4.** The S3 client for `NewYDBJobs` must copy
    `mediaadapters.NewS3BlobStore`'s options, including the `Region: "auto"`
    fallback when `cfg.ObjectStorageRegion` is empty.

### Plan B7 planning spike

Run on 2026-11-15 against `ydbplatform/local-ydb:26.3.1.23` while planning the
campaign port, on throwaway tables shaped like the planned `000008_campaign`
`campaign_recipients` and `campaign_lists`. A dry run of the whole plan then
passed on the real migration, with `go test ./...`, `golangci-lint run` and
`make lint-arch` clean.

- **Bulk insert from a parameter.** `INSERT INTO t (...) SELECT $tenant_id,
  $campaign_id, id, subscriber_id, email, "pending"u FROM AS_TABLE($rows)`, with
  `$rows AS List<Struct<id: Utf8, subscriber_id: Utf8, email: Utf8>>` bound as
  `types.ListValue(types.StructValue(types.StructFieldValue(...)...)...)`:
  PASS. `ydb.ParamsBuilder()` has no list-of-struct form; `Param(name).Any(v)`
  takes the `types` value.
- **Conflicts in a bulk insert.**
  - A batch that repeats a key fails whole: `PRECONDITION_FAILED`, issue 2012
    `Duplicated keys found`. `db.IsYDBConflict` is true for it.
  - A row whose key exists fails whole: `PRECONDITION_FAILED`, issue 2012
    `Conflict with existing key`.
  - **Continuing after a conflict (the open B2 item): FAIL.** In one `DoTx`, a
    conflicting `INSERT` returns the conflict, and the next statement fails with
    `NOT_FOUND`, `Transaction not found: <id>`. The conflict already rolled the
    transaction back.
- `SELECT ... FROM t VIEW idx_email WHERE tenant_id = $t AND campaign_id = $c
  AND email IN $emails`, with `$emails AS List<Utf8>`, through a `GLOBAL UNIQUE
  SYNC` index: PASS. `IN` takes a bound list.
- `UPDATE t ON SELECT tenant_id, campaign_id, id, "sent"u AS status,
  CurrentUtcTimestamp() AS sent_at FROM t VIEW idx_id WHERE tenant_id = $t AND
  id = $id`, where the index is on `(tenant_id, id)` and the primary key is
  `(tenant_id, campaign_id, id)`: PASS. An id that matches no row updates
  nothing and returns no error.
- `SELECT ... WHERE tenant_id = $t AND campaign_id = $c ORDER BY id LIMIT $l
  OFFSET $o` over a primary-key prefix, and `SELECT status, COUNT(*) ... GROUP
  BY status`: PASS.
- A `GLOBAL UNIQUE SYNC` index on `(tenant_id, campaign_id, list_id)` with a
  nullable `list_id`: two rows with a NULL `list_id` are accepted, and a
  repeated non-NULL `list_id` conflicts. This is the Postgres `UNIQUE
  (campaign_id, list_id)`, whose NULLs are distinct too.
- `DELETE FROM t ON SELECT <key> FROM t VIEW idx WHERE tenant_id = $t AND
  subscriber_id = $s`, and `UPDATE t SET col = NULL WHERE tenant_id = $t AND
  other IS NULL`: PASS.

The dry run added:

- `BulkInsert` of 2300 recipients with repeats across the 1000-row chunk
  boundary inserts 2300 rows, on both backends. Six concurrent `BulkInsert`s of
  the same 40 emails insert each email once and none fails (three repeated
  runs). Six concurrent `UpsertLinks` of the same URLs return identical ids.
- Sabotage checks: with the existence read removed, the dedup test fails; with
  the template unlink, the list-target delete or the recipient delete removed,
  its test fails.

Decisions for Plan B7:

- **Scope.** `000008_campaign` creates the eight tables the campaign ports
  own: `templates`, `campaigns`, `campaign_lists`, `campaign_recipients`,
  `links`, `link_clicks`, `campaign_views`, `transactional_messages`.
  `campaign_analytics` and `delivery_events` belong to deliverability (B8).
- **No CHECK constraints, no defaults.** The domain keeps `status`, `kind` and
  the one-of-list-or-segment rule valid. Every `INSERT` writes every `NOT NULL`
  column. Counts and `max_send_errors` are `Int64` so a Go `int` binds without
  a narrowing conversion. `body_doc` and `theme` are JSON stored and read back
  whole, so plain nullable `Utf8` (Postgres `jsonb` normalizes key order and
  spacing). `normalizeRawJSON` only trims whitespace and maps `null` to nil, so
  YDB returns `body_doc`, `theme` and `segment_query` byte-for-byte as written
  and Postgres its canonical `jsonb` form; no Go code compares the bytes.
- **Keys.**
  - `templates`, `campaigns`, `links` and the two event tables: `(tenant_id,
    id)`. `templates` has a unique `(tenant_id, name)` index, as Postgres has.
  - `campaign_recipients`: `(tenant_id, campaign_id, id)`, so a campaign's
    recipients are one key range in id order, which the batch window reads. The
    send worker and the tracking endpoints know a recipient by id alone, so
    `GLOBAL SYNC (tenant_id, id)` finds it; the writes go `UPDATE ON` through
    it. A unique `(tenant_id, campaign_id, email)` index reproduces
    `UNIQUE (campaign_id, email)`.
  - `campaign_lists`: `(tenant_id, campaign_id, id)` with a generated id,
    because a segment target has no natural key. Unique `(tenant_id,
    campaign_id, list_id)` reproduces `UNIQUE (campaign_id, list_id)`.
- **`Recipients.BulkInsert`** (the open B2/V5 item). Dedupe the input by email
  in Go (the first wins), then per chunk of 1000: one `DoTx` that reads the
  emails the campaign already has and inserts the rest with `AS_TABLE`. A
  concurrent insert of the same email aborts one transaction and the SDK
  retries it. The ids are generated before the `DoTx`, the count is assigned at
  its end (V6). The returned count can fall short only when a first attempt
  committed and its response was lost.
- **`Recipients.Pending`** keeps the Postgres window: `ORDER BY id LIMIT
  OFFSET` over every recipient of the campaign, then the pending rows of it,
  read in chunks of 1000 inside the adapter. Pending is filtered in Go, so a
  window never shifts as rows are sent.
- **The Postgres foreign keys.**
  - Stand-in reads, as `YDBMemberships.Attach` does: `YDBRecipients.BulkInsert`
    reads the chunk's subscribers in its `DoTx` and fails if one is missing,
    and `YDBCampaigns.SaveTargets` reads the targeted lists in its `DoTx` and
    fails if one is missing. A concurrent `Subscribers.Delete` or list delete
    then conflicts with the insert, so a start job racing a delete cannot mail
    a deleted subscriber.
  - `YDBLists.Delete` also deletes the campaign's `campaign_lists` rows for the
    list, through `idx_campaign_lists_list`. Without it a campaign would keep
    a target that resolves nothing, or fails.
  - `YDBSubscribers.Delete` also deletes the subscriber's `campaign_recipients`
    rows, through `idx_campaign_recipients_subscriber`. The alternative, letting
    readers tolerate a dangling id, was rejected: an erased subscriber's address
    would stay in the campaign and a running campaign would still mail it. The
    `link_clicks` and `campaign_views` rows of those recipients stay: no query
    reads them, so they carry no index.
  - `YDBTemplates.Delete` sets `campaigns.template_id` to NULL, by a scan of
    the tenant's campaigns (a ceiling). `transactional_messages.template_id` is
    only written, so it is left dangling.
  - `campaigns.sending_domain_id` (`ON DELETE SET NULL`) needs nothing: no code
    deletes a sending domain.
  - `campaigns.Update` never writes `template_id`, as the Postgres `UPDATE`
    does not.
- **Tracking lookups without a tenant.** `ResolveTenantForLink` and
  `ResolveTenantForCampaign` read `idx_links_id` and `idx_campaigns_id`,
  `GLOBAL UNIQUE SYNC` indexes on `(id)`. They stand in for the two `SECURITY
  DEFINER` functions. `UpsertLinks` reads the URLs the campaign has and inserts
  the others, because an `INSERT` onto an existing key aborts the transaction.
  `RecordClick` and `RecordView` read the recipient through the id index and
  record nothing unless it belongs to the link's campaign.
- **Indexes for B8, created here.** The recipients and transactional tables
  are written by this plan's migration, so it also creates what delivery
  feedback reads: `GLOBAL SYNC (provider_message_id)` on `campaign_recipients`
  and `GLOBAL UNIQUE SYNC (provider_message_id)` on `transactional_messages`.
  A later `ALTER TABLE ... ADD INDEX` would have to sit alone in its own file.
- **Tests.** `templates`, `campaigns`, archive, `recipients`, `tracking` and
  `transactional` tests become pg+ydb contract suites through an `eachBackend`
  harness, as in audience. The send-pipeline and resumability tests (the start
  and batch workers) run on both backends. The two start-worker quota tests
  stay on Postgres: billing is not ported. The two cascades into audience's
  adapters are tested in the audience package, on YDB only, with raw SQL, as
  B5's `user_list_roles` test does.
- **A V6 fix on Postgres.** `Recipients.BulkInsert`'s `inserted` counter is
  reset at the start of its closure (the V6 table lists it). No other Postgres
  adapter changes.
- **Found by the B7 review, fixed after B9 (both backends).** `StartWorker.Work`
  set a campaign's `recipient_count` and fanned out batches from
  `sent + failed + pending`, which left out `skipped` recipients, while the
  `Pending` window spans every row. A campaign resumed after some recipients were
  skipped could fan out too few batches, leave its last recipients unselected,
  and never finish. `RecipientRepository.Counts` now also returns `skipped`, and
  the start worker counts every row (`TestStartWorkerResumeCoversSkippedRecipients`).
- **For B8, B9 and C4.**
  - **B8.** Attribution resolves a provider message id with no tenant bound:
    read `campaign_recipients VIEW idx_campaign_recipients_provider` and
    `transactional_messages VIEW idx_transactional_messages_provider`; each
    entry carries `tenant_id`. With a tenant, add `AND tenant_id = $t` (the
    index's second key component filters). Analytics reads `campaign_recipients`
    by the `(tenant_id, campaign_id)` key range and `campaigns` by `(tenant_id,
    id)`. `delivery_events` carries `campaign_id` and `campaign_recipient_id`
    with `ON DELETE SET NULL` on Postgres; here a campaign's recipients now go
    when their subscriber is deleted, so B8 must tolerate or null a dangling
    `campaign_recipient_id`. `campaign_recipients.status` has four values
    (`pending`, `sent`, `failed`, `skipped`).
  - **B9.** Nothing: campaigns reference media by ref string only, validated in
    the domain.
  - **C4.** Wire `NewYDBTemplates`, `NewYDBCampaigns`, `NewYDBRecipients`,
    `NewYDBTracking` and `NewYDBTransactionalMessages`, each over the
    `*ydb.Driver`, and the already-ported `NewYDBRateLimiter` for the batch
    worker. `BatchWorker.Work` and `StartWorker.Work` take them through their
    ports, unchanged. The start worker's `recipients.BulkInsert` writes in
    chunks of 1000 rows, each its own transaction: a campaign.start job that
    dies halfway re-runs and inserts only the rest.

### Plan B8 planning spike

Run on 2026-11-15 against `ydbplatform/local-ydb:26.3.1.23` while planning the
deliverability port, on the real `000009_deliverability` and the `000008`
campaign tables. A dry run of the whole plan then passed stage by stage, with
`go build`, `go vet`, `golangci-lint run`, `make lint-arch` and `go test ./...`
(49 `ok` packages) clean.

- **Refresh as one `UPSERT ... SELECT`.** Tested on 3 campaigns (one a draft),
  4 recipients (3 of one campaign, one of them `failed`) and 6 delivery events
  (one of another tenant): PASS, and a second run returns the same rows.
  - Shape: `$sent = SELECT campaign_id, COUNT(*) AS n FROM campaign_recipients
    WHERE tenant_id = $t AND status = "sent"u GROUP BY campaign_id;` and
    `$ev = SELECT campaign_id, COUNT(DISTINCT IF(event_kind = "delivery"u,
    recipient_email)) AS delivered, ... GROUP BY campaign_id;`, then `UPSERT
    INTO campaign_analytics (...) SELECT c.tenant_id AS tenant_id, c.id AS
    campaign_id, Unwrap(CAST(COALESCE(s.n, 0ul) AS Int64)) AS sent_count, ...
    FROM campaigns AS c LEFT JOIN $sent AS s ON s.campaign_id = c.id LEFT JOIN
    $ev AS e ON e.campaign_id = c.id WHERE c.tenant_id = $t AND c.status !=
    "draft"u`.
  - Result: the campaign with 2 sent recipients and events `delivery` x3 (two
    distinct addresses), `open`, `bounce` got sent 2, delivered 2, opened 1,
    bounced 1; the other tenant's `click` was not counted; the second campaign
    got sent 1 and zeros; the draft got no row.
  - What failed first: without `AS tenant_id, AS campaign_id` the statement is
    `BAD_REQUEST` ("Column names in SELECT don't match column specification");
    `CAST(COALESCE(n, 0) AS Int64)` is `Optional<Int64>` and fails with
    "Can't set NULL or optional value to not null column", which `0ul` and
    `Unwrap(...)` fix.
- **Totals and the recent list.** `SELECT COALESCE(SUM(sent_count), 0), ...
  FROM campaign_analytics WHERE tenant_id = $t` returns `0` for a tenant with
  no rows. `SELECT ... FROM campaign_analytics AS ca JOIN campaigns AS c ON
  c.tenant_id = ca.tenant_id AND c.id = ca.campaign_id WHERE ca.tenant_id = $t
  ORDER BY COALESCE(c.started_at, c.created_at) DESC LIMIT 10`: PASS, newest
  first.
- **Scanning.** An `Int64` column does not scan into `*int` (`cast failed
  'Int64(2)' to '*int' destination`); scan into `int64` and convert.
- **Pending sweep read.** `SELECT id FROM inbound_feedback_events VIEW
  idx_inbound_status WHERE status = "pending"u AND received_at < $before ORDER
  BY received_at LIMIT $limit` over `GLOBAL SYNC (status, received_at)`: PASS,
  only the rows older than the cutoff, oldest first.
- **`Interval`** takes a string literal, not an expression
  (`Interval("PT"u || ...)` fails with "Expected literal string as argument").
  Nothing in the port needs it.

The dry run added: `StageInbound` and `RecordEvent` each pass a six-goroutine
race on both backends (three repeated runs); with the existence read removed,
the idempotence and race tests fail on YDB; a Cyrillic `ILIKE` filter on the
suppression list passes on Postgres and YDB.

Decisions for Plan B8:

- **Scope.** `000009_deliverability` creates `inbound_feedback_events`,
  `delivery_events`, `suppression_list`, `bounce_settings` and
  `campaign_analytics`. `audit_log` is iam's (000005); `YDBAuditLog` writes to
  it, as the Postgres `AuditLog` does. `transactional_messages` is B7's.
- **No CHECK constraints, no defaults, no foreign keys**, as B7.
- **Keys.**
  - `inbound_feedback_events`: `(id)`, the one table here without a tenant,
    because a notification is staged before its tenant is known. `GLOBAL UNIQUE
    SYNC (dedupe_key)` is the idempotency key. `raw_payload` is a plain `Utf8`
    column (JSON stored and read back whole).
  - `delivery_events`: `(tenant_id, id)`, plus `GLOBAL UNIQUE SYNC (tenant_id,
    inbound_event_id)` for `RecordEvent`. The Postgres indexes
    `(campaign_id, event_kind)` and `(tenant_id, recipient_email)` are not
    created: the refresh groups the tenant's events by one key-range scan, and
    nothing reads by recipient.
  - `suppression_list`: `(tenant_id, id)`, because `List` pages in id order,
    plus `GLOBAL UNIQUE SYNC (tenant_id, email)`, which `Upsert`, `Remove` and
    the pre-send check read.
  - `bounce_settings`: `(tenant_id)`. `campaign_analytics`: `(tenant_id,
    campaign_id)`; Postgres keyed it by `campaign_id` alone.
- **Retry-safe by construction (V6).** The YDB `List`, `GetDashboard` and
  `PendingInbound` build their rows in a local and assign them at the end, which
  covers the `analytics_pg.go` `dash.Recent` and `suppression_pg.go` `List`
  sites for the YDB runtime. The Postgres adapters do not change (River does not
  retry a pgx transaction).
- **Dangling ids.** `delivery_events.campaign_recipient_id` is written and
  never read back, so a recipient deleted with its subscriber (B7) leaves a
  dangling id that nothing follows. `campaign_id` is read: the refresh groups
  the events by it and joins them to `campaigns`, so an event of an unknown
  campaign counts nowhere. No campaign delete exists, so no `campaign_analytics`
  cascade is needed. A campaign's `sent_count` is the count of its `sent`
  recipients, so it falls when a subscriber's deletion removes recipients (B7).
  On Postgres the same delete cascades to `campaign_recipients`, and
  `delivery_events.campaign_recipient_id ON DELETE SET NULL` clears the id. That
  used to violate `CHECK ((campaign_recipient_id IS NOT NULL) <>
  (transactional_message_id IS NOT NULL))`, so the delete failed. Migration
  `000025_delivery_event_source_check` relaxes the check to "at most one
  source", so an event outlives its deleted source on both backends.
- **Feedback recovery (the C1 carry-over).** B8 owns the port, so it adds
  `EventRepository.PendingInbound(ctx, receivedBefore, limit)` on both backends
  (Postgres: the existing `status` and `received_at` indexes) and
  `command.SweepPendingFeedbackHandler`, which enqueues `feedback.process` for
  up to 500 notifications pending past `OlderThan`. A job that is only slow runs
  twice, which `ProcessFeedback` tolerates. C4 wires the cron route. This is
  small, and leaving the port to C4 would put a port change into the assembly
  plan.
- **`ProcessFeedback` changes by one line** (the final review found that a
  failure inside `Apply` after `RecordEvent` committed was never retried; the
  `recorded &&` guard is dropped), and `cmd/consumer` stays a SaaS binary.
  The `/feedback` route already stages through `IngestNotificationHandler`,
  which takes the `EventRepository` port.
- **Tests.** The suppression, settings, checker, audit, events, feedback-worker
  and analytics tests become pg+ydb contract suites through an `eachBackend`
  harness (`backend_test.go`), as in campaign. `stageInbound` retires the
  notifications a test leaves pending, so the shared database does not pile
  them up for the `PendingInbound` test.
- **Found by the B8 review, fixed after B9.**
  - Both backends: `SuppressionApplier.Apply` passed `e.ID()` as the entry's
    source event id, but nothing set it, so `suppression_list.source_event_id`
    was always NULL. `RecordEvent` now returns the row id (`(id string, err
    error)`; the unused `recorded` flag is gone). On a retry that is the row an
    earlier attempt wrote. `ProcessFeedback` hands `event.Recorded(id)` to the
    applier. Postgres reads the existing id with a second statement after
    `ON CONFLICT DO NOTHING RETURNING id` returns no row.
  - Postgres only (an existing SaaS bug): deleting a subscriber that had any
    campaign delivery event failed on the CHECK above. Fixed by migration
    `000025` (see "Dangling ids"), pinned by
    `TestSubscribersDeleteKeepsDeliveryEvents`.
- **For B9 and C4.**
  - **B9.** Nothing: no deliverability table references media, and media owns
    no table deliverability reads. The next migration number is `000010`.
  - **C4.**
    - Wire `NewYDBEvents`, `NewYDBSuppressions`, `NewYDBSettings`,
      `NewYDBAnalytics`, `NewYDBSuppressionChecker` and `NewYDBAuditLog`, each
      over the `*ydb.Driver`. `NewYDBSuppressionChecker` is what the campaign
      start and batch workers take as their `SuppressionChecker`, and also
      what `SendTransactional` (`internal/service/application.go` ~518) and
      `ConfirmSubscription` (~716, ~758) take;
      `NewSuppressionApplier` takes the two YDB repositories unchanged;
      `NewProcessFeedbackHandler` and `NewIngestNotificationHandler` take
      `NewYDBEvents`.
    - Add `POST /cron/feedback-sweep` to `WorkerRoutes`: it runs
      `command.NewSweepPendingFeedbackHandler(events, enqueuer).Handle(ctx,
      SweepPendingFeedback{OlderThan: ...})`, where `enqueuer` is the `*ymq.Enqueuer`
      (it implements `EnqueueFeedbackProcess`). `WorkerRoutes` needs widening for
      it: `cronEnqueuer` (`internal/service/worker_routes.go` ~22) lacks
      `EnqueueFeedbackProcess`, `WorkerRoutes` has no `EventRepository` or
      sweep field, and the test `recordingEnqueuer` needs the method. Pick
      `OlderThan` above the worst-case queue latency, not only the feedback
      job's visibility timeout plus its retry window (otherwise a throttled
      backlog makes every tick add up to 500 duplicate jobs), for example 15
      minutes, and fire the timer about as often. It needs no tenant: the
      staging table has none. Add the matching timer trigger to the Terraform
      module (C3), and extend the DLQ alert note: a pending notification older
      than the threshold is the symptom.
    - `cmd/consumer` is not used in serverless mode.

### Plan B9 planning spike

Run on 2026-11-15 against `ydbplatform/local-ydb:26.3.1.23` while planning the
media port, on the real `000010_media` table. The adapter and the contract
suite of the plan were applied to the worktree and run; a dry run of the whole
plan then passed stage by stage (see the plan).

- **No new YQL construct.** Every statement is one B1 to B8 already verified:
  a keyed `SELECT`, `INSERT` with `CurrentUtcTimestamp()`, `DELETE ... RETURNING
  id` (not-found on no row, as `YDBTemplates.Delete`), `UPDATE` by primary key,
  and `ORDER BY created_at DESC, id DESC` over the tenant's key range. Only
  the `Int32` binding (from `int32`) is new; the `Timestamp` binding
  (`invitations_ydb.go`) and `Optional<Utf8>` from a `*string` (`jobs_ydb.go`,
  `campaigns_ydb.go`) already exist. It worked on the first run, and the
  nullable `Timestamp` and `Utf8` columns scan into `*time.Time` and `*string`.
- **The suite passes on both backends.** Add/Get/List/Delete, unknown and
  non-UUID ids, tenant scoping (a second tenant's `Get`, `Delete`,
  `MarkPublished` and `List` see nothing), newest-first order, `MarkPublished`
  twice, a PDF with no dimensions, and the uploader id (YDB only, because on
  Postgres it is a foreign key to `users`). Sabotage: with the `tenant_id`
  predicate removed from `Get` and from the `MarkPublished` read, the tenant
  scoping tests fail on YDB.
- **Race.** Six concurrent `MarkPublished` calls on one asset beside six
  concurrent `Add`s, three runs: no error, seven rows, the asset published, the
  list in non-increasing `created_at` order.
- **Use cases over both repositories** (`PublishAssetsHandler`,
  `DeleteAssetHandler`, with the in-memory blob store): publishing twice copies
  once (the second run has the blob store fail on any copy) and returns the
  same text, which is the V6 retry argument for the three campaign `Update`
  closures; another tenant's asset is `ErrMediaMissing` and nothing is copied;
  deleting keeps the public copy by default and removes it with `Unpublish`.

Decisions for Plan B9:

- **One table, `media_assets`, in `000010_media`.** Key `(tenant_id, id)`; no
  secondary index. Every read is by key or the tenant's range, and a library is
  hundreds of rows. No CHECK, DEFAULT or foreign key, as B7.
- **`width` and `height` are `NOT NULL Int32`, `0` for a PDF.** Postgres stores
  NULL because of `CHECK (> 0)` and reads it back through `COALESCE(.., 0)`;
  with no CHECKs on YDB the sentinel is the simpler column. `uploaded_by` and
  `published_at` stay nullable.
- **`List` orders by `created_at DESC, id DESC` in YQL.** The tie-break makes
  the order total; Postgres has none. Ceiling recorded.
- **`MarkPublished` is read, then write, in one `DoTx`** (the pg adapter's
  `COALESCE(published_at, $2)` plus a rows-affected check). It returns
  `ErrMediaNotFound` for a missing row.
- **`Delete` is a single `DELETE ... RETURNING id`.** The handler already
  removes the public copy and the private object around it, in this order, so a
  failed object delete leaves an orphan and never a row that points nowhere.
- **The blob store is unchanged.** Both runtimes use `S3BlobStore`. The media
  repository makes no S3 call inside its own `DoTx`; the campaign `Update`
  closures' publish copy is the V6 exception, which `TestPublishAssetsIsRepeatable`
  covers.
  `MemoryBlobStore` keeps serving the use-case tests.
- **User decisions of the media library are untouched.** Deleting an asset
  keeps its public copy unless `Unpublish` is set; no SVG; library URLs stay on
  the app origin with a 302 to a presigned URL. Nothing here reaches the HTTP
  layer.
- **No cascade to keep.** `uploaded_by REFERENCES users ON DELETE SET NULL` and
  `tenant_id REFERENCES tenants ON DELETE CASCADE` have no YDB counterpart
  because no code deletes a user or a tenant (checked: no `DELETE FROM users`,
  `tenants` or `platform_users` outside tests and migrations). Recorded in
  "Ceilings".
- **Callers checked.** The port is used by `UploadAsset` (`Add`),
  `PublishAssets` (`Get`, `MarkPublished`), `DeleteAsset` (`Get`, `Delete`),
  `ListAssets` (`List`) and `AssetContentURL` (`Get`), all in
  `internal/media/app`. `campaign` and `tenant` (branding) reach it only through
  the `PublishAssets` handler, so the port covers every read and write path.
  There are no media workers.

Resolved by C3 (merged after this plan):

- `S3BlobStore.Publish` sends `ACL: public-read` on the copy. That is the access
  model C3 kept: the bucket has no bucket policy, and anonymous read comes only
  from the per-object ACL. C3 grants the app service account `storage.editor`
  on the bucket, because `storage.uploader` cannot set that ACL. The B9 worry
  that the ACL was redundant and might lack a role is therefore resolved.
  Only C4's smoke test, which publishes once through the real service account,
  confirms it against Object Storage.

For C4 and the completeness of Plan B:

- **Wire** `mediaadapters.NewYDBAssets(driver)` in place of `NewAssets(pool)` in
  `buildMedia` (`internal/service/application.go` ~266). The blob store is the
  existing `NewS3BlobStore` with `cfg.ObjectStorage*`, as in SaaS; the
  `Region: "auto"` fallback and path-style addressing are inside it, and
  `NewYDBJobs`' S3 client copies them (B6).
- **Plan B is complete for every in-scope context.** Every repository constructor
  the composition root wires has a YDB version: sending (`NewYDBSendingDomains`),
  auth (`NewYDBUsers`, `NewYDBSessions`, `NewYDBEmailVerifications`), tenant
  (`NewYDBTenants`, `NewYDBInvitations`, `NewYDBSettings`, `NewYDBBranding`),
  iam (`NewYDBUsers`, `NewYDBSessions`, `NewYDBRoles`, `NewYDBAPIKeys`,
  `NewYDBRecoveryCodes`, `NewYDBAudit`), audience (`NewYDBLists`, `Subscribers`,
  `Memberships`, `Fields`, `Jobs`, `SubscriptionPages`, `PendingSubscriptions`),
  campaign (`NewYDBCampaigns`, `Templates`, `Recipients`, `Tracking`,
  `TransactionalMessages`, `RateLimiter`), deliverability (`NewYDBEvents`,
  `Suppressions`, `Settings`, `Analytics`, `SuppressionChecker`, `AuditLog`) and
  media (`NewYDBAssets`). The service-level `onboarding` has `newYDBOnboarding`.
- **What is still pg-, Redis- or River-bound, and is C4's wiring, not a port:**
  - billing (`billingadapters.*`, the `billing` app and `api/billing_handlers.go`):
    not ported. `buildCampaign` builds `usageRecorder` and `quotaGate` from
    billing pg adapters unconditionally (~491); the serverless builder passes
    `nil` (`NewSendTransactionalHandler` accepts them, as the campaign workers
    do) and leaves the billing routes unmounted.
  - `audienceadapters.NewSubmissionThrottle` and `authadapters.NewResendThrottle`
    take a Redis URL: the serverless builder uses `ydbthrottle.Counter`
    (see "Throttling"). `campaignadapters.NewRateLimiter` likewise has
    `NewYDBRateLimiter`.
  - The River-based workers (`Import`, `Export`, `Optin`, `Start`, `Batch`,
    `Verify`, `Feedback`, `Analytics`, and `authadapters.NewVerificationWorker`
    over `UserRepository`) are generic over the ports and take the
    YDB repositories; `authadapters.NewVerificationCleanupWorker` is not needed
    (table TTL). `cmd/scheduler`'s `sweepPendingDomains`,
    `enqueueAnalyticsRefresh` and `enqueueUsageRollup` read pg directly: the
    `/cron/domain-verify-sweep` and `/cron/analytics-refresh` routes that replace
    the first two were built in C1 (`internal/service/worker_routes.go`). Only the
    billing usage-rollup has no route (billing is not wired); B8's
    `/cron/feedback-sweep` route and its timer remain for C4/C3.
  - `iamadapters.NewTOTP`, `authadapters.NewPasswordHasher` and
    `campaignadapters.NewPostboxMessenger` hold no storage, as do
    `deliverabilityadapters.NewSuppressionApplier`,
    `campaignadapters.NewUnsubscribeLinker` and
    `sendingadapters.NewPostboxProvisioner` (generic over ports).
    `NewNotificationParser` and `NewStreamReader` are used only by `cmd/consumer`
    and are unused in serverless mode.
  - `platform/jobs` (River) and `platform/tenantdb` are pg-only by design and
    stay out of the serverless builder.
### Plan C3 planning spike

Run on 2026-11-15 while planning the Terraform module and image publishing.
Provider `yandex-cloud/yandex` v0.248.0 (from `terraform providers schema
-json`), Terraform v1.16.1. A draft of the whole module passed
`terraform validate` and `terraform test` with mock providers. A
`terraform plan` against a real folder (read-only, nothing applied) planned 41
resources with `certificate_id` and 44 with `dns_zone_id`. Docs are the
`yandex-cloud/docs` sources (`en/`).

- **Provider arguments** (resolves the V2 and "to verify in Plan C" items):
  - `yandex_ydb_database_serverless.serverless_database`:
    `enable_throttling_rcu_limit` (bool), `throttling_rcu_limit`,
    `provisioned_rcu_limit`, `storage_size_limit`. `database_path` and
    `ydb_full_endpoint` are exported.
  - `yandex_function_trigger`: a `container` block (`id`, `path`,
    `service_account_id`, `retry_attempts`, `retry_interval`) beside
    `message_queue` (`queue_id` = the queue ARN, `batch_size`,
    `batch_cutoff`, `visibility_timeout`), `timer` (`cron_expression`,
    `payload`), `data_streams` (`database` = the database path,
    `stream_name`, `batch_size` in bytes up to 64 KB, `batch_cutoff`) and
    `dlq` (`queue_id` = ARN). The docs say the retry settings apply to every
    trigger type except Message Queue, which relies on the queue's redrive
    policy.
  - `yandex_message_queue.id` is the queue URL (provider source:
    `d.SetId(output.QueueUrl)`). Creating a queue takes a static key
    (`access_key`/`secret_key`).
  - `yandex_serverless_container`: `concurrency`, `execution_timeout`,
    `provision_policy { min_instances }`, `secrets { id, version_id, key,
    environment_variable }`. There is **no per-container instance limit**.
  - `yandex_serverless_container_iam_member` binds a role on one container.
    YMQ, Postbox and YDS roles bind only at folder or cloud level (YMQ docs).
  - `yandex_api_gateway.custom_domains { fqdn, certificate_id }`; the
    `domain` attribute is the default `*.apigw.yandexcloud.net` domain.
  - `yandex_cm_certificate` (`managed { challenge_type }`, exported
    `challenges`) plus `data "yandex_cm_certificate" { wait_validation = true }`.
  - `yandex_iam_service_account_static_access_key`, `yandex_lockbox_secret`,
    `yandex_lockbox_secret_version`, `yandex_lockbox_secret_iam_member`,
    `yandex_ydb_database_iam_member`, `yandex_storage_bucket_iam_binding`,
    `yandex_ydb_topic` (with a `consumer` block): all present.
  - **No alerting resource.** Only `yandex_monitoring_dashboard` exists, so DLQ
    alerts are a README step.
- **Containers strip `Cookie` and `Authorization` on direct HTTPS calls**
  (`serverless-containers/concepts/invoke.md`, "Filtering message headers").
  The BFF forwards the session cookie to `${NV_GO_API_URL}/t/:slug/api/...`.
  That is the same path the public gateway routes to `frontend`, so it cannot
  go through the public gateway either. Decision: a second gateway, `bff`,
  routes `/t/{slug}/api/{path+}` to `api`, and `NV_GO_API_URL` is its default
  domain. The docs do not say whether a gateway passes `Cookie` and
  `Authorization` through. That is assumed, and the C4 smoke test checks it
  first (login, and an API-key `POST /t/{slug}/api/tx`).
- **Gateway routing.** Fixed segments outrank path parameters, which outrank
  greedy `{x+}` parameters, and of two greedy routes the longer wins
  (`api-gateway/concepts/index.md`). So the three BFF paths beat
  `/t/{slug}/api/{path+}`, which beats `/{path+}`. The integration is
  `x-yc-apigateway-integration: {type: serverless_containers, container_id,
  service_account_id}`, and the container sees the original path and `Host`.
- **Platform limits:**
  - API Gateway caps a request and a response at 2.5 MB, and a container
    request at 3.5 MB. The biggest SPA chunk is 2.2 MB today.
  - The container timeout maximum is 1 h. Above 10 min the container is
    "long-lived", and the platform may force-terminate an instance with 10 min
    notice.
  - Environment variables, Lockbox secrets included, total at most 4 KB.
  - Images must be Linux x86_64.
  - `PORT` is set by the platform. Nitro reads `NITRO_PORT ?? PORT`, so the
    frontend image needs no change. The Go services must listen on `$PORT`
    (C4).
  - Default quotas per availability zone, shared by every container in the
    cloud: 10 instances and 10 concurrent requests. Above that a call queues,
    then fails with `429 TooManyRequests`. Provisioned instances are capped
    at 2 per cloud.
- **A throttled invocation consumes a receive count.** The docs say an MQ
  trigger returns a message to the queue with its visibility timeout on any
  invocation error, and nothing exempts a 429. Each trigger receive increments
  `ApproximateReceiveCount`. So a campaign fan-out beyond the request quota
  burns retries, and a throttled `jobs-long` message waits the full long
  visibility timeout before its next try. Inferred from the docs, not stated
  by them.
- **Roles named by the docs.** MQ trigger: `editor` on the queue's folder and
  `serverless.containers.invoker` on the container's folder. Timer: invoker on
  the folder. Data Streams: invoker on the container and `yds.admin` on the
  stream. The module grants narrower roles: `ymq.reader` (plus `ymq.writer`
  for the feedback DLQ), `yds.admin` on the folder, and invoker on `worker`
  alone. Unverified until the C4 smoke test.
- **Provider hashes.** `terraform providers lock` ignores the CLI
  `network_mirror` setting; it needs `-net-mirror=https://terraform-mirror.yandexcloud.net/`.
- **Mock-provider tests** must use `command = plan`. An `apply` runs the
  migrate step's `local-exec` even when `terraform_data.migrate` is overridden.
  Values that assertions read are mocked with `override_during = plan`.

Decisions for Plan C3:

- Service accounts `app`, `gateway`, `trigger` and `queues` replace `api`,
  `worker` and `invoker` (see "Terraform module"). The input
  `system_from_email` becomes `system_sender_domain`, since the config takes a
  domain and sends from `no-reply@<domain>`.
- **Timeouts and visibility.** `worker` runs with `execution_timeout` 3600 s.
  C4's dispatcher bounds short jobs to 60 s (`NVELOPE_JOB_TIMEOUT`) and long
  jobs to 55 min (`NVELOPE_LONG_JOB_TIMEOUT`). The `jobs` queue's visibility
  timeout is 120 s and `jobs-long`'s is 3900 s. The trigger's own
  `visibility_timeout` stays unset.
- **Retry budget.** `maxReceiveCount` is 20 on `jobs` (about 40 min of
  retries) and 10 on `jobs-long` (about 11 h), because a 429 consumes a
  receive. DLQs keep messages for 14 days. The feedback trigger retries 5
  times, 60 s apart, then writes to the `nvelope-feedback-dlq` queue.
- **Instance limits** cannot be set per container. Sizing becomes a quota
  request in the README and a C4 smoke-test measurement. `jobs-long` keeps the
  one `worker` container: a second container would share the same quota, so it
  would not isolate anything.
- **IAM.** Invoker is bound per container. `gateway` gets it on `api` and
  `frontend`, `trigger` on `worker` only. A mock-provider test pins the
  `worker` binding.
- **Timers.** The domain sweep fires every 15 min from the same local that sets
  `NVELOPE_SENDING_DOMAIN_VERIFY_INTERVAL`. Analytics refreshes hourly.
- **Media and uploads.** `NVELOPE_MEDIA_MAX_BYTES` is 2 MiB, under the gateway
  cap. A CI step fails the frontend build if any static asset exceeds 2400 KiB.
- **The Data Streams trigger's consumer.** The topic has
  `lifecycle { ignore_changes = [consumer] }`, so a later apply does not
  delete a consumer the trigger registers.
- **Images.** A `v*` tag push builds `api`, `worker` and `frontend` with
  `--platform linux/amd64` and pushes them to `cr.yandex/<registry_id>`. The
  registry is public (`container-registry.images.puller` for `system:allUsers`).
- **Real-YMQ probe: PASS** (2026-11-15). SendMessage through the production Enqueuer, then ReceiveMessage and DeleteMessage with service/sqs v1.42.27, against https://message-queue.api.cloud.yandex.net. YMQ accepts the AWS JSON protocol.

## Ceilings

These are known limits, marked in code with `ponytail:` comments.

- `count(*) + OFFSET` pagination costs RU proportional to the rows scanned.
  Fine for a single tenant with hundreds of thousands of subscribers. The
  upgrade path is keyset pagination.
- Cold start: Go is about 100–300 ms, the Node BFF about 1 s. The upgrade path
  is `provisioned_instances > 0`, which costs money.
- The fixed-window throttle allows up to 2× the limit at a window boundary.
  This is acceptable for anti-abuse and Postbox pacing.
- Two concurrent first-run setups with the right token both pass the "workspace does
  not exist" check; the second then fails on the slug and leaves a verified account
  with no workspace. Only the token holder can race. The upgrade path is one
  transaction across auth and tenant, as invitation onboarding does.
- The default serverless YDB throttle of 10 RU/s (V5) throttles bulk jobs
  (imports, sends). The upgrade path is raising or removing the limit through
  the `ydb_throttling_rcu_limit` Terraform input.
- The YDB container used by tests also holds the `dbtest_ready` and
  `dbtest_ddl_probe` tables, so tests must not assert an exact table set.
- One worker invocation runs one job (batch size 1). Throughput scales through
  container instances, not batching.
- YDB has no partial index, so the pending-invitation check
  (`YDBInvitations.Create`) reads every invitation row of the tenant, whatever
  its status. That is fine for one organisation's invitation history. The
  check also takes a read lock on the tenant's whole invitation range, so
  concurrent `Create`s in one tenant (any email) and concurrent `Update`s or
  accepts of any of its invitations abort each other and are retried by the
  SDK. That is correct, and fine for one organisation. The upgrade path is a
  `GLOBAL SYNC` index on `(tenant_id, email_key)`, which fixes both cost and
  contention. When it lands, re-run `TestInvitationsCreateIsRaceFree`, since
  the guarantee would then rest on an index-range lock.
- `YDBRoles.Delete` checks for remaining assignments by scanning the tenant's
  `user_roles` and `user_list_roles` rows, because there is no `role_id`
  index. Concurrent assignments in the tenant abort the delete, and the SDK
  retries it. That is fine for one organisation's users. The upgrade path is
  `GLOBAL SYNC` indexes on `(tenant_id, role_id)`.
- A segment becomes one YQL query whose text grows with the condition tree
  (about 150 bytes and one `DECLARE` per condition). Past the 10 KB query-text
  limit (V5), YDB rejects it with `BAD_REQUEST`. That is several dozen
  conditions; Postgres has no such limit. The upgrade path is passing the
  operands as one `List<Utf8>` parameter.
- `YDBLists.Delete` removes every membership of the list in one transaction.
  V5 found no documented transaction-size limit. The upgrade path is deleting
  in pages before the list row.
- Every page and count of `Search`, `InList` and `RunSegment` evaluates its
  predicate over the tenant's subscribers (`InList` through the list index),
  which the `count(*) + OFFSET` ceiling covers.
- A user's "check now" on a pending domain (`RecheckDomain`) starts one more
  verification chain beside the live one, in both runtimes. Each chain polls
  every interval until the domain verifies or the window (72 h) closes. The
  upgrade path is a worker-side debounce that exempts user-requested polls
  (see "YMQ enqueuer").
- When job delivery stalls for more than two poll intervals (a trigger backlog
  or outage), every sweep tick re-arms each pending domain. Those chains all
  survive until the domain verifies or the 72 h window closes. The upgrade path
  is the same worker-side debounce that exempts user-requested polls.
- Import and export files in Object Storage are never deleted, as Postgres
  keeps `file_bytes` forever. The upgrade path is a lifecycle rule on `jobs/`
  together with a retention period for job rows. An object whose row insert
  failed and whose best-effort delete also failed stays as an unreferenced
  private object.
- An import stores at most its first 10000 row failures on YDB, because a YDB
  cell holds 16 MiB. `failed_count` stays exact. The upgrade path is to keep
  the full list as an Object Storage object next to the job file.
- The `/jobs` dispatcher assumes the trigger's batch size 1. With a larger
  batch, one failed job redelivers the whole batch, including jobs that
  already succeeded.
- `YDBRecipients.Pending` reads the campaign's recipients from the start of the
  id order to the end of its window, so batch N of a campaign reads N windows
  of rows, and `Counts` reads every recipient row of the campaign each time a
  batch ends. Fine for one organisation's campaigns. The upgrade path is a
  per-campaign ordinal column to seek on, and counters kept on the campaign row.
- `YDBTemplates.Delete` scans the tenant's campaigns to clear `template_id`.
  The upgrade path is a `GLOBAL SYNC` index on `(tenant_id, template_id)`.
- `YDBCampaigns.All` and `Archived` scan the tenant's campaigns and sort them,
  reading every campaign body of the tenant. The upgrade path is keyset
  paging plus an index on `(tenant_id, created_at)` or `(tenant_id,
  archive_visible, archived_at)`.
- `YDBRecipients.BulkInsert` returns a count that falls short when a chunk
  committed and its response was lost, because the retry finds its rows. No
  caller uses the count.
- Every `MarkSent` updates `campaign_recipients` and the provider-message-id
  index. Until a recipient is sent, its `provider_message_id` is NULL, so
  the index is global and every unsent, failed and skipped recipient of every
  campaign and tenant shares the NULL key; the primary key suffix of the index
  entry lets YDB split it.
- `YDBAnalytics.Refresh` scans every recipient of the tenant (filtering by
  status, not only the `sent` ones) and every delivery event of the tenant on
  each run, and `delivery_events` has no index on `campaign_id`. Fine for one
  organisation's history. The upgrade path is counters kept on the campaign
  row, or a refresh that reads only events newer than the last one.
- `Refresh` is one read-write statement over the tenant's recipient and event
  ranges, so while a campaign is sending, concurrent recipient updates and
  event inserts may invalidate its locks and the SDK retries until the job
  deadline (not measured; the dashboard goes stale during sends). The upgrade
  path is to read the counts in a snapshot read-only transaction, then `UPSERT`
  from a list parameter.
- `SweepPendingFeedbackHandler` re-enqueues at most 500 notifications per
  sweep; the next tick takes the rest. `inbound_feedback_events` rows are never
  deleted (as on Postgres), and `idx_inbound_status` holds every row, so the
  `pending` range is a short range of an index that grows with history (status
  `pending` sorts between `failed` and `unattributed`). The upgrade path is a
  retention job for processed rows. No production code ever sets `InboundFailed`,
  so a notification that fails on every attempt is re-enqueued on every tick,
  and 500 of them would starve newer ones; the DLQ alert is the operator signal.
- `YDBAssets.List` reads and sorts every asset of the tenant (the port has no
  paging), and the media library page renders them all. Fine for one
  organisation's library. The upgrade path is a `GLOBAL SYNC` index on
  `(tenant_id, created_at)` and keyset paging through the port.
- `media_assets.uploaded_by` has no `ON DELETE SET NULL` counterpart on YDB:
  no code deletes users or tenants, so nothing can leave it dangling. A user
  or tenant delete added later must clear it (or the asset rows) explicitly.
- API Gateway caps requests and responses at 2.5 MB. Media uploads are held to
  2 MiB (`NVELOPE_MEDIA_MAX_BYTES`), a CSV import is capped by the same limit,
  and a static asset above 2400 KiB fails CI. The upgrade path is presigned
  uploads straight to the bucket and serving static assets from Object
  Storage.
- The default container quotas (10 instances and 10 concurrent requests per
  availability zone, shared by every container in the cloud) throttle a
  campaign fan-out. They also starve `api` while it runs. A throttled job
  message burns a receive and waits a full visibility timeout. The upgrade
  paths are a quota increase and a staggered batch enqueue (`not_before`).
- Job retries use a fixed visibility timeout with no backoff. That is about
  40 min for short jobs and 11 h for long ones before the DLQ. The upgrade
  path is a per-attempt delayed re-send in the dispatcher.
- Analytics refresh hourly in serverless mode, so dashboards lag by up to an
  hour.
- The TOTP encryption key lives in Lockbox and in Terraform state. Losing the
  state rotates it on the next apply and orphans every enrolled TOTP secret.
- The `bff` gateway's default domain is public. It exposes only `api`'s
  tenant API, which the public gateway already exposes, plus Go's own
  visual-save handlers, which sanitize and validate on their own.

## Implementation phasing

The plan is split into three plans. Each one merges to `main` on its own.

- **Plan A: SaaS-safe groundwork.** Worker decoupling (`Work(ctx, args)`,
  `jobs.AddWorker`); the `dbtest.YDB` local-YDB test container; the V1, V2, V4,
  V5 and V6 spikes and audit, with findings recorded in this spec.
- **Plan B: YDB adapters.** Split into B1 (the migration runner, the
  contract-test harness and the sending context as its pilot, the snooze
  reset from V6, and `ydbthrottle`) and one plan per remaining context, in
  this order: auth, tenant, iam, audience, campaign, deliverability, media.
  Campaign (B7) also creates the two provider-message-id indexes that
  deliverability (B8) reads; see "Plan B7 planning spike". Deliverability (B8)
  also adds the one port method the feedback recovery sweep needs; see "Plan B8
  planning spike". Media (B9) is the last context: with it, every repository
  the composition root wires has a YDB version, billing excepted; see "Plan B9
  planning spike".
  Audience takes two plans: B5 (the core: lists, subscribers, memberships,
  fields) and B6 (import/export jobs, subscription pages, pending
  subscriptions); see "Plan B5 planning spike".
  Auth comes before tenant because `Tenants.ListMembers` joins
  `platform_users`, which auth owns, and auth depends on no tenant table.
  The tenant plan (B3) also ports `internal/service/onboarding.go` `AcceptForNewUser` (it writes `platform_users`, `platform_sessions`, `invitations` and `platform_user_tenants` in one transaction) and shares the auth email-key derivation.
  Each context converts its `*_pg_test.go` into a pg+ydb contract suite in the
  same task that adds its YDB adapter. The harness shape is fixed by the first
  context (sending, the smallest). Building it earlier, with only one
  implementation, would be a speculative abstraction.
- **Plan C: Serverless runtime and packaging.** Split into four plans. C1, C2
  and C3 run in parallel with Plan B: they build on the ports, Plan A and the
  B1 runner, not on the remaining YDB adapters.
  - **C1, job runtime:** the YMQ enqueuer and `dbtest.SQSEndpoint`; the `/jobs`
    dispatcher with its fakecloud pump tests; the worker routes `/cron/*`,
    `/feedback` and `/admin/migrate`. Library code and handlers only; nothing
    starts them until C4.
  - **C2, single-tenant bootstrap:** setup-token signup, `/api/platform/config`
    and the frontend flags.
  - **C3, packaging:** the Terraform module and image publishing. Only
    `terraform validate`, `test` and `plan` can run before C4.
  - **C4, assembly (after Plan B):** the serverless composition root, the
    serverless modes of `cmd/worker` and `cmd/api`, the README quickstart and
    the smoke test.
