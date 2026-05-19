# Phase 1 Data Model: Phase 5 — Billing & Metering

Two migrations. `000014_billing` adds the control-plane `plans` catalog and the
RLS-protected `tenant_subscriptions`, `invoices`, `invoice_line_items`, and
`payment_attempts`. `000015_usage_metering` adds the RLS-protected
`usage_events` and `usage_counters`.

All monetary amounts are `bigint` minor units (kopecks); currency is a separate
`text` code (`RUB` for all Phase 5 plans). The standard tenant policy is:

```sql
ALTER TABLE <t> ENABLE ROW LEVEL SECURITY;
ALTER TABLE <t> FORCE  ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON <t>
    USING      (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);
```

## Entities

### Plan — `plans` (control-plane, NO RLS)

A purchasable offering. The same catalog for every tenant.

| Column | Type | Notes |
|---|---|---|
| `id` | `uuid` PK | `gen_random_uuid()` |
| `code` | `text` UNIQUE | stable machine code, e.g. `starter` |
| `name` | `text` NOT NULL | display name |
| `price_minor` | `bigint` NOT NULL | recurring fee in minor units |
| `currency` | `text` NOT NULL | `CHECK (currency = 'RUB')` for Phase 5 |
| `billing_period` | `interval` NOT NULL | e.g. `'1 month'` |
| `included_sends` | `bigint` NOT NULL | sends covered by the base fee |
| `overage_mode` | `text` NOT NULL | `CHECK IN ('block','meter')` |
| `overage_price_minor` | `bigint` NOT NULL DEFAULT 0 | per-send price past the allowance (`meter` mode) |
| `status` | `text` NOT NULL | `CHECK IN ('draft','published','archived')` |
| `created_at` / `updated_at` | `timestamptz` | |

Only `published` plans are subscribable. `GRANT SELECT, INSERT, UPDATE, DELETE
... TO nvelope_app`. No `tenant_id`, no RLS — a plan is not tenant data.

### Subscription — `tenant_subscriptions` (RLS)

The billing relationship between a tenant and a plan. Aggregate root.

| Column | Type | Notes |
|---|---|---|
| `id` | `uuid` PK | |
| `tenant_id` | `uuid` NOT NULL → `tenants(id)` | RLS key |
| `plan_id` | `uuid` NOT NULL → `plans(id)` | |
| `state` | `text` NOT NULL | `CHECK IN ('pending','active','past_due','suspended','canceled')` |
| `current_period_start` | `timestamptz` NOT NULL | |
| `current_period_end` | `timestamptz` NOT NULL | renewal is due at/after this |
| `cancel_at_period_end` | `boolean` NOT NULL DEFAULT false | set by a cancel request |
| `canceled_at` | `timestamptz` | when cancellation took effect |
| `created_at` / `updated_at` | `timestamptz` | |

- Partial unique index: at most one non-terminal subscription per tenant —
  `CREATE UNIQUE INDEX ON tenant_subscriptions (tenant_id) WHERE state <> 'canceled'`.
- Index on `(current_period_end)` and `(state)` for the sweep function.

**State machine** (see research R7):

```
pending  ──first charge ok──▶ active
pending  ──first charge fail─▶ past_due
active   ──renewal fail──────▶ past_due
active   ──cancel request────▶ active (cancel_at_period_end=true) ──period end──▶ canceled
past_due ──retry ok──────────▶ active
past_due ──retries exhausted─▶ suspended
suspended ──balance settled──▶ active
```

`canceled` is terminal. Transition methods on the entity reject an illegal move
with `ErrInvalidSubscriptionTransition`.

### Invoice — `invoices` (RLS)

A bill for one billing period of a subscription.

| Column | Type | Notes |
|---|---|---|
| `id` | `uuid` PK | |
| `tenant_id` | `uuid` NOT NULL → `tenants(id)` | RLS key |
| `subscription_id` | `uuid` NOT NULL → `tenant_subscriptions(id)` | |
| `period_start` | `timestamptz` NOT NULL | |
| `period_end` | `timestamptz` NOT NULL | |
| `total_minor` | `bigint` NOT NULL | sum of line-item amounts |
| `currency` | `text` NOT NULL | |
| `status` | `text` NOT NULL | `CHECK IN ('open','paid','uncollectible','void')` |
| `attempt_count` | `integer` NOT NULL DEFAULT 0 | dunning attempts so far |
| `next_attempt_at` | `timestamptz` | when the next dunning retry is due |
| `issued_at` | `timestamptz` NOT NULL DEFAULT now() | |
| `paid_at` | `timestamptz` | |
| `created_at` / `updated_at` | `timestamptz` | |

- **Unique** `(subscription_id, period_start)` — guarantees one invoice per
  subscription per period (research R5), the backbone of renewal idempotency.
- Index `(status, next_attempt_at)` for the dunning sweep.

### Invoice Line Item — `invoice_line_items` (RLS)

A single charge on an invoice.

| Column | Type | Notes |
|---|---|---|
| `id` | `uuid` PK | |
| `tenant_id` | `uuid` NOT NULL → `tenants(id)` | RLS key |
| `invoice_id` | `uuid` NOT NULL → `invoices(id)` ON DELETE CASCADE | |
| `kind` | `text` NOT NULL | `CHECK IN ('subscription','overage')` |
| `description` | `text` NOT NULL | |
| `quantity` | `bigint` NOT NULL | 1 for the base fee; overage send count |
| `unit_price_minor` | `bigint` NOT NULL | |
| `amount_minor` | `bigint` NOT NULL | `quantity * unit_price_minor` |
| `created_at` | `timestamptz` | |

### Payment Attempt — `payment_attempts` (RLS)

One attempt to charge an invoice through the gateway.

| Column | Type | Notes |
|---|---|---|
| `id` | `uuid` PK | |
| `tenant_id` | `uuid` NOT NULL → `tenants(id)` | RLS key |
| `invoice_id` | `uuid` NOT NULL → `invoices(id)` ON DELETE CASCADE | |
| `attempt_number` | `integer` NOT NULL | 1-based; part of the gateway idempotency key |
| `status` | `text` NOT NULL | `CHECK IN ('succeeded','failed')` |
| `gateway_reference` | `text` | provider's charge id (mock returns a deterministic one) |
| `failure_reason` | `text` | populated on `failed` |
| `created_at` | `timestamptz` NOT NULL DEFAULT now() | |

- Unique `(invoice_id, attempt_number)`.
- A `succeeded` row is the "already paid" guard the charge command re-checks.

### Usage Event — `usage_events` (RLS) — migration `000015`

One recorded billable action.

| Column | Type | Notes |
|---|---|---|
| `id` | `uuid` PK | |
| `tenant_id` | `uuid` NOT NULL → `tenants(id)` | RLS key |
| `event_type` | `text` NOT NULL | `CHECK IN ('campaign_send','transactional_send')` |
| `quantity` | `bigint` NOT NULL DEFAULT 1 | |
| `source_ref` | `text` NOT NULL | campaign-recipient id / transactional-message id |
| `occurred_at` | `timestamptz` NOT NULL | when the send happened |
| `period_start` | `timestamptz` NOT NULL | the billing period the event is attributed to |
| `rolled_up_at` | `timestamptz` | set when included in a counter; NULL = pending |
| `created_at` | `timestamptz` NOT NULL DEFAULT now() | |

- **Unique** `(tenant_id, event_type, source_ref)` — recording the same send
  twice is a no-op (research R11).
- Index `(tenant_id, period_start) WHERE rolled_up_at IS NULL` — the rollup
  scan and the quota gate's un-rolled tail read.

### Usage Counter — `usage_counters` (RLS) — migration `000015`

A per-tenant, per-period aggregate produced by `usage.rollup`.

| Column | Type | Notes |
|---|---|---|
| `id` | `uuid` PK | |
| `tenant_id` | `uuid` NOT NULL → `tenants(id)` | RLS key |
| `period_start` | `timestamptz` NOT NULL | |
| `period_end` | `timestamptz` NOT NULL | |
| `event_type` | `text` NOT NULL | |
| `total_quantity` | `bigint` NOT NULL DEFAULT 0 | rolled-up sends in the period |
| `included_quantity` | `bigint` NOT NULL DEFAULT 0 | within the plan allowance |
| `overage_quantity` | `bigint` NOT NULL DEFAULT 0 | beyond the allowance |
| `updated_at` | `timestamptz` NOT NULL DEFAULT now() | |

- **Unique** `(tenant_id, period_start, event_type)` — one counter per period.
  `usage.rollup` upserts this row, so a re-run never creates a duplicate.

## Relationships

```
plans (catalog)
   │ 1
   ▼ N
tenant_subscriptions ──1──N──▶ invoices ──1──N──▶ invoice_line_items
                                   │ 1
                                   ▼ N
                              payment_attempts

tenants ──1──N──▶ usage_events ──(rolled up into)──▶ usage_counters
```

## Derived read models (not tables)

- **SubscriptionView** — current plan, state, period window, and current-period
  usage vs. allowance. Returned by `GET /subscription`. Usage is computed as the
  rolled counter total plus the un-rolled `usage_events` tail (research R10).
- **InvoiceView** — an invoice with its line items and payment attempts.
  Returned by `GET /invoices/{id}`.

## Validation rules (enforced on the domain entities)

- A `Plan` is subscribable only when `status = 'published'`.
- A tenant may hold at most one non-`canceled` subscription (DB partial unique
  index + a domain pre-check returning `ErrSubscriptionExists`).
- `Money` arithmetic refuses to mix currencies.
- An `Invoice` total equals the sum of its line-item amounts (assembled
  atomically by the domain, never patched).
- Subscription state changes go only through transition methods (research R7).
- `usage.rollup` never re-counts an event: it only reads `rolled_up_at IS NULL`
  rows and stamps them within the same transaction as the counter upsert.
