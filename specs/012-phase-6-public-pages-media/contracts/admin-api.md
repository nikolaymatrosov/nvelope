# Contract: Admin API (authenticated)

JSON endpoints under the existing tenant-scoped group
`/t/{slug}/api/...`, behind the existing `resolveTenant` + `authz`
middleware. Permission checks reuse the IAM permission strings. Typed domain
errors are mapped to HTTP status codes once, in `internal/api/errmap.go`.

New permission strings (added to the IAM permission union):

- `subscription_page:manage` — configure public subscription pages.
- `branding:manage` — configure tenant branding / custom CSS.
- `media:get`, `media:manage` — view / upload-delete media.
- Campaign archive toggling reuses the existing `campaign:manage`.

## Subscription page configuration

### `GET /t/{slug}/api/subscription-pages`

List the tenant's subscription pages. Requires `subscription_page:manage`.

- `200` — array of page configs.

### `POST /t/{slug}/api/subscription-pages`

Create a page. Body: `slug`, `title`, `target_list_ids`, `fields`,
`sending_domain_id`, `from_name`, `from_local_part`. Requires
`subscription_page:manage`.

- `201` — created.
- `400 validation_failed` — empty list set, blank title, bad local-part.
- `409 subscription_page_slug_taken` — duplicate `(tenant, slug)`.
- `404 list_not_found` / `sending_domain_not_found` — a referenced entity is
  not the tenant's.

### `PUT /t/{slug}/api/subscription-pages/{id}`

Update a page (including `active`). Requires `subscription_page:manage`.

- `200` — updated. `400` / `404` / `409` as above.

## Branding

### `GET /t/{slug}/api/branding`

Current branding. Requires `branding:manage`.

- `200` — branding (platform defaults if never set).

### `PUT /t/{slug}/api/branding`

Body: `logo_media_id`, `primary_color`, `custom_css`. Requires
`branding:manage`.

- `200` — saved (CSS sanitised on save).
- `400 invalid_color` — non-hex `primary_color`.
- `400 unsafe_css` — custom CSS contains a disallowed construct (FR-022).
- `404 media_not_found` — `logo_media_id` is not the tenant's.

## Campaign archive toggle

### `POST /t/{slug}/api/campaigns/{id}/archive`

Body: `{ "visible": bool }`. Requires `campaign:manage`.

- `200` — `archive_visible` updated; `archived_at` set on first enable.
- `404 campaign_not_found`.
- `409 campaign_not_sent` — attempt to archive a draft / never-sent campaign
  (FR-016).

## Media library

### `GET /t/{slug}/api/media`

Paginated list of the tenant's media assets (filename, content type, size,
`public_url`, created date, uploader). Served from the RLS-protected
`media_assets` table — never by listing the bucket. Requires `media:get`.

- `200` — `{ items: [...], total, page }`.

### `POST /t/{slug}/api/media`

`multipart/form-data` upload of one file. Requires `media:manage`.

- `201` — `{ id, filename, content_type, size_bytes, public_url }`. Bytes are
  written to object storage at `media/{tenant_id}/{id}/{filename}` before the
  metadata row is inserted (FR-029).
- `400 unsupported_media_type` — content type not in the allowlist.
- `400 media_too_large` — exceeds the configured size cap.
- `400 empty_upload` — zero-byte or missing file.

### `DELETE /t/{slug}/api/media/{id}`

Requires `media:manage`.

- `204` — metadata row and object both removed.
- `404 media_not_found` — not the tenant's asset (RLS makes another tenant's
  asset invisible regardless of the id supplied — FR-028).

## Error-kind → HTTP mapping (added to `errmap.go`)

| Domain error kind | HTTP |
|---|---|
| `validation_failed`, `invalid_color`, `unsafe_css`, `unsupported_media_type`, `media_too_large`, `empty_upload` | 400 |
| `subscription_page_slug_taken`, `campaign_not_sent` | 409 |
| `list_not_found`, `sending_domain_not_found`, `campaign_not_found`, `media_not_found` | 404 |

The public pages do not use this JSON mapping — they render branded HTML
(`error.html`) and use only `200` / `404` as described in
[public-pages.md](./public-pages.md).
