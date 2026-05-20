# Brainstorm — Move email-HTML rendering to the BFF (react-email)

**Feature**: Phase 7 — Visual Email Editor
**Branch**: `014-visual-email-editor`
**Date**: 2026-05-20
**Status**: design approved; pending `/speckit-clarify` + `/speckit-plan` updates
**Input**: [research.md § R3, § R4](./research.md), [plan.md](./plan.md),
[tasks.md](./tasks.md), the in-flight implementation through commit
`6824db0` (Phase 2 foundational complete; T034–T045 written; partial
frontend in progress).

## 1. Why we are reconsidering

Phase 2 shipped an in-house Go renderer
([internal/campaign/adapters/visualrender/render.go](../../internal/campaign/adapters/visualrender/render.go))
per research.md § R4. The renderer works — 284 LOC, golden-tested across
every block type — but it is missing the email-client-specific polish
that battle-tested libraries already encode (Outlook VML fallbacks for
buttons, MSO conditional comments around column tables, fine-grained
pixel-vs-percent sizing strategies).

`react-email` (the Resend MIT library at react.email) handles those
quirks out of the box. Three earlier evaluations rejected it because
running JavaScript at API render time was framed as "shelling out to a
JS runtime from a Go service" (research.md § R4 alternatives) — but the
repository already runs a Node tier next to the Go API: TanStack Start +
Nitro is the SPA host ([frontend/vite.config.ts](../../frontend/vite.config.ts)).
That Node tier is a Backend-For-Frontend (BFF). Today it only proxies
`/api/*` and `/t/{slug}/api/*` to the Go service. The BFF already runs
on every deploy, already exists in our monitoring, and already serves
authenticated user traffic. Giving it one more responsibility — turning
a VisualDoc into the email HTML — costs no new operational surface
beyond what already exists.

## 2. Topology decision

**Topology A — BFF orchestrates, Go API validates and persists.** Approved
by the user during brainstorming (2026-05-20).

```text
browser
  │ PUT /t/{slug}/api/campaigns/{id}/visual { subject, bodyDoc, theme? }
  ▼
Nitro server route (BFF, inside TanStack Start)
  │
  ├─ GET /t/{slug}/api/subscriber-fields     ← cookie-forwarded, used for slug allow-list
  ├─ GET /t/{slug}/api/branding              ← only if theme === null
  │
  ├─ validate doc (TS): shape, columns count, mediaRef host, namespace allow-list,
  │                     slug membership, campaign-key allow-list, RawHTML size
  │   rejects with 400 invalid_doc / 422 unknown_placeholder / 422 invalid_media_ref
  │   BEFORE paying the render cost
  │
  ├─ render via @react-email/components → { bodyHtml, bodyText, warnings }
  │
  │ PUT /t/{slug}/api/campaigns/{id}/visual { subject, bodyDoc, bodyHtml, bodyText, theme? }
  ▼
Go API (cookie-authenticated as the same user)
  │ ✓ revalidate doc (defense in depth — Constitution IV)
  │ ✓ sanitize HTML via bluemonday (always — never trust upstream HTML)
  │ ✓ persist body_doc, body_html, body_text, theme atomically
  └─ ← campaign view (200 OK)
```

### 2a. Principles enforced

1. **Browser is never authoritative.** The BFF runs server-side, on the
   same origin as the SPA host; the session cookie naturally flows
   through to it. The BFF call to Go uses the same forwarded cookie.
   This preserves the spirit of research.md § R3 ("server-side at save
   time") — *server* now means the BFF, but the browser is still not
   producing the canonical HTML.
2. **Defense in depth on validation.** The BFF validates pre-render for
   UX (fast feedback before the render cost). The Go API re-validates
   pre-persist as the authoritative gate (owns the field registry,
   owns tenant isolation). Both validators MUST enforce the same
   rules; the validator-sync plan is in § 4.
3. **Sanitizer always wins.** react-email's output is well-formed, but
   the Go-side bluemonday sanitization stays as the final layer before
   persistence (per Constitution IV — server-side sanitization is
   authoritative). The sanitizer never trusts the BFF's HTML.

### 2b. URL topology

The BFF intercepts the two visual-editor paths **before** the catch-all
Vite/Nitro proxy:

- `PUT /t/{slug}/api/campaigns/{id}/visual` → Nitro route
- `POST /t/{slug}/api/campaigns/{id}/render-preview` → Nitro route
- Everything else → still proxied transparently to the Go API at
  `localhost:8080` (dev) or the equivalent in prod.

The frontend's `api.ts` client does **not** change. From the browser's
perspective the URLs are identical to what the Go API previously
exposed. The BFF is invisible to the SPA.

### 2c. Authentication

The BFF reads the session cookie from the incoming request and forwards
it on the Go HTTP call. Go's existing session middleware authenticates
the user identically to a direct browser→Go call. No service account,
no JWT, no on-behalf-of header.

If the cookie is missing or rejected by Go on the branding / subscriber-
fields fetches, the BFF surfaces `401 unauthenticated`. If the cookie
is valid for the user but the user lacks `campaigns:manage`, Go returns
`403`, which the BFF passes through verbatim.

## 3. Component mapping (VisualDoc → react-email)

| VisualDoc block | react-email component | Notes |
|---|---|---|
| Paragraph | `<Text>` | inline marks (bold/italic/underline/strike/color/link) inside as raw `<strong>`/`<em>`/`<u>`/`<s>`/`<span>`/`<Link>` |
| Heading (level 1–3) | `<Heading as="h{level}">` | sizes from theme |
| BulletList | inline-styled raw `<ul>` + `<li>` | react-email has no list primitive; structured raw markup is safe under bluemonday |
| OrderedList | inline-styled raw `<ol>` + `<li>` | same as above |
| ListItem | nested children rendered as their block types | first child inlined for prose-style items |
| Quote | inline-styled raw `<blockquote>` | minimal styling tuned for emails |
| Code | `<CodeBlock>` | monospace, padded, scrollable on mobile |
| Image | `<Img>` | enforces `display:block; border:0` etc. |
| Button | `<Button>` | react-email's VML+table button handles Outlook |
| Divider | `<Hr>` | inline-styled |
| Columns (2/3/4) | `<Row>` + N × `<Column>` | width: `${100/count}%`; MSO conditional comments are react-email's responsibility |
| RawHTML | `dangerouslySetInnerHTML` | passes through to Go's sanitizer, which is the final gate |
| MergeTag (inline) | literal text `{{ namespace.key }}` | react-email renders text verbatim; the send pipeline substitutes at send time |
| Text (inline) | literal text + marks | see Paragraph row |

Marks (`bold`, `italic`, `underline`, `strike`, `color`, `link`) are
applied in fixed outer-to-inner order so closing tags stack
symmetrically:
`link → color → bold → italic → underline → strike → text`. This
mirrors the order the existing Go renderer uses
([render.go](../../internal/campaign/adapters/visualrender/render.go)
`renderTextRun`), so the byte output across renderers should be
broadly comparable, even if not exactly identical.

## 4. Validation sync — rules duplicated, drift-catchers in place

Both validators MUST enforce the same rules, derived from one place:
the spec. They are listed below with the canonical reference and the
implementation site on each side.

| Rule | Spec | BFF (TypeScript) | Go (re-validation) |
|---|---|---|---|
| Doc envelope: `type === "doc"`, `version === 1` | [data-model.md](./data-model.md) | `validate/envelope.ts` | `domain.Validate` |
| Block type ∈ closed set | [data-model.md](./data-model.md) | `validate/blocks.ts` switch | `validateNode` switch |
| `Heading.attrs.level ∈ {1,2,3}` | FR-002 | enforced | `validateNode` |
| `Columns.attrs.count ∈ {2,3,4}` and `content.length === count` | FR-015 | enforced | `validateNode` |
| `Image.attrs.mediaRef` matches tenant media URL prefix | FR-021 | `ObjectStoragePublicBaseURL` env prefix check | `MediaRefValidator.IsTenantMediaRef` |
| Link/button href scheme ∈ `{http, https, mailto, tel}` | [research.md § R5](./research.md) | `validate/link.ts` | `validateLink` |
| `MergeTag.attrs.namespace ∈ {subscriber, campaign}` | FR-016 | enforced | `validateMergeTag` |
| `MergeTag.attrs.key` for `subscriber` ∈ tenant registry (built-in ∪ custom) | FR-016a, FR-016c | fetched from `GET /subscriber-fields` | `FieldSet.HasSlug` against the same source |
| `MergeTag.attrs.key` for `campaign` ∈ platform allow-list | FR-016 | static const mirrored from Go | `AllowedCampaignMergeTags` |
| `RawHTML.attrs.html.length ≤ 64 KiB` | [data-model.md](./data-model.md) | enforced | `maxRawHTMLBytes` const |

### 4a. Drift-catcher: the campaign-namespace allow-list

The campaign-namespace allow-list (`unsubscribe_url`, `preference_url`,
`archive_url`, `view_in_browser_url`, `tenant_name`, `current_date`) is
the highest drift risk — it's a closed list the platform controls, and
adding an entry on one side without the other silently breaks save-time
validation.

Mitigations:

1. **Cross-reference comments** on both sides pointing at each other's
   file.
2. **Drift-catcher test** in the BFF (`campaign-keys.test.ts`) that
   reads `internal/campaign/domain/visualdoc.go`, parses the
   `AllowedCampaignMergeTags` map literal, and asserts deep-equality
   with the TS const. If Go adds a key and TS isn't updated, the
   frontend test suite fails loudly.
3. A code-review checklist item: any change to either side requires the
   matching change in the same commit.

### 4b. `mediaRef` host check across processes

The BFF reads `process.env.OBJECT_STORAGE_PUBLIC_BASE_URL`; the Go API
reads `cfg.ObjectStoragePublicBaseURL`. Same env var on both sides. A
deployment-config drift surfaces as `422 invalid_media_ref` in dev or
staging long before prod.

## 5. Code delta

### 5a. Go side

| File | Change |
|---|---|
| `internal/campaign/adapters/visualrender/render.go` | **DELETE** — replaced by BFF render |
| `internal/campaign/adapters/visualrender/render_golden_test.go` | **DELETE** |
| `internal/campaign/adapters/visualrender/sanitize.go` | **KEEP** — last-step sanitization stays authoritative |
| `internal/campaign/adapters/visualrender/sanitize_test.go` | **KEEP** |
| `internal/campaign/adapters/visualrender/placeholders.go` + test | **KEEP** — used by Go's revalidation pass |
| `internal/campaign/domain/visualdoc.go` + `visualdoc_validate.go` + `visualdoc_json.go` + tests | **KEEP** — unchanged |
| `internal/campaign/domain/theme.go` + tests | **KEEP** |
| `internal/campaign/domain/renderer.go` (`Renderer` consumer-owned interface) | **DELETE** — no Go renderer to satisfy |
| `internal/campaign/domain/template.go` `NewVisualTemplate` | **MODIFY** — drop the `Renderer` parameter; accept pre-rendered `bodyHtml`/`bodyText` |
| `internal/campaign/domain/campaign.go` `NewVisualCampaign` / `ApplyVisualSave` | **MODIFY** — same shape change |
| `internal/campaign/domain/visualconstructors_test.go` | **MODIFY** — drop `fakeRenderer`; assert html/text pass through |
| `internal/campaign/app/command/save_visual_campaign.go` | **MODIFY** — drops `renderer`, `FieldsLister`, `BrandingResolver` constructor deps; accepts pre-rendered html+text in the command struct; still validates the doc + sanitizes the html |
| `internal/campaign/app/command/render_preview.go` | **DELETE** — preview is BFF-only |
| `internal/api/visual_handlers.go` save handler | **MODIFY** — accepts `{ subject, bodyDoc, bodyHtml, bodyText, theme? }` in the request body; `bodyHtml`/`bodyText` are required |
| `internal/api/visual_handlers.go` preview handler | **DELETE** |
| `internal/api/server.go` | **MODIFY** — drop the `POST /campaigns/{id}/render-preview` route mount |
| `internal/service/application.go` `buildCampaign` | **MODIFY** — drop the `renderer`, `fieldsAdapter`, `brandingAdapter` shims and the `visualrender` import |

### 5b. BFF side (new)

| File | Purpose |
|---|---|
| `frontend/src/server/render/components.tsx` | VisualBlock → react-email component mapping |
| `frontend/src/server/render/index.ts` | Public `renderVisualDoc(doc, theme) → { bodyHtml, bodyText, warnings }` |
| `frontend/src/server/render/render.test.ts` | Golden tests for every block type, asserted against fixture files |
| `frontend/src/server/render/render-marks.test.ts` | Bold/italic/underline/strike/color/link combinations |
| `frontend/src/server/validate/envelope.ts` | Doc envelope check (version, type === "doc") |
| `frontend/src/server/validate/blocks.ts` | Block-type switch + per-block rule enforcement |
| `frontend/src/server/validate/link.ts` | Scheme allow-list |
| `frontend/src/server/validate/campaign-keys.ts` | Static mirror of `AllowedCampaignMergeTags` |
| `frontend/src/server/validate/index.ts` | Public `validateVisualDoc(doc, ctx) → ValidationResult` |
| `frontend/src/server/validate/*.test.ts` | Per-rule unit tests + the drift-catcher (`campaign-keys.test.ts` reading Go source) |
| `frontend/src/server/clients/go-api.ts` | Typed Go-API client with cookie forwarding (subscriber-fields, branding, visual-save) |
| `frontend/src/server/routes/visual-save.ts` | Nitro route handling `PUT /t/:slug/api/campaigns/:id/visual` |
| `frontend/src/server/routes/render-preview.ts` | Nitro route handling `POST /t/:slug/api/campaigns/:id/render-preview` |
| `frontend/src/server/routes/*.test.ts` | Route-level tests with msw mocks |
| `frontend/vite.config.ts` | Tighten proxy to exclude the two BFF-owned paths |
| `frontend/package.json` | Add `@react-email/components` + `@react-email/render` (MIT, exact-pinned) |

### 5c. Estimated net code delta

- **Go LOC removed:** ~530 (`render.go` 284 + `render_golden_test.go` 246)
- **Go LOC modified:** ~150 (save command, HTTP handler, aggregate constructors, service composition)
- **TS LOC added:** ~600 (render module, validate module, two Nitro routes, tests)

## 6. Task delta against [tasks.md](./tasks.md)

### Phase 0 — foundational changes

| Task | Action |
|---|---|
| T021 | **REMOVE** — `internal/campaign/adapters/visualrender/render.go` |
| T022 | **REMOVE** — `render_golden_test.go` |
| T023 | **KEEP** — sanitizer stays |
| T024 | **KEEP** |
| T025 | **KEEP** — placeholder extraction used by Go's revalidation |
| T026 | **KEEP** |

### Phase 1 — aggregate + command surface changes

| Task | Action |
|---|---|
| T017 (`renderer.go` consumer-owned interfaces) | **AMEND** — drop the `Renderer` interface (no Go renderer to satisfy). Keep the `FieldSet` interface — `domain.Validate` still needs it for the Go-side revalidation pass |
| T018 | **AMEND** — `NewVisualTemplate` drops the `renderer Renderer` parameter; accepts pre-rendered `bodyHtml`/`bodyText` |
| T019 | **AMEND** — same shape change for `NewVisualCampaign` |
| T020 | **AMEND** — constructor tests drop the `fakeRenderer`; assert html/text pass through |
| T034 | **AMEND** — `SaveVisualCampaign` command drops `Renderer`/`FieldsLister`/`BrandingResolver` from its constructor; accepts pre-rendered html+text |
| T035 | **REMOVE** — preview is BFF-only |
| T036 | **AMEND** — HTTP request shape for `PUT /visual` adds required `bodyHtml`/`bodyText` |
| T037 | **REMOVE** — `POST /campaigns/{id}/render-preview` route, handler, integration tests |

### Phase 2 — new BFF task block (replaces T021/T022/T035)

| New ID | Task |
|---|---|
| **TB1** | Add `@react-email/components` + `@react-email/render` to `frontend/package.json`; pin exact versions; `pnpm install` |
| **TB2** | Create `frontend/src/server/render/components.tsx` — VisualBlock → react-email component mapping |
| **TB3** | Create `frontend/src/server/render/index.ts` — public `renderVisualDoc(doc, theme)` |
| **TB4** | `frontend/src/server/render/render.test.ts` — golden tests for every block type + marks |
| **TB5** | Create `frontend/src/server/validate/*` — TS port of doc validation |
| **TB6** | `frontend/src/server/validate/*.test.ts` — including the cross-stack drift-catcher reading Go source |
| **TB7** | Create `frontend/src/server/clients/go-api.ts` — typed Go-API client with cookie forwarding |
| **TB8** | Create `frontend/src/server/routes/visual-save.ts` — Nitro route handler |
| **TB9** | Create `frontend/src/server/routes/render-preview.ts` — Nitro route handler |
| **TB10** | `frontend/src/server/routes/*.test.ts` — route-level tests with msw mocks |
| **TB11** | Update `frontend/vite.config.ts` — tighten proxy to exclude the two BFF-owned paths |
| **TB12** | Update `MEMORY.md` / `CLAUDE.md` to document the BFF's render responsibility |

### Phase 3 — frontend editor (unchanged)

Frontend editor work (T046–T058) stays as written. The frontend
doesn't know the BFF orchestrates internally — it still calls
`api.saveVisualCampaign(slug, id, { subject, bodyDoc, theme })`. The
component tests (T059–T062) stay as designed; assertions about the
response shape are unchanged.

## 7. Spec/research updates required

| File | Update |
|---|---|
| `specs/014-visual-email-editor/research.md` § R4 | **REWRITE** — replace "in-house Go renderer" decision with "BFF + react-email"; preserve the table-based / inline-style discussion since react-email already enforces both |
| `specs/014-visual-email-editor/research.md` § R3 | **CLARIFY** — "server-side render" now means BFF Nitro, not `cmd/api`. R3's intent (browser is not authoritative) is preserved |
| `specs/014-visual-email-editor/plan.md` Technical Context | **AMEND** — Frontend (new): add `@react-email/components`, `@react-email/render`. Backend (new): remove `golang.org/x/net/html` from rendering deps (still used by US4 raw-HTML → doc conversion later) |
| `specs/014-visual-email-editor/plan.md` Constitution Check | **RE-EVALUATE** Constitution V — the BFF becomes load-bearing for visual saves. Document this as an accepted trade-off (it's already load-bearing as the SPA host; the operational surface does not grow) |
| `specs/014-visual-email-editor/contracts/tenant-api.md` | **AMEND** — `PUT /visual` request body adds `bodyHtml`/`bodyText` as required server-internal fields (not visible to the SPA); add a note that the endpoint is BFF-hosted with Go as the persistence layer |
| `specs/014-visual-email-editor/data-model.md` | **AMEND** — `Renderer` consumer-owned interface removed from `internal/campaign/domain/`; renderer adapter package removed from the layout |
| `specs/014-visual-email-editor/tasks.md` | **AMEND** — apply the task delta from § 6 above |

## 8. Open questions for `/speckit-clarify`

The following did not need to be locked down during brainstorming but
the planning phase should ask the user about them explicitly:

1. **Render warnings** — react-email does not currently emit
   "sanitizer_stripped" style warnings the way our Go sanitizer does.
   When the Go sanitizer strips content (e.g. RawHTML that contained
   `<script>`), the warning is currently returned to the operator via
   the save response. Should the BFF inspect the rendered output
   pre-Go-sanitize for the same kinds of disallowed constructs and
   surface its own warnings, OR should warnings remain exclusively
   Go-side (emitted only by bluemonday)?

2. **react-email version pinning policy** — golden tests are pinned to
   the exact react-email version. When upgrading minor versions, the
   golden fixtures will likely shift whitespace. What's the policy for
   accepting fixture updates? (Recommendation: treat fixture updates
   as code-review-visible, ship in a dedicated PR per upgrade.)

3. **BFF observability** — the Go API has structured logging
   (`tenant_id`, `actor_id`, `request_id`). What's the BFF's logging
   stance for the two new routes? Per Constitution V we should mirror
   the Go format. Does TanStack Start + Nitro have a convention here
   or do we add one?

4. **Audit events** — the audit event `campaign.save_visual` was
   originally emitted by the Go save handler. Does the event payload
   change (e.g. add `bff_render_ms`, `warnings_count`)? Where exactly
   does the audit-log row get written — still by Go, since the BFF
   does not own the audit pipeline?

5. **BFF failure modes** — if the BFF cannot reach Go for the
   subscriber-fields or branding fetch, the save fails closed
   (`502 bad_gateway`). Is that the right user-facing semantics, or
   should the BFF degrade gracefully (e.g. render without branding
   defaults, let Go reject the doc, surface a useful error)?

6. **Templates** — US2 ships visual templates. The same renderer move
   applies: `PUT /templates/{id}/visual` should also become a BFF
   route. Confirm the same architecture extends to templates and
   apply the same task-delta when US2 lands.

7. **Cross-stack contract test** — § 5 of the design proposes a Go
   integration test that sends a `bodyHtml` containing `<script>` to
   Go's save endpoint and asserts the sanitizer strips it. This is
   the *only* test in either codebase that verifies "BFF cannot
   bypass the sanitizer even if it produced bad HTML". Is one such
   test enough or do we want a broader contract-test suite?

## 9. What this does NOT change

- The send pipeline. `cmd/worker` continues to consume `body_html` +
  `body_text` from the row exactly as today.
- The placeholder substitutor in `internal/sending/domain/substitution.go`.
- The subscriber custom-field registry (`subscriber_fields` table,
  built-in pseudo-rows, CQRS handlers).
- The doc validation, sanitizer, and placeholder-extraction code in
  Go (`internal/campaign/adapters/visualrender/sanitize.go`,
  `placeholders.go`).
- The frontend's TipTap editor surface, extensions, drag handle,
  slash menu, bubble menu, merge-tag picker, or preview iframe.
- The HTTP response shape returned to the SPA.
- The frontend `api.ts` client API.
- The route table in `frontend/src/routes/`.
- The constitutional checks in [plan.md](./plan.md): all six
  principles continue to hold under topology A (with the BFF being
  re-classified as part of "server-side" for Constitution IV's
  purposes).

## 10. Implementation order (post-`/speckit-plan`)

1. Apply the spec/research/plan/contract/data-model amendments in § 7.
2. Apply the task-delta in § 6 to `tasks.md` (modify T017–T020,
   T034–T037; insert TB1–TB12 in a new Phase 2 block).
3. Implement the Go-side changes in Phase 1 (small surface; existing
   tests catch most regressions).
4. Implement TB1–TB12 in order.
5. Resume the frontend editor work (T046–T062) — that work was largely
   complete in the pre-pivot session and is unaffected by this design.

---

**Approved**: 2026-05-20 (user)
**Next**: `/speckit-clarify` against the open questions in § 8, then
`/speckit-plan` to update R4 + tasks T021–T026 + T034 (and the
templates equivalent T063–T065 to be considered as part of the same
amendment).
