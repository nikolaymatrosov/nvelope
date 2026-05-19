---
description: "Task list for Phase 5 — Billing & Metering"
---

# Tasks: Phase 5 — Billing & Metering

**Input**: Design documents from `/specs/010-phase-5-billing-metering/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/

**Tests**: Included. Constitution II (Test-Backed Delivery, NON-NEGOTIABLE)
names billing and quota enforcement a critical path requiring integration
coverage against real boundaries; the plan's Testing section makes the test
tasks below mandatory, not optional.

**Organization**: Tasks are grouped by user story. Each story is an
independently implementable and testable increment.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependency on incomplete tasks)
- **[Story]**: US1–US5, mapping to the spec's prioritised user stories
- All paths are repo-relative from `/Users/nikthespirit/Documents/experiment/nvelope`

## Path Conventions

Go web service. New bounded context at `internal/billing/{domain,app,adapters}`.
Migrations in `internal/db/migrations/`. Services in `cmd/`.

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Scaffolding and configuration that every later phase builds on.

- [X] T001 Create the `internal/billing` context skeleton: `domain/`, `app/command/`, `app/query/`, `adapters/` directories and `internal/billing/app/application.go` with an empty `Application{Commands, Queries}` value
- [X] T002 [P] Add config keys `BillingSweepInterval`, `UsageRollupInterval`, `DunningMaxAttempts`, `DunningRetryInterval` to `internal/config/config.go` with defaults (1h, 15m, 3, 72h) and `NVELOPE_`-prefixed env binding
- [X] T003 [P] Register the `billing:get` and `billing:manage` permissions in `internal/iam` alongside the existing permission set

**Checkpoint**: Context skeleton, config, and permissions exist.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Schema, money primitive, job args, and error mapping that ALL user stories depend on.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete.

- [X] T004 Write migration `internal/db/migrations/000014_billing.{up,down}.sql`: control-plane `plans` (no RLS), RLS-protected `tenant_subscriptions`, `invoices`, `invoice_line_items`, `payment_attempts` per data-model.md (constraints, indexes, `nvelope_app` grants), plus the `SECURITY DEFINER` function `billing_due_subscriptions()` returning `(tenant_id, subscription_id, reason)`
- [X] T005 [P] Implement the `Money` value type (minor units + currency, currency-mixing-safe arithmetic) in `internal/billing/domain/money.go`
- [X] T006 [P] Define billing slug errors (`plan_not_found`, `plan_not_published`, `subscription_exists`, `no_subscription`, `invoice_not_found`, `invoice_not_settleable`, `payment_failed`, `quota_exceeded`, `tenant_suspended`, `ErrInvalidSubscriptionTransition`) via the shared `apperr` package in `internal/billing/domain/errors.go`
- [X] T007 Add `BillingSweepArgs`, `BillingChargeArgs`, `UsageRollupArgs` job kinds and their `SendEnqueuer` enqueue methods to `internal/platform/jobs/jobs.go`
- [X] T008 Extend `internal/api/errmap.go` to map the billing slugs to HTTP status codes per contracts/http-api.md
- [X] T009 [P] Integration test: `000014_billing` applies and reverts cleanly, in `internal/db` (or the existing migration test location)

**Checkpoint**: Schema, money, job args, and error mapping ready — user stories can begin.

---

## Phase 3: User Story 1 - Subscribe to a paid plan (Priority: P1) 🎯 MVP

**Goal**: A tenant subscribes to a published plan; the first invoice is generated and charged synchronously through the mock gateway; the subscription becomes active.

**Independent Test**: Create a published plan, `POST /subscription`, confirm a `paid` invoice and an `active` subscription; confirm a declined gateway leaves the subscription `past_due`; confirm a second subscribe is rejected.

### Tests for US1

- [X] T010 [P] [US1] Unit tests for the `Subscription` state machine (valid + rejected transitions) in `internal/billing/domain/subscription_test.go`
- [X] T011 [P] [US1] Unit test `MockGateway` determinism and idempotency (same key → same result, never double-charges) in `internal/billing/adapters/mock_gateway_test.go`
- [X] T012 [P] [US1] Integration test `subscriptions_pg` + cross-tenant isolation in `internal/billing/adapters/subscriptions_pg_test.go`
- [X] T013 [P] [US1] Integration test `invoices_pg` (invoice + line items + payment attempts) + cross-tenant isolation in `internal/billing/adapters/invoices_pg_test.go`
- [X] T014 [P] [US1] Component test the `Subscribe` command: success, gateway decline, duplicate-subscription rejection, unpublished plan, in `internal/billing/app/command/subscribe_test.go`

### Implementation for US1

- [X] T015 [P] [US1] `Plan` catalog entity (validating constructor + hydration path, `OverageMode`, `IsSubscribable`) in `internal/billing/domain/plan.go`
- [X] T016 [P] [US1] `Subscription` aggregate with the lifecycle state machine (transition methods, typed rejection) in `internal/billing/domain/subscription.go`
- [X] T017 [P] [US1] `Invoice` + `InvoiceLineItem` aggregate (total = sum of line items, status transitions) in `internal/billing/domain/invoice.go`
- [X] T018 [P] [US1] `PaymentAttempt`, the `PaymentGateway` port, and `ChargeRequest`/`ChargeResult`/`ChargeOutcome` types in `internal/billing/domain/payment.go`
- [X] T019 [US1] Repository interfaces (`PlanRepository`, `SubscriptionRepository`, `InvoiceRepository`) in `internal/billing/domain/repository.go`
- [X] T020 [P] [US1] `plans_pg` adapter (read published catalog) in `internal/billing/adapters/plans_pg.go`
- [X] T021 [P] [US1] `subscriptions_pg` adapter in `internal/billing/adapters/subscriptions_pg.go`
- [X] T022 [P] [US1] `invoices_pg` adapter (invoices, line items, payment attempts) in `internal/billing/adapters/invoices_pg.go`
- [X] T023 [P] [US1] Deterministic `MockGateway` (`PaymentGateway` impl, programmable rule set) in `internal/billing/adapters/mock_gateway.go`
- [X] T024 [US1] Shared `ChargeInvoice` command (load invoice, skip-if-paid guard, gateway charge, record attempt, mark paid, advance subscription on success) in `internal/billing/app/command/charge_invoice.go`
- [X] T025 [US1] `Subscribe` command (validate plan, reject duplicate, create subscription + first invoice, run `ChargeInvoice` synchronously) in `internal/billing/app/command/subscribe.go`
- [X] T026 [P] [US1] `ListPlans` query in `internal/billing/app/query/plans.go`
- [X] T027 [P] [US1] `GetSubscription` query (subscription + plan; usage block added in US3) in `internal/billing/app/query/subscription.go`
- [X] T028 [P] [US1] `ListInvoices` + `GetInvoice` queries in `internal/billing/app/query/invoices.go`
- [X] T029 [US1] Populate `Application{Commands, Queries}` and add billing constructors to `internal/billing/app/application.go`
- [X] T030 [US1] Wire the billing context (repositories, `MockGateway`, `Application`) in `internal/service/application.go`
- [X] T031 [US1] `internal/api/billing_handlers.go`: `GET /plans`, `POST /subscription`, `GET /subscription`, `GET /invoices`, `GET /invoices/{id}` with `billing:get`/`billing:manage` permission checks
- [X] T032 [US1] Mount the billing routes in `internal/api/server.go` under the authenticated `/t/{slug}/api` group
- [X] T033 [US1] Write an `audit_log` entry for the privileged subscribe action

**Checkpoint**: A tenant can subscribe and be charged; MVP is demonstrable.

---

## Phase 4: User Story 2 - Recurring renewal billing (Priority: P2)

**Goal**: Subscriptions due for renewal are automatically re-invoiced and charged; cancellation stops future renewals.

**Independent Test**: Advance an `active` subscription's `current_period_end` into the past, run `billing.sweep`, confirm a new invoice is generated, charged, and the period advanced; confirm a canceled subscription is not renewed.

### Tests for US2

- [X] T034 [P] [US2] Component test `billing.sweep`: enqueues one `billing.charge` per due subscription, no duplicates, in `internal/billing/adapters/sweep_worker_test.go`
- [X] T035 [P] [US2] Component test renewal via `billing.charge`: new invoice generated, charged, period advanced, in `internal/billing/adapters/charge_worker_test.go`
- [X] T036 [P] [US2] Integration test charge idempotency: a retried `billing.charge` produces exactly one successful charge and one paid invoice, in `internal/billing/adapters/charge_worker_test.go`
- [X] T037 [P] [US2] Component test `CancelSubscription` + sweep transition to `canceled` at period end, in `internal/billing/app/command/cancel_test.go`

### Implementation for US2

- [X] T038 [US2] Extend `ChargeInvoice` (T024) with renewal-invoice generation: create the next period's invoice + `subscription` line item, on `(subscription_id, period_start)` conflict load the existing invoice, in `internal/billing/app/command/charge_invoice.go`
- [X] T039 [P] [US2] `due_subscriptions` adapter calling the `billing_due_subscriptions()` `SECURITY DEFINER` function in `internal/billing/adapters/due_subscriptions.go`
- [X] T040 [US2] `RunBillingSweep` command (fan out `billing.charge` per due row, unique by subscription id) in `internal/billing/app/command/sweep.go`
- [X] T041 [P] [US2] `billing.sweep` River worker in `internal/billing/adapters/sweep_worker.go`
- [X] T042 [P] [US2] `billing.charge` River worker (tenant-bound transaction, delegates to `ChargeInvoice`) in `internal/billing/adapters/charge_worker.go`
- [X] T043 [P] [US2] `CancelSubscription` command (set `cancel_at_period_end`) in `internal/billing/app/command/cancel.go`
- [X] T044 [US2] Handle `cancel_at_period_end` in the sweep/charge path: transition to `canceled` instead of renewing
- [X] T045 [US2] `DELETE /subscription` handler in `internal/api/billing_handlers.go` + audit-log entry
- [X] T046 [US2] Register the `billing.sweep` and `billing.charge` workers in `cmd/worker/main.go`
- [X] T047 [US2] Add the `billing.sweep` ticker (and startup enqueue) to `cmd/scheduler/main.go`

**Checkpoint**: Recurring renewals charge automatically; cancellation works.

---

## Phase 5: User Story 3 - Usage metering (Priority: P3)

**Goal**: Every campaign and transactional send is recorded as a usage event; a periodic rollup aggregates events into per-period counters.

**Independent Test**: Send a known number of campaign and transactional messages, run `usage.rollup`, confirm the counter matches; re-run rollup and confirm no double-counting.

### Tests for US3

- [X] T048 [P] [US3] Integration test `usage_pg` (events + counters) + cross-tenant isolation in `internal/billing/adapters/usage_pg_test.go`
- [X] T049 [P] [US3] Component test `usage.rollup` idempotency: re-run never double-counts; new period accumulates separately, in `internal/billing/adapters/rollup_worker_test.go`
- [X] T050 [P] [US3] Test usage-event recording idempotency: the same `source_ref` recorded twice is a no-op, in `internal/billing/adapters/usage_recorder_test.go`

### Implementation for US3

- [X] T051 [US3] Write migration `internal/db/migrations/000015_usage_metering.{up,down}.sql`: RLS-protected `usage_events` and `usage_counters` per data-model.md (unique constraints, partial index, grants)
- [X] T052 [P] [US3] `UsageEvent`, `UsageCounter` entities and quota arithmetic (`included`/`overage` split) in `internal/billing/domain/usage.go`
- [X] T053 [US3] `UsageRepository` interface in `internal/billing/domain/repository.go`
- [X] T054 [P] [US3] `usage_pg` adapter (record events `ON CONFLICT DO NOTHING`; upsert counters; un-rolled tail sum) in `internal/billing/adapters/usage_pg.go`
- [X] T055 [US3] `RollupUsage` command (aggregate un-rolled events, upsert counters, stamp `rolled_up_at` in one transaction) in `internal/billing/app/command/rollup.go`
- [X] T056 [P] [US3] `usage.rollup` River worker in `internal/billing/adapters/rollup_worker.go`
- [X] T057 [P] [US3] `UsageRecorder` port in `internal/campaign/domain/quota.go` (consumer-owned)
- [X] T058 [US3] `UsageRecorder` adapter (resolve the subscription's current period, record one `usage_event` per send ref) in `internal/billing/adapters/usage_recorder.go`
- [X] T059 [US3] Record campaign-send usage events from `internal/campaign/adapters/batch_worker.go` (one per sent recipient)
- [X] T060 [US3] Record a transactional-send usage event from `internal/campaign/app/command/transactional.go`
- [X] T061 [US3] Wire the `UsageRecorder` adapter into the campaign batch worker + transactional handler in `internal/service/application.go`
- [X] T062 [US3] Extend the `GetSubscription` query (T027) with the usage block (counter total + un-rolled tail, remaining/overage)
- [X] T063 [US3] Register the `usage.rollup` worker in `cmd/worker/main.go` and the per-tenant enqueue tick in `cmd/scheduler/main.go`

**Checkpoint**: Sends are metered; counters are accurate and idempotent.

---

## Phase 6: User Story 4 - Quota enforcement (Priority: P4)

**Goal**: Campaign starts and transactional sends are checked against the tenant's allowance; `block` mode rejects over-allowance sends, `meter` mode allows them as overage.

**Independent Test**: Set a low allowance, exhaust it; confirm a `block`-mode tenant is rejected and a `meter`-mode tenant proceeds with overage recorded.

### Tests for US4

- [X] T064 [P] [US4] Integration test the `QuotaGate` adapter: within allowance allows; `block` over-allowance blocks; `meter` over-allowance allows; in `internal/billing/adapters/quota_gate_test.go`
- [X] T065 [P] [US4] Component test campaign start: a `block`-mode campaign exceeding the remaining allowance is rejected whole; a `meter`-mode campaign proceeds; in `internal/campaign/adapters/start_worker_test.go`
- [X] T066 [P] [US4] Component test transactional send: blocked when over a `block`-mode allowance, in `internal/campaign/app/command/transactional_test.go`

### Implementation for US4

- [X] T067 [US4] Add the `QuotaGate` port (`Authorize`, `QuotaDecision`) to `internal/campaign/domain/quota.go`
- [X] T068 [US4] `QuotaGate` adapter (subscription-state check, usage = counter + un-rolled tail, `block`/`meter` decision) in `internal/billing/adapters/quota_gate.go`
- [X] T069 [US4] Authorize the whole campaign in `internal/campaign/adapters/start_worker.go` after recipient resolution; `block` + over-allowance → fail the campaign with `quota_exceeded`
- [X] T070 [US4] Authorize transactional sends in `internal/campaign/app/command/transactional.go` (1 unit) before send
- [X] T071 [US4] Wire the `QuotaGate` adapter into the campaign start worker + transactional handler in `internal/service/application.go`

**Checkpoint**: Quotas are enforced in both overage modes.

---

## Phase 7: User Story 5 - Dunning and suspension on payment failure (Priority: P5)

**Goal**: A failed charge is retried on a bounded schedule; exhausted retries suspend the tenant; settling the balance reinstates it.

**Independent Test**: Program the mock gateway to decline a tenant's charges; run the sweep through all retries; confirm suspension and that sends are rejected; settle the invoice and confirm reinstatement.

### Tests for US5

- [X] T072 [P] [US5] Component test the dunning path: a failed charge sets `past_due` + `next_attempt_at`; retries are spaced; exhaustion → `suspended` + `uncollectible` invoice; in `internal/billing/adapters/charge_worker_test.go`
- [X] T073 [P] [US5] Test that a `suspended` subscription makes the `QuotaGate` reject campaign and transactional sends with `tenant_suspended`, in `internal/billing/adapters/quota_gate_test.go`
- [X] T074 [P] [US5] Component test `SettleInvoice`: a successful settle marks the invoice `paid` and reinstates a `suspended` subscription to `active`, in `internal/billing/app/command/settle_test.go`

### Implementation for US5

- [X] T075 [P] [US5] Dunning schedule/retry policy (`DunningMaxAttempts`, `DunningRetryInterval`) in `internal/billing/domain/dunning.go`
- [X] T076 [US5] Extend `ChargeInvoice` (T024/T038) failure path: increment `attempt_count`, set `next_attempt_at`, transition to `past_due`; on exhausted attempts mark the invoice `uncollectible` and the subscription `suspended`
- [X] T077 [P] [US5] `SettleInvoice` command (charge an open/uncollectible invoice via `ChargeInvoice`; on success reinstate a suspended subscription) in `internal/billing/app/command/settle.go`
- [X] T078 [US5] `POST /invoices/{id}/settle` handler in `internal/api/billing_handlers.go` + audit-log entry
- [X] T079 [US5] Confirm `billing.sweep` picks up `dunning`-reason rows from `billing_due_subscriptions()` and re-arms `billing.charge` for due `past_due` invoices

**Checkpoint**: Dunning, suspension, send-blocking, and reinstatement all work — Epic B exit criteria met.

---

## Phase 8: Polish & Cross-Cutting Concerns

- [X] T080 [P] Confirm every billing command/query handler is wrapped with the standard logging/metrics decorator
- [X] T081 [P] Seed a `published` plan via a dev fixture/seed path so the quickstart steps are runnable
- [X] T082 Run the full quickstart.md manual verification (subscribe → meter → enforce → renew → dunning → reinstate)
- [X] T083 Verify `cmd/migrate up` then `down 2` apply and revert cleanly
- [X] T084 Run `make test` and `go build ./...`; confirm a green suite, including cross-tenant isolation tests for every new repository

---

## Dependencies & Execution Order

- **Setup (Phase 1)** → **Foundational (Phase 2)** → user stories.
- **US1 (P1)** depends only on Phases 1–2. It is the MVP.
- **US2 (P2)** depends on US1 (reuses `ChargeInvoice`, `Subscription`, `Invoice`, repositories).
- **US3 (P3)** depends on Phases 1–2 and US1 (needs a subscription to attribute usage and a period). Largely parallel to US2.
- **US4 (P4)** depends on US3 (the `QuotaGate` reads `usage_counters` + un-rolled events).
- **US5 (P5)** depends on US2 (the `billing.charge` path) and US4 (the gate enforces the `suspended` block).
- **Polish (Phase 8)** last.

Story completion order: US1 → US2 → US3 → US4 → US5.

## Parallel Execution Examples

- **Phase 2**: T005, T006, T009 run in parallel (T004 first; T007, T008 independent).
- **US1 domain**: T015–T018 in parallel (separate files), then T019, then adapters T020–T023 in parallel.
- **US1 tests**: T010–T014 in parallel.
- **US2**: T039, T041, T042, T043 in parallel after T038.
- **US3**: T052, T054, T056, T057 in parallel after T051.
- **US5 tests**: T072, T073, T074 in parallel.

## Implementation Strategy

- **MVP**: Phases 1–3 (Setup + Foundational + US1). A tenant can subscribe and
  be charged — a demonstrable, shippable slice.
- **Incremental**: ship US2, then US3, then US4, then US5; each phase ends at a
  checkpoint that is independently testable and demonstrable.
- Each user story phase must exit with a green test suite and a clean migration
  apply, per Constitution II and the phase exit criteria.
