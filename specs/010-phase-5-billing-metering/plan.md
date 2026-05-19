# Implementation Plan: Phase 5 — Billing & Metering

**Branch**: `010-phase-5-billing-metering` | **Date**: 2026-05-19 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/010-phase-5-billing-metering/spec.md`

## Summary

Phase 5 turns nvelope from a free platform into a metered subscription product.
Today a tenant can send campaigns and transactional mail without limit and
without ever being billed. Phase 5 adds an in-house billing engine: a tenant
subscribes to a **plan**, the platform charges a recurring fee through a
swappable **payment gateway**, **meters** every send, **enforces quotas** at
campaign start and transactional send, and — when a payment fails — runs
**dunning** (bounded retries) and finally **suspends sending**.

Technical approach: add one bounded context, `internal/billing`, holding all
five concerns behind the project's calibrated `domain`/`app`/`adapters` split —
plans & subscriptions, invoicing, payment, usage metering, and quota
enforcement. They share the same data and the same tenant, and the
constitution's "layer scope is proportional to need" favours one context over
five (YAGNI).

Two migrations add the schema: `000014_billing` (control-plane `plans` catalog
plus RLS-protected `tenant_subscriptions`, `invoices`, `invoice_line_items`,
`payment_attempts`) and `000015_usage_metering` (RLS-protected `usage_events`
and `usage_counters`). `plans` is a platform catalog with no tenant data, so it
carries no RLS, mirroring `tenants`; every other table is tenant-scoped with
the standard `app.tenant_id` policy.

The `PaymentGateway` is a domain-owned port. Phase 5 ships only a deterministic
`MockGateway` whose outcome is a pure function of the idempotency key, so tests
and demos are repeatable and a real Russian payment provider is a later,
additive phase.

The subscription engine is a state machine on the `Subscription` aggregate
(`pending → active → past_due → {active, suspended} → active`, plus
`active → canceled`). Three durable River jobs drive it: a periodic
`billing.sweep` finds subscriptions due for renewal or a dunning retry and fans
out `billing.charge` jobs; `billing.charge` generates the period invoice (once),
charges it through the gateway, records a `payment_attempt`, and advances the
subscription; a periodic `usage.rollup` aggregates raw `usage_events` into
`usage_counters`. The first charge at subscribe time runs the **same** charge
code path synchronously so the tenant gets immediate feedback (see Complexity
Tracking).

Metering and enforcement reuse the Phase 4 port pattern: the campaign context
declares a domain-owned `QuotaGate` interface (mirroring `SuppressionChecker`);
a `billing` adapter implements it and is wired in `service/`. The campaign
start worker authorizes the whole campaign against the tenant's remaining
allowance before fan-out; the campaign batch worker and the transactional
handler record a `usage_event` per send. A suspended subscription makes the
gate reject every send (FR-026); a `block`-mode plan rejects sends past the
allowance, a `meter`-mode plan accepts them and records the excess as overage.

This phase is backend-only; the billing UI is a later increment. It satisfies
Epic B.

## Technical Context

**Language/Version**: Go 1.26

**Primary Dependencies**: all existing — `riverqueue/river` (durable job queue),
`jackc/pgx/v5` (PostgreSQL), `go-chi/chi/v5` (HTTP), `redis/go-redis/v9`,
`golang-migrate/migrate/v4`, `knadh/koanf` (config). **No new dependency** —
the `MockGateway` is in-process Go; a real payment SDK is a later phase.

**Storage**: PostgreSQL (shared database, RLS). One new control-plane table,
`plans` (a platform-managed catalog, no tenant data, no RLS — like `tenants`).
Six new tenant-scoped, RLS-protected tables: `tenant_subscriptions`,
`invoices`, `invoice_line_items`, `payment_attempts`, `usage_events`,
`usage_counters`. River's queue tables are unchanged. No new Redis usage.

**Testing**: `go test ./...` with `testify`; integration tests against a real
`postgres:17` via `testcontainers-go` (existing `internal/dbtest` harness).
Constitution II names billing and quota enforcement a critical path, so they
get integration coverage against real boundaries: the subscription lifecycle,
renewal and dunning through the deterministic `MockGateway`, charge
idempotency (a retried `billing.charge` never double-charges), usage-rollup
idempotency (re-run never double-counts), quota enforcement in both overage
modes, suspension blocking sends, and cross-tenant isolation for every new
repository.

**Target Platform**: Linux server; the existing four stateless Go services on
Kubernetes. The subscription/invoice/plan routes are served by `cmd/api`;
`billing.sweep`, `billing.charge`, and `usage.rollup` jobs run on `cmd/worker`;
`cmd/scheduler` periodically enqueues `billing.sweep` and one `usage.rollup`
per active tenant. `cmd/consumer` is untouched.

**Project Type**: Web service (Go backend). Backend-only phase.

**Performance Goals**: The quota gate adds at most two indexed reads
(`usage_counters` row plus a bounded sum of not-yet-rolled `usage_events`) to a
send, so it does not materially slow campaign start or a transactional send.
`billing.sweep` and `usage.rollup` are periodic background jobs whose cost is
proportional to subscriptions/tenants due, not total history.

**Constraints**: Tenant isolation is the data layer's job (RLS), never
application code alone — every tenant-scoped billing table carries `tenant_id`
and a `FORCE ROW LEVEL SECURITY` policy. `billing.sweep` needs a cross-tenant
read of subscriptions that are due; it gets it through a `SECURITY DEFINER`
function returning only `(tenant_id, subscription_id, reason)`, never by
bypassing RLS in application code (mirrors the Phase 3/4 tenant-resolution
pattern). All recurring billing work is durable and resumable: River recovers
the jobs, and exactly-once charging is guaranteed by a unique invoice-per-period
constraint plus a gateway idempotency key. The payment provider is reached only
through the `PaymentGateway` abstraction.

**Scale/Scope**: 5 user stories, 27 functional requirements, 7 key entities.
Roughly: 2 new migrations, 1 new bounded context (`billing`), 3 new River job
kinds + 3 workers, ~7 new authenticated tenant routes, one new permission pair
(`billing:get` / `billing:manage`), and small edits to the Phase 3 campaign
start/batch workers and the transactional handler to authorize and record
usage.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

Evaluated against `.specify/memory/constitution.md` v1.1.0.

| Principle | Status | Notes |
| --- | --- | --- |
| I. Tenant Isolation by Default | PASS | Every new tenant-plane table (`tenant_subscriptions`, `invoices`, `invoice_line_items`, `payment_attempts`, `usage_events`, `usage_counters`) carries `tenant_id` and `ENABLE`/`FORCE ROW LEVEL SECURITY` with the same `app.tenant_id` policy as Phase 2–4. `plans` is a platform catalog with no tenant data, so — like `tenants` — it is control-plane and carries no RLS. `billing.sweep`'s only cross-tenant read goes through a `SECURITY DEFINER` function that projects just `(tenant_id, subscription_id, reason)`; `billing.charge` and `usage.rollup` then run inside the resolved tenant's bound transaction. Cross-tenant isolation tests are added for every new repository. |
| II. Test-Backed Delivery | PASS | Billing and quota enforcement are named critical paths in Principle II. They get integration coverage against real boundaries — Postgres + River via testcontainers — for the subscription state machine, renewal and dunning through the deterministic `MockGateway`, charge idempotency (retried `billing.charge` → exactly one successful charge), rollup idempotency (re-run → no double count), both overage modes, and suspension blocking sends. Phase exits with green `go test ./...` and a clean migration apply. |
| III. Incremental, Shippable Phases | PASS | Five independently shippable slices map to the spec's prioritised user stories: US1 subscribe + first charge, US2 recurring renewal, US3 usage metering, US4 quota enforcement, US5 dunning + suspension. Build for this phase only — real payment-provider integration, proration, mid-cycle plan changes, and the billing UI are explicitly out of scope (spec Assumptions). Completes Epic B. |
| IV. Security & Consent by Design | PASS | The payment provider is reached only through the `PaymentGateway` port; the `MockGateway` holds no secrets, and a real gateway's credentials will be secret config, never logged. Subscribe, cancel, and settle are privileged tenant actions written to the existing `audit_log`. Routes are gated by a new `billing:get` / `billing:manage` permission pair and the backend re-checks every request. Money amounts are integer minor units (kopecks) to avoid floating-point error. |
| V. Operable & Observable Services | PASS | All four services stay stateless: subscription, invoice, and usage state live in Postgres; job state in River. `billing.sweep`, `billing.charge`, and `usage.rollup` are durable, retry-capable River jobs. Exactly-once charging survives a lost gateway response via a unique `invoices (subscription_id, period_start)` constraint, a "skip if already paid" guard, and a deterministic gateway idempotency key; the sweep re-arms lost work; `usage.rollup` is idempotent via a per-event `rolled_up_at` marker. Every new command/query handler keeps the standard logging/metrics decorator. |
| VI. Layered Architecture & Domain Integrity | PASS | The new `billing` context uses the calibrated `domain`/`app`/`adapters` split. `Subscription`, `Invoice`, and `Plan` are rich entities with unexported fields, validating constructors, and a separate documented hydration path; the lifecycle state machine and dunning policy are domain behaviour, not handler `if`s. The `PaymentGateway` port is declared by the `billing` domain (it depends on it); the `QuotaGate` port consumed by the Phase 3 send paths is declared by the `campaign` context (the consumer) and implemented by a `billing` adapter. Errors carry slugs via the shared `apperr` package; transport mapping stays in `api/errmap.go`. Wiring is plain constructors in `service/`. |

**Gate result: PASS — two documented design choices, recorded in Complexity Tracking.**

No new dependency is introduced. The two choices recorded below are the
synchronous first charge in the subscribe command and the quota gate reading a
rolled counter plus a live tail of un-rolled events.

*Post-design re-check*: **PASS** — the data model adds only RLS-protected
tenant-plane tables plus one control-plane catalog table; contracts add no
transport leakage into domain code; the gateway sits behind a domain-owned
port and the quota gate behind a consumer-owned port. Design holds the gate.

## Project Structure

### Documentation (this feature)

```text
specs/010-phase-5-billing-metering/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/
│   ├── http-api.md      # Phase 1 output — subscription / invoice / plan routes
│   ├── jobs.md          # Phase 1 output — billing.sweep, billing.charge, usage.rollup
│   └── ports.md         # Phase 1 output — domain-owned Go interfaces
├── checklists/
│   └── requirements.md  # Spec quality checklist (/speckit-specify)
└── tasks.md             # Phase 2 output (/speckit-tasks — NOT created here)
```

### Source Code (repository root)

```text
internal/
├── billing/                          # NEW bounded context
│   ├── domain/
│   │   ├── plan.go                    # Plan catalog entity, OverageMode
│   │   ├── subscription.go            # Subscription aggregate + lifecycle state machine
│   │   ├── invoice.go                 # Invoice + InvoiceLineItem aggregate
│   │   ├── payment.go                 # PaymentAttempt; PaymentGateway port; Charge req/result
│   │   ├── dunning.go                 # dunning schedule / retry policy
│   │   ├── usage.go                   # UsageEvent, UsageCounter, quota arithmetic
│   │   ├── money.go                   # Money (minor units + currency) value type
│   │   ├── repository.go              # plan/subscription/invoice/usage repo interfaces
│   │   └── errors.go                  # context-specific slug errors
│   ├── app/
│   │   ├── application.go             # Application{Commands, Queries}
│   │   ├── command/                   # Subscribe, CancelSubscription, ChargeInvoice,
│   │   │                               # RunBillingSweep, SettleInvoice, RollupUsage
│   │   └── query/                     # ListPlans, GetSubscription, ListInvoices, GetInvoice
│   └── adapters/
│       ├── plans_pg.go                # plans catalog repo
│       ├── subscriptions_pg.go        # tenant_subscriptions repo
│       ├── invoices_pg.go             # invoices + invoice_line_items + payment_attempts repo
│       ├── usage_pg.go                # usage_events + usage_counters repo
│       ├── due_subscriptions.go       # SECURITY DEFINER cross-tenant due-subscription read
│       ├── mock_gateway.go            # deterministic MockGateway (PaymentGateway impl)
│       ├── sweep_worker.go            # River worker for billing.sweep
│       ├── charge_worker.go           # River worker for billing.charge
│       ├── rollup_worker.go           # River worker for usage.rollup
│       ├── quota_gate.go              # QuotaGate adapter (campaign-owned port)
│       └── *_test.go
├── campaign/
│   ├── domain/
│   │   └── quota.go                   # NEW — QuotaGate interface + types (consumer-owned port)
│   ├── adapters/
│   │   ├── start_worker.go            # EXTEND — authorize whole campaign before fan-out
│   │   └── batch_worker.go            # EXTEND — record a usage_event per sent recipient
│   └── app/command/
│       └── transactional.go           # EXTEND — authorize + record usage on each send
├── platform/
│   └── jobs/
│       └── jobs.go                    # EXTEND — billing.sweep / billing.charge / usage.rollup args
├── api/
│   ├── server.go                      # EXTEND — mount billing routes
│   ├── billing_handlers.go             # NEW — subscription / invoice / plan routes
│   └── errmap.go                       # EXTEND — map billing error slugs
├── service/
│   └── application.go                  # EXTEND — wire the billing context + QuotaGate adapter
├── iam/                                # EXTEND — register billing:get / billing:manage permissions
├── config/
│   └── config.go                       # EXTEND — sweep/rollup intervals, dunning policy, mock config
└── db/migrations/
    ├── 000014_billing.{up,down}.sql            # NEW — plans, subscriptions, invoices, line items, payment attempts
    └── 000015_usage_metering.{up,down}.sql     # NEW — usage_events, usage_counters

cmd/
├── worker/main.go                      # EXTEND — register sweep / charge / rollup workers
└── scheduler/main.go                   # EXTEND — enqueue billing.sweep + per-tenant usage.rollup
```

**Structure Decision**: One new bounded context, `internal/billing`, following
the project's calibrated three-layer DDD layout (`domain`/`app`/`adapters`)
documented in `PATTERNS.md`. Plans, subscriptions, invoicing, payment, metering,
and quota enforcement are kept in a single context — they share the
subscription/invoice data and the same tenant, and the constitution's "layer
scope is proportional to need" favours not multiplying contexts (YAGNI). The
context shares the single `internal/api` transport layer and the
`internal/service` composition root. The `PaymentGateway` is a domain-owned
port with an in-process `MockGateway` adapter, mirroring how
`internal/sending` abstracts its provisioner. The `QuotaGate` is a
consumer-owned port: it is declared in the `campaign` context (which depends on
it) and implemented by a `billing` adapter, exactly mirroring the Phase 4
`SuppressionChecker`. River job infrastructure is extended in place in
`internal/platform/jobs`, matching Phases 3–4.

## Phase 0 — Research

Complete. See [research.md](./research.md). All decisions were resolved from
in-repo inspection of the Phase 3/4 patterns (`internal/platform/jobs`,
`internal/deliverability`, `cmd/scheduler`, the 008 contracts) and standard
recurring-billing practice; no `NEEDS CLARIFICATION` remain.

## Phase 1 — Design & Contracts

Complete:
- [data-model.md](./data-model.md) — the seven entities, their columns, the
  subscription state machine, and the RLS posture of each table.
- [contracts/http-api.md](./contracts/http-api.md) — the billing routes
  (`/plans`, `/subscription`, `/invoices`), request/response shapes, permission
  requirements, and error mapping.
- [contracts/jobs.md](./contracts/jobs.md) — the `billing.sweep`,
  `billing.charge`, and `usage.rollup` job kinds, payloads, idempotency, and
  scheduling.
- [contracts/ports.md](./contracts/ports.md) — the domain-owned `PaymentGateway`
  port and the consumer-owned `QuotaGate` port.
- [quickstart.md](./quickstart.md) — run, verify, and test instructions.
- Agent context (`CLAUDE.md`) updated to point at this plan.

## Phase 2 — Next step

Run `/speckit-tasks` to generate `tasks.md`. Suggested task ordering:

1. **Schema & money primitive** — the two migrations, the `Money` value type,
   and the seven repositories with cross-tenant isolation tests.
2. **US1 Subscribe + first charge** (P1) — `Plan`/`Subscription`/`Invoice`
   entities, the `PaymentGateway` port + `MockGateway`, the shared
   `ChargeInvoice` command, the `Subscribe` command (synchronous first charge),
   and the `/plans` + `POST /subscription` routes.
3. **US2 Recurring renewal** (P2) — `billing.sweep` + `billing.charge` workers,
   the `SECURITY DEFINER` due-subscription function, renewal invoice generation,
   period advancement, and the scheduler tick.
4. **US3 Usage metering** (P3) — `usage_events`/`usage_counters`, the
   `usage.rollup` worker, the scheduler per-tenant enqueue, and the campaign /
   transactional usage-recording edits.
5. **US4 Quota enforcement** (P4) — the `QuotaGate` port + `billing` adapter,
   the campaign start-worker whole-campaign authorization, the transactional
   pre-send authorization, and both overage modes.
6. **US5 Dunning & suspension** (P5) — the dunning schedule, retry exhaustion →
   suspension, the suspended-tenant send block, `POST /invoices/{id}/settle`,
   and reinstatement.

## Complexity Tracking

> Constitution Check passed with two documented design choices.

| Decision | Why Needed | Alternative Rejected Because |
|----------|------------|------------------------------|
| The first charge at subscribe time runs synchronously inside the `Subscribe` command, reusing the shared `ChargeInvoice` code path; only renewals and dunning go through the `billing.charge` job. | Spec US1 scenario 2 requires the tenant to be told immediately when the first payment fails, and an active subscription is the precondition for everything else — a self-service "subscribe now" flow expects an immediate result. | A fully asynchronous first charge (enqueue `billing.charge`, return `pending`, poll) was considered. It is more uniform but forces a polling UX onto the most latency-sensitive moment in the product, and the charge logic is the *same* shared command either way — so the synchronous call adds no duplicated logic, only an inline invocation. Renewals stay asynchronous because no user is waiting on them. |
| The `QuotaGate` computes current-period usage as the rolled-up `usage_counters` total **plus** a live sum of `usage_events` not yet rolled up, rather than reading the counter alone. | `usage.rollup` runs periodically, so the counter lags reality by up to one rollup interval. Enforcing a hard `block`-mode quota off a stale counter would let a tenant overshoot by a whole interval's worth of sends. | Reading only the counter is simpler but makes enforcement inaccurate by design. Incrementing the counter synchronously on every send (instead of recording an event) was also considered; it doubles the write bookkeeping and creates a hot row per tenant. The "counter + un-rolled tail" read is one extra indexed, bounded query and keeps `usage_events` the single source of truth that `usage.rollup` reconciles. |
