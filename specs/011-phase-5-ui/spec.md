# Feature Specification: Phase 5 — Billing & Metering — Frontend UI

**Feature Branch**: `011-phase-5-ui`

**Created**: 2026-05-19

**Status**: Draft

**Input**: User description: "add UI to features introduced in prev spec" — i.e. Phase 5 — Billing & Metering (010): plans, tenant subscriptions, invoices and line items, payment attempts, the subscription lifecycle, usage metering and counters, quota enforcement, and dunning/suspension on payment failure.

## User Scenarios & Testing *(mandatory)*

<!--
  Phase 5 (Billing & Metering) is already implemented in the backend: tenants can
  hold one subscription to a plan; subscriptions move through a lifecycle
  (pending → active → past-due → suspended → cancelled); invoices are generated
  and charged through a payment gateway each billing period; usage events are
  metered and rolled up into per-period counters; quotas are enforced at campaign
  start and transactional send; failed charges are retried (dunning) and exhaust
  to tenant suspension. Today none of this is visible to a tenant administrator —
  there is no web interface to choose a plan, subscribe, view usage against the
  plan allowance, read invoice and payment history, or recover a suspended
  account. This feature delivers that interface.

  It extends the existing tenant workspace app shell (the persistent sidebar
  layout from the Phase 1 & 2 UI) and sits alongside the Phase 3 sending UI and
  Phase 4 deliverability UI. The "users" are tenant administrators — the people
  inside a workspace responsible for the billing relationship. Renewal, charging,
  dunning, and usage rollup remain background processes with no operator-facing
  controls; this UI only reads their results and exposes the two operator-driven
  actions: subscribing to a plan and settling an outstanding balance.

  Backend constraints reflected by this UI: every plan, subscription, invoice,
  and usage figure is confined to a single tenant; access is gated by the
  existing permission strings; usage counters are produced by a periodic rollup
  job, so current-period figures may lag live sends by a few minutes; a tenant
  holds at most one active subscription; the mock payment gateway is the only
  gateway in this phase, so no real card-entry surface is required.
-->

### User Story 1 - View current plan and subscription status (Priority: P1)

A tenant administrator opens a "Billing" area in the workspace and sees, at a
glance, the tenant's current plan, the subscription's lifecycle state (pending,
active, past-due, suspended, or cancelled), and the current billing period's
start and end dates. When the subscription is healthy the area reads as
reassuring; when it is past-due or suspended the area leads with a prominent,
plain-language explanation of what is wrong and what to do next. A tenant with
no subscription sees an explicit "no subscription" state that points to the plan
catalogue.

**Why this priority**: The billing area is the home every other story is reached
from, and the subscription-status read is the single most important thing an
administrator needs — especially the past-due and suspended warnings, which are
the difference between a tenant noticing a problem and silently losing the
ability to send. It is independently demonstrable: open the billing area and
read the current state.

**Independent Test**: As an administrator of a tenant with an active
subscription, open the billing area and confirm the plan name, the active state,
and the billing period dates are shown. Repeat for a tenant in past-due and a
tenant in suspended state and confirm each shows its distinct warning. Repeat
for a tenant with no subscription and confirm the no-subscription state appears.

**Acceptance Scenarios**:

1. **Given** a tenant with an active subscription, **When** the administrator
   opens the billing area, **Then** they see the plan name, the active
   subscription state, and the current billing period's start and end dates.
2. **Given** a tenant whose subscription is past-due, **When** the administrator
   opens the billing area, **Then** a prominent warning explains the payment
   failed and that retries are in progress, and points to the unpaid invoice.
3. **Given** a tenant whose subscription is suspended, **When** the administrator
   opens the billing area, **Then** a prominent warning explains the account is
   suspended for non-payment and that sending is disabled until the balance is
   settled.
4. **Given** a tenant with no subscription, **When** the administrator opens the
   billing area, **Then** an explicit no-subscription state is shown with a path
   to the plan catalogue, rather than a blank screen or an error.
5. **Given** an administrator of one tenant, **When** they open the billing area,
   **Then** every figure shown belongs only to their own tenant.

---

### User Story 2 - Browse plans and subscribe (Priority: P1)

A tenant administrator with no active subscription opens a plan catalogue that
lists the available plans, each showing its price, currency, billing period
length, included usage allowance, and overage mode (whether sends beyond the
allowance are blocked or metered as overage). The administrator selects a plan,
reviews a confirmation summary of what they are about to be charged, and
confirms. The interface then reflects the outcome: on a successful first charge
the subscription becomes active; on a declined charge the administrator is told
the payment failed and the subscription was not activated, and is invited to
retry.

**Why this priority**: Subscribing is the one self-service action that
establishes the billing relationship; without it the rest of the billing area
has nothing to show. It is independently demonstrable end to end: pick a plan,
confirm, and land on an active subscription.

**Independent Test**: As an administrator of a tenant with no subscription, open
the plan catalogue, confirm each plan's price, period, allowance, and overage
mode are shown, select a plan, confirm the charge summary, and confirm the
subscription becomes active after a successful charge. Repeat with the mock
gateway set to decline and confirm the failure is communicated and the
subscription is not activated.

**Acceptance Scenarios**:

1. **Given** a tenant with no active subscription, **When** the administrator
   opens the plan catalogue, **Then** each plan shows its price, currency,
   billing period length, included allowance, and overage mode.
2. **Given** the plan catalogue, **When** the administrator selects a plan,
   **Then** a confirmation summary shows the plan and the amount that will be
   charged for the first period before any charge is made.
3. **Given** the confirmation summary, **When** the administrator confirms and
   the charge succeeds, **Then** the subscription becomes active and the billing
   area reflects the new plan and active state.
4. **Given** the confirmation summary, **When** the administrator confirms and
   the charge is declined, **Then** the interface explains the payment failed,
   the subscription is not activated, and a retry path is offered.
5. **Given** a tenant that already has an active subscription, **When** the
   administrator opens the plan catalogue, **Then** the subscribe action is
   unavailable and the interface explains a tenant may hold only one
   subscription at a time.

---

### User Story 3 - View current-period usage against allowance (Priority: P2)

A tenant administrator opens a usage view within the billing area and sees how
much of the plan's allowance has been consumed in the current billing period —
shown as a figure of metered sends used against the included allowance, with a
clear visual indication of how close the tenant is to the limit. When the
allowance is exhausted the view states what happens next based on the plan's
overage mode (sends blocked, or sends metered as overage). The view indicates
when the usage figures were last refreshed, so the administrator understands
they are near-real-time rather than live. Prior closed periods' usage remains
readable.

**Why this priority**: Usage visibility lets an administrator anticipate hitting
the limit and avoid a surprise block or overage bill, but it is a refinement on
top of the plan and status reads in Stories 1 and 2, which already give the
administrator the essential billing picture.

**Independent Test**: As an administrator of a tenant that has sent a known
number of emails in the current period, open the usage view and confirm the
consumed figure matches and is shown against the plan allowance. Drive usage to
the allowance and confirm the view states the overage-mode consequence. Open a
prior period and confirm its usage is preserved and readable.

**Acceptance Scenarios**:

1. **Given** a tenant with sending activity in the current period, **When** the
   administrator opens the usage view, **Then** the consumed metered-send count
   is shown against the plan's included allowance.
2. **Given** the usage view, **When** it is displayed, **Then** it shows when the
   usage figures were last refreshed.
3. **Given** a tenant that has consumed its full allowance on a `block`-mode
   plan, **When** the administrator opens the usage view, **Then** the view
   states that further sends will be blocked until the period resets.
4. **Given** a tenant that has consumed its full allowance on a `meter`-mode
   plan, **When** the administrator opens the usage view, **Then** the view
   states that further sends will be billed as overage and shows the overage
   accumulated so far.
5. **Given** a tenant with usage in a prior, closed billing period, **When** the
   administrator views that period, **Then** its usage figure is preserved and
   readable and is not mixed with the current period.

---

### User Story 4 - View invoice and payment history (Priority: P2)

A tenant administrator opens an invoice history within the billing area and sees
every invoice for the tenant, each row showing the billing period it covers, the
total, the currency, and its paid/unpaid status. Opening an invoice reveals its
line items — the plan subscription fee and any overage charges — each with a
description, quantity, and amount, and the record of payment attempts made
against it, including the outcome of each attempt and the reason for any
failure. An unpaid invoice is clearly distinguished from a paid one.

**Why this priority**: Invoice and payment history is what an administrator
consults to reconcile billing and to understand exactly why a payment failed,
but it reports on the billing relationship rather than establishing or governing
it, so it follows the status and subscribe stories.

**Independent Test**: As an administrator of a tenant with several invoices in
mixed paid/unpaid states, open the invoice history and confirm each invoice
shows its period, total, currency, and status. Open a paid invoice and confirm
its line items and a successful payment attempt are shown. Open an unpaid
invoice and confirm its failed payment attempts and failure reasons are shown.

**Acceptance Scenarios**:

1. **Given** a tenant with invoices, **When** the administrator opens the invoice
   history, **Then** each invoice shows its billing period, total, currency, and
   paid/unpaid status.
2. **Given** a tenant with no invoices yet, **When** the administrator opens the
   invoice history, **Then** an explicit empty state is shown.
3. **Given** an invoice, **When** the administrator opens it, **Then** its line
   items are shown, each with a description, quantity, and amount.
4. **Given** an invoice, **When** the administrator opens it, **Then** the
   payment attempts made against it are shown, each with its outcome and, for a
   failure, the failure reason.
5. **Given** an unpaid invoice, **When** the administrator views the invoice
   history, **Then** it is visually distinguished from paid invoices.

---

### User Story 5 - Recover a suspended account (Priority: P3)

A tenant administrator of a suspended tenant sees, prominently across the
workspace, that the account is suspended for non-payment and that sending is
disabled. From the billing area they can settle the outstanding balance by
retrying payment on the unpaid invoice. When the charge succeeds the interface
confirms the account is reinstated, the suspension warning clears, and sending
is re-enabled. While suspended, the administrator retains read access to the
workspace so they can reach the billing area to resolve it.

**Why this priority**: This is the recovery path out of the worst billing state,
and it matters, but it is reached only by the minority of tenants who fall into
suspension, and the suspended-state warning itself is already delivered by
Story 1 — Story 5 adds the settle-and-reinstate action on top.

**Independent Test**: As an administrator of a suspended tenant, confirm the
suspension is surfaced across the workspace, open the billing area, settle the
outstanding balance with the mock gateway set to approve, and confirm the
account is reinstated, the warning clears, and sending is available again.

**Acceptance Scenarios**:

1. **Given** a suspended tenant, **When** the administrator is anywhere in the
   workspace, **Then** a persistent indication that the account is suspended for
   non-payment and sending is disabled is shown.
2. **Given** a suspended tenant with an outstanding balance, **When** the
   administrator opens the billing area, **Then** a settle-balance action is
   offered against the unpaid invoice.
3. **Given** the settle-balance action, **When** the administrator triggers it
   and the charge succeeds, **Then** the interface confirms the account is
   reinstated, the suspension indication clears, and sending is re-enabled.
4. **Given** the settle-balance action, **When** the administrator triggers it
   and the charge is again declined, **Then** the interface explains the payment
   failed and the account remains suspended, with the option to try again.
5. **Given** a suspended tenant, **When** the administrator navigates the
   workspace, **Then** read access to data, settings, and billing history
   remains available so the balance can be settled.

---

### Edge Cases

- The billing area is opened for a tenant whose subscription is `pending` (first
  charge not yet resolved) — the area shows an explicit in-progress state rather
  than presenting the tenant as either active or unsubscribed.
- The plan catalogue is opened when no plans are published — an explicit empty
  state is shown rather than a blank catalogue.
- The administrator confirms a subscribe, but the gateway response is delayed —
  the interface shows a clear in-progress state and resolves to exactly one
  outcome (active or failed) without letting the administrator submit a second
  charge.
- The administrator opens the usage view before the first rollup has run for the
  current period — the consumed figure is shown as zero with an indication that
  figures have not yet been computed, not an error.
- An administrator tries to start a campaign or transactional send while the
  tenant is over a `block`-mode allowance — the quota-exceeded reason from the
  backend is surfaced in the sending UI as a clear, actionable message that
  points to the billing/usage area.
- An administrator tries to start a campaign or transactional send while the
  tenant is suspended — the suspended-for-non-payment reason is surfaced in the
  sending UI and points to the billing area to settle the balance.
- An invoice is opened that does not exist within the tenant, or belongs to
  another tenant — a not-found state is shown, never another tenant's invoice.
- The subscription state changes in the background (e.g. a renewal charge fails)
  while the administrator has the billing area open — the area reconciles to the
  current state on refresh without a hard error.
- An administrator without the billing permission opens the workspace — the
  billing area and its navigation entry are hidden or disabled, consistent with
  the existing UI permission gating.
- The invoice history is very long — it pages or loads incrementally without
  freezing.

## Requirements *(mandatory)*

### Functional Requirements

#### Subscription status & billing home

- **FR-001**: The interface MUST provide a billing area within the tenant
  workspace presenting the tenant's current plan, the subscription's lifecycle
  state, and the current billing period's start and end dates.
- **FR-002**: The billing area MUST present a prominent, plain-language warning
  when the subscription is past-due, explaining that a payment failed and
  retries are in progress, and linking to the unpaid invoice.
- **FR-003**: The billing area MUST present a prominent, plain-language warning
  when the subscription is suspended, explaining that the account is suspended
  for non-payment and sending is disabled.
- **FR-004**: The billing area MUST present an explicit no-subscription state,
  with a path to the plan catalogue, when the tenant has no subscription.
- **FR-005**: The billing area MUST present an explicit in-progress state when
  the subscription is pending (first charge not yet resolved), distinct from
  both the active and no-subscription states.

#### Plan catalogue & subscribing

- **FR-006**: The interface MUST provide a plan catalogue listing the available
  plans, each showing its price, currency, billing period length, included
  usage allowance, and overage mode (block or meter).
- **FR-007**: The plan catalogue MUST present an explicit empty state when no
  plans are available to subscribe to.
- **FR-008**: The interface MUST let an administrator select a plan and MUST
  present a confirmation summary of the plan and the amount to be charged for
  the first period before any charge is made.
- **FR-009**: The interface MUST, on a successful first charge, reflect the
  subscription as active and update the billing area to the new plan.
- **FR-010**: The interface MUST, on a declined first charge, explain that the
  payment failed, indicate the subscription was not activated, and offer a
  retry path.
- **FR-011**: The interface MUST make the subscribe action unavailable for a
  tenant that already has an active subscription, and MUST explain that a tenant
  may hold only one subscription at a time.
- **FR-012**: The interface MUST present a clear in-progress state while a
  subscribe charge is resolving and MUST prevent the administrator from
  submitting a second charge for the same subscribe action.

#### Usage & allowance

- **FR-013**: The interface MUST provide a usage view presenting the metered
  sends consumed in the current billing period against the plan's included
  allowance, with a visual indication of proximity to the limit.
- **FR-014**: The usage view MUST display when the usage figures were last
  refreshed, and MUST indicate when figures have not yet been computed for the
  current period.
- **FR-015**: The usage view MUST state, when the allowance is exhausted, the
  consequence determined by the plan's overage mode — that further sends will be
  blocked, or that they will be billed as overage — and MUST show overage
  accumulated so far for a meter-mode plan.
- **FR-016**: The usage view MUST keep prior closed periods' usage readable and
  separate from the current period's figure.

#### Invoices & payments

- **FR-017**: The interface MUST provide an invoice history listing the tenant's
  invoices, each showing the billing period covered, the total, the currency,
  and the paid/unpaid status.
- **FR-018**: The invoice history MUST present an explicit empty state when the
  tenant has no invoices.
- **FR-019**: The interface MUST let an administrator open an invoice and see its
  line items, each with a description, quantity, and amount.
- **FR-020**: The interface MUST, for an opened invoice, show the payment
  attempts made against it, each with its outcome and, for a failure, the
  failure reason.
- **FR-021**: The interface MUST visually distinguish unpaid invoices from paid
  invoices in the invoice history.
- **FR-022**: The interface MUST show a not-found state when an invoice is
  requested that does not exist within the operator's tenant, and MUST never
  display another tenant's invoice.
- **FR-023**: The interface MUST load a long invoice history incrementally
  (paging or infinite scroll) without freezing.

#### Suspension & recovery

- **FR-024**: The interface MUST surface, persistently across the workspace, that
  the account is suspended for non-payment and that sending is disabled, while
  the tenant is suspended.
- **FR-025**: The interface MUST offer a settle-balance action against the unpaid
  invoice for a suspended tenant from within the billing area.
- **FR-026**: The interface MUST, when a settle-balance charge succeeds, confirm
  the account is reinstated, clear the suspension indication, and reflect that
  sending is re-enabled.
- **FR-027**: The interface MUST, when a settle-balance charge is declined,
  explain the payment failed, indicate the account remains suspended, and offer
  to try again.
- **FR-028**: The interface MUST keep read access to workspace data, settings,
  and billing history available while the tenant is suspended.

#### Quota & suspension feedback in the sending UI

- **FR-029**: The sending interface MUST surface a quota-exceeded outcome — when
  a campaign start or transactional send is rejected for exceeding a block-mode
  allowance — as a clear, actionable message that points to the billing/usage
  area.
- **FR-030**: The sending interface MUST surface a suspended-account outcome —
  when a campaign start or transactional send is rejected because the tenant is
  suspended — as a clear, actionable message that points to the billing area to
  settle the balance.

#### Cross-cutting

- **FR-031**: The interface MUST scope every plan, subscription, invoice, usage
  figure, and payment record shown to the operator's current tenant.
- **FR-032**: The interface MUST hide or disable the billing area and its
  navigation entry for operators who lack the relevant permission, consistent
  with the existing UI permission gating.
- **FR-033**: Every new area MUST live within the existing tenant workspace app
  shell (persistent sidebar layout) and follow the navigation, loading, empty,
  and error-state conventions established by the Phase 1–4 UI.
- **FR-034**: The interface MUST reconcile to the current server state without a
  hard error when the subscription, invoice, or usage state changes in the
  background while a billing view is open.
- **FR-035**: The interface MUST present clear, actionable messages for failed
  operations (subscribe, settle balance) rather than silent failures.

### Key Entities *(include if feature involves data)*

- **Plan (as shown)**: A purchasable offering presented to the administrator —
  price, currency, billing period length, included usage allowance, and overage
  mode (block or meter).
- **Subscription status (as shown)**: The tenant's current plan, lifecycle state
  (pending, active, past-due, suspended, cancelled), and current billing
  period's start and end dates.
- **Usage (as shown)**: The metered sends consumed in the current period against
  the plan allowance, any overage accumulated, the last-refreshed time, and the
  readable figures for prior periods.
- **Invoice (as shown)**: A bill for one billing period — billing period
  covered, total, currency, and paid/unpaid status — composed of line items.
- **Invoice line item (as shown)**: A single charge on an invoice — description,
  quantity, and amount.
- **Payment attempt (as shown)**: One attempt to charge an invoice — its outcome
  and, for a failure, the failure reason.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: An administrator can determine the tenant's plan and subscription
  health (active, past-due, or suspended) within 10 seconds of opening the
  billing area.
- **SC-002**: An administrator of an unsubscribed tenant can go from the plan
  catalogue to an active subscription in a single self-service flow without
  contacting support.
- **SC-003**: 100% of subscribe and settle-balance actions resolve to exactly one
  outcome shown to the administrator (success or a stated failure), with no way
  to submit a duplicate charge for the same action.
- **SC-004**: An administrator can read how much of the plan allowance has been
  consumed in the current period, and whether they are near the limit, within 10
  seconds of opening the usage view.
- **SC-005**: An administrator can locate any invoice and read why a payment
  failed without consulting any other tool or contacting support.
- **SC-006**: A suspended tenant's administrator is shown the suspension on every
  workspace page and can reach the settle-balance action from the billing area.
- **SC-007**: An administrator who settles an outstanding balance sees the
  account reinstated and the suspension indication cleared, with no residual
  block, the next time they act in the workspace.
- **SC-008**: Operators only ever see plans, subscriptions, invoices, and usage
  figures belonging to their own tenant.
- **SC-009**: A campaign start or transactional send rejected for quota or
  suspension reasons shows the administrator an actionable message that names
  the cause and points to the billing area.

## Assumptions

- The Phase 5 backend (010) is delivered and exposes the tenant-scoped HTTP
  endpoints for plans, subscriptions, the subscribe and settle-balance actions,
  usage counters, invoices, line items, and payment attempts; this feature
  builds the UI on top of those endpoints and adds no new backend capability.
- The interface extends the existing tenant workspace web application and its
  sidebar app shell, reusing the navigation, permission-gating, loading, empty,
  and error-state patterns established by the Phase 1–4 UI.
- The mock payment gateway is the only gateway in this phase; subscribing and
  settling a balance trigger a gateway charge without the administrator entering
  card details, so no payment-instrument capture surface is built. A real
  payment-provider UI (card entry, saved instruments) is a later phase.
- Renewal, charging, dunning, and usage rollup run as background processes; this
  UI reads their results and exposes only the two operator-driven actions
  (subscribe, settle balance) — it provides no controls to trigger or configure
  those background processes.
- Usage counters are produced by a periodic rollup job; a lag of a few minutes
  between a send and its appearance in the usage view is acceptable and is
  communicated via the displayed last-refreshed time.
- Plan changes (mid-period upgrade, downgrade, proration) and cancellation are
  out of scope for this UI phase, consistent with the Phase 5 backend scope; the
  supported self-service actions are initial subscribe and settling a balance.
- Quota-exceeded and suspended-account outcomes are returned by the existing
  Phase 3 sending endpoints; this feature surfaces those outcomes in the
  existing campaign and transactional sending UI rather than redesigning it.
- Desktop web is the primary target, consistent with the existing workspace UI;
  a dedicated mobile layout is out of scope.
- Monetary amounts are displayed in the plan's currency (Russian rouble by
  default), consistent with the single-currency-per-plan backend assumption.
