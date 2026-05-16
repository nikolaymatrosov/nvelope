# In-House Billing Engine — Design

> Replaces the planned Stripe integration with a self-hosted subscription engine.
> Reason: Stripe is not available in Russia.

## Background

The architecture originally delegated subscription billing, metered overage, and
invoices to **Stripe**. Stripe does not operate in Russia, so nvelope must own the
billing logic itself.

"In-house billing" does **not** mean processing card transactions ourselves — that
requires a licensed acquirer. It means nvelope owns the **subscription engine**
(plan state, recurring renewal scheduling, dunning, overage calculation, invoices,
quota enforcement) and reduces the payment provider to two operations behind an
interface: tokenize a card, and charge a token.

## Decisions

| Topic | Decision |
|---|---|
| Payment provider | Provider-agnostic `PaymentGateway` interface |
| First implementation | `MockGateway` only; real RU provider (YooKassa / CloudPayments / T-Bank) deferred to a later phase |
| Customer types | Card payments only, with auto-renewal |
| Overage | Per-plan: `block` (hard-stop at quota) or `meter` (allow + bill overage) |
| Renewal trigger | Scheduler `billing.sweep` finds due subscriptions, enqueues `billing.charge` jobs |
| Payment failure | Dunning: `past_due` + scheduled retries, then `suspended` |
| Currency | RUB (minor units) |

## Architecture

### Approach: Scheduler sweep

The existing Scheduler service runs a periodic `billing.sweep` (~every 15 min) that
queries for subscriptions due to renew or due for a dunning retry, and enqueues one
`billing.charge` River job per subscription on the Worker. This mirrors existing
patterns (Scheduler "enqueues periodic jobs", River job types, listmonk-style
polling), makes retries trivial (just rows the next sweep picks up), and is
self-healing if a pod crashes.

Rejected alternative — per-subscription scheduled job at exact `current_period_end`:
more precise timing, but retries require rescheduling, lost jobs need a reconciliation
sweep anyway, and it adds a second pattern to maintain.

### Components — `internal/billing`

| Unit | Responsibility |
|---|---|
| `gateway` | `PaymentGateway` interface (`Tokenize`, `Charge`, `Refund`) + `MockGateway`. The only provider-specific seam. |
| `plans` | Plan CRUD, plan limits + overage rates. Control-plane. |
| `subscriptions` | Subscription state machine: create, renew, cancel, dunning transitions. |
| `invoices` | Invoice generation (plan + overage line items), retrieval, PDF render. |
| `quota` | Quota checks at campaign-start / tx-send; reads `usage_counters`, applies block/meter rules. |

Renewal jobs (`billing.sweep`, `billing.charge`) live in `internal/jobs` alongside
existing job types, not in `billing`.

The `metering` package is unchanged — it still emits `usage_events` and rolls up
`usage_counters`. Billing only *reads* counters.

## Data Model

All control-plane (no RLS). Changes relative to the current architecture doc are noted.

### `plans` (modified)

| Column | Notes |
|---|---|
| id, name | |
| price_amount | minor units (RUB) |
| billing_interval | `month` / `year` |
| limits | JSONB: max_subscribers, emails_per_month, max_domains, max_users |
| overage_mode | `block` / `meter` |
| overage_rates | JSONB: price per 1k emails / per subscriber over quota |
| active | |

### `tenant_subscriptions` (modified)

| Column | Notes |
|---|---|
| tenant_id, plan_id | |
| status | `trialing` / `active` / `past_due` / `suspended` / `canceled` |
| current_period_start, current_period_end | |
| card_token | gateway token |
| card_last4, card_brand | display only |
| dunning_attempt | int, retry counter |
| next_retry_at | when the next dunning charge is due |
| cancel_at_period_end | bool |

`stripe_subscription_id` is removed.

### `invoices` (new)

id, tenant_id, subscription_id, number, status (`draft`/`paid`/`failed`/`void`),
period_start, period_end, currency, total_amount, issued_at, paid_at.

### `invoice_line_items` (new)

id, invoice_id, kind (`plan`/`overage`), description, quantity, unit_amount, amount.

### `payment_attempts` (new)

id, invoice_id, gateway, gateway_payment_id, status (`pending`/`succeeded`/`failed`),
amount, error_code, created_at.

Separate from `invoices` so each dunning retry is its own auditable row against one
invoice.

`usage_events` / `usage_counters` are unchanged.

## Subscription Lifecycle & Dunning

State enum: `trialing` → `active` → `past_due` → `suspended` → `canceled`.

```
(signup) → trialing ──card added──→ active
                          │
            period ends   ▼
        ┌──── billing.charge on invoice ────┐
        │ success                    failure │
        ▼                                    ▼
   active (new period)                   past_due
                                             │
                              billing.sweep finds next_retry_at due
                                             │ enqueues billing.charge
                          ┌──────────────────┴───────────────┐
                     success                          failure (attempt++)
                          ▼                                   │
                   active (new period)        attempt < 4 → reschedule retry
                                              attempt = 4 → suspended
```

- **Retry schedule:** the initial renewal charge plus 4 dunning retries on days 1, 3, 5,
  7 after period end. `dunning_attempt` counts the retries (1–4); when retry 4 fails the
  subscription is suspended. The schedule is a configurable constant.
- **`suspended`** sets `tenants.status = suspended` — blocks sending, preserves data
  (per architecture §6). The owner re-activates by adding a working card, which
  triggers an immediate `billing.charge`.
- **Cancellation:** `cancel_at_period_end` → the sweep transitions to `canceled` at
  period end instead of charging. No proration.
- **`billing.sweep`** (Scheduler, ~every 15 min) selects subscriptions where
  `current_period_end <= now` (active, due to renew) OR `next_retry_at <= now`
  (past_due), and enqueues one **unique** `billing.charge` job per subscription.
  River unique-job support prevents double-charging if sweeps overlap.

## Overage & Quota

- At period end, before charging, `subscriptions` asks `quota` for the closed
  period's overage by comparing `usage_counters` against plan limits.
- **`overage_mode = block`:** no overage line item; `quota` already hard-blocked
  sends during the period when counters reached the limit.
- **`overage_mode = meter`:** sends were allowed past quota; overage units (emails
  over quota, in 1k blocks) become an `overage` line item priced by
  `plans.overage_rates`.
- **Quota enforcement** (`quota` package) runs at campaign-start and tx-send:
  `block` plans reject over-limit requests; `meter` plans allow them and let metering
  accumulate. Soft-warn near limit (UI banner) stays as architecture §6 describes.

## Invoices

- `billing.charge` first **builds the invoice** (status `draft`): one `plan` line
  item plus any `overage` line items, totaled.
- It then calls `gateway.Charge(card_token, total)`, recording a `payment_attempt`.
- Success → invoice `paid`, subscription period advances. Failure → invoice `failed`,
  dunning begins.
- Invoice retrieval via API; **PDF render** is server-side from invoice rows (Go
  template → PDF library). No external invoice service.
- Each new period with overage produces a fresh invoice; dunning retries reuse the
  *same* invoice (new `payment_attempt` rows).

## PaymentGateway Interface & Mock

```go
type PaymentGateway interface {
    Name() string
    Tokenize(ctx context.Context, c CardDetails) (Token, error)        // dev/test; real flow uses hosted fields
    Charge(ctx context.Context, r ChargeRequest) (ChargeResult, error)
    Refund(ctx context.Context, r RefundRequest) (RefundResult, error)
}
```

- `ChargeRequest` carries an **idempotency key** (= `payment_attempt` id) and
  **receipt fields** (customer email, line items) — unused by the mock, but present
  so a real RU provider can fiscalize (54-ФЗ) without an interface change.
- **`MockGateway`** is deterministic: configurable to succeed, or to fail for
  specific tokens/amounts (e.g. token `tok_decline` always fails), so dunning and the
  full lifecycle are testable end-to-end without a real provider.
- The gateway is selected by config (`BILLING_GATEWAY=mock`); real providers slot in
  later as additional implementations.

## API Surface

Billing is platform-level, so routes live in the **platform area**, not the tenant
admin app:

- `GET /api/platform/plans` — available plans
- `GET /api/platform/tenants/{id}/subscription` — current subscription + card summary
- `POST /api/platform/tenants/{id}/subscription` — choose / change plan
- `POST /api/platform/tenants/{id}/payment-method` — add/replace card; triggers an
  immediate charge if the subscription is `past_due`
- `POST /api/platform/tenants/{id}/subscription/cancel` — set `cancel_at_period_end`
- `GET /api/platform/tenants/{id}/invoices` — list invoices
- `GET /api/platform/tenants/{id}/invoices/{id}` — invoice detail
- `GET /api/platform/tenants/{id}/invoices/{id}/pdf` — PDF download

The **Stripe webhook receiver** (architecture §1) is removed — charges complete
synchronously inside the `billing.charge` job; no inbound webhook is needed for the
mock gateway. A real provider needing async confirmation would add its own receiver
when integrated.

## Testing

- **Unit:** state-machine transitions, dunning schedule, overage calculation,
  invoice totaling.
- **Integration via `MockGateway`:** happy-path renewal; decline → 4 retries →
  `suspended`; recovery after a card update.
- **Quota:** `block` rejects over-limit, `meter` allows and produces an overage line
  item.

## Out of Scope

- Real payment-provider integration (YooKassa / CloudPayments / T-Bank) — a later phase.
- 54-ФЗ fiscalization implementation — interface carries the data; deferred.
- Bank-transfer / legal-entity invoicing (счёт, акт, УПД).
- Proration on plan change mid-period.
