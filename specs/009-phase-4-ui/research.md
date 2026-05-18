# Phase 0 Research: Phase 4 — Deliverability & Analytics — Frontend UI

All decisions are resolved from in-repo inspection. No `NEEDS CLARIFICATION`
markers remain.

## Decision 1 — No backend work is required

**Decision**: This feature is frontend-only. All five Phase 4 endpoints already
exist and are wired.

**Rationale**: `internal/api/server.go` (lines 169–178) registers
`GET/POST /suppressions`, `DELETE /suppressions/{email}`,
`GET/PUT /bounce-settings`, `GET /campaigns/{id}/analytics`, and
`GET /dashboard`. The handlers exist in `internal/api/suppression_handlers.go`
and `internal/api/analytics_handlers.go`. Contrast with the Phase 3 UI plan,
which had to add `CancelCampaign` / `DeleteTemplate`; here there is no gap.

**Alternatives considered**: A backend slice — rejected, nothing is missing.

## Decision 2 — Permission gating reuses existing strings

**Decision**: No new `Permission` values. Gate on the strings the backend
handlers already enforce.

**Rationale**: `suppression_handlers.go` uses `PermSendingGet` to list/read and
`PermSendingManage` to add/remove and to update bounce settings.
`analytics_handlers.go` uses `PermCampaignsGet` for both the campaign analytics
and dashboard handlers. The frontend `Permission` union in `api-types.ts`
already contains `sending:get`, `sending:manage`, and `campaigns:get`. So:
- Suppression list nav + view → `sending:get`
- Add/remove suppression, update bounce settings → `sending:manage`
- Dashboard nav + view, campaign analytics → `campaigns:get`

**Alternatives considered**: Adding `analytics:*` / `suppressions:*`
permissions — rejected; the backend does not enforce them, so they would be
advisory-only fiction and `ALL_PERMISSIONS` would drift from the server.

## Decision 3 — Navigation placement

**Decision**: Add two sidebar nav entries — "Dashboard" (`campaigns:get`) and
"Suppression list" (`sending:get`). Campaign analytics is a child route of the
existing campaign detail page, not a nav entry. Bounce-action settings is a
sibling route of the suppression list, reached from a control on that page.

**Rationale**: The existing `sidebar.tsx` `NAV` array is the established
extension point — each entry carries a `requires: Permission[]` filter.
Analytics is inherently per-campaign, so it belongs under
`campaigns/$id` rather than as a top-level destination. Bounce settings is a
small P3 configuration surface naturally adjacent to the suppression list it
governs; giving it its own nav entry would over-weight a two-toggle form.

**Alternatives considered**: (a) Folding bounce settings into the existing
workspace Settings page — rejected; it is a deliverability concern, not a
general workspace setting, and grouping it with suppression keeps the mental
model tight. (b) Replacing the existing `t/$slug/` Overview tiles page with the
dashboard — rejected; the Overview is a navigational launcher, the dashboard is
a metrics surface; conflating them would lose the launcher and complicate
permission gating (Overview is always visible, the dashboard needs
`campaigns:get`).

## Decision 4 — Data fetching and refresh model

**Decision**: Use plain TanStack Query reads with the default 30 s
`staleTime`; do not poll. Display the server `refreshedAt` timestamp on the
analytics view so the operator understands the figures are near-real-time.

**Rationale**: Phase 4 analytics are served from pre-computed summaries
refreshed by a backend schedule (within ~5 minutes per SC-008); there is no
live stream to subscribe to and aggressive polling would only re-fetch
unchanged summaries. The Phase 3 UI polls only genuinely live resources
(pending domains, running campaigns) via dedicated hooks; analytics is not
live, so it follows the standard non-polling query pattern used by lists,
audit, etc. The `refreshedAt` field (nullable) is surfaced directly so the UI
never implies the numbers are live.

**Alternatives considered**: Short-interval polling like `use-campaign.ts` —
rejected; summaries change at most every few minutes, so polling adds load for
no freshness gain. A manual "refresh" button — deferred; React Query's
refetch-on-mount plus the 30 s stale window already covers the spec's "reopen
or refresh the view" acceptance scenario.

## Decision 5 — Suppression list pagination and filtering

**Decision**: Use cursor-based incremental loading (the endpoint returns
`nextCursor`), with the `reason` filter and `email` substring search passed as
query params and folded into the query key so a filter change starts a fresh
cursor sequence.

**Rationale**: `GET /suppressions` supports `cursor`, `limit` (default 50),
`reason`, and `email` params (008 `contracts/http-api.md`). Cursor paging is
already the shape the endpoint exposes, so the UI mirrors it rather than
faking offset paging. Filters belong in the query key (the established pattern
— see `subscribersSearch` in `query.ts`) so cache entries do not collide and
FR-013 ("without losing the active filter") holds naturally.

**Alternatives considered**: Loading the whole list client-side and filtering
in memory — rejected; suppression lists can grow large and the endpoint is
built for server-side filtering.

## Decision 6 — Idempotent add / concurrent-removal handling

**Decision**: Treat `POST /suppressions` as idempotent (it returns `201` with
the entry whether newly created or pre-existing) and treat a `404` from
`DELETE /suppressions/{email}` as success-equivalent — invalidate the list
query and reconcile to server state without surfacing a hard error.

**Rationale**: The 008 contract states the add endpoint is idempotent and
`DELETE` returns `404 suppression_not_found` when the entry is already gone.
FR-014 requires the UI to reconcile silently. The global error handler in
`query.ts` already swallows `403`/`404` from toasts; the mutation's
`onError`/`onSettled` can invalidate `queryKeys.suppressions` so the list
re-syncs.

**Alternatives considered**: Showing an error on the `404` removal race —
rejected by FR-014.

## Decision 7 — Email validation on manual add

**Decision**: Validate the email client-side with the same lightweight check
used elsewhere in the app (a basic RFC-shaped pattern) for an inline message,
and additionally surface the backend `422 validation_failed` if it slips
through.

**Rationale**: FR-011 requires an inline validation message before anything is
added. Client-side validation gives immediate feedback; the backend remains
authoritative and returns `422 validation_failed` for anything the client
check misses, which is mapped to an inline form error rather than a toast.

**Alternatives considered**: Backend-only validation — rejected; it costs a
round-trip and FR-011 wants immediate inline feedback.

## Decision 8 — Rate display and zero denominators

**Decision**: A shared `RateValue` component renders a fraction (0.0–1.0) as a
percentage; the backend already yields `0.0` for a zero denominator, so the
component simply renders `0%` with no special-casing.

**Rationale**: The 008 contract computes rates on read and "a zero denominator
yields `0.0`". FR-005 only requires the UI not to error or blank — rendering
the `0.0` it receives as `0%` satisfies this directly. Centralising the
fraction→percentage formatting in one component keeps the analytics view and
the dashboard consistent.

**Alternatives considered**: Computing rates in the frontend from raw counts —
rejected; the backend owns rate definitions (open/click over `delivered`,
bounce/complaint over `sent`) and recomputing them risks divergence.
