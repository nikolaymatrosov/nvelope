# Contract: Billing HTTP API

All routes are mounted by `cmd/api` under the existing tenant-scoped prefix
`/t/{slug}/api`, inside the authenticated group, alongside the Phase 1–4 routes.
Handlers live in `internal/api/billing_handlers.go`. Amounts are integer minor
units (kopecks); the JSON field name carries the `Minor` suffix.

New permission pair, registered in `internal/iam`:
- `billing:get` — view plans, the subscription, invoices.
- `billing:manage` — subscribe, cancel, settle an invoice.

Privileged actions (subscribe, cancel, settle) are written to the existing
`audit_log`.

## GET /t/{slug}/api/plans

List published plans (the subscribable catalog). Permission: `billing:get`.

Response `200`:

```json
{
  "plans": [
    {
      "id": "uuid",
      "code": "starter",
      "name": "Starter",
      "priceMinor": 990000,
      "currency": "RUB",
      "billingPeriod": "1 month",
      "includedSends": 50000,
      "overageMode": "block",
      "overagePriceMinor": 0
    }
  ]
}
```

## POST /t/{slug}/api/subscription

Subscribe the tenant to a plan. Permission: `billing:manage`. The first charge
runs **synchronously** (research R12), so the response reflects the outcome.

Request:

```json
{ "planId": "uuid" }
```

Response `201` — first charge succeeded:

```json
{
  "subscription": {
    "id": "uuid",
    "planId": "uuid",
    "state": "active",
    "currentPeriodStart": "2026-05-19T00:00:00Z",
    "currentPeriodEnd": "2026-06-19T00:00:00Z",
    "cancelAtPeriodEnd": false
  },
  "invoice": { "id": "uuid", "status": "paid", "totalMinor": 990000, "currency": "RUB" }
}
```

Errors:
- `402 payment_failed` — gateway declined the first charge; subscription is
  `past_due`, invoice `open` (US1 scenario 2).
- `409 subscription_exists` — the tenant already holds a non-canceled
  subscription (US1 scenario 3 / FR-002).
- `404 plan_not_found` / `422 plan_not_published` — bad or unsubscribable plan.

## GET /t/{slug}/api/subscription

The tenant's current subscription with current-period usage. Permission:
`billing:get`.

Response `200`:

```json
{
  "subscription": {
    "id": "uuid",
    "plan": { "id": "uuid", "code": "starter", "name": "Starter", "overageMode": "block" },
    "state": "active",
    "currentPeriodStart": "2026-05-19T00:00:00Z",
    "currentPeriodEnd": "2026-06-19T00:00:00Z",
    "cancelAtPeriodEnd": false
  },
  "usage": {
    "includedSends": 50000,
    "usedSends": 31240,
    "overageSends": 0,
    "remainingSends": 18760
  }
}
```

`usedSends` is the rolled-counter total plus the un-rolled `usage_events` tail
(research R10). Response `404 no_subscription` when the tenant has none.

## DELETE /t/{slug}/api/subscription

Cancel the subscription at period end. Permission: `billing:manage`. Sets
`cancel_at_period_end = true`; the subscription stays `active` until the period
elapses, then `billing.sweep` moves it to `canceled` instead of renewing. No
refund of the current period (spec Assumptions).

Response `200` — the updated subscription (`cancelAtPeriodEnd: true`).

## GET /t/{slug}/api/invoices

List the tenant's invoices, newest first. Permission: `billing:get`.
Query: `?limit=&offset=` (defaults 50 / 0, max 200).

Response `200`:

```json
{
  "invoices": [
    {
      "id": "uuid",
      "periodStart": "2026-05-19T00:00:00Z",
      "periodEnd": "2026-06-19T00:00:00Z",
      "totalMinor": 990000,
      "currency": "RUB",
      "status": "paid",
      "issuedAt": "2026-05-19T00:00:01Z",
      "paidAt": "2026-05-19T00:00:02Z"
    }
  ],
  "total": 4
}
```

## GET /t/{slug}/api/invoices/{id}

One invoice with its line items and payment attempts. Permission: `billing:get`.

Response `200`:

```json
{
  "id": "uuid",
  "status": "open",
  "totalMinor": 990000,
  "currency": "RUB",
  "attemptCount": 2,
  "nextAttemptAt": "2026-05-25T00:00:00Z",
  "lineItems": [
    { "kind": "subscription", "description": "Starter — May 2026",
      "quantity": 1, "unitPriceMinor": 990000, "amountMinor": 990000 }
  ],
  "paymentAttempts": [
    { "attemptNumber": 1, "status": "failed", "failureReason": "card_declined",
      "createdAt": "2026-05-19T00:00:02Z" },
    { "attemptNumber": 2, "status": "failed", "failureReason": "card_declined",
      "createdAt": "2026-05-22T00:00:00Z" }
  ]
}
```

Response `404 invoice_not_found` for an unknown id (RLS also makes another
tenant's invoice invisible).

## POST /t/{slug}/api/invoices/{id}/settle

Settle an outstanding (`open` / `uncollectible`) invoice immediately — the
reinstatement path (US5 scenario 5 / FR-016). Permission: `billing:manage`.
Runs the shared `ChargeInvoice` synchronously.

Response `200` — charge succeeded; invoice `paid`; if the subscription was
`suspended` it returns to `active` and sending is re-enabled.

Errors:
- `402 payment_failed` — the settle charge was declined; nothing changes.
- `409 invoice_not_settleable` — the invoice is already `paid` or `void`.

## Error mapping

Billing domain errors carry slugs via the shared `apperr` package; the single
mapping point `internal/api/errmap.go` is extended:

| Slug | HTTP |
|---|---|
| `plan_not_found`, `invoice_not_found`, `no_subscription` | 404 |
| `plan_not_published`, `invoice_not_settleable` | 422 |
| `subscription_exists` | 409 |
| `payment_failed` | 402 |
| `quota_exceeded`, `tenant_suspended` | 403 (returned by the send paths, not these routes) |

Domain and use-case code never reference HTTP status codes; transport mapping
stays solely in `errmap.go`.
