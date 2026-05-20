# Phase 0 Research — Phase 6 UI

All decisions below resolved from in-repo inspection of the Phase 6 backend
(branch 012) and the established Phase 1–5 frontend conventions. There are no
remaining `NEEDS CLARIFICATION` markers.

---

## Decision 1 — Visitor-facing pages stay server-rendered

**Decision**: Do NOT build SPA routes for the visitor-facing surfaces
(subscribe form, "check your email", confirm success / expired-link /
already-confirmed, preference page, unsubscribe confirmation, public archive
index, standalone archive page, RSS feed, error / not-available pages). They
ship as Go HTML templates served directly by the Phase 6 backend.

**Rationale**:
- The templates already exist in
  [internal/api/templates/](../../internal/api/templates/): `subscribe.html`,
  `confirm.html`, `preferences.html`, `unsubscribed.html`,
  `archive_index.html`, `archive_campaign.html`, `error.html`, `layout.html`.
  They are wired up by the public-tenant middleware
  (`resolvePublicTenant` in
  [internal/api/public_middleware.go](../../internal/api/public_middleware.go))
  and the token-resolution path
  (`resolvePreferenceToken` in
  [internal/api/preference_handlers.go](../../internal/api/preference_handlers.go)).
- The frontend SPA currently has no public/unauthenticated routes
  (`frontend/src/routes/` contains only `index.tsx`, `login.tsx`,
  `signup.tsx`, `invite/`, `tenants/`, and the authenticated `t/$slug/`
  tree). Adding a separate public sub-app would duplicate logic that the
  backend already owns and would put two tenant-resolution paths in the
  codebase, violating Principle VI.
- Server rendering keeps tenant CSS sanitization, the rate-limit response,
  the token validation, and the RSS XML output co-located with the
  authoritative data — these are not SPA concerns.

**Alternatives considered**:
- A second React sub-app under `routes/p/$tenantSlug/*` rendering the same
  pages. Rejected: introduces duplicate tenant resolution and a second
  client of the same endpoints, with no benefit (the visitor flows have
  almost no interactivity).
- Hybrid (visitor pages SSR'd by the backend but progressively enhanced
  with React). Rejected as YAGNI for Phase 6.

**Impact on the spec's user stories**: US1, US2, US3 describe visitor UX
that is delivered by the already-shipped templates. The SPA's contribution
to those stories is indirect — saving branding in the workspace flows
through to the templates' rendering — plus any small copy polish the spec
edges require.

---

## Decision 2 — Three new workspace nav entries, no consolidation

**Decision**: Add three separate sidebar entries — "Public pages",
"Branding", "Media" — gated by `subscription_pages:manage`,
`branding:manage`, and `media:get` respectively.

**Rationale**:
- Each maps to a distinct backend resource family with its own permission
  string. Collapsing them into a single nav item would force users with
  only one of the permissions to see a dead-end landing page.
- The phase-5-ui spec set the precedent of one nav entry per resource
  family (Billing/Plans/Usage/Invoices live under a single segment because
  they share a permission set; here, they don't).
- The "Public pages" segment hosts both the subscription-pages list and
  the per-tenant `PublicUrlList` (subscription URLs, preference-link
  template, archive index URL, RSS feed URL), so a single landing page can
  satisfy FR-030.

**Alternatives considered**:
- A single "Public" mega-section combining all three. Rejected — it would
  conflate three permissions and three workflows.

---

## Decision 3 — Archive-visible toggle lives on campaign detail, not in "Public pages"

**Decision**: Render the archive-visible toggle inline on
`routes/t/$slug/campaigns/$id.tsx`, gated by the existing `campaigns:manage`
permission. Do not surface a duplicate list-of-archivable-campaigns under
"Public pages".

**Rationale**:
- The backend endpoint is `POST /t/{slug}/api/campaigns/{id}/archive` and
  it sits under the campaigns resource, not under subscription pages or
  branding. Putting the UI control on the campaign detail keeps the
  authoring/operating surface co-located with the data it mutates.
- Phase-1–5 UI consistently places per-resource toggles on the detail
  route (e.g. list activation, sending-domain enable). A second
  "manageable archive list" page would be redundant.
- The "Public pages" area still surfaces the per-tenant archive *index*
  URL in the `PublicUrlList` so the administrator can share it (FR-030);
  the per-campaign toggle remains on campaigns.

**Alternatives considered**:
- A separate "Archive" sub-tab inside "Public pages" listing every sent
  campaign with a toggle. Rejected — it would duplicate the campaigns
  list and split the toggle from the campaign edit surface.

---

## Decision 4 — Custom CSS edited as plain text with sanitized preview

**Decision**: Use a plain `<Textarea>` (shadcn) for the custom CSS field
with a max-length client-side check matching the backend limit, and render
the **sanitized result the backend returns on save** as a read-only
preview block underneath. Do not embed a syntax-highlighted code editor.

**Rationale**:
- Sanitization is enforced server-side (`SaveBranding` rejects /
  normalizes disallowed constructs). The UI's job is to communicate that
  this happens and show the result — not to re-implement the sanitizer.
- A plain textarea matches every other multi-line input in the existing
  workspace UI and avoids pulling in a code-editor dependency (Monaco,
  CodeMirror) that the rest of the workspace does not use.
- The size-limit FR-028 is enforced inline; the editor surfaces the limit
  in the description text and disables save when exceeded with a clear
  message.

**Alternatives considered**:
- A Monaco/CodeMirror CSS editor with syntax highlighting. Rejected as
  scope creep — not justified for one field in one route.

---

## Decision 5 — Media uploads use multipart, no presigned URLs in this phase

**Decision**: Upload media via `POST /t/{slug}/api/media` as
`multipart/form-data` with a `file` field, mirroring the Phase 6 backend's
`handleUploadMedia`. Stream the request and show a single progress
indicator. Do not introduce a presigned-URL direct-to-S3 path.

**Rationale**:
- The backend already accepts multipart, caps the request at
  `s.cfg.MediaMaxBytes`, and returns the asset's stable `public_url`.
- The existing frontend has a `requestMultipart` helper used by
  `import-export/` for CSV upload; the same helper extends to media with
  a minor change.
- Presigned-URL uploads would require a new backend endpoint and a more
  complex client flow with no benefit at the file sizes Phase 6 targets
  (~10 MB default).

**Alternatives considered**:
- Direct browser-to-S3 with a presigned URL. Rejected — no backend
  support and out of scope for this phase.

---

## Decision 6 — Media picker is a modal launched from the HTML body field

**Decision**: Add a single `<MediaPicker>` modal component used from the
HTML-body field in `campaigns/$id.tsx`. The modal browses the same
tenant-scoped media list and, on select, inserts the asset's `public_url`
into the textarea at the cursor position (or appends, if the textarea has
no focus).

**Rationale**:
- The campaign editor today is two plain textareas (HTML body, text
  body). A WYSIWYG/page-builder is explicitly out of scope (spec
  Assumptions). Inserting a raw URL into the HTML body is the minimum
  viable integration that FR-040 requires.
- Reusing the same backend list endpoint avoids a second media surface.
- The picker can later be reused from any future authoring surface (e.g.
  templates) without modification.

**Alternatives considered**:
- Inline drag-and-drop into the HTML field. Rejected — the editor is a
  plain textarea and drag-and-drop would require either a rich editor or
  a bespoke handler with little benefit.
- A "current campaign asset library" tab within the campaign editor.
  Rejected — duplicates the global media library.

---

## Decision 7 — Public URL bundle lives on the "Public pages" landing route

**Decision**: Render a `<PublicUrlList>` component on
`routes/t/$slug/public-pages/index.tsx` that lists, with copy controls and a
"preview" link:
- each saved subscription page's public URL,
- the preference-link template (with a clear "filled per subscriber" note),
- the per-tenant archive index URL,
- the per-tenant RSS feed URL.

**Rationale**:
- FR-030 mandates a single place where these are individually copyable.
- The "Public pages" landing route is already the natural home for the
  subscription-pages list, so adding the URL bundle keeps the
  administrator's "everything I might share" surface on one page.
- The preference-link template is informational only — the actual
  per-subscriber token is generated server-side. The component renders the
  template (e.g. `/p/{token}`) and a brief explanation.

**Alternatives considered**:
- A separate "Public URLs" route. Rejected — over-fragmentation for a
  small component.

---

## Decision 8 — Permission union extended in lock-step with the backend

**Decision**: Add the four Phase 6 permissions to the frontend `Permission`
union and `ALL_PERMISSIONS` array exactly as they appear in the backend
catalogue:
- `subscription_pages:manage`
- `branding:manage`
- `media:get`
- `media:manage`

**Rationale**:
- The backend catalogue
  ([internal/iam/domain/permission.go](../../internal/iam/domain/permission.go))
  is authoritative; the frontend's `Permission` union mirrors it so that
  the existing `usePermissions(slug).canAny([...])` gating works against
  these new strings.
- The frontend's `ALL_PERMISSIONS` array drives the role-management UI;
  failing to add the new entries would make them invisible in role
  assignment.

**Alternatives considered**:
- Lazy/dynamic permission strings fetched from the backend. Rejected —
  diverges from the established Phase 1–5 pattern and offers no benefit at
  this scale.
