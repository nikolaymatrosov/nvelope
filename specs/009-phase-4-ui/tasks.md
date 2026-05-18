---

description: "Task list for Phase 4 — Deliverability & Analytics — Frontend UI"
---

# Tasks: Phase 4 — Deliverability & Analytics — Frontend UI

**Input**: Design documents from `/specs/009-phase-4-ui/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/tenant-api.md

**Tests**: Test tasks ARE included. Constitution Principle II (Test-Backed
Delivery) is non-negotiable, and plan.md commits to a colocated `*.test.tsx`
per new route and per new shared component.

**Organization**: Tasks are grouped by user story. This feature is
frontend-only — no Go backend, schema, or migration change. All paths are under
`frontend/`.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: US1–US4, mapping to the spec's user stories
- Exact file paths are included in every task

## Path Conventions

Web application. Frontend SPA lives in `frontend/`; routes under
`frontend/src/routes/t/$slug/`. The Go backend is not touched.

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Confirm the working environment before any code changes.

- [x] T001 Verify the Phase 4 backend endpoints respond locally — `GET /t/{slug}/api/suppressions`, `GET /t/{slug}/api/bounce-settings`, `GET /t/{slug}/api/campaigns/{id}/analytics`, `GET /t/{slug}/api/dashboard` — per `specs/009-phase-4-ui/quickstart.md`; run `cd frontend && pnpm install`.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Shared frontend plumbing every user story depends on. All four
files below are single shared files, so these tasks must complete before any
story phase begins.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete.

- [x] T002 Add the Phase 4 view shapes to `frontend/src/lib/api-types.ts` — `SuppressionEntry`, `SuppressionListResponse`, `SuppressionReason` (`"hard_bounce" | "complaint" | "manual"`), `BounceSettings`, `CampaignAnalytics`, `DashboardView`, `RecentCampaign` — per `specs/009-phase-4-ui/data-model.md`.
- [x] T003 Add the API client method groups to `frontend/src/lib/api.ts` — `api.suppressions.list/add/remove`, `api.bounceSettings.get/update`, `api.analytics.campaign`, `api.analytics.dashboard` — each taking `slug` first and routing through `tp(slug, …)`, per `specs/009-phase-4-ui/contracts/tenant-api.md`.
- [x] T004 Add query keys to `frontend/src/lib/query.ts` — `dashboard(slug)`, `campaignAnalytics(slug, id)`, `suppressions(slug, { reason, email })`, `bounceSettings(slug)` — following the existing slug-led key factory pattern.
- [x] T005 Add two permission-gated nav entries to `frontend/src/components/shell/sidebar.tsx` — "Dashboard" (`segment: "dashboard"`, requires `["campaigns:get", "campaigns:manage"]`) and "Suppression list" (`segment: "suppressions"`, requires `["sending:get", "sending:manage"]`) — with suitable lucide icons.
- [x] T006 [P] Create the `RateValue` shared component in `frontend/src/components/common/rate-value.tsx` — renders a 0.0–1.0 fraction as a percentage, rendering `0%` for a zero value (no special-casing needed; backend yields `0.0`).
- [x] T007 [P] Create the `MetricTile` shared component in `frontend/src/components/common/metric-tile.tsx` — a labelled count with an optional rate line, for use by the analytics view and the dashboard.
- [x] T008 [P] Add `rate-value.test.tsx` and `metric-tile.test.tsx` in `frontend/src/components/common/` covering fraction→percentage formatting, the zero case, and the optional-rate render.

**Checkpoint**: Shared plumbing ready — user stories can now proceed.

---

## Phase 3: User Story 1 - View campaign analytics (Priority: P1) 🎯 MVP

**Goal**: An operator opens a sent campaign and sees its sent/delivered/opened/
clicked/bounced/complained counts and the four rates, with the last-refreshed
time, an awaiting-data state, and a not-found state.

**Independent Test**: Open a campaign with mixed activity and confirm counts and
rates match; open a just-sent campaign and confirm the awaiting-data state.

### Implementation for User Story 1

- [x] T009 [US1] Create the campaign analytics route `frontend/src/routes/t/$slug/campaigns/$id.analytics.tsx` — fetch via `api.analytics.campaign`, gate on `campaigns:get`, render counts with `MetricTile` and rates with `RateValue`, show `refreshedAt`, the `refreshedAt === null` "awaiting data" state (FR-004), 0% on zero denominators (FR-005), and the `404 campaign-not-found` state (FR-006); wrap async in the existing `AsyncState` component.
- [x] T010 [US1] Add a link to the analytics route from the campaign detail page `frontend/src/routes/t/$slug/campaigns/$id.tsx`, shown for sent campaigns and gated on `campaigns:get` (FR-001).
- [x] T011 [P] [US1] Add `frontend/src/routes/t/$slug/campaigns/$id.analytics.test.tsx` — covers counts/rates render, the awaiting-data state, the zero-denominator render, and the not-found state.

**Checkpoint**: User Story 1 is independently functional and testable.

---

## Phase 4: User Story 2 - Manage the suppression list (Priority: P1)

**Goal**: An operator views the tenant's suppression list with reason and date,
filters by reason, searches by address, loads long lists incrementally, adds an
address with email validation, and removes an entry behind a confirmation.

**Independent Test**: View the list, filter and search, add a valid and an
invalid address, remove an entry confirming the prompt appears first.

### Implementation for User Story 2

- [x] T012 [US2] Create the suppression list route `frontend/src/routes/t/$slug/suppressions/index.tsx` — fetch via `api.suppressions.list`, gate viewing on `sending:get`, render each entry's address, reason label, and date (`lib/format.ts`), and the empty state when `items` is empty with no active filter (FR-007, FR-008); wrap async in `AsyncState`.
- [x] T013 [US2] Add the reason filter and address search controls to `frontend/src/routes/t/$slug/suppressions/index.tsx` — bound to the `reason` and `email` query params and folded into the `queryKeys.suppressions` key so a filter change starts a fresh cursor sequence (FR-009).
- [x] T014 [US2] Add cursor-based incremental loading (load-more or infinite scroll) to `frontend/src/routes/t/$slug/suppressions/index.tsx` using the response `nextCursor`, preserving the active filter and search (FR-013).
- [x] T015 [US2] Add the manual add-address form to `frontend/src/routes/t/$slug/suppressions/index.tsx` — gated on `sending:manage`, using `FormField` with client-side email validation for an inline message, mapping the backend `422 validation_failed` to an inline field error, and treating `POST` as idempotent — invalidate `queryKeys.suppressions` on success (FR-010, FR-011, FR-014).
- [x] T016 [US2] Add confirmed removal to `frontend/src/routes/t/$slug/suppressions/index.tsx` — gated on `sending:manage`, using the existing `ConfirmDialog` before `api.suppressions.remove`, treating a `404 suppression_not_found` as success-equivalent (invalidate, no error toast), per FR-012 and FR-014.
- [x] T017 [P] [US2] Add `frontend/src/routes/t/$slug/suppressions/index.test.tsx` — covers list render, empty state, reason filter + address search, incremental loading, add with valid/invalid email, and confirmed removal including the `404` race.

**Checkpoint**: User Stories 1 and 2 both work independently.

---

## Phase 5: User Story 3 - Workspace deliverability dashboard (Priority: P2)

**Goal**: An operator opens the workspace dashboard and sees aggregate counts,
overall bounce/complaint rates, and a recent-campaign list with drill-down.

**Independent Test**: Open the dashboard with recent activity and confirm
totals, rates, and recent campaigns; click a campaign and confirm its analytics
view opens; confirm the empty state with no activity.

### Implementation for User Story 3

- [x] T018 [US3] Create the dashboard route `frontend/src/routes/t/$slug/dashboard/index.tsx` — fetch via `api.analytics.dashboard`, gate on `campaigns:get`, render aggregate counts with `MetricTile`, overall bounce/complaint rates with `RateValue`, and the empty state when `totals.sent === 0` with no recent campaigns (FR-015, FR-018); wrap async in `AsyncState`.
- [x] T019 [US3] Add the recent-campaign list to `frontend/src/routes/t/$slug/dashboard/index.tsx` — each row shows name, sent count, open/bounce/complaint rates, most-recent first, and links to `/t/$slug/campaigns/$id/analytics` (FR-016, FR-017).
- [x] T020 [P] [US3] Add `frontend/src/routes/t/$slug/dashboard/index.test.tsx` — covers totals + rates render, the recent-campaign list and drill-down link, and the empty state.

**Checkpoint**: User Stories 1, 2, and 3 all work independently.

---

## Phase 6: User Story 4 - Configure bounce actions (Priority: P3)

**Goal**: An operator opens bounce-action settings, sees two toggles defaulting
to on with explanatory copy, changes one, saves, and the change persists.

**Independent Test**: Open the settings, confirm both toggles default to on,
turn one off, save, reload, confirm it persisted.

### Implementation for User Story 4

- [x] T021 [US4] Create the bounce-settings route `frontend/src/routes/t/$slug/suppressions/settings.tsx` — fetch via `api.bounceSettings.get`, gate viewing on `sending:get`, render the `suppressOnHardBounce` and `suppressOnComplaint` toggles with explanatory copy on the deliverability consequence of disabling each (FR-019, FR-021); wrap async in `AsyncState`.
- [x] T022 [US4] Add save behaviour to `frontend/src/routes/t/$slug/suppressions/settings.tsx` — gated on `sending:manage`, an in-form dirty copy of the toggles, `api.bounceSettings.update` on save, cache update from the response, and a success toast (FR-020).
- [x] T023 [US4] Add a link to the bounce-settings route from the suppression list page `frontend/src/routes/t/$slug/suppressions/index.tsx` (research.md Decision 3).
- [x] T024 [P] [US4] Add `frontend/src/routes/t/$slug/suppressions/settings.test.tsx` — covers default-on toggles, changing + saving, and persistence after reload.

**Checkpoint**: All four user stories are independently functional.

---

## Phase 7: Polish & Cross-Cutting Concerns

**Purpose**: Verification and consistency across the feature.

- [x] T025 [P] Confirm permission gating end-to-end — sign in as a user lacking `sending:*` / `campaigns:*` and verify the Dashboard and Suppression list nav entries and the campaign analytics link are hidden/disabled (FR-023, SC-007).
- [x] T026 Run `cd frontend && pnpm typecheck && pnpm lint && pnpm test` and fix any failures; confirm `make test` still passes (no backend change expected).
- [x] T027 Walk through `specs/009-phase-4-ui/quickstart.md` against a running stack to validate all Success Criteria.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — start immediately.
- **Foundational (Phase 2)**: Depends on Setup — BLOCKS all user stories.
- **User Stories (Phase 3–6)**: All depend on Foundational completion.
  - US1 and US2 are both P1; US3 (P2) and US4 (P3) follow.
  - With staff, stories can proceed in parallel after Phase 2.
- **Polish (Phase 7)**: Depends on all targeted stories being complete.

### User Story Dependencies

- **US1 (P1)**: After Foundational — no dependency on other stories.
- **US2 (P1)**: After Foundational — no dependency on other stories.
- **US3 (P2)**: After Foundational — links to US1's analytics route; if US1 is
  not yet built, the drill-down link is still correct but lands on a route added
  by US1. Build US1 first if delivering sequentially.
- **US4 (P3)**: After Foundational — T023 adds a link from the US2 page; build
  US2 first if delivering sequentially.

### Within Each User Story

- For US2 and US4 the route file is shared across tasks, so those tasks run
  sequentially within the story; only the colocated `*.test.tsx` is `[P]`.
- US1's route, the campaign-detail link, and its test touch different files.

### Parallel Opportunities

- T006, T007, T008 (shared components + their tests) run in parallel.
- After Phase 2, US1/US2/US3/US4 can be staffed in parallel.
- Each story's `*.test.tsx` task is `[P]` against its implementation files.

---

## Parallel Example: Foundational shared components

```bash
Task: "Create RateValue in frontend/src/components/common/rate-value.tsx"
Task: "Create MetricTile in frontend/src/components/common/metric-tile.tsx"
Task: "Add rate-value.test.tsx and metric-tile.test.tsx"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Phase 1: Setup.
2. Phase 2: Foundational (CRITICAL — blocks all stories).
3. Phase 3: User Story 1 — campaign analytics.
4. **STOP and VALIDATE**: open a sent campaign's analytics view.
5. Deploy/demo if ready.

### Incremental Delivery

1. Setup + Foundational → foundation ready.
2. US1 (campaign analytics) → test → demo (MVP).
3. US2 (suppression list) → test → demo.
4. US3 (dashboard) → test → demo.
5. US4 (bounce settings) → test → demo.

### Parallel Team Strategy

After Phase 2: Developer A on US1, B on US2, C on US3, D on US4. Build US1
before US3's drill-down demo and US2 before US4's link demo.

---

## Notes

- This feature touches no Go code — `make test` must stay green unchanged.
- [P] = different files, no dependencies.
- The shared files in Phase 2 (`api-types.ts`, `api.ts`, `query.ts`,
  `sidebar.tsx`) are intentionally done once, up front, to avoid cross-story
  merge conflicts.
- Reuse existing shared components — `AsyncState`, `ConfirmDialog`, `FormField`
  — rather than building new ones.
- Commit after each task or logical group.
