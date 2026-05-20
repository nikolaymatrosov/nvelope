---

description: "Task list for Phase 6 — Public Pages & Media — Frontend UI"
---

# Tasks: Phase 6 — Public Pages & Media — Frontend UI

**Input**: Design documents from `/specs/013-phase-6-ui/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/tenant-api.md

**Tests**: Included — the plan (Constitution II) requires each new route to ship a colocated `*.test.tsx`.

**Organization**: Tasks are grouped by user story to enable independent implementation and testing.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (US1–US5)
- All paths are relative to the repository root

## Path Conventions

Web application — frontend SPA changes under `frontend/src/`, plus minor copy
polish in the existing Go templates under `internal/api/templates/`. The Go
backend handlers, routes, schema, and migrations are **not** touched: all
Phase 6 endpoints already exist in `internal/api/` (see
[contracts/tenant-api.md](./contracts/tenant-api.md)).

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: No project initialization is needed — the `frontend/` SPA
already exists. This phase only confirms the baseline.

- [X] T001 Confirm the `frontend/` workspace builds and the existing test suite is green (`cd frontend && pnpm install && pnpm test --run`) and that the Go suite is green (`make test`) before starting.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Shared types, API client surfaces, query keys, permissions,
sidebar nav, and shared components every user story depends on.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete.

- [X] T002 [P] Add the Phase 6 view shapes to `frontend/src/lib/api-types.ts`: `SubscriptionPageView`, `SubscriptionPageFieldView`, `BrandingView`, `MediaAssetView` — field shapes per `specs/013-phase-6-ui/data-model.md`.
- [X] T003 Extend `CampaignView` in `frontend/src/lib/api-types.ts` with the new field `ArchiveVisible: boolean`.
- [X] T004 Add `'subscription_pages:manage'`, `'branding:manage'`, `'media:get'`, and `'media:manage'` to the `Permission` union and the `ALL_PERMISSIONS` array in `frontend/src/lib/api-types.ts`.
- [X] T005 Add a `subscriptionPages` namespace to the API client in `frontend/src/lib/api.ts` with methods `list(slug)` → `GET /subscription-pages`, `get(slug, id)` → `GET /subscription-pages/{id}`, `create(slug, input)` → `POST /subscription-pages`, `update(slug, id, input)` → `PUT /subscription-pages/{id}`, `remove(slug, id)` → `DELETE /subscription-pages/{id}`. All paths via the `tp(slug, …)` helper.
- [X] T006 Add a `branding` namespace to the API client in `frontend/src/lib/api.ts` with methods `get(slug)` → `GET /branding` and `save(slug, input)` → `PUT /branding`. All paths via `tp(slug, …)`.
- [X] T007 Add a `setArchive(slug, id, visible)` method to the existing `campaigns` namespace in `frontend/src/lib/api.ts` calling `POST /campaigns/{id}/archive` via `tp(slug, …)`.
- [X] T008 Add a `media` namespace to the API client in `frontend/src/lib/api.ts` with methods `list(slug)` → `GET /media`, `upload(slug, file, onProgress)` → `POST /media` (multipart, reusing the existing `requestMultipart` helper pattern from import/export), `remove(slug, id)` → `DELETE /media/{id}`. All paths via `tp(slug, …)`.
- [X] T009 [P] Add query keys to `frontend/src/lib/query.ts`: `subscriptionPages(slug)`, `subscriptionPage(slug, id)`, `branding(slug)`, `media(slug)`, `mediaAsset(slug, id)`.
- [X] T010 [P] Create the `PublicUrlList` shared component in `frontend/src/components/common/public-url-list.tsx` — accepts a list of `{label, url, kind}` rows and renders each with a copy-to-clipboard control (using `navigator.clipboard.writeText` + a `sonner` toast on success) and a "preview" link that opens the URL in a new browser tab; the `kind = 'token-template'` rows are rendered with a short explanatory note that the token is filled per subscriber.
- [X] T011 [P] Create the `CssEditor` shared component in `frontend/src/components/common/css-editor.tsx` — wraps a shadcn `<Textarea>` with a description string declaring the configured limit, a live byte counter, a disabled-save signal when the input exceeds the limit, and a read-only "sanitized preview" block populated from a `sanitized?: string | null` prop (rendered as preformatted text).
- [X] T012 [P] Create the `MediaPicker` shared component in `frontend/src/components/common/media-picker.tsx` — controlled-open modal: when open, fetches `api.media.list(slug)` (gated by `media:get`), renders the assets as a grid with image previews, and calls an `onPick(asset: MediaAssetView)` callback when a row is selected; an empty state directs the user to the Media library.
- [X] T013 Add three nav entries to `frontend/src/components/shell/sidebar.tsx` NAV array (in the natural sidebar ordering): `{label: 'Public pages', segment: 'public-pages', requires: ['subscription_pages:manage']}`, `{label: 'Branding', segment: 'branding', requires: ['branding:manage']}`, `{label: 'Media', segment: 'media', requires: ['media:get']}` — using the existing nav-entry shape and icons (e.g. `Globe`, `Palette`, `Image` from `lucide-react`).

**Checkpoint**: Types, client, query keys, permissions, shared components, and nav are ready — user stories can now begin in parallel.

---

## Phase 3: User Story 1 — Subscribe through a tenant's public page (Priority: P1)

**Goal**: A visitor lands on `/t/{slug}/subscribe/{pageSlug}`, submits, sees
the "check your email" page, follows the confirmation link, and lands on the
confirmation success page — all rendered server-side with the tenant's
branding applied.

**Independent Test**: Open a tenant's public subscription URL in a private
browser session, submit a new address, click the confirmation link from the
delivered email, and confirm the success page appears and the address shows
as active on the tenant's list. Repeat with an expired token and an
already-active address.

> The visitor-facing surface is already implemented by the Go HTML templates
> in `internal/api/templates/` (`subscribe.html`, `confirm.html`,
> `unsubscribed.html`, `error.html`, `layout.html`) and routed by the public
> middleware in `internal/api/server.go`. The tasks below are
> spec-edge-case polish on the existing templates only.

- [X] T014 [P] [US1] Audit `internal/api/templates/subscribe.html` against spec FR-001 to FR-005: confirm it renders configured visible/required profile fields + email, applies the tenant's `BrandingView` (logo, primary color, custom CSS), and surfaces inline field-level validation errors. Adjust copy or layout only if a behaviour gap is found; do not introduce new endpoints.
- [X] T015 [P] [US1] Audit `internal/api/templates/confirm.html` against spec FR-007 and FR-008: confirm the confirmation-success page names the list(s) joined and offers a link to the preference page (using the `/p/{token}` template); confirm the "already confirmed" outcome lands on a benign page rather than an error. Adjust template copy/branches only if a gap is found.
- [X] T016 [P] [US1] Audit `internal/api/templates/error.html` against spec FR-006, FR-009, and FR-010: confirm the expired-link page offers a one-click resend (POST to `/t/{slug}/confirm/{token}/resend`), the "not available" page renders for deleted/deactivated lists, and the rate-limited message is clear. Adjust copy only if a gap is found.

**Checkpoint**: US1 is independently demonstrable per its Independent Test once branding has been saved via US4 part B.

---

## Phase 4: User Story 2 — Manage preferences or unsubscribe via a personal link (Priority: P1)

**Goal**: A subscriber opens `/p/{token}`, sees their current profile and
list memberships, updates either, can unsubscribe per-list or from all, and
gets an explicit confirmation. Tampered/expired links land on the
access-denied page. One-click unsubscribe POSTed by an email client
completes without page interaction.

**Independent Test**: Generate a preference link for a known subscriber,
open it in a private browser session, change a list membership, save, reload,
confirm persistence; use unsubscribe-from-all and confirm the subscriber is
removed from future sends; open a tampered token and confirm the
access-denied page exposes no subscriber data.

> The visitor-facing surface is already implemented by `preferences.html`,
> `unsubscribed.html`, and the token-resolution path in
> `internal/api/preference_handlers.go`. The tasks below are spec-edge-case
> polish on the existing templates only.

- [X] T017 [P] [US2] Audit `internal/api/templates/preferences.html` against spec FR-011 to FR-013: confirm the page shows profile fields and list memberships, lets the subscriber update either, offers per-list unsubscribe and unsubscribe-from-all, applies tenant branding, and surfaces an explicit confirmation after save/unsubscribe. Adjust copy/markup only if a gap is found.
- [X] T018 [P] [US2] Audit the access-denied path against spec FR-014 and FR-016: confirm that invalid/tampered/expired tokens and tokens belonging to deleted subscribers all land on a generic access-denied state (likely a branch of `error.html`) that exposes NO subscriber identity, list membership, or profile data, and confirm the preference page is not client-side cached across reloads (the existing handler issues fresh data per request — verify no `Cache-Control` regression).
- [X] T019 [P] [US2] Audit `internal/api/templates/unsubscribed.html` against spec FR-015: confirm one-click unsubscribe (`GET/POST /u/{token}`) completes without page interaction and that the minimal "you have been unsubscribed" confirmation renders cleanly when a browser does load the link.

**Checkpoint**: US2 is independently demonstrable per its Independent Test once branding has been saved via US4 part B.

---

## Phase 5: User Story 3 — Browse a tenant's public campaign archive and RSS feed (Priority: P2)

**Goal**: A visitor opens `/t/{slug}/archive` and sees archive-visible
campaigns newest-first; clicking one renders the campaign as a standalone
page with the tenant's branding; pointing a feed reader at
`/t/{slug}/feed.xml` returns a valid RSS feed. Drafts, hidden, or other-
tenant campaigns return a not-found page.

**Independent Test**: Mark a sent campaign archive-visible (via US4 part C),
open the tenant's archive index URL, confirm the campaign is listed, open
the standalone page and confirm tenant branding is applied, point a feed
validator at the RSS URL and confirm clean validation. Mark the campaign
hidden and confirm it disappears within the freshness window.

> The visitor-facing surface is already implemented by `archive_index.html`,
> `archive_campaign.html`, and the RSS handler in
> `internal/api/rss_handler.go`. The tasks below are spec-edge-case polish on
> the existing templates only.

- [X] T020 [P] [US3] Audit `internal/api/templates/archive_index.html` against spec FR-017 and FR-019: confirm newest-first ordering, title/send-date/link per row, tenant branding application, and the empty-archive state (no campaigns) showing as a friendly page — not a blank or error. Adjust copy only if a gap is found.
- [X] T021 [P] [US3] Audit `internal/api/templates/archive_campaign.html` against spec FR-018 and FR-022: confirm the campaign content renders as a standalone web page with the tenant's branding and custom CSS applied, and that one tenant's CSS cannot affect another tenant's page. Adjust markup/copy only if a gap is found.
- [X] T022 [P] [US3] Audit the RSS output of `internal/api/rss_handler.go` against spec FR-021 and edge cases: confirm the feed validates against a standard RSS validator for zero, one, and many archive-visible campaigns; confirm conditional-GET behavior (the feed responds correctly to a feed reader's revalidation). Adjust handler-emitted headers/fields only if a gap is found.
- [X] T023 [P] [US3] Audit the not-found path for `GET /t/{slug}/archive/{campaignId}` (handler `handleArchiveCampaign`) against spec FR-020: confirm drafts, scheduled, hidden, deleted, and cross-tenant campaign IDs all return a not-found page and never expose campaign content.

**Checkpoint**: US3 is independently demonstrable per its Independent Test once one campaign has been flagged archive-visible via US4 part C.

---

## Phase 6: User Story 4 — Configure public pages and branding from the workspace (Priority: P1)

**Goal**: An administrator configures subscription pages (bound lists,
visible/required fields), tenant branding (logo URL, primary color, custom
CSS with a sanitized preview and size limit), the per-campaign
archive-visible toggle, and sees the per-tenant public URL bundle in one
place — all from inside the workspace.

**Independent Test**: As an administrator, create a subscription page bound
to a list with one required custom field, save tenant branding (logo,
primary color, custom CSS), copy the subscription URL, open it in a private
browser session and confirm Story 1 renders with the branding and the
configured fields; toggle archive-visible on a sent campaign and confirm
Story 3's surfaces reflect the change within the freshness window.

### Part A — Subscription pages

- [X] T024 [US4] Create `frontend/src/routes/t/$slug/public-pages/index.tsx` — list route: queries `api.subscriptionPages.list(slug)`, renders each page as a row (title, slug, bound lists, public URL, active badge); shows an explicit "create your first subscription page" empty state; gates the route under `subscription_pages:manage`; renders the `PublicUrlList` for the per-tenant URLs (each saved subscription page's URL + the `/p/{token}` template + `/t/{slug}/archive` + `/t/{slug}/feed.xml`); offers "create new" and "preview" controls.
- [X] T025 [US4] Create `frontend/src/routes/t/$slug/public-pages/$id.tsx` — edit/create route: form using `@tanstack/react-form` for slug, title, target list IDs (multi-select against `api.listLists`), the field-config list (key/label/required rows with add/remove; `email` field is implicit and not editable), `sending_domain_id` (select against the existing sending-domains list, optional), `from_name`, `from_local_part`, `active` toggle; save calls `api.subscriptionPages.create` or `update` and invalidates `subscriptionPages(slug)` and `subscriptionPage(slug, id)`; delete is behind the shared `<ConfirmDialog>` and calls `api.subscriptionPages.remove`; "preview" button opens the saved page's `PublicURL` in a new tab; maps backend `incorrect_input` (e.g. slug taken) to inline field errors, `subscription_page_not_found` to a not-found state, and `403` to a placeholder.
- [X] T026 [P] [US4] Add `frontend/src/routes/t/$slug/public-pages/index.test.tsx` — colocated test using `renderWithClient` + mocked `@/lib/api`: covers empty state, populated list, copy-URL action toasts, permission-gated nav hiding, and tenant scoping (every list call asserts `slug` is forwarded).
- [X] T027 [P] [US4] Add `frontend/src/routes/t/$slug/public-pages/$id.test.tsx` — colocated test: covers create form happy path, slug-taken inline error, target-list-required validation, field add/remove, sending-domain optional, delete confirm flow, and not-found rendering on `subscription_page_not_found`.

### Part B — Branding

- [X] T028 [US4] Create `frontend/src/routes/t/$slug/branding/index.tsx` — branding route: queries `api.branding.get(slug)`; renders a form with logo URL (free-form URL input or a "pick from media library" button that opens `MediaPicker` and sets the URL to the picked asset's `PublicURL`), primary color (HTML color input restricted to `#RRGGBB`), and the `CssEditor` shared component bound to `customCss` + `CustomCSSBytes` + `CustomCSSLimitBytes` from the view; save calls `api.branding.save(slug, …)` and on success re-fetches the view so the sanitized preview reflects the server-returned CSS; route gated under `branding:manage`.
- [X] T029 [P] [US4] Add `frontend/src/routes/t/$slug/branding/index.test.tsx` — covers the GET-then-PUT round trip, the size-limit-exceeded disabled-save state, the sanitized-preview-after-save assertion, the `MediaPicker` integration for logo selection, and the permission-gated nav hiding.

### Part C — Archive-visible toggle

- [X] T030 [US4] Extend `frontend/src/routes/t/$slug/campaigns/$id.tsx` with an "Archive visibility" control rendered for sent campaigns (`campaigns:manage` gated). Calls `api.campaigns.setArchive(slug, id, visible)`, invalidates `campaign(slug, id)` and `campaigns(slug)`, shows a `sonner` toast on success, disables the control while the mutation is in flight, and surfaces errors with the existing `apperr`-to-state mapping pattern.
- [X] T031 [P] [US4] Extend `frontend/src/routes/t/$slug/campaigns/$id.test.tsx` with assertions for the archive toggle: hidden when the user lacks `campaigns:manage`, disabled while in-flight, toast on success, error toast on `campaign_not_found`, and the `ArchiveVisible` field reflected in the displayed state.

**Checkpoint**: US4 is independently demonstrable per its Independent Test — saving branding and toggling archive flow through to Stories 1, 2, 3 server-side renderings.

---

## Phase 7: User Story 5 — Manage media in the tenant library (Priority: P2)

**Goal**: A team member browses tenant-scoped media with previews, uploads
new files with progress feedback (rejected up-front for size/type
violations), copies stable URLs, deletes assets behind a confirm, and
inserts an asset reference into the existing campaign HTML body via a
picker. Cross-tenant access is impossible.

**Independent Test**: Upload an image, confirm it appears with a preview and
a copyable URL; attempt a too-large or wrong-type upload and confirm the
specific rejection reason and that nothing is stored; insert an asset from
the picker into the HTML body field on `campaigns/$id`; delete the asset and
confirm it disappears; from a different tenant, attempt to GET the asset URL
and confirm the request is denied.

- [X] T032 [US5] Create `frontend/src/routes/t/$slug/media/index.tsx` — library route: queries `api.media.list(slug)`; renders the assets as a responsive grid with image previews (using `IsImage`) and a filename/size/created-at caption; explicit empty state with the upload control; an upload form using a file input that validates size and content-type client-side against the constants from `frontend/src/lib/api-types.ts` and rejects with a specific reason before sending; on submit calls `api.media.upload(slug, file, onProgress)`, shows an in-flight progress indicator, and invalidates `media(slug)` on success; each row has a "copy URL" control (writes `PublicURL`) and a "delete" control behind the shared `<ConfirmDialog>` calling `api.media.remove(slug, id)`. Route gated under `media:get`; upload + delete gated under `media:manage`.
- [X] T033 [US5] Create `frontend/src/routes/t/$slug/media/$id.tsx` — asset detail route: shows the preview, filename, content type, size, created-at, and a prominent copyable `PublicURL`; not-found state on `media_asset_not_found`; route gated under `media:get`.
- [X] T034 [US5] Extend `frontend/src/routes/t/$slug/campaigns/$id.tsx` to attach the `MediaPicker` modal to the HTML body field — an "Insert from media library" button next to the HTML body label opens the picker; on select, inserts the asset's `PublicURL` (as a plain string) at the current cursor position of the HTML-body textarea (or appends if the textarea is unfocused); gated under `media:get`.
- [X] T035 [P] [US5] Add `frontend/src/routes/t/$slug/media/index.test.tsx` — covers empty state, populated grid, upload happy path with progress, oversize/disallowed-type up-front rejection (no network call), 413 from server rendered inline, delete confirm flow, tenant scoping on every call, and permission-gated upload/delete control visibility.
- [X] T036 [P] [US5] Add `frontend/src/routes/t/$slug/media/$id.test.tsx` — covers happy-path detail render with copy-URL action and the not-found rendering on `media_asset_not_found`.
- [X] T037 [P] [US5] Extend `frontend/src/routes/t/$slug/campaigns/$id.test.tsx` with assertions for the media picker: button visible only with `media:get`, picker open/close, asset insertion into the HTML body at the cursor, append behaviour when the textarea is unfocused.
- [X] T038 [P] [US5] Add `frontend/src/components/common/media-picker.test.tsx` — covers tenant-scoped list rendering, empty state directing to the Media library, and the `onPick` callback firing with the selected asset.

**Checkpoint**: US5 is independently demonstrable per its Independent Test — upload, copy, insert, delete, and reject flows all work.

---

## Phase 8: Polish & Cross-Cutting Concerns

**Purpose**: Confirm cross-cutting requirements (tenant isolation,
accessibility, permission gating, error states) are met across the new
surfaces and the existing public templates pick up the saved branding
end-to-end.

- [X] T039 [P] Run `pnpm test --run` and `pnpm typecheck` in `frontend/` and confirm a green baseline; fix any drift introduced by Phase 2 type changes.
- [X] T040 [P] Run `make test` from the repo root and confirm the Go suite (including the existing Phase 6 integration tests) stays green — this UI phase introduces no Go production code.
- [X] T041 [P] Manual cross-tenant isolation check per spec SC-004: signed in as a member of Tenant A, attempt to fetch Tenant B's media asset URL (guessing the storage path), Tenant B's archive index URL behaviour, and Tenant B's subscription-page public URL — confirm the backend denies cross-tenant access in every case and the UI never exposes another tenant's data.
- [X] T042 [P] Manual accessibility pass per spec FR-044 on the workspace surfaces added by this phase (Public pages list, Public-page edit, Branding, Media library, Media detail): keyboard navigation, focus rings, labels on every input, and 4.5:1 contrast on text. Document any issues found.
- [X] T043 Walk through the quickstart end-to-end (per `specs/013-phase-6-ui/quickstart.md`) and confirm each user-story Independent Test passes against a locally running backend on branch `012-phase-6-public-pages-media` or later.

---

## Dependencies

- **Phase 1 (T001)** is a baseline check.
- **Phase 2 (T002–T013)** is foundational and blocks every user-story phase. Within Phase 2: T003 depends on T002 reading the same file; T005–T008 may run together once T002+T004 land; T010–T012 are independent shared components and run in parallel; T013 (sidebar) depends on T004 (permissions).
- **Phases 3, 4, 5 (US1, US2, US3)** are visitor-facing template audits and may run in parallel with each other and with Phase 6 (US4). They have no SPA dependencies on Phase 2 — they touch only `internal/api/templates/`.
- **Phase 6 (US4)** depends on Phase 2 (types, client, components, nav). Within Phase 6: Part A (T024–T027), Part B (T028–T029), and Part C (T030–T031) are independent of each other and may run in parallel.
- **Phase 7 (US5)** depends on Phase 2. T034 and T037 touch `campaigns/$id.tsx` and `campaigns/$id.test.tsx`, which are also touched by Phase 6 Part C (T030, T031) — sequence these (T030/T031 first or after, not concurrently) to avoid merge conflicts in the same files.
- **Phase 8** depends on every preceding phase being complete.

## Parallel Execution Examples

Within Phase 2 once T002 and T004 land:

```text
T005 (api.subscriptionPages)    [P]
T006 (api.branding)             [P]
T007 (api.campaigns.setArchive) [P]
T008 (api.media)                [P]
T009 (query keys)               [P]
T010 (PublicUrlList)            [P]
T011 (CssEditor)                [P]
T012 (MediaPicker)              [P]
```

Within Phase 6 (US4) once Phase 2 is complete:

```text
T024 + T026                     [Part A — pages list]
T025 + T027                     [Part A — pages edit]
T028 + T029                     [Part B — branding]
T030 + T031                     [Part C — archive toggle]
```

(Parts A/B/C run in parallel; T030/T031 must NOT run concurrently with
T034/T037 because they touch the same `campaigns/$id.{tsx,test.tsx}`
files.)

Phase 5 (US3) template audits run in parallel within the phase:

```text
T020 (archive_index.html)       [P]
T021 (archive_campaign.html)    [P]
T022 (RSS handler)              [P]
T023 (not-found path)           [P]
```

## Implementation Strategy — MVP first, incremental delivery

1. **MVP** = Phase 2 + Phase 6 Part A (US4 subscription-pages CRUD). Once
   that lands, an administrator can create a subscription page and the
   already-shipped `subscribe.html` template serves it on the public URL —
   the platform's "public subscription page exists and is configurable from
   the workspace" promise is met end-to-end.
2. **Second slice** = Phase 6 Part B (branding) + Phase 3, 4, 5 template
   audits. Saving branding flows into all three visitor-facing surfaces
   (US1, US2, US3) the moment a public page is opened.
3. **Third slice** = Phase 6 Part C (archive toggle). Story 3's archive
   surfaces and RSS feed gain content via the per-campaign flag.
4. **Fourth slice** = Phase 7 (US5 media library + picker). Independent of
   everything else; ships when ready.
5. **Closing** = Phase 8 cross-cutting checks and end-to-end quickstart
   walkthrough.

Each slice is independently demonstrable per the spec's Independent Test
for the corresponding user story.
