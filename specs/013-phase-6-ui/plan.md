# Implementation Plan: Phase 6 — Public Pages & Media — Frontend UI

**Branch**: `013-phase-6-ui` | **Date**: 2026-05-20 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/013-phase-6-ui/spec.md`

## Summary

Deliver the tenant-workspace web interface for the already-built Phase 6
Public Pages & Media backend: configure per-tenant subscription pages, edit
tenant branding (logo URL, primary color, custom CSS), toggle per-campaign
archive visibility, and manage a tenant-scoped media library (list, upload,
delete) with a picker that inserts an asset reference into the existing
campaign HTML-body editor. The workspace also presents the per-tenant public
URLs (subscription, preference-link template, archive index, RSS feed) so an
administrator can copy and share them.

The visitor-facing surfaces — public subscription form, "check your email"
page, confirmation success / expired-link / already-confirmed pages,
per-subscriber preference page, unsubscribe confirmation, public archive
index, standalone archive campaign page, RSS feed, and the not-available /
access-denied / error pages — are **already implemented as server-rendered
Go HTML templates** in [internal/api/templates/](../../internal/api/templates/)
(`subscribe.html`, `confirm.html`, `preferences.html`, `unsubscribed.html`,
`archive_index.html`, `archive_campaign.html`, `error.html`, `layout.html`)
and routed by the public-tenant middleware in
[internal/api/server.go](../../internal/api/server.go). They consume the
branding the admin configures in this UI; they need **no React SPA route**.
Any cosmetic polish to those templates that the spec edges expose
(rate-limit message text, expired-link wording, already-confirmed page) is
in-scope of this feature but additive — the templates are already shipped.

The SPA work extends the existing Phase 1–5 React/TanStack app shell with
three new permission-gated nav areas (Public pages, Branding, Media) and a
small extension to the existing campaign detail route for the
archive-visible toggle and the media picker on the HTML body field.
**No new backend endpoints, no schema, no migrations** — all Phase 6
endpoints (`subscription-pages` CRUD, `branding` GET/PUT, `campaigns/{id}/archive`,
`media` list/upload/delete) already exist in `internal/api/` (see
[contracts/tenant-api.md](./contracts/tenant-api.md) for the full list).

## Technical Context

**Language/Version**: TypeScript 5.9 / React 19 (frontend only); a small
amount of Go template polish if edge-case copy needs to change.

**Primary Dependencies**: TanStack Start/Router/Query/Form/Table, shadcn +
Radix UI, Tailwind v4, lucide-react, sonner

**Storage**: PostgreSQL via the existing Phase 6 schema (subscription pages,
branding, archive flag on `campaigns`, media assets) plus S3-compatible
object storage for media bytes — no new tables, no new migrations; the UI
is a pure consumer of existing endpoints.

**Testing**: vitest + @testing-library/react (colocated `*.test.tsx` per
route, `renderWithClient` helper, mocked `@/lib/api` and
`@tanstack/react-router`). Multipart upload behavior unit-tested by mocking
`api.media.upload`.

**Target Platform**: Modern desktop browsers for the authenticated workspace.
The visitor-facing templates that ship with the Go backend already cover
mobile browsers and the baseline accessibility expectations (FR-044).

**Project Type**: Web application — existing `frontend/` SPA extended;
visitor-facing public pages stay server-rendered in Go.

**Performance Goals**: Interactive UI. Media uploads stream multipart to the
backend with a progress indicator and disable the submit control until
resolved. Archive-visible toggle is a single mutation; the success criterion
SC-005 (≤5 min freshness) is met by the backend's existing archive
materialization, not by the UI.

**Constraints**: No new frontend framework; extend not rebuild the app
shell; no new backend endpoints, commands, or schema; CSS sanitization,
file-type and size limits, and tenant-storage prefixing are enforced
server-side and the UI communicates them but is not the source of truth;
the SPA never authors visitor-facing pages — those stay server-rendered.

**Scale/Scope**: 3 new nav areas (Public pages, Branding, Media), ~5 new
file-routes (`public-pages/index.tsx`, `public-pages/$id.tsx`,
`branding/index.tsx`, `media/index.tsx`, `media/$id.tsx`), 2 small inline
extensions to existing routes (`campaigns/$id.tsx` for the archive toggle
and the media picker on the HTML body field), 1 new API client namespace
group (`subscriptionPages`, `branding`, `media`, `campaigns.setArchive`)
with ~10 methods, ~6 new query keys, 4 new `Permission` union members
(`subscription_pages:manage`, `branding:manage`, `media:get`,
`media:manage`), and ~3 small shared presentational components
(`MediaPicker`, `PublicUrlList`, `CssEditor`).

## Constitution Check

*GATE: evaluated against `.specify/memory/constitution.md` v1.1.0.*

- **I. Tenant Isolation by Default** — PASS. No data-layer changes. Every
  API call goes through the tenant-scoped client (`tp(slug, …)`), which
  makes `slug` a required first argument, so a call site cannot omit tenant
  scope. The Phase 6 backend scopes every subscription page, branding row,
  media asset, and archive flag to the tenant; media is tenant-prefixed in
  object storage and the `UploadAsset`/`DeleteAsset`/`ListAssets` commands
  enforce isolation. The visitor-facing public pages are tenant-resolved by
  `resolvePublicTenant` middleware in
  [internal/api/public_middleware.go](../../internal/api/public_middleware.go)
  and the preference/unsubscribe handlers verify the signed token before
  returning any subscriber data.
- **II. Test-Backed Delivery** — PASS. Each new frontend route ships a
  colocated `*.test.tsx` covering its primary flow plus the key
  empty/in-progress/error states (no subscription pages yet; CSS save with a
  sanitization notice; media upload rejection by size/type; delete-confirm;
  archive toggle round-trip; media picker insertion). No backend change, so
  the existing Phase 6 suite stays green.
- **III. Incremental, Shippable Phases** — PASS. The five user stories from
  the spec are independently shippable. The visitor-facing surfaces (US1,
  US2, US3) are already in production via the Phase 6 backend templates and
  pick up branding the moment an administrator saves it in US4; the media
  library (US5) is independent of US1–US4. No speculative scope; vanity
  domains, real card capture, public-page WYSIWYG, and i18n of public copy
  are explicitly excluded.
- **IV. Security & Consent by Design** — PASS. Nav and action gating reuse
  the four Phase 6 permission strings declared in
  [internal/iam/domain/permission.go](../../internal/iam/domain/permission.go):
  `subscription_pages:manage`, `branding:manage`, `media:get`,
  `media:manage`. The backend re-checks every request and stays
  authoritative; a `403`/`404` is rendered in place, a `401` routes to
  sign-in. The UI surfaces that custom CSS is sanitized server-side and
  shows the sanitized preview rather than the raw input (FR-028). Public
  pages remain server-rendered and never expose another tenant's data.
- **V. Operable & Observable Services** — PASS. The frontend is stateless.
  No service, job, or queue change.
- **VI. Layered Architecture & Domain Integrity** — PASS. No backend code.
  The frontend keeps transport isolated in `lib/api.ts`; routes consume
  typed view shapes from `lib/api-types.ts` and never construct URLs
  themselves; error kinds returned by the four endpoint families are mapped
  to UI states in one place per route.

**Result**: PASS — no violations, Complexity Tracking not required.

*Post-design re-check*: the design adds no new layers, no DI, no schema
change, no duplicated infrastructure, and no backend code. Still PASS.

## Project Structure

### Documentation (this feature)

```text
specs/013-phase-6-ui/
├── plan.md              # This file
├── research.md          # Phase 0 output — backend & frontend findings
├── data-model.md        # Phase 1 output — view shapes the UI consumes
├── quickstart.md        # Phase 1 output — run, verify, test instructions
├── contracts/
│   └── tenant-api.md    # Phase 1 output — the Phase 6 endpoints the UI consumes
├── checklists/
│   └── requirements.md  # Spec quality checklist (/speckit-specify)
└── tasks.md             # Phase 2 output (/speckit-tasks — NOT created here)
```

### Source Code (repository root)

```text
frontend/src/
├── lib/
│   ├── api.ts                       # + subscriptionPages namespace (list/get/create/update/delete)
│   │                                # + branding namespace (get/save)
│   │                                # + campaigns.setArchive(slug, id, visible)
│   │                                # + media namespace (list/upload/delete) — multipart
│   ├── api-types.ts                 # + SubscriptionPageView, BrandingView, MediaAssetView
│   │                                # + 'subscription_pages:manage' | 'branding:manage'
│   │                                #   | 'media:get' | 'media:manage' in Permission
│   │                                # + add same four entries to ALL_PERMISSIONS
│   └── query.ts                     # + query keys: subscriptionPages, subscriptionPage,
│                                    #   branding, media
├── components/
│   ├── common/
│   │   ├── media-picker.tsx         # NEW — modal: browse tenant media + insert reference
│   │   ├── public-url-list.tsx      # NEW — copyable per-tenant URL bundle
│   │   └── css-editor.tsx           # NEW — textarea + sanitized-preview + size limit
│   └── shell/
│       └── sidebar.tsx              # + 3 nav entries (Public pages, Branding, Media),
│                                    #   gated on the corresponding permissions
└── routes/t/$slug/
    ├── public-pages/
    │   ├── index.tsx                # list subscription pages + create-first empty state +
    │   │                            #   Public URL bundle (US4 part 1)
    │   ├── index.test.tsx
    │   ├── $id.tsx                  # edit page: bound lists, visible/required fields,
    │   │                            #   public URL, preview, delete (US4 part 2)
    │   └── $id.test.tsx
    ├── branding/
    │   ├── index.tsx                # logo URL, primary color, custom CSS editor with
    │   │                            #   sanitized preview, size limit (US4 part 3)
    │   └── index.test.tsx
    ├── media/
    │   ├── index.tsx                # library grid + upload control + delete (US5)
    │   ├── index.test.tsx
    │   ├── $id.tsx                  # asset detail: preview + copyable stable URL
    │   └── $id.test.tsx
    └── campaigns/
        ├── $id.tsx                  # + archive-visible toggle (gated campaigns:manage)
        │                            # + MediaPicker hook on HTML body editor (US5 + US4.5)
        └── $id.test.tsx             # + tests for the toggle and the picker insertion
```

**Structure Decision**: Existing web-application layout. The frontend SPA in
`frontend/` is extended with file-routes under the established
`routes/t/$slug/` tree, in three new segments (`public-pages/`, `branding/`,
`media/`). Two existing files (`routes/t/$slug/campaigns/$id.tsx` and
`components/shell/sidebar.tsx`) are extended; nothing else is touched.
Visitor-facing pages stay where they live today, in
[internal/api/templates/](../../internal/api/templates/). The Go backend is
not touched for endpoints, schema, or middleware; minor template-copy
adjustments (if any) are tracked as tasks but not as new endpoints.

## Phase 0 — Research

Complete. See [research.md](./research.md). All decisions resolved from
in-repo inspection of the Phase 6 backend (`internal/api/server.go`,
`internal/api/public_handlers.go`, `internal/api/preference_handlers.go`,
`internal/api/archive_handlers.go`, `internal/api/branding_handlers.go`,
`internal/api/media_handlers.go`, `internal/api/subscription_page_handlers.go`,
`internal/api/templates/*.html`) and the Phase 1–5 frontend conventions
(`frontend/src/lib/api.ts`, `frontend/src/lib/api-types.ts`,
`frontend/src/components/shell/sidebar.tsx`, prior route tests); no
`NEEDS CLARIFICATION` remain.

## Phase 1 — Design & Contracts

Complete:

- [data-model.md](./data-model.md) — the view shapes the UI consumes
  (`SubscriptionPageView`, `SubscriptionPageFieldView`, `BrandingView`,
  `MediaAssetView`) and the four new `Permission` union members; no new
  persisted entities.
- [contracts/tenant-api.md](./contracts/tenant-api.md) — the Phase 6
  endpoints the UI consumes, their request/response shapes, permission
  requirements, and error-kind → UI-state mapping.
- [quickstart.md](./quickstart.md) — run, verify, and test instructions.
- Agent context (`CLAUDE.md`) updated to point at this plan.

## Phase 2 — Next step

Run `/speckit-tasks` to generate `tasks.md`. Suggested task ordering:

1. **Shared frontend plumbing** — extend `api-types.ts` with the four view
   shapes and the four new `Permission` members (also added to
   `ALL_PERMISSIONS`); add the `subscriptionPages`, `branding`, `media`,
   and `campaigns.setArchive` API client surfaces; add the query keys; add
   the three sidebar nav entries; add the `MediaPicker`, `PublicUrlList`,
   and `CssEditor` shared components.
2. **US4 part A — Subscription pages** (P1) —
   `public-pages/index.tsx` and `public-pages/$id.tsx` with the list /
   create-first empty state, the bound-list and field-config editor, the
   per-page public URL display with copy and preview, and delete behind a
   confirm.
3. **US4 part B — Branding** (P1) — `branding/index.tsx` with the logo URL,
   primary color, custom CSS editor (sanitized preview + size limit), and
   the per-tenant `PublicUrlList` (subscription URLs, preference-link
   template, archive index, RSS feed).
4. **US4 part C — Archive toggle** (P1) — inline `campaigns:manage`-gated
   archive-visible toggle on `campaigns/$id.tsx` calling
   `api.campaigns.setArchive`.
5. **US5 — Media library** (P2) — `media/index.tsx` with the upload
   control + progress + size/type rejection, the library listing with
   image previews and an explicit empty state, and the delete confirm;
   `media/$id.tsx` with the asset detail and stable URL; the
   `MediaPicker` modal wired into the HTML-body field on
   `campaigns/$id.tsx` (inserts an asset reference).
6. **Visitor-template polish** (in scope only if surfaced) — small copy or
   layout adjustments to `subscribe.html`, `confirm.html`,
   `preferences.html`, `unsubscribed.html`, `archive_index.html`,
   `archive_campaign.html`, `error.html` to cover the rate-limited message,
   the already-confirmed page, and the not-available page from the spec
   edges. No new routes.
7. **Tests for every new and changed route** — colocated `*.test.tsx`
   using the existing `renderWithClient` helper and `@/lib/api` mock
   conventions; tenant-isolation assertions on every list call.

US4 is split across three tasks because the spec treats it as a single
"configure public pages" story but the UI ships it as three independent
sub-surfaces (subscription pages, branding, archive toggle) that the
administrator reaches separately. Each sub-task is still independently
demonstrable per the spec Independent Test for US4.

## Complexity Tracking

No constitution violations — section intentionally empty.
