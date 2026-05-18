# Implementation Plan: Phase 4 — Deliverability & Analytics

**Branch**: `008-phase-4-deliverability-analytics` | **Date**: 2026-05-18 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/008-phase-4-deliverability-analytics/spec.md`

## Summary

Phase 4 closes the deliverability feedback loop on top of the Phase 3 sending
pipeline. Today nvelope sends through Yandex Postbox but never consumes the
delivery feedback Postbox produces, so it keeps mailing dead and hostile
addresses and has no real campaign analytics.

Postbox does not call a webhook. It writes every delivery-feedback notification
— `Bounce`, `Complaint`, `Delivery`, `Open`, `Click`, and others — as JSON to a
**Yandex Data Streams** stream (a YDB-style topic). Phase 4 adds a new
`cmd/consumer` service that reads that topic with the YDB Go SDK's topic reader,
attributes each event to the originating send, automatically suppresses
hard-bouncing and complaining addresses, skips suppressed recipients before
every send, and exposes per-campaign and workspace analytics computed from the
stream events.

Technical approach: add one bounded context, `internal/deliverability`, holding
three concerns behind the calibrated `domain`/`app`/`adapters` split — feedback
ingestion, suppression, and analytics. A new long-lived `cmd/consumer` service
reads the topic through a thin `internal/platform/datastreams` client (wrapping
`github.com/ydb-platform/ydb-go-sdk/v3`); for each notification it stages a
control-plane `inbound_feedback_events` row and enqueues a durable
`feedback.process` River job, then commits the topic offset. The topic's
server-side consumer offset makes the reader resume after a restart without loss
or re-counting (FR-010). The `feedback.process` job resolves the owning tenant
from the provider message id via a `SECURITY DEFINER` lookup, records a
`delivery_events` row, and — for a bounce or complaint — updates the suppression
list. The Phase 3 send paths gain a domain-owned `SuppressionChecker` port: the
campaign `start`/`batch` workers and the transactional handler skip suppressed
recipients and record the skip. Provider message IDs returned by Postbox at send
time are persisted (on `campaign_recipients` and a new `transactional_messages`
table) so feedback can be attributed by message ID. Analytics is served from an
RLS-protected `campaign_analytics` summary table refreshed by a periodic
`analytics.refresh` job; all six counts — including opened, clicked, and
delivered — are aggregated from the `delivery_events` stream events. A native
materialized view is rejected because it cannot carry Row-Level Security (see
research R4). This phase is backend-only; the dashboard UI is a later increment.

## Technical Context

**Language/Version**: Go 1.26

**Primary Dependencies**: existing — `riverqueue/river` v0.37 (durable job queue),
`jackc/pgx/v5` (PostgreSQL), `go-chi/chi/v5` (HTTP), `redis/go-redis/v9`,
`golang-migrate/migrate/v4`, `knadh/koanf` (config). **One new dependency**:
`github.com/ydb-platform/ydb-go-sdk/v3` — the official Yandex Database / Data
Streams Go SDK, used for its topic reader to consume the Postbox feedback
stream. No inbound HTTP webhook and no AWS SDK service client are added.

**Storage**: PostgreSQL (shared database, RLS) for all new tenant-scoped tables
and River's queue tables. One new control-plane (non-tenant) table,
`inbound_feedback_events`, stages raw notifications for idempotent
de-duplication and holds unattributed events. Stream consumer offsets are
**not** stored in Postgres — they live server-side on the Data Streams topic
consumer. Redis is unchanged. Raw notification payloads are retained for
audit/debugging; rendered messages are still not persisted.

**Testing**: `go test ./...` with `testify`; integration tests against a real
`postgres:17` via `testcontainers-go` (existing `internal/dbtest` harness).
Notification parsing, idempotent ingestion, attribution, suppression, and
pre-send filtering are covered by component tests with a fake stream feed —
the YDB topic reader sits behind a domain-owned port, so tests inject staged
notifications without standing up a real Data Streams topic. Analytics refresh
correctness and cross-tenant isolation get integration tests against real
Postgres. The campaign send path's new pre-send suppression check is covered
with the existing send-pipeline component test harness.

**Target Platform**: Linux server; four stateless Go services (`cmd/api`,
`cmd/worker`, `cmd/scheduler`, and the new `cmd/consumer`) on Kubernetes. The
suppression and analytics routes are served by `cmd/api`; `feedback.process` and
`analytics.refresh` jobs run on `cmd/worker`; `cmd/scheduler` enqueues the
periodic analytics refresh; `cmd/consumer` reads the Postbox feedback topic.

**Project Type**: Web service (Go backend). This phase is backend-only.

**Performance Goals**: The stream consumer keeps the unread-notification backlog
bounded under normal and burst load (SC-010) by doing only a parse, one
idempotent insert, and one job enqueue per notification before committing the
offset. Attribution and suppression run asynchronously. A campaign analytics
view renders in under 2 seconds for a 100,000-recipient campaign (SC-007)
because it reads pre-computed `campaign_analytics` rows, never raw events.
Analytics reflect new events within 5 minutes (SC-008), bounded by the refresh
interval.

**Constraints**: Tenant isolation is the data layer's job (RLS), never
application code alone — this rules out a native materialized view for analytics
(matviews cannot carry RLS). The consumer resolves the owning tenant from the
provider message ID through a `SECURITY DEFINER` lookup, mirroring the Phase 3
tracking pattern. The Data Streams topic is a trusted, access-controlled channel
— the consumer authenticates to it with the platform's own credentials and there
is no per-notification signature to verify. The inbound topic is reached only
through the thin `internal/platform/datastreams` abstraction. All feedback and
analytics-refresh work is durable and resumable: the topic's consumer offset
recovers the reader, and River recovers the jobs.

**Scale/Scope**: 3 user stories, 26 functional requirements, ~4 key entities.
Roughly: 3 new migrations, 1 new bounded context (`deliverability`), 1 new
service (`cmd/consumer`), 2 new River job kinds + 2 workers, 6 new authenticated
tenant routes, small edits to the Phase 3 campaign send path and transactional
handler to persist provider message IDs and apply the pre-send suppression
check.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

Evaluated against `.specify/memory/constitution.md` v1.1.0.

| Principle | Status | Notes |
| --- | --- | --- |
| I. Tenant Isolation by Default | PASS | Every new tenant-plane table (`delivery_events`, `suppression_list`, `bounce_settings`, `transactional_messages`, `campaign_analytics`) carries `tenant_id` and `ENABLE`/`FORCE ROW LEVEL SECURITY` with the same `app.tenant_id` policy as Phase 2/3. The consumer learns the tenant only after resolving the provider message ID via a `SECURITY DEFINER` lookup (the Phase 3 `tracking_tenant_for_*` pattern); the `feedback.process` job then runs inside that tenant's bound transaction. Analytics is an RLS-protected summary table, *not* a native materialized view — see research R4. Unattributed events stay in the control-plane `inbound_feedback_events` table, never in a tenant table with a guessed tenant. Cross-tenant isolation tests are added for every new repository and for the analytics query. |
| II. Test-Backed Delivery | PASS | Critical paths — feedback ingestion, async job processing, and the pre-send suppression gate — get integration coverage against real boundaries: Postgres + River via testcontainers, a fake stream feed for the consumer, and resumability tests (worker killed mid-`feedback.process` → event recorded exactly once). Idempotent de-duplication of re-read notifications and analytics-refresh correctness are explicit tests. Phase exits with green `go test ./...` and a clean migration apply. |
| III. Incremental, Shippable Phases | PASS | Three independently shippable slices: US1 feedback ingestion, US2 suppression + pre-send checks, US3 analytics. Build for this phase only — reputation scoring, provider failover, and the dashboard UI are explicitly out of scope (spec Assumptions). This phase completes Epic F. |
| IV. Security & Consent by Design | PASS | The consumer reaches the feedback topic only over an authenticated, least-privilege channel using the platform's own Data Streams credentials, which are secret config and never logged. The topic is a trusted source — only Postbox writes to it — so authenticity is established by the access-controlled channel, not a per-record signature. Suppression *protects* recipient consent by guaranteeing complained addresses are never mailed again. Manual suppression add/remove are privileged tenant actions written to the existing `audit_log`. |
| V. Operable & Observable Services | PASS | `cmd/api`, `cmd/worker`, `cmd/scheduler`, and `cmd/consumer` stay stateless: inbound notifications are staged in Postgres, job state in River, and the stream position lives server-side on the topic consumer — no in-process work state survives a restart. Feedback processing and analytics refresh are durable, retry-capable River jobs; the `inbound_feedback_events` dedupe key makes a re-read or retried job record an event exactly once. Unattributed events and consumer failures are surfaced as metrics/structured logs (FR-009). Every new command/query handler keeps the standard logging/metrics decorator. The single-instance consumer is a documented choice — see Complexity Tracking. |
| VI. Layered Architecture & Domain Integrity | PASS | The new `deliverability` context uses the calibrated `domain`/`app`/`adapters` split. `DeliveryEvent` and `SuppressionEntry` are rich entities with unexported fields, validating constructors, and a separate documented hydration path; classification (suppress-or-not) is domain behaviour, not handler `if`s. The `FeedbackStream` reader port and the `SuppressionChecker` port consumed by the Phase 3 campaign/transactional send paths are declared by the consuming layer; `deliverability` adapters conform. Errors carry slugs via the shared `apperr` package; transport mapping stays in `api/errmap.go`. Wiring is plain constructors in `service/`. |

**Gate result: PASS — two documented design choices, recorded in Complexity Tracking.**

One new dependency is introduced — the YDB Go SDK — because consuming a Yandex
Data Streams topic is the provider's actual feedback mechanism and the SDK's
topic reader gives server-side consumer offsets that satisfy FR-010 directly.
The matview-vs-summary-table decision and the single-instance consumer decision
are recorded below.

Re-check after Phase 1 design: **PASS** — the data model adds only RLS-protected
tenant-plane tables plus one control-plane staging table; contracts add no
transport leakage into domain code; the stream reader sits behind a domain-owned
port and the pre-send check behind a domain-owned port. Design holds the gate.

## Project Structure

### Documentation (this feature)

```text
specs/008-phase-4-deliverability-analytics/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/
│   ├── http-api.md      # Phase 1 output — analytics + suppression routes
│   ├── jobs.md          # Phase 1 output — feedback.process & analytics.refresh
│   └── ports.md         # Phase 1 output — domain-owned Go interfaces
└── tasks.md             # Phase 2 output (/speckit-tasks — NOT created here)
```

### Source Code (repository root)

```text
internal/
├── deliverability/                  # NEW bounded context
│   ├── domain/
│   │   ├── event.go                  # DeliveryEvent aggregate (kind, attribution)
│   │   ├── notification.go           # InboundNotification value type, event kinds
│   │   ├── suppression.go            # SuppressionEntry aggregate (reason, source event)
│   │   ├── settings.go               # BounceSettings (hard-bounce/complaint toggles)
│   │   ├── stream.go                 # FeedbackStream reader port (domain-owned)
│   │   ├── repository.go             # event/suppression/settings/analytics repo interfaces
│   │   ├── analytics.go              # CampaignAnalytics / Dashboard read models
│   │   └── errors.go                 # context-specific slug errors
│   ├── app/
│   │   ├── application.go            # Application{Commands, Queries}
│   │   ├── command/                  # IngestNotification, ProcessFeedback,
│   │   │                              # AddSuppression, RemoveSuppression,
│   │   │                              # UpdateBounceSettings, RefreshAnalytics
│   │   └── query/                    # ListSuppressions, GetBounceSettings,
│   │                                  # GetCampaignAnalytics, GetDashboard
│   └── adapters/
│       ├── events_pg.go              # delivery_events + inbound_feedback_events repo
│       ├── suppression_pg.go         # suppression_list repo
│       ├── settings_pg.go            # bounce_settings repo
│       ├── analytics_pg.go           # campaign_analytics read + refresh
│       ├── suppression_checker.go    # SuppressionChecker adapter (campaign port)
│       ├── stream_reader.go          # FeedbackStream adapter over platform/datastreams
│       ├── notification_parse.go     # Postbox notification JSON → InboundNotification
│       ├── feedback_worker.go        # River worker for feedback.process
│       ├── analytics_worker.go       # River worker for analytics.refresh
│       └── *_test.go
├── campaign/
│   ├── domain/
│   │   └── messenger.go              # EXTEND — add SuppressionChecker interface
│   ├── adapters/
│   │   ├── start_worker.go           # EXTEND — skip suppressed recipients
│   │   ├── batch_worker.go           # EXTEND — persist provider_message_id, re-check
│   │   └── recipients_pg.go          # EXTEND — store provider_message_id, 'skipped' status
│   └── app/command/
│       └── transactional.go          # EXTEND — pre-send suppression check + record tx message
├── platform/
│   ├── datastreams/                  # NEW — thin YDB Data Streams topic-reader client
│   │   └── reader.go
│   └── jobs/
│       └── jobs.go                   # EXTEND — feedback.process & analytics.refresh args
├── api/
│   ├── server.go                     # EXTEND — mount suppression + analytics routes
│   ├── suppression_handlers.go       # NEW — suppression list + bounce settings routes
│   ├── analytics_handlers.go         # NEW — campaign analytics + dashboard routes
│   └── errmap.go                     # EXTEND — map deliverability error slugs
├── service/
│   └── application.go                # EXTEND — wire the deliverability context
├── config/
│   └── config.go                     # EXTEND — feedback stream + analytics settings
└── db/migrations/
    ├── 000011_delivery_feedback.{up,down}.sql    # NEW
    ├── 000012_suppression.{up,down}.sql          # NEW
    └── 000013_campaign_analytics.{up,down}.sql   # NEW

cmd/
├── consumer/main.go                  # NEW — reads the Postbox feedback topic
├── worker/main.go                    # EXTEND — register feedback & analytics workers
└── scheduler/main.go                 # EXTEND — periodically enqueue analytics.refresh
```

**Structure Decision**: One new bounded context, `internal/deliverability`,
following the project's calibrated three-layer DDD layout
(`domain`/`app`/`adapters`) documented in `PATTERNS.md`. Feedback ingestion,
suppression, and analytics are kept in a single context — they share the
`delivery_events` data and the same tenant-resolution path, and the
constitution's "layer scope is proportional to need" favours not multiplying
contexts (YAGNI). The context shares the single `internal/api` transport layer
and the `internal/service` composition root. The Data Streams topic reader is a
new thin infrastructure client in `internal/platform/datastreams`, mirroring
`internal/platform/postbox`, consumed through a domain-owned `FeedbackStream`
port. The pre-send suppression gate is a domain-owned `SuppressionChecker` port
declared in the `campaign` context and implemented by a `deliverability`
adapter, wired in `service/`. River job infrastructure is extended in place in
`internal/platform/jobs`, matching Phase 3.

## Complexity Tracking

> Constitution Check passed with two documented design choices.

| Decision | Why Needed | Alternative Rejected Because |
|----------|------------|------------------------------|
| Analytics served from an RLS-protected `campaign_analytics` summary table, not a native PostgreSQL materialized view | Principle I requires tenant isolation enforced in the data layer. A native materialized view cannot have an RLS policy and is not isolated by the RLS of its base tables, so querying it would rely on application-code filtering alone. | A native matview plus a `security_barrier` view was considered; it adds a second object and still depends on a hand-written tenant filter rather than a first-class RLS policy. The summary table gives the same pre-computed, periodically-refreshed read path (FR-024/025) with a real RLS policy, at the cost of a refresh job that already exists for the queue-driven design. |
| The `cmd/consumer` stream reader runs as a single instance rather than a horizontally-scaled pool | Principle V wants stateless, scalable services. A Data Streams topic reader holds per-partition read state; scaling it horizontally needs partition leasing/coordination across instances. | Coordinated multi-instance partition leasing was considered and deferred (YAGNI): the feedback stream is low-volume, and the topic itself is the durable buffer — a restarted single instance resumes from the server-side consumer offset with zero loss, so the service is resumable even though it is not yet horizontally scaled. Scaling by partition is a clean follow-up if volume demands it. |
