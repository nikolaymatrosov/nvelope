# Feature Specification: Phase 5 — Billing & Metering

**Feature Branch**: `010-phase-5-billing-metering`

**Created**: 2026-05-19

**Status**: Draft

**Input**: User description: "Phase 5 — Billing & Metering: plans, tenant subscriptions, invoices, invoice line items, payment attempts; a PaymentGateway interface with a deterministic MockGateway; an in-house subscription engine with a lifecycle state machine, billing.sweep / billing.charge jobs, invoice generation, and dunning (retries → suspension); usage_events, the usage.rollup job, and usage_counters; quota enforcement at campaign start and transactional send (block vs meter overage modes); tenant suspension on payment failure."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Subscribe to a paid plan (Priority: P1)

A tenant administrator chooses one of the available plans and subscribes their
tenant to it. The subscription becomes active once the first charge succeeds,
and from that moment the tenant has access to the plan's included allowances.

**Why this priority**: Without the ability to subscribe to a plan and be
charged for it, there is no billing relationship at all. This is the
foundational slice that every other story depends on.

**Independent Test**: Can be fully tested by creating a plan, having a tenant
subscribe to it, and confirming the first invoice is generated and charged
successfully through the mock gateway, leaving the subscription in an active
state.

**Acceptance Scenarios**:

1. **Given** a tenant with no subscription and a published plan, **When** the
   tenant subscribes to that plan, **Then** a subscription is created, an
   invoice for the first billing period is generated, the mock gateway charges
   it successfully, and the subscription becomes active.
2. **Given** a tenant subscribing to a plan, **When** the mock gateway declines
   the first charge, **Then** the subscription does not become active, the
   invoice is marked unpaid, and the tenant is informed the payment failed.
3. **Given** a tenant that already has an active subscription, **When** the
   tenant attempts to subscribe again, **Then** the request is rejected because
   a tenant may only hold one active subscription at a time.

---

### User Story 2 - Recurring renewal billing (Priority: P2)

At the end of each billing period the platform automatically renews the
tenant's subscription: it generates the next invoice, charges the tenant
through the gateway, and extends the billing period when the charge succeeds.

**Why this priority**: Recurring revenue is the core purpose of a subscription
product. It builds directly on Story 1 and turns a one-time charge into an
ongoing billing relationship.

**Independent Test**: Can be fully tested by advancing an active subscription
to the end of its billing period, running the renewal process, and confirming
a new invoice is generated, charged, and the billing period extended.

**Acceptance Scenarios**:

1. **Given** an active subscription whose billing period is ending, **When** the
   renewal process runs, **Then** a new invoice is generated for the next
   period, charged through the gateway, and the billing period is advanced.
2. **Given** several tenants whose subscriptions renew on the same day, **When**
   the renewal process runs, **Then** each subscription is renewed exactly once
   with no duplicate invoices or duplicate charges.
3. **Given** a renewal charge that the gateway declines, **When** the renewal
   process runs, **Then** the subscription enters a past-due state and the
   dunning process (Story 5) begins.

---

### User Story 3 - Usage metering (Priority: P3)

Every billable action a tenant takes — primarily email sends from campaigns and
transactional sends — is recorded as a usage event. The platform periodically
rolls these events up into per-tenant, per-period usage counters that reflect
how much of the plan's allowance has been consumed.

**Why this priority**: Metering is required before quotas can be enforced or
overage billed. It is observable on its own (counters can be inspected) and
delivers value as usage reporting even before enforcement exists.

**Independent Test**: Can be fully tested by generating a known number of
sends, running the rollup process, and confirming the usage counter for the
current period matches the expected total.

**Acceptance Scenarios**:

1. **Given** a tenant that has sent emails during the current billing period,
   **When** the rollup process runs, **Then** the tenant's usage counter for
   that period reflects the total number of metered sends.
2. **Given** usage events that have already been rolled up, **When** the rollup
   process runs again, **Then** counters are not double-counted.
3. **Given** a new billing period begins, **When** usage is metered, **Then** it
   accumulates against the new period's counter and the prior period's counter
   is preserved unchanged.

---

### User Story 4 - Quota enforcement (Priority: P4)

When a tenant starts a campaign or makes a transactional send, the platform
checks the tenant's remaining allowance for the current period. Depending on
the plan's overage mode, requests beyond the allowance are either blocked or
allowed and metered as overage for later billing.

**Why this priority**: Enforcement protects revenue and infrastructure but
depends on metering (Story 3) being in place. It is the point where billing
state actively governs product behaviour.

**Independent Test**: Can be fully tested by setting a low quota, consuming it,
and confirming that a `block`-mode tenant is prevented from sending further
while a `meter`-mode tenant continues sending with overage recorded.

**Acceptance Scenarios**:

1. **Given** a tenant within its allowance, **When** the tenant starts a
   campaign or makes a transactional send, **Then** the send proceeds normally.
2. **Given** a tenant on a `block` overage mode plan that has reached its
   allowance, **When** the tenant starts a campaign or makes a transactional
   send, **Then** the send is rejected with a clear quota-exceeded reason.
3. **Given** a tenant on a `meter` overage mode plan that has reached its
   allowance, **When** the tenant continues sending, **Then** the sends proceed
   and the excess is recorded as overage usage for that period.
4. **Given** a campaign that would partially exceed a `block`-mode tenant's
   remaining allowance, **When** the campaign is started, **Then** the system
   applies a consistent, predictable rule for whether the campaign is rejected
   or capped, and communicates that outcome to the tenant.

---

### User Story 5 - Dunning and suspension on payment failure (Priority: P5)

When a charge fails, the platform retries the payment a bounded number of times
on a defined schedule. If every retry fails, the tenant is suspended: sending is
disabled until the outstanding balance is settled.

**Why this priority**: This is the consequence path for non-payment. It depends
on the billing and enforcement machinery from earlier stories and completes the
exit criteria, but the product is demonstrably useful without it.

**Independent Test**: Can be fully tested by forcing the mock gateway to
decline a charge, running the dunning process through all retries, and
confirming the tenant is suspended and can no longer send.

**Acceptance Scenarios**:

1. **Given** an invoice whose charge failed, **When** the dunning process runs,
   **Then** the payment is retried according to the defined schedule and each
   attempt is recorded.
2. **Given** an invoice that fails every dunning retry, **When** the final retry
   fails, **Then** the tenant is suspended and sending is disabled.
3. **Given** a suspended tenant, **When** the tenant attempts to start a
   campaign or make a transactional send, **Then** the request is rejected with
   a reason indicating the account is suspended for non-payment.
4. **Given** a tenant in dunning, **When** a retry charge succeeds before
   retries are exhausted, **Then** the invoice is marked paid, dunning stops,
   and the subscription returns to active.
5. **Given** a suspended tenant, **When** the outstanding balance is settled,
   **Then** the tenant is reinstated and sending is re-enabled.

---

### Edge Cases

- What happens when the renewal process runs while a previous run for the same
  subscription has not finished? Renewal must be idempotent — at most one
  invoice and one charge per subscription per billing period.
- What happens when a charge is submitted but the gateway response is lost or
  delayed? The platform must not double-charge; a payment attempt must resolve
  to exactly one outcome.
- What happens when a tenant is suspended mid-campaign? In-flight sends already
  accepted are governed by a defined rule (allowed to drain or halted), and no
  new sends are accepted.
- What happens when usage events arrive for a billing period that has already
  closed? They are attributed to the period in which the action occurred, not
  the period in which they were processed.
- What happens when a tenant subscribes, immediately cancels, or downgrades?
  Plan changes and cancellation handling are bounded in Assumptions.
- What happens when a tenant has no subscription at all? A defined default
  applies (see Assumptions) so quota checks and sending have deterministic
  behaviour.
- What happens when the same usage event is recorded twice (e.g. a retried
  send job)? Metering must not count the same billable action more than once.

## Requirements *(mandatory)*

### Functional Requirements

#### Plans & subscriptions

- **FR-001**: System MUST allow plans to be defined, each with a price, a
  billing period length, a currency, included usage allowances, and an overage
  mode (`block` or `meter`).
- **FR-002**: System MUST allow a tenant to subscribe to exactly one plan, and
  MUST reject a subscription request from a tenant that already has an active
  subscription.
- **FR-003**: System MUST record each subscription's lifecycle state and only
  permit valid transitions between states (e.g. pending → active, active →
  past-due, past-due → active, past-due → suspended, suspended → active,
  active → cancelled).
- **FR-004**: System MUST track, for each subscription, the current billing
  period's start and end so renewal timing is unambiguous.

#### Invoices & payments

- **FR-005**: System MUST generate an invoice for each billing period a tenant
  is charged for, itemised into line items (at minimum the plan subscription
  fee, plus any overage charges).
- **FR-006**: System MUST record every payment attempt against an invoice,
  including its outcome (success or failure) and the reason for a failure.
- **FR-007**: System MUST charge invoices through a payment gateway abstraction
  so the gateway implementation can be swapped without changing billing logic.
- **FR-008**: System MUST provide a deterministic mock gateway whose outcome
  (approve, decline, error) is predictable and controllable for testing and
  development.
- **FR-009**: System MUST ensure a single invoice results in at most one
  successful charge, even if charging is retried or runs concurrently.
- **FR-010**: System MUST mark an invoice as paid only when a charge against it
  succeeds, and as unpaid/past-due otherwise.

#### Renewal & dunning

- **FR-011**: System MUST automatically detect subscriptions due for renewal
  and generate and charge the next period's invoice without manual action.
- **FR-012**: System MUST ensure renewal is idempotent — at most one invoice
  and one charge per subscription per billing period.
- **FR-013**: System MUST retry a failed charge a bounded number of times on a
  defined schedule before giving up.
- **FR-014**: System MUST suspend a tenant when all dunning retries for an
  invoice are exhausted without success.
- **FR-015**: System MUST stop dunning and return the subscription to active if
  a retry charge succeeds before retries are exhausted.
- **FR-016**: System MUST allow a suspended tenant to be reinstated once the
  outstanding balance is settled, re-enabling sending.

#### Usage metering

- **FR-017**: System MUST record a usage event for every billable action,
  attributed to the tenant and to the billing period in which the action
  occurred.
- **FR-018**: System MUST treat campaign sends and transactional sends as
  metered billable actions.
- **FR-019**: System MUST periodically roll usage events up into per-tenant,
  per-period usage counters.
- **FR-020**: System MUST ensure the rollup process is idempotent — usage
  events are never counted more than once across repeated rollup runs.
- **FR-021**: System MUST keep each billing period's usage counter separate so
  consumption resets at the start of every period and historical periods
  remain readable.

#### Quota enforcement

- **FR-022**: System MUST check the tenant's remaining allowance before a
  campaign starts and before a transactional send is accepted.
- **FR-023**: System MUST reject sends that exceed the allowance when the
  tenant's plan uses `block` overage mode, returning a clear quota-exceeded
  reason.
- **FR-024**: System MUST allow sends that exceed the allowance when the
  tenant's plan uses `meter` overage mode, and record the excess as overage
  usage for billing.
- **FR-025**: System MUST apply a consistent, predictable rule when a campaign
  would only partially fit within a `block`-mode tenant's remaining allowance.
- **FR-026**: System MUST reject any campaign start or transactional send from a
  suspended tenant, returning a reason that indicates account suspension for
  non-payment.

#### Visibility

- **FR-027**: Tenant administrators MUST be able to see their current plan,
  subscription state, current-period usage against allowance, and invoice and
  payment history.

### Key Entities *(include if feature involves data)*

- **Plan**: A purchasable offering. Defines price, currency, billing period
  length, included usage allowances, and overage mode (`block` or `meter`).
- **Tenant Subscription**: The billing relationship between a tenant and a
  plan. Holds the lifecycle state and the current billing period's start and
  end. A tenant has at most one active subscription.
- **Invoice**: A bill for one billing period of a subscription, with a total,
  currency, and paid/unpaid status. Composed of invoice line items.
- **Invoice Line Item**: A single charge on an invoice — e.g. the plan
  subscription fee or an overage charge — with a description, quantity, and
  amount.
- **Payment Attempt**: A record of one attempt to charge an invoice through the
  gateway, with its outcome and any failure reason. Multiple attempts may exist
  per invoice during dunning.
- **Usage Event**: A single recorded billable action (e.g. an email send),
  attributed to a tenant and a billing period.
- **Usage Counter**: An aggregated, per-tenant, per-period total of usage,
  produced by rolling up usage events; the basis for quota checks and overage
  billing.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A tenant can go from no subscription to an active paid
  subscription, with the first period charged, in a single self-service flow.
- **SC-002**: 100% of active subscriptions due for renewal are renewed exactly
  once per billing period, with no duplicate invoices and no duplicate charges.
- **SC-003**: For any billing period, the rolled-up usage counter matches the
  count of billable actions taken in that period within 100% accuracy after the
  rollup completes.
- **SC-004**: 100% of sends from a `block`-mode tenant that has exhausted its
  allowance are rejected; 100% of sends from a `meter`-mode tenant in the same
  situation are accepted and recorded as overage.
- **SC-005**: A tenant whose payment fails is retried the full configured number
  of times and then suspended; 100% of suspended tenants are prevented from
  starting campaigns or making transactional sends.
- **SC-006**: A tenant that settles an outstanding balance is reinstated and
  able to send again, with no residual block remaining.
- **SC-007**: Repeated runs of the renewal, charging, and rollup processes
  produce no change beyond the first successful run (full idempotency).
- **SC-008**: A tenant administrator can view current plan, subscription state,
  period usage, and billing history without contacting support.

## Assumptions

- **Currency**: Plans are priced in a single currency per plan (Russian rouble
  by default, given the target market); multi-currency support is out of scope.
- **Billing period**: Plans bill on a fixed recurring period (monthly by
  default). Usage-based-only or per-second metering plans are out of scope.
- **Real payment provider**: Only the deterministic mock gateway is delivered in
  this phase. Integration with a real Russian payment provider is a later
  phase; the gateway abstraction exists so that integration is additive.
- **Dunning schedule**: A bounded retry count on a fixed schedule is used (a
  reasonable default such as 3 retries over several days); exact values are a
  configuration detail to be settled during planning.
- **Plan changes**: Mid-period upgrades, downgrades, and proration are out of
  scope for this phase; the supported change paths are initial subscribe and
  cancellation at period end. Cancellation stops future renewals but does not
  refund the current period.
- **Tenants without a subscription**: A tenant with no active subscription is
  treated under a defined default (e.g. a restricted free tier or no sending);
  the precise default is decided during planning but quota behaviour is
  deterministic regardless.
- **Metered actions**: Email sends (campaign and transactional) are the metered
  billable actions for this phase. Other potential metrics (stored subscribers,
  API calls) are not metered now but the usage-event model does not preclude
  them later.
- **Suspension scope**: Suspension disables outbound sending. Read access to the
  platform (viewing data, settings, billing history) remains available so the
  tenant can settle their balance.
- **Background processing**: Renewal, charging, dunning, and usage rollup run as
  scheduled/queued background processes; tenants are not expected to trigger
  them manually.
- **Existing platform**: This phase builds on the existing tenancy, campaign,
  and transactional sending capabilities delivered in earlier phases.
