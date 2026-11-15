# Private media library, publish-on-send, image optimization — Design

Date: 2026-11-14
Status: Implemented

## Goal

Make the tenant media library private: drafts and uploaded files are
reachable only by workspace members, and an asset gets a public copy only
when content that uses it goes live. Deleting a library asset then no longer
breaks sent emails or the archive. Uploads are re-encoded server-side, which
strips metadata, normalizes formats for email clients, and caps dimensions.

This is **track B**. It builds on
[track A](2026-11-14-content-security-design.md) and closes the media items
track A deferred.

## Findings this design closes

| #  | Finding | Evidence |
|----|---------|----------|
| M1 | Every upload is `public-read`; the URL pasted into content *is* the object. Drafts, unsent files, and arbitrary PDFs are publicly fetchable forever, so the bucket doubles as free public hosting. | `media/adapters/blobstore_s3.go` `Put` (`ACL: public-read`) |
| M2 | Deleting an asset deletes the object, breaking images in delivered emails and the archive. | `media/app/command/delete_asset.go` |
| M3 | The stored Content-Type is whatever the client declared; SVG and PDF are served inline with no `Content-Disposition`. | `api/media_handlers.go` (`header.Header.Get("Content-Type")`), `media/domain/asset.go` allowlist |
| M4 | The `mediaRef` check is a platform-wide prefix: any tenant's image URL passes for every tenant. | `service/application.go` `tenantMediaRefValidator` |
| M5 | Photos keep EXIF, including GPS. | bytes stored verbatim |
| M6 | No dimension cap. Decoding (needed for M5) would expose decompression bombs. | — |
| M7 | Images are stored at upload size; Outlook desktop ignores CSS `max-width` and renders the natural pixel width. | `frontend/src/server/render/components.tsx` `ImageView` (no `width`) |

## Decisions

- **Content stores a library URL on the app origin**, not a bucket URL:
  `{BASE_URL}/t/{slug}/api/media/{assetId}/content`. For a member, Go answers
  it with a redirect to a short-lived presigned URL.
  - It works without extra code in the editor canvas, the media picker, and a
    browser tab, because all of them are same-origin and carry the session
    cookie.
  - The two sandboxed surfaces — the visual-editor preview iframe and the
    detail pages' stored-body preview — swap library URLs for presigned URLs
    before setting `srcdoc`. Their `sandbox=""` stays.
  - Rejected: storing the private bucket URL. Every admin surface would need
    a mapping, and a copied URL answers 403.
  - Rejected: `sandbox="allow-same-origin"` on the preview so cookies flow.
    It is one attribute, but it weakens a platform-protecting sandbox.
- **One bucket, two prefixes.** The library is private. Published copies sit
  under `published/` with an object ACL `public-read`, which is today's
  mechanism. Rejected: a second bucket, which means more config and
  credentials. It stays the upgrade path, for example to put a CDN in front of
  public copies only.
- **One public copy per asset, at a deterministic key.** Publishing is
  idempotent, and there is no publications table: `media_assets.published_at`
  records it.
- **A copy outlives the library asset** by default. An explicit "also remove
  the published copy" option covers takedown (GDPR requests, mistakes).
- **SVG is no longer accepted.** Gmail and Outlook do not render it, and it is
  the riskiest type. An SVG logo for public pages can still be an external
  https URL.
- **Only the optimized image is kept.** No originals.
- **No production data exists**: no backfill of existing objects, no
  compatibility for the `public_url` field.

## Design

### 1. Storage layout

| Object | Key | Access |
|--------|-----|--------|
| Library asset | `media/{tenantId}/{assetId}/{filename}` | private (no ACL) |
| Published copy | `published/{tenantId}/{assetId}/{filename}` | `public-read` ACL |

- **Deployment.** Public read access must be off at bucket level, or the
  library is public anyway. `deploy/` docs and the local config get a note.
  `NVELOPE_OBJECT_STORAGE_PUBLIC_BASE_URL` keeps its meaning: the base URL
  that public objects are reachable at.
- **`BlobStore` port.** It becomes:
  - `Put(ctx, key, contentType, length, body)`: private; the ACL is dropped.
  - `Delete(ctx, key)`: unchanged.
  - `Publish(ctx, srcKey, dstKey, contentType, disposition)`: a server-side
    `CopyObject` with `MetadataDirective: REPLACE`, `ACL: public-read`, the
    given `Content-Type`, and `Content-Disposition`.
  - `PresignGet(ctx, key, ttl, disposition)`: through `s3.NewPresignClient`;
    the disposition is passed as `ResponseContentDisposition`.
  - `PublicURL(key)`: unchanged, used for published keys only.
  - `BuildKey`: unchanged. A sibling `PublishedKey(tenantID, id, filename)`
    builds the `published/` key.
- **Disposition rule.** Images get `inline`. PDF gets `attachment;
  filename="…"`, the RFC 6266 form with `filename*` for non-ASCII names.
- The in-memory fake implements the same port.

### 2. Upload pipeline

`UploadAssetHandler` reads the body into memory; it is already capped at
`MediaMaxBytes` (10 MiB) by the handler's `MaxBytesReader`. It then:

1. **Detects the type** from `image.DecodeConfig` format names plus a `%PDF-` prefix check, so the domain does not import `net/http` (`0fdf95c`). The client-declared
   Content-Type is ignored. Allowed: `image/png`, `image/jpeg`, `image/gif`,
   `image/webp`, `application/pdf`. Anything else gives
   `unsupported_media_type`.
2. **Bounds images** with `image.DecodeConfig`. Over 25 megapixels gives 422
   `image_too_large`. This runs before any full decode, so a decompression bomb
   never allocates.
3. **Processes images** by type:

   | Input | Processing | Output |
   |-------|------------|--------|
   | JPEG | Decode, apply EXIF orientation, downscale if wider than 1200 px, encode at quality 85 | JPEG |
   | PNG | Decode, downscale if wider than 1200 px, encode with `png.BestCompression` | PNG |
   | WebP | Decode (`x/image/webp`), downscale if wider than 1200 px | PNG if any pixel is not opaque, else JPEG q85; the filename extension changes to match |
   | GIF | Stored as-is (animation is preserved), dimensions recorded | GIF |
   | PDF | Stored as-is | PDF |

   - The 1200 px limit is twice the 600 px email content width, so images
     stay sharp on high-density screens. Downscaling preserves aspect ratio
     and uses `x/image/draw.CatmullRom`.
   - Re-encoding writes no metadata, so EXIF and GPS are dropped (M5).
   - EXIF orientation is a small in-package reader: the APP1 `Exif` segment
     and tag `0x0112`, about 60 lines and no dependency. Without it, phone
     photos come out rotated once the EXIF block is gone.
   - WebP is converted because Outlook desktop does not render it.
4. **Limits concurrency.** At most 2 images decode at once per API process
   (a package-level semaphore): about 100 MB of pixels each at the 25 MP cap.
   `ponytail:` per-process limit; move decoding to a worker job if upload
   volume ever needs it.
5. **Stores** the processed bytes: size, `width`, and `height` are those of
   the output.

New dependency: `golang.org/x/image` (webp, draw).

Migration `000024_private_media`:

```sql
ALTER TABLE media_assets
  DROP COLUMN public_url,
  ADD COLUMN width        int CHECK (width > 0),
  ADD COLUMN height       int CHECK (height > 0),
  ADD COLUMN published_at timestamptz;
```

`width` and `height` are NULL for PDF.

### 3. Admin access

- **`GET /t/{slug}/api/media/{id}/content`**:
  - Requires `media:get`. The asset is loaded under RLS; a missing asset gives
    404.
  - Answers 302 to `PresignGet(storageKey, 15 min, disposition)`, with
    `Cache-Control: private, max-age=600` so the cached redirect expires
    before the signature does.
- **`GET /media` and the `POST /media` response.** Each item gains:
  - `url`: the library URL, the value that goes into content.
  - `preview_url`: presigned, valid for 1 h.
  - `width` and `height`.
  - `published`: a boolean.

  `public_url` is removed.
- **Library URL format.** `{BASE_URL}/t/{slug}/api/media/{assetId}/content`.
  `BASE_URL` is `cfg.BaseURL`, the origin the SPA is served from. The slug is
  immutable, so stored URLs stay valid.
- **SPA:**
  - Media library, detail page, and picker: thumbnails use `preview_url`.
    Copy and insert use `url`.
  - Detail page: shows a "published" badge.
  - `PreviewIframe` and `VisualBodyPreview`: before assigning `srcdoc`,
    replace each library URL with its `preview_url` from the media list query
    (a shared `usePreviewHtml` hook). That query is shared with
    the picker; `staleTime` is 30 min, under the 1 h signature. A URL whose
    asset is not in the list (deleted) is left alone and shows as broken,
    which is the correct signal.
  - Upload copy no longer mentions SVG.

### 4. Publish on send

**Media command.** `PublishAssets{TenantID, Texts []string} → ([]string,
error)`:

1. Find library URLs with
   `{BASE_URL}/t/[a-z0-9-]+/api/media/([0-9a-f-]{36})/content`, deduplicated by
   asset id.
2. Load each asset under RLS. If any are missing, return 422 `media_missing`
   listing every missing id, and publish nothing. Missing includes a foreign
   tenant's id, which RLS hides.
3. For each asset with `published_at IS NULL`, call
   `Publish(storageKey, PublishedKey(...))`, then set `published_at`.
4. Return the texts with each library URL replaced by
   `PublicURL(PublishedKey(...))`. This is a plain string replacement: the URL
   contains nothing that HTML or CSS escapes, so it matches the same way in
   `src`, `href`, `background`, and `url()`.

**Consumer ports.** Campaign and tenant contexts declare their own port and
bridge it to the media command in `internal/service/bridges.go`:

```go
// campaign/app/command and tenant/app/command
type MediaPublisher interface {
    Publish(ctx context.Context, tenantID string, texts ...string) ([]string, error)
}
```

**Call sites.**

- **`StartCampaignHandler`.** Inside the existing `campaigns.Update` closure,
  publish `BodyHTML` and `BodyText` and store the result. This happens through
  a domain method allowed only on a draft that is being started, `PublishBody(html, text)`,
  called right before `Start(now)` in the same `Update` closure.
  - Doing it inside the closure means the body that gets published is the
    locked row, so a concurrent draft edit cannot slip in.
  - Effect: a running or finished campaign's stored body holds public URLs, so
    the batch worker, archive, and RSS feed need no change.
  - `body_doc` keeps library URLs. Duplicating a sent campaign copies public
    URLs into a raw body, which is harmless because they still resolve.
  - `ponytail:` S3 copies run inside the transaction (a few hundred ms per
    start). Split into publish-then-verify if starts ever contend.
- **Transactional templates.** Every save of a `KindTransactional` template
  (raw create and update, visual save) publishes `BodyHTML` and `BodyText`
  before persisting. A transactional template is live as soon as it is saved,
  because the API can send it at any moment. Campaign-kind templates are never
  published.
- **Branding.** `SaveBranding` publishes `LogoURL` when it is a library URL
  and stores the public URL. External `https://` URLs stay allowed, as in
  track A §5. The branding picker sets `logo_url` to the asset's `url`.

A library asset deleted after it was used:

- In a **draft**, it blocks the start with `media_missing`, which names the
  missing images.
- In a **transactional visual template**, it blocks the next save the same
  way. Already-stored public URLs keep working.
- In a **sent campaign**, nothing breaks; the public copy remains.

### 5. Delete and takedown

- `DELETE /t/{slug}/api/media/{id}` without parameters removes the row and the
  private object. The public copy, if any, stays.
- `?unpublish=true` also deletes `published/…` (absent is not an error) and
  records `recordAudit(ctx, "media.unpublished", assetID, {"filename": …})`.
- UI: the delete dialog shows the checkbox only for published assets. Its
  text warns that the image disappears from already-sent emails and the
  archive.

### 6. Validation and rendering

- **`mediaRef`.** Image blocks must carry a library URL with the tenant's own
  slug: `{BASE_URL}/t/{slug}/api/media/{uuid}/content`.
  - BFF: `mediaUrlPrefix` becomes `${NVELOPE_BASE_URL}/t/${slug}/api/media/`,
    with the slug from the route, plus the UUID/`/content` tail check. This is
    the per-tenant check.
  - Go: the validator checks only that the ref is a library URL under
    `BASE_URL`, for any slug. Go's HTTP save path never decodes VisualDoc
    nodes (the `VisualDoc.UnmarshalJSON` codec is still deferred), so a Go
    slug check would only reach the converter and command-level tests;
    plumbing the slug through four commands buys nothing.
  - The final guard is RLS at publish (§4): a foreign tenant's asset is
    "missing" there.

  This closes M4.
- **BFF env.** The BFF reads `NVELOPE_BASE_URL`, the same variable and value
  Go uses, instead of `OBJECT_STORAGE_PUBLIC_BASE_URL`.
- **Image `width`.** The Image block gains an optional `width`: an integer
  from 1 to 600, in CSS pixels.
  - On insert or upload, the editor sets it to `min(asset.width, 600)`. It is
    omitted for assets without dimensions.
  - The renderer emits the `width` attribute plus
    `style="max-width:100%;height:auto"`. Without `width` it renders as today.
  - The BFF validator checks it. Go needs no change, because it never
    decodes the nodes (see above).
  - There is no inspector control to edit it yet.

  This closes M7.

## Error handling

| Case | Result |
|------|--------|
| Detected type not allowed (includes SVG) | 422 `unsupported_media_type` |
| Image over 25 MP | 422 `image_too_large` |
| Undecodable image (truncated, corrupt) | 422 `unsupported_media_type` |
| Start, transactional save, or branding save references a missing asset | 422 `media_missing` (ids listed); nothing sent or saved |
| S3 copy fails during publish | 5xx; the start or save fails, nothing is stored; already-made copies are harmless and reused on retry |
| `GET …/content` for a missing or foreign asset | 404 |
| `mediaRef` with a foreign slug or bad format | 422 `invalid_media_ref` (unchanged code) |

## Testing

- **Upload processing** (unit, generated fixtures):
  - JPEG with EXIF orientation 6 and GPS: the output is rotated, has no APP1,
    and is at most 1200 px wide.
  - Small PNG: dimensions are unchanged and it is re-encoded.
  - WebP with alpha gives PNG; WebP without alpha gives JPEG; the extension
    changes.
  - GIF: the bytes are unchanged.
  - A PNG header declaring 20000×20000 is rejected before decode.
  - SVG bytes declared as `image/png` are rejected.
  - PDF is stored as-is.
- **`PublishAssets`** (in-memory repo and fake blob store):
  - Rewrites URLs in `src`, `href`, and `url()`.
  - Publishing twice copies once.
  - A missing id fails with every missing id listed and copies nothing.
  - A foreign tenant's id counts as missing.
- **Campaign start:** the stored body holds public URLs; `media_missing`
  leaves the campaign a draft.
- **Transactional template:** saving publishes; a campaign-kind save does not.
- **Branding:** a library logo is stored as its public URL; an external https
  URL is unchanged.
- **API e2e:**
  - `GET …/content` gives 302 for a member and 403 without `media:get`.
  - The list carries `url` and `preview_url`.
  - `DELETE ?unpublish=true` removes the published key and writes an audit
    row.
- **S3 adapter** (existing `blobstore_s3_test.go` style): `Put` sends no ACL;
  `Publish` sends `public-read`, `REPLACE`, and the disposition; a presigned
  URL carries `response-content-disposition`.
- **BFF:** validator accepts own-slug URLs and rejects foreign-slug and
  bucket URLs; `ImageView` emits `width`; the drift catcher stays green.
- **SPA:** `PreviewIframe` swaps library URLs for `preview_url`; the delete
  dialog shows the checkbox only when `published`.

## Out of scope

- Editing image `width` in the inspector.
- Resizing animated GIFs.
- Keeping originals.
- A separate public bucket or CDN.
- Stripping PDF metadata.
- Removing a public copy after a plain delete (operator only; see Accepted
  risks).
- Track A follow-ups: merge-tag URL schemes in `href`, the `SanitizeDocument`
  sentinel nonce, the `{{{key}}}` hint in the template editor, and permission
  on `PUT /settings`. They get their own small plan.

## Accepted risks

- **A plain delete orphans the public copy.** No UI can remove it afterwards.
  This is deliberate, so sent mail keeps working. Takedown is offered at
  delete time.
- **Transactional template images are public from the first save.** These
  templates are live at save.
- **PDF metadata (author, software) is kept.** PDFs are published only on
  send, and only as downloads.
- **A published copy is reachable by anyone who has the URL.** That is the
  point of a copy embedded in email. The key carries an unguessable UUID, and
  the bucket is not listable.

## Implementation order

1. Storage port, private `Put`, upload pipeline, migration, and the
   `…/content` endpoint and list fields.
2. `PublishAssets` and its call sites: campaign start, transactional
   templates, branding. Ship steps 1 and 2 together. After step 1 alone, sent
   emails would carry private library URLs.
3. SPA and BFF: library URL, preview swap, `mediaRef` validation, image
   `width`, delete dialog.
