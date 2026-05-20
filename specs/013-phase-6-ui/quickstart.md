# Quickstart — Phase 6 UI

How to run, verify, and test the Phase 6 frontend work locally.

## Prerequisites

- Node 22+ and pnpm available in your shell.
- A running Docker daemon (for the backend test containers, if running the
  full Go suite).
- The Go backend on branch `012-phase-6-public-pages-media` (or later)
  running locally — Phase 6 endpoints (subscription pages, branding,
  media, archive toggle) must be available.

## Run the workspace UI

```sh
cd frontend
pnpm install
pnpm dev
```

The dev server proxies API calls to the Go backend; see existing
`frontend/vite.config.ts` / Vite proxy settings (unchanged in this phase).

## Verify each user story end to end

These map 1:1 to the spec's Independent Test for each user story.

### US4 — Configure public pages and branding (P1)

1. Sign in as a user with `subscription_pages:manage`, `branding:manage`,
   and `campaigns:manage`.
2. Open **Public pages** in the sidebar → empty state with a "create your
   first subscription page" call to action.
3. Create a subscription page bound to one list with `email` plus one
   required custom field (e.g. `first_name`). Save → the page appears in
   the list with its public URL.
4. Copy the public URL and open it in a private browser session. The Go
   template `subscribe.html` should render with the configured fields.
5. Open **Branding** in the sidebar; upload a logo URL, pick a primary
   color, paste a small custom CSS rule, save. Confirm the sanitized
   preview block appears and matches the saved CSS.
6. Reload the public subscription URL — the branding should be applied
   (logo + color reflected by the server-rendered template).
7. Open a sent campaign (`campaigns/$id`), toggle archive-visible on,
   confirm a success toast. Open the per-tenant archive index URL (from
   the "Public URLs" card on the Public pages landing) — the campaign
   appears within the freshness window.

### US5 — Manage media (P2)

1. Sign in as a user with `media:get` and `media:manage`.
2. Open **Media** in the sidebar → empty state with the upload control.
3. Upload an image (e.g. `< 1 MB PNG`) — progress shows, the asset appears
   in the grid with a preview thumbnail. Open its detail view; copy the
   stable public URL.
4. Attempt to upload a file that exceeds `MediaMaxBytes` or uses a
   disallowed type — the upload is rejected up-front with a specific
   reason and nothing is added to the listing.
5. Open the campaign editor (`campaigns/$id`), focus the HTML-body field,
   open the **Insert from media library** picker, select the asset; its
   public URL is inserted at the cursor.
6. Delete the asset behind the confirm dialog — it disappears from the
   grid. Re-open the campaign editor; the inserted URL still appears as
   text in the HTML body (the asset is gone server-side; the rendered
   archive page will show the missing-asset placeholder).

### US1 / US2 / US3 — Visitor-facing flows (already shipped by the backend)

These pages are server-rendered Go templates; verify them in a browser
against the local backend (no SPA changes required, but the templates
must pick up the branding saved in US4):

1. Open the subscription page URL — it should display the saved logo,
   primary color, and custom CSS.
2. Submit a new address — land on the "check your email" page; retrieve
   the email and click the confirmation link → confirmation success page.
3. From a campaign footer (or by minting a preference token via the
   tenant CLI / a test helper), open `/p/{token}` — the preference page
   should display under the tenant's branding.
4. From `/t/{slug}/archive` confirm the archive index renders; click an
   archived campaign to see the standalone page; point a feed validator
   at `/t/{slug}/feed.xml` — RSS validates cleanly.

## Tests

### Frontend unit + component tests

```sh
cd frontend
pnpm test           # vitest watch mode
pnpm test --run     # one-shot run for CI
```

Every new and changed route ships a colocated `*.test.tsx`:
- `public-pages/index.test.tsx`, `public-pages/$id.test.tsx`
- `branding/index.test.tsx`
- `media/index.test.tsx`, `media/$id.test.tsx`
- `campaigns/$id.test.tsx` gains assertions for the archive toggle and
  the media picker insertion.

Tests use the existing `renderWithClient` helper and mock `@/lib/api`
through `vi.mock("@/lib/api", { api: { ... } })` exactly as the prior UI
phases do.

### Backend suite (unchanged)

```sh
make test           # full Go suite — must remain green
```

This phase adds no Go code and no migrations, so the existing Phase 6
test suite is the regression baseline.

## Make sure the constitution gates pass

- **Tenant isolation**: every API call in `api.subscriptionPages`,
  `api.branding`, `api.campaigns.setArchive`, `api.media` takes `slug` as
  the first argument and routes through `tp(slug, …)`. Grep for any new
  `fetch(`, `axios(`, or hand-built URL in the new routes — there must
  be none.
- **Test-backed delivery**: each new route has a colocated test exercising
  its primary flow and at least one edge state (empty / in-progress /
  error). The campaign-detail test gains the archive toggle and media
  picker assertions.
- **Incremental, shippable phases**: each user story (US4 a/b/c, US5) is
  individually demonstrable using the steps above. None depends on
  another being delivered first.
- **Layered architecture**: the new routes import only from `@/lib/api`,
  `@/lib/api-types`, `@/lib/query`, `@/components/...`, and `@/hooks/...`
  — no direct transport calls in route files.
