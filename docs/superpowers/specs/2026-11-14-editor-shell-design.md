# Visual editor shell (site-builder style) — Design

Date: 2026-11-14
Status: Approved (pending implementation plan)

## Goal

Make the email visual editor look and work like a site-builder: a full-screen
editor with a top bar, a dotted canvas holding a fixed-width white "page", a
right-hand panel with **Layers** on top and **Content / Design** settings below,
and a left slide-out **library** for inserting content. The reference is a
Russian site builder (screenshots shared in the brainstorming session:
«Библиотека секций», «Слои», «Контент / Дизайн / Настройки», device switcher).

This is **phase 1 of 5**. It changes only the shell. The data model, BFF
render/validate layer, and Go backend are untouched.

## Roadmap (context, not in scope here)

The chosen paradigm is **sections on top of the document**: an email becomes a
stack of sections, and text inside a section stays inline-editable via TipTap.
Each phase gets its own spec → plan → implementation cycle.

1. **Shell** — this document.
2. **Section model** — a top-level `section` block in `VisualDoc`, a catalog of
   built-in section presets (header, hero, text, button, cards, footer), and a
   «Секции» group in the library. Touches TS/Go types, both validators, render,
   sanitizer.
3. **Role-based design** — section-level styles that cascade to children
   (e.g. "card title", "card background", "image border").
4. **My sections** — tenant-saved section presets (new table + endpoints).
5. **Global sections** — shared header/footer referenced by many templates,
   resolved at render time.

Permanently out of scope (email clients cannot render them, or no use case):
carousel, animations ("Динамика"), card flip side, `box-shadow`, opacity/rgba,
custom breakpoints and a tablet viewport, Figma import, an AI assistant, and
"Publish".

## Context

- The editor today is `ThreePaneEditor` (feature 017): `StructureOutline` on the
  left, the `VisualEmailEditor` TipTap canvas in the center, `BlockParamsPanel`
  on the right, built on `react-resizable-panels`. All three panes share
  `useBlockSelection`.
- It is embedded inline in a card on `routes/t/$slug/templates/$id.tsx` and
  `routes/t/$slug/campaigns/$id.tsx`, next to the name/subject form fields.
  Save sits at the bottom of the form.
- `saveVisual` + the `409 stale_row` reload/force-overwrite flow is duplicated
  across both routes.
- `ThemeControls` renders above the canvas inside `VisualEmailEditor`.
- `PreviewIframe` (server-rendered HTML, 600/375 toggle) exists but no route
  uses it.
- `SlashCommandMenu` has its own item list with hard-coded English labels.
- Every tenant route renders inside `AppShell` (sidebar + top bar) from
  `routes/t/$slug/route.tsx`.
- `campaigns/$id.tsx` has no `<Outlet/>`, so a nested `$id.edit.tsx` child would
  never render. The edit routes must be non-nested (`$id_.edit.tsx`).

## Decisions

### Entry and routing

- New full-screen routes:
  - `routes/t/$slug/templates/$id_.edit.tsx` → `/t/$slug/templates/$id/edit`
  - `routes/t/$slug/campaigns/$id_.edit.tsx` → `/t/$slug/campaigns/$id/edit`
- Both declare `staticData: { fullscreen: true }`. `AppShell` reads it via
  `useMatches()` and, when set, renders `children` without the sidebar, top bar,
  or page padding. Session/TOTP gating in `/t/$slug` keeps applying.
  `StaticDataRouteOption` is augmented with `fullscreen?: boolean`.
- Each route is thin: it loads the row and renders a shared `EditorScreen` with
  the entity-specific bits (breadcrumb label, back link, save function, whether
  the row is editable).
- **Detail pages** lose the inline editor. They keep name, subject, a
  **preview of the saved `body_html`** in a sandboxed `srcdoc` iframe (no render
  call; empty placeholder when blank), and an **«Открыть редактор»** button
  (hidden without `campaigns:manage`). The code-only (raw HTML) mode and "edit
  as HTML only" (opt-out) stay on the detail page. The detail form saves
  name/subject through the legacy update (which never touches `body_doc`); the
  editor saves the body.
- **Legacy raw-HTML rows**: the "convert to visual" button becomes a link to the
  editor. The editor calls `convert-to-visual` on load and opens the result as
  an unsaved (dirty) document. Nothing persists until the operator saves, the
  same non-persisting review flow as today.
- Transactional templates never open the editor; a direct visit shows a
  "not available" message with a back link.
- Campaigns that are not drafts open the editor read-only.

### Top bar

Left to right:

| Element | Behavior |
| --- | --- |
| `←` + breadcrumbs «Шаблоны / {name}» or «Кампании / {name}» | Back to the detail page. Unsaved changes → confirm (router `useBlocker` + `beforeunload`). |
| «+ Добавить блок» (primary) | Opens the library. Renamed «Добавить секцию» in phase 2. |
| Undo / Redo | TipTap history (`UndoRedo` ships in StarterKit 3). Disabled per `editor.can()`; re-rendered via `useEditorState`. |
| Desktop / Mobile toggle | Sets canvas width: theme `containerWidth` (default 600) or 375. The canvas only narrows; true mobile rendering is checked in Preview. |
| «Предпросмотр» | Full-screen dialog hosting the existing `PreviewIframe`. |
| «Сохранить» | Visual PUT, also ⌘/Ctrl+S. Disabled when not dirty. |

Read-only mode: a «Только просмотр» badge; Add, Undo/Redo, and Save are hidden.

### Canvas

- Grey dotted background (CSS `radial-gradient`), centered white column at the
  selected width, vertical scroll.
- Empty document (a single empty paragraph) → empty state «Добавьте первый
  блок» with a lucide icon and a button that opens the library.
- Hovered block: thin grey outline. Selected block: 2px blue outline (the
  existing `SelectionDecoration`), shown only while a block is selected.
- **Selection starts empty.** `useBlockSelection` reports nothing selected until
  the operator clicks into the canvas or a layer (ProseMirror always holds some
  selection internally, so the hook tracks an "active" flag). Esc or clicking
  the root «Письмо» clears it, and it stays cleared across later transactions.
- **Floating block toolbar**: centered under the selected block, showing the
  block name (same label as in Layers) plus «Дублировать» и «Удалить».
  Absolutely positioned inside the white page from the block's DOM rect. It does
  not use TipTap's `BubbleMenu`, so it never competes with the text-formatting
  bubble menu, and it renders in jsdom tests.
- Esc clears block selection (the root «Письмо» becomes current) unless an
  editor plugin (e.g. the slash menu) already handled the key. Delete and
  Backspace on a node selection are ProseMirror defaults.
- Unchanged: in-canvas `DragHandle`, text-formatting bubble menu, slash menu,
  merge-tag picker, media picker, raw-HTML modal.

### Right panel

The left pane is removed. The screen is canvas + right panel, with the panel
width resizable and persisted (`react-resizable-panels`, as today). Inside it,
a **vertical split** (height persisted):

- **Layers (top)** — the existing `StructureOutline`, moved right. Adds a root
  «Письмо» node that is current when nothing is selected. Per-row delete and
  duplicate move into a ⋮ menu. Click-select, drag-reorder, and container
  collapse are unchanged.
- **Settings (bottom)** — tabs «Контент» and «Дизайн». A tab with no fields for
  the current selection is hidden.

| Selection | Контент | Дизайн (field sets unchanged from feature 017) |
| --- | --- | --- |
| Письмо (root) | — | `ThemeControls` (inherit branding / pin override), moved out of `VisualEmailEditor` |
| Button | label, href | background, color, font family/size/weight, radius, padding, border |
| Image | replace from media library, alt, href | alignment, radius, border |
| Heading | level H1–H3 | color, font family/size/weight, line height, alignment, padding |
| Paragraph / list / quote | — | same as heading |
| Columns | — | column count 2/3/4, background, padding, radius, border |
| Column | — | background, padding, radius, border |
| Divider | — | border (line width/style/color), padding |
| Raw HTML | «Редактировать HTML» (opens the existing modal) | — |
| List item / code | — | — (panel says "no properties") |

Implementation: `ParamFormProps` gains `tab: "content" | "design"` plus
`editor` and `pos` (for actions that are more than an attr patch), and each
`*Params` form renders the subset for that tab. Changing the column count adds
empty columns, or folds the removed columns' content into the last kept column,
so no content is lost.

**Design tab look** (restyle of `StyleControls`, same `BlockStyle` fields and
bounds as feature 017):

- Groups with headings and dividers: «Типографика», «Отступы и выравнивание»,
  «Фон», «Границы».
- Padding: four inputs in one row, labelled «Слева / Сверху / Снизу / Справа»,
  with a `px` suffix.
- Alignment: segmented control.
- Border («Обводка») and radius («Скругление») are behind switches. Switching off
  clears the fields, so the block inherits again.

Narrow viewport (< 1024px): the right panel opens as a `Sheet` from a top-bar
button.

### Library

- A left `Sheet` (~680px) over the canvas, titled «Библиотека блоков». Opened by
  «+ Добавить блок» or the empty-state button. Closes on ✕, Esc, or a click
  outside.
- Left column: a category tree with one group «Основные», listing block types.
  Right column: variant cards for the active type.

| Type | Variants |
| --- | --- |
| Текст | paragraph, bullet list, ordered list, quote, code |
| Заголовок | H1, H2, H3 |
| Изображение | empty placeholder (media picked from the block, as today) |
| Кнопка | button |
| Колонки | 2, 3, 4 |
| Разделитель | divider |
| HTML | raw HTML block |

- Card previews are small static JSX/CSS mocks, with no image assets.
- Click inserts the block **after the selected top-level block**, or at the end
  if nothing is selected. The new block becomes selected and scrolled into view,
  and the library closes. Insertion into columns stays with the slash menu and
  Layers drag.
- **One block catalog** (`blockCatalog.ts`: key, type, content builder; labels
  from i18n) feeds both the library and `SlashCommandMenu`. This also moves the
  slash-menu labels into i18n (en + ru). Slash items now insert the catalog
  content: on an empty line the result is the same as today; on a line with
  text, a new block is inserted instead of converting the line.
- Phase 2 adds a «Секции» group above «Основные»; phase 4 adds «Мои секции».

### Error handling

- Row load goes through `AsyncState`. Not found or forbidden → message + back
  link.
- Save errors → toast. `409 stale_row` keeps today's Reload / Force-overwrite
  flow, moved into a shared `useVisualSave` hook (removes the duplication).
  Force-overwrite re-sends the operator's document with a fresh token; today's
  fallback path refetches and would save the server's copy instead.
- Dirty = current doc + theme differ from the last saved snapshot.
- Preview render failure → error inside the preview dialog; the editor keeps
  working.
- Direct visit to `/edit` without `campaigns:manage` → read-only.

## Components

```text
frontend/src/
├── routes/t/$slug/
│   ├── route.tsx / components/shell/app-shell.tsx   # EXTENDED — fullscreen via staticData
│   ├── templates/$id.tsx                            # CHANGED — inline editor → preview + «Открыть редактор»
│   ├── templates/$id_.edit.tsx                      # NEW
│   ├── campaigns/$id.tsx                            # CHANGED — same as templates
│   └── campaigns/$id_.edit.tsx                      # NEW
└── components/visual-editor/
    ├── EditorScreen.tsx                             # NEW — legacy conversion + session + layout; replaces ThreePaneEditor
    ├── ThreePaneEditor.tsx (+ tests, stories)       # DELETED
    ├── VisualBodyPreview.tsx                        # NEW — saved body_html iframe for detail pages
    ├── VisualEmailEditor.tsx                        # CHANGED — unused toolbar/ThemeControls props removed
    ├── blockCatalog.ts                              # NEW — shared insertables + insertBlock()
    ├── extensions/Columns.tsx                       # CHANGED — setColumnsCount()
    ├── hooks/useBlockSelection.ts                   # CHANGED — starts empty, clear() sticks
    ├── hooks/useVisualSave.ts                       # NEW — save, stale_row, dirty snapshot
    ├── ui/EditorTopBar.tsx                          # NEW
    ├── ui/BlockToolbar.tsx                          # NEW — floating label under the selected block
    ├── ui/SlashCommandMenu.tsx                      # CHANGED — items from blockCatalog
    ├── panels/BlockLibrary.tsx                      # NEW
    ├── panels/StructureOutline.tsx                  # CHANGED — root node, ⋮ menu, exported helpers
    ├── panels/BlockParamsPanel.tsx                  # CHANGED — Content/Design tabs; root slot → ThemeControls
    ├── panels/params/*                              # CHANGED — tab split; RawHtmlParams NEW
    ├── panels/params/StyleControls.tsx              # CHANGED — grouped layout, switches
    └── visual-editor.css                            # CHANGED — screen, canvas, outlines, toolbar
locales/{en,ru}/visualEditor.json                    # EXTENDED
```

## Testing

Vitest + Storybook. No Go, BFF, or render tests change, because those layers do
not change.

- `EditorScreen`: load, save, ⌘S, unsaved-changes guard, read-only, `409` flow.
- `BlockLibrary`: tree and cards, insert after selection, insert at end, opening
  from the empty state.
- `BlockParamsPanel`: tab visibility per block type; root shows theme controls.
- `StructureOutline`: root node, ⋮ menu duplicate/delete.
- `BlockToolbar`: duplicate and delete.
- `SlashCommandMenu`: labels come from i18n.
- Template/campaign route tests rewritten for the preview + «Открыть редактор»
  detail page; edit-route tests added.
- The existing i18n parity test covers new keys.
- A Storybook story per new component.
