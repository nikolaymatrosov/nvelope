# Phase 0 — Research: Phase 5 Billing & Metering UI

All decisions resolved from in-repo inspection. No `NEEDS CLARIFICATION`
remain.

## R1 — Backend endpoints already exist; no backend work

**Decision**: Build the UI purely as a consumer of the existing Phase 5
endpoints in `internal/api/billing_handlers.go`, registered in
`internal/api/server.go`.

**Rationale**: All seven endpoints the UI needs are implemented and wired:
`GET /plans`, `POST /subscription`, `GET /subscription`, `DELETE
/subscription`, `GET /invoices`, `GET /invoices/{id}`, `POST
/invoices/{id}/settle`. The subscribe and settle endpoints already perform a
synchronous gateway charge and return the resolved outcome. Adding backend
code would violate Constitution III (YAGNI) and the spec's explicit
"adds no new backend capability" assumption.

**Alternatives considered**: A dedicated aggregate "billing summary" endpoint
to populate the billing home in one call — rejected; `GET /subscription`
already returns the subscription plus the embedded `UsageView`, and `GET
/plans` is a separate cheap call, so the home needs at most two queries.

## R2 — Synchronous charge: in-progress state, no duplicate submit

**Decision**: Treat `POST /subscription` and `POST /invoices/{id}/settle` as
synchronous request/response actions. While in flight, render an in-progress
state and disable the trigger; on resolution show exactly one outcome
(success toast + state refresh, or a declined-charge message with a retry
path). Do not poll.

**Rationale**: The backend charges the gateway synchronously and returns the
final subscription/invoice state in the response (`201`/`200` on success,
`402 payment_failed` on decline). The spec requires each action to resolve to
exactly one outcome with no duplicate charge (FR-012, SC-003); disabling the
button for the request duration plus React Query's single-flight mutation is
sufficient — no idempotency key is needed because the user cannot submit
twice.

**Alternatives considered**: Optimistic UI — rejected; a charge can decline,
so the outcome must come from the server. Background polling — rejected; the
charge is synchronous, there is nothing to poll.

## R3 — Usage figures come embedded in the subscription view

**Decision**: Read current-period usage from the `UsageView` embedded in `GET
/subscription` (`includedSends`, `usedSends`, `overageSends`,
`remainingSends`). Display the rollup-driven nature with a last-refreshed
indication.

**Rationale**: `SubscriptionView.Usage` already carries the rolled-up
counters; no separate usage endpoint exists or is needed. Usage counters are
produced by the periodic `usage.rollup` job, so figures lag live sends by
minutes — the UI states this rather than implying live data, consistent with
how Phase 4 analytics handle `refreshedAt`.

**Alternatives considered**: A dedicated `/usage` endpoint with per-period
history — rejected; not implemented, and the spec's prior-period requirement
(FR-016) is satisfiable from the closed-period invoice line items (overage
quantity) plus the current `UsageView`. Prior-period detail beyond what
invoices show is out of scope.

## R4 — Permission gating reuses `billing:get` / `billing:manage`

**Decision**: Gate the Billing nav entry and read routes on `billing:get`;
gate the subscribe, cancel, and settle actions on `billing:manage`. Add both
strings to the frontend `Permission` union in `api-types.ts`.

**Rationale**: `internal/iam/domain/permission.go` defines exactly
`PermBillingGet = "billing:get"` and `PermBillingManage = "billing:manage"`.
The frontend `Permission` union currently lists 18 strings and does not
include the billing pair; it must mirror the backend. Gating reuses the
existing `usePermissions(slug)` hook and the sidebar `requires` array, exactly
as Phase 3/4 nav entries do.

**Alternatives considered**: A single `billing:*` permission — rejected; the
backend separates read from manage, and the UI must too so a read-only
operator sees the billing area without a subscribe/settle button.

## R5 — Quota & suspension errors surfaced in the existing sending UI

**Decision**: In the existing campaign-start and transactional-send screens,
map the backend error kinds `quota_exceeded` and `tenant_suspended` (both
HTTP `403`) to clear, actionable messages that link to the billing/usage
area. No redesign of the sending screens.

**Rationale**: `internal/campaign/domain/errors.go` defines these error kinds
and the Phase 3 handlers return them on a blocked send. The Phase 3 UI already
renders failed sends with a message; this feature only adds two recognised
error kinds and their copy + a link. Distinguishing them from a generic `403`
(insufficient permission) is necessary so the operator is sent to billing,
not told they lack access.

**Alternatives considered**: A pre-send quota check that hides the send button
when over allowance — rejected; the backend is authoritative, usage figures
lag, and a `meter`-mode tenant is *allowed* to exceed, so a client-side gate
would be both wrong and redundant. The UI reacts to the actual server outcome.

## R6 — Invoice detail as master/detail, not a separate route

**Decision**: Render invoice detail (line items + payment attempts) within
`billing/invoices.tsx` as a master/detail panel, not as a separate
`$id`-parameterised file-route.

**Rationale**: This matches the existing list+detail pattern used elsewhere in
the workspace and keeps the route count minimal (Constitution III). `GET
/invoices/{id}` is a separate fetch triggered when a row is opened; the
not-found state (`404 invoice_not_found`) is handled inline.

**Alternatives considered**: A `billing/invoices/$id.tsx` child route —
rejected as unnecessary ceremony for a panel that does not need to be
deep-linkable in this phase.

## R7 — Money display: minor units + currency

**Decision**: Add a small shared `Money` component that formats a minor-unit
integer (`priceMinor`, `totalMinor`, `amountMinor`) plus a currency code into
a localized amount. Default currency is RUB.

**Rationale**: Every monetary figure from the backend is an integer in minor
units with a sibling `currency` field. Centralising the formatting in one
component avoids per-route drift and gives a single place to handle the
single-currency-per-plan assumption.

**Alternatives considered**: Inline formatting per route — rejected; it
duplicates logic and risks inconsistent rounding/symbols.

## R8 — Cancellation: read-only in this UI phase

**Decision**: The UI does not expose a cancel-subscription control in this
phase, even though `DELETE /subscription` exists. If a subscription has
`cancelAtPeriodEnd: true`, the billing home displays that fact read-only.

**Rationale**: The spec scopes self-service actions to subscribe and settle
only; plan changes and cancellation UI are explicitly out of scope. The
`cancelAtPeriodEnd` flag is still shown so the operator is not surprised by a
non-renewing subscription.

**Alternatives considered**: Wiring a cancel button now — rejected; out of
spec scope and YAGNI. The endpoint remains available for a later phase.
