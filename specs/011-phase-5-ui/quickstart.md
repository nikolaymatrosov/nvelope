# Quickstart — Phase 5 Billing & Metering UI

This feature is **frontend-only**. It extends the existing `frontend/` SPA and
consumes the already-running Phase 5 backend; no migrations, no new services.

## Prerequisites

- The Phase 5 backend (010) is built and the billing endpoints are mounted in
  `internal/api/server.go`.
- A running Postgres and the four Go services (`cmd/api`, `cmd/worker`,
  `cmd/scheduler`, `cmd/consumer`) — `cmd/api` serves the billing endpoints;
  `cmd/worker` / `cmd/scheduler` run the renewal, charging, dunning, and
  `usage.rollup` background jobs.
- Node toolchain for the frontend (the existing `frontend/` workspace).

## Run

```sh
# backend (serves /t/{slug}/api/...)
go run ./cmd/api
go run ./cmd/worker
go run ./cmd/scheduler

# frontend
cd frontend && pnpm install && pnpm dev
```

Open the workspace at `/t/{slug}/billing`.

## Verify by user story

**US1 — Subscription status.** Open `/t/{slug}/billing` for: a tenant with an
active subscription (plan name, active state, period dates shown); a past-due
tenant (warning + unpaid-invoice link); a suspended tenant (suspension
warning); a tenant with no subscription (no-subscription state + catalogue
link); a `pending` tenant (in-progress state).

**US2 — Plan catalogue & subscribe.** As an unsubscribed tenant, open
`/t/{slug}/billing/plans`, confirm each plan shows price, currency, period,
allowance, and overage mode. Select a plan, review the charge summary, confirm.
With the mock gateway set to approve, the subscription becomes `active`. With
it set to decline, the failure is shown and the subscription is not activated.
For an already-subscribed tenant the subscribe action is unavailable.

**US3 — Usage view.** Send a known number of emails, let `usage.rollup` run,
open `/t/{slug}/billing/usage`, confirm the consumed count against the
allowance and the last-refreshed indication. Drive usage to the allowance and
confirm the overage-mode consequence copy (blocked vs metered).

**US4 — Invoice history.** Open `/t/{slug}/billing/invoices`; confirm each
invoice shows period, total, currency, and paid/unpaid status. Open a paid
invoice (line items + a succeeded attempt) and an unpaid one (failed attempts +
failure reasons). Confirm the empty state for a tenant with no invoices.

**US5 — Recovery.** As a suspended tenant, confirm the suspension banner
appears on every workspace page. From `/t/{slug}/billing` settle the
outstanding balance with the mock gateway set to approve; confirm
reinstatement, the banner clearing, and sending re-enabled. Also confirm a
campaign start while over a block-mode allowance shows the `quota_exceeded`
message, and a send while suspended shows the `tenant_suspended` message, each
linking to billing.

## Test

```sh
cd frontend && pnpm test
```

Each new route (`billing/index.tsx`, `billing/plans.tsx`, `billing/usage.tsx`,
`billing/invoices.tsx`) ships a colocated `*.test.tsx` using `renderWithClient`
with `@/lib/api` and `@tanstack/react-router` mocked, covering the primary flow
plus the empty/in-progress/error states (no-subscription, pending, past-due,
suspended, declined charge, not-found invoice).

## Mock gateway control

The Phase 5 `MockGateway` is deterministic — its approve/decline/error outcome
is controllable for testing (see `specs/010-phase-5-billing-metering/contracts/
ports.md`). Use it to exercise both the success and the declined-charge
branches of subscribe and settle without a real payment provider.

## Definition of done

- All five user stories verified per the steps above.
- `pnpm test` green, including the new route tests.
- The Billing nav entry is hidden for an operator lacking `billing:get`; the
  subscribe/settle actions are hidden for one lacking `billing:manage`.
- No backend change; the existing Phase 5 Go suite stays green.
