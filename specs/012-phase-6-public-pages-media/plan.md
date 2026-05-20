# Implementation Plan: Phase 6 — Public Pages & Media

**Branch**: `012-phase-6-public-pages-media` | **Date**: 2026-05-19 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/012-phase-6-public-pages-media/spec.md`

## Summary

Phase 6 makes the platform self-serve for subscribers and unlocks media for
campaign authoring. It adds four backend capabilities on top of the existing
Phase 0–5 services:

1. **Public subscription + double opt-in** — an unauthenticated, server-rendered
   subscription page per tenant; submissions create a `pending_subscription`,
   send a confirmation email through a durable job, and only promote to a real
   `subscriber` membership when the confirmation link is followed.
2. **Self-serve preference management** — a token-addressed preference page
   where a subscriber edits profile fields and list memberships, plus
   single-click and RFC 8058 one-click unsubscribe.
3. **Campaign archive + RSS** — sent campaigns marked archive-visible are
   exposed as a public archive index, individual archived campaign pages, and a
   per-tenant RSS feed, all rendered with per-tenant branding and sanitised
   custom CSS.
4. **Media library** — a tenant-scoped media library backed by S3-compatible
   object storage; metadata rows are RLS-protected, object keys are
   tenant-prefixed and carry an unguessable per-asset UUID so email-embeddable
   URLs need no authentication while remaining non-enumerable.

All public pages are **server-rendered Go `html/template`** served by `cmd/api`
outside the authenticated `/t/{slug}/api` group. The React SPA stays the
admin-only surface and gains no public routes; it gains authenticated admin
screens for subscription-page config, branding, archive toggles, and the media
library only as Phase 6 *UI* (a later, separate spec — out of scope here).

This phase touches the `audience`, `campaign`, and `tenant` bounded contexts,
adds one new `media` context, adds three database migrations, and adds one new
River job type. No new service binary is introduced.

## Technical Context

**Language/Version**: Go 1.26

**Primary Dependencies**: chi v5 (routing), `html/template` + `encoding/xml`
(stdlib, public pages and RSS — no new templating/feed library), pgx v5
(PostgreSQL), River v0.37 (durable jobs), `aws-sdk-go-v2` + new
`aws-sdk-go-v2/service/s3` (object storage — the SDK is already a direct
dependency for the Postbox SES client), `redis/go-redis` via the existing
`internal/platform/ratelimit` sliding-window limiter

**Storage**: PostgreSQL (existing shared datastore, RLS per tenant) for all
metadata — `subscription_pages`, `pending_subscriptions`, `tenant_branding`,
`media_assets`, plus new columns on `campaigns` and `subscribers`.
S3-compatible object storage (Yandex Object Storage) for media bytes only.

**Testing**: `go test ./...` with testcontainers-go `postgres:17`; integration
tests for the opt-in lifecycle, preference/unsubscribe, archive/RSS rendering,
and media upload; tenant-isolation tests in `test/` extended to every new
table; the S3 adapter is exercised against a MinIO testcontainer, with a small
in-memory `BlobStore` fake used by use-case tests.

**Target Platform**: Linux server (the existing `cmd/api` and `cmd/worker`
containers)

**Project Type**: Web service (Go backend). Public pages are server-rendered
HTML from the API binary; the existing `frontend/` SPA is not modified in this
phase.

**Performance Goals**: Public pages render in well under 1 s; the archive index
and RSS feed are live queries (no rollup) so a newly archived campaign is
visible immediately (SC-005). Confirmation email delivery is asynchronous via
River and does not block the subscribe response.

**Constraints**: Public pages must be usable with no client-side JavaScript and
be tenant-scoped with zero cross-tenant leakage (SC-004); per-tenant custom CSS
must be sanitised and scoped so it cannot execute script or affect other
tenants or platform chrome (FR-022); confirmation/preference tokens are stored
only as hashes (reusing `internal/token`); subscription submissions are
rate-limited per address and per source IP (FR-009); media object keys must be
unguessable and the bucket must not be publicly listable (FR-028).

**Scale/Scope**: 3 migrations; 1 new bounded context (`internal/media/`);
extensions to 3 existing contexts (`audience`, `campaign`, `tenant`); ~1 new
River job (`optin.send`); a new public HTTP surface (~9 routes) with an
embedded `html/template` set; ~6 new authenticated admin endpoints; ~31
functional requirements; 4 independently shippable user stories.

## Constitution Check

*GATE: evaluated against `.specify/memory/constitution.md` v1.1.0.*

- **I. Tenant Isolation by Default** — PASS (with one justified design choice,
  see Complexity Tracking). Every new table (`subscription_pages`,
  `pending_subscriptions`, `tenant_branding`, `media_assets`) carries
  `tenant_id` from its first migration and gets the standard
  `ENABLE/FORCE ROW LEVEL SECURITY` + `tenant_isolation` policy; new columns
  live on already-RLS-protected tables. Public pages resolve the tenant from
  the URL slug or from a token row and then open a tenant-scoped transaction,
  so the data layer remains the authoritative backstop. Media **bytes** are
  served from object storage via unguessable capability URLs (email clients
  cannot authenticate) — isolation there rests on tenant-prefixed +
  per-asset-UUID keys and a non-listable bucket, while the media **library
  listing** stays behind the RLS-protected `media_assets` table. New
  `test/isolation_test.go` cases prove cross-tenant denial on every new table.
- **II. Test-Backed Delivery** — PASS. Integration tests cover the full opt-in
  lifecycle (submit → pending → confirm → membership), expiry and resend,
  duplicate/suppressed-address handling, preference update, single- and
  one-click unsubscribe, archive visibility filtering, RSS validity, and media
  upload/delete/isolation. The confirmation-email job is tested through the
  River worker. Phase exits with a green suite and clean migrations.
- **III. Incremental, Shippable Phases** — PASS. The four user stories ship in
  priority order — P1 public subscription + opt-in, P1 preferences/unsubscribe,
  P2 archive + RSS, P2 media library — each independently demonstrable per its
  spec Independent Test. No speculative scope: vanity domains, localisation,
  and image thumbnailing are explicitly excluded.
- **IV. Security & Consent by Design** — PASS. Double opt-in *is* the consent
  mechanism and is mandatory for public pages. Confirmation and preference
  tokens are random 32-byte values stored only as SHA-256 hashes
  (`internal/token`); confirmation tokens are single-use and time-limited.
  Submissions are rate-limited. Custom CSS is sanitised on save. Self-serve
  actions (subscribe, confirm, preference change, unsubscribe) are recorded for
  audit (FR-031). The S3 adapter uses scoped credentials behind a thin
  `BlobStore` abstraction.
- **V. Operable & Observable Services** — PASS. `cmd/api` stays stateless;
  templates are compiled once and embedded with `embed.FS`. The confirmation
  email is a durable, retry-capable River job (`optin.send`) so a restarted
  worker never drops or double-sends it. No new long-lived service.
- **VI. Layered Architecture & Domain Integrity** — PASS. The new `media`
  context follows `domain`/`app`/`adapters`; the `BlobStore` interface is
  declared by the domain/use-case layer and implemented by the S3 adapter
  (consumer-owned contract). Pending-subscription, branding, and archive logic
  extend their owning contexts as validating-constructor entities with a
  separate hydration path. HTML rendering and RSS XML live only in the
  transport layer (`internal/api`); domain code knows nothing of HTTP.
  Commands and queries stay distinct and are wired through the existing
  composition root with no DI framework.

**Result**: PASS — one design choice (public capability-URL media serving) is
recorded in Complexity Tracking as a deliberate, justified deviation from
"all tenant data behind RLS"; it is not a violation because the bytes are
content authored for public embedding in email.

*Post-design re-check*: data-model and contracts add no DI framework, no new
service binary, no duplicated infrastructure, and keep transport mapping in one
place per surface. Still PASS.

## Project Structure

### Documentation (this feature)

```text
specs/012-phase-6-public-pages-media/
├── plan.md              # This file
├── research.md          # Phase 0 output — resolved decisions
├── data-model.md        # Phase 1 output — entities, tables, state machines
├── quickstart.md        # Phase 1 output — run/verify/test instructions
├── contracts/
│   ├── public-pages.md  # Public, unauthenticated HTTP surface + RSS
│   └── admin-api.md     # Authenticated admin endpoints (config, media)
├── checklists/
│   └── requirements.md  # Spec quality checklist (/speckit-specify)
└── tasks.md             # Phase 2 output (/speckit-tasks — NOT created here)
```

### Source Code (repository root)

```text
internal/
├── audience/                       # EXTENDED
│   ├── domain/
│   │   ├── pending_subscription.go      # NEW — PendingSubscription entity + repo iface
│   │   ├── subscription_page.go         # NEW — SubscriptionPage config entity + repo iface
│   │   ├── subscriber.go                # + preference-token field + hydration
│   │   └── membership.go                # reuse existing Confirm/Unsubscribe state machine
│   ├── app/
│   │   ├── command/
│   │   │   ├── submit_public_subscription.go   # NEW — public submit → pending + enqueue
│   │   │   ├── confirm_subscription.go         # NEW — token → promote to membership
│   │   │   ├── resend_confirmation.go          # NEW — expired-link recovery
│   │   │   ├── update_preferences.go           # NEW — self-serve profile/list edit
│   │   │   ├── public_unsubscribe.go           # NEW — single + one-click unsubscribe
│   │   │   └── save_subscription_page.go       # NEW — admin config of the page
│   │   └── query/
│   │       ├── get_subscription_page.go        # NEW — render the public form
│   │       ├── get_pending_by_token.go         # NEW — confirmation landing
│   │       └── get_preferences.go              # NEW — render the preference page
│   └── adapters/
│       ├── pending_subscriptions_pg.go         # NEW
│       ├── subscription_pages_pg.go            # NEW
│       └── optin_worker.go                     # NEW — River worker: confirmation email
├── campaign/                       # EXTENDED
│   ├── domain/campaign.go               # + ArchiveVisible flag + SetArchiveVisible()
│   ├── app/command/set_archive_visibility.go   # NEW
│   ├── app/query/list_archive.go               # NEW — archive-visible campaigns
│   ├── app/query/get_archived_campaign.go      # NEW
│   └── adapters/campaigns_pg.go          # + archive columns in queries
├── tenant/                         # EXTENDED
│   ├── domain/branding.go               # NEW — TenantBranding entity + CSS sanitise + repo
│   ├── app/command/save_branding.go     # NEW
│   ├── app/query/get_branding.go        # NEW
│   └── adapters/branding_pg.go          # NEW
├── media/                          # NEW bounded context
│   ├── domain/
│   │   ├── asset.go                     # MediaAsset entity (validating constructor)
│   │   ├── repository.go                # MediaRepository interface (metadata)
│   │   └── blobstore.go                 # BlobStore interface (consumer-owned)
│   ├── app/
│   │   ├── command/upload_asset.go      # NEW — validate type/size, store, persist
│   │   ├── command/delete_asset.go      # NEW
│   │   └── query/list_assets.go         # NEW
│   └── adapters/
│       ├── assets_pg.go                 # MediaRepository (RLS-protected metadata)
│       └── blobstore_s3.go              # BlobStore impl over aws-sdk-go-v2/s3
├── platform/
│   └── jobs/jobs.go                # + OptinSendArgs job type + enqueue method
├── config/config.go                # + object-storage config (endpoint, bucket, creds, public base URL)
└── api/                            # EXTENDED — single transport layer
    ├── public_handlers.go               # NEW — subscribe/confirm/preferences/unsubscribe/archive
    ├── public_middleware.go             # NEW — resolvePublicTenant (slug → tenant, no session)
    ├── rss_handler.go                   # NEW — per-tenant RSS XML
    ├── media_handlers.go                # NEW — authenticated upload/list/delete
    ├── branding_handlers.go             # NEW — authenticated branding config
    ├── subscription_page_handlers.go    # NEW — authenticated page config + archive toggle
    ├── templates/                       # NEW — embedded html/template files
    │   ├── layout.html  subscribe.html  confirm.html  preferences.html
    │   ├── unsubscribed.html  archive_index.html  archive_campaign.html  error.html
    └── server.go                        # + public route group, media routes, embed

internal/db/migrations/
├── 000017_public_subscription.up.sql / .down.sql   # subscription_pages, pending_subscriptions, subscriber preference token
├── 000018_archive_branding.up.sql / .down.sql      # campaigns archive columns, tenant_branding
└── 000019_media_library.up.sql / .down.sql         # media_assets

cmd/
├── api/main.go        # + wire media app, branding, public handlers, template embed
└── worker/main.go     # + register OptinWorker (confirmation email)

test/
└── isolation_test.go  # + cross-tenant cases for the four new tables
```

**Structure Decision**: Existing Go web-service layout. Public-subscription and
preference logic extend the `audience` context because they operate on the
`subscribers`/`lists`/`subscriber_lists` aggregates and reuse the existing
`Membership` opt-in state machine — a separate context would split one
aggregate across two owners. Archive logic extends `campaign` (it is a read
projection over sent campaigns); branding extends `tenant` (tenant-level
config). Only **media** is genuinely homeless and becomes a new bounded context
with a full `domain`/`app`/`adapters` split, since it owns a new aggregate and
a new external dependency (object storage). All HTTP — public HTML, RSS, and
admin JSON — stays in the single `internal/api` transport layer per the
project convention; `html/template` files are embedded so `cmd/api` stays
stateless and self-contained.

## Phase 0 — Research

Complete. See [research.md](./research.md). Decisions resolved from in-repo
inspection (chi router and `resolveTenant` middleware in `internal/api`, the
`audience` opt-in state machine, the `internal/token` hashing helper, the
Postbox `Messenger` send path, the River job and enqueuer pattern, the RLS
migration pattern) and the two architecture choices confirmed with the user
(server-rendered Go templates for public pages; unguessable tenant-prefixed
capability URLs for media bytes). No `NEEDS CLARIFICATION` remain.

## Phase 1 — Design & Contracts

Complete:
- [data-model.md](./data-model.md) — the four new entities/tables
  (`SubscriptionPage`, `PendingSubscription`, `TenantBranding`, `MediaAsset`),
  the new `campaigns`/`subscribers` columns, the pending-subscription and
  membership state machines, validation rules, and the three migrations.
- [contracts/public-pages.md](./contracts/public-pages.md) — the
  unauthenticated public HTTP surface (subscription page GET/POST, confirmation,
  resend, preferences GET/POST, single- and one-click unsubscribe, archive
  index, archived campaign page, RSS feed) with status codes and error pages.
- [contracts/admin-api.md](./contracts/admin-api.md) — the authenticated admin
  endpoints (subscription-page config, branding config, campaign archive
  toggle, media upload/list/delete) with permission requirements and
  error-kind → HTTP mapping.
- [quickstart.md](./quickstart.md) — run, verify, and test instructions,
  including the MinIO testcontainer for the S3 adapter.
- Agent context (`CLAUDE.md`) updated to point at this plan.

## Phase 2 — Next step

Run `/speckit-tasks` to generate `tasks.md`. Suggested task ordering:

1. **Shared plumbing** — migrations 000017–000019; object-storage config keys;
   the `OptinSendArgs` job type + enqueue method; the embedded
   `html/template` layout and `resolvePublicTenant` middleware.
2. **US1 Public subscription + double opt-in** (P1) — `SubscriptionPage` config
   entity/repo + admin save; `PendingSubscription` entity/repo; the public
   subscribe GET/POST handlers; `SubmitPublicSubscription`,
   `ConfirmSubscription`, `ResendConfirmation` commands; the `OptinWorker`
   confirmation-email job; rate limiting on submit.
3. **US2 Preferences & unsubscribe** (P1) — subscriber preference-token column;
   `GetPreferences` query; `UpdatePreferences` and `PublicUnsubscribe`
   commands; preference page GET/POST and the single-/one-click unsubscribe
   routes; List-Unsubscribe header integration note for the sending pipeline.
4. **US3 Archive + RSS + branding** (P2) — `campaigns` archive columns +
   `SetArchiveVisibility`; `TenantBranding` entity with CSS sanitisation +
   admin config; `ListArchive`/`GetArchivedCampaign` queries; archive index,
   archived campaign page, and RSS handlers.
5. **US4 Media library** (P2) — the `media` bounded context: `MediaAsset`
   domain, `MediaRepository` (`media_assets` with RLS), `BlobStore` interface +
   S3 adapter, `UploadAsset`/`DeleteAsset`/`ListAssets` use cases, the
   authenticated media endpoints, and the in-memory `BlobStore` fake.
6. **Isolation & verification** — extend `test/isolation_test.go` for all four
   new tables; full suite + clean-migration check.

Each user story is independently demonstrable and testable per its spec
Independent Test.

## Complexity Tracking

| Violation / Deviation | Why Needed | Simpler Alternative Rejected Because |
|-----------------------|------------|--------------------------------------|
| Media bytes served from object storage via unguessable public capability URLs rather than an RLS-gated, app-authorised path | Images embedded in campaign emails must be fetchable by email clients that carry no session or credentials; a login-gated URL cannot render in an inbox. The archive reuses the same long-lived URLs. | App-proxied, authorised serving was rejected because it cannot satisfy anonymous email-client fetches at all; presigned, expiring URLs were rejected because archived campaigns and already-delivered emails would break when the signature expires. Isolation is preserved by tenant-prefixed keys carrying a random per-asset UUID, a non-listable bucket, and keeping the media-library *listing* behind the RLS-protected `media_assets` table. |
| New `media` bounded context (a 9th context) | Media owns a new aggregate and a new external dependency (object storage) with no existing owner; folding it into `campaign` would couple campaign sending to blob storage. | Extending an existing context was rejected because no current context owns files or object storage; the new context is minimal (domain/app/adapters) and adds no service binary. |
