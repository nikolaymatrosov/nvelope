# Contract — Phase 5 Billing Endpoints Consumed by the UI

This feature adds **no** endpoints. It documents the seven existing Phase 5
endpoints (in `internal/api/billing_handlers.go`, registered in
`internal/api/server.go`) the UI consumes, plus the two sending-error kinds it
surfaces. All endpoints are mounted under `/t/{slug}/api` and require an
authenticated tenant session; the UI reaches them through the tenant-scoped
client `tp(slug, …)` in `frontend/src/lib/api.ts`.

Error responses carry a machine-readable `kind` slug. The UI maps each kind to
a specific in-place UI state — it never branches on error strings or HTTP
status text.

## GET /plans

List the available plans for the catalogue (US2).

- **Permission**: `billing:get`
- **Request**: none
- **Response `200`**: `PlanView[]` — `{ id, code, name, priceMinor, currency,
  billingPeriod, includedSends, overageMode, overagePriceMinor }`
- **UI**: render the catalogue; an empty array → explicit empty state
  (FR-007).

## POST /subscription

Subscribe the tenant to a plan; synchronously charges the first invoice (US2).

- **Permission**: `billing:manage`
- **Request**: `{ "planId": string }`
- **Response `201`**: `{ subscription: SubscriptionView, invoice: InvoiceView }`
  — on a successful charge the subscription `state` is `active` and the
  invoice `status` is `paid`.
- **Errors**:
  - `402 payment_failed` — gateway declined; subscription stays `past_due`,
    invoice stays `open`. UI: explain the payment failed, subscription not
    activated, offer retry (FR-010).
  - `409 subscription_exists` — tenant already has a subscription. UI: the
    subscribe action is unavailable with an explanatory message (FR-011).
  - `404 plan_not_found` / `422 plan_not_published` — UI: surface as an
    actionable error; the plan is no longer subscribable.
- **UI**: the trigger is disabled while the request is in flight; exactly one
  outcome is shown (FR-012, SC-003).

## GET /subscription

Read the current subscription and embedded usage (US1, US3).

- **Permission**: `billing:get`
- **Request**: none
- **Response `200`**: `SubscriptionView` — includes `state`, period dates,
  `cancelAtPeriodEnd`, and the embedded `UsageView`.
- **Errors**:
  - `404 no_subscription` — UI: render the explicit no-subscription state with
    a path to the catalogue (FR-004), **not** an error screen.
- **UI**: `state` drives the billing-home presentation (see data-model
  enumeration); `pending` → in-progress state (FR-005).

## DELETE /subscription

Cancel at period end. **Not wired into the UI in this phase** (research R8) —
documented for completeness; the `cancelAtPeriodEnd` flag from `GET
/subscription` is shown read-only.

- **Permission**: `billing:manage`

## GET /invoices

List the tenant's invoices for the invoice history (US4).

- **Permission**: `billing:get`
- **Request**: query `?limit=&offset=` (paged; default page size per the
  existing `DEFAULT_PAGE_SIZE` convention)
- **Response `200`**: `{ items: InvoiceView[], total: number }` — list entries
  carry the summary fields (`id`, `status`, `totalMinor`, `currency`,
  period/dates); `lineItems` and `paymentAttempts` are empty until the detail
  fetch.
- **UI**: paged/incremental load (FR-023); an empty list → explicit empty
  state (FR-018).

## GET /invoices/{id}

Read a single invoice with line items and payment attempts (US4).

- **Permission**: `billing:get`
- **Request**: none
- **Response `200`**: `InvoiceView` with populated `lineItems` and
  `paymentAttempts`.
- **Errors**:
  - `404 invoice_not_found` — unknown invoice or another tenant's invoice. UI:
    not-found state, never another tenant's data (FR-022).

## POST /invoices/{id}/settle

Settle an outstanding invoice; synchronously re-charges the gateway and
reinstates a suspended subscription on success (US5).

- **Permission**: `billing:manage`
- **Request**: none
- **Response `200`**: `InvoiceView` with `status: paid`. On success the
  associated subscription is reinstated (`active`).
- **Errors**:
  - `402 payment_failed` — charge declined again; nothing changes. UI: explain
    the payment failed, account remains suspended, offer to try again
    (FR-027).
  - `409 invoice_not_settleable` — invoice already `paid` or `void`. UI:
    reconcile to current state without a hard error (FR-034).
- **UI**: the settle trigger is disabled while in flight; on success confirm
  reinstatement, clear the suspension banner, reflect sending re-enabled
  (FR-026).

## Sending-error kinds surfaced in the existing sending UI

These are **not new endpoints**. The existing campaign-start and
transactional-send endpoints return these error kinds (HTTP `403`) when Phase 5
quota enforcement blocks a send; the UI surfaces them in the existing campaign
and transactional screens (FR-029, FR-030).

| Error kind | Returned when | UI surfacing |
|------------|---------------|--------------|
| `quota_exceeded` | Block-mode allowance exhausted, or the tenant has no active subscription | Actionable message naming the cause, linking to `billing/usage` |
| `tenant_suspended` | The subscription is `suspended` for non-payment | Actionable message naming the cause, linking to `billing` to settle the balance |

These are distinguished from a generic permission `403` so the operator is
routed to billing rather than told they lack access.

## Error-kind → UI-state mapping (summary)

| HTTP | kind | UI state |
|------|------|----------|
| 402 | `payment_failed` | Declined-charge message + retry path |
| 409 | `subscription_exists` | Subscribe disabled + explanation |
| 409 | `invoice_not_settleable` | Reconcile silently to current state |
| 404 | `no_subscription` | No-subscription state + catalogue link |
| 404 | `invoice_not_found` | Not-found state |
| 403 | `quota_exceeded` | Quota message in sending UI + usage link |
| 403 | `tenant_suspended` | Suspended message in sending UI + billing link |
| 401 | — | Route to sign-in (existing app-shell behaviour) |
| 403 | (other) | Insufficient-permission state (existing behaviour) |
