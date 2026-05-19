# Contract: Billing River Jobs

Three new durable River job kinds, defined in `internal/platform/jobs/jobs.go`
alongside the Phase 1–4 args, and consumed by workers on `cmd/worker`. They run
on the existing send queue (`WorkerSendQueue`) unless a dedicated `billing`
queue is judged worthwhile during implementation.

## billing.sweep

Periodic. The scheduler enqueues it on the `BILLING_SWEEP_INTERVAL` tick with
`UniqueOpts{ByArgs: true}` so a slow sweep is never stacked. The worker finds
every subscription that needs a charge and fans out `billing.charge` jobs.

```go
type BillingSweepArgs struct{}
func (BillingSweepArgs) Kind() string { return "billing.sweep" }
```

**Worker behaviour** (`sweep_worker.go`):
1. Call the `SECURITY DEFINER` SQL function `billing_due_subscriptions()`,
   which returns `(tenant_id, subscription_id, reason)` for:
   - `reason = 'renewal'` — `state = 'active'`, `current_period_end <= now()`.
   - `reason = 'dunning'` — `state = 'past_due'` with an `open` invoice whose
     `next_attempt_at <= now()`.
2. Enqueue one `billing.charge` per row, `UniqueOpts{ByArgs: true}` keyed on the
   subscription id — a charge already pending for that subscription is not
   re-stacked.

The sweep is pure fan-out; it changes no billing state itself, so a re-run is
harmless.

## billing.charge

The single charge code path (research R12). Carries only the subscription id —
all state is in Postgres.

```go
type BillingChargeArgs struct {
    TenantID       string `json:"tenant_id"`
    SubscriptionID string `json:"subscription_id"`
}
func (BillingChargeArgs) Kind() string { return "billing.charge" }
```

**Worker behaviour** (`charge_worker.go`), all inside one tenant-bound
transaction, delegating to the shared `ChargeInvoice` command:
1. Load the subscription.
2. Resolve the invoice to charge:
   - `state = 'active'` and period ended → **renewal**: insert the next
     period's invoice with a `subscription` line item. The unique
     `(subscription_id, period_start)` constraint makes a concurrent/retried
     insert a no-op — on conflict, load the existing invoice.
   - `state = 'past_due'` → **dunning**: use the existing `open` invoice.
3. If the invoice already has a `succeeded` payment attempt, stop — exactly-once
   guard (research R5).
4. Append an `overage` line item if the closing period metered overage in
   `meter` mode (sum from `usage_counters`).
5. Call `PaymentGateway.Charge` with `IdempotencyKey = invoice_id + ":" +
   attempt_number`.
6. Record a `payment_attempt` with the outcome.
7. **On success**: invoice → `paid`, `paid_at` set; subscription → `active`,
   `current_period_*` advanced by the plan's `billing_period`, invoice
   `attempt_count` reset.
8. **On failure**: invoice `attempt_count++`. If
   `attempt_count >= DUNNING_MAX_ATTEMPTS` → invoice `uncollectible`,
   subscription → `suspended` (FR-014). Else set
   `next_attempt_at = now() + DUNNING_RETRY_INTERVAL`, subscription →
   `past_due` (FR-013).

Idempotency: the unique invoice constraint, the "already paid" guard, and the
gateway idempotency key together guarantee at most one successful charge even
if River retries the job or a gateway response is lost (research R5).

## usage.rollup

Periodic, per tenant. The scheduler enqueues one per active tenant on the
`USAGE_ROLLUP_INTERVAL` tick (mirroring `analytics.refresh`), with
`UniqueOpts{ByArgs: true}`.

```go
type UsageRollupArgs struct {
    TenantID string `json:"tenant_id"`
}
func (UsageRollupArgs) Kind() string { return "usage.rollup" }
```

**Worker behaviour** (`rollup_worker.go`), inside one tenant-bound transaction:
1. Select `usage_events` for the tenant where `rolled_up_at IS NULL`, grouped by
   `(period_start, event_type)`.
2. Upsert each group into `usage_counters` on
   `(tenant_id, period_start, event_type)`, adding the group's
   `SUM(quantity)` to `total_quantity` and recomputing `included_quantity` /
   `overage_quantity` against the subscription's plan allowance.
3. Stamp the processed events' `rolled_up_at = now()` in the same transaction.

Idempotency: only `rolled_up_at IS NULL` rows are read, and they are stamped in
the same transaction as the counter upsert — a re-run or a retried job sees no
unprocessed rows and counts nothing twice (FR-020).

## Scheduler changes (`cmd/scheduler`)

Two new tickers, alongside the existing domain-verify and analytics-refresh
tickers:
- `BILLING_SWEEP_INTERVAL` (default 1h) → enqueue one `billing.sweep`.
- `USAGE_ROLLUP_INTERVAL` (default 15m) → enqueue one `usage.rollup` per active
  tenant (reuse the active-tenant query already used for `analytics.refresh`).

Both are also enqueued once at startup so a freshly started scheduler does not
wait a full interval.

## Worker changes (`cmd/worker`)

Register three workers on the River `Workers` set: `NewSweepWorker`,
`NewChargeWorker(billingApp, gateway)`, `NewRollupWorker(billingApp)`. The
`MockGateway` is constructed once at the composition root and injected.
