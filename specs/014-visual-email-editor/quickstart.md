# Phase 1 — Quickstart

**Feature**: Phase 7 — Visual Email Editor
**Branch**: `014-visual-email-editor`
**Date**: 2026-05-20

How to bring up the feature end-to-end on a developer machine, run the
test suite, and manually walk each of the five user stories.

## Prerequisites

- Docker daemon running (for the `nvelope-test-pg` testcontainer used
  by the Go integration tests).
- Go 1.24+, Node 22+, pnpm 10+.
- Working tree on branch `014-visual-email-editor`.

## Bring-up

```bash
# 1. Pull deps
go mod download
pnpm --filter ./frontend install

# 2. Run the new migration locally (devs commonly point at a local
#    postgres or let the testcontainer handle it; either way the
#    integration tests apply migrations themselves):
make test-db-clean           # if you want a clean DB
make migrate-up              # apply 000020_visual_editor_and_subscriber_fields

# 3. Start the API and the worker
go run ./cmd/api &
go run ./cmd/worker &

# 4. Start the SPA
pnpm --filter ./frontend dev
```

## Test suite

```bash
# Go — runs unit + integration (testcontainers spins up postgres)
make test

# Frontend — vitest
pnpm --filter ./frontend test
```

New coverage to add per user story (see [plan.md](./plan.md)
**Testing** section):

- Renderer golden tests in `internal/campaign/adapters/visualrender/`.
- Sanitizer negative tests for every disallowed construct.
- Placeholder validation tests across save and send paths.
- Tenant-isolation integration test for `subscriber_fields` CRUD.
- Send-pipeline integration test substituting per recipient.
- Vitest component tests for `<VisualEmailEditor />`, the merge-tag
  picker, and the legacy code-view fallback.

## User-story walkthroughs

### US1 — Author a campaign visually

1. Sign in as a tenant operator with `campaigns:manage`.
2. Settings → Fields → add a custom field `country` (type: text).
3. Campaigns → New → land in the visual editor.
4. Type `/`, pick "Two-column layout"; in the left column insert a
   Heading and a Paragraph, in the right column insert an Image
   (pick from the Phase 6 media library).
5. Select the paragraph text; the bubble menu appears; toggle bold;
   click "Merge tag" → pick "First name"; a chip appears.
6. Drag the columns block above the heading using the handle on its
   left; observe the drop indicator and the reorder.
7. Click Preview → switch between desktop / mobile.
8. Save → confirm the API returned `bodyHtml`, `bodyText`, `bodyDoc`,
   and the row shows "Saved".
9. Send a test to your own address; confirm Gmail / Apple Mail
   renders the columns, the image, the bold paragraph, and your
   first name substituted.

### US2 — Author and reuse a campaign template

1. Templates → New → choose kind "campaign" → land in the visual
   editor.
2. Build a hero + body layout as in US1; save.
3. Campaigns → New → pick that template as the starting point;
   confirm the campaign editor opens pre-filled with the same blocks.
4. Open an older raw-HTML template from before this branch (or seed
   one with `body_doc IS NULL`); confirm it opens in the **CodeView**
   editor, not the visual surface, and is not silently parsed.

### US3 — Theme defaults from tenant branding

1. Branding → set a distinctive primary color (e.g. `#cc3366`).
2. Campaigns → New → confirm the default button color and link color
   in the visual editor match the branding without any per-campaign
   override.
3. Open theme controls → pick a different button color → save.
4. Send a test; confirm the inbox shows the **overridden** color.
5. Branding → change the primary color again; reopen the saved
   campaign; confirm the override **survives** (still your picked
   color, not the new branding primary).

### US4 — Code view round-trip

1. Open a visually-authored campaign.
2. Switch to code view (button in the editor chrome); the CodeMirror
   editor shows the rendered HTML.
3. Add a `<div class="footer-note">` wrapper around a section;
   return to the visual surface.
4. Confirm the change is preserved — either round-tripped, or shown
   as a labelled "raw HTML" block in the visual surface that still
   serializes the wrapper verbatim on save.
5. From the campaign menu, choose "Edit as HTML only" → the visual
   editor is dismissed; campaign remains sendable.
6. Take a legacy raw-HTML campaign → choose "Convert to visual" →
   confirm convertible regions become blocks and unconvertible
   regions become RawHTML blocks.

### US5 — Image insertion paths

1. In the visual editor, drag a `.png` from your desktop onto the
   canvas; confirm an inline upload progress chip appears, the
   upload completes, and the image is inserted in place.
2. Open Media → confirm the same asset appears as a new entry.
3. In a second campaign, click an empty Image block → "From media
   library" → pick the same asset; confirm it's referenced (not
   re-uploaded).
4. Paste an image from clipboard; same upload-and-insert path.
5. Try to upload a file that exceeds the Phase 6 size limit; confirm
   it's rejected up front with a specific reason and nothing is
   stored.
6. Delete the asset from the media library; reopen the campaign
   that referenced it; confirm a "no longer available" placeholder
   appears, not a broken image.
7. Inspect the saved `body_html` (via `GET /campaigns/{id}`) and
   confirm every `<img src>` is a tenant media URL — no data URLs,
   no third-party hotlinks.

## Verifying constitution gates manually

- **Tenant isolation**: sign in to two tenants in two browsers; the
  custom fields, saved templates, and media references in one tenant
  never appear in the other's pickers or previews.
- **Render server-side**: open browser devtools while saving a
  campaign — the request body is `{ bodyDoc, subject, theme }` only;
  the response carries `bodyHtml` and `bodyText` that the client did
  not produce.
- **Placeholder validation at save**: type `{{ subscriber.first_naem }}`
  (typo) in code view, save; the API returns
  `400 unknown_placeholder` with the offending slug listed.
- **No worker change**: send a campaign authored visually and a
  campaign authored as raw HTML side by side; both go through the
  same `cmd/worker` run; logs show identical span shapes.

## Open follow-ups

- HTML file upload as a convenience (drag a `.html` file to seed code
  view) — see [research.md § R13](./research.md). Not on the
  critical path; can land as a small follow-up PR.
- Migration of legacy custom-attribute usage into the new registry —
  not required for ship; tenants opt in by adding the corresponding
  registry rows.
