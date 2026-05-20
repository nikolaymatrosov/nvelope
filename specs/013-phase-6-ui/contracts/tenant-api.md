# Tenant API Contracts — Phase 6 UI Consumer

This document lists the Phase 6 backend endpoints the frontend SPA consumes.
All endpoints already exist in the Go backend (branch 012); this UI work
adds **no new endpoints, no schema changes, no migrations**. Visitor-facing
endpoints (e.g. `GET /t/{slug}/subscribe/{pageSlug}`) are listed here only
for completeness — the SPA does NOT call them; they render HTML directly
from the Go templates and are reached by a visitor's browser, not by the
React app.

Authenticated endpoints listed below are reached through the existing
tenant-scoped client `tp(slug, suffix)` in
[frontend/src/lib/api.ts](../../../frontend/src/lib/api.ts). All requests
carry the session cookie; the backend re-checks authorization on every
request and is authoritative.

All paths are relative to the API root the SPA already targets.

---

## 1. Subscription page config (admin)

**Permission**: `subscription_pages:manage`

| Method | Path                                              | Purpose                          |
|--------|---------------------------------------------------|----------------------------------|
| GET    | `/t/{slug}/api/subscription-pages`                | List the tenant's subscription pages |
| POST   | `/t/{slug}/api/subscription-pages`                | Create a subscription page       |
| PUT    | `/t/{slug}/api/subscription-pages/{id}`           | Update a subscription page       |
| DELETE | `/t/{slug}/api/subscription-pages/{id}`           | Delete a subscription page       |

**Request body** (POST, PUT):

```json
{
  "slug": "newsletter",
  "title": "Subscribe to our newsletter",
  "target_list_ids": ["<uuid>"],
  "fields": [{ "key": "first_name", "label": "First name", "required": true }],
  "sending_domain_id": "<uuid|null>",
  "from_name": "Acme",
  "from_local_part": "hello",
  "active": true
}
```

**Response** (GET list, GET item, POST, PUT) returns `SubscriptionPageView`
or `{ items: SubscriptionPageView[] }` for the list (see
[data-model.md](./data-model.md#subscriptionpageview)).

**Errors**:

| Backend error kind      | UI state                                        |
|-------------------------|-------------------------------------------------|
| `incorrect_input`       | inline field-level error (e.g. slug taken)      |
| `subscription_page_not_found` | not-found page (route landing or detail)  |
| `unauthorized` (401)    | redirect to sign-in                             |
| `forbidden` (403)       | "you don't have access" inline placeholder      |

---

## 2. Branding (admin)

**Permission**: `branding:manage`

| Method | Path                              | Purpose                          |
|--------|-----------------------------------|----------------------------------|
| GET    | `/t/{slug}/api/branding`          | Get the tenant's branding        |
| PUT    | `/t/{slug}/api/branding`          | Save the tenant's branding       |

**Request body** (PUT):

```json
{
  "logo_url": "https://media.example/logo.png",
  "primary_color": "#1A73E8",
  "custom_css": ".hdr { font-family: serif }"
}
```

**Response** (GET, PUT) returns `BrandingView` (see
[data-model.md](./data-model.md#brandingview)). The `custom_css` returned
is the **sanitized** result — the UI displays this as the read-only
preview while preserving the operator's raw input in the textarea until a
successful save.

**Errors**:

| Backend error kind   | UI state                                                    |
|----------------------|-------------------------------------------------------------|
| `incorrect_input`    | inline message — typically size limit exceeded              |
| `unauthorized` (401) | redirect to sign-in                                         |
| `forbidden` (403)    | "you don't have access" placeholder                         |

---

## 3. Campaign archive visibility toggle (admin)

**Permission**: `campaigns:manage` (existing)

| Method | Path                                              | Purpose                          |
|--------|---------------------------------------------------|----------------------------------|
| POST   | `/t/{slug}/api/campaigns/{id}/archive`            | Set archive visibility           |

**Request body**:

```json
{ "visible": true }
```

**Response**: 204 No Content, or the updated `CampaignView` (UI invalidates
the campaign and campaigns-list queries on success).

**Errors**:

| Backend error kind     | UI state                          |
|------------------------|-----------------------------------|
| `campaign_not_found`   | not-found state                   |
| `unauthorized` (401)   | redirect to sign-in               |
| `forbidden` (403)      | toggle disabled                   |

---

## 4. Media library (workspace, team-facing)

**Permissions**: `media:get` (list), `media:manage` (upload, delete)

| Method | Path                              | Purpose                          |
|--------|-----------------------------------|----------------------------------|
| GET    | `/t/{slug}/api/media`             | List the tenant's media          |
| POST   | `/t/{slug}/api/media`             | Upload a file (multipart)        |
| DELETE | `/t/{slug}/api/media/{id}`        | Delete a media asset             |

**Request body** (POST, `multipart/form-data`):

- `file`: the file part — required
- (no other fields in this phase)

**Request size cap**: enforced server-side at `MediaMaxBytes` (config).
The UI rejects oversized files inline before sending; on a server reject,
surfaces the reason.

**Response** (GET) returns `{ items: MediaAssetView[] }`. (POST) returns
`MediaAssetView` for the newly stored asset. (DELETE) returns 204 No
Content.

**Errors**:

| Backend error kind         | UI state                                                            |
|----------------------------|---------------------------------------------------------------------|
| `incorrect_input`          | inline reason (type not allowed; size exceeded; multipart missing)  |
| `media_asset_not_found`    | not-found state on detail; row removed from list                    |
| `payload_too_large` (413)  | inline "file is too large" with the configured limit                |
| `unauthorized` (401)       | redirect to sign-in                                                 |
| `forbidden` (403)          | upload/delete controls disabled                                     |

---

## 5. Visitor-facing endpoints (NOT consumed by the SPA)

These endpoints render HTML directly from
[internal/api/templates/](../../../internal/api/templates/) and serve the
visitor's browser, not the React app. They are listed here for completeness
and to document the public URL bundle the workspace UI surfaces.

| Method   | Path                                                   | Purpose                               | Listed in `PublicUrlList`?       |
|----------|--------------------------------------------------------|---------------------------------------|----------------------------------|
| GET/POST | `/t/{slug}/subscribe/{pageSlug}`                       | Public subscription form              | Yes (per subscription page)      |
| GET      | `/t/{slug}/confirm/{token}`                            | Confirm pending subscription          | No (token-bearing)               |
| POST     | `/t/{slug}/confirm/{token}/resend`                     | Request fresh confirmation email      | No                               |
| GET/POST | `/p/{token}`                                           | Preference page                       | Yes — template form `/p/{token}` |
| GET/POST | `/u/{token}`                                           | One-click unsubscribe                 | Yes — template form `/u/{token}` |
| GET      | `/t/{slug}/archive`                                    | Public archive index                  | Yes                              |
| GET      | `/t/{slug}/archive/{campaignId}`                       | Standalone archived campaign page     | No (per-campaign URL)            |
| GET      | `/t/{slug}/feed.xml`                                   | RSS feed                              | Yes                              |

The `PublicUrlList` component renders the rows marked "Yes" in this table
with copy controls and "preview in new tab" links.

---

## Cross-cutting

- All authenticated calls go through the tenant-scoped client
  `tp(slug, …)`; the `slug` is a required first argument, eliminating
  per-call-site tenant-scope omission.
- All endpoints return errors in the existing `apperr` envelope
  (`{ error: { kind, message } }`). Routes map `kind` → UI state in one
  place per route; transport branching does not leak into business logic.
- The `media:get`-permission listing in #4 doubles as the source for the
  `<MediaPicker>` modal used from the campaign HTML-body field — no
  separate endpoint is introduced.
