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

- All containers are **private**. API Gateway and triggers invoke them with an
  `invoker` service account that has `serverless.containers.invoker`.
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
| `INSERT ... ON CONFLICT DO NOTHING` | An INSERT conflict aborts the whole interactive transaction ("Plan B2 planning spike"). Inside a transaction use `SELECT` then `INSERT` (as `AddYDBMembership` does, Plan B3). A standalone single-statement INSERT may treat `db.IsYDBConflict` as a no-op. `UPSERT` is wrong where it would overwrite existing data. Still open, for the campaign plan: whether `Recipients.BulkInsert` can continue after a conflict, or needs `SELECT` then `INSERT`/chunk-level `UPSERT`. |
| `SELECT ... FOR UPDATE` (7) | removed. A serializable conflict aborts the tx and the SDK retries it. |
| `RETURNING` (26) | supported as is |
| `now()` (28) | `CurrentUtcTimestamp()` |
| `jsonb` | `JsonDocument` where YQL queries into the document (audience attributes, segments). JSON that is only stored and read back whole (e.g. `sending_domains.dkim_records`) is a plain `Utf8` column. |
| `text[]` read back whole (`roles.permissions`, `api_keys.permissions`) | a JSON array in a plain `Utf8` column (Plan B4) |
| `citext` (emails, slugs) | Where the domain already lower-cases the value (`tenants.slug`, iam `users.email`), the column itself carries the unique index. Otherwise the value is stored as entered, plus a lower-cased `email_key` column that carries the index (`platform_users`, `invitations`). |
| foreign keys | YDB has none. Where a pg foreign key makes a write fail and the referenced table belongs to the same context, the YDB adapter reads the referenced row in the same `DoTx` (iam role assignments read `roles`, Plan B4). A cascade the port must keep becomes an explicit delete in the parent's delete transaction (the same `DoTx` as the parent's delete). |
| `ORDER BY` on a text column | YDB orders `Utf8` by bytes, pg by the database collation, so mixed-case and non-ASCII values order differently. Acceptable for display lists (iam `roles` by `name`). If a later plan needs collation-equal order, sort in Go or store a sort key. |
| bound parameters (`$1`, `@name`) | `$name` parameters, each declared explicitly with `DECLARE $name AS <Type>;`. String literals that target a `Utf8` column take the `u` suffix (`"pending"u`); a bare `"..."` literal is `String`. Identifiers avoid YQL keywords (e.g. `throttle_key`, not `key`). |
| segment SQL translator (`internal/audience/adapters/segment_sql.go`: jsonb `?`, `->>`, `::numeric`) | needs its own YQL port, using `JSON_VALUE` / `JSON_EXISTS` on `JsonDocument` or equivalent. Exact functions **to verify in Plan B**. |
| `ILIKE` (3 sites: `deliverability/adapters/suppression_pg.go:91` email filter; `audience/adapters/segment_sql.go:68` `col::text ILIKE`; `segment_sql.go:87` `attributes->>key ILIKE`) | YQL `ILIKE` (V1 passed only for an ASCII literal against an ASCII literal; Utf8 columns, parameters and Cyrillic are **to verify in Plan B**). Otherwise a stored lowercase column with `LIKE`. |
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
- Each queue has a DLQ through a redrive policy with `maxReceiveCount=5`.
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
  intervals. A live chain polls every interval, so the sweep never starts a
  second chain beside it. `VerifyWorker` and SaaS are unchanged.

  The worker-side debounce planned earlier (`VerifyWorker.Work` returns `nil`
  when `now - LastCheckedAt <= interval/2`) was dropped in Plan C1 planning.
  `RecheckDomain` (the user's "check now") enqueues a plain `EnqueueVerify`, so
  under the debounce a recheck within 7.5 minutes of the last poll (default
  interval 15 min) would not poll at all, in SaaS too. The duplicate chain a
  recheck starts is pre-existing behavior in both runtimes (see Ceilings).

### `POST /jobs` (Message Queue trigger, batch size 1)

The handler decodes the trigger payload and dispatches by `kind` through a
`map[string]func(ctx, json.RawMessage) error` registry. The context deadline is
the container timeout minus a safety margin.

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
- While no user exists, `POST /api/platform/signup` requires the setup token. A
  valid token:
  - creates the first user, **skipping email verification**, because nothing can
    send mail before the first login and holding the token already proves the
    right to install;
  - creates the tenant through the existing `create_workspace` command, with the
    user as owner.

  Without the token, or once any user exists, signup returns 403.
- After that, people join only by invitation (`invite_teammate` /
  `accept_invitation`).
- `POST /api/platform/tenants` returns 409 once the tenant exists.
- The setup-token comparison is constant-time.

### UI feature flags

- A public `GET /api/platform/config` returns
  `{"features": {"billing": bool, "multiTenant": bool}, "setupRequired": bool}`.
  It is added to the OpenAPI spec (spec-first API).
- The frontend hides billing screens and the tenant switcher when the flags are
  false. It shows the setup-token field on the signup page when
  `setupRequired` is true.
- No other frontend changes.

### Terraform module: `deploy/terraform/nvelope-serverless/`

Inputs: `folder_id`, `domain`, `image_tag`, `tenant_slug`, `system_from_email`,
`send_rate_per_sec`, `provisioned_instances` (default 0),
`ydb_throttling_rcu_limit` (default 1000 RU/s; 0 or unset disables the limit,
per the docs; the default value is a Plan C choice). Serverless YDB is
throttled at 10 RU/s by default (V5), which would make imports and sends crawl.
The provider arguments are **to verify in Plan C**, since Plan A did not read
them in a source. Candidates on `yandex_ydb_database_serverless`:
`serverless_database { throttling_rcu_limit, provisioned_rcu_limit,
storage_size_limit }`.

Resources:

- Service accounts `api`, `worker`, `invoker`, each with minimal roles (YDB
  editor, YMQ reader/writer, storage editor on the media bucket, Postbox sender,
  Lockbox payload viewer, as each needs).
- Static access keys for YMQ and S3, stored in Lockbox.
- Serverless YDB database and a YDB topic for Postbox feedback.
- YMQ queues `jobs` and `jobs-long`, each with a DLQ.
- An Object Storage bucket for media.
- A Lockbox secret holding the session secret, setup token and static keys.
- Serverless Containers `api`, `frontend` and `worker`, with images from the
  project's public Container Registry at `image_tag`. The registry is created
  once by the maintainers, and its ID is a module default.
- API Gateway with an OpenAPI spec that routes paths to the containers, a
  custom domain, and a Let's Encrypt certificate from Certificate Manager. This
  requires a DNS zone in YC; otherwise the user supplies a certificate ID.
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
by release version.

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

No closure passed to a repository `Update*` calls an enqueuer, mailer, verifier, blob store or Postbox client, sleeps, or uses a channel. The enqueues in `StartCampaign`, `ResumeCampaign` and `StartWorker` run after `Update` returns, and the verifier check in `VerifyWorker` runs before it. `MediaRefValidator.IsTenantMediaRef(ref string) bool` takes no context and is pure, so `ApplyVisualSave` inside the save-visual closures is safe. Plain assignments to outer variables (`updated = s`, `res.CurrentUpdatedAt = ...`) are safe: the last run wins. The pg adapter bodies reference only `tx`, locals and pure helpers (the adapters hold only `pool`); there is no injected port, no HTTP/S3/Postbox call, no enqueue, no channel op, and no `slog`/`log` call inside any transaction body.

| Site (file:line) | Class | What repeats on retry | Fix in Plan B |
|------------------|-------|-----------------------|---------------|
| `internal/sending/adapters/verify_worker.go:53` | unsafe-mutation | `snooze = true` is only ever set, never reset. A retry whose second run takes the `!d.IsPending()` early return keeps the stale `true` and snoozes a job whose domain is already terminal. | Set `snooze = false` as the first statement of the closure. |
| `internal/billing/app/command/charge_invoice.go:177` (billing, not ported) | unsafe-mutation, safe-time-inside | `exhausted = true` is only ever set. A retry that sees a lower attempt count keeps the stale `true` and suspends the subscription at line 188. `time.Now()` also feeds `NextAttemptAt`. | Not ported. If ever ported: set `exhausted = false` at closure start; compute `now` before the closure. |
| `internal/billing/app/command/charge_invoice.go:86` (billing, not ported) | safe-time-inside | `s.Cancel(time.Now())` timestamp advances on retry. | Not ported. |
| `internal/billing/app/command/charge_invoice.go:259` (billing, not ported) | safe-time-inside | `i.MarkPaid(time.Now())` timestamp advances on retry. | Not ported. |
| `internal/iam/app/command/sessions.go:152` | safe-time-inside | `s.Revoke(time.Now())` timestamp advances on retry. | Accept: timestamp may advance on retry. |
| `internal/campaign/app/command/set_archive_visibility.go:35` | safe-time-inside | `SetArchiveVisible(..., time.Now())` timestamp advances on retry. | Accept: timestamp may advance on retry. |
| `internal/campaign/app/command/campaigns.go:214` | safe-time-inside | `c.Start(time.Now())` timestamp advances on retry. The enqueue is outside the closure. | Accept; keep `EnqueueStart` after `Update` returns. |
| `internal/campaign/adapters/batch_worker.go:258` | safe-time-inside | `c.Finish(time.Now())` timestamp advances on retry. | Accept: timestamp may advance on retry. |
| `internal/audience/adapters/import_worker.go:40` | safe-time-inside | `j.Start(time.Now())` timestamp advances on retry. | Accept: timestamp may advance on retry. |
| `internal/audience/adapters/import_worker.go:49` | safe-time-inside | `j.Complete(..., time.Now())` timestamp advances on retry. | Accept: timestamp may advance on retry. |
| `internal/audience/adapters/export_worker.go:44` | safe-time-inside | `j.Start(time.Now())` timestamp advances on retry. | Accept: timestamp may advance on retry. |
| `internal/audience/adapters/export_worker.go:53` | safe-time-inside | `j.Fail(time.Now())` timestamp advances on retry. | Accept: timestamp may advance on retry. |
| `internal/audience/adapters/export_worker.go:66` | safe-time-inside | `j.Complete(..., time.Now())` timestamp advances on retry. | Accept: timestamp may advance on retry. |
| `internal/auth/app/command/signup.go:87-99` (the `issueVerification` callback passed to `CreateWithVerification`; `CreateWithSession` takes the same kind of `issueSession` callback) | safe (last write wins) | `token.New()` runs again and the outer `rawToken` is overwritten by the last run, which is the run whose hash is persisted. | None; documented on the port. |
| `internal/campaign/adapters/recipients_pg.go:49` (`BulkInsert`) | unsafe-mutation (pg body) | `inserted += tag.RowsAffected()` on an outer counter: a retry double-counts rows the first attempt inserted (the rolled-back attempt's count is not undone). | Declare the counter inside the closure and assign it to the outer variable once at the end, or reset `inserted = 0` at closure start. |
| `internal/audience/adapters/lists_pg.go:165`, `memberships_pg.go:119`, `subscribers_pg.go:205`, `subscribers_pg.go:247`, `subscribers_pg.go:285`, `subscription_pages_pg.go:191`, `internal/billing/adapters/invoices_pg.go:179` (billing, not ported), `internal/campaign/adapters/templates_pg.go:152`, `campaigns_pg.go:201`, `campaigns_pg.go:232`, `campaigns_pg.go:285`, `recipients_pg.go:88`, `internal/deliverability/adapters/suppression_pg.go:118`, `analytics_pg.go:100` (`dash.Recent`), `internal/iam/adapters/audit_pg.go:82`, `roles_pg.go:156`, `apikeys_pg.go:132`, `internal/sending/adapters/domains_pg.go:164`, `domains_pg.go:189`, `internal/media/adapters/assets_pg.go:115` | unsafe-mutation (pg body, read paths) | Each does `append` to a slice declared outside the transaction closure. A retry after a partial read appends the rows again, returning duplicates. | Rewritten in Plan B anyway: build the slice inside the closure and assign it to the outer variable at the end (`result = local`), or reset it (`out = nil`) at closure start. Apply to every YDB read helper that collects rows inside a `DoTx` closure. |

All other sites are `safe`.

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

## Ceilings

These are known limits, marked in code with `ponytail:` comments.

- `count(*) + OFFSET` pagination costs RU proportional to the rows scanned.
  Fine for a single tenant with hundreds of thousands of subscribers. The
  upgrade path is keyset pagination.
- Cold start: Go is about 100–300 ms, the Node BFF about 1 s. The upgrade path
  is `provisioned_instances > 0`, which costs money.
- The fixed-window throttle allows up to 2× the limit at a window boundary.
  This is acceptable for anti-abuse and Postbox pacing.
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
- A user's "check now" on a pending domain (`RecheckDomain`) starts one more
  verification chain beside the live one, in both runtimes. Each chain polls
  every interval until the domain verifies or the window (72 h) closes. The
  upgrade path is a worker-side debounce that exempts user-requested polls
  (see "YMQ enqueuer").
- The `/jobs` dispatcher assumes the trigger's batch size 1. With a larger
  batch, one failed job redelivers the whole batch, including jobs that
  already succeeded.

## Implementation phasing

The plan is split into three plans. Each one merges to `main` on its own.

- **Plan A: SaaS-safe groundwork.** Worker decoupling (`Work(ctx, args)`,
  `jobs.AddWorker`); the `dbtest.YDB` local-YDB test container; the V1, V2, V4,
  V5 and V6 spikes and audit, with findings recorded in this spec.
- **Plan B: YDB adapters.** Split into B1 (the migration runner, the
  contract-test harness and the sending context as its pilot, the snooze
  reset from V6, and `ydbthrottle`) and one plan per remaining context, in
  this order: auth, tenant, iam, audience, campaign, deliverability, media.
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
    `terraform validate` and `plan` can run before C4.
  - **C4, assembly (after Plan B):** the serverless composition root, the
    serverless modes of `cmd/worker` and `cmd/api`, the README quickstart and
    the smoke test.
