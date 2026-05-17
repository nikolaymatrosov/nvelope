---
description: "Task list for Phase 3 — Sending Pipeline"
---

# Tasks: Phase 3 — Sending Pipeline

**Input**: Design documents from `/specs/006-phase-3-sending-pipeline/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/

**Tests**: Test tasks ARE included — the project constitution makes Test-Backed
Delivery non-negotiable, and the critical paths in this phase (email sending,
async job processing, rate limiting, tenant isolation) require integration
coverage against real boundaries.

**Organization**: Tasks are grouped by user story so each story can be
implemented, tested, and demonstrated independently.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[Story]**: US1 / US2 / US3 — maps to a spec.md user story
- Every task includes an exact file path

## Path Conventions

Go backend, calibrated DDD layout per `PATTERNS.md`: bounded contexts under
`internal/<ctx>/{domain,app,adapters}`, shared transport in `internal/api`,
shared infrastructure in `internal/platform`, composition root in
`internal/service`, migrations in `internal/db/migrations`.

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Pull in the two new dependencies and the Redis test container.

- [X] T001 Add `github.com/redis/go-redis/v9` and the standalone AWS SigV4 signer (`github.com/aws/aws-sdk-go-v2/aws`, `.../aws/signer/v4`, `.../credentials`) to `go.mod`; run `go mod tidy` and verify the build still compiles with `go build ./...`
- [X] T002 [P] Add a `redis:7` service to `docker-compose.yml` and a `NVELOPE_REDIS_URL` entry to `.env.example`
- [X] T003 [P] Add a Redis testcontainer helper in `internal/dbtest/` (e.g. `redis.go`) that starts a `redis:7` container and returns its DSN, mirroring the existing Postgres harness

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Configuration, the Postbox client, the Redis rate limiter, and the
River job-args extension — all shared by every user story.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete.

- [X] T004 Extend `internal/config/config.go` with the Phase 3 settings: `PostboxRegion`, `PostboxEndpoint`, `PostboxAccessKeyID`, `PostboxSecretAccessKey`, `RedisURL`, `WorkerSendQueue`, `GlobalSendRateLimit`, `GlobalSendRateWindow`, `DefaultTenantSendRateLimit`, `DefaultTenantSendRateWindow`, `SendingDomainVerifyInterval`, `SendingDomainVerifyWindow`, `CampaignBatchSize`; apply defaults and fail-fast validation for required Postbox/Redis values; mark the secret fields so they are never logged
- [X] T005 [P] Implement AWS SigV4 request signing in `internal/platform/postbox/sigv4.go` — wrap `aws/signer/v4` with static credentials, service name `ses`, configured region
- [X] T006 [P] Implement the Postbox SES-compatible HTTP client in `internal/platform/postbox/client.go` — `New(Config)`, `CreateEmailIdentity`, `GetEmailIdentity`, `SendEmail`, each signing requests via T005
- [X] T007 [P] Unit-test the Postbox client + SigV4 against a recorded/stubbed HTTP server in `internal/platform/postbox/client_test.go`; add an opt-in integration test gated by `NVELOPE_POSTBOX_INTEGRATION` in `internal/platform/postbox/integration_test.go`
- [X] T008 [P] Implement the Redis sliding-window limiter in `internal/platform/ratelimit/limiter.go` — `New(redisURL, globalLimit)` and `Allow(ctx, tenantID, perTenantLimit) (allowed, retryAfter, err)`, using one atomic Lua script over a sorted-set log per key (`rl:tenant:{id}`, `rl:global`) with a one-window TTL
- [X] T009 [P] Integration-test the rate limiter in `internal/platform/ratelimit/limiter_test.go` against the Redis container (T003): assert per-tenant and global limits hold under concurrent goroutines and that `retryAfter` is correct
- [X] T010 Extend `internal/platform/jobs/jobs.go` with the three new job-arg types and kinds — `DomainVerifyArgs`/`domain.verify`, `CampaignStartArgs`/`campaign.start`, `CampaignBatchArgs`/`campaign.batch` — plus `EnqueueVerify`, `EnqueueStart`, `EnqueueBatch` enqueuer methods and a `WorkerSendQueue`-aware worker-client constructor
- [X] T011 [P] Unit-test the new enqueuers in `internal/platform/jobs/jobs_test.go` (job kinds stable, payloads carry only identifiers)

**Checkpoint**: Shared infrastructure ready — user stories can now begin.

---

## Phase 3: User Story 1 — Verify a sending domain (Priority: P1) 🎯 MVP

**Goal**: A tenant adds a sending domain, receives DKIM/SPF/DMARC DNS records,
and the platform polls until the domain reaches `verified` or `failed`.

**Independent Test**: In a clean tenant, add a domain, confirm DNS records are
returned with status `pending`, publish them on a real test domain, and confirm
automatic transition to `verified`; confirm an unverified domain ends `failed`
after the window and cannot be used to send.

### Migration & domain layer

- [ ] T012 [US1] Write migration `internal/db/migrations/000008_sending_domains.{up,down}.sql` — the `sending_domains` table with RLS (`ENABLE`/`FORCE`, `tenant_isolation` policy), constraints and indexes per data-model.md
- [ ] T013 [P] [US1] Implement the `SendingDomain` aggregate in `internal/sending/domain/domain.go` — unexported fields, validating constructor, documented hydration path, `MarkVerified`/`MarkFailed`/`RecordCheck` transition methods
- [ ] T014 [P] [US1] Define the context's slug errors in `internal/sending/domain/errors.go` (`domain-invalid`, `domain-already-exists`, `domain-not-found`, `domain-not-pending`, `provisioning-failed`) via the shared `apperr` package
- [ ] T015 [P] [US1] Declare the domain-owned interfaces — `SendingDomainRepository` in `internal/sending/domain/repository.go`, `DomainProvisioner` + `IdentityVerifier` in `internal/sending/domain/provisioner.go`
- [ ] T016 [P] [US1] Unit-test the `SendingDomain` entity transitions and constructor validation in `internal/sending/domain/domain_test.go`

### Adapters

- [ ] T017 [US1] Implement `SendingDomainRepository` (pgx + RLS, closure-based `Update`) in `internal/sending/adapters/domains_pg.go`
- [ ] T018 [P] [US1] Implement the Postbox-backed `DomainProvisioner` + `IdentityVerifier` adapter in `internal/sending/adapters/provisioner_postbox.go`, wrapping the `platform/postbox` client (T006); compose the platform's SPF/DMARC records
- [ ] T019 [US1] Implement the `domain.verify` River worker in `internal/sending/adapters/verify_worker.go` — load row, check identity, `MarkVerified`/`MarkFailed`/snooze per research.md R4 and contracts/jobs.md
- [ ] T020 [US1] Integration-test `domains_pg.go` in `internal/sending/adapters/domains_pg_test.go` against Postgres, including a cross-tenant isolation test (tenant A cannot read/write tenant B's domains even with the app-level filter omitted)
- [ ] T021 [US1] Component-test the `verify_worker` in `internal/sending/adapters/verify_worker_test.go` using a fake verifier — verified path, failed-after-window path, snooze-while-pending path

### App layer

- [ ] T022 [P] [US1] Implement the `AddDomain` and `RecheckDomain` commands in `internal/sending/app/command/domains.go` (AddDomain provisions synchronously then enqueues `domain.verify`; RecheckDomain rejects non-`pending` domains)
- [ ] T023 [P] [US1] Implement the `ListDomains` and `GetDomain` queries in `internal/sending/app/query/domains.go` returning read-model views
- [ ] T024 [US1] Assemble `internal/sending/app/application.go` (`Application{Commands, Queries}`) and declare the `DomainVerifyEnqueuer` interface in the app layer
- [ ] T025 [US1] Unit-test the `AddDomain`/`RecheckDomain` commands in `internal/sending/app/command/domains_test.go` with fake repository, provisioner, and enqueuer

### Transport & wiring

- [ ] T026 [US1] Implement the sending-domain HTTP handlers in `internal/api/sending_handlers.go` — `POST/GET /sending-domains`, `GET /sending-domains/{id}`, `POST /sending-domains/{id}/recheck` per contracts/http-api.md
- [ ] T027 [US1] Mount the sending-domain routes in `internal/api/server.go` inside the `/t/{slug}/api` authz group, and extend `internal/api/errmap.go` to map the T014 error slugs to status codes
- [ ] T028 [US1] Wire the `sending` context in `internal/service/application.go` (new `buildSending` function — repository, Postbox provisioner adapter, enqueuer, decorated handlers) and add it to the `Application` struct and `internal/api/server.go`'s `New`
- [ ] T029 [US1] Register the `verify_worker` in `cmd/worker/main.go` and add the `WorkerSendQueue` to the worker client config
- [ ] T030 [US1] Implement the periodic `domain.verify` recovery sweep in `cmd/scheduler/main.go` — enqueue a unique job (keyed on domain ID) for each still-`pending` domain
- [ ] T031 [US1] Add an API-level integration test in `internal/api/sending_handlers_test.go` covering add → records returned → recheck → status, including the isolation case

**Checkpoint**: US1 fully functional — a tenant can verify a sending domain.

---

## Phase 4: User Story 2 — Create and send a campaign with tracking (Priority: P1)

**Goal**: A tenant authors a campaign (optionally from a template), targets
lists/segments, starts the send, and every recipient receives exactly one
tracked message; sends are rate-limited, resumable, and auto-pause on error.

**Independent Test**: With a verified domain and a list of test subscribers,
create a campaign from a template, start it, and confirm every subscriber
receives the message once, links are rewritten, the pixel is present, progress
counts are accurate, and a worker restart mid-send produces no duplicates.

### Migrations & domain layer

- [ ] T032 [US2] Write migration `internal/db/migrations/000009_templates_campaigns.{up,down}.sql` — `templates`, `campaigns`, `campaign_lists`, `campaign_recipients` tables with RLS, constraints (including `UNIQUE (campaign_id, email)`) and indexes per data-model.md
- [ ] T033 [US2] Write migration `internal/db/migrations/000010_campaign_tracking.{up,down}.sql` — `links`, `link_clicks`, `campaign_views` tables with RLS, constraints and indexes per data-model.md
- [ ] T034 [P] [US2] Implement the `Template` aggregate in `internal/campaign/domain/template.go` (constructor validation, `campaign`/`transactional` kind)
- [ ] T035 [P] [US2] Implement the `Campaign` aggregate in `internal/campaign/domain/campaign.go` — lifecycle methods `Start`/`Pause`/`Resume`/`Finish`/`Cancel`/`RecordProgress`, draft-only editing, start preconditions
- [ ] T036 [P] [US2] Implement the `CampaignRecipient` type in `internal/campaign/domain/recipient.go` (status, `MarkSent`/`MarkFailed`)
- [ ] T037 [P] [US2] Implement link-rewriting and open-pixel insertion in `internal/campaign/domain/tracking.go` (rewrite tracked URLs to `/l/{id}?s=`, append `/o/{id}?s=` pixel)
- [ ] T038 [P] [US2] Define the context's slug errors in `internal/campaign/domain/errors.go` (`template-name-taken`, `template-invalid`, `template-not-found`, `template-kind-mismatch`, `campaign-invalid`, `campaign-not-found`, `campaign-not-draft`, `campaign-not-editable`, `sending-domain-required`, `campaign-no-recipients`)
- [ ] T039 [P] [US2] Declare the domain-owned interfaces — `TemplateRepository`, `CampaignRepository`, `RecipientRepository`, `TrackingRepository` in `internal/campaign/domain/repository.go`; `Messenger` + `RateLimiter` in `internal/campaign/domain/messenger.go`
- [ ] T040 [P] [US2] Unit-test the `Campaign` lifecycle, `Template` validation, and link/pixel rewriting in `internal/campaign/domain/campaign_test.go` and `tracking_test.go`

### Adapters

- [ ] T041 [P] [US2] Implement `TemplateRepository` (pgx + RLS) in `internal/campaign/adapters/templates_pg.go`
- [ ] T042 [P] [US2] Implement `CampaignRepository` (pgx + RLS, closure `Update`) in `internal/campaign/adapters/campaigns_pg.go`
- [ ] T043 [P] [US2] Implement `RecipientRepository` in `internal/campaign/adapters/recipients_pg.go` — `BulkInsert` with `ON CONFLICT (campaign_id, email) DO NOTHING`, `Pending`, `MarkSent`/`MarkFailed`, `Counts`
- [ ] T044 [P] [US2] Implement `TrackingRepository` in `internal/campaign/adapters/tracking_pg.go` — `UpsertLinks`, `RecordClick`, `RecordView`, `ResolveTenantForLink`/`...ForCampaign`
- [ ] T045 [P] [US2] Implement the Postbox-backed `Messenger` adapter in `internal/campaign/adapters/messenger_postbox.go` — build the MIME message, set `X-Tenant`/`X-Campaign`/`X-Subscriber` headers, call `platform/postbox` `SendEmail`
- [ ] T046 [P] [US2] Implement the `RateLimiter` adapter in `internal/campaign/adapters/ratelimiter.go` wrapping `platform/ratelimit` (T008)
- [ ] T047 [US2] Implement the `campaign.start` River worker in `internal/campaign/adapters/start_worker.go` — resolve lists/segments, dedup recipients via `BulkInsert`, create `links`, set `recipient_count`, enqueue `campaign.batch` jobs
- [ ] T048 [US2] Implement the `campaign.batch` River worker in `internal/campaign/adapters/batch_worker.go` — short-circuit on non-`running` state, per-recipient rate-limit check (snooze on denial), render, send, mark sent/failed, record progress, auto-pause, finish
- [ ] T049 [US2] Integration-test `campaigns_pg.go`, `templates_pg.go`, `recipients_pg.go`, `tracking_pg.go` in `internal/campaign/adapters/*_pg_test.go` against Postgres, including a cross-tenant isolation test per repository and a `UNIQUE (campaign_id, email)` dedup assertion
- [ ] T050 [US2] Component-test the send pipeline in `internal/campaign/adapters/send_pipeline_test.go` with a fake messenger and the real Redis limiter — full send to 100% of recipients each exactly once, rate-limit pacing, auto-pause past `max_send_errors`
- [ ] T051 [US2] Resumability test in `internal/campaign/adapters/resumability_test.go` — cancel the worker context mid-`campaign.batch`, restart, assert the campaign finishes with no recipient `sent` twice

### App layer

- [ ] T052 [P] [US2] Implement the template commands `CreateTemplate`/`UpdateTemplate` in `internal/campaign/app/command/templates.go`
- [ ] T053 [P] [US2] Implement the campaign commands `CreateCampaign`/`UpdateCampaign`/`StartCampaign`/`PauseCampaign`/`ResumeCampaign` in `internal/campaign/app/command/campaigns.go` (CreateCampaign inherits omitted fields from the template; StartCampaign validates a verified domain + targets, then enqueues `campaign.start`)
- [ ] T054 [P] [US2] Implement the queries `ListTemplates`/`GetTemplate`/`ListCampaigns`/`GetCampaign` (with progress counts) in `internal/campaign/app/query/`
- [ ] T055 [US2] Assemble `internal/campaign/app/application.go` and declare the `CampaignEnqueuer` interface in the app layer
- [ ] T056 [US2] Unit-test the campaign commands in `internal/campaign/app/command/campaigns_test.go` with fakes (start preconditions, template-kind mismatch, draft-only editing)

### Transport & wiring

- [ ] T057 [US2] Implement the template/campaign HTTP handlers in `internal/api/campaign_handlers.go` per contracts/http-api.md
- [ ] T058 [P] [US2] Implement the public tracking handlers in `internal/api/tracking_handlers.go` — `GET /o/{campaignId}` (records a view, returns a 1×1 GIF) and `GET /l/{linkId}` (records a click, 302-redirects), resolving the tenant from the UUID before the bound transaction
- [ ] T059 [US2] Mount the campaign routes (authz group) and the public `/o` and `/l` routes (router root) in `internal/api/server.go`; extend `internal/api/errmap.go` for the T038 slugs
- [ ] T060 [US2] Wire the `campaign` context in `internal/service/application.go` (`buildCampaign` — repositories, Postbox messenger, rate limiter, enqueuer, decorated handlers) and add it to `Application` and `internal/api/server.go`'s `New`
- [ ] T061 [US2] Register the `start_worker` and `batch_worker` in `cmd/worker/main.go`
- [ ] T062 [US2] API-level integration test in `internal/api/campaign_handlers_test.go` — create template → create campaign → start → poll progress → confirm tracking pixel and rewritten links in the sent message; plus a tracking-endpoint test asserting events attribute to the correct tenant

**Checkpoint**: US1 and US2 both functional — a tenant can verify a domain and
send a tracked campaign.

---

## Phase 5: User Story 3 — Send transactional email via API (Priority: P2)

**Goal**: A tenant developer sends a single transactional message via an
API-key-authenticated endpoint, rendered from a transactional template and
delivered immediately through the verified domain.

**Independent Test**: With a valid scoped API key and a transactional template,
call `POST /t/{slug}/api/tx` and confirm one message is delivered; confirm
missing/invalid/wrongly-scoped keys are rejected with no send.

- [ ] T063 [US3] Implement the API-key authentication middleware in `internal/api/apikey_middleware.go` — read `Authorization: Bearer`, resolve via the existing iam `AuthenticateAPIKey` query, verify the key belongs to the resolved tenant and carries the transactional-send scope
- [ ] T064 [US3] Implement the `SendTransactional` command in `internal/campaign/app/command/transactional.go` — load the `transactional` template, render with variables, rate-limit check, send synchronously via the `Messenger`, emit a usage event; add it to `internal/campaign/app/application.go`
- [ ] T065 [US3] Implement the `tx` HTTP handler in `internal/api/tx_handlers.go` — map `rate-limited` to `429` with a `Retry-After` header, kind-mismatch to `422` per contracts/http-api.md
- [ ] T066 [US3] Mount `POST /t/{slug}/api/tx` in `internal/api/server.go` behind `resolveTenant` + the new API-key middleware; wire the `SendTransactional` handler in `internal/service/application.go`
- [ ] T067 [US3] Unit-test `SendTransactional` in `internal/campaign/app/command/transactional_test.go` (template-not-found, kind-mismatch, unverified domain, rate-limited)
- [ ] T068 [US3] API-level integration test in `internal/api/tx_handlers_test.go` — valid key sends one message; missing/invalid/wrongly-scoped keys rejected with no send; cross-tenant key rejected

**Checkpoint**: All three user stories independently functional — the phase
exit criterion is met.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Documentation, the full verification bundle, and the phase exit gate.

- [ ] T069 [P] Update `docs/architecture.md` and `docs/implementation-plan.md` to mark Phase 3 delivered and note any decisions that diverged from the original outline
- [ ] T070 [P] Update `.env.example` with every new `NVELOPE_` setting from T004 and a short comment per value
- [ ] T071 Run the quickstart in `specs/006-phase-3-sending-pipeline/quickstart.md` end-to-end against a local stack and the Postbox staging account; fix any drift between the doc and reality
- [ ] T072 Run the full verification bundle — `make test` (Postgres + Redis + River integration), `make lint`, and a clean `go run ./cmd/migrate up` on a fresh database — and confirm all gates are green

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: no dependencies — start immediately.
- **Foundational (Phase 2)**: depends on Setup — BLOCKS all user stories.
- **US1 (Phase 3)**: depends on Foundational. No dependency on US2/US3.
- **US2 (Phase 4)**: depends on Foundational. Independently testable; in practice
  US2 sends from a domain verified by US1, but US2's tests seed a verified domain
  directly so the story stays independently testable.
- **US3 (Phase 5)**: depends on Foundational and reuses the `campaign` context's
  `Messenger`/`RateLimiter` and template machinery from US2 — so US3 is best
  sequenced after US2.
- **Polish (Phase 6)**: depends on all targeted user stories being complete.

### User Story Dependencies

- **US1 (P1)**: starts after Phase 2. Self-contained.
- **US2 (P1)**: starts after Phase 2. Self-contained for testing.
- **US3 (P2)**: starts after Phase 2; shares the `campaign` context with US2 —
  schedule after US2 to avoid editing `internal/campaign/app/application.go`
  concurrently.

### Within Each User Story

- Migration → domain entities/interfaces → adapters → app handlers → transport
  → wiring → integration tests.
- Tests for an entity/adapter are written alongside it; integration tests run
  after the adapter and wiring exist.
- Models before services; services before endpoints.

### Parallel Opportunities

- Setup: T002, T003 in parallel.
- Foundational: T005+T006+T007 (postbox) run parallel to T008+T009 (ratelimit);
  T011 parallel once T010 lands.
- US1: T013/T014/T015/T016 parallel (distinct files); T018 parallel to T017;
  T022/T023 parallel.
- US2: T034–T040 largely parallel; the adapter tasks T041–T046 parallel
  (distinct files); T052/T054 parallel.
- Once Phase 2 is done, US1 and US2 can be staffed by different developers in
  parallel.

---

## Parallel Example: User Story 1 domain layer

```bash
# After T012 (migration), launch the domain-layer tasks together:
Task: "Implement the SendingDomain aggregate in internal/sending/domain/domain.go"
Task: "Define slug errors in internal/sending/domain/errors.go"
Task: "Declare interfaces in internal/sending/domain/repository.go + provisioner.go"
Task: "Unit-test the SendingDomain entity in internal/sending/domain/domain_test.go"
```

---

## Implementation Strategy

### MVP First (User Story 1 only)

1. Phase 1: Setup.
2. Phase 2: Foundational (CRITICAL — blocks all stories).
3. Phase 3: US1 — domain verification.
4. **STOP and VALIDATE**: verify a real domain end-to-end.

### Incremental Delivery

1. Setup + Foundational → infrastructure ready.
2. US1 → verify a domain → demo (first shippable slice).
3. US2 → send a tracked campaign → demo (the phase's core value).
4. US3 → transactional API → demo.
5. Phase 6 → green verification bundle → phase exit.

### Parallel Team Strategy

After Phase 2: Developer A takes US1, Developer B takes US2. US3 follows US2
(shared `campaign` context). Polish is done together once stories land.

---

## Notes

- `[P]` = different files, no dependency on an incomplete task.
- Every new tenant-plane table ships with RLS and a cross-tenant isolation test
  (constitution I) — these are not optional.
- River job payloads carry only identifiers; per-recipient status rows are what
  make sends resumable and idempotent (constitution V).
- Postbox is faked for routine tests and verified for real behind
  `NVELOPE_POSTBOX_INTEGRATION` (constitution II / research.md R12).
- Commit after each task or logical group; stop at any checkpoint to validate a
  story independently.
