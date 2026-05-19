# Phase 0 Research: Phase 5 — Billing & Metering

All decisions were resolved from in-repo inspection of the Phase 3/4 patterns
and standard recurring-billing practice. No `NEEDS CLARIFICATION` remain.

## R1 — One `billing` context, not five

**Decision**: Put plans, subscriptions, invoicing, payment, metering, and quota
enforcement in a single bounded context, `internal/billing`, with the calibrated
`domain`/`app`/`adapters` split.

**Rationale**: These concerns share the same aggregates (a `Subscription` owns
`Invoice`s; an `Invoice` drives `PaymentAttempt`s; usage feeds the quota gate
that protects the same subscription) and the same tenant. Constitution
"layer scope is proportional to need" and YAGNI argue against multiplying
contexts. Phase 4's `deliverability` context did the same for three concerns.

**Alternatives considered**: Separate `subscription`, `payment`, and `metering`
contexts — rejected as premature ceremony with cross-context chatter for data
that is one transactional unit.

## R2 — `plans` is control-plane, the rest are tenant-scoped

**Decision**: `plans` is a platform-managed catalog with no `tenant_id` and no
RLS (like `tenants`). `tenant_subscriptions`, `invoices`, `invoice_line_items`,
`payment_attempts`, `usage_events`, and `usage_counters` each carry `tenant_id`
and `ENABLE`/`FORCE ROW LEVEL SECURITY` with the standard `app.tenant_id`
policy.

**Rationale**: A plan is the same offering for every tenant — it is not tenant
data, so an RLS policy would be meaningless. Everything else is tenant-owned
financial data and must be isolated by the data layer per Constitution I.

**Alternatives considered**: A `tenant_id`-nullable `plans` table for
tenant-custom plans — rejected; custom pricing is out of scope (spec
Assumptions) and YAGNI.

## R3 — Money as integer minor units

**Decision**: All monetary amounts are stored and computed as `bigint` minor
units (kopecks) plus a separate currency code; a `Money` value type wraps them
in the domain.

**Rationale**: Floating-point money accumulates rounding error across line
items, overage arithmetic, and retries. Integer minor units are exact. A single
plan has a single currency (spec Assumptions), so currency travels alongside
the amount but is never mixed in arithmetic.

**Alternatives considered**: `numeric(12,2)` — exact, but invites accidental
float conversion in Go and needs care with scanning; integer kopecks are
unambiguous.

## R4 — Deterministic `MockGateway` keyed on an idempotency key

**Decision**: The `PaymentGateway` port takes a `ChargeRequest` carrying an
`IdempotencyKey` (derived from `invoice_id` + `attempt_number`). The
`MockGateway` returns an outcome that is a **pure function** of that key:
approve by default, with a programmable rule set tests use to force a decline
or a gateway error for chosen keys/tenants/amounts. Re-charging the same key
returns the same outcome.

**Rationale**: Determinism makes renewal, dunning, and idempotency tests
repeatable without wall-clock or randomness. The idempotency key is exactly how
real payment providers dedupe a re-submitted charge, so the abstraction is
honest and the real gateway is a drop-in later.

**Alternatives considered**: A random/flaky mock — rejected; non-deterministic
tests violate Constitution II. Amount-pattern-only triggers (e.g. amounts
ending in `13` decline) — kept as one convenient rule but not the only one, so
tests can target a specific tenant.

## R5 — Exactly-once charging

**Decision**: Three layers guarantee a single invoice is charged at most once:
1. A unique constraint `invoices (subscription_id, period_start)` — at most one
   invoice per subscription per period, so renewal cannot double-invoice.
2. The `ChargeInvoice` command skips any invoice that already has a `succeeded`
   `payment_attempt` (re-check inside the transaction).
3. The gateway `IdempotencyKey` makes a re-submitted charge a no-op at the
   provider, covering the window where a `payment_attempt` row was written but
   the job crashed before commit.

**Rationale**: River retries jobs, and a gateway response can be lost.
Constitution V requires durable work that never duplicates. The three layers
cover the in-Postgres race, the logical "already paid" case, and the
lost-response case respectively.

**Alternatives considered**: A distributed lock — rejected; the unique
constraint plus the idempotency key are simpler and need no extra
infrastructure.

## R6 — `billing.sweep` cross-tenant read via `SECURITY DEFINER`

**Decision**: `billing.sweep` runs as a River job on `cmd/worker`. It finds
subscriptions due for renewal or a dunning retry through a `SECURITY DEFINER`
SQL function `billing_due_subscriptions()` that returns only
`(tenant_id, subscription_id, reason)`. The job then enqueues one
`billing.charge` per row. `billing.charge` runs inside the resolved tenant's
RLS-bound transaction.

**Rationale**: `tenant_subscriptions` has `FORCE ROW LEVEL SECURITY`, so the
RLS-bound app role cannot see across tenants. Phase 3/4 already use a
`SECURITY DEFINER` lookup to resolve a tenant before binding a transaction
(the `tracking_tenant_for_*` pattern). Reusing it keeps RLS the authoritative
backstop and exposes only the minimal projection — never full rows.

**Alternatives considered**: Running the sweep on the privileged
(`MigrateDatabaseURL`) pool like the scheduler's domain sweep — workable, but
spec 5.2 explicitly names a `billing.sweep` *job*, and a job on the worker
keeps the heavy query off the scheduler. A `SECURITY DEFINER` function is the
established in-repo way to do a scoped cross-tenant read from the app role.

## R7 — Subscription lifecycle state machine

**Decision**: `Subscription` is an aggregate with an explicit state machine.
States and the only permitted transitions:

```
pending  → active        (first charge succeeds)
pending  → past_due      (first charge fails)
active   → past_due      (renewal charge fails)
active   → canceled      (tenant cancels; effective at period end)
past_due → active        (a retry charge succeeds)
past_due → suspended     (all dunning retries exhausted)
suspended → active       (outstanding balance settled / reinstated)
```

`canceled` and a fully elapsed cancellation are terminal. Transitions are
methods on the entity that reject an illegal move with a typed error; handlers
never mutate the state field directly.

**Rationale**: Constitution VI — business rules live on the entity, an invalid
state is unrepresentable. A spec'd, closed transition set makes dunning and
suspension auditable.

**Alternatives considered**: A free-text status with handler `if`s — rejected;
exactly the anaemic-struct smell the constitution forbids.

## R8 — Sending allowed in `past_due`, blocked in `suspended`

**Decision**: The `QuotaGate` permits sends when the subscription state is
`active` or `past_due`, and rejects them when it is `suspended`, `canceled`,
`pending`, or absent. Suspension is therefore a *subscription* state, not a flip
of `tenants.status`.

**Rationale**: `past_due` is the dunning grace window — cutting a tenant off the
instant one charge fails is hostile and premature; suspension after exhausted
retries is the real consequence (spec US5). Keeping suspension inside the
billing aggregate (not `tenants.status`) means a suspended tenant still has
read access to settle their balance (spec Assumptions), since `tenants.status`
governs authentication and the whole platform.

**Alternatives considered**: Flipping `tenants.status` to `suspended` —
rejected; that table's `suspended` value is a platform-admin action that blocks
everything including login, contradicting the spec's "read access remains"
assumption.

## R9 — Whole-campaign authorization for `block` mode

**Decision**: For a `block`-mode plan, if a campaign's full recipient count
would exceed the tenant's remaining allowance, the **entire campaign is
rejected** at campaign start and moved to a `failed`/blocked state with a
quota-exceeded reason; it is never partially sent. A `meter`-mode plan always
proceeds and records the overage. The check happens in the campaign **start
worker**, after recipients are resolved and the count is known.

**Rationale**: Spec FR-025 demands a "consistent, predictable rule". All-or-
nothing is predictable and communicated once at start; a partially-sent
campaign is confusing and hard to reason about for the tenant. "Before a
campaign starts" (FR-022) is satisfied at the start worker, the first point the
recipient count exists.

**Alternatives considered**: Capping the campaign to the remaining allowance —
rejected; a silently truncated send list is a worse surprise than a clear
rejection.

## R10 — Quota gate reads counter + un-rolled tail

**Decision**: Current-period usage = the `usage_counters` rolled total for the
period **plus** a live `SUM(quantity)` of `usage_events` for the period with
`rolled_up_at IS NULL`. See plan Complexity Tracking.

**Rationale**: `usage.rollup` is periodic, so the counter lags. A hard
`block`-mode quota enforced off a stale counter would let a tenant overshoot by
a full rollup interval. The two reads are indexed and bounded.

## R11 — Usage events recorded by the send paths, idempotently

**Decision**: The campaign batch worker records one `usage_event` per sent
recipient; the transactional handler records one per send. Each event carries a
`source_ref` (the campaign-recipient id or transactional-message id) and the
table has a unique constraint `(tenant_id, event_type, source_ref)`, so a
retried send job re-recording the same send is a no-op.

**Rationale**: Constitution V — a retried job must not duplicate work; spec
edge case "the same usage event recorded twice". The send paths already have a
stable per-send id from Phase 3/4 (`campaign_recipients`,
`transactional_messages`), so `source_ref` needs no new identifier.

**Alternatives considered**: One aggregate event per batch — loses the natural
idempotency key and makes a partially-failed batch ambiguous.

## R12 — `billing.charge` is the single charge code path

**Decision**: A shared `ChargeInvoice` command holds all charge logic (load
invoice, skip if paid, call gateway, record `payment_attempt`, advance
subscription, apply dunning on failure). The `Subscribe` command calls it
synchronously for the first charge; the `billing.charge` worker calls it for
renewals and dunning retries.

**Rationale**: Constitution — do not duplicate logic. One tested code path
covers first charge, renewal, and retry; the only difference is who invokes it
and whether a caller is waiting.

## R13 — Scheduling intervals and dunning policy as config

**Decision**: New config keys (with sane defaults, `NVELOPE_` prefixed):
`BILLING_SWEEP_INTERVAL` (default 1h), `USAGE_ROLLUP_INTERVAL` (default 15m),
`DUNNING_MAX_ATTEMPTS` (default 3), `DUNNING_RETRY_INTERVAL` (default 72h). The
scheduler enqueues `billing.sweep` and a per-tenant `usage.rollup` on these
ticks, mirroring the existing `AnalyticsRefreshInterval` pattern.

**Rationale**: Spec Assumptions leave exact dunning values to planning. Config
keys keep them tunable without code change and match the existing
`config.go` interval pattern.
