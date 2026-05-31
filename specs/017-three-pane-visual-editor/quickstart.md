# Quickstart: Three-Pane Visual Editor

**Feature**: 017-three-pane-visual-editor | **Date**: 2026-05-31

How to run, verify, and manually test the three-pane editor. Assumes feature 014
(visual editor) is already in place and working.

## Prerequisites

- Docker daemon running (integration tests use testcontainers `postgres:17`).
- `frontend/` deps installed (`pnpm install`); the new shadcn `resizable`
  component pulls in `react-resizable-panels` (MIT).
- A tenant with `campaigns:manage` / `templates:manage` permission to author.

## Add the layout primitive (one-time)

```bash
cd frontend
pnpm dlx shadcn@latest add resizable    # adds src/components/ui/resizable.tsx + react-resizable-panels
```

## Run the stack

```bash
# Backend (API) + worker as usual
make run                # or: go run ./cmd/api  (+ ./cmd/worker for sends)

# Frontend + BFF (TanStack Start + Nitro) — hosts the visual save/preview routes
cd frontend && pnpm dev
```

Open a campaign in the workspace: `/t/{slug}/campaigns/{id}`. A new campaign lands
in the visual editor, now rendered as three panes.

## Verify (automated)

```bash
# Go — domain validator bounds + sanitizer survival/strip + visual round-trip + isolation
go test ./internal/campaign/...
go test ./internal/api/... -run VisualSanitization

# Frontend + BFF — render goldens (styled variants), validator bounds, panels, selection sync
cd frontend
pnpm test src/server/render          # golden fixtures incl. *-styled
pnpm test src/server/validate        # style-bounds + existing drift-catcher
pnpm test src/components/visual-editor
pnpm test src/i18n                    # en/ru namespace parity for new visualEditor keys
```

Storybook (per the repo Storybook workflow): preview the new
`StructureOutline` and `BlockParamsPanel` stories and run their story tests before
handoff.

## Manual test — US1 (parameters panel, P1)

1. Open a new campaign; type a heading and add a Button block via slash command.
2. Click the button on the canvas → the right panel shows button params.
3. Change background color, corner radius, padding, and font weight → the canvas
   button updates live (< 1 s), nothing else changes.
4. Click "reset" on one field → it reverts to the theme default; the panel marks
   it inherited again.
5. Save. Reopen the campaign → the button keeps every parameter (lossless
   round-trip).
6. Send a test → the inbox button shows the chosen background, radius, padding,
   and weight (Gmail/Apple Mail/Outlook).

## Manual test — US2 (structure outline, P2)

1. Build a campaign with a 3-column layout, each column holding a paragraph +
   image.
2. Open the left panel → confirm the outline mirrors the nesting; each entry is
   labelled by type + a short content label.
3. Click a nested paragraph entry → the canvas scrolls to and selects it; the
   params panel loads its controls; the outline entry highlights.
4. Drag an outline entry to reorder → the canvas reflects the move; try dropping a
   column container into its own column → rejected, document unchanged.
5. Duplicate then delete a block from the outline → canvas + output reflect both.
6. Collapse the columns container entry → its children hide.

## Manual test — US3 (collapsible panels, P2)

1. Collapse the left panel → the canvas widens; collapse the right panel → widens
   again.
2. Re-expand each via its affordance → returns to prior width with content intact.
3. Reload the editor → panels return to the last-used collapsed/expanded state and
   widths.
4. Shrink the browser to ≤ 1024 px → side panels collapse by default and open as
   overlays; the canvas stays usable with no horizontal overflow.

## What did NOT change (sanity checks)

- No new migration: `git diff` shows no files under `internal/db/migrations/`.
- No new HTTP endpoint: the save/preview routes are the same 014 routes.
- `cmd/worker` send path untouched: it still reads `body_html` / `body_text`.
- Pre-017 campaigns (no per-block `style`) open and render exactly as before.
