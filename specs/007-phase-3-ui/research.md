# Phase 0 Research: Phase 3 Sending Pipeline — Frontend UI

All technical context unknowns were resolved by inspecting the existing
Phase 1 & 2 frontend (`frontend/`) and the Phase 3 backend (`internal/`).
No external research was required — the stack, conventions, and API surface
are already established in-repo.

## Decision 1 — Reuse the existing frontend stack verbatim

**Decision**: Build the four new areas as TanStack Start file-routes under
`frontend/src/routes/t/$slug/`, using React 19, TanStack Query for all server
state, TanStack Form for forms, shadcn/Radix components, and Tailwind v4 — the
exact stack of the Phase 1 & 2 UI.

**Rationale**: The spec mandates extending, not rebuilding, the existing app
shell and design system (FR-003, SC-009, Assumptions). The repo already has a
typed API client (`lib/api.ts`), a query-key factory (`lib/query.ts`), an
`AsyncState` wrapper for loading/empty/error/populated states, `ConfirmDialog`,
`DataTable`, `FormField`, and a permission-gated `WorkspaceSidebar`. Every
requirement maps onto an existing primitive.

**Alternatives considered**: Introducing a new component or data-fetching
library — rejected; the spec forbids new frameworks and it would fork the
design system.

## Decision 2 — Add a tiny backend slice for cancel + delete

**Decision**: Before the UI work, add two backend operations the API does not
yet expose:
- `POST /t/{slug}/api/campaigns/{id}/cancel` — a `CancelCampaign` command
  wrapping the existing `Campaign.Cancel()` domain method (already implemented
  in `internal/campaign/domain/campaign.go`).
- `DELETE /t/{slug}/api/templates/{id}` — a `DeleteTemplate` command.

Both gated by `campaigns:manage`, following the existing command-handler and
route conventions.

**Rationale**: FR-009 (delete template), FR-019 (cancel campaign), and FR-025
(confirmation for both) require these endpoints. The current router
(`internal/api/server.go`) exposes start/pause/resume but no cancel, and
template create/list/get/update but no delete. The spec's assumption that the
Phase 3 backend is "fully implemented" is incorrect on these two points. The
campaign domain already has a validated `Cancel()` transition, so the command
side is thin. **Confirmed with the user** (2026-05-18): add the backend
endpoints rather than descope.

**Alternatives considered**: Descoping cancel/delete from this UI phase —
rejected by the user. Frontend-only stubs calling non-existent endpoints —
rejected; would ship broken buttons.

## Decision 3 — Extend the frontend `Permission` union with Phase 3 strings

**Decision**: Add `sending:get`, `sending:manage`, `campaigns:get`,
`campaigns:manage`, and `transactional:send` to the `Permission` type and
`ALL_PERMISSIONS` array in `frontend/src/lib/api-types.ts`.

**Rationale**: The backend defines these in `internal/iam/domain/permission.go`,
but the frontend `Permission` union stops at the Phase 1 & 2 set. Nav gating
(`WorkspaceSidebar`), action gating (`usePermissions` / `can` / `canAny`), and
the API-key issuance checkbox grid (`ALL_PERMISSIONS`) all key off these
types. Adding them to `ALL_PERMISSIONS` automatically surfaces
`transactional:send` as a selectable API-key scope (FR-021), satisfying the
spec assumption that the existing key UI is reused with the new scope added.

**Alternatives considered**: A separate Phase 3 permission constant —
rejected; splits one concept across two lists and breaks the existing
`derivePermissions` join.

## Decision 4 — Live updates via TanStack Query `refetchInterval`

**Decision**: Domain verification status and running-campaign send progress
poll on an interval using the same pattern as `useJobStatus`: a query whose
`refetchInterval` returns a fixed interval while non-terminal and `false` once
terminal. Domain list: poll every ~5 s while any domain is `pending`. Campaign
detail: poll every ~3 s while status is `running`.

**Rationale**: FR-007 and FR-018 require status/progress to update without a
manual reload; the spec's Assumptions explicitly allow interval re-fetching and
mandate no specific real-time transport. `useJobStatus` already proves this
pattern in the codebase. Re-opening a view re-runs the query, satisfying the
"navigate away and return" edge case.

**Alternatives considered**: SSE or WebSocket transport — rejected; no backend
support exists and the spec does not require it (YAGNI, Principle III).

## Decision 5 — Derive the auto-pause reason from counts

**Decision**: When a campaign's status is `paused`, the UI labels it
"auto-paused after send errors" when `failed_count >= max_send_errors`;
otherwise it presents it as an operator-initiated pause.

**Rationale**: FR-020 requires surfacing an auto-paused campaign "with its
reason". `CampaignView` (`internal/campaign/app/query/campaigns.go`) carries
`status`, `failed_count`, and `max_send_errors` but no explicit pause-reason
field. The error-threshold breach is the only auto-pause trigger, so the reason
is fully derivable client-side without a backend change.

**Alternatives considered**: Adding a `pause_reason` field to `CampaignView` —
rejected as unnecessary scope; the counts already carry the signal.

## Decision 6 — Recipient targeting reuses lists API + segment builder

**Decision**: The campaign editor targets recipients by selecting one or more
workspace lists (via the existing `api.listLists`) and/or by composing segments
with the existing `SegmentBuilder` common component. Selected segments are sent
as the `segments` array in the campaign create/update payload; lists as
`list_ids`.

**Rationale**: FR-015 requires targeting "one or more lists or segments". The
campaign create/update handlers already accept `list_ids` and `segments`
(`internal/api/campaign_handlers.go`). `SegmentBuilder` and the lists query
already exist, so no new targeting primitive is needed.

**Alternatives considered**: A bespoke recipient picker — rejected; duplicates
`SegmentBuilder`.

## Decision 7 — Transactional area is mostly static reference content

**Decision**: The Transactional Sending route renders: (a) the API-key panel
reused from the Security screen filtered to / pre-selecting the
`transactional:send` scope, and (b) static reference content — the `tx`
endpoint path (`/t/{slug}/api/tx`), the requirement to reference a
transactional template, and the JSON payload shape (`template_id`, `to`,
`sending_domain_id`, `from_name`, `from_local_part`, `variables`). It also
checks whether the workspace has a verified sending domain and shows a notice
when none exists.

**Rationale**: FR-021–FR-024 scope the transactional UI to credential issuance
plus developer reference. The payload shape is fixed by `handleTransactionalSend`
(`internal/api/tx_handlers.go`); no live call is made from the UI.

**Alternatives considered**: An interactive "send test transactional message"
form — rejected; out of scope for this phase and not in the spec.
