# Implementation Plan: Phase 4 — Deliverability & Analytics — Frontend UI

**Branch**: `009-phase-4-ui` | **Date**: 2026-05-18 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/009-phase-4-ui/spec.md`

## Summary

Deliver the tenant-workspace web interface for the already-built Phase 4
deliverability features: per-campaign analytics, the workspace deliverability
dashboard, the per-tenant suppression list, and bounce-action settings. The UI
extends the existing Phase 1–3 React/TanStack app shell with two new
permission-gated nav areas (Dashboard, Suppression list), one child route under
the existing campaign detail (analytics), and a bounce-settings panel. It reuses
the shared design system, the typed tenant-scoped API client, and the
async-state conventions. **No backend work is required** — all five Phase 4
endpoints (`/suppressions`, `/bounce-settings`, `/campaigns/{id}/analytics`,
`/dashboard`) already exist and are wired in `internal/api/server.go`.

## Technical Context

**Language/Version**: TypeScript 5.9 / React 19 (frontend only)

**Primary Dependencies**: TanStack Start/Router/Query/Form/Table, shadcn +
Radix UI, Tailwind v4, lucide-react, sonner

**Storage**: PostgreSQL via the existing Phase 4 schema — no new tables or
migrations in this feature; the UI is a pure consumer of existing endpoints

**Testing**: vitest + @testing-library/react (colocated `*.test.tsx` per route)

**Target Platform**: Modern desktop browsers

**Project Type**: Web application — existing `frontend/` SPA; backend untouched

**Performance Goals**: Interactive UI. Analytics and dashboard are served from
pre-computed summaries and refreshed by a backend schedule; the UI re-fetches
on a modest stale window (~60 s) rather than polling aggressively — no live
transport

**Constraints**: No new frontend framework; extend not rebuild the app shell;
no new backend endpoints, commands, or schema; analytics figures are
near-real-time (minutes of lag), so the UI shows the server `refreshedAt`
timestamp rather than implying live data

**Scale/Scope**: 2 new nav areas, 4 new file-routes (dashboard, suppression
list, bounce settings, campaign analytics), ~3 new API client method groups,
~4 new query keys, 1–2 shared presentational components (metric tile, rate
display); no new hooks beyond thin query wrappers

## Constitution Check

*GATE: evaluated against `.specify/memory/constitution.md` v1.1.0.*

- **I. Tenant Isolation by Default** — PASS. No data-layer changes. Every API
  call goes through the tenant-scoped client (`tp(slug, …)`); `slug` is a
  required first argument, so a call site cannot omit tenant scope. The backend
  endpoints scope every figure to the tenant via RLS and request context.
- **II. Test-Backed Delivery** — PASS. Each new frontend route ships a
  colocated `*.test.tsx` covering its primary flow and key empty/awaiting-data
  states. No backend change, so no new backend tests are needed; the existing
  Phase 4 suite stays green.
- **III. Incremental, Shippable Phases** — PASS. The four user stories are
  independently shippable in priority order (P1 analytics, P1 suppression list,
  P2 dashboard, P3 bounce settings). No speculative scope; per-recipient event
  detail and consumer monitoring are explicitly excluded.
- **IV. Security & Consent by Design** — PASS. Nav/action gating reuses the
  existing permission strings — `campaigns:get` for analytics and the
  dashboard, `sending:get` to view the suppression list and bounce settings,
  `sending:manage` to add/remove suppressions and update bounce settings. The
  backend re-checks every request and remains authoritative; a `403`/`404` is
  rendered in place, a `401` routes to sign-in.
- **V. Operable & Observable Services** — PASS. The frontend is stateless. No
  service or queue change.
- **VI. Layered Architecture & Domain Integrity** — PASS. No backend code. The
  frontend keeps transport isolated in `lib/api.ts`; routes consume typed view
  shapes from `lib/api-types.ts` and never construct URLs themselves.

**Result**: PASS — no violations, Complexity Tracking not required.

*Post-design re-check*: the design adds no new layers, no DI, no schema change,
no duplicated infrastructure, and no backend code. Still PASS.

## Project Structure

### Documentation (this feature)

```text
specs/009-phase-4-ui/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/
│   └── tenant-api.md    # Phase 1 output — the Phase 4 endpoints the UI consumes
├── checklists/
│   └── requirements.md  # Spec quality checklist (/speckit-specify)
└── tasks.md             # Phase 2 output (/speckit-tasks — NOT created here)
```

### Source Code (repository root)

```text
frontend/src/
├── lib/
│   ├── api.ts                       # + suppressions, bounceSettings, analytics, dashboard methods
│   ├── api-types.ts                 # + SuppressionEntry, BounceSettings, CampaignAnalytics, DashboardView
│   └── query.ts                     # + query keys: dashboard, campaignAnalytics, suppressions, bounceSettings
├── components/
│   └── common/
│       ├── metric-tile.tsx          # NEW — labelled count + optional rate tile
│       └── rate-value.tsx           # NEW — fraction → percentage display (0% on zero denominator)
├── components/shell/
│   └── sidebar.tsx                  # + 2 nav entries (Dashboard, Suppression list), permission-gated
└── routes/t/$slug/
    ├── dashboard/
    │   └── index.tsx                # workspace deliverability dashboard (US3)
    ├── suppressions/
    │   ├── index.tsx                # suppression list: view/filter/search/add/remove (US2)
    │   └── settings.tsx             # bounce-action settings panel (US4)
    └── campaigns/
        └── $id.analytics.tsx        # per-campaign analytics view (US1)
```

**Structure Decision**: Existing web-application layout. The frontend SPA in
`frontend/` is extended with file-routes under the established
`routes/t/$slug/` tree. Campaign analytics is a child route of the existing
`campaigns/$id.tsx` detail page (reached via a link/tab from it). Bounce-action
settings is a sibling route of the suppression list, reached from a control on
the suppression-list page. No new top-level directories; the Go backend is not
touched.

## Phase 0 — Research

Complete. See [research.md](./research.md). All decisions resolved from in-repo
inspection of the Phase 4 backend (`internal/api/server.go`,
`analytics_handlers.go`, `suppression_handlers.go`, the 008 contracts) and the
Phase 1–3 frontend conventions; no `NEEDS CLARIFICATION` remain.

## Phase 1 — Design & Contracts

Complete:
- [data-model.md](./data-model.md) — the view shapes the UI consumes
  (`SuppressionEntry`, `BounceSettings`, `CampaignAnalytics`, `DashboardView`);
  no new persisted entities.
- [contracts/tenant-api.md](./contracts/tenant-api.md) — the five Phase 4
  endpoints the UI consumes, their request/response shapes, permission
  requirements, and error mapping.
- [quickstart.md](./quickstart.md) — run, verify, and test instructions.
- Agent context (`CLAUDE.md`) updated to point at this plan.

## Phase 2 — Next step

Run `/speckit-tasks` to generate `tasks.md`. Suggested task ordering:

1. **Shared frontend plumbing** — extend `api-types.ts` with the four view
   shapes, add the `suppressions` / `bounceSettings` / `analytics` /
   `dashboard` API client methods, add query keys, add the two sidebar nav
   entries, and add the `MetricTile` and `RateValue` shared components.
2. **US1 Campaign analytics** (P1) — `campaigns/$id.analytics.tsx` with counts,
   rates, refreshed-at, awaiting-data and not-found states + link from the
   campaign detail page.
3. **US2 Suppression list** (P2 ordering note: P1) — `suppressions/index.tsx`
   with the list, reason filter, address search, incremental loading, the
   add-address form with email validation, and confirmed removal.
4. **US3 Dashboard** (P2) — `dashboard/index.tsx` with aggregate metrics, the
   recent-campaign list, drill-down links, and the empty state.
5. **US4 Bounce settings** (P3) — `suppressions/settings.tsx` with the two
   toggles, explanatory copy, save, and persistence.

Each user story is independently demonstrable and testable per its spec
Independent Test.

## Complexity Tracking

No constitution violations — section intentionally empty.
