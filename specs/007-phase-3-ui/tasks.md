---
description: "Task list for Phase 3 Sending Pipeline — Frontend UI"
---

# Tasks: Phase 3 Sending Pipeline — Frontend UI

**Input**: Design documents from `/specs/007-phase-3-ui/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/tenant-api.md, quickstart.md

**Tests**: Test tasks ARE included — Constitution Principle II (Test-Backed Delivery) is
non-negotiable, and plan.md mandates a colocated `*.test.tsx` per new route and Go
handler/command tests for the new backend endpoints.

**Organization**: Tasks are grouped by user story. US1 and US2 are both Priority P1;
US1 (Sending Domains) is the recommended MVP because every campaign and transactional
send is gated on a verified domain.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[Story]**: US1–US4 maps to the spec user stories
- All paths are repository-relative

## Path Conventions

- Frontend SPA: `frontend/src/`
- Go backend: `internal/`

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Confirm the workspace is ready for Phase 3 UI work

- [X] T001 Verify branch `007-phase-3-ui`, run `cd frontend && pnpm install`, and confirm `go build ./...` and `cd frontend && pnpm typecheck` both pass on a clean tree

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Shared frontend plumbing every Phase 3 screen depends on — transport, types, query keys, and navigation

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

- [X] T002 [P] Extend `frontend/src/lib/api-types.ts`: add `sending:get`, `sending:manage`, `campaigns:get`, `campaigns:manage`, `transactional:send` to the `Permission` union and `ALL_PERMISSIONS`; add `DNSRecord`, `DomainView`, `TemplateView`, `CampaignView`, and `CampaignCreate`/`CampaignUpdate` request types per data-model.md
- [X] T003 [P] Extend `frontend/src/lib/api.ts` with tenant-scoped methods for all Phase 3 endpoints in contracts/tenant-api.md: `addSendingDomain`, `listSendingDomains`, `getSendingDomain`, `recheckSendingDomain`, `createTemplate`, `listTemplates`, `getTemplate`, `updateTemplate`, `deleteTemplate`, `createCampaign`, `listCampaigns`, `getCampaign`, `updateCampaign`, `startCampaign`, `pauseCampaign`, `resumeCampaign`, `cancelCampaign`
- [X] T004 [P] Extend `frontend/src/lib/query.ts` key factory with `sendingDomains`/`sendingDomain`, `templates`/`templatesPage`/`template`, `campaigns`/`campaignsPage`/`campaign` keys (all slug-led)
- [X] T005 [P] Add four permission-gated nav entries — Sending Domains, Templates, Campaigns, Transactional Sending — to `frontend/src/components/shell/sidebar.tsx` with appropriate `requires` permission arrays and lucide icons

**Checkpoint**: Foundation ready — user story implementation can now begin

---

## Phase 3: User Story 1 — Verify a sending domain (Priority: P1) 🎯 MVP

**Goal**: Operators add a sending domain, copy its DKIM/SPF/DMARC records, trigger a re-check, and watch status reach `verified` without a reload.

**Independent Test**: Open Sending Domains, add a domain, confirm DNS records are displayed and copyable, trigger a re-check, confirm the domain reaches `verified` via background polling and a failed domain shows a reason.

### Tests for User Story 1

- [X] T006 [P] [US1] Route test `frontend/src/routes/t/$slug/sending-domains/index.test.tsx` — empty state, add-domain form, recheck busy state, permission gating
- [X] T007 [P] [US1] Route test `frontend/src/routes/t/$slug/sending-domains/$id.test.tsx` — DNS records render and are copyable, failed domain shows reason

### Implementation for User Story 1

- [X] T008 [P] [US1] Create `frontend/src/components/common/dns-record-row.tsx` — a copyable DNS record row (type, name, value) reused for DKIM/SPF/DMARC, using the shared design system and a copy-to-clipboard action
- [X] T009 [P] [US1] Create `frontend/src/hooks/use-sending-domains.ts` — query hook that polls (~5 s `refetchInterval`) while any domain status is `pending`, mirroring the `useJobStatus` pattern
- [X] T010 [US1] Create `frontend/src/routes/t/$slug/sending-domains/index.tsx` — domain list with status badges, add-domain dialog (TanStack Form + `FormField`), per-row re-check action with busy state, `AsyncState` loading/empty/error/populated states, permission gating via `usePermissions`
- [X] T011 [US1] Create `frontend/src/routes/t/$slug/sending-domains/$id.tsx` — domain detail showing status, `DnsRecordRow` list for DKIM/SPF/DMARC, failure reason for failed domains, and a re-check action; reuse `use-sending-domains` polling
- [X] T012 [US1] Run `cd frontend && pnpm test src/routes/t/\$slug/sending-domains && pnpm typecheck && pnpm lint` and confirm green

**Checkpoint**: Sending Domains is fully functional and independently testable — MVP deployable

---

## Phase 4: User Story 2 — Author, send, and monitor a campaign (Priority: P1)

**Goal**: Operators create/edit a campaign (optionally from a template), select a verified domain, target lists/segments, start it with confirmation, watch live send progress, and pause/resume/cancel it.

**Independent Test**: Create a campaign, set content, pick a verified domain, target a list, confirm start is blocked without a verified domain, start it and watch sent/failed/remaining advance without reloading, then pause/resume/cancel.

### Backend slice — cancel campaign (prerequisite for FR-019/FR-025)

- [X] T013 [US2] Add a `CancelCampaign{ TenantID, CampaignID }` command and handler in `internal/campaign/app/command/campaigns.go`, mirroring `PauseCampaign` and calling the existing `Campaign.Cancel()` domain method
- [X] T014 [US2] Wire the `CancelCampaign` command into the campaign `Application` `Commands` struct and its constructor in `internal/campaign/app/`
- [X] T015 [US2] Add `handleCancelCampaign` to `internal/api/campaign_handlers.go` (gated by `PermCampaignsManage`, returns `200 {"status":"cancelled"}`) and register `POST /campaigns/{id}/cancel` in `internal/api/server.go`
- [X] T016 [P] [US2] Add a handler test for `handleCancelCampaign` to `internal/api/campaign_handlers_test.go` and a command test for `CancelCampaign` in `internal/campaign/app/command/campaigns_test.go`

### Tests for User Story 2

- [X] T017 [P] [US2] Route test `frontend/src/routes/t/$slug/campaigns/index.test.tsx` — list, status badges, create flow, permission gating
- [X] T018 [P] [US2] Route test `frontend/src/routes/t/$slug/campaigns/$id.test.tsx` — start blocked without verified domain, start confirmation, progress display, pause/resume/cancel action visibility per status, auto-paused reason

### Implementation for User Story 2

- [X] T019 [P] [US2] Create `frontend/src/hooks/use-campaign.ts` — query hook that polls (~3 s `refetchInterval`) while campaign status is `running`, exposing derived `remaining` and `autoPaused` (per research Decision 5)
- [X] T020 [US2] Create `frontend/src/routes/t/$slug/campaigns/index.tsx` — campaign list with lifecycle status badges, create-campaign dialog, `AsyncState` states, permission gating
- [X] T021 [US2] Create `frontend/src/routes/t/$slug/campaigns/$id.tsx` — campaign editor: subject/body fields, optional template pre-fill (campaign-kind templates only), verified-domain selector (verified domains only via `listSendingDomains`), recipient targeting via lists + `SegmentBuilder`, start with `ConfirmDialog` (disabled without a verified domain, FR-016), live progress (sent/failed/remaining) via `use-campaign`, and pause/resume/cancel actions matching current status with `ConfirmDialog` for cancel
- [X] T022 [US2] Run `make test` (campaign handler/command tests) and `cd frontend && pnpm test src/routes/t/\$slug/campaigns && pnpm typecheck && pnpm lint` — confirm green

**Checkpoint**: Campaigns and Sending Domains both work independently

---

## Phase 5: User Story 3 — Manage reusable templates (Priority: P2)

**Goal**: Operators create, edit, and delete templates (campaign or transactional kind); campaign templates feed the campaign editor.

**Independent Test**: Create a campaign template and a transactional template, edit one, delete one (with confirmation), and confirm the campaign template appears as a starting point in the campaign editor.

### Backend slice — delete template (prerequisite for FR-009/FR-025)

- [X] T023 [US3] Add a `DeleteTemplate{ TenantID, TemplateID }` command and handler in `internal/campaign/app/command/templates.go`
- [X] T024 [US3] Wire the `DeleteTemplate` command into the campaign `Application` `Commands` struct and its constructor in `internal/campaign/app/`
- [X] T025 [US3] Add `handleDeleteTemplate` to `internal/api/campaign_handlers.go` (gated by `PermCampaignsManage`, returns `204`) and register `DELETE /templates/{id}` in `internal/api/server.go`
- [X] T026 [P] [US3] Add a handler test for `handleDeleteTemplate` to `internal/api/campaign_handlers_test.go` and a command test for `DeleteTemplate` in `internal/campaign/app/command/templates_test.go`

### Tests for User Story 3

- [X] T027 [P] [US3] Route test `frontend/src/routes/t/$slug/templates/index.test.tsx` — empty state, create flow, delete confirmation, permission gating
- [X] T028 [P] [US3] Route test `frontend/src/routes/t/$slug/templates/$id.test.tsx` — edit form persists name/subject/content

### Implementation for User Story 3

- [X] T029 [US3] Create `frontend/src/routes/t/$slug/templates/index.tsx` — template list with kind badge, create-template dialog (name, subject, body, kind selector), delete via `ConfirmDialog`, `AsyncState` states, permission gating
- [X] T030 [US3] Create `frontend/src/routes/t/$slug/templates/$id.tsx` — edit form for name/subject/body_html/body_text (kind is read-only after creation)
- [X] T031 [US3] Run `make test` (template handler/command tests) and `cd frontend && pnpm test src/routes/t/\$slug/templates && pnpm typecheck && pnpm lint` — confirm green

**Checkpoint**: Sending Domains, Campaigns, and Templates all work independently

---

## Phase 6: User Story 4 — Set up transactional sending via API key (Priority: P3)

**Goal**: Administrators issue/revoke a `transactional:send`-scoped API key and see the transactional endpoint reference; the area warns when no verified domain exists.

**Independent Test**: Issue an API key with the transactional scope, confirm the secret shows once, revoke it, and confirm the endpoint/payload reference and the no-verified-domain notice display.

### Tests for User Story 4

- [X] T032 [P] [US4] Route test `frontend/src/routes/t/$slug/transactional/index.test.tsx` — show-once secret, revoke confirmation, endpoint reference, no-verified-domain notice, permission gating

### Implementation for User Story 4

- [X] T033 [US4] Create `frontend/src/routes/t/$slug/transactional/index.tsx` — reuse the API-key issue/show-once/revoke panel pattern from `routes/t/$slug/security/index.tsx` (pre-selecting `transactional:send`); static reference block for the `tx` endpoint path, transactional-template requirement, and payload shape from contracts/tenant-api.md; a notice when `listSendingDomains` returns no verified domain (FR-024)
- [X] T034 [US4] Run `cd frontend && pnpm test src/routes/t/\$slug/transactional && pnpm typecheck && pnpm lint` — confirm green

**Checkpoint**: All four user stories independently functional

---

## Phase 7: Polish & Cross-Cutting Concerns

**Purpose**: Verify the whole feature and cross-cutting requirements (FR-025–FR-029, SC-006–SC-010)

- [X] T035 Audit all new screens for the cross-cutting requirements: every async view has loading/empty/error/populated states (FR-027), all destructive actions confirm (FR-025), forms show inline validation + busy state + readable errors (FR-026), 403 shows an authorization message (FR-028), 401 routes to sign-in (FR-029)
- [X] T036 [P] Run the full verification bundle: `make test`, `cd frontend && pnpm test && pnpm typecheck && pnpm lint` — confirm all green
- [X] T037 Execute the quickstart.md verification steps (SC-001 through SC-010) against a running stack

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies
- **Foundational (Phase 2)**: Depends on Setup — BLOCKS all user stories
- **User Stories (Phase 3–6)**: All depend on Foundational; otherwise independent and may run in parallel
- **Polish (Phase 7)**: Depends on all user stories being complete

### User Story Dependencies

- **US1 (P1)**: After Foundational — no backend slice needed (endpoints exist)
- **US2 (P1)**: After Foundational — includes its own backend slice (cancel campaign, T013–T016)
- **US3 (P2)**: After Foundational — includes its own backend slice (delete template, T023–T026)
- **US4 (P3)**: After Foundational — reuses the existing API-key UI pattern

No user story depends on another; each is independently testable.

### Within Each User Story

- Backend slice (commands → wiring → handler/route → tests) before frontend
- Hooks/components before routes that consume them
- Route tests are written alongside and must pass before the story checkpoint

### Parallel Opportunities

- Foundational T002–T005 are all different files → run in parallel
- Within a story, tasks marked [P] (different files, distinct test files, hooks vs components) run in parallel
- After Foundational, US1/US2/US3/US4 can be staffed in parallel by different developers

---

## Parallel Example: Foundational Phase

```bash
# All four foundational tasks touch different files:
Task: "T002 Extend api-types.ts with Phase 3 permissions and view types"
Task: "T003 Extend api.ts with Phase 3 client methods"
Task: "T004 Extend query.ts with Phase 3 query keys"
Task: "T005 Add four nav entries to sidebar.tsx"
```

## Parallel Example: User Story 2

```bash
# Tests and the polling hook are independent files:
Task: "T016 Backend cancel-campaign handler + command tests"
Task: "T017 campaigns/index.test.tsx"
Task: "T018 campaigns/$id.test.tsx"
Task: "T019 use-campaign.ts polling hook"
```

---

## Implementation Strategy

### MVP First (User Story 1 only)

1. Complete Phase 1: Setup
2. Complete Phase 2: Foundational (CRITICAL — blocks all stories)
3. Complete Phase 3: User Story 1 — Sending Domains
4. **STOP and VALIDATE**: every campaign/transactional send is gated on a verified domain, so this is the meaningful first slice
5. Deploy/demo

### Incremental Delivery

1. Setup + Foundational → foundation ready
2. US1 Sending Domains → test → deploy (MVP)
3. US2 Campaigns (+ cancel backend slice) → test → deploy
4. US3 Templates (+ delete backend slice) → test → deploy
5. US4 Transactional → test → deploy

### Parallel Team Strategy

After Foundational: Developer A → US1, Developer B → US2, Developer C → US3, Developer D → US4. Stories integrate independently; the two small backend slices live inside US2 and US3.

---

## Notes

- [P] = different files, no dependency on an incomplete task
- The two backend endpoints (cancel campaign, delete template) are the only backend changes; no schema migration
- The frontend `Permission` union must be extended (T002) before any nav/action gating compiles
- Commit after each task or logical group
- Stop at any checkpoint to validate a story independently
