# Quickstart: Phase 6 — Public Pages & Media

## Prerequisites

- Go 1.26, Docker daemon running (testcontainers + MinIO).
- An S3-compatible object store for local runs. Either point at Yandex Object
  Storage, or run MinIO locally:
  ```sh
  docker run -p 9000:9000 -p 9001:9001 \
    -e MINIO_ROOT_USER=nvelope -e MINIO_ROOT_PASSWORD=nvelope-secret \
    minio/minio server /data --console-address ":9001"
  ```
  Create a bucket (e.g. `nvelope-media`) and allow anonymous **GetObject** on
  the `media/` prefix; do **not** enable anonymous bucket listing.

## Configuration

New `internal/config` keys (env-prefixed `NVELOPE_`):

- `OBJECT_STORAGE_ENDPOINT` — S3-compatible endpoint URL.
- `OBJECT_STORAGE_REGION`, `OBJECT_STORAGE_BUCKET`.
- `OBJECT_STORAGE_ACCESS_KEY`, `OBJECT_STORAGE_SECRET_KEY` — scoped, least
  privilege (`PutObject`, `GetObject`, `DeleteObject` on the bucket).
- `OBJECT_STORAGE_PUBLIC_BASE_URL` — base for the stable `public_url`
  (e.g. `https://nvelope-media.storage.example`).
- `PUBLIC_BASE_URL` — origin used to build confirmation/preference links.
- `OPTIN_CONFIRMATION_TTL` — pending-subscription token lifetime (default
  `168h`).
- `MEDIA_MAX_BYTES` — per-file cap (default `10485760`).

## Run

```sh
make migrate          # applies 000017–000019
go run ./cmd/api      # serves admin API + public pages + RSS
go run ./cmd/worker   # runs the optin.send confirmation-email job
```

## Verify the user stories

**US1 — public subscription + double opt-in**
1. As an admin, `POST /t/{slug}/api/subscription-pages` to create a page bound
   to a list and a verified sending domain.
2. Open `GET /t/{slug}/subscribe/{page-slug}` in a browser; submit a new email.
3. Confirm the worker sent a confirmation email; open the `/c/{token}` link.
4. Check the subscriber now has a `confirmed` membership on the target list.
5. Re-submit the same address: still a single pending/subscriber row; the
   "check your email" page never reveals existing-subscriber status.

**US2 — preferences & unsubscribe**
1. Open the subscriber's `/p/{token}` preference page; change a list
   membership and a profile field; save; reload to confirm persistence.
2. Use "unsubscribe from all"; confirm the subscriber is suppressed and
   excluded from the next send.
3. `POST /u/{token}` with `List-Unsubscribe=One-Click`; confirm unsubscribe
   with no page interaction.

**US3 — archive & RSS**
1. `POST /t/{slug}/api/campaigns/{id}/archive {"visible":true}` on a sent
   campaign.
2. Open `GET /t/{slug}/archive` — the campaign is listed; open its page and
   confirm tenant branding + custom CSS render.
3. Fetch `GET /t/{slug}/feed.xml` and run it through a feed validator.
4. Confirm a draft campaign's archive URL returns `404`.

**US4 — media library**
1. `POST /t/{slug}/api/media` with an image; confirm `201` + `public_url`.
2. Fetch the `public_url` directly (no auth) — image loads.
3. `GET /t/{slug}/api/media` lists it; `DELETE` removes row + object.
4. Reject an oversized file and a disallowed type.

## Tests

```sh
make test                     # full suite, testcontainers postgres:17
go test ./internal/media/...  # media context, incl. MinIO testcontainer
go test ./test/...            # cross-tenant isolation, incl. the 4 new tables
```

Use-case tests substitute an in-memory `BlobStore` fake; the S3 adapter is
tested against a MinIO testcontainer. Tenant-isolation tests assert that
tenant B cannot read tenant A's `subscription_pages`, `pending_subscriptions`,
`tenant_branding`, or `media_assets` rows even with the application filter
removed.

## Exit criteria

- Subscribers self-serve end-to-end via public pages (US1 + US2).
- Media uploads work and are usable in campaign content (US4).
- Full test suite green; migrations 000017–000019 apply and roll back cleanly.
