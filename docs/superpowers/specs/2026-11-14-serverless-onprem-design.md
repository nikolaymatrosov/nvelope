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
  `terraform apply`.
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
- **Keyset pagination.** Existing `count(*) + OFFSET` pagination is ported as is
  (see Ceilings).
- **Provisioned (warm) instances by default.** Available as a Terraform
  variable, defaulted to 0 to preserve zero idle cost.

## Decisions

| # | Decision | Rationale |
|---|----------|-----------|
| D1 | Serverless YDB is the on-prem database | Only pay-per-request relational store in YC. Managed PG bills per hour. |
| D2 | Same `main` branch, adapters selected via `NVELOPE_RUNTIME=saas\|serverless` | The ports already exist (DDD layout). A long-lived fork would drift from an actively developed `main` within weeks. |
| D3 | Single tenant per install | YDB has no RLS. Today's adapters rely on RLS and do not filter by `tenant_id` (e.g. `DELETE FROM subscribers WHERE id = $1`). |
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

- `ydbtx.Do(ctx, db, fn)` replaces `tenantdb.WithTenant`. It is built on
  `db.Query().DoTx` from `ydb-go-sdk/v3` (already in `go.mod`), uses
  serializable isolation and ignores `tenantID`.
- The SDK **retries** the closure on `ABORTED`/`UNAVAILABLE`. So every
  transaction closure must be retry-safe: database operations only, no external
  side effects (no enqueue, no email, no HTTP). The plan audits all 129
  transaction call sites. Today enqueue is already outside the transaction
  (`client.Insert`, never `InsertTx`).

### Schema and migrations

- One baseline YQL migration in `internal/db/ydbmigrations/000001_baseline.up.sql`
  covering the in-scope tables (about 38 of the 45). The 21-migration Postgres
  history is not replayed.
- Runner: `golang-migrate` v4.19.1 has **no** `ydb` driver (checked during
  planning). The default is a small in-repo runner:
  - embedded `*.up.sql` files are applied in order with
    `db.Query().Exec`;
  - applied versions are recorded in a `schema_migrations` table.

  If spike V4 shows that query-service `Exec` cannot run a multi-statement DDL
  file, the fallback is `pressly/goose` v3.30.1, which has a `ydb` dialect.
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
| `INSERT ... ON CONFLICT DO NOTHING` | `INSERT` and treat "already exists" as no-op, or `SELECT` then `UPSERT` in the same tx |
| `SELECT ... FOR UPDATE` (7) | removed. A serializable conflict aborts the tx and the SDK retries it. |
| `RETURNING` (26) | supported as is |
| `now()` (28) | `CurrentUtcTimestamp()` |
| `jsonb` | `JsonDocument` |
| `ILIKE` (1, subscriber search) | YQL `ILIKE` if verified (V1). Otherwise a stored `email_lower` column with `LIKE`. |
| `SECURITY DEFINER` functions (6) | ordinary queries. With no RLS, the privilege bypass they existed for is unnecessary. Billing-only functions are dropped. |

### Throttling

`ydbthrottle.Counter` is a fixed-window counter on a table
`throttle_windows(key Utf8, window_start Timestamp, count Uint64, expire_at Timestamp)`
with TTL on `expire_at`. `Allow(key, limit)` runs one read-modify-write
transaction. It reads the current window's row, then `UPSERT`s `count + 1`. The
SDK retries on a serializable conflict. When the limit is exceeded it returns
`retryAfter = window_end - now`. It implements all three existing ports:

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
  access keys from Lockbox.
- **`UniqueOpts{ByArgs: true}` is not reproduced** (6 sites in `jobs.go`).
  Standard YMQ queues are used. V3 was resolved during planning:

  | Kind | Serverless handling |
  |------|---------------------|
  | `billing.sweep`, `billing.charge`, `usage.rollup` | not wired (billing is out of scope) |
  | `auth.verification_cleanup` | not wired (table TTL replaces it) |
  | `analytics.refresh` | duplicates are harmless: an idempotent recompute, and the cron interval is far longer than one refresh |
  | `domain.verify` via `EnqueueVerifyUnique` | **needs a debounce** (below) |

  `domain.verify` needs the debounce because the sweep relies on uniqueness to
  avoid stacking chains (`cmd/scheduler` `sweepPendingDomains`). Without it,
  every sweep would start one more self-snoozing chain per pending domain.

  The fix lands in Plan C, test-first: `VerifyWorker.Work` returns `nil`
  without polling when
  `now - LastCheckedAt <= interval/2`. Of any two chains, one always exits at
  its next run, so exactly one survives. SaaS behavior is unchanged, because
  River's uniqueness already prevents the duplicate.

### `POST /jobs` (Message Queue trigger, batch size 1)

The handler decodes the trigger payload and dispatches by `kind` through a
`map[string]func(ctx, json.RawMessage) error` registry. The context deadline is
the container timeout minus a safety margin.

| Outcome | Action | HTTP |
|---------|--------|------|
| `nil` | — (the trigger deletes the message) | 200 |
| `*rivertype.JobSnoozeError{Duration: d}` (from `river.JobSnooze`) | send a copy with `DelaySeconds = min(d, 900s)` and `not_before = now + d` | 200 |
| `not_before` in the future | re-send with the remaining delay, capped at 900 s | 200 |
| any other error, unknown `kind`, undecodable body | log with kind and args | 500 → redelivered after the visibility timeout, then DLQ |

### `POST /cron/:name` (timer triggers)

Each route enqueues the same jobs `cmd/scheduler` enqueues today:

- `domain-verify-sweep`
- `analytics-refresh` for the single tenant

`cmd/scheduler` selects pending domains and active tenants with raw SQL on the
pool (`sweepPendingDomains`, `enqueueAnalyticsRefresh`). The cron handlers need
the same reads as small YDB queries; they land in Plan C.

The campaign workers already accept `nil` for the billing ports `quota` and
`usage` ("a deployment without billing"), so serverless wiring passes `nil`.

Billing sweep, usage rollup and verification cleanup are not wired: billing is
out of scope, and TTL replaces the cleanup.

### `POST /feedback` (Data Streams trigger on the Postbox topic)

- Decodes each message in the trigger batch and calls the existing
  `deliverability/app/command.IngestNotificationHandler` (staging row +
  `feedback.process` enqueue). This is the same handler `cmd/consumer` calls.
- Any failure returns 500. The trigger retries per its retry settings, then the
  message goes to the trigger's DLQ.

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
`send_rate_per_sec`, `provisioned_instances` (default 0).

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
- Postbox address and configuration set with the topic as feedback destination,
  if the provider supports them (V2). Otherwise this is a documented manual step.

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
- **Job runtime tests use real SQS semantics from fakecloud.** YMQ speaks the
  SQS API, so the YMQ enqueuer and the `/jobs` dispatcher are tested against
  [fakecloud](https://github.com/faiscadev/fakecloud), a local AWS emulator.
  It runs as a `dbtest.SQS(t)` testcontainer helper, following the
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

## Ceilings

These are known limits, marked in code with `ponytail:` comments.

- `count(*) + OFFSET` pagination costs RU proportional to the rows scanned.
  Fine for a single tenant with hundreds of thousands of subscribers. The
  upgrade path is keyset pagination.
- Cold start: Go is about 100–300 ms, the Node BFF about 1 s. The upgrade path
  is `provisioned_instances > 0`, which costs money.
- The fixed-window throttle allows up to 2× the limit at a window boundary.
  This is acceptable for anti-abuse and Postbox pacing.
- One worker invocation runs one job (batch size 1). Throughput scales through
  container instances, not batching.

## Implementation phasing

The plan is split into three plans. Each one merges to `main` on its own.

- **Plan A: SaaS-safe groundwork.** Worker decoupling (`Work(ctx, args)`,
  `jobs.AddWorker`); the `dbtest.YDB` local-YDB test container; the V1, V2, V4,
  V5 and V6 spikes and audit, with findings recorded in this spec.
- **Plan B: YDB adapters.** `ydbtx`, the migration runner and baseline schema,
  then per-context adapters in this order: sending, tenant, iam, auth,
  audience, campaign, deliverability, media. Each context converts its
  `*_pg_test.go` into a pg+ydb contract suite in the same task that adds its
  YDB adapter. The harness shape is fixed by the first context (sending, the
  smallest). Building it earlier, with only one implementation, would be a
  speculative abstraction. Then `ydbthrottle`.
- **Plan C: Serverless runtime and packaging.** YMQ enqueuer; `/jobs`, `/cron`,
  `/feedback` and `/admin/migrate`; the serverless composition root;
  single-tenant bootstrap; `/api/platform/config` and the frontend flags; the
  Terraform module; image publishing; README quickstart; the smoke test.
