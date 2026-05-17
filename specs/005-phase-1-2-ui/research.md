# Phase 0 Research: Phases 1 & 2 — Frontend UI

All NEEDS CLARIFICATION items from the Technical Context are resolved below.
Decisions are scoped to the existing `frontend/` TanStack Start application.

## Decision 1 — Server-state management: TanStack Query

**Decision**: Add `@tanstack/react-query` and route all API reads/writes through
query and mutation hooks. Wrap the app in a `QueryClientProvider` in
`routes/__root.tsx`.

**Rationale**: FR-034 requires every async view to show distinct loading,
empty, error, and populated states, and the spec has ~25–30 such views. The
current screens hand-roll this with `useState`/`useEffect` (see
`routes/t.$slug.tsx`), which does not scale and gives no cache invalidation
after mutations (FR-010/FR-012/FR-014 all need a view to refresh after a
write). TanStack Query gives loading/error/empty states, automatic refetch and
invalidation, and request de-duplication for free. The SSR-integration package
`@tanstack/react-router-ssr-query` is **already** a project dependency, so the
query library is the intended-but-missing peer, not a new framework.

**Alternatives considered**:
- *Keep raw `useEffect` fetching* — rejected: no cache, manual invalidation at
  every call site, easy to forget a state branch (violates FR-034 at scale).
- *SWR* — rejected: TanStack Query pairs with the already-installed TanStack
  Router SSR-query adapter and the team's existing TanStack stack.

## Decision 2 — API client layer and the PascalCase/snake_case split

**Decision**: Keep a single typed transport module (`src/lib/api.ts` +
`src/lib/api-types.ts`). It is the only place `fetch` is called. The platform
API and tenant-settings/invitation endpoints return **snake_case** JSON
(structs carry `json:` tags); the audience and IAM endpoints return
**PascalCase** JSON (those view structs have **no** `json:` tags, so Go
serializes exported field names verbatim — e.g. `ListView` → `{"ID","Name",
"CreatedAt",...}`). The client declares response types matching exactly what
each endpoint emits — it does **not** rename fields — and per-domain view types
record the casing so screens consume them directly.

**Rationale**: This casing inconsistency is real and observable in the backend
(`internal/tenant/app/query/views.go` has tags; `internal/audience/app/query/
views.go` and `internal/iam/app/query/*.go` do not). The spec's Assumptions say
the backend is stable and this feature is UI-only, so the UI must conform to
the API as-shipped rather than request a backend change. Recording the casing
in the typed view layer keeps the surprise in one documented place
(see `contracts/api-client.md`) instead of leaking into every component.

**Alternatives considered**:
- *Backend change to add `json:` tags* — rejected: out of scope; spec fixes the
  backend as stable.
- *A runtime case-normalizing wrapper* — rejected: hides which shape an
  endpoint truly returns and adds an untyped transform; explicit per-endpoint
  types are clearer and type-safe. (Flagged as a follow-up the backend team may
  choose to fix; not required for this feature.)

## Decision 3 — Permission-aware UI gating

**Decision**: Derive the current user's **workspace-level** effective
permissions client-side and use them to hide/disable navigation and actions
(FR-009). Derivation: `GET /t/{slug}/api/tenant` returns the member list with
each member's `role` name; the signed-in user's id comes from
`GET /api/platform/me`; `GET /t/{slug}/api/roles` returns every role with its
`Permissions` array. Join *my role name → role.Permissions* to get my
permission set. Every tenant has a bootstrap **"Owner"** role carrying
`AllPermissions` (`internal/iam/app/command/sessions.go`), and it appears in
the `/roles` listing, so the Owner case resolves too. **Per-list** roles cannot
be enumerated from any existing endpoint, so per-list gating is **reactive**:
attempt the action and handle a `403` (Decision 4).

The backend remains authoritative — every guarded route re-checks permissions
(`requirePermission`/`requireListPermission`). UI gating is advisory: it
prevents presenting actions the user provably cannot perform, and a `403`
always produces a clear authorization message.

**Rationale**: FR-009 and SC-008 require proactively hiding/disabling
unavailable actions, so reactive-only handling is insufficient for
workspace-level nav. No endpoint returns the resolved principal's permission
set directly, but the role-join above reconstructs it from data that *is*
exposed. Per-list grants are a narrower surface; reactive handling there keeps
scope contained without a backend change.

**Alternatives considered**:
- *Reactive-only (gate nothing, react to 403)* — rejected: violates FR-009 for
  the persistent sidebar nav.
- *Add a `GET /t/{slug}/api/me` effective-permissions endpoint* — rejected for
  this iteration: backend is fixed as stable by the spec. Recorded as a
  recommended follow-up that would also make per-list gating proactive.

## Decision 4 — Auth/session error routing (single error-mapping point)

**Decision**: `src/lib/errors.ts` normalizes the backend error envelope
`{"error": "<slug>", "message": "<text>"}` into a typed `ApiError { status,
slug, message }`. A TanStack Query global `onError` (and the `fetch` wrapper)
routes by status exactly once: `401` on the platform plane → redirect to
`/login`; `401`/`session`-pending on the tenant plane → re-open the workspace
session or show the TOTP challenge; `403` → render a clear authorization
message and leave data unchanged (FR-021); `404` on a workspace slug → the
"not found / no access" screen (FR-006, edge cases). Components branch on the
normalized `slug`/`kind`, never on raw status codes or message strings
(Constitution Principle VI — errors mapped once).

**Rationale**: FR-006/FR-021/SC-010 and several edge cases need consistent
handling of expiry, denial, and missing workspaces. Concentrating the mapping
mirrors the backend's own single mapping point (`internal/api/errmap.go`) and
keeps transport concerns out of screens.

**Alternatives considered**:
- *Per-screen error handling* — rejected: drift-prone, and the constitution
  requires error→treatment mapping in exactly one place.

## Decision 5 — Workspace shell, routing, and the session/TOTP gate

**Decision**: Use TanStack Router file-based routing. Convert the single
`routes/t.$slug.tsx` into a `routes/t.$slug/` directory: `route.tsx` is the
**layout route** rendering the persistent sidebar app shell (nav to
Subscribers, Lists, People & Access, Import/Export, Audit, Settings, plus
workspace name and account/sign-out), and nested files are the section
screens. The layout's `beforeLoad`/loader opens the workspace session
(`POST /t/{slug}/api/session`); if the result `state` is `totp_pending` it
renders the TOTP challenge (`POST /t/{slug}/api/session/totp`) before
revealing the shell; if the tenant fetch is `404`/forbidden it renders the
"not found / no access" screen.

**Rationale**: FR-007/FR-008 require a persistent sidebar shell with an active-
section indicator; a layout route is the idiomatic TanStack way to share that
chrome across every workspace screen. The session must be opened before any
guarded `/t/{slug}/api/*` call succeeds, and `OpenWorkspaceSession` returns a
`state` of active vs. totp-pending — the layout is the natural single gate for
both (FR-027).

**Alternatives considered**:
- *Repeat the shell in every screen* — rejected: violates "shared infrastructure
  lives once" and FR-033's consistency requirement.

## Decision 6 — Design system: shadcn component set

**Decision**: Generate the shadcn primitives the screens need into
`components/ui/` using the shadcn MCP/CLI with the project's existing
`components.json` (style `radix-nova`, base color `mist`, lucide icons).
Expected set: `sidebar`, `input`, `label`, `card`, `dialog`, `alert-dialog`,
`table`, `badge`, `dropdown-menu`, `select`, `tabs`, `sonner` (toasts),
`skeleton`, `separator`, `avatar`, `tooltip`, `textarea`, `checkbox`. `button`
already exists and is reused. Feature-level composites (async-state wrapper,
confirm-dialog, JSON attribute editor, segment builder, data-table) live in
`components/common/`.

**Rationale**: FR-033/SC-009 require one shared design system replacing the
minimal styling. shadcn is already configured (`components.json`, `shadcn` dev
dependency, `@import "shadcn/tailwind.css"` in `styles.css`) — using it is the
"reuse the existing component library" path the Assumptions mandate.

**Alternatives considered**:
- *Hand-build primitives* — rejected: shadcn is already wired; rebuilding is
  wasted effort.
- *A different component library (MUI, Mantine)* — rejected: introduces a new
  framework, contradicting the Assumptions.

## Decision 7 — Forms and validation

**Decision** (revised 2026-05-17): Use **TanStack Form** (`@tanstack/react-form`)
for all forms. `src/components/common/form-field.tsx` keeps a presentational
`FormField` (label + control + inline error) plus reusable `rules` validators;
forms bind them through `form.Field` render props. Each form shows inline field
errors, a busy/disabled submit state during a pending mutation, and a readable
server error on failure (FR-032). Submission state comes from the TanStack
Query mutation's `isPending`.

**Rationale**: TanStack Form pairs with the team's existing TanStack stack
(Router, Query), gives type-safe field state, blur/submit/async validation, and
field listeners (used for slug derivation on the create-workspace form). The
original "no form library" choice below was reconsidered and overridden by the
user; the validation rules stay small and shared via `rules`.

**Superseded alternative**: *Native controlled components with a per-form
validation helper, no form library* — originally chosen for YAGNI, then
revised. *React Hook Form* — not chosen; TanStack Form matches the existing
stack.

## Decision 12 — Table rendering: TanStack Table

**Decision**: Build `src/components/common/data-table.tsx` on **TanStack Table**
(`@tanstack/react-table`, headless). It consumes `ColumnDef<T>` definitions and
server `{ items, total }` with `manualPagination`; the list, subscriber, and
audit views render through it.

**Rationale**: TanStack Table is headless (no imposed markup — it renders into
the shadcn `Table` primitives), matches the existing TanStack stack, and leaves
room for sorting/selection without reworking call sites. Server-side
limit/offset paging (FR-016) is preserved via `manualPagination`.

## Decision 8 — Custom-attribute JSON editor

**Decision**: A `<textarea>`-based editor in `components/common/` that holds
the attribute object as formatted JSON text, runs `JSON.parse` on change/blur,
shows a parse error inline, and blocks save until the structure is valid
(FR-013). On load it pretty-prints the existing `Attributes` object.

**Rationale**: Subscriber `Attributes` is an arbitrary `map[string]any`
(`internal/audience/app/query/views.go`). A raw-but-validated JSON editor is
the simplest correct surface and handles the "large or deeply nested JSON" edge
case without a bespoke key/value UI.

**Alternatives considered**:
- *Structured key/value rows* — rejected: cannot represent nested/array values
  the backend allows; deferred as a possible later enhancement.
- *A full JSON-schema form* — rejected: no schema exists for free-form
  attributes.

## Decision 9 — Segment / query builder

**Decision**: A recursive builder in `components/common/` that produces the
backend `Node` tree: a group node (`conj` = `and`/`or` + `children`) or a leaf
(`field`, `attr`, or `member` condition). Field conditions are limited to
`email`/`name`/`state`; operators are `eq, neq, exists, contains, gt, lt, gte,
lte`. The builder posts the tree to `POST /subscribers/query` (results) and
`POST /subscribers/query/count` (total) for FR-015. Simple email/name text
search uses `GET /subscribers?q=` instead.

**Rationale**: The segment shape is fixed by `internal/audience/domain/
segment.go` (`Node`, `FieldCondition`, `AttrCondition`, `MemberCondition`,
`Conjunction`, `SegmentOp`). The UI must emit exactly that JSON; the backend
validates and rejects bad trees, so the builder mirrors the allowed fields/
operators to keep users on the valid path. Note the segment JSON casing: the
domain `Node` struct has no `json:` tags, so the backend decodes PascalCase
keys (`Conj`, `Children`, `Field`, `Attr`, `Member`) — the builder must emit
that casing.

**Alternatives considered**:
- *Free-text query language* — rejected: no parser exists backend-side; the API
  accepts only the structured tree.

## Decision 10 — Import upload and export download

**Decision**: Import posts `multipart/form-data` to `POST /t/{slug}/api/import`
with the file under `file` and repeated `list_ids` form fields; the API client
gains a multipart-aware path (the current JSON-only `request` helper cannot do
this). Export posts JSON to `POST /export` and yields a `job_id`. Both job
kinds are tracked by polling `GET /jobs/{id}` with TanStack Query
`refetchInterval` while `status` is non-terminal, surfacing
created/updated/failed counts on completion (FR-023). A completed export is
downloaded by navigating the browser to `GET /jobs/{id}/download` (FR-024). Re-
opening an in-progress job view simply re-runs the status query (FR-025).

**Rationale**: `handleStartImport` reads a 32 MiB-bounded multipart upload with
`file` + `list_ids` fields; `handleStartExport` takes a JSON `{selection,
list_id, segment}` body; job status is a pollable read model
(`JobStatusView`). Polling with TanStack Query is the resumable, restart-safe
way to reflect server-side job progress without the UI holding job state.

**Alternatives considered**:
- *WebSocket/SSE job updates* — rejected: no such endpoint exists; polling a
  durable server-side job is sufficient and matches the API.

## Decision 11 — Testing approach

**Decision**: Vitest + Testing Library component/behavior tests, mocking the
API client module (not `fetch`) so tests assert UI behavior against typed
responses. Each user story carries tests for its critical paths: auth/session
routing and the TOTP gate, permission-driven hide/disable, destructive-action
confirmation, and the loading/empty/error/populated state matrix. `pnpm
typecheck` and `pnpm lint` are static gates. There is one existing example
test (`components/ui/button.test.tsx`).

**Rationale**: Constitution Principle II requires automated coverage of
critical paths before "done"; the spec's SC-006/SC-007/SC-008/SC-010 are
directly testable as component behaviors. Mocking at the typed-client boundary
keeps tests fast and free of a running backend while still exercising real
component logic.

**Alternatives considered**:
- *End-to-end (Playwright) against a live backend* — rejected for this
  iteration: heavier infra; component tests at the client boundary cover the
  spec's success criteria. E2E can be added later if regression risk warrants.
