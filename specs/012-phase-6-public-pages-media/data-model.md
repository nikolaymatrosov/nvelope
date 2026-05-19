# Phase 1 Data Model: Public Pages & Media

Covers four new entities/tables, new columns on two existing tables, the
relevant state machines, and the three migrations. Every tenant-scoped table
follows the project RLS pattern (`ENABLE` + `FORCE ROW LEVEL SECURITY`, a
`tenant_isolation` policy keyed on `current_setting('app.tenant_id')`, and a
`GRANT` to `nvelope_app`).

## Entity: SubscriptionPage

Per-tenant configuration of one public subscription page. A tenant may have
several. Owned by the `audience` context.

| Field | Type | Rules |
|---|---|---|
| id | uuid | PK |
| tenant_id | uuid | FK tenants, RLS key |
| slug | text | URL segment, unique per tenant, lowercase kebab |
| title | text | shown on the page, non-empty |
| target_list_ids | uuid[] | ≥1 list; all must belong to the tenant |
| fields | jsonb | ordered list of `{key, label, required}` beyond email |
| sending_domain_id | uuid | FK sending_domains; identity for opt-in email |
| from_name | text | non-empty |
| from_local_part | text | valid local-part |
| active | bool | inactive ⇒ public page shows "not available" |
| created_at / updated_at | timestamptz | |

**Validating constructor**: `NewSubscriptionPage(...)` rejects an empty
`target_list_ids`, a blank title/from-name, an invalid local-part, or a
duplicate `(tenant_id, slug)`. **Hydration**: `HydrateSubscriptionPage(...)` —
persistence only, not a constructor.

## Entity: PendingSubscription

A not-yet-confirmed public submission. Promoted to `subscriber` + `membership`
rows on confirmation, then deleted. Owned by the `audience` context.

| Field | Type | Rules |
|---|---|---|
| id | uuid | PK |
| tenant_id | uuid | FK tenants, RLS key |
| subscription_page_id | uuid | FK subscription_pages |
| email | citext | valid address |
| attributes | jsonb | submitted custom fields, validated against the page's `fields` |
| target_list_ids | uuid[] | snapshot of the page's lists at submit time |
| confirmation_token_hash | text | SHA-256 of the token; unique |
| expires_at | timestamptz | submit time + configured TTL (default 7 days) |
| created_at | timestamptz | |

Unique `(tenant_id, email, subscription_page_id)` — a repeat submit for the
same address/page refreshes the existing row (new token + expiry) rather than
stacking duplicates.

**State machine** (the row's lifecycle, not a status column):

```
(submit) ─────────────► PENDING row exists
PENDING ──confirm (valid, unexpired token)──► row deleted; subscriber promoted
PENDING ──token expired──► confirm shows "expired"; resend issues a new token
PENDING ──resend──► same row, new token_hash + new expires_at
PENDING ──TTL sweep / never confirmed──► row eventually purged, no subscriber
```

Confirmation promotes the address: upsert a `subscriber` (reuse
`SubscriberRepository.UpsertByEmail`) then `Attach`/`SetStatus` each target
list membership to `confirmed`, unless the address is suppressed (then the
membership is not confirmed and the page reports a neutral message).

## Entity: TenantBranding

Per-tenant branding applied across all of that tenant's public pages. One row
per tenant. Owned by the `tenant` context.

| Field | Type | Rules |
|---|---|---|
| tenant_id | uuid | PK + FK tenants, RLS key |
| logo_media_id | uuid \| null | optional FK media_assets |
| primary_color | text | `#rrggbb`, validated |
| custom_css | text | sanitised on save (see research D9); ≤ 64 KB |
| updated_at | timestamptz | |

**Validating constructor / mutator**: `SetCustomCSS(...)` runs the sanitiser
and rejects disallowed constructs with a typed validation error; `SetPrimaryColor`
rejects a non-hex value. Absent row ⇒ platform-default branding.

## Entity: MediaAsset

One uploaded file owned by a tenant. New `media` bounded context. Metadata
lives in `media_assets` (RLS-protected); bytes live in object storage.

| Field | Type | Rules |
|---|---|---|
| id | uuid | PK; also the unguessable key segment |
| tenant_id | uuid | FK tenants, RLS key |
| filename | text | original name, sanitised, non-empty |
| content_type | text | from an allowed-type allowlist |
| size_bytes | bigint | > 0 and ≤ configured max (default 10 MB) |
| storage_key | text | `media/{tenant_id}/{id}/{filename}` |
| public_url | text | `{public_base_url}/{storage_key}` — stable |
| uploaded_by | uuid | FK users (tenant-plane), for audit |
| created_at | timestamptz | |

**Validating constructor**: `NewMediaAsset(...)` rejects an empty filename, a
content type outside the allowlist, or a size of 0 / over the max — so an
invalid asset is unrepresentable. **Hydration**: `HydrateMediaAsset(...)`.
**Upload ordering**: object bytes are written to the `BlobStore` first; the
metadata row is inserted only after a successful put, so an interrupted upload
leaves no listed-but-missing asset (FR-029). **Delete** removes the metadata
row and the object.

`MediaRepository` (metadata) and `BlobStore` (bytes) are two interfaces
declared by the `media` domain/use-case layer; the adapters
(`assets_pg.go`, `blobstore_s3.go`) implement them.

## New columns on existing tables

**`campaigns`** (migration 000018):

| Column | Type | Rules |
|---|---|---|
| archive_visible | bool NOT NULL DEFAULT false | only a *sent* campaign may be set true |
| archived_at | timestamptz \| null | set when first made archive-visible; drives archive/RSS ordering |

`Campaign.SetArchiveVisible(v)` rejects toggling on a draft/never-sent campaign
and is a no-op-safe idempotent mutator.

**`subscribers`** (migration 000017):

| Column | Type | Rules |
|---|---|---|
| preference_token_hash | text \| null | SHA-256 of the subscriber's preference-link token; unique per tenant |

Populated on first need (subscription confirmation, or campaign send when a
footer link is required). `Subscriber` gains a `RotatePreferenceToken()`
mutator and the field is included in `HydrateSubscriber`.

## Membership state machine (reused, unchanged)

Phase 6 drives the existing `subscriber_lists.subscription_status` machine; no
new states are added:

```
unconfirmed ──confirm──► confirmed ──unsubscribe──► unsubscribed
unsubscribed ──resubscribe (via preference page)──► confirmed
```

- Public double opt-in: confirmation moves each target membership to
  `confirmed`.
- Preference page: subscriber may move a membership to `unsubscribed` or, for a
  list they previously left, back to `confirmed`.
- One-click / single-click unsubscribe: moves the relevant membership(s) to
  `unsubscribed` and adds the address to the tenant suppression scope per
  existing rules.

## Migrations

**000017_public_subscription** — `subscription_pages`, `pending_subscriptions`
(both with full RLS), and the `subscribers.preference_token_hash` column +
unique index. Down: drop in reverse.

**000018_archive_branding** — `campaigns.archive_visible` /
`campaigns.archived_at` columns; `tenant_branding` table with RLS. Down: drop
columns and table.

**000019_media_library** — `media_assets` table with RLS and a
`(tenant_id, created_at)` index for the library listing. Down: drop table.

All three follow the existing numbered `*.up.sql` / `*.down.sql` golang-migrate
format and the standard RLS block; `tenant_id` is present from the first
version of every new table and never retrofitted.

## Validation rules summary (traceability)

| Rule | Source FR |
|---|---|
| Email format validated before a pending row is created | FR-003 |
| Pending rows created unconfirmed; membership confirmed only via token | FR-004, FR-005 |
| Confirmation token single-use + `expires_at`; resend issues a new one | FR-004, FR-006 |
| Suppressed address is not silently re-subscribed on confirm | FR-008 |
| Preference token is unguessable, hashed at rest, unique | FR-010, FR-015 |
| Custom CSS sanitised + scoped on save | FR-022 |
| Only sent campaigns can be archive-visible; drafts denied public access | FR-016, FR-020 |
| Media key tenant-prefixed; metadata RLS-scoped | FR-024, FR-028 |
| Media type allowlist + size cap enforced in the constructor | FR-025 |
| Object bytes written before metadata row | FR-029 |
