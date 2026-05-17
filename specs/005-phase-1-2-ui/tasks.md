---
description: "Task list for Phases 1 & 2 — Frontend UI"
---

# Tasks: Phases 1 & 2 — Frontend UI

**Input**: Design documents from `/specs/005-phase-1-2-ui/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/api-client.md, quickstart.md

**Tests**: Included. Constitution Principle II (Test-Backed Delivery) is
non-negotiable; research.md Decision 11 defines Vitest + Testing Library
component/behavior tests at the typed-client boundary. Each user story carries
tests for its critical paths.

**Organization**: Tasks are grouped by user story. All paths are under
`frontend/` (the existing TanStack Start app); paths below omit the `frontend/`
prefix only where the file is obviously frontend-scoped.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[Story]**: US1–US5, maps to spec.md user stories
- Exact file paths are included in each description

## Path Conventions

Frontend app root is `frontend/`. Source under `frontend/src/`. Tests live
beside the unit under test as `*.test.tsx` (matching the existing
`src/components/ui/button.test.tsx`).

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Bring in the missing dependency and component set the design
system needs.

- [X] T001 Add `@tanstack/react-query` to `frontend/package.json` and run `pnpm install` (research.md Decision 1)
- [X] T002 [P] Generate the shadcn primitives into `frontend/src/components/ui/` using the shadcn MCP/CLI against the existing `frontend/components.json`: `sidebar input label card dialog alert-dialog table badge dropdown-menu select tabs sonner skeleton separator avatar tooltip textarea checkbox` (research.md Decision 6; `button` already exists)
- [X] T003 [P] Create the feature directory structure: `frontend/src/components/shell/`, `frontend/src/components/common/`, `frontend/src/hooks/`, `frontend/src/routes/t.$slug/` (plan.md Project Structure)

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Transport layer, server-state plumbing, error routing, permission
derivation, and shared async/confirm components — every user story depends on
these.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete.

- [X] T004 [P] Create `frontend/src/lib/api-types.ts` with all request/response/view types from data-model.md, recording per-endpoint casing (snake_case platform/tenant; PascalCase audience/IAM) and the `Permission` enum, `Node`/`FieldCondition`/`AttrCondition`/`MemberCondition` segment types, and the workspace session state union
- [X] T005 [P] Create `frontend/src/lib/errors.ts`: `ApiError { status, slug, message }` plus a normalizer that parses the `{error, message}` envelope, and status-category helpers (401/403/404/409/422/500 per `internal/api/errmap.go`)
- [X] T006 Expand `frontend/src/lib/api.ts` into the full typed client per `contracts/api-client.md` — every platform and tenant-plane method, tenant methods taking `slug` as first argument, a multipart-aware path for `startImport`, all resolving to `ApiResult<T>` and raising `ApiError` (depends on T004, T005)
- [X] T007 Create `frontend/src/lib/query.ts`: the `QueryClient`, a query/mutation key factory keyed by `slug` + resource, and a global `onError` that delegates to the error router (depends on T005)
- [X] T008 Wrap the app in `QueryClientProvider` in `frontend/src/routes/__root.tsx` and add the `<Toaster />` (sonner) for global feedback (depends on T007)
- [X] T009 [P] Create `frontend/src/lib/permissions.ts`: derive effective workspace-level permissions by joining the signed-in user's `Member.role` to the role catalogue, plus `can(permission)` gating helpers (research.md Decision 3; depends on T004)
- [X] T010 Create the session/auth error router used by `query.ts`: 401 platform → `/login`, 401/session-pending tenant → re-open session/TOTP, 403 → authorization message, 404 slug → not-found screen — in `frontend/src/lib/errors.ts` (research.md Decision 4; depends on T005, T006)
- [X] T011 [P] Create `frontend/src/components/common/async-state.tsx`: a wrapper rendering distinct loading (skeleton), empty, error, and populated states for any query (FR-034)
- [X] T012 [P] Create `frontend/src/components/common/confirm-dialog.tsx`: an alert-dialog-based confirmation gate for destructive actions (FR-031)
- [X] T013 [P] Create `frontend/src/components/common/data-table.tsx`: a paged table consuming `{ items, total }` + limit/offset, used by list/subscriber/audit views (FR-016)
- [X] T014 [P] Create `frontend/src/components/common/form-field.tsx`: native controlled field wrapper with inline error + busy state, plus a small validation helper (research.md Decision 7; FR-032)
- [X] T015 Create `frontend/src/hooks/use-session.ts`, `frontend/src/hooks/use-workspace.ts`, and `frontend/src/hooks/use-permissions.ts` exposing the current platform account, workspace, and derived permissions to screens (depends on T006, T009)
- [X] T016 [P] Contract test for the typed client in `frontend/src/lib/api.test.ts`: verb/path correctness, slug interpolation, multipart `startImport` fields, and `ApiError` normalization (contracts/api-client.md)

**Checkpoint**: Foundation ready — user story implementation can now begin.

---

## Phase 3: User Story 1 - Onboard onto the platform and open a workspace (Priority: P1) 🎯 MVP

**Goal**: A redesigned onboarding flow (register, sign in, create/pick a
workspace) and the persistent sidebar app shell that hosts every later story.

**Independent Test**: On a clean environment, register an account, sign in,
create a workspace, and arrive inside the sidebar shell. Sign out and back in,
and confirm the workspace appears and can be re-entered. Open a slug you are
not a member of → a clear "not found / no access" screen.

### Tests for User Story 1

- [X] T017 [P] [US1] Behavior test for register/sign-in/sign-out flow incl. duplicate-email and invalid-credentials errors in `frontend/src/routes/signup.test.tsx` and `frontend/src/routes/login.test.tsx`
- [X] T018 [P] [US1] Behavior test for the workspace shell — active-section indicator, workspace name, and the not-found/no-access screen — in `frontend/src/routes/t.$slug/route.test.tsx`

### Implementation for User Story 1

- [X] T019 [P] [US1] Build `frontend/src/components/shell/sidebar.tsx`: navigation to Subscribers, Lists, People & Access, Import/Export, Audit, Settings with an active-section indicator (FR-007, FR-008)
- [X] T020 [P] [US1] Build `frontend/src/components/shell/top-bar.tsx`: current workspace name + account/sign-out control (FR-007)
- [X] T021 [US1] Compose `frontend/src/components/shell/app-shell.tsx` from sidebar + top bar, hiding/disabling nav entries the user's permissions disallow (FR-009; depends on T015, T019, T020)
- [X] T022 [US1] Create the workspace layout route `frontend/src/routes/t.$slug/route.tsx`: open the workspace session in `beforeLoad`/loader, render the not-found/no-access screen on 404/forbidden, render the app-shell on `active` (FR-006, FR-007; research.md Decision 5; depends on T015, T021)
- [X] T023 [US1] Create `frontend/src/routes/t.$slug/index.tsx`: workspace overview landing screen inside the shell
- [X] T024 [US1] Redesign `frontend/src/routes/signup.tsx` with the design system: validation, duplicate-email error, busy state, guide to first-workspace creation (FR-001, FR-032)
- [X] T025 [US1] Redesign `frontend/src/routes/login.tsx`: non-specific invalid-credentials error, busy state, sign-out wiring (FR-002, FR-032)
- [X] T026 [US1] Redesign `frontend/src/routes/index.tsx` as the workspace picker: list memberships, enter a workspace, sign out (FR-004)
- [X] T027 [US1] Redesign `frontend/src/routes/tenants.new.tsx`: create a workspace with name + unique slug, surfacing slug-conflict (409) clearly (FR-003, FR-032)
- [X] T028 [US1] Remove the legacy `frontend/src/routes/t.$slug.tsx` flat route now superseded by the `t.$slug/` directory layout

**Checkpoint**: US1 fully functional — onboarding and the shell work end to end.

---

## Phase 4: User Story 2 - Manage lists and subscribers (Priority: P1)

**Goal**: Full lists and subscribers management inside the workspace —
create/edit/delete, custom attributes, list membership, subscription state,
search and segment queries.

**Independent Test**: Create a list, create subscribers with custom
attributes, attach them, edit a subscriber, change subscription state, run a
segment query, delete a subscriber and a list — every change reflected in the
relevant views.

### Tests for User Story 2

- [X] T029 [P] [US2] Behavior test for list create/edit/delete with confirmation in `frontend/src/routes/t.$slug/lists.test.tsx`
- [X] T030 [P] [US2] Behavior test for subscriber create (incl. duplicate-email 409), edit, membership change, and delete in `frontend/src/routes/t.$slug/subscribers.test.tsx`
- [X] T031 [P] [US2] Test for the JSON attribute editor and segment builder in `frontend/src/components/common/json-attribute-editor.test.tsx` and `frontend/src/components/common/segment-builder.test.tsx`

### Implementation for User Story 2

- [X] T032 [P] [US2] Build `frontend/src/components/common/json-attribute-editor.tsx`: textarea-based, `JSON.parse` validation, inline error, blocks save on invalid structure (FR-013; research.md Decision 8)
- [X] T033 [P] [US2] Build `frontend/src/components/common/segment-builder.tsx`: recursive group/leaf builder emitting the PascalCase `Node` tree, fields `email/name/state`, ops `eq/neq/exists/contains/gt/lt/gte/lte` (FR-015; research.md Decision 9)
- [X] T034 [US2] Create `frontend/src/routes/t.$slug/lists.index.tsx`: paged list view with loading/empty/error states and a create-list dialog (FR-010, FR-016, FR-034)
- [X] T035 [US2] Create `frontend/src/routes/t.$slug/lists.$id.tsx`: list detail showing its subscribers, each subscription state, total count; edit and confirm-delete (FR-010, FR-011, FR-031)
- [X] T036 [US2] Create `frontend/src/routes/t.$slug/subscribers.index.tsx`: paged subscriber view, email/name search box, segment-query panel showing matches + total count (FR-015, FR-016)
- [X] T037 [US2] Create the create-subscriber dialog/form within the subscribers view: email + optional name + custom attributes + target lists, handling duplicate-email 409 (FR-012)
- [X] T038 [US2] Create `frontend/src/routes/t.$slug/subscribers.$id.tsx`: subscriber detail — edit name/attributes/state, add/remove lists, change per-list subscription state, confirm-delete (FR-012, FR-013, FR-014, FR-031)
- [X] T039 [US2] Wire list/subscriber mutations to TanStack Query cache invalidation so every view refreshes after a write (FR-010, FR-012, FR-014)

**Checkpoint**: US1 and US2 both work independently — the core product is demonstrable.

---

## Phase 5: User Story 3 - Invite teammates and manage roles (Priority: P2)

**Goal**: A unified People & Access area — invite by email, manage pending
invitations and members, create roles, assign workspace-level and per-list
roles, with permission-aware gating.

**Independent Test**: Invite an email, accept it as the invitee, create a
limited role, assign it, confirm permitted vs. blocked actions, grant a
per-list role and confirm access widens for that one list.

### Tests for User Story 3

- [X] T040 [P] [US3] Behavior test for invite, revoke, and the members list in `frontend/src/routes/t.$slug/access.test.tsx`
- [X] T041 [P] [US3] Behavior test for role create/edit/delete, assignment, and non-admin gating in `frontend/src/routes/t.$slug/access.roles.test.tsx`
- [X] T042 [P] [US3] Behavior test for the invitation-acceptance screen (valid, invalid, expired) in `frontend/src/routes/invite.$token.test.tsx`

### Implementation for User Story 3

- [X] T043 [US3] Create `frontend/src/routes/t.$slug/access.tsx`: a tabbed People & Access layout (Members, Invitations, Roles) gated so role controls are unavailable to non-admins (FR-009, US3 scenario 7)
- [X] T044 [US3] Build the Members + Invitations panels: list members with roles, invite by email showing the accept link, list pending invitations, revoke an invitation (FR-017, FR-018)
- [X] T045 [US3] Build the Roles panel: create/edit/delete roles selecting permissions from the `Permission` enum, with confirm-delete (FR-019, FR-031)
- [X] T046 [US3] Build role-assignment controls: assign a workspace-level role to a member, grant/remove a per-list role on a specific list (FR-020)
- [X] T047 [US3] Redesign `frontend/src/routes/invite.$token.tsx`: invitation-acceptance screen reachable from an invite link, accept (creating an account if needed), with clear invalid/expired states (FR-005, US3 scenarios 2–3)
- [X] T048 [US3] Surface a clear authorization message on any 403 from a guarded action, leaving data unchanged (FR-021; uses T010)

**Checkpoint**: US1–US3 all work independently — collaboration and access control are in place.

---

## Phase 6: User Story 4 - Import and export subscribers (Priority: P2)

**Goal**: CSV/ZIP import with target-list selection and progress, and export of
all/list/segment selections with a downloadable result.

**Independent Test**: Upload a CSV of mixed new/existing emails to a target
list, see the result summary with created/updated/failed counts, confirm
subscribers appear on the list; then export the list and download the CSV.

### Tests for User Story 4

- [X] T049 [P] [US4] Behavior test for import upload, job polling, and the result summary in `frontend/src/routes/t.$slug/import-export.test.tsx`
- [X] T050 [P] [US4] Test for the job-status polling hook (terminal vs. non-terminal states) in `frontend/src/hooks/use-job-status.test.tsx`

### Implementation for User Story 4

- [X] T051 [P] [US4] Create `frontend/src/hooks/use-job-status.ts`: polls `GET /jobs/{id}` with TanStack Query `refetchInterval` while the status is non-terminal (FR-023, FR-025; research.md Decision 10)
- [X] T052 [US4] Create `frontend/src/routes/t.$slug/import-export.tsx`: the area layout, gated so upload/export controls are unavailable without import/export permission and the restriction is explained (FR-022, US4 scenario 5)
- [X] T053 [US4] Build the import panel: CSV/ZIP file picker, target-list multi-select, multipart upload, live progress, and a result summary (created/updated/skipped/failed, per-row failures) (FR-022, FR-023)
- [X] T054 [US4] Build the export panel: choose selection (all / a list / a segment), start the job, poll progress, and download the CSV via `GET /jobs/{id}/download` on completion (FR-024)
- [X] T055 [US4] Ensure re-opening an in-progress job view re-fetches and displays current status (FR-025, edge case)

**Checkpoint**: US1–US4 all work independently — audience data flows in and out.

---

## Phase 7: User Story 5 - Secure the account and workspace (Priority: P3)

**Goal**: TOTP enrolment and session challenge, scoped API keys, the audit
trail, and workspace settings.

**Independent Test**: Enrol in TOTP and confirm a code; sign out/in and meet
the TOTP challenge; issue an API key (secret shown once), revoke it; view the
audit trail; update workspace settings.

### Tests for User Story 5

- [X] T056 [P] [US5] Behavior test for TOTP enrol/confirm/disable and the session challenge (valid/invalid code) in `frontend/src/routes/t.$slug/security.test.tsx`
- [X] T057 [P] [US5] Behavior test for API key issue (one-time secret) and revoke in `frontend/src/routes/t.$slug/security.apikeys.test.tsx`
- [X] T058 [P] [US5] Behavior test for the audit trail and settings save in `frontend/src/routes/t.$slug/audit.test.tsx` and `frontend/src/routes/t.$slug/settings.test.tsx`

### Implementation for User Story 5

- [X] T059 [US5] Build the TOTP challenge screen and integrate it into `frontend/src/routes/t.$slug/route.tsx`: when session open returns `totp_pending`, prompt for a code, post to `/session/totp`, retry clearly on failure (FR-027; research.md Decision 5)
- [X] T060 [US5] Create `frontend/src/routes/t.$slug/security.tsx`: TOTP enrolment (display secret/QR from the `uri`, confirm a code, show recovery codes) and disable (FR-026)
- [X] T061 [US5] Build the API keys panel within security: issue a scoped key showing the secret exactly once with a non-retrievable warning, list active keys, revoke a key (FR-028)
- [X] T062 [US5] Create `frontend/src/routes/t.$slug/audit.tsx`: paged audit trail listing actor, action, target, and time (FR-029)
- [X] T063 [US5] Create `frontend/src/routes/t.$slug/settings.tsx`: view and update workspace settings generically over the fields the settings endpoint exposes, confirming a successful save (FR-030)

**Checkpoint**: All five user stories are independently functional.

---

## Phase 8: Polish & Cross-Cutting Concerns

**Purpose**: Consistency and verification across all stories.

- [X] T064 [P] Audit every screen for the four async states (loading/empty/error/populated) — no blank screens (FR-034, SC-006)
- [X] T065 [P] Audit every destructive action for a confirmation step (FR-031, SC-007)
- [X] T066 Verify permission gating across every Phase 2 area — unavailable actions are hidden/disabled with an explanation (FR-009, SC-008)
- [X] T067 [P] Design-system consistency pass: confirm no screen retains the previous minimal styling, shared spacing/typography throughout (FR-033, SC-009)
- [X] T068 Verify session-expiry routing mid-task across stories produces a clear sign-in message, not a broken screen (FR-006, SC-010)
- [X] T069 Run the verification bundle — `pnpm typecheck`, `pnpm lint`, `pnpm test` — and the quickstart.md manual smoke tests; fix any failures

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — start immediately.
- **Foundational (Phase 2)**: Depends on Setup — BLOCKS all user stories.
- **User Stories (Phase 3–7)**: All depend on Foundational completion.
  - US1 (P1) should land first — it builds the shell every other story renders inside.
  - US2–US5 depend on the US1 shell layout route (`t.$slug/route.tsx`) existing; given that, they are otherwise independent.
- **Polish (Phase 8)**: Depends on all targeted user stories being complete.

### User Story Dependencies

- **US1 (P1)**: After Foundational. No dependency on other stories. **MVP.**
- **US2 (P1)**: After US1 (needs the shell layout). Independently testable.
- **US3 (P2)**: After US1. Independently testable. Touches permission gating shared with all stories.
- **US4 (P2)**: After US1. Export-by-segment reuses the US2 segment builder; if US2 is not done, an "all/list" export still ships.
- **US5 (P3)**: After US1. The TOTP challenge (T059) edits the US1 route file — sequence after T022/T028.

### Within Each User Story

- Tests are written alongside implementation and must pass before the story checkpoint.
- Shared components before the routes that consume them.
- Story complete before moving to the next priority.

### Parallel Opportunities

- Setup: T002 and T003 run in parallel.
- Foundational: T004, T005 in parallel; then T009, T011, T012, T013, T014, T016 in parallel after their deps.
- Within a story, all `[P]` test tasks run together, and `[P]` component tasks (e.g. T019/T020, T032/T033) run together.
- With multiple developers, US2–US5 can proceed in parallel once US1's shell route exists.

---

## Parallel Example: User Story 1

```bash
# Tests for US1 together:
Task: "Behavior test for register/sign-in/sign-out in src/routes/signup.test.tsx + login.test.tsx"
Task: "Behavior test for the workspace shell in src/routes/t.$slug/route.test.tsx"

# Shell components for US1 together:
Task: "Build src/components/shell/sidebar.tsx"
Task: "Build src/components/shell/top-bar.tsx"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup.
2. Complete Phase 2: Foundational (CRITICAL — blocks all stories).
3. Complete Phase 3: User Story 1.
4. **STOP and VALIDATE**: Run the US1 independent test from quickstart.md.
5. Deploy/demo — onboarding + shell is a shippable slice.

### Incremental Delivery

1. Setup + Foundational → foundation ready.
2. US1 → test → deploy/demo (MVP — onboarding & shell).
3. US2 → test → deploy/demo (lists & subscribers — core product).
4. US3 → test → deploy/demo (people & access).
5. US4 → test → deploy/demo (import/export).
6. US5 → test → deploy/demo (security, audit, settings).
7. Polish pass.

### Parallel Team Strategy

1. The team completes Setup + Foundational together.
2. One developer lands US1 (the shell is a shared prerequisite).
3. Once US1's shell route exists, US2–US5 proceed in parallel across developers.
4. Coordinate on `t.$slug/route.tsx` (US1 T022/T028 and US5 T059 both touch it).

---

## Notes

- `[P]` tasks = different files, no dependencies on incomplete tasks.
- `[Story]` label maps each task to a spec.md user story for traceability.
- Every user story is independently completable and testable.
- The backend API is fixed and stable — no backend files change in this feature.
- Watch the casing split: audience/IAM endpoints return PascalCase JSON (research.md Decision 2).
- Commit after each task or logical group.
- Stop at any checkpoint to validate a story independently.
