# Phase 0 Research: Deliverability & Analytics

This document resolves the open technical questions behind the Phase 4 plan.
Each entry records the decision, why it was chosen, and what was rejected.

## R1 — Consuming the Postbox feedback stream

**Decision**: Postbox delivers all delivery feedback by writing JSON
notifications to a **Yandex Data Streams** stream (a YDB-style topic). The
platform consumes it with **`github.com/ydb-platform/ydb-go-sdk/v3`** — the
official Yandex Database / Data Streams Go SDK — using its **topic reader**. A
new long-lived `cmd/consumer` service opens a reader bound to the configured
topic path and a registered **consumer name**; it reads messages in batches,
hands each notification to the deliverability app, and commits the message once
it is staged. The topic, endpoint, database path, consumer name, and credentials
are supplied as configuration; credentials use the SDK's standard mechanisms
(service-account key file, metadata, or static credentials).

**Rationale**: The YDB SDK's topic reader maintains the **consumer offset
server-side**: after a restart the reader resumes exactly where it committed,
neither losing nor re-counting notifications. That satisfies FR-010 directly,
with no offset/checkpoint table to build or operate. The SDK is the provider's
own first-party client for this transport, so partition handling, reconnection,
and backoff are handled for us.

**Alternatives considered**:
- *AWS Kinesis-compatible API* (Data Streams also exposes one): rejected — it
  would mean DIY shard iterators and a self-managed checkpoint table to satisfy
  FR-010, reimplementing what the topic consumer already does server-side.
- *An inbound HTTP webhook* (the original spec assumption): rejected — Postbox
  does not call a webhook; it only writes to the stream. There is nothing to
  receive.

## R2 — Attributing a notification to the originating send

**Decision**: Persist the **provider message ID** returned by
`postbox.Client.SendEmail` for every send, and attribute notifications by it.
Each notification carries it as `mail.messageId`. For campaign sends, a
`provider_message_id` column on `campaign_recipients` (the per-recipient send row
already exists) holds it. For transactional sends, which have no persisted row
today, a new `transactional_messages` table written by the `SendTransactional`
handler holds it. The `feedback.process` job resolves the owning tenant from the
provider message ID via a `SECURITY DEFINER` lookup, then matches the message ID
to a `campaign_recipients` row or a `transactional_messages` row inside the
tenant transaction.

**Rationale**: The provider message ID is the only stable, unambiguous key tying
a notification back to a specific send; matching on recipient email alone is
ambiguous when an address is mailed by several campaigns. The `SECURITY DEFINER`
lookup mirrors the proven Phase 3 `tracking_tenant_for_link` pattern, so the
consumer can resolve a tenant before binding `app.tenant_id`.

**Alternatives considered**:
- *Match on recipient email + most recent send*: rejected — ambiguous and wrong
  whenever the same address is in two campaigns; also cannot distinguish
  campaign from transactional.
- *A custom tracking tag echoed by the provider*: rejected — the provider's
  `tags` are best-effort; `mail.messageId` is always present.

## R3 — Notification format and event-kind mapping

**Decision**: Parse the documented Postbox notification schema (see
`docs/postbox-notifications.md`). Each notification is a JSON object with a
top-level `eventType`, a `mail` object carrying `messageId` and `timestamp`, a
per-kind nested object (`bounce`, `complaint`, `delivery`, `open`, `click`,
etc.), and an `eventId`. Phase 4 maps event types as follows:

| Postbox `eventType` | Phase 4 handling |
| --- | --- |
| `Bounce` | Recorded as a `delivery_events` bounce. Every bounce is permanent (hard) — `bounce.bounceType` is always `Permanent`. Drives suppression. |
| `Complaint` | Recorded as a `delivery_events` complaint. Drives suppression. |
| `Delivery` | Recorded as a `delivery_events` delivery. Feeds the analytics *delivered* count. |
| `Open` | Recorded as a `delivery_events` open. Feeds the analytics *opened* count. |
| `Click` | Recorded as a `delivery_events` click. Feeds the analytics *clicked* count. |
| `Send`, `DeliveryDelay`, `Unsubscribe` | Read past and committed without recording — not in Phase 4 scope. |

The recipient address is taken from the kind-specific object
(`bounce.bouncedRecipients[].emailAddress`,
`complaint.complainedRecipients[].emailAddress`,
`delivery.recipients[]`, or `mail.commonHeaders.to`), the occurred-at time from
the kind-specific `timestamp`, the provider message id from `mail.messageId`, and
the dedupe key from `eventId`.

**Rationale**: There is no soft/transient bounce in the Postbox model — a
`Bounce` event is always `Permanent`; transient delays arrive as separate
`DeliveryDelay` events, which Phase 4 ignores (clarified 2026-05-18). Sourcing
opens, clicks, and deliveries from the stream gives a single, consistent feedback
table and removes Phase 4's dependence on the Phase 3 tracking pixel/redirect for
analytics.

**Alternatives considered**:
- *Soft/hard bounce classification*: rejected — the provider does not model a
  transient bounce as a `Bounce`, so a soft-bounce tally and threshold would key
  off an event type that never arrives.
- *Keep Phase 3 pixel/redirect as the open/click source*: rejected per the
  2026-05-18 clarification — the stream is now the analytics source of truth for
  opens and clicks.

## R4 — Analytics storage: materialized view vs. summary table

**Decision**: Serve analytics from a regular **`campaign_analytics` summary
table** carrying `tenant_id` and protected by Row-Level Security, refreshed by a
periodic `analytics.refresh` River job. A native PostgreSQL materialized view is
**not** used.

**Rationale**: Constitution Principle I requires tenant isolation enforced by the
data layer, not application code. A native materialized view cannot have an RLS
policy, and the RLS of its base tables does not propagate to it — any query
against the matview would depend on a hand-written `WHERE tenant_id = …` filter
in application code, the single-point-of-failure the constitution forbids. A
plain table with `ENABLE/FORCE ROW LEVEL SECURITY` gives the same pre-computed,
periodically-refreshed read path the feature asks for, with a first-class RLS
policy identical to every other tenant table.

**Alternatives considered**:
- *Native materialized view + `security_barrier` view on top*: rejected — adds a
  second database object and still enforces isolation through a hand-written
  filter rather than an RLS policy.
- *Compute analytics on the fly per request*: rejected — fails SC-007 (sub-2s
  render for a 100k-recipient campaign) because it rescans raw event tables on
  every view.

## R5 — Refreshing analytics without breaking isolation

**Decision**: The `analytics.refresh` job refreshes **per tenant inside a
tenant-bound transaction**. The scheduler enqueues one `analytics.refresh` job
per active tenant on a fixed interval (default 60s, configurable); each job sets
`app.tenant_id`, then upserts that tenant's `campaign_analytics` rows from a
`GROUP BY` aggregate over `campaign_recipients` (the *sent* count) and
`delivery_events` (distinct recipients per kind for *delivered*, *opened*,
*clicked*, *bounced*, *complained*). Recompute is full per campaign — counts are
cheap aggregates — keeping the job idempotent and resumable.

**Rationale**: Running the aggregate inside the tenant transaction means every
read and write the job performs is RLS-checked — the refresh cannot accidentally
mix tenants. Per-tenant jobs inherit River's per-tenant fairness, so one large
tenant's refresh cannot starve another's, satisfying Principle V. A 60s interval
comfortably meets SC-008 (events visible within 5 minutes).

**Alternatives considered**:
- *One global refresh job aggregating all tenants*: rejected — it would have to
  bypass RLS to read every tenant's rows, weakening the isolation guarantee.
- *Incremental/event-driven refresh*: rejected for this phase as premature
  optimization (YAGNI); the periodic full recompute is simple and well within
  the latency budget.

## R6 — Idempotency across re-reads, retries, and duplicates

**Decision**: The consumer does the minimum synchronous work per notification:
parse it, derive the dedupe key from `eventId`, `INSERT … ON CONFLICT DO NOTHING`
into the control-plane `inbound_feedback_events` table, enqueue a
`feedback.process` job keyed to that row, then commit the topic message. All
attribution and suppression happen in the job. A re-read or duplicate
notification hits the unique constraint, inserts nothing, and re-enqueues a job
that finds the row already processed and no-ops. The `feedback.process` job is
itself idempotent: the `delivery_events` `UNIQUE(inbound_event_id)` constraint
plus `suppression_list` `ON CONFLICT DO NOTHING` make a retried job converge to
the same state.

**Rationale**: Staging with a unique key at ingestion makes the whole pipeline
idempotent regardless of provider duplicates, consumer restarts that re-read
uncommitted messages, or River job retries — each distinct event is recorded
exactly once (FR-007, SC-003). Committing the topic message only *after* the row
is staged and the job enqueued means a crash mid-notification simply re-reads it,
losing nothing (FR-010, SC-001).

**Alternatives considered**:
- *Process attribution + suppression synchronously in the consumer*: rejected —
  it would couple consumer throughput to database contention and make a failure
  mid-attribution lose progress; the staged row + River job keep it durable.
- *De-duplicate only inside the job*: rejected — staging with a unique constraint
  at ingestion is the simplest place to make the whole pipeline idempotent and
  also gives a durable record of every notification received.
