# Phase 1 Data Model — View Shapes the UI Consumes

This feature introduces **no new persisted entities and no schema changes**.
Phase 6 (branch 012) already defines and persists subscription pages,
branding rows, the campaign archive-visible flag, and media assets. This
document specifies the **view shapes** the frontend consumes from the
already-existing Phase 6 endpoints, plus the four new `Permission` union
members.

All view shapes live in
[frontend/src/lib/api-types.ts](../../frontend/src/lib/api-types.ts) and are
the only types routes are allowed to import. Routes MUST NOT construct
URLs or read fields off raw responses; the client surface in
[frontend/src/lib/api.ts](../../frontend/src/lib/api.ts) is the single
boundary.

---

## SubscriptionPageView

A tenant-owned public subscription-page configuration. Backed by
`GET/POST/PUT /t/{slug}/api/subscription-pages` and
`/t/{slug}/api/subscription-pages/{id}` — see
[contracts/tenant-api.md](./contracts/tenant-api.md#1-subscription-page-config-admin).

```typescript
type SubscriptionPageView = {
  ID: string                          // UUID
  Slug: string                        // path-safe slug appearing in the public URL
  Title: string                       // shown on the public page header
  TargetListIDs: Array<string>        // one or more list IDs this page subscribes to
  Fields: Array<SubscriptionPageFieldView>
  SendingDomainID: string | null      // which verified sending domain the confirmation email is sent from
  FromName: string                    // From header display name
  FromLocalPart: string               // local-part of the From address (e.g. "hello")
  Active: boolean                     // false = "not available" public page
  PublicURL: string                   // absolute URL the admin can copy and share
  CreatedAt: string                   // ISO-8601
  UpdatedAt: string                   // ISO-8601
}

type SubscriptionPageFieldView = {
  Key: string                         // matches a subscriber attribute key
  Label: string                       // shown to the visitor
  Required: boolean
}
```

**Validation rules surfaced in the UI**:
- `Slug` MUST be path-safe (lowercase, alphanumeric, hyphens); backend
  enforces; the form rejects invalid input inline.
- `TargetListIDs` MUST contain at least one list; form disables save when
  empty.
- `Title` is required and non-empty.
- `Fields[].Key` MUST NOT include `email` (always present, always
  required, always shown — not configurable).
- The `Email` field is implicit on the public page and is not listed in
  `Fields[]`.

**State transitions surfaced in the UI**:
- `Active: false` → the public URL returns the "not available" page;
  the admin sees an "inactive" badge in the list.
- Delete is hard delete (backend behavior); the UI confirms and removes
  the row from the list query.

---

## BrandingView

A tenant's branding applied to every one of its public pages. Backed by
`GET/PUT /t/{slug}/api/branding`.

```typescript
type BrandingView = {
  LogoURL: string | null              // public URL of a media asset OR a free-form URL
  PrimaryColor: string | null         // hex (#RRGGBB) — null = use platform default
  CustomCSS: string                   // sanitized CSS as returned by the backend (may be "")
  CustomCSSBytes: number              // size of the sanitized output, in bytes
  CustomCSSLimitBytes: number         // backend-enforced size limit
  UpdatedAt: string                   // ISO-8601
}
```

**Validation rules surfaced in the UI**:
- `PrimaryColor` MUST match `#[0-9A-Fa-f]{6}` if non-null; the color
  input restricts to that shape.
- `CustomCSS` is plain text in the form; on save the response carries the
  **sanitized** result, which the UI shows as a read-only preview block.
  The unsaved textarea retains the raw input the administrator typed.
- `CustomCSSBytes > CustomCSSLimitBytes` blocks save; the editor surfaces
  the limit in the description text and disables the save control with a
  clear message.

**State transitions surfaced in the UI**:
- Saving branding is a single PUT; the next render of any public page
  picks up the new branding (FR-027).

---

## MediaAssetView

A media asset owned by a tenant. Backed by
`GET/POST/DELETE /t/{slug}/api/media` and
`DELETE /t/{slug}/api/media/{id}`.

```typescript
type MediaAssetView = {
  ID: string                          // UUID
  Filename: string                    // original upload filename
  ContentType: string                 // e.g. "image/png"
  SizeBytes: number
  PublicURL: string                   // stable, copyable URL usable in campaign HTML
  IsImage: boolean                    // true → render preview thumbnail in the grid
  CreatedAt: string                   // ISO-8601
}
```

**Validation rules surfaced in the UI**:
- Size and content-type are checked client-side before the multipart
  request is sent; the limits come from a small constants module
  populated at build time from the backend config (one source of truth on
  the server; the client constants are a copy used only for early
  rejection).
- A rejected upload (size or type) MUST NOT leave a row in the listing
  (FR-035).

**State transitions surfaced in the UI**:
- Upload progress is reflected by an in-flight indicator on the upload
  control. On 2xx, the asset is appended to the list. On 4xx, the error
  reason is shown inline and nothing is added.
- Delete is hard delete; confirmed via the shared `<ConfirmDialog>` and
  removes the row optimistically with rollback on error.

---

## Campaign archive flag (extension to existing CampaignView)

The existing `CampaignView` returned by `GET /t/{slug}/api/campaigns/{id}`
gains one new field after Phase 6:

```typescript
type CampaignView = {
  // ... existing fields ...
  ArchiveVisible: boolean             // true → appears at archive index, standalone page, RSS feed
}
```

**State transitions surfaced in the UI**:
- The toggle on `campaigns/$id.tsx` calls
  `POST /t/{slug}/api/campaigns/{id}/archive` with `{visible: true|false}`,
  which mutates `ArchiveVisible` server-side. The success criterion
  SC-005 (≤5 min freshness) is delivered by the backend's archive
  materialization; the UI shows a success toast on the round-trip.

---

## Permission union additions

The frontend `Permission` union in
[frontend/src/lib/api-types.ts](../../frontend/src/lib/api-types.ts) gains
four entries, mirroring
[internal/iam/domain/permission.go](../../internal/iam/domain/permission.go):

```typescript
type Permission =
  // ... existing 20 permissions ...
  | "subscription_pages:manage"
  | "branding:manage"
  | "media:get"
  | "media:manage"

const ALL_PERMISSIONS: Array<Permission> = [
  // ... existing 20 ...
  "subscription_pages:manage",
  "branding:manage",
  "media:get",
  "media:manage",
]
```

**Permission gating in the UI**:

| Surface                                              | Required permission           |
|------------------------------------------------------|-------------------------------|
| Sidebar entry "Public pages"                         | `subscription_pages:manage`   |
| Sidebar entry "Branding"                             | `branding:manage`             |
| Sidebar entry "Media"                                | `media:get`                   |
| Subscription-page CRUD actions                       | `subscription_pages:manage`   |
| Branding save                                        | `branding:manage`             |
| Archive-visible toggle on campaign detail            | `campaigns:manage` (existing) |
| Media upload + delete                                | `media:manage`                |
| Media list + picker browsing                         | `media:get`                   |

Operators lacking the relevant permission see the area hidden or the
action disabled, consistent with FR-046 and the existing Phase 1–5
permission-gating pattern.

---

## What this feature does NOT add

- No new persisted entities (subscription pages, branding rows, media
  assets, archive flag are all already persisted by Phase 6).
- No new database tables or columns.
- No new domain events.
- No new permission strings beyond the four declared in the Phase 6
  backend.
- No view shapes for the visitor-facing pages — those are server-rendered
  HTML, not consumed by the SPA.
