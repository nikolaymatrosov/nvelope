# Feature Specification: Phase 7 — Visual Email Editor

**Feature Branch**: `014-visual-email-editor`

**Created**: 2026-05-20

**Status**: Draft

**Input**: User description: "Next big thing I want to add is Visual email editor. I found React Email Editor — an embeddable visual editor built on TipTap/ProseMirror that produces email-ready HTML, with rich text editing, bubble menus, slash commands, multi-column layouts, email theming, image upload, HTML export, and custom extensions. Also explore TipTap's Notion-like editor with drag handles on the left of the content for dragging and adding components."

## Clarifications

### Session 2026-05-20

- Q: How are visually-authored campaigns/templates stored? → A: Store the structured block document (JSON) and the rendered HTML and plain-text side by side. The structured document is the editor's source of truth; the rendered HTML and plain-text are what the send pipeline consumes — no new send path.
- Q: When does the structured document become HTML/plain-text? → A: At save time, server-side. The frontend POSTs only the structured document; the API renders, sanitizes, and persists all three pieces atomically. The send pipeline does no rendering.
- Q: What is the personalization placeholder syntax? → A: Namespaced Liquid-ish double-curly tags — `{{ subscriber.<field> }}` for subscriber profile and custom fields, `{{ campaign.<name> }}` for campaign-level values (e.g. `{{ campaign.unsubscribe_url }}`, `{{ campaign.preference_url }}`). The visual editor preserves these verbatim through save → reload; substitution happens server-side at send time per recipient.
- Q: Where do the available merge-tag names come from? → A: Introduce a new tenant-scoped subscriber custom-field registry. Each definition has a `slug` (used in `{{ subscriber.<slug> }}`), `display_name`, and a `type` (text, number, date, boolean, url). Built-in subscriber fields (email, name/first_name/last_name, state) are surfaced as pseudo-rows so the picker treats them uniformly. Phase 6 subscription-page "visible profile fields" and Phase 7 merge-tag picker both read from the same registry. The send-time renderer validates every placeholder against the registry and surfaces a clear save-time error for unknown slugs. The Phase 2 free-form `attributes` map on subscribers stays as-is for backwards compatibility.
- Q: How are placeholders displayed and inserted in the visual editor? → A: As chips/pills inserted from a picker (bubble-menu button + slash-command entry). The picker lists every entry in the tenant's subscriber custom-field registry (including built-in pseudo-rows) and every known campaign-level value. Chips render styled in the editor with the display name (e.g. "First name") and the raw tag visible on hover; on save and code-view they serialize to the literal `{{ subscriber.<slug> }}` / `{{ campaign.<name> }}` placeholder. Operators may still type raw braces in code view.

## User Scenarios & Testing *(mandatory)*

<!--
  Phases 1–6 deliver the platform's backend and most of its workspace UI:
  tenants, subscribers, lists, sending domains, templates, campaigns, the send
  pipeline, deliverability, analytics, billing, public subscription/preference
  pages, branding, the campaign archive and RSS feed, and a tenant media
  library. The Phase 3 UI (007) explicitly defers WYSIWYG to Phase 7:
  "A visual / WYSIWYG email editor is out of scope (Phase 7); template and
  campaign content is edited as plain HTML and/or text in a basic editor."
  This feature delivers that Phase 7 editor.

  Today, an operator authoring a campaign or template types or pastes raw
  HTML/plain text into a basic textarea. There is no preview of how the
  message will render in a real client, no block-level layout (columns,
  buttons, dividers), and no integration with the tenant media library other
  than copy-pasting URLs by hand. Authors who are not comfortable with HTML
  cannot produce a good-looking email without an outside tool, and authors
  who are comfortable still hit the same email-client quirks every time.

  Phase 7 introduces a visual, block-based editor that replaces (or augments)
  the plain content field in the existing campaign and template editors. It
  is a single embedded surface inside the workspace — there is no separate
  app. It produces email-ready HTML (table-based, inlined styles where
  required, compatible with major clients including Gmail, Apple Mail,
  Outlook, mobile webviews) and a plain-text alternative for the same
  content. The editor uses bubble menus for text formatting and slash
  commands plus drag handles for inserting and rearranging blocks. It
  integrates with the Phase 6 media library for image insertion and with the
  Phase 6 tenant branding for default theming. Campaigns and templates
  authored visually are sent through exactly the same Phase 3 pipeline as
  HTML-authored ones — the editor's only contract with the rest of the
  platform is the HTML and plain-text it emits.

  Transactional templates are out of scope as primary editing targets in this
  phase: they are typically authored by developers and may contain
  templating that the visual editor would not preserve. Campaign templates
  and campaigns themselves are the targets.

  Backwards-compatibility constraints: existing campaigns and templates
  authored as raw HTML/text in the Phase 1–6 UI MUST continue to send and
  open in a fall-back code-editing mode; they are not silently rewritten by
  the new editor.
-->

### User Story 1 - Author a campaign visually with blocks, formatting, and images (Priority: P1)

A tenant operator opens the campaign editor for a new campaign and lands directly in the visual editor. They start typing in the content area; the text is styled. They type `/` and a slash-command menu appears with insertable blocks (heading, paragraph, image, button, divider, two-column layout, three-column layout, etc.); they pick "two-column layout" and the editor inserts a two-column block they can fill in. They select a phrase of text and a floating bubble menu appears with bold, italic, link, and color controls. To the left of each block, a drag handle lets them grab a block and reorder it; the same handle exposes a quick-add button that opens the same block menu. They insert an image by clicking the image block and picking an asset from the tenant media library; the image renders in place. They preview the campaign in desktop and mobile widths, save, and start the send. The resulting email arrives in their inbox with the same layout, images, and styling visible in the preview.

**Why this priority**: This is the heart of the feature — without visual authoring producing a sendable, well-rendered email, none of the other stories matter. It is independently demonstrable end to end: open the campaign editor, author content visually, send a test, confirm the rendered email matches the preview.

**Independent Test**: As an operator, open the campaign editor, author a campaign using paragraphs, a heading, a two-column layout, a button, and an image picked from the media library, preview it in desktop and mobile widths, send it to a test address, and confirm the inbox-rendered email matches the preview in layout, image, button, and styling.

**Acceptance Scenarios**:

1. **Given** the campaign editor is open for a new campaign, **When** the operator focuses the content area, **Then** the visual editor is the default surface and accepts typing immediately without any extra setup step.
2. **Given** the visual editor, **When** the operator types `/`, **Then** a slash-command menu appears listing insertable blocks (at minimum: heading, paragraph, image, button, divider, bulleted list, numbered list, quote, code, two-column, three-column, four-column).
3. **Given** the operator hovers over or focuses a block, **When** the drag handle to the left of the block is visible, **Then** they can drag the block to a new position and the editor moves the block (including its nested children for multi-column blocks) intact.
4. **Given** the drag handle is visible on a block, **When** the operator clicks the quick-add control on it, **Then** the same insertable-block menu appears and the chosen block is inserted relative to that block.
5. **Given** the operator selects a range of text, **When** the bubble menu appears, **Then** it offers at least bold, italic, link, text color, and (where the selection is inside a heading) heading-level controls; applying any of them updates the selection in place.
6. **Given** the operator inserts an image block, **When** they choose "from media library", **Then** the tenant media library picker (from Phase 6) opens and a chosen asset is inserted into the block as an inline image.
7. **Given** the visual editor, **When** the operator inserts a multi-column block (two, three, or four columns), **Then** each column is independently editable and accepts the same set of block types as the top level (text, images, buttons, etc.).
8. **Given** the visual editor, **When** the operator opens the preview, **Then** they can switch between a desktop width and a mobile width preview and the layout adapts accordingly.
9. **Given** a campaign authored entirely in the visual editor, **When** the operator starts the send, **Then** the recipient inbox renders the same blocks, images, columns, and styling as the preview in major email clients (Gmail web/app, Apple Mail desktop/iOS, Outlook desktop/web).
10. **Given** the visual editor, **When** the operator saves the campaign as a draft and reopens it, **Then** the content reappears with the same blocks, formatting, and nested layouts intact.

---

### User Story 2 - Author and reuse campaign templates visually (Priority: P1)

A tenant operator opens the templates area and creates a new campaign template using the same visual editor as Story 1. They build a reusable layout (header with logo, two-column hero, body, footer block) and save the template. Later, when authoring a campaign, they pick that template as the starting point; the campaign editor opens pre-filled with the template's blocks, which they edit visually before sending. Existing campaign templates that were authored as raw HTML before this phase MUST keep working as starting points and MUST open in a fall-back code editor rather than being silently parsed into blocks.

**Why this priority**: Reusable templates are how operators get leverage out of the editor — without template authoring, every campaign starts from scratch. Templates were established as a P2 capability in Phase 3 UI (007); making them visual is a P1 of this phase because the editor's value compounds through them. It is independently demonstrable: create a visual template, then pick it when authoring a campaign and confirm the campaign editor opens pre-filled with the template's blocks.

**Independent Test**: As an operator, create a campaign template in the visual editor with a multi-block layout (logo, hero, two-column body, footer), save it, then open the campaign editor, pick that template as the starting point, and confirm the campaign editor is pre-filled with the template's blocks and remains fully editable. Separately, take an existing raw-HTML campaign template from before this phase, open it, and confirm it opens in the fall-back code editor without being rewritten.

**Acceptance Scenarios**:

1. **Given** the templates area, **When** the operator creates a new campaign template, **Then** the visual editor is the default authoring surface.
2. **Given** a saved visual campaign template, **When** the operator opens the campaign editor and picks that template as the starting point, **Then** the campaign editor is pre-filled with the same blocks and remains fully editable.
3. **Given** a campaign template authored as raw HTML before this phase, **When** the operator opens it for editing, **Then** it opens in the fall-back code editor (not the visual editor) and is not silently parsed or rewritten.
4. **Given** a campaign template authored as raw HTML before this phase, **When** the operator picks it as a starting point for a campaign, **Then** the campaign editor opens in the fall-back code editor with that content pre-filled, and the operator may opt in to "convert to visual editor" with an explicit warning that the conversion is best-effort.
5. **Given** the operator opts in to converting a raw-HTML template to the visual editor, **When** the conversion runs, **Then** content that cannot be represented as blocks is preserved in a "raw HTML" block rather than silently dropped.
6. **Given** transactional templates exist (Phase 3 UI), **When** the operator opens one for editing, **Then** the basic/code editor remains the authoring surface and the visual editor is not the default for transactional templates.

---

### User Story 3 - Match a campaign's look to the tenant's brand (Priority: P2)

A tenant administrator has already configured branding in Phase 6 (logo, primary/secondary colors). When an operator opens the visual editor for a campaign or template, the editor's default theme — default text color, link color, button color, font, container width — derives from the tenant branding so a freshly inserted block looks like it belongs to the brand without any extra step. The operator may override the theme per campaign/template (pick different colors, fonts, container width) inside the editor; the override applies only to that campaign or template and does not change the tenant branding. The recipient inbox renders the same colors and fonts as the in-editor preview.

**Why this priority**: Themed defaults make the editor immediately useful for non-designers and remove the most common cause of off-brand campaigns. It is independent of the core authoring flow in Story 1 (the editor still works without themed defaults) so it sits at P2. It is independently demonstrable: change the tenant brand color in Phase 6, open the visual editor on a fresh campaign, and confirm that a newly inserted button or heading uses the brand color by default.

**Independent Test**: As an administrator, set a distinctive brand primary color in the Phase 6 branding area. As an operator, open the visual editor on a fresh campaign; insert a button and a heading; confirm both use the brand primary color by default without any per-campaign override. Override the campaign's theme to a different button color, save, send a test, and confirm the recipient inbox shows the overridden color, not the tenant default.

**Acceptance Scenarios**:

1. **Given** the tenant has Phase 6 branding configured (logo, primary/secondary colors), **When** an operator opens the visual editor on a new campaign or template, **Then** the editor's defaults (text color, link color, button color, font, container width) are derived from the tenant branding.
2. **Given** the visual editor is open, **When** the operator opens the theme controls, **Then** they can override the editor's default colors, fonts, and container width per campaign/template, and the override is shown in the editor and the preview.
3. **Given** the operator overrides the theme of a single campaign, **When** they save and reopen any other campaign or template, **Then** the override is scoped to the first campaign and other campaigns/templates still use the tenant's branding defaults.
4. **Given** a themed campaign, **When** it is sent and opened in a recipient inbox, **Then** the colors, fonts, and container width match the in-editor preview in major email clients.
5. **Given** the tenant branding is later changed in Phase 6, **When** an existing campaign that used the defaults (no overrides) is reopened, **Then** the editor's defaults reflect the new branding; existing campaigns with explicit overrides keep their overrides.

---

### User Story 4 - Switch between visual and code editing safely (Priority: P2)

An advanced operator authoring a campaign in the visual editor needs to tweak something the visual editor does not expose (a hand-written conditional, a specific HTML construct, a wrapper element). They open a "code" view that shows the email-ready HTML the editor would emit, edit it, and return to the visual editor; the visual editor either round-trips their change cleanly or marks the affected region as a "raw HTML" block that displays as opaque content in the visual surface but is preserved verbatim in the output. The operator can also abandon the visual editor entirely for a given campaign and continue in code-only mode; the campaign is sendable in either mode. Existing raw-HTML campaigns and templates from before this phase open in code-only mode by default and only switch to the visual editor on explicit opt-in.

**Why this priority**: Code escape-hatches are essential for the small fraction of authors who need them and for the migration story from raw HTML, but they are not what most authors will touch. They sit at P2 because Stories 1–3 deliver the primary value and this story enables advanced use and migration. It is independently demonstrable: open a visual campaign in code view, edit the HTML, return to the visual editor, and confirm the edit is preserved (either round-tripped or surfaced as a raw-HTML block).

**Independent Test**: As an operator, open a visual campaign, switch to code view, edit a section of the HTML (e.g. add a wrapping div with a class), return to the visual editor, and confirm the edit is preserved verbatim in the output. Separately, take a raw-HTML campaign from before this phase, open it, confirm it is in code-only mode by default, opt in to the visual editor, and confirm the conversion either succeeds or surfaces unconvertible regions as raw-HTML blocks rather than dropping them.

**Acceptance Scenarios**:

1. **Given** a campaign or template authored in the visual editor, **When** the operator opens code view, **Then** they see the email-ready HTML the editor would emit, in a code editor with syntax highlighting.
2. **Given** the operator edits the HTML in code view, **When** they return to the visual editor, **Then** the edit is preserved — either round-tripped into the corresponding blocks, or surfaced as a raw-HTML block that the visual surface displays as opaque content.
3. **Given** a campaign that contains a raw-HTML block, **When** the operator opens it, **Then** the visual surface clearly marks the raw-HTML region (with a label and a "edit as HTML" affordance) rather than rendering it as if it were a block the editor created.
4. **Given** an operator wishes to author in code only for a given campaign, **When** they explicitly choose "edit as HTML only", **Then** the visual editor is dismissed for that campaign and code-only authoring is offered; the campaign remains sendable.
5. **Given** an existing raw-HTML campaign or template from before this phase, **When** the operator opens it, **Then** it opens in code-only mode by default and the visual editor is only offered behind an explicit "convert" action with a warning about best-effort conversion.
6. **Given** a campaign uses any campaign-level personalization syntax already supported by the Phase 3 send pipeline (e.g. a subscriber-field placeholder), **When** the operator inserts or edits it in the visual editor, **Then** the editor preserves the placeholder syntax verbatim through save → reload → send.

---

### User Story 5 - Insert and reuse images from the tenant media library (Priority: P2)

When an operator inserts an image block, the editor opens the Phase 6 tenant media library picker; the operator browses the existing library, picks an asset, and the image is inserted with its stable reference URL. If the operator drags an image file onto the editor or pastes one from the clipboard, the editor uploads it to the tenant media library (subject to the existing Phase 6 size and type limits) and inserts a reference to the new asset; the upload also appears as a new entry in the media library so it is reusable. The editor never inserts a data-URL image or a non-library URL into the produced HTML — every image in the output is a stable, tenant-scoped media library reference. If an image referenced from a campaign is later deleted from the media library, the campaign editor shows a placeholder with a clear "no longer available" message rather than a broken image, matching the Phase 6 behaviour.

**Why this priority**: Image insertion from the media library is part of the core authoring loop, but Story 1 already accepts library images via the picker; this story extends that to drag-and-drop and paste-to-upload and pins down the contract that the editor never emits non-library image URLs. It is independently demonstrable: drag an image onto the editor and confirm a new asset appears in the media library and is referenced by the campaign's output HTML.

**Independent Test**: As an operator, open the visual editor, drag a local image file onto the canvas, confirm the editor uploads it and inserts it in place, then open the Phase 6 media library and confirm the same asset appears as a new entry; reuse the same asset on a second campaign via the picker. Inspect the produced HTML and confirm every image src is a media-library reference (not a data URL, not an outside URL, not a relative path).

**Acceptance Scenarios**:

1. **Given** the visual editor with an image block, **When** the operator chooses "from media library", **Then** the Phase 6 tenant media library picker opens and a chosen asset is inserted as an image referencing its stable URL.
2. **Given** the visual editor, **When** the operator drags an image file from their desktop onto the canvas, **Then** the editor uploads the file to the tenant media library subject to Phase 6's size and type limits, inserts the image, and the new asset appears in the media library on the next view.
3. **Given** the visual editor, **When** the operator pastes an image from the clipboard, **Then** the same upload-and-insert behaviour as drag-and-drop applies.
4. **Given** an upload that exceeds the media library's size limit or uses a disallowed type, **When** the drag, paste, or picker upload is attempted, **Then** the editor surfaces a specific reason inline and does not insert the image; nothing is stored.
5. **Given** a campaign that references a media asset by URL, **When** the asset is later deleted from the media library, **Then** opening the campaign editor shows a "no longer available" placeholder for that image rather than a broken or empty image.
6. **Given** any campaign or template authored in the visual editor, **When** its produced HTML is inspected, **Then** every image source is a tenant-scoped media library reference — no data URLs, no third-party hotlinks the operator did not explicitly type as a code-view edit.

---

### Edge Cases

- The operator pastes a large block of rich content (a Word document, a Google Doc, a webpage) into the visual editor — the editor preserves headings, paragraphs, lists, links, and basic inline styling, and discards constructs that would not render in email clients rather than silently producing broken output.
- The operator drags a block off the end of the document or onto itself — the editor surfaces a clear drop indicator and either no-ops (drop on self) or appends (drop off the end) rather than throwing.
- The operator opens two browser tabs on the same campaign and edits both — the second save surfaces a clear "this campaign was changed in another tab" message rather than silently overwriting, consistent with the rest of the workspace UI.
- A campaign authored visually is opened by an operator on a smaller screen — the editor remains usable (drag handles, bubble menu, slash command all accessible) without the canvas becoming clipped.
- The operator inserts a multi-column block on a campaign that will be opened in Outlook desktop — the produced HTML uses a layout primitive that Outlook desktop renders correctly (e.g. table-based columns), not a CSS-grid construct.
- The slash command menu is opened mid-word — typing the rest of the word filters the menu; pressing escape dismisses it and leaves the typed `/` and following characters in place.
- The operator inserts an image, the upload is interrupted mid-flight — the placeholder in the editor surfaces a clear "upload failed, try again" affordance and does not persist a broken reference into the saved campaign.
- The editor session is left idle for a long time and the auth session expires — saving surfaces a clear "session expired, sign in again" message rather than silently losing the draft; on re-authentication the in-progress content is recoverable from the last autosave.
- An operator opens an existing visual campaign whose theme used to derive from tenant branding that has since changed — the editor shows the current resolved defaults and a small indicator that the campaign is using tenant defaults (not pinned), so the operator understands why colors look different.
- An operator round-trips a campaign through code view and applies HTML the visual editor cannot represent (e.g. a script tag) — the editor strips or refuses constructs that would not render in email clients (and warns the operator), rather than silently sending them in the campaign.
- An operator on a very long campaign opens the slash command menu — the menu remains anchored to the current cursor position and does not jump the page.
- The operator collapses or pastes content into a nested column — the column expands to fit and the surrounding row recomputes without overflowing the email's container width.
- The operator works in the editor offline (loss of network) — typing and reordering remain responsive locally; saves are queued and surface a clear "offline, saved when you reconnect" indicator rather than silently failing.
- The transactional-template editor (Phase 3 UI) is opened — it continues to use the basic/code editor and the visual editor is not surfaced as the default, consistent with the scoping in this phase.

## Requirements *(mandatory)*

### Functional Requirements

#### Visual authoring surface

- **FR-001**: The campaign editor MUST embed a visual editor as the default content-authoring surface for new campaigns and new campaign templates.
- **FR-002**: The visual editor MUST support, at minimum, the following block types: paragraph, heading (multiple levels), bulleted list, numbered list, quote, code block, link, image, button, divider, and multi-column layouts of two, three, and four columns whose individual columns can themselves contain any of the other block types.
- **FR-003**: The visual editor MUST present a slash-command menu when the operator types `/`, listing every insertable block type with a name, a short description, and a quick filter as the operator types.
- **FR-004**: The visual editor MUST present a per-block drag handle on hover/focus to the left of each block; the handle MUST let the operator drag the block to a new position and MUST expose a quick-add control that opens the same insertable-block menu.
- **FR-005**: The visual editor MUST present a floating ("bubble") formatting menu when the operator selects a range of text, offering at minimum bold, italic, link, text color, and (in headings) heading-level controls.
- **FR-006**: The visual editor MUST support keyboard editing for all block-level operations exposed via mouse (insert via slash command, reorder via keyboard shortcut alternative to drag, focus traversal across blocks and columns) so that the editor is usable without a pointer.
- **FR-007**: The visual editor MUST offer a desktop-width and a mobile-width preview that reflect how the produced HTML will render at those viewport widths.
- **FR-008**: The visual editor MUST autosave drafts on a regular cadence and MUST surface a clear "saved", "saving", and "saved locally, waiting for network" state.
- **FR-009**: The visual editor MUST surface a clear conflict message (rather than silent overwrite) when the same campaign or template is saved from another browser tab/session since the operator opened it.
- **FR-010**: The visual editor MUST allow paste from external rich-text sources (browser, document editor) and MUST preserve paragraphs, headings, lists, links, and basic inline styling while discarding constructs that would not render in email clients.

#### Email-ready output

- **FR-011**: The visual editor MUST produce email-ready HTML as the campaign's and template's content, compatible with major recipient clients including Gmail (web and mobile), Apple Mail (desktop and iOS), and Outlook (desktop and web).
- **FR-012**: The visual editor MUST also produce a plain-text alternative of the same content suitable for use as the text/plain part of the sent message.
- **FR-013**: The produced HTML MUST be the canonical content that the Phase 3 send pipeline uses; this feature MUST NOT introduce a second sending path.
- **FR-013a**: Campaigns and templates authored visually MUST be persisted as three side-by-side pieces of content: (1) the structured block document the editor manipulates (the editor's source of truth, reloaded losslessly on next open), (2) the rendered email-ready HTML, and (3) the rendered plain-text alternative. The Phase 3 send pipeline reads (2) and (3); the editor reads (1). Existing rows from before this phase that have no structured document are valid and continue to be served from (2)/(3) only.
- **FR-013b**: The render from structured document to email-ready HTML and plain-text MUST happen server-side at save time. The frontend MUST POST only the structured document on save; the API MUST render, sanitize, and persist all three pieces atomically. The send pipeline (`cmd/worker`) MUST NOT render visual documents — it consumes the stored HTML and plain-text as-is.
- **FR-014**: The produced HTML MUST never include a data-URL image, a script tag, or other constructs incompatible with email clients; the editor MUST strip or refuse such constructs (and warn the operator if they were introduced via paste or code view).
- **FR-015**: Multi-column blocks MUST render correctly in major recipient clients, including Outlook desktop (i.e. using a table-based or equivalent compatible layout primitive, not CSS grid).
- **FR-016**: The visual editor MUST support and preserve verbatim a namespaced double-curly placeholder syntax — `{{ subscriber.<slug> }}` for the subscriber's profile and custom fields (e.g. `{{ subscriber.first_name }}`, `{{ subscriber.email }}`) and `{{ campaign.<name> }}` for campaign-level values (e.g. `{{ campaign.unsubscribe_url }}`, `{{ campaign.preference_url }}`). Substitution is performed by the Phase 3 send pipeline server-side at send time per recipient; the editor MUST NOT substitute or evaluate placeholders at save or preview time except by rendering a clearly-labelled "sample data" preview.
- **FR-016a**: The platform MUST provide a tenant-scoped subscriber custom-field registry. Each definition carries a stable `slug` (used in `{{ subscriber.<slug> }}`), a `display_name`, a `type` (one of: text, number, date, boolean, url), and an optional default value. The registry is editable by operators with the existing audience-management permission, persisted on the tenant plane, and tenant-isolated under the same RLS rules as other Phase 2 audience data.
- **FR-016b**: The subscriber custom-field registry MUST surface built-in subscriber fields (at minimum: email, name/first_name/last_name, state) as pseudo-rows in the registry API so the visual editor's merge-tag picker and the Phase 6 subscription-page "visible profile fields" picker treat built-in and custom fields uniformly. Phase 6 subscription-page configuration (FR-024/FR-025 in 013) MUST read from the same registry — there is one canonical list of subscriber fields per tenant.
- **FR-016c**: On save of a visually-authored campaign or template (and on save of any code-only campaign or template whose body contains placeholders), the API MUST validate every `{{ subscriber.<slug> }}` placeholder against the registry; unknown slugs MUST cause the save to fail with a clear, named error that identifies the offending placeholder(s). Known campaign-level names are validated against a fixed allow-list maintained by the platform.
- **FR-016d**: The visual editor MUST present a merge-tag picker (accessible from a bubble-menu button and from a slash-command entry) whose contents are the union of (a) the tenant's subscriber custom-field registry — built-in pseudo-rows and tenant-defined custom fields — and (b) the platform's allow-list of campaign-level values. Selecting an entry inserts a chip rendered with the field's display name and the raw tag visible on hover; on save and in code view the chip serializes to the literal placeholder string (e.g. `{{ subscriber.first_name }}`). Operators MAY still type raw braces in code view; on returning to the visual surface, recognized raw placeholders MUST round-trip into chips.
- **FR-016e**: The Phase 2 free-form `attributes` map on subscribers (Phase 2 FR-010) MUST remain valid storage for backwards compatibility; attribute keys that have no corresponding registry slug are accepted on read/write but MUST NOT appear in the merge-tag picker. Deleting a registry definition MUST NOT delete underlying attribute data.

#### Tenant media-library integration

- **FR-017**: Inserting an image block MUST open the Phase 6 tenant media library picker; the picker's result MUST be inserted as an image referencing the asset's stable URL.
- **FR-018**: The visual editor MUST support drag-and-drop of an image file from the operator's desktop and paste of an image from the clipboard; in both cases the editor MUST upload the file to the tenant media library subject to Phase 6's size and type limits and insert a reference to the new asset.
- **FR-019**: An upload that violates the media library's size or type limit MUST be rejected up front with a specific reason and MUST NOT be persisted.
- **FR-020**: When an image referenced from a campaign or template is later deleted from the media library, the visual editor MUST show a "no longer available" placeholder rather than a broken image, consistent with Phase 6 behaviour.
- **FR-021**: Every image in the produced HTML MUST be a tenant-scoped media-library reference — the editor MUST NOT emit non-library URLs except where the operator explicitly typed them in code view.

#### Theming

- **FR-022**: The visual editor's default theme (text color, link color, button color, font, container width) MUST be derived from the tenant's Phase 6 branding when no per-campaign/per-template override exists.
- **FR-023**: The visual editor MUST let the operator override the theme per campaign or per template; overrides MUST be scoped to that campaign/template and MUST NOT change the tenant branding.
- **FR-024**: When the tenant branding changes, campaigns and templates that did not pin an override MUST reflect the new branding on next open; campaigns and templates with explicit overrides MUST keep their overrides.
- **FR-025**: The recipient inbox MUST render the same colors, fonts, and container width as the in-editor preview in major email clients.

#### Code view & migration

- **FR-026**: The visual editor MUST offer a "code" view that shows the email-ready HTML it would emit, in a code editor with syntax highlighting.
- **FR-027**: Edits made in code view MUST be preserved on return to the visual editor — either round-tripped into the corresponding blocks, or surfaced as a raw-HTML block in the visual surface that displays the region as opaque content while preserving it verbatim in the output.
- **FR-028**: The visual editor MUST clearly label raw-HTML blocks in the visual surface and MUST provide an "edit as HTML" affordance directly on them.
- **FR-029**: The visual editor MUST let the operator opt out of the visual editor for a given campaign or template ("edit as HTML only"); the campaign or template MUST remain sendable in that mode.
- **FR-030**: Existing campaigns and templates authored as raw HTML before this phase MUST open in code-only mode by default; the visual editor MUST only be offered behind an explicit "convert to visual editor" action with a clear warning that conversion is best-effort.
- **FR-031**: When raw HTML is converted to the visual editor, content that cannot be represented as blocks MUST be preserved in a raw-HTML block rather than silently dropped.

#### Scope, permissions, and consistency

- **FR-032**: The visual editor MUST be the default editing surface for campaigns and campaign templates; transactional templates (Phase 3 UI) MUST continue to use the basic/code editor and MUST NOT default to the visual editor.
- **FR-033**: The visual editor MUST live inside the existing campaign and template editors in the tenant workspace app shell and MUST follow the navigation, loading, empty, and error-state conventions established by the Phase 1–6 UI.
- **FR-034**: Access to the visual editor MUST follow the existing campaign and template permission gating from Phase 3 UI; operators without those permissions MUST NOT see the editor or its inputs.
- **FR-035**: Every failed operation in the editor (save, autosave, image upload, theme save, conversion from raw HTML) MUST surface a clear, named reason rather than a silent failure or a generic error, consistent with the Phase 1–6 UI conventions.

### Key Entities *(include if feature involves data)*

- **Visual document (as shown)**: The structured, block-based representation of a campaign's or template's content that the visual editor manipulates. Persisted as JSON alongside the rendered HTML and plain-text on the same campaign/template row; it is the editor's source of truth, the rendered HTML/plain-text are what the send pipeline consumes. Absent on rows authored before this phase.
- **Block (as shown)**: A unit of the visual document — paragraph, heading, image, button, divider, list item, quote, code, raw-HTML region, or a multi-column container holding nested blocks per column.
- **Editor theme (as shown)**: The colors, fonts, and container width used as the editor's defaults. Inherits from the tenant's Phase 6 branding unless explicitly overridden on a campaign or template.
- **Produced HTML (as shown)**: The email-ready, client-compatible HTML emitted from the visual document — the canonical content that the Phase 3 send pipeline sends. Plain-text alternative is produced alongside.
- **Raw-HTML block (as shown)**: A block whose content is verbatim HTML — used to host either pre-existing raw-HTML content during migration, or code-view edits the editor cannot round-trip into structured blocks. Rendered as opaque in the visual surface and preserved exactly in the produced HTML.
- **Media reference (as shown)**: An image inside the produced HTML whose source is a stable tenant-scoped media-library URL from Phase 6 — never a data URL, never an unmanaged outside URL inserted by the visual flows.
- **Subscriber custom-field registry (new in this phase)**: A tenant-scoped, ordered list of subscriber field definitions. Each definition has a stable `slug`, a `display_name`, a `type` (text, number, date, boolean, url), and an optional default. Built-in subscriber fields (email, name/first_name/last_name, state) are surfaced as pseudo-rows so the registry is the single source of truth for "which subscriber fields exist on this tenant". Read by the merge-tag picker (Phase 7), the subscription-page field picker (Phase 6), and the send-time renderer (Phase 3); editable by operators with the existing audience-management permission.
- **Merge tag chip (as shown)**: The editor-side representation of a placeholder. Carries the namespace (`subscriber` or `campaign`), the slug/name, and the display name. Renders as a styled inline token in the visual surface; serializes verbatim to `{{ <namespace>.<slug> }}` in the produced HTML and plain-text.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: An operator new to the platform can author and send a multi-block campaign (heading + paragraph + image + button + two-column section) in under 10 minutes from opening the campaign editor, with no hand-written HTML and no outside tool.
- **SC-002**: For visually-authored campaigns sent to a panel covering Gmail web, Gmail mobile, Apple Mail desktop, Apple Mail iOS, Outlook desktop, and Outlook web, the rendered inbox layout matches the in-editor desktop or mobile preview in 100% of cases for the core block set (paragraph, heading, list, link, image, button, divider, two-/three-/four-column).
- **SC-003**: 100% of images in visually-authored campaigns reference the tenant media library — automated inspection of produced HTML finds no data URLs and no non-library URLs introduced by visual flows (drag/paste/picker).
- **SC-004**: A campaign authored in the visual editor, then round-tripped through code view (HTML edited and returned), preserves the operator's HTML edit verbatim in the produced output in 100% of cases — either round-tripped or surfaced as a raw-HTML block.
- **SC-005**: Existing raw-HTML campaigns and templates authored before this phase continue to send successfully and continue to open for editing in the fall-back code-only editor; 0% of them are silently rewritten by the new editor on open.
- **SC-006**: An operator who changes the tenant branding sees the editor's default theme reflect the change the next time they open a campaign or template that has no explicit theme override, and existing overrides remain pinned in 100% of cases.
- **SC-007**: Image uploads via drag, paste, or picker that violate the media library's size or type limit are rejected up front with a specific reason in 100% of cases and persist nothing — verified by attempting boundary cases and inspecting both the editor and the media library.
- **SC-008**: Autosave preserves an operator's in-progress changes in the visual editor across an accidental tab close or browser refresh in 100% of cases up to the last autosave checkpoint.
- **SC-009**: Every failed operation in the visual editor (save conflict, autosave failure, upload failure, theme save, code-view-conversion failure) surfaces a clear, named reason — operators are never left with a silent failure or a generic error.
- **SC-010**: All five user stories can be demonstrated end-to-end against the tenant workspace without manual data setup beyond normal tenant configuration, satisfying the Phase 7 visual-editor exit criterion from both the campaign-author and template-author perspectives.
- **SC-011**: 100% of campaigns and templates saved with a placeholder whose subscriber slug is not present in the tenant's custom-field registry are rejected at save time with a clear, named error that identifies the offending placeholder(s) — operators are never able to send a campaign whose substitution would fail at send time due to an unknown slug.
- **SC-012**: The merge-tag picker in the visual editor and the "visible profile fields" picker on Phase 6 subscription pages list the exact same set of subscriber fields for any given tenant — the two surfaces never diverge.

## Assumptions

- Phases 1–6 are delivered: the tenant workspace app shell and its permission gating; the campaign and campaign-template editors (Phase 3 UI / 007); the Phase 3 send pipeline that consumes a campaign's content as email-ready HTML and plain-text; the Phase 6 tenant branding (logo, colors); the Phase 6 tenant media library and its picker; the Phase 6 media size/type limits.
- The campaign and campaign-template editors continue to be the surfaces in which this feature embeds; this phase does not introduce a separate editor application.
- Transactional templates remain authored in the basic/code editor; defaulting them to the visual editor is out of scope, because they are commonly authored by developers and may contain templating that the visual editor would not preserve.
- Personalization (subscriber-field placeholders, link tracking insertion, open-pixel insertion) continues to be handled by the Phase 3 send pipeline at send time; the visual editor preserves placeholders verbatim but does not implement personalization itself.
- This phase introduces the first tenant-scoped subscriber custom-field registry; Phase 6 subscription-page configuration (013) is expected to be aligned to read from it as part of this phase, so both surfaces share one canonical list of subscriber fields. Pre-existing free-form subscriber `attributes` remain valid storage and are not migrated; they simply don't appear in the merge-tag picker until the tenant adds matching registry definitions.
- The produced HTML targets the major recipient clients listed in FR-011; long-tail client-specific quirks beyond that set are best-effort.
- A custom-extension surface for advanced operators or partners to author their own block types is foreseen but not part of this phase; only the built-in block set in FR-002 is in scope.
- Real-time collaboration / multi-operator co-editing of the same campaign is explicitly out of scope. No collaboration service, presence indicators, shared cursors, comments, or collaborative-extension features are introduced; only single-operator authoring UI is delivered. The conflict behaviour in FR-009 is the only multi-tab/multi-session affordance this phase delivers.
- Localization of the editor's UI follows the workspace's existing language coverage; multi-language campaign content (translating the same campaign into multiple languages from inside the editor) is out of scope for this phase.
- Desktop web is the primary target for the visual editor; the editor remains usable on smaller screens but is not optimized for phone-sized authoring.
- The choice of underlying editor technology is an implementation decision deferred to the plan phase; the spec's contract with the rest of the platform is the produced HTML, the plain-text alternative, and the structured document the editor saves and reloads. Candidate technologies surveyed include React Email Editor (`@react-email/editor`, MIT, built on TipTap/ProseMirror, ships email-aware extensions and HTML export) and an in-house build on top of TipTap core + StarterKit (MIT) with custom drag-handle, slash-command, and bubble-menu components inspired by TipTap's Notion-like editor template. TipTap Pro extensions (e.g. the pre-built DragHandle extension) and TipTap's "Notion-like editor" template both require a paid Start-plan subscription; if either is used in production, it MUST be a separate licensing decision made during the plan phase. Only the visual-editing UI components from those references are in scope — collaboration/presence/comments features are explicitly excluded (see prior assumption).
