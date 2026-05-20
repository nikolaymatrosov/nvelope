---
description: "Task list for Phase 6 — Public Pages & Media"
---

# Tasks: Phase 6 — Public Pages & Media

**Input**: Design documents from `/specs/012-phase-6-public-pages-media/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/

**Tests**: Test tasks ARE included — Constitution Principle II (Test-Backed
Delivery) is non-negotiable, and the spec defines an Independent Test per story.
Critical paths (tenant isolation, the opt-in lifecycle, async jobs) get
integration coverage against real boundaries.

**Organization**: Tasks are grouped by user story for independent
implementation and testing.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: US1–US4 maps to the spec user stories
- Exact file paths are in each description

## Path Conventions

Go web service. Backend code under `internal/`, services under `cmd/`,
migrations under `internal/db/migrations/`, cross-tenant tests under `test/`.
Context-local tests are colocated `*_test.go` files.

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Repo-level prerequisites for Phase 6.

- [X] T001 Add `github.com/aws/aws-sdk-go-v2/service/s3` (and `config`/`credentials` modules) to `go.mod` and run `go mod tidy`
- [X] T002 [P] Add object-storage + public-page config keys to `internal/config/config.go` (`OBJECT_STORAGE_ENDPOINT/REGION/BUCKET/ACCESS_KEY/SECRET_KEY`, `OBJECT_STORAGE_PUBLIC_BASE_URL`, `PUBLIC_BASE_URL`, `OPTIN_CONFIRMATION_TTL`, `MEDIA_MAX_BYTES`) with defaults, and cover parsing in `internal/config/config_test.go`
- [X] T003 [P] Update `quickstart.md` env list and `deploy/` env templates with the new config keys

**Checkpoint**: Project builds with new config and the S3 SDK available.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Shared public-page transport infrastructure used by US1–US3.

**⚠️ CRITICAL**: No public-page user story can begin until this phase is complete.

- [X] T004 Create the embedded template set: `internal/api/templates/layout.html` and `internal/api/templates/error.html`, plus a Go `embed.FS` + parsed `*template.Template` loader in `internal/api/templates.go` (auto-escaping; `.nv-public` wrapper; branding placeholders)
- [X] T005 Implement `resolvePublicTenant` middleware in `internal/api/public_middleware.go` — resolve `/t/{slug}/...` to a tenant with no session, open the tenant-scoped transaction, render branded `error.html` "not available" on unknown/inactive tenant
- [X] T006 [P] Add the `OptinSendArgs{TenantID, PendingSubscriptionID}` job type and `EnqueueOptinSend` method to `internal/platform/jobs/jobs.go`
- [X] T007 Register the public route group (no auth) and a `resolvePublicTenant`-scoped subtree in `internal/api/server.go`, and wire the template loader into the `Server` struct

**Checkpoint**: Public routing, templates, and the opt-in job type exist — user stories can begin.

---

## Phase 3: User Story 1 - Self-serve subscription with double opt-in (Priority: P1) 🎯 MVP

**Goal**: A visitor subscribes via a public page, receives a confirmation
email, and becomes an active subscriber only after confirming.

**Independent Test**: Create a subscription page via the admin API, submit a
new email on the public page, open the confirmation link from the sent email,
and verify the subscriber has a `confirmed` membership; an unconfirmed submit
leaves only a pending row.

### Migration & domain (US1)

- [X] T008 [US1] Write migration `internal/db/migrations/000017_public_subscription.up.sql` / `.down.sql` — `subscription_pages` and `pending_subscriptions` tables (full RLS block) and the `subscribers.preference_token_hash` column + unique index
- [X] T009 [P] [US1] Create the `SubscriptionPage` entity (validating constructor + `Hydrate*`) in `internal/audience/domain/subscription_page.go` with its `SubscriptionPageRepository` interface
- [X] T010 [P] [US1] Create the `PendingSubscription` entity (validating constructor + `Hydrate*`, token/expiry rules) in `internal/audience/domain/pending_subscription.go` with its `PendingSubscriptionRepository` interface
- [X] T011 [US1] Add `preference_token_hash` + `RotatePreferenceToken()` to `internal/audience/domain/subscriber.go` and its `HydrateSubscriber` path

### Adapters (US1)

- [X] T012 [P] [US1] Implement `SubscriptionPagesRepository` (RLS tx) in `internal/audience/adapters/subscription_pages_pg.go`
- [X] T013 [P] [US1] Implement `PendingSubscriptionsRepository` (RLS tx, upsert-by-email-and-page) in `internal/audience/adapters/pending_subscriptions_pg.go`

### Use cases (US1)

- [X] T014 [P] [US1] Implement `SaveSubscriptionPage` command (create/update, list/domain ownership checks) in `internal/audience/app/command/save_subscription_page.go`
- [X] T015 [P] [US1] Implement `GetSubscriptionPage` query in `internal/audience/app/query/get_subscription_page.go`
- [X] T016 [US1] Implement `SubmitPublicSubscription` command (validate email/fields, rate-limit via `internal/platform/ratelimit`, upsert pending row, enqueue `optin.send` in the same tx) in `internal/audience/app/command/submit_public_subscription.go`
- [X] T017 [P] [US1] Implement `GetPendingByToken` query in `internal/audience/app/query/get_pending_by_token.go`
- [X] T018 [US1] Implement `ConfirmSubscription` command (token + expiry check, suppression check, upsert subscriber, confirm memberships, delete pending row, idempotent) in `internal/audience/app/command/confirm_subscription.go`
- [X] T019 [US1] Implement `ResendConfirmation` command (new token + expiry, re-enqueue job) in `internal/audience/app/command/resend_confirmation.go`

### Confirmation email job (US1)

- [X] T020 [US1] Implement `OptinWorker` River worker in `internal/audience/adapters/optin_worker.go` — render the confirmation email from a Go template and send via the `campaign` `Messenger` using the page's verified sending domain; register it in `cmd/worker/main.go`

### Transport (US1)

- [X] T021 [P] [US1] Create the `subscribe.html` and `confirm.html` templates in `internal/api/templates/`
- [X] T022 [US1] Implement public subscribe handlers (`GET`/`POST /t/{slug}/subscribe/{page-slug}`) and confirmation handlers (`GET /c/{token}`, `POST /c/{token}/resend`) in `internal/api/public_handlers.go`; mount the routes in `server.go`
- [X] T023 [US1] Implement authenticated subscription-page admin handlers (`GET`/`POST /t/{slug}/api/subscription-pages`, `PUT /.../{id}`) in `internal/api/subscription_page_handlers.go`; add the `subscription_page:manage` permission to the IAM permission union
- [X] T024 [US1] Add the US1 typed-error mappings (`validation_failed`, `subscription_page_slug_taken`, `list_not_found`, `sending_domain_not_found`) to `internal/api/errmap.go`
- [X] T025 [US1] Wire the US1 commands/queries, repositories, and the `audience` application into the composition root in `cmd/api/main.go`

### Tests (US1)

- [X] T026 [P] [US1] Integration test of the opt-in lifecycle (submit → pending → confirm → `confirmed` membership; expiry; resend; duplicate-address neutrality; suppressed-address no-resubscribe) in `internal/audience/adapters/pending_subscriptions_pg_test.go` / a `*_test.go` exercising the commands
- [X] T027 [P] [US1] Cross-tenant isolation test for `subscription_pages` and `pending_subscriptions` in `test/isolation_test.go`
- [X] T028 [P] [US1] Test that the `OptinWorker` sends through a fake `Messenger` in `internal/audience/adapters/optin_worker_test.go`

**Checkpoint**: US1 is fully functional — public subscription with double opt-in works end to end.

---

## Phase 4: User Story 2 - Preference management & self-serve unsubscribe (Priority: P1)

**Goal**: A subscriber opens a token link to update profile fields and list
memberships, and can unsubscribe (single-click, one-click, or per-list).

**Independent Test**: Generate a preference link for a known subscriber, change
a list membership and a profile field, save, reload to confirm persistence;
use the unsubscribe action and verify the subscriber stops receiving campaigns.

### Use cases (US2)

- [X] T029 [P] [US2] Implement `GetPreferences` query (resolve subscriber by preference token, return profile + per-list membership state) in `internal/audience/app/query/get_preferences.go`
- [X] T030 [US2] Implement `UpdatePreferences` command (validate + apply profile and per-list membership changes immediately) in `internal/audience/app/command/update_preferences.go`
- [X] T031 [US2] Implement `PublicUnsubscribe` command (move memberships to `unsubscribed`, add to tenant suppression scope) in `internal/audience/app/command/public_unsubscribe.go`

### Transport (US2)

- [X] T032 [P] [US2] Create the `preferences.html` and `unsubscribed.html` templates in `internal/api/templates/`
- [X] T033 [US2] Implement preference + unsubscribe handlers (`GET`/`POST /p/{token}`, `GET`/`POST /u/{token}` with RFC 8058 `List-Unsubscribe=One-Click` body handling) in `internal/api/public_handlers.go`; mount the routes
- [X] T034 [US2] Add the `List-Unsubscribe` / `List-Unsubscribe-Post` headers (pointing at `/u/{token}`) to outbound campaign mail in the `campaign` sending path, ensuring each recipient's subscriber has a preference token
- [X] T035 [US2] Wire the US2 commands/queries into `cmd/api/main.go`

### Tests (US2)

- [X] T036 [P] [US2] Integration test: preference update persists, per-list change applies, invalid/tampered/expired token denied with no data exposure, single- and one-click unsubscribe both suppress in `internal/audience/app/command/update_preferences_test.go`
- [X] T037 [P] [US2] Test that an unsubscribed subscriber is excluded from a subsequent campaign send (recipient resolution) in the `campaign` test package

**Checkpoint**: US1 + US2 both work — subscribers can fully self-serve.

---

## Phase 5: User Story 3 - Public campaign archive & RSS feed (Priority: P2)

**Goal**: Tenants expose archive-visible campaigns as a public archive index,
individual archived pages, and an RSS feed, all with per-tenant branding/CSS.

**Independent Test**: Mark a sent campaign archive-visible, open the public
archive index and confirm it lists and renders with tenant branding, and
confirm the RSS feed validates and includes the campaign.

### Migration & domain (US3)

- [X] T038 [US3] Write migration `internal/db/migrations/000018_archive_branding.up.sql` / `.down.sql` — `campaigns.archive_visible` + `campaigns.archived_at` columns and the `tenant_branding` table (full RLS block)
- [X] T039 [P] [US3] Add `ArchiveVisible`/`archivedAt` + `SetArchiveVisible()` (rejects draft/never-sent) to `internal/campaign/domain/campaign.go` and its hydration path
- [X] T040 [P] [US3] Create the `TenantBranding` entity in `internal/tenant/domain/branding.go` — validating constructor, `SetPrimaryColor` (hex check), `SetCustomCSS` (sanitiser per research D9), and the `BrandingRepository` interface

### Adapters (US3)

- [X] T041 [P] [US3] Implement `BrandingRepository` (RLS tx, upsert) in `internal/tenant/adapters/branding_pg.go`
- [X] T042 [US3] Add `archive_visible`/`archived_at` to the read/write queries in `internal/campaign/adapters/campaigns_pg.go`

### Use cases (US3)

- [X] T043 [P] [US3] Implement `SetArchiveVisibility` command in `internal/campaign/app/command/set_archive_visibility.go`
- [X] T044 [P] [US3] Implement `ListArchive` and `GetArchivedCampaign` queries (archive-visible only, newest-first by `archived_at`) in `internal/campaign/app/query/list_archive.go` and `get_archived_campaign.go`
- [X] T045 [P] [US3] Implement `SaveBranding` command and `GetBranding` query in `internal/tenant/app/command/save_branding.go` and `internal/tenant/app/query/get_branding.go`

### Transport (US3)

- [X] T046 [P] [US3] Create the `archive_index.html` and `archive_campaign.html` templates in `internal/api/templates/`
- [X] T047 [US3] Implement archive handlers (`GET /t/{slug}/archive`, `GET /t/{slug}/archive/{campaign-id}` — 404 on draft/hidden) in `internal/api/public_handlers.go`
- [X] T048 [US3] Implement the RSS handler (`GET /t/{slug}/feed.xml`, RSS 2.0 via `encoding/xml`, valid-but-empty for zero campaigns) in `internal/api/rss_handler.go`
- [X] T049 [US3] Implement authenticated branding handlers (`GET`/`PUT /t/{slug}/api/branding`) in `internal/api/branding_handlers.go` and the archive-toggle handler (`POST /t/{slug}/api/campaigns/{id}/archive`); add the `branding:manage` permission; render branding/sanitised CSS into the public layout
- [X] T050 [US3] Add the US3 typed-error mappings (`invalid_color`, `unsafe_css`, `campaign_not_sent`) to `internal/api/errmap.go`; wire the US3 commands/queries into `cmd/api/main.go`

### Tests (US3)

- [X] T051 [P] [US3] Integration test: only sent campaigns can be archive-visible, drafts/hidden return 404, archive index orders newest-first, RSS output is well-formed in the `campaign` test package
- [X] T052 [P] [US3] Test the CSS sanitiser rejects `</style>`, `@import`, `expression(`, `javascript:`, and non-https `url()` in `internal/tenant/domain/branding_test.go`
- [X] T053 [P] [US3] Cross-tenant isolation test for `tenant_branding` in `test/isolation_test.go`

**Checkpoint**: US1–US3 all work independently.

---

## Phase 6: User Story 4 - Tenant media library (Priority: P2)

**Goal**: Tenants upload media to S3-compatible storage, browse their library,
and reference assets in campaigns, with strict per-tenant isolation.

**Independent Test**: Upload an image, confirm it appears in the library with a
usable URL, and confirm a different tenant cannot see or fetch that file.

### Migration & domain (US4)

- [X] T054 [US4] Write migration `internal/db/migrations/000019_media_library.up.sql` / `.down.sql` — `media_assets` table (full RLS block) with a `(tenant_id, created_at)` index
- [X] T055 [P] [US4] Create the `MediaAsset` entity (validating constructor: type allowlist, size cap, non-empty filename; `Hydrate*`) in `internal/media/domain/asset.go`
- [X] T056 [P] [US4] Declare the `MediaRepository` interface in `internal/media/domain/repository.go` and the `BlobStore` interface in `internal/media/domain/blobstore.go`

### Adapters (US4)

- [X] T057 [P] [US4] Implement `MediaRepository` (RLS tx) in `internal/media/adapters/assets_pg.go`
- [X] T058 [P] [US4] Implement the S3 `BlobStore` adapter (tenant-prefixed `media/{tenantID}/{assetID}/{filename}` keys, `Put`/`Delete`, public-URL build) in `internal/media/adapters/blobstore_s3.go`
- [X] T059 [P] [US4] Implement an in-memory `BlobStore` fake for use-case tests in `internal/media/adapters/blobstore_memory.go`

### Use cases (US4)

- [X] T060 [US4] Implement `UploadAsset` command (validate, write bytes to `BlobStore` first, then insert the metadata row) in `internal/media/app/command/upload_asset.go`
- [X] T061 [P] [US4] Implement `DeleteAsset` command (remove metadata row + object) in `internal/media/app/command/delete_asset.go`
- [X] T062 [P] [US4] Implement `ListAssets` query in `internal/media/app/query/list_assets.go`
- [X] T063 [US4] Create the `media` application assembler (`internal/media/app/app.go`) wiring commands/queries with the standard decorators

### Transport (US4)

- [X] T064 [US4] Implement authenticated media handlers (`GET`/`POST /t/{slug}/api/media`, `DELETE /.../{id}`) in `internal/api/media_handlers.go`; add the `media:get`/`media:manage` permissions; add the US4 typed-error mappings (`unsupported_media_type`, `media_too_large`, `empty_upload`, `media_not_found`) to `internal/api/errmap.go`
- [X] T065 [US4] Construct the S3 `BlobStore`, wire the `media` application into the composition root, and mount the media routes in `cmd/api/main.go` and `internal/api/server.go`

### Tests (US4)

- [X] T066 [P] [US4] Integration test: upload/list/delete, size-cap and type-allowlist rejection, interrupted upload leaves no listed asset, against a MinIO testcontainer in `internal/media/adapters/blobstore_s3_test.go`
- [X] T067 [P] [US4] Use-case tests for `UploadAsset`/`DeleteAsset`/`ListAssets` with the in-memory `BlobStore` fake in `internal/media/app/command/upload_asset_test.go`
- [X] T068 [P] [US4] Cross-tenant isolation test for `media_assets` in `test/isolation_test.go`

**Checkpoint**: All four user stories are independently functional.

---

## Phase 7: Polish & Cross-Cutting Concerns

**Purpose**: Phase-exit verification and final consistency.

- [X] T069 [P] Verify all three migrations apply and roll back cleanly (`make migrate` up/down) and that `test/migrate_test.go` covers 000017–000019
- [X] T070 [P] Run `make test` (full suite + tenant-isolation tests) and confirm green
- [X] T071 [P] Walk through every story in `quickstart.md` and confirm both exit criteria (subscribers self-serve; media uploads work)
- [X] T072 [P] Update `docs/architecture.md` and `docs/implementation-plan.md` to mark Phase 6 / Epics G & H complete and document the public-page + media surfaces

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — start immediately.
- **Foundational (Phase 2)**: Depends on Setup — BLOCKS all public-page stories (US1–US3). US4 (media) depends only on Setup.
- **User Stories (Phase 3–6)**: Depend on their phase prerequisites below.
- **Polish (Phase 7)**: Depends on all desired stories being complete.

### User Story Dependencies

- **US1 (P1)**: Setup + Foundational. No dependency on other stories. 🎯 MVP.
- **US2 (P1)**: Setup + Foundational + migration 000017 from US1 (the
  `preference_token_hash` column ships in 000017). Otherwise independent — can
  be tested with a manually created subscriber.
- **US3 (P2)**: Setup + Foundational. Independent of US1/US2.
- **US4 (P2)**: Setup only (no public-page transport needed — admin API). Fully
  independent; can run in parallel with US1–US3.

### Within Each User Story

- Migration → domain entities → adapters → use cases → transport → tests.
- Tests for critical paths exercise real boundaries (DB, River, MinIO).
- Story complete and independently testable before moving to the next priority.

### Parallel Opportunities

- Setup: T002 and T003 are [P].
- Foundational: T006 is [P] with T004/T005.
- US4 (media) is fully independent and can be developed in parallel with the
  entire public-page track (US1–US3) by a separate developer.
- Within each story, [P]-marked tasks (different files) run in parallel — see
  examples below.

---

## Parallel Example: User Story 1

```bash
# After migration T008, the two new entities are independent:
Task: "Create SubscriptionPage entity in internal/audience/domain/subscription_page.go"
Task: "Create PendingSubscription entity in internal/audience/domain/pending_subscription.go"

# After entities, the two repositories are independent:
Task: "Implement SubscriptionPagesRepository in internal/audience/adapters/subscription_pages_pg.go"
Task: "Implement PendingSubscriptionsRepository in internal/audience/adapters/pending_subscriptions_pg.go"

# After adapters, the read-side use cases are independent:
Task: "Implement GetSubscriptionPage query in internal/audience/app/query/get_subscription_page.go"
Task: "Implement GetPendingByToken query in internal/audience/app/query/get_pending_by_token.go"
```

---

## Implementation Strategy

### MVP First (User Story 1 only)

1. Phase 1: Setup.
2. Phase 2: Foundational (blocks US1–US3).
3. Phase 3: US1 — public subscription + double opt-in.
4. **STOP and VALIDATE**: run the US1 Independent Test; demo.

### Incremental Delivery

1. Setup + Foundational → foundation ready.
2. US1 → test → demo (MVP — subscribers can subscribe).
3. US2 → test → demo (subscribers fully self-serve — first exit criterion met).
4. US3 → test → demo (archive + RSS).
5. US4 → test → demo (media uploads work — second exit criterion met).

### Parallel Team Strategy

After Setup, one developer takes the media track (US4, independent of the
public-page transport) while the public-page track completes Foundational and
then US1 → US2 → US3. The two tracks integrate only at `cmd/api/main.go` wiring.

---

## Notes

- [P] = different files, no dependency on an incomplete task.
- [Story] label maps each task to its spec user story for traceability.
- Every new table gets the standard RLS block; isolation tests (T027, T053,
  T068) are review gates, not afterthoughts.
- Commit after each task or logical group.
- Stop at any checkpoint to validate a story independently.
