# Implementation Plan: Phase 3 — Sending Pipeline

**Branch**: `006-phase-3-sending-pipeline` | **Date**: 2026-05-17 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/006-phase-3-sending-pipeline/spec.md`

## Summary

Phase 3 turns nvelope from an audience manager into a mail platform: a tenant can
verify a sending domain and send a campaign — or a single transactional message —
through Yandex Postbox, with open and click tracking. The backend already runs
River (Postgres-backed job queue), the calibrated DDD layout (`<ctx>/domain`,
`<ctx>/app`, `<ctx>/adapters`, shared `api/` transport, `service/` composition
root), RLS-backed tenant isolation, and scoped API keys from Phase 2.

Technical approach: add two bounded contexts and two shared platform packages.
The `sending` context owns the `sending_domains` aggregate and the `domain.verify`
polling worker. The `campaign` context owns templates, campaigns, the
`campaign.start` → `campaign.batch` send pipeline, recipient deduplication, open
pixel / click-link generation, and the API-key-authenticated transactional `tx`
send. A shared `platform/postbox` package wraps the Postbox SES-compatible HTTP
API with AWS SigV4 request signing; a shared `platform/ratelimit` package
implements Redis-coordinated per-tenant and global sliding-window limiting. New
River job kinds (`campaign.start`, `campaign.batch`, `domain.verify`) are
registered in the worker; the scheduler enqueues periodic domain re-checks. Two
public, unauthenticated tracking endpoints resolve tenant/campaign/subscriber from
a link UUID alone.

## Technical Context

**Language/Version**: Go 1.26

**Primary Dependencies**: existing — `riverqueue/river` v0.37 (job queue),
`jackc/pgx/v5` (Postgres), `go-chi/chi/v5` (HTTP), `golang-migrate/migrate/v4`,
`knadh/koanf` (config). **Add** — `redis/go-redis/v9` (Redis client for
cross-pod rate-limit counters) and `aws-sdk-go-v2/aws/signer/v4` +
`aws-sdk-go-v2/aws/credentials` (standalone AWS SigV4 request signer for the
Postbox SES-compatible API). No other AWS SDK service clients are pulled in.

**Storage**: PostgreSQL (shared database, RLS) for all tenant-scoped tables and
River's queue tables; Redis for sliding-window rate-limit counters only (no
durable state). Generated export/preview bytes are not stored; rendered messages
are sent and not persisted.

**Testing**: `go test ./...` with `testify`; integration tests against a real
`postgres:17` via `testcontainers-go` (existing `internal/dbtest` harness);
Postbox is exercised through a fake implementing the messenger/provisioner
interfaces for component tests, plus an opt-in integration test against a real
Postbox staging account gated by env vars; Redis rate-limit tests run against a
real Redis container via testcontainers.

**Target Platform**: Linux server; three stateless Go services (`cmd/api`,
`cmd/worker`, `cmd/scheduler`) on Kubernetes.

**Project Type**: Web service (Go backend). This phase is backend-only; the
campaign/sending UI is a later phase.

**Performance Goals**: No hard latency target. Sending throughput is governed by
the tenant's plan rate limit and the global cap, not by code hot paths. A
campaign send must make continuous progress for every tenant under concurrent
load (fairness), and must complete without duplicate sends across worker
restarts.

**Constraints**: Tenant isolation is the data layer's job (RLS), never
application code alone. Postbox is reached only through the thin
`platform/postbox` abstraction. All background work is durable and resumable on
the River queue. Rate limits are enforced centrally in Redis so the limit holds
regardless of worker-pod count. Domain and use-case code never imports
transport, driver, or external-client packages; error→status mapping stays in
the one existing place (`api/errmap.go`).

**Scale/Scope**: 3 user stories, 37 functional requirements, ~8 key entities.
Roughly: 3 new migrations, 2 new bounded contexts (`sending`, `campaign`), 2 new
shared platform packages (`postbox`, `ratelimit`), 3 new River job kinds + 3
workers, ~14 new API routes (12 authenticated tenant routes, 1 API-key `tx`
route, 2 public tracking routes).

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

Evaluated against `.specify/memory/constitution.md` v1.1.0.

| Principle | Status | Notes |
| --- | --- | --- |
| I. Tenant Isolation by Default | PASS | Every new tenant-plane table (`sending_domains`, `templates`, `campaigns`, `campaign_lists`, `campaign_recipients`, `links`, `link_clicks`, `campaign_views`) carries `tenant_id` and `ENABLE`/`FORCE ROW LEVEL SECURITY` with the same `app.tenant_id` policy as Phase 2. Every River job payload carries `tenant_id`; the worker sets `SET LOCAL app.tenant_id` before any tenant data access (existing `tenantdb` helper). Public tracking endpoints resolve tenant from the link/view UUID and then run inside that tenant's bound transaction. Cross-tenant isolation tests are added for every new repository. |
| II. Test-Backed Delivery | PASS | Critical paths here — email sending, async job processing, rate limiting — get integration coverage against real boundaries: Postgres + River via testcontainers, Redis via testcontainers, a fake Postbox for component tests plus an opt-in real-Postbox integration test. Resumability (worker killed mid-send → no duplicates) and rate-limit correctness across concurrent goroutines are explicit tests. Phase exits with green `go test ./...` and a clean migration apply. |
| III. Incremental, Shippable Phases | PASS | Three independently shippable slices: US1 domain verification, US2 campaign send with tracking, US3 transactional API. Build for this phase only — bounce/complaint webhooks, suppression list, and analytics dashboards are explicitly deferred to Phase 4 and are not designed in here. |
| IV. Security & Consent by Design | PASS | Domain ownership is proven by DNS verification before any send (FR-014). The `tx` endpoint is authenticated by a scoped API key reusing Phase 2's `AuthenticateAPIKey` query and key-scope check. Postbox is reached only over SigV4-signed requests with per-environment least-privilege credentials. Privileged actions (domain added/verified, campaign started) are written to the existing `audit_log`. Postbox credentials and the Redis DSN are secret config, never logged. |
| V. Operable & Observable Services | PASS | All three services stay stateless: rate-limit counters live in Redis, job state in Postgres, no in-process work state. The send pipeline is durable and resumable — `campaign.start` and `campaign.batch` are River jobs with retries/backoff; per-recipient status rows make redelivery idempotent so a restarted worker never drops or duplicates a send. Every command/query handler keeps the standard logging/metrics decorator. |
| VI. Layered Architecture & Domain Integrity | PASS | Each new context uses the calibrated `domain`/`app`/`adapters` split. `sending_domains`, `Template`, and `Campaign` are rich entities with unexported fields, validating constructors, a separate documented hydration path, and lifecycle behaviour as methods (e.g. `Campaign.Start`, `SendingDomain.MarkVerified`). The `Messenger`, `DomainProvisioner`, and `RateLimiter` interfaces are declared by the consuming domain/app layer; the `postbox` and `ratelimit` adapters conform. Errors carry slugs via the shared `apperr` package; transport mapping stays in `api/errmap.go`. Wiring is plain constructors in `service/`. |

**Gate result: PASS — no violations. Complexity Tracking not required.**

Two new dependencies are introduced; neither is speculative. Redis is mandated by
the constitution's "quotas and rate limits are enforced centrally and applied
consistently across all instances" and by `docs/architecture.md`. The standalone
AWS SigV4 signer is the minimum needed to authenticate to Postbox's SES-compatible
API; alternatives are weighed in `research.md` (R2).

Re-check after Phase 1 design: **PASS** — the data model adds only RLS-protected
tenant-plane tables; contracts add no transport leakage into domain code; the two
new external integrations sit behind domain-owned interfaces. Design holds the
gate.

## Project Structure

### Documentation (this feature)

```text
specs/006-phase-3-sending-pipeline/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/
│   ├── http-api.md      # Phase 1 output — new HTTP routes & payloads
│   ├── jobs.md          # Phase 1 output — River job kinds & payloads
│   └── ports.md         # Phase 1 output — domain-owned Go interfaces
└── tasks.md             # Phase 2 output (/speckit-tasks — NOT created here)
```

### Source Code (repository root)

```text
internal/
├── sending/                       # NEW bounded context — sending domains
│   ├── domain/
│   │   ├── domain.go               # SendingDomain aggregate (status, DNS records)
│   │   ├── repository.go           # SendingDomainRepository interface (domain-owned)
│   │   ├── provisioner.go          # DomainProvisioner + IdentityVerifier interfaces
│   │   └── errors.go               # context-specific slug errors
│   ├── app/
│   │   ├── application.go          # Application{Commands, Queries}
│   │   ├── command/                # AddDomain, RecheckDomain
│   │   └── query/                  # ListDomains, GetDomain
│   └── adapters/
│       ├── domains_pg.go           # SendingDomainRepository (pgx + RLS)
│       ├── verify_worker.go        # River worker for domain.verify
│       └── *_test.go
├── campaign/                      # NEW bounded context — templates & campaigns
│   ├── domain/
│   │   ├── template.go             # Template aggregate
│   │   ├── campaign.go             # Campaign aggregate (lifecycle, progress)
│   │   ├── recipient.go            # CampaignRecipient (per-send-target status)
│   │   ├── tracking.go             # tracking-link / open-pixel rewriting
│   │   ├── repository.go           # repository interfaces (domain-owned)
│   │   ├── messenger.go            # Messenger + RateLimiter interfaces (domain-owned)
│   │   └── errors.go
│   ├── app/
│   │   ├── application.go
│   │   ├── command/                # CreateTemplate, UpdateTemplate, CreateCampaign,
│   │   │                           # UpdateCampaign, StartCampaign, SendTransactional
│   │   └── query/                  # ListTemplates, GetTemplate, ListCampaigns,
│   │                               # GetCampaign (with progress)
│   └── adapters/
│       ├── templates_pg.go
│       ├── campaigns_pg.go
│       ├── recipients_pg.go
│       ├── tracking_pg.go          # links, link_clicks, campaign_views
│       ├── start_worker.go         # River worker for campaign.start
│       ├── batch_worker.go         # River worker for campaign.batch
│       └── *_test.go
├── platform/
│   ├── postbox/                   # NEW shared — Postbox SES-compatible client
│   │   ├── client.go               # CreateEmailIdentity, GetEmailIdentity, SendEmail
│   │   ├── sigv4.go                # AWS SigV4 request signing wrapper
│   │   └── *_test.go
│   ├── ratelimit/                 # NEW shared — Redis sliding-window limiter
│   │   ├── limiter.go              # per-tenant + global sliding window (Lua script)
│   │   └── *_test.go
│   └── jobs/
│       └── jobs.go                 # EXTEND — add campaign/domain job args & kinds
├── api/
│   ├── server.go                   # EXTEND — mount sending, campaign, tx, tracking routes
│   ├── sending_handlers.go          # NEW
│   ├── campaign_handlers.go         # NEW
│   ├── tx_handlers.go               # NEW — API-key-authenticated transactional send
│   ├── tracking_handlers.go         # NEW — public open-pixel / click-redirect
│   ├── apikey_middleware.go         # NEW — API-key auth for the tx route
│   └── errmap.go                    # EXTEND — map new context error slugs
├── service/
│   └── application.go               # EXTEND — wire sending + campaign contexts
├── config/
│   └── config.go                    # EXTEND — Postbox creds, Redis DSN, queues, limits
└── db/migrations/
    ├── 000008_sending_domains.{up,down}.sql       # NEW
    ├── 000009_templates_campaigns.{up,down}.sql   # NEW
    └── 000010_campaign_tracking.{up,down}.sql     # NEW

cmd/
├── worker/main.go                   # EXTEND — register the 3 new River workers
└── scheduler/main.go                # EXTEND — periodically enqueue domain.verify
```

**Structure Decision**: Two new bounded contexts (`internal/sending`,
`internal/campaign`) follow the project's calibrated three-layer DDD layout
documented in `PATTERNS.md` — `domain`/`app`/`adapters` per context, sharing the
single `internal/api` transport layer and the `internal/service` composition
root. Cross-cutting integrations that both campaign sending and transactional
sending need — the Postbox client and the Redis rate limiter — live once in
`internal/platform/` (per the constitution's "shared infrastructure lives once"),
each behind an interface owned by the `campaign`/`sending` domain that consumes
it. River job infrastructure is extended in place in `internal/platform/jobs`,
matching how Phase 2's import/export jobs are organized.

## Complexity Tracking

> No constitution violations — this section is intentionally empty.
