# Implementation Plan: Phase 5 — Billing & Metering — Frontend UI

**Branch**: `011-phase-5-ui` | **Date**: 2026-05-19 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/011-phase-5-ui/spec.md`

## Summary

Deliver the tenant-workspace web interface for the already-built Phase 5
Billing & Metering backend: a billing area showing the current plan and
subscription state, a plan catalogue and self-service subscribe flow, a
current-period usage view, an invoice and payment history, and a
settle-balance recovery path for suspended tenants. The UI also surfaces
quota-exceeded and suspended-account outcomes in the existing campaign and
transactional sending screens.

The UI extends the existing Phase 1–4 React/TanStack app shell with one new
permission-gated nav area (Billing) carrying a small set of routes, plus a
persistent workspace-wide suspension banner. It reuses the shared design
system, the typed tenant-scoped API client (`tp(slug, …)`), the
`async-state`/`confirm-dialog`/`data-table` patterns, and the permission
gating. **No backend work is required** — all seven Phase 5 endpoints
(`/plans`, `/subscription` GET/POST/DELETE, `/invoices`, `/invoices/{id}`,
`/invoices/{id}/settle`) already exist in
`internal/api/billing_handlers.go`, and campaign-start / transactional-send
already return the `quota_exceeded` and `tenant_suspended` errors this UI
surfaces.

## Technical Context

**Language/Version**: TypeScript 5.9 / React 19 (frontend only)

**Primary Dependencies**: TanStack Start/Router/Query/Form/Table, shadcn +
Radix UI, Tailwind v4, lucide-react, sonner

**Storage**: PostgreSQL via the existing Phase 5 schema — no new tables or
migrations; the UI is a pure consumer of existing endpoints

**Testing**: vitest + @testing-library/react (colocated `*.test.tsx` per
route, `renderWithClient` helper, mocked `@/lib/api` and
`@tanstack/react-router`)

**Target Platform**: Modern desktop browsers

**Project Type**: Web application — existing `frontend/` SPA; Go backend
untouched

**Performance Goals**: Interactive UI. Subscribe and settle-balance are
synchronous gateway charges — the UI shows an in-progress state and disables
the action while the request is in flight. Usage figures come from a periodic
rollup job, so the UI displays the server-provided figures and a
last-refreshed indication rather than polling.

**Constraints**: No new frontend framework; extend not rebuild the app shell;
no new backend endpoints, commands, or schema; the mock gateway means no
card-entry surface; subscribe and settle must each resolve to exactly one
outcome and must not allow a duplicate in-flight charge.

**Scale/Scope**: 1 new nav area (Billing), ~4 new file-routes (billing home /
subscription status, plan catalogue, usage view, invoice history with invoice
detail), 1 workspace-wide suspension banner in the app shell, 1 new API client
namespace (`billing`) with ~6 methods, ~5 new query keys, 2 new `Permission`
union members (`billing:get`, `billing:manage`), and surfacing two new error
kinds in the existing sending screens. ~2 small shared presentational
components (usage meter, money/amount display).

## Constitution Check

*GATE: evaluated against `.specify/memory/constitution.md` v1.1.0.*

- **I. Tenant Isolation by Default** — PASS. No data-layer changes. Every API
  call goes through the tenant-scoped client (`tp(slug, …)`), which makes
  `slug` a required first argument, so a call site cannot omit tenant scope.
  The Phase 5 backend scopes every plan, subscription, invoice, and usage
  figure to the tenant; `/invoices/{id}` returns `404 invoice_not_found` for
  another tenant's invoice and the UI renders that as a not-found state.
- **II. Test-Backed Delivery** — PASS. Each new frontend route ships a
  colocated `*.test.tsx` covering its primary flow plus the key
  empty/in-progress/error states (no-subscription, pending, past-due,
  suspended, declined charge). No backend change, so the existing Phase 5
  suite stays green.
- **III. Incremental, Shippable Phases** — PASS. The five user stories are
  independently shippable in priority order (P1 status, P1 subscribe, P2
  usage, P2 invoices, P3 recovery). No speculative scope; plan
  upgrade/downgrade, proration, cancellation UI, and a real card-entry surface
  are explicitly excluded.
- **IV. Security & Consent by Design** — PASS. Nav and action gating reuse the
  Phase 5 permission strings — `billing:get` to view the billing area, plans,
  subscription, usage, and invoices; `billing:manage` to subscribe, cancel, or
  settle. The backend re-checks every request and stays authoritative; a
  `403`/`404` is rendered in place, a `401` routes to sign-in. The mock
  gateway means no payment-instrument data is captured or stored by the UI.
- **V. Operable & Observable Services** — PASS. The frontend is stateless. No
  service, job, or queue change.
- **VI. Layered Architecture & Domain Integrity** — PASS. No backend code. The
  frontend keeps transport isolated in `lib/api.ts`; routes consume typed view
  shapes from `lib/api-types.ts` and never construct URLs themselves; error
  kinds are mapped to UI states in one place per route.

**Result**: PASS — no violations, Complexity Tracking not required.

*Post-design re-check*: the design adds no new layers, no DI, no schema
change, no duplicated infrastructure, and no backend code. Still PASS.

## Project Structure

### Documentation (this feature)

```text
specs/011-phase-5-ui/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output — view shapes the UI consumes
├── quickstart.md        # Phase 1 output
├── contracts/
│   └── tenant-api.md    # Phase 1 output — the Phase 5 endpoints the UI consumes
├── checklists/
│   └── requirements.md  # Spec quality checklist (/speckit-specify)
└── tasks.md             # Phase 2 output (/speckit-tasks — NOT created here)
```

### Source Code (repository root)

```text
frontend/src/
├── lib/
│   ├── api.ts                       # + billing namespace: plans, getSubscription,
│   │                                #   subscribe, cancelSubscription, listInvoices,
│   │                                #   getInvoice, settleInvoice
│   ├── api-types.ts                 # + PlanView, SubscriptionView, UsageView,
│   │                                #   InvoiceView, LineItemView, PaymentAttemptView;
│   │                                #   + 'billing:get' | 'billing:manage' in Permission
│   └── query.ts                     # + query keys: plans, subscription, invoices,
│                                    #   invoicesPage, invoice
├── components/
│   ├── common/
│   │   ├── usage-meter.tsx          # NEW — used/included allowance bar + overage state
│   │   └── money.tsx                # NEW — minor-unit amount + currency display
│   └── shell/
│       ├── sidebar.tsx              # + 1 nav entry (Billing), gated on billing:get
│       └── suspension-banner.tsx    # NEW — workspace-wide suspended-for-non-payment banner
└── routes/t/$slug/
    └── billing/
        ├── index.tsx                # billing home: plan + subscription state, warnings,
        │                            #   no-subscription / pending states (US1, US5 entry)
        ├── plans.tsx                # plan catalogue + subscribe flow (US2)
        ├── usage.tsx                # current-period usage vs allowance (US3)
        └── invoices.tsx             # invoice history + invoice detail (US4)
```

**Structure Decision**: Existing web-application layout. The frontend SPA in
`frontend/` is extended with file-routes under the established
`routes/t/$slug/` tree, in a new `billing/` segment. The billing home
(`index.tsx`) is the landing route; plans, usage, and invoices are sibling
routes reached from it. Invoice detail is rendered within `invoices.tsx`
(master/detail) rather than as a separate file-route, matching the existing
list+detail pattern. The suspension banner lives in the app shell so it
appears on every workspace page (FR-024). No new top-level directories; the Go
backend is not touched.

## Phase 0 — Research

Complete. See [research.md](./research.md). All decisions resolved from
in-repo inspection of the Phase 5 backend
(`internal/api/billing_handlers.go`, `internal/api/server.go`, the billing
query views, `internal/iam/domain/permission.go`, the 010 contracts) and the
Phase 1–4 frontend conventions; no `NEEDS CLARIFICATION` remain.

## Phase 1 — Design & Contracts

Complete:
- [data-model.md](./data-model.md) — the view shapes the UI consumes
  (`PlanView`, `SubscriptionView`, `UsageView`, `InvoiceView`,
  `LineItemView`, `PaymentAttemptView`) and the subscription-state /
  invoice-status enumerations; no new persisted entities.
- [contracts/tenant-api.md](./contracts/tenant-api.md) — the seven Phase 5
  endpoints the UI consumes, their request/response shapes, permission
  requirements, and error-kind → UI-state mapping (including the
  `quota_exceeded` and `tenant_suspended` errors surfaced in the sending UI).
- [quickstart.md](./quickstart.md) — run, verify, and test instructions.
- Agent context (`CLAUDE.md`) updated to point at this plan.

## Phase 2 — Next step

Run `/speckit-tasks` to generate `tasks.md`. Suggested task ordering:

1. **Shared frontend plumbing** — extend `api-types.ts` with the six view
   shapes and the two new `Permission` members; add the `billing` API client
   namespace; add the query keys; add the `Billing` sidebar nav entry; add the
   `UsageMeter` and `Money` shared components; add the `SuspensionBanner` to
   the app shell.
2. **US1 Subscription status** (P1) — `billing/index.tsx` with the plan +
   state read, past-due and suspended warnings, and the no-subscription and
   pending states.
3. **US2 Plan catalogue & subscribe** (P1) — `billing/plans.tsx` with the plan
   list, the confirmation summary, the in-flight in-progress state, the
   success and declined-charge outcomes, the empty catalogue state, and the
   already-subscribed disabled state.
4. **US3 Usage view** (P2) — `billing/usage.tsx` with the usage meter against
   allowance, the overage-mode consequence copy, last-refreshed indication,
   and prior-period readability.
5. **US4 Invoice history** (P2) — `billing/invoices.tsx` with the paginated
   invoice list, the invoice detail (line items + payment attempts), the
   paid/unpaid distinction, the empty state, and the not-found state.
6. **US5 Recovery** (P3) — wire the settle-balance action into
   `billing/index.tsx`/`invoices.tsx`, finish the workspace-wide
   `SuspensionBanner`, and surface the `quota_exceeded` / `tenant_suspended`
   error kinds in the existing campaign-start and transactional-send screens.

Each user story is independently demonstrable and testable per its spec
Independent Test.

## Complexity Tracking

No constitution violations — section intentionally empty.
