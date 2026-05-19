# Phase 1 — Data Model: Phase 5 Billing & Metering UI

This feature persists nothing. It introduces no tables and no migrations. The
"entities" below are the **view shapes** the UI consumes from the existing
Phase 5 endpoints — they are added to `frontend/src/lib/api-types.ts` so routes
work against typed data and never construct URLs or parse untyped JSON.

Field names mirror the backend JSON. Monetary values are integers in **minor
units** (e.g. kopecks); each carries a sibling `currency` (ISO code, RUB by
default).

## PlanView

A purchasable offering shown in the plan catalogue (US2). Source: `GET
/plans`.

| Field | Type | Notes |
|-------|------|-------|
| `id` | string | Plan identifier, used as `planId` in subscribe |
| `code` | string | Stable machine code |
| `name` | string | Display name |
| `priceMinor` | number | Recurring price per period, minor units |
| `currency` | string | ISO currency code |
| `billingPeriod` | string | e.g. `monthly` |
| `includedSends` | number | Allowance of metered sends per period |
| `overageMode` | `'block' \| 'meter'` | Behaviour past the allowance |
| `overagePriceMinor` | number | Per-send overage price, minor units (relevant when `overageMode === 'meter'`) |

## SubscriptionView

The tenant's billing relationship and its current usage (US1, US3). Source:
`GET /subscription` (and the body of `POST /subscription`).

| Field | Type | Notes |
|-------|------|-------|
| `id` | string | Subscription identifier |
| `plan` | `PlanRef` | `{ id, code, name }` of the subscribed plan |
| `state` | `SubscriptionState` | Lifecycle state — see enumeration below |
| `currentPeriodStart` | string (ISO datetime) | Start of the current billing period |
| `currentPeriodEnd` | string (ISO datetime) | End of the current billing period |
| `cancelAtPeriodEnd` | boolean | True when the subscription will not renew (shown read-only) |
| `usage` | `UsageView` | Embedded current-period usage |

`GET /subscription` returns `404 no_subscription` when the tenant has none —
the UI renders the no-subscription state rather than an error.

### SubscriptionState (enumeration)

`pending` → `active` → `past_due` → `suspended` → `cancelled`. The UI maps each
to a billing-home presentation:

| State | UI presentation |
|-------|-----------------|
| `pending` | In-progress state — first charge not yet resolved (FR-005) |
| `active` | Healthy — plan + period dates, reassuring (FR-001) |
| `past_due` | Prominent warning — payment failed, retries in progress, link to unpaid invoice (FR-002) |
| `suspended` | Prominent warning — suspended for non-payment, sending disabled; settle-balance offered (FR-003, FR-025) |
| `cancelled` | Subscription ended; path back to the plan catalogue |

The UI does not transition state itself — state changes are a backend
consequence of subscribe, settle, renewal, or dunning. The UI re-reads and
reconciles (FR-034).

## UsageView

Current-period consumption, embedded in `SubscriptionView.usage` (US3).

| Field | Type | Notes |
|-------|------|-------|
| `includedSends` | number | The plan allowance for the period |
| `usedSends` | number | Metered sends consumed so far this period |
| `overageSends` | number | Sends recorded as overage (meter-mode) |
| `remainingSends` | number | Allowance left; `0` when exhausted |

Figures come from the periodic `usage.rollup` job, so they lag live sends. The
UI shows a last-refreshed indication; when the first rollup has not run the
counts are `0` and the UI states figures are not yet computed (FR-014).

## InvoiceView

A bill for one billing period (US4). Source: `GET /invoices` (list, summary
fields) and `GET /invoices/{id}` (full, with `lineItems` and
`paymentAttempts`).

| Field | Type | Notes |
|-------|------|-------|
| `id` | string | Invoice identifier |
| `subscriptionId` | string | Owning subscription |
| `status` | `InvoiceStatus` | `open \| paid \| void` — see below |
| `totalMinor` | number | Invoice total, minor units |
| `currency` | string | ISO currency code |
| `attemptCount` | number | Number of payment attempts made |
| `issuedAt` | string (ISO datetime) \| null | When the invoice was issued |
| `paidAt` | string (ISO datetime) \| null | When it was paid, if paid |
| `nextAttemptAt` | string (ISO datetime) \| null | When dunning will next retry |
| `lineItems` | `LineItemView[]` | Present on detail fetch |
| `paymentAttempts` | `PaymentAttemptView[]` | Present on detail fetch |

`GET /invoices/{id}` returns `404 invoice_not_found` for an unknown invoice or
another tenant's invoice — the UI renders a not-found state (FR-022).

### InvoiceStatus (enumeration)

| Status | UI presentation |
|--------|-----------------|
| `open` | Unpaid — visually distinguished; settleable when the tenant is past-due/suspended |
| `paid` | Settled — neutral presentation |
| `void` | Cancelled bill — neutral, not settleable |

The list view distinguishes `open` from `paid` (FR-021). The settle action
targets an `open` invoice; `POST /invoices/{id}/settle` returns `409
invoice_not_settleable` for an already-paid or void invoice.

## LineItemView

One charge on an invoice (US4). Present in `InvoiceView.lineItems`.

| Field | Type | Notes |
|-------|------|-------|
| `kind` | string | e.g. `subscription`, `overage` |
| `description` | string | Human-readable description |
| `quantity` | number | Count (e.g. overage sends) |
| `unitPriceMinor` | number | Per-unit price, minor units |
| `amountMinor` | number | Line total, minor units |

## PaymentAttemptView

One attempt to charge an invoice (US4). Present in
`InvoiceView.paymentAttempts`; multiple attempts accumulate during dunning.

| Field | Type | Notes |
|-------|------|-------|
| `attemptNumber` | number | 1-based ordinal |
| `status` | `'succeeded' \| 'failed'` | Outcome of the attempt |
| `gatewayReference` | string | Gateway-side reference |
| `failureReason` | string | Reason text, populated for a failed attempt |
| `createdAt` | string (ISO datetime) | When the attempt was made |

## Sending-error kinds (not a view shape)

The UI also recognises two backend error **kinds** returned by the existing
campaign-start and transactional-send endpoints (both HTTP `403`), surfaced in
the sending screens (FR-029, FR-030):

| Error kind | Meaning | UI message |
|------------|---------|------------|
| `quota_exceeded` | Block-mode allowance exhausted, or no active subscription | "You've used your plan's send allowance" + link to the usage view |
| `tenant_suspended` | Subscription suspended for non-payment | "Sending is disabled — your account is suspended for non-payment" + link to the billing area |

## Relationships

```text
PlanView ──subscribed-by──> SubscriptionView ──embeds──> UsageView
                                   │
                                   └──billed-by──> InvoiceView ──┬─> LineItemView[]
                                                                 └─> PaymentAttemptView[]
```

No new persisted entities, no schema change, no migration.
