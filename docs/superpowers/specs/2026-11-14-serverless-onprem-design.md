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
- Runner: the `golang-migrate/v4` that is already installed (v4.19), with its
  `ydb` database driver.
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
- `jobs.River[T]` is one generic wrapper that adapts such a worker to
  `river.AddWorker` for SaaS.
- `jobs.Snooze(d)` is a project-owned error. The River wrapper maps it to
  `river.JobSnooze(d)`. The YMQ dispatcher maps it to a delayed re-send.
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
  Standard YMQ queues rely on worker idempotency instead. The plan checks each
  of the 6 kinds (V3). Any kind that is not idempotent gets a FIFO queue with
  `MessageDeduplicationId = hash(kind, args)`.

### `POST /jobs` (Message Queue trigger, batch size 1)

The handler decodes the trigger payload and dispatches by `kind` through a
`map[string]func(ctx, json.RawMessage) error` registry. The context deadline is
the container timeout minus a safety margin.

| Outcome | Action | HTTP |
|---------|--------|------|
| `nil` | — (the trigger deletes the message) | 200 |
| `jobs.Snooze(d)` | send a copy with `DelaySeconds = min(d, 900s)` and `not_before = now + d` | 200 |
| `not_before` in the future | re-send with the remaining delay, capped at 900 s | 200 |
| any other error, unknown `kind`, undecodable body | log with kind and args | 500 → redelivered after the visibility timeout, then DLQ |

### `POST /cron/:name` (timer triggers)

Each route enqueues the same jobs `cmd/scheduler` enqueues today:

- `domain-verify-sweep`
- `analytics-refresh` for the single tenant

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
- `/admin/migrate` runs `golang-migrate up` against YDB and is idempotent.

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
- **Job runtime unit tests.** Dispatcher outcome table (`nil`, snooze,
  `not_before`, error, unknown kind, bad body → status and re-send) against a
  fake SQS client.
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

- **V1** YQL `ILIKE` availability in serverless YDB. Fallback: an
  `email_lower` column.
- **V2** `yandex` Terraform provider support for Postbox resources and for the
  YDB topic consumer used by the Data Streams trigger.
- **V3** Idempotency of each of the 6 `ByArgs`-unique job kinds. Whether
  Message Queue triggers accept FIFO queues, in case a FIFO queue is needed.
- **V4** That `golang-migrate`'s `ydb` driver handles the baseline DDL,
  including TTL and unique indexes. Fallback: run plain DDL through `ydb-go-sdk`
  in `/admin/migrate`.
- **V5** Serverless YDB per-query limits (result rows, duration) for export and
  recipient resolution. Page through results where needed.
- **V6** Transaction closure audit: all 129 sites are retry-safe.

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

- **Plan A: SaaS-safe groundwork.** V1–V6 spikes; worker decoupling
  (`Work(ctx, args)`, `jobs.River[T]`, `jobs.Snooze`); the contract-test
  harness, with pg only and no behavior change.
- **Plan B: YDB adapters.** `ydbtx`, the baseline schema, then per-context
  adapters in this order: tenant, iam, auth, audience, sending, campaign,
  deliverability, media. Each context switches on the `ydb` leg of its contract
  suite. Then `ydbthrottle`.
- **Plan C: Serverless runtime and packaging.** YMQ enqueuer; `/jobs`, `/cron`,
  `/feedback` and `/admin/migrate`; the serverless composition root;
  single-tenant bootstrap; `/api/platform/config` and the frontend flags; the
  Terraform module; image publishing; README quickstart; the smoke test.
