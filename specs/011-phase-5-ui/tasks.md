---

description: "Task list for Phase 5 — Billing & Metering — Frontend UI"
---

# Tasks: Phase 5 — Billing & Metering — Frontend UI

**Input**: Design documents from `/specs/011-phase-5-ui/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/tenant-api.md

**Tests**: Included — the plan (Constitution II) requires each new route to ship a colocated `*.test.tsx`.

**Organization**: Tasks are grouped by user story to enable independent implementation and testing.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (US1–US5)
- All paths are relative to the repository root

## Path Conventions

Web application — frontend only. New code lives under `frontend/src/`. The Go
backend is **not** touched: all seven Phase 5 endpoints already exist in
`internal/api/billing_handlers.go`.

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: No project initialization needed — the `frontend/` SPA already
exists. This phase only confirms the baseline.

- [x] T001 Confirm the `frontend/` workspace builds and the existing test suite is green (`cd frontend && pnpm install && pnpm test`) before starting.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Shared types, API client, query keys, permissions, and shared
components every user story depends on.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete.

- [x] T002 [P] Add the billing view shapes to `frontend/src/lib/api-types.ts`: `PlanView`, `PlanRef`, `SubscriptionView`, `SubscriptionState` (`pending|active|past_due|suspended|cancelled`), `UsageView`, `InvoiceView`, `InvoiceStatus` (`open|paid|void`), `LineItemView`, `PaymentAttemptView` — field shapes per `specs/011-phase-5-ui/data-model.md`.
- [x] T003 Add `'billing:get'` and `'billing:manage'` to the `Permission` union in `frontend/src/lib/api-types.ts`.
- [x] T004 Add a `billing` namespace to the API client in `frontend/src/lib/api.ts` with methods: `plans(slug)` → `GET /plans`; `getSubscription(slug)` → `GET /subscription`; `subscribe(slug, planId)` → `POST /subscription`; `cancelSubscription(slug)` → `DELETE /subscription`; `listInvoices(slug, limit, offset)` → `GET /invoices`; `getInvoice(slug, id)` → `GET /invoices/{id}`; `settleInvoice(slug, id)` → `POST /invoices/{id}/settle`. All paths via the `tp(slug, …)` helper.
- [x] T005 [P] Add query keys to `frontend/src/lib/query.ts`: `plans(slug)`, `subscription(slug)`, `invoices(slug)`, `invoicesPage(slug, limit, offset)`, `invoice(slug, id)`.
- [x] T006 [P] Create the `Money` shared component in `frontend/src/components/common/money.tsx` — formats a minor-unit integer + currency code into a localized amount (default RUB).
- [x] T007 [P] Create the `UsageMeter` shared component in `frontend/src/components/common/usage-meter.tsx` — renders used/included as a proportional bar with a near-limit visual indication and an exhausted state.
- [x] T008 Add a `Billing` nav entry to `frontend/src/components/shell/sidebar.tsx` NAV array, segment `billing`, gated on `requires: ['billing:get']`.

**Checkpoint**: Types, client, query keys, permissions, shared components, and nav are ready — user stories can now begin.

---

## Phase 3: User Story 1 - View current plan and subscription status (Priority: P1) 🎯 MVP

**Goal**: A billing home that shows the current plan, subscription state, and
billing period, with prominent past-due / suspended warnings and explicit
no-subscription and pending states.

**Independent Test**: Open `/t/{slug}/billing` for tenants in active,
past-due, suspended, no-subscription, and pending states and confirm each
shows its distinct presentation.

- [x] T009 [US1] Create the billing home route `frontend/src/routes/t/$slug/billing/index.tsx` — fetch `billing.getSubscription`, render plan name, `state`, and current billing period start/end for an `active` subscription; show `cancelAtPeriodEnd` read-only when set.
- [x] T010 [US1] In `billing/index.tsx`, render the `404 no_subscription` response as an explicit no-subscription state with a link to the plan catalogue (`billing/plans`), and the `pending` state as an explicit in-progress state.
- [x] T011 [US1] In `billing/index.tsx`, render the `past_due` state as a prominent warning (payment failed, retries in progress) linking to the unpaid invoice, and the `suspended` state as a prominent warning (suspended for non-payment, sending disabled).
- [x] T012 [P] [US1] Create the colocated test `frontend/src/routes/t/$slug/billing/index.test.tsx` covering active, no-subscription, pending, past-due, and suspended states with mocked `@/lib/api`.

**Checkpoint**: The billing home reads and presents every subscription state.

---

## Phase 4: User Story 2 - Browse plans and subscribe (Priority: P1)

**Goal**: A plan catalogue and self-service subscribe flow with a confirmation
summary, an in-progress state, and success / declined-charge outcomes.

**Independent Test**: As an unsubscribed tenant, open `/t/{slug}/billing/plans`,
select a plan, confirm, and land on an active subscription; repeat with the
mock gateway declining and confirm the failure is shown.

- [x] T013 [US2] Create the plan catalogue route `frontend/src/routes/t/$slug/billing/plans.tsx` — fetch `billing.plans`, render each plan's price (`Money`), currency, billing period, included allowance, and overage mode; show an explicit empty state for an empty list.
- [x] T014 [US2] In `billing/plans.tsx`, add the plan-selection confirmation summary showing the plan and the first-period charge amount before any charge is made.
- [x] T015 [US2] In `billing/plans.tsx`, wire the subscribe mutation (`billing.subscribe`): disable the trigger while in flight, show an in-progress state, and on `201` reflect the active subscription and route to / refresh the billing home.
- [x] T016 [US2] In `billing/plans.tsx`, map `402 payment_failed` to a declined-charge message with a retry path, and `409 subscription_exists` to a disabled subscribe action with an explanatory message.
- [x] T017 [P] [US2] Create the colocated test `frontend/src/routes/t/$slug/billing/plans.test.tsx` covering the catalogue render, empty state, the confirm + successful subscribe flow, the declined-charge outcome, and the already-subscribed disabled state.

**Checkpoint**: A tenant can subscribe to a plan end to end.

---

## Phase 5: User Story 3 - View current-period usage against allowance (Priority: P2)

**Goal**: A usage view showing metered sends consumed against the plan
allowance, the overage-mode consequence, a last-refreshed indication, and
prior-period readability.

**Independent Test**: Open `/t/{slug}/billing/usage` and confirm the consumed
figure matches sends made, shown against the allowance; drive usage to the
limit and confirm the overage-mode consequence copy.

- [x] T018 [US3] Create the usage route `frontend/src/routes/t/$slug/billing/usage.tsx` — fetch `billing.getSubscription`, render the embedded `UsageView` via `UsageMeter` (used vs included), and show a last-refreshed indication; show zeroes with a "not yet computed" note when no rollup has run.
- [x] T019 [US3] In `billing/usage.tsx`, render the exhausted-allowance consequence: for a `block`-mode plan state that further sends will be blocked; for a `meter`-mode plan state that further sends bill as overage and show `overageSends` accumulated.
- [x] T020 [US3] In `billing/usage.tsx`, present prior closed-period usage (from settled invoice line items) readable and separate from the current period.
- [x] T021 [P] [US3] Create the colocated test `frontend/src/routes/t/$slug/billing/usage.test.tsx` covering normal usage, the exhausted block-mode and meter-mode states, and the not-yet-computed state.

**Checkpoint**: A tenant can read current-period usage against the allowance.

---

## Phase 6: User Story 4 - View invoice and payment history (Priority: P2)

**Goal**: An invoice history listing invoices with status, plus an invoice
detail showing line items and payment attempts.

**Independent Test**: Open `/t/{slug}/billing/invoices`; confirm each invoice
shows period, total, currency, and paid/unpaid status; open a paid and an
unpaid invoice and confirm line items and payment attempts.

- [x] T022 [US4] Create the invoice history route `frontend/src/routes/t/$slug/billing/invoices.tsx` — fetch `billing.listInvoices` paged, render each invoice's billing period, total (`Money`), currency, and paid/unpaid status, visually distinguishing `open` from `paid`; show an explicit empty state and incremental/paged loading.
- [x] T023 [US4] In `billing/invoices.tsx`, add the master/detail panel: on row open fetch `billing.getInvoice`, render `lineItems` (description, quantity, amount) and `paymentAttempts` (outcome, failure reason); render `404 invoice_not_found` as a not-found state.
- [x] T024 [P] [US4] Create the colocated test `frontend/src/routes/t/$slug/billing/invoices.test.tsx` covering the list, the paid/unpaid distinction, the empty state, the detail panel (line items + attempts), and the not-found state.

**Checkpoint**: A tenant can review invoice and payment history.

---

## Phase 7: User Story 5 - Recover a suspended account (Priority: P3)

**Goal**: A workspace-wide suspension banner, a settle-balance action, and
surfacing of quota / suspension errors in the sending UI.

**Independent Test**: As a suspended tenant, confirm the banner on every page,
settle the balance, and confirm reinstatement; confirm a blocked campaign
start and send show the quota / suspension messages.

- [x] T025 [P] [US5] Create the `SuspensionBanner` component in `frontend/src/components/shell/suspension-banner.tsx` — reads the subscription state and renders a persistent suspended-for-non-payment banner with a link to the billing area; mount it in the app shell so it appears on every workspace page.
- [x] T026 [US5] In `billing/index.tsx` (and the invoice detail in `billing/invoices.tsx`), add the settle-balance action against the unpaid invoice (`billing.settleInvoice`): disable while in flight; on `200` confirm reinstatement, clear the banner, and refresh subscription state.
- [x] T027 [US5] Map the settle-balance outcomes: `402 payment_failed` → declined message, account remains suspended, try-again offered; `409 invoice_not_settleable` → reconcile silently to current state.
- [x] T028 [US5] In the existing campaign-start screen (`frontend/src/routes/t/$slug/campaigns/`), surface the `403 quota_exceeded` and `403 tenant_suspended` error kinds as actionable messages linking to `billing/usage` and `billing` respectively.
- [x] T029 [US5] In the existing transactional-send screen (`frontend/src/routes/t/$slug/transactional/`), surface the `403 quota_exceeded` and `403 tenant_suspended` error kinds as actionable messages linking to `billing/usage` and `billing` respectively.
- [x] T030 [P] [US5] Create the colocated test `frontend/src/components/shell/suspension-banner.test.tsx` and extend `billing/index.test.tsx` / `billing/invoices.test.tsx` to cover the settle-balance success and declined outcomes.

**Checkpoint**: Suspension is surfaced, the balance can be settled, and quota / suspension errors are actionable in the sending UI.

---

## Phase 8: Polish & Cross-Cutting Concerns

**Purpose**: Verification and consistency across all stories.

- [x] T031 Confirm permission gating end to end: the `Billing` nav entry is hidden for an operator without `billing:get`; subscribe and settle actions are hidden for one without `billing:manage`.
- [x] T032 [P] Confirm every billing view reconciles to current server state without a hard error when the subscription/invoice/usage changes in the background (FR-034).
- [x] T033 Run `cd frontend && pnpm test` and confirm the full suite, including all new route tests, is green.
- [ ] T034 Run the `specs/011-phase-5-ui/quickstart.md` verification steps for all five user stories against a running backend with the deterministic mock gateway.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies.
- **Foundational (Phase 2)**: Depends on Setup — BLOCKS all user stories.
- **User Stories (Phase 3–7)**: All depend on Foundational completion.
  - US1, US2, US3, US4 are independent and can proceed in parallel.
  - US5 depends on US1 (settle action lives in the billing home) and US4 (settle from invoice detail).
- **Polish (Phase 8)**: Depends on all desired user stories being complete.

### User Story Dependencies

- **US1 (P1)**: After Foundational. No dependency on other stories.
- **US2 (P1)**: After Foundational. Independent; links to the billing home but testable alone.
- **US3 (P2)**: After Foundational. Independent.
- **US4 (P2)**: After Foundational. Independent.
- **US5 (P3)**: After Foundational; integrates with US1 and US4 for the settle action placement.

### Within Each User Story

- Route implementation before its colocated test refinement; tests marked [P] can be authored alongside.
- Error-state handling tasks follow the happy-path route task for the same file.

### Parallel Opportunities

- Foundational: T002/T005/T006/T007 are [P] (different files); T003 and T004 touch shared files (`api-types.ts`, `api.ts`).
- Once Foundational completes, US1–US4 can be developed in parallel by different developers.
- Each story's colocated test task ([P]) can be written alongside the route.

---

## Parallel Example: Foundational Phase

```bash
# After T001, launch the independent foundational tasks together:
Task: "Add billing view shapes to frontend/src/lib/api-types.ts"   # T002
Task: "Add query keys to frontend/src/lib/query.ts"                # T005
Task: "Create Money component in components/common/money.tsx"      # T006
Task: "Create UsageMeter component in components/common/usage-meter.tsx"  # T007
```

---

## Implementation Strategy

### MVP First (User Stories 1 + 2)

1. Complete Phase 1: Setup.
2. Complete Phase 2: Foundational (CRITICAL — blocks all stories).
3. Complete Phase 3 (US1) and Phase 4 (US2) — together these deliver the MVP: a tenant can subscribe and see subscription status.
4. **STOP and VALIDATE**: Test US1 and US2 independently.
5. Deploy/demo if ready.

### Incremental Delivery

1. Setup + Foundational → foundation ready.
2. US1 → test → demo.
3. US2 → test → demo (MVP — subscribe + status).
4. US3 → test → demo (usage visibility).
5. US4 → test → demo (invoice history).
6. US5 → test → demo (recovery + sending-UI error surfacing).

### Parallel Team Strategy

After Foundational: Developer A takes US1, B takes US2, C takes US3, D takes
US4; US5 follows once US1 and US4 land.

---

## Notes

- [P] tasks = different files, no dependencies.
- [Story] label maps each task to a user story for traceability.
- This feature touches no backend code; the existing Phase 5 Go suite stays green.
- Commit after each task or logical group.
- Stop at any checkpoint to validate a story independently.
