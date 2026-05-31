# Feature Specification: Three-Pane Visual Editor — Structure Outline & Block Parameters

**Feature Branch**: `017-three-pane-visual-editor`

**Created**: 2026-05-31

**Status**: Draft

**Input**: User description: "I want make visual editor three pane. Side pannels shoul be collapsable. Left pannel shoud show the document structure. Right pane should be params editor so user can easyly fine tune params of the selected block — bg, corner radius, font params etc"

## Context

Phase 7 (feature 014) delivered the visual email editor as a single editing
surface: a block-based canvas with slash commands, drag handles, a floating
text-formatting bubble menu, media-library image insertion, merge-tag chips, a
desktop/mobile preview, and a global per-campaign/per-template theme. The
canvas is the editor's whole UI — to change anything about a block today the
operator either edits inline or uses the bubble menu, and the only styling that
can be tuned is the document-wide theme (text color, link color, button color,
font, container width). There is no way to see the document at a glance, and no
way to give an individual block its own background, spacing, corner radius, or
font treatment.

This feature reframes that single canvas as the **center pane of a three-pane
editor**. It adds a **left panel** that shows the document's block structure as
a navigable outline, and a **right panel** that is a **parameters editor** for
whichever block is currently selected — letting an operator fine-tune a single
block's appearance (background, corner radius, padding, border, font family /
size / weight / color, alignment, spacing, and the type-specific knobs each
block exposes) without touching the rest of the document. Both side panels are
**collapsible** so the operator can reclaim the full width for the canvas when
they only want to write.

This is a UI/authoring-experience layer on top of the existing editor. It
introduces no new send path: per-block parameters are persisted on the same
structured block document, rendered to email-ready HTML and plain text by the
same server-side render-at-save flow (FR-013a/FR-013b of 014), and sent through
the same Phase 3 pipeline. The set of email-client-safe styling primitives the
parameters editor may expose is bounded by the same client-compatibility
constraints as 014 (Gmail, Apple Mail, Outlook desktop/web, mobile webviews;
table-based layout, no constructs that break in Outlook).

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Fine-tune a selected block's appearance in the parameters panel (Priority: P1)

A tenant operator is authoring a campaign in the visual editor. They click a
button block on the canvas. A parameters panel on the right immediately shows
the controls relevant to that block: background color, text color, corner
radius, padding, font family / size / weight, and alignment. They pick a new
background color from a color control, increase the corner radius with a slider
or stepper, and bump the font weight to bold. The canvas updates live to show
the restyled button. They then click a section/container block and the panel
swaps to that block's controls (background, padding, border, corner radius,
content alignment). They save; the preview and the delivered email render the
button and section exactly as styled.

**Why this priority**: This is the headline value of the feature — "so user can
easily fine tune params of the selected block." Without a working parameters
panel that persists styling into the sent email, the rest (a structure tree, a
collapsible chrome) is just navigation around an editor that can't do anything
new. It is independently demonstrable end to end: select a block, change its
parameters, save, and confirm the inbox-rendered email reflects the change.

**Independent Test**: As an operator, open the campaign editor, select a button
block, change its background color, corner radius, and font weight in the
right-hand parameters panel, confirm the canvas updates live, save, send a test,
and confirm the delivered email renders the button with the new background,
corner radius, and font weight.

**Acceptance Scenarios**:

1. **Given** the visual editor with content, **When** the operator selects a
   block on the canvas, **Then** the right-hand parameters panel shows the
   editable parameters for that block's type and reflects the block's current
   values.
2. **Given** a block is selected and the parameters panel is showing its
   controls, **When** the operator changes a parameter (e.g. background color,
   corner radius, padding, font size/weight/family, text color, alignment),
   **Then** the change is applied to that block only and the canvas reflects it
   live without affecting other blocks.
3. **Given** a block is selected, **When** the operator selects a different
   block, **Then** the parameters panel swaps to the newly selected block's
   parameter set and current values, with each change applied as it is made.
4. **Given** no block is selected (e.g. the selection is cleared), **When** the
   operator looks at the parameters panel, **Then** it shows an empty/neutral
   state prompting them to select a block, rather than stale controls.
5. **Given** the operator changed one or more block parameters, **When** they
   save and reopen the campaign/template, **Then** every block reappears with
   the parameters they set, intact.
6. **Given** the operator changed block parameters, **When** they save and send
   (or preview), **Then** the produced email renders those parameters in the
   major email clients within the limits of each client, using
   email-client-safe styling (no constructs that break Outlook).
7. **Given** a parameter that accepts a constrained value (e.g. a color, a
   bounded numeric like corner radius or padding), **When** the operator
   interacts with its control, **Then** the control only permits valid,
   email-safe values (no free-form entry that could produce invalid or
   unsupported output).

---

### User Story 2 - Navigate and understand the document via the structure outline (Priority: P2)

The same operator is working on a long campaign with nested multi-column
layouts. They open the left panel and see the document as an indented outline:
each block listed by its type and a short label (e.g. heading text, button
label, image alt), with multi-column blocks expandable to reveal the blocks
inside each column. They click an entry deep inside a three-column layout; the
canvas scrolls to that block and selects it, and the parameters panel loads its
controls. They drag an outline entry to a new position to reorder it, and use a
per-entry action to duplicate or delete a block. The currently selected block is
highlighted in the outline at all times, so the three panes always agree on
"what is selected."

**Why this priority**: A structure outline makes a non-trivial document
navigable and is the second half of the user's explicit request ("left pannel
shoud show the document structure"). It is valuable but secondary to actually
being able to change block parameters — an operator can still select blocks by
clicking the canvas without the outline. It is independently demonstrable: build
a nested document, open the outline, click a deep entry, and confirm the canvas
selects the corresponding block.

**Independent Test**: As an operator, build a campaign with a three-column
layout containing nested blocks, open the left structure panel, confirm the
outline mirrors the document hierarchy, click a nested entry and confirm the
canvas scrolls to and selects that block, then reorder and delete a block from
the outline and confirm the canvas and produced output reflect the change.

**Acceptance Scenarios**:

1. **Given** a document with several blocks including a multi-column layout,
   **When** the operator opens the left structure panel, **Then** it shows every
   block as an indented outline entry that mirrors the document's nesting, with
   each entry labelled by block type and a short content-derived label.
2. **Given** the structure outline, **When** the operator clicks an outline
   entry, **Then** the corresponding block is selected, the canvas scrolls it
   into view, and the parameters panel loads that block's controls.
3. **Given** a block is selected (by any means — canvas click or outline click),
   **When** the operator looks at the structure outline, **Then** the matching
   outline entry is highlighted as selected, keeping all three panes in sync.
4. **Given** the structure outline, **When** the operator drags an outline entry
   to a new valid position, **Then** the block (with its nested children, for
   multi-column blocks) is moved in the document, matching the behavior of the
   canvas drag handle.
5. **Given** an outline entry, **When** the operator invokes its delete or
   duplicate action, **Then** the block is removed or duplicated in the document
   and the canvas, outline, and produced output reflect the change.
6. **Given** a deeply nested or long document, **When** the operator collapses a
   container entry in the outline, **Then** its child entries are hidden so the
   operator can focus on the top-level structure.

---

### User Story 3 - Collapse and restore the side panels to control the workspace (Priority: P2)

The operator wants maximum room to write, so they collapse the left structure
panel and the right parameters panel; the canvas expands to fill the freed
space. When they want to restyle a block, they expand the right panel again. The
editor remembers their panel layout so that the next time they open the editor
the panels are in the state they left them. On a narrow viewport the editor
keeps the canvas usable rather than crushing three panes into unusable widths.

**Why this priority**: Collapsibility is explicitly requested and protects the
core writing experience on smaller screens, but it is a refinement of the
layout rather than a new capability — the editor is usable with both panels
shown. It is independently demonstrable: collapse each panel, confirm the canvas
reflows, reload, and confirm the layout is remembered.

**Independent Test**: As an operator, collapse the left panel and confirm the
canvas widens; collapse the right panel and confirm the same; expand both;
reload the editor and confirm the panels return to the last-used collapsed/
expanded state; shrink the viewport and confirm the canvas stays usable.

**Acceptance Scenarios**:

1. **Given** the three-pane editor, **When** the operator collapses the left
   structure panel, **Then** it hides (leaving an affordance to re-expand) and
   the canvas reflows to use the freed width.
2. **Given** the three-pane editor, **When** the operator collapses the right
   parameters panel, **Then** it hides (leaving an affordance to re-expand) and
   the canvas reflows to use the freed width.
3. **Given** a collapsed panel, **When** the operator activates its re-expand
   affordance, **Then** the panel reappears at its prior width with its content
   intact.
4. **Given** the operator has set a particular collapsed/expanded layout, **When**
   they leave and reopen the editor, **Then** the panels are restored to that
   layout.
5. **Given** a viewport too narrow to show three usable panes side by side,
   **When** the editor renders, **Then** it keeps the canvas usable (e.g. side
   panels collapse by default or overlay the canvas) rather than rendering three
   unusably-narrow columns.

---

### Edge Cases

- **No block selected / selection lost**: After deleting the selected block, or
  when the document is empty, the parameters panel shows its empty state and the
  outline shows the (possibly empty) document without a highlighted entry.
- **Block type with few or no styling params**: Some blocks (e.g. a divider, a
  raw-HTML block) expose a small or different parameter set; the panel shows
  only the parameters that apply to that type, never a generic dump of
  irrelevant controls. Raw-HTML blocks expose minimal/no style params and direct
  the operator to "edit as HTML."
- **Selection inside a nested column**: Selecting a block inside a multi-column
  layout selects exactly that nested block (not its container) across all three
  panes; the outline highlights the nested entry.
- **Reorder to an invalid target**: Dragging an outline entry to a position that
  would create an invalid structure (e.g. dropping a column container inside its
  own column) is rejected and the document is left unchanged.
- **Parameter that the email render cannot honor**: If a chosen parameter cannot
  render identically in a given client (a known email-client limitation), the
  editor still saves a valid email-safe approximation and does not emit unsafe
  markup; this is surfaced consistently with 014's sanitizer-warning behavior.
- **Concurrent edit / stale row**: Saving parameter changes is subject to the
  same optimistic-concurrency conflict handling as 014 (`409 stale_row` →
  reload / force-overwrite).
- **Existing raw-HTML / code-only campaigns**: Campaigns and templates that open
  in code-only mode (pre-Phase-7 content) do not show the three-pane visual
  chrome; the three-pane layout is part of the visual editing surface only.
- **Pre-017 visual documents**: Visual documents authored before this feature
  (with no per-block parameters set) open with each block at its theme/default
  appearance; absence of parameters is valid and renders exactly as it does
  today.
- **Very deep / very large document**: The structure outline and panels remain
  responsive and scrollable for documents with many blocks and deep nesting.

## Requirements *(mandatory)*

### Functional Requirements

#### Three-pane layout & selection model

- **FR-001**: The visual editor MUST present a three-pane layout — a left
  structure panel, the existing block-editing canvas in the center, and a right
  parameters panel — within the existing campaign and template editors, without
  introducing a separate app or a second send path.
- **FR-002**: The three panes MUST share a single block-selection state: at most
  one block is the "selected" block at a time, and selecting a block by any means
  (clicking it on the canvas or clicking its entry in the structure outline) MUST
  update the canvas highlight, the outline highlight, and the parameters panel
  together so all three panes agree on what is selected.
- **FR-003**: The three-pane chrome MUST be shown only for the visual editing
  surface; campaigns/templates in code-only mode (including pre-Phase-7
  raw-HTML content per 014 FR-029/FR-030) MUST NOT show the structure or
  parameters panels.
- **FR-004**: The three-pane layout MUST follow the navigation, loading, empty,
  and error-state conventions of the Phase 1–6 UI and MUST be fully localized via
  the app i18n mechanism (feature 015), with no hard-coded user-facing strings.

#### Left panel — document structure outline

- **FR-005**: The left panel MUST render the document as an indented outline that
  mirrors the block hierarchy, including the blocks nested inside each column of a
  multi-column layout.
- **FR-006**: Each outline entry MUST be labelled by its block type and a short,
  content-derived label (e.g. heading/paragraph text excerpt, button label, image
  alt text), so the operator can identify blocks without reading the canvas.
- **FR-007**: Clicking an outline entry MUST select the corresponding block,
  scroll it into view on the canvas, and load its parameters into the right panel.
- **FR-008**: The currently selected block MUST be visually highlighted in the
  outline regardless of how it was selected.
- **FR-009**: The operator MUST be able to reorder blocks by dragging outline
  entries, producing the same document result as the canvas drag handle (014
  FR-004), including moving a multi-column block with its nested children intact;
  reorders that would produce an invalid structure MUST be rejected without
  changing the document.
- **FR-010**: The outline MUST provide per-entry delete and duplicate actions
  whose effects are reflected on the canvas, in the outline, and in the produced
  output.
- **FR-011**: Container entries (multi-column layouts and their columns) MUST be
  collapsible/expandable in the outline so the operator can hide nested detail.

#### Right panel — block parameters editor

- **FR-012**: When a block is selected, the right panel MUST present exactly the
  set of editable parameters that apply to that block's type, populated with the
  block's current values; when no block is selected it MUST show a neutral
  empty/prompt state rather than stale controls.
- **FR-013**: The parameters editor MUST expose, where applicable to the block
  type, at minimum: background color, text/foreground color, corner radius,
  padding/inner spacing, border (width/style/color), content alignment, and font
  parameters (font family, size, weight, line-height) — plus the type-specific
  parameters each block already carries (e.g. button label and link, image width
  and alt and link, heading level, divider thickness/color, column widths/gap).
- **FR-014**: Changing a parameter MUST apply only to the selected block (and,
  where the parameter is a container property such as background or padding, only
  to that container), MUST update the canvas live, and MUST NOT alter the
  document-wide theme (014 FR-022–FR-024) or other blocks.
- **FR-015**: Each parameter control MUST constrain input to valid,
  email-client-safe values (e.g. color pickers, bounded numeric steppers/sliders
  for radius/padding/size, a curated font list), so the parameters editor cannot
  produce values that fail validation or break rendering in major clients.
- **FR-016**: The set of parameters offered MUST be limited to styling that
  renders reliably across the major email clients targeted by 014 (Gmail web/
  mobile, Apple Mail desktop/iOS, Outlook desktop/web); parameters MUST NOT
  introduce markup that 014's sanitizer would strip or that breaks Outlook (e.g.
  no CSS-grid-only constructs).
- **FR-017**: Per-block parameters MUST be persisted on the same structured block
  document as part of the existing explicit-save flow (014 FR-008/FR-013a) — they
  are block attributes, not a separate store — and MUST round-trip losslessly:
  reopening the campaign/template MUST restore every block's parameters.
- **FR-018**: Per-block parameters MUST be honored by the existing server-side
  render-at-save flow (014 FR-013b) so the rendered email-ready HTML and
  plain-text reflect the parameters; this feature MUST NOT add a client-side
  canonical-HTML render path and MUST NOT change the Phase 3 send pipeline.
- **FR-019**: The operator MUST be able to reset a block's parameters back to the
  theme/default appearance, distinguishing "explicitly set on this block" from
  "inherited from theme/default."

#### Collapsible panels & layout persistence

- **FR-020**: Each side panel (left structure, right parameters) MUST be
  independently collapsible and re-expandable, leaving a clear affordance to
  re-expand a collapsed panel; collapsing a panel MUST reflow the canvas to use
  the freed width.
- **FR-021**: The editor MUST remember each operator's panel layout (which panels
  are collapsed/expanded, and panel widths if resizable) and restore it the next
  time that operator opens the editor.
- **FR-022**: On viewports too narrow to show three usable panes side by side,
  the editor MUST keep the editing canvas usable (e.g. collapse side panels by
  default and/or present them as overlays) rather than rendering three
  unusably-narrow columns.

#### Permissions & consistency

- **FR-023**: Access to the three-pane editor and its panels MUST follow the
  existing campaign and template permission gating (014 FR-034); operators
  without those permissions MUST NOT see the editor or its panels.
- **FR-024**: Every failed operation surfaced through the new panels (e.g.
  applying a parameter, reordering or deleting via the outline, saving parameter
  changes) MUST surface a clear, named reason consistent with Phase 1–6 UI
  conventions rather than failing silently.

### Key Entities *(include if feature involves data)*

- **Block parameters (new on existing blocks)**: The per-block set of styling
  attributes the right panel edits — background color, foreground/text color,
  corner radius, padding/inner spacing, border, alignment, and font parameters
  (family, size, weight, line-height) — plus each block type's pre-existing
  type-specific attributes. Stored as attributes on the block within the existing
  structured block document (014's "Block" / "Visual document" entities); absent
  values mean "inherit theme/default." Consumed by the server-side render to
  produce the email-ready HTML/plain-text.
- **Block selection (editor state)**: The single currently-selected block,
  shared across the canvas, the structure outline, and the parameters panel.
  Editor session state, not persisted with the document.
- **Document structure outline (as shown)**: The derived, navigable tree view of
  the structured block document shown in the left panel — one entry per block,
  nested to match column containers, labelled by type and a content-derived
  label. A projection of the existing document, not a separate stored artifact.
- **Panel layout preference (new, per operator)**: The operator's remembered
  three-pane layout — which side panels are collapsed/expanded and their widths.
  Scoped to the operator (and the editor), not to a campaign/template, and not
  part of the sent content.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: An operator can change a selected block's background color, corner
  radius, and font weight and see the canvas reflect each change within 1 second,
  without affecting any other block.
- **SC-002**: 100% of per-block parameters set in the editor are present and
  identical after saving and reopening the campaign/template (lossless
  round-trip).
- **SC-003**: A block restyled via the parameters panel renders with the set
  background, corner radius, spacing, and font treatment in the in-editor
  preview and in the delivered email across the major targeted clients, within
  each client's documented limits.
- **SC-004**: In a document of at least 30 blocks including nested multi-column
  layouts, an operator can locate and select any block via the structure outline
  in under 10 seconds, and the canvas, outline, and parameters panel always agree
  on the selected block (zero desync).
- **SC-005**: Collapsing either side panel widens the canvas, and the operator's
  collapsed/expanded layout is restored on the next editor open in 100% of
  sessions.
- **SC-006**: On a viewport of 1024 px width or less, the editing canvas remains
  usable (no horizontal overflow of the canvas, no panel narrower than its
  minimum usable width) with the side panels collapsed or overlaid.
- **SC-007**: No parameter value set through the parameters editor produces
  output that 014's sanitizer strips or that fails server-side validation
  (0 sanitizer-stripped warnings attributable to parameter-editor controls).
- **SC-008**: At least 80% of operators complete a "restyle this button" task on
  the first attempt without assistance, and report it is easier to fine-tune a
  block's appearance with the parameters panel than with the pre-017 editor.

## Assumptions

- **Builds on feature 014**: This feature assumes the Phase 7 visual editor
  (014) is in place — the block-based canvas, the structured block document as
  the source of truth, the server-side render-at-save flow through the BFF, the
  document-wide theme, and the existing block types (paragraph, heading, list,
  quote, code, image, button, divider, multi-column, raw-HTML, merge-tag). This
  feature adds the surrounding three-pane chrome and per-block parameters; it
  does not re-specify the canvas's editing behavior.
- **Per-block parameters extend the existing document and output**: Because
  "fine tune params of the selected block" is only meaningful if it changes the
  delivered email, per-block parameters are persisted on the structured block
  document and rendered into the email-ready HTML/plain-text by the same
  server-side render-at-save path — not as editor-only cosmetics. No new send
  path or client-side canonical render is introduced.
- **Email-client safety bounds the parameter set**: The exact list of exposed
  parameters per block type will be finalized in planning, constrained to
  styling that renders reliably across the clients 014 targets; controls
  constrain values so unsafe/unsupported output cannot be produced.
- **Single-block selection**: The parameters panel edits one selected block at a
  time. Multi-select (editing several blocks at once) is out of scope for this
  feature.
- **Structure outline is a projection**: The left panel is derived live from the
  structured block document; it is not a separately-stored artifact and does not
  change the persistence model beyond the per-block parameters above.
- **Panel layout is an operator preference**: Collapsed/expanded state (and panel
  widths, if resizable) are remembered per operator for the editor as a whole,
  not per campaign/template, and are not part of the sent content.
- **Save semantics unchanged**: Persistence remains explicit-save with the same
  optimistic-concurrency conflict handling (`ifUnmodifiedSince` / `409
  stale_row`) as 014; autosave remains out of scope (deferred per 014).
- **Permissions, i18n, and UI conventions inherited**: Access gating reuses 014's
  campaign/template permissions; all new UI strings are localized via feature
  015; loading/empty/error states follow Phase 1–6 conventions.

## Dependencies

- **Feature 014 — Visual Email Editor**: provides the canvas, block document,
  block types, server-side render-at-save flow, theme, and preview that this
  feature wraps and extends.
- **Feature 015 — App i18n / language switcher**: provides the localization
  mechanism all new panel UI strings must use.
- **Phase 6 branding / theme (via 014)**: supplies the theme defaults that
  unset per-block parameters inherit.
