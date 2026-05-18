# Contract: River Jobs — Deliverability & Analytics

Two new River job kinds, registered in `internal/platform/jobs/jobs.go` and run
by `cmd/worker`. Job args carry only identifiers — all state lives in
PostgreSQL, so the workers stay stateless and the jobs are resumable across
restarts.

The Postbox feedback stream itself is **not** a River job — it is consumed by
the long-lived `cmd/consumer` service (see `contracts/ports.md`,
`FeedbackStream`). The consumer's only handoff to the queue is enqueuing
`feedback.process`.

## `feedback.process`

**Args** (`FeedbackProcessArgs`)

```go
type FeedbackProcessArgs struct {
    InboundEventID string `json:"inbound_event_id"`
}
func (FeedbackProcessArgs) Kind() string { return "feedback.process" }
```

Enqueued by the `IngestNotification` command (called by `cmd/consumer`)
immediately after staging an `inbound_feedback_events` row. Runs on the sending
queue.

**Behaviour**

1. Load the `inbound_feedback_events` row by id. If `status` is already
   `attributed` or `unattributed`, no-op (idempotent on retry/re-read).
2. Resolve the owning tenant via `feedback_tenant_for_message(provider_message_id)`.
3. **No tenant matched** → set `status = 'unattributed'`, `processed_at = now()`;
   emit an `unattributed` metric/log (FR-006, FR-009); finish.
4. **Tenant matched** → open a transaction bound to that tenant
   (`SET LOCAL app.tenant_id`):
   a. Match the provider message id to a `campaign_recipients` row or a
      `transactional_messages` row.
   b. Insert a `delivery_events` row (`UNIQUE(inbound_event_id)` makes this
      idempotent — a retried job that already wrote the event skips to step d).
   c. **If the event is a bounce or a complaint**, load the tenant's
      `bounce_settings` (defaults if absent) and apply suppression:
      - **bounce** and `suppress_on_hard_bounce` → upsert `suppression_list`
        with reason `hard_bounce`.
      - **complaint** and `suppress_on_complaint` → upsert `suppression_list`
        with reason `complaint`.
      `suppression_list` upserts are `ON CONFLICT (tenant_id, email) DO NOTHING`.
      A `delivery`, `open`, or `click` event records the `delivery_events` row
      and applies no suppression.
   d. Set `inbound_feedback_events.status = 'attributed'`,
      `processed_at = now()`.
5. A transient error returns it so River retries with backoff; `status` stays
   `pending` (or is set to `failed` only after retries are exhausted).

**Idempotency**: the `inbound_feedback_events.dedupe_key` (`eventId`) unique
constraint de-duplicates provider re-delivery and consumer re-reads at ingestion;
the `delivery_events` `UNIQUE(inbound_event_id)` constraint plus the
`suppression_list` `ON CONFLICT DO NOTHING` make a retried job converge to the
same state — each distinct event is recorded exactly once (FR-007, SC-003).

## `analytics.refresh`

**Args** (`AnalyticsRefreshArgs`)

```go
type AnalyticsRefreshArgs struct {
    TenantID string `json:"tenant_id"`
}
func (AnalyticsRefreshArgs) Kind() string { return "analytics.refresh" }
```

Enqueued by `cmd/scheduler` on a fixed interval (default 60s, configurable) —
one job per active tenant, so River's per-tenant fairness applies. Enqueued with
`UniqueOpts{ByArgs: true}` so a slow refresh is not stacked.

**Behaviour**

1. Open a transaction bound to `TenantID` (`SET LOCAL app.tenant_id`).
2. For every campaign of the tenant that is `running`, `paused`, or `finished`,
   recompute the six counts: *sent* from `campaign_recipients`
   (`status = 'sent'`), and *delivered* / *opened* / *clicked* / *bounced* /
   *complained* as distinct recipients per `event_kind` in `delivery_events`.
3. `INSERT … ON CONFLICT (campaign_id) DO UPDATE` the `campaign_analytics` rows,
   setting `refreshed_at = now()`.

Because every read and write happens inside the tenant-bound transaction, the
refresh is RLS-checked and cannot mix tenants (research R5). The full recompute
is idempotent and resumable — a re-run produces the same rows.

## Registration & wiring

- `cmd/worker/main.go` — register `FeedbackProcessWorker` and
  `AnalyticsRefreshWorker` on the existing send-queue River client.
- `cmd/scheduler/main.go` — add a periodic tick that lists active tenants and
  enqueues one `analytics.refresh` per tenant, alongside the existing
  `domain.verify` recovery sweep.
- `cmd/consumer/main.go` — the new feedback-stream consumer; reads the topic and
  calls the `IngestNotification` command, which stages the row and enqueues
  `feedback.process`.
- `internal/platform/jobs/jobs.go` — add the two arg types, their `Kind()`
  methods, and `EnqueueFeedbackProcess` / `EnqueueAnalyticsRefresh` on
  `SendEnqueuer`.
