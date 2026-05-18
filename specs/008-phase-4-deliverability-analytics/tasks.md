---
description: "Task list for Phase 4 — Deliverability & Analytics"
---

# Tasks: Phase 4 — Deliverability & Analytics

**Input**: Design documents from `/specs/008-phase-4-deliverability-analytics/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/ (http-api.md, jobs.md, ports.md)

**Tests**: Test tasks ARE included — plan.md "Testing" and Constitution Principle II ("Test-Backed Delivery") explicitly require component and integration coverage for ingestion, async processing, the pre-send suppression gate, idempotent de-duplication, and analytics-refresh correctness.

**Organization**: Tasks are grouped by user story (US1, US2, US3) so each story can be implemented, tested, and shipped as an independent increment.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[Story]**: Which user story the task belongs to (US1, US2, US3)
- All paths are relative to the repository root.

## ⚠️ Pre-work note — spec drift

The branch already contains partial work from an earlier *webhook + HMAC signature* design that the 2026-05-18 clarifications superseded: Postbox delivers feedback via a **Yandex Data Streams topic**, not an HTTP webhook, and there is **no per-notification signature**. Tasks T004–T005 remove those contradicting artifacts before new work begins.

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Project initialization, new dependency, configuration, and removal of superseded artifacts.

- [X] T001 Add `github.com/ydb-platform/ydb-go-sdk/v3` to `go.mod` and run `go mod tidy`
- [X] T002 [P] Extend `internal/config/config.go` with feedback-stream settings (Data Streams endpoint, topic/path, consumer name, credentials) and analytics settings (refresh interval, default 60s); update `internal/config/config_test.go`
- [X] T003 [P] Add the new feedback-stream and analytics config keys to `.env.example`
- [X] T004 Remove the superseded webhook route: delete `internal/api/webhook_handlers.go` and `internal/api/webhook_handlers_test.go`, and drop the webhook mount from `internal/api/server.go`
- [X] T005 [P] Remove the superseded signature verifier: delete `internal/deliverability/domain/verifier.go`, `internal/deliverability/adapters/verifier_hmac.go`, and `internal/deliverability/adapters/verifier_hmac_test.go`

**Checkpoint**: Build compiles, new dependency resolves, no webhook/signature code remains.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Database schema, domain scaffolding, ports, and job infrastructure shared by all three user stories.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete.

### Migrations

- [X] T006 [P] Write migration `internal/db/migrations/000011_delivery_feedback.up.sql` / `.down.sql`: create control-plane `inbound_feedback_events` (no RLS; `dedupe_key` UNIQUE, `event_kind`/`status` CHECKs, indexes on `status` and `received_at`; `GRANT SELECT,INSERT,UPDATE TO nvelope_app`), tenant-plane `delivery_events` (RLS, `UNIQUE(inbound_event_id)`, exactly-one-of attribution CHECK, indexes on `(campaign_id,event_kind)` and `(tenant_id,recipient_email)`), tenant-plane `transactional_messages` (RLS, `provider_message_id` UNIQUE), alter `campaign_recipients` (add `provider_message_id`, add `'skipped'` to `status` CHECK, index on `provider_message_id`), and the `feedback_tenant_for_message(text)` `SECURITY DEFINER` function (`REVOKE ALL FROM PUBLIC`, `GRANT EXECUTE TO nvelope_app`)
- [X] T007 [P] Write migration `internal/db/migrations/000012_suppression.up.sql` / `.down.sql`: create tenant-plane `suppression_list` (RLS, `reason` CHECK, `UNIQUE(tenant_id,email)`, FK `source_event_id`→`delivery_events`) and `bounce_settings` (RLS, `tenant_id` PK, both toggles default `true`)
- [X] T008 [P] Write migration `internal/db/migrations/000013_campaign_analytics.up.sql` / `.down.sql`: create tenant-plane `campaign_analytics` (RLS, `campaign_id` PK, six count columns default 0, `refreshed_at`, index on `tenant_id`)
- [X] T009 Verify all three migrations apply and roll back cleanly against a fresh `postgres:17` (`make test-db-clean` then run the migration harness)

### Domain scaffolding & ports

- [X] T010 [P] Define context error slugs in `internal/deliverability/domain/errors.go` (`ErrSuppressionNotFound` → `suppression_not_found`, `ErrRecipientSuppressed` → `recipient_suppressed`, validation errors → `validation_failed`)
- [X] T011 [P] Declare the `FeedbackStream` reader port and `StreamMessage` value type in `internal/deliverability/domain/stream.go` per contracts/ports.md
- [X] T012 Declare the repository ports in `internal/deliverability/domain/repository.go`: `EventRepository`, `SuppressionRepository`, `SettingsRepository`, `AnalyticsRepository`, and the `FeedbackEnqueuer` / `AnalyticsEnqueuer` interfaces (per contracts/ports.md)

### Job infrastructure

- [X] T013 Extend `internal/platform/jobs/jobs.go`: add `FeedbackProcessArgs` and `AnalyticsRefreshArgs` with their `Kind()` methods, and `EnqueueFeedbackProcess` / `EnqueueAnalyticsRefresh` on `SendEnqueuer`; update `internal/platform/jobs/jobs_test.go`

**Checkpoint**: Schema is live, domain ports and job args compile — user stories can now begin.

---

## Phase 3: User Story 1 - Ingest and attribute delivery feedback (Priority: P1) 🎯 MVP

**Goal**: A `cmd/consumer` service reads the Postbox Data Streams topic, stages each notification idempotently, and a `feedback.process` job attributes every event to its message/recipient/campaign — storing unmatched events as unattributed.

**Independent Test**: Send a message to a known-bad address, write a bounce notification to the (fake) stream, confirm the consumer stages it and the worker records an attributed `delivery_events` row tied to the right message/recipient/campaign. Separately, feed a notification for an unknown message and confirm it is stored as `unattributed`, not discarded. Re-feed a duplicate `eventId` and confirm exactly one event.

### Tests for User Story 1 ⚠️

> Write these tests FIRST and confirm they FAIL before implementing.

- [X] T014 [P] [US1] Domain tests for `DeliveryEvent` and `InboundNotification` (valid/invalid construction, `SuppressionReason()`, `IsBounce()`/`IsComplaint()`, hydration path) in `internal/deliverability/domain/event_test.go`
- [X] T015 [P] [US1] Tests for Postbox notification JSON parsing (bounce, complaint, delivery, open, click, and ignored types like Send/DeliveryDelay) in `internal/deliverability/adapters/notification_parse_test.go`
- [X] T016 [P] [US1] Integration test for `events_pg` repo: stage/load inbound, `TenantForMessage` lookup, `RecordEvent` idempotency on `UNIQUE(inbound_event_id)`, and cross-tenant isolation in `internal/deliverability/adapters/events_pg_test.go`
- [X] T017 [P] [US1] Component test for the `IngestNotification` command — staging + `feedback.process` enqueue, idempotent on duplicate `dedupe_key` — in `internal/deliverability/app/command/feedback_test.go`
- [X] T018 [P] [US1] Component test for the `feedback.process` worker: attributed path, unattributed path, duplicate re-read recorded once, resumability (worker killed mid-process → event recorded exactly once) in `internal/deliverability/adapters/feedback_worker_test.go`

### Implementation for User Story 1

- [X] T019 [P] [US1] Implement `InboundNotification` value type and event kinds in `internal/deliverability/domain/inbound.go`
- [X] T020 [P] [US1] Implement the `DeliveryEvent` aggregate (`NewDeliveryEvent`, `HydrateDeliveryEvent`, `SuppressionReason()`, `IsBounce()`, `IsComplaint()`) in `internal/deliverability/domain/event.go`
- [X] T021 [P] [US1] Implement Postbox notification JSON → `InboundNotification` parsing in `internal/deliverability/adapters/notification_parse.go`
- [X] T022 [P] [US1] Implement the thin YDB Data Streams topic-reader client in `internal/platform/datastreams/reader.go` (wraps `ydb-go-sdk/v3`, server-side consumer offset)
- [X] T023 [US1] Implement the `FeedbackStream` adapter over `platform/datastreams` in `internal/deliverability/adapters/stream_reader.go` (depends on T011, T022)
- [X] T024 [US1] Implement `EventRepository` (`inbound_feedback_events` staging + `delivery_events`) in `internal/deliverability/adapters/events_pg.go` (depends on T006, T012)
- [X] T025 [P] [US1] Implement the `transactional_messages` repository in `internal/campaign/adapters/transactional_pg.go` with its cross-tenant test in `internal/campaign/adapters/transactional_pg_test.go` (depends on T006)
- [X] T026 [US1] Implement the `IngestNotification` command (stage row + enqueue `feedback.process`) in `internal/deliverability/app/command/feedback.go` (depends on T024, T013)
- [X] T027 [US1] Implement the `ProcessFeedback` command — load inbound row, resolve tenant via `feedback_tenant_for_message`, attribute, record `delivery_events`, mark status — in `internal/deliverability/app/command/feedback.go` (depends on T024, T026)
- [X] T028 [US1] Implement the `feedback.process` River worker in `internal/deliverability/adapters/feedback_worker.go` (depends on T027)
- [X] T029 [US1] Persist `provider_message_id` on successful send and record `transactional_messages`: extend `internal/campaign/adapters/batch_worker.go` and `internal/campaign/adapters/recipients_pg.go`, and `internal/campaign/app/command/transactional.go` (depends on T025)
- [X] T030 [US1] Build the `deliverability` `Application{Commands,Queries}` skeleton with ingestion commands wired in `internal/deliverability/app/application.go`
- [X] T031 [US1] Create the `cmd/consumer/main.go` service: read the topic via `FeedbackStream`, call `IngestNotification`, commit the offset
- [X] T032 [US1] Register `FeedbackProcessWorker` on the send-queue River client in `cmd/worker/main.go`
- [X] T033 [US1] Wire the ingestion slice (event repo, stream adapter, consumer application) in `internal/service/application.go`
- [X] T034 [US1] Emit metrics/structured logs for unattributed events and consumer processing failures (FR-009) in the worker and consumer

**Checkpoint**: Feedback flows end-to-end — notifications are read, staged, attributed, and observable. US1 is independently demonstrable and shippable as the MVP.

---

## Phase 4: User Story 2 - Automatic suppression and pre-send checks (Priority: P1)

**Goal**: Hard bounces and complaints automatically suppress an address; every campaign and transactional send skips suppressed recipients; operators view, add, and remove entries and configure bounce actions.

**Independent Test**: Trigger a hard bounce, confirm the address appears on the suppression list with reason `hard_bounce`; start a campaign including that address and confirm the recipient is `skipped` with the reason recorded; manually remove the entry and confirm the address is mailable again.

### Tests for User Story 2 ⚠️

> Write these tests FIRST and confirm they FAIL before implementing.

- [X] T035 [P] [US2] Domain tests for `SuppressionEntry` and `BounceSettings` (validating constructors, email lower-casing, `Default()`, toggle behaviour) in `internal/deliverability/domain/suppression_test.go` and `settings_test.go`
- [X] T036 [P] [US2] Integration tests for `suppression_pg` and `settings_pg` repos including cross-tenant isolation in `internal/deliverability/adapters/suppression_pg_test.go` and `settings_pg_test.go`
- [X] T037 [P] [US2] Component test: `feedback.process` applies suppression for bounce/complaint per `bounce_settings`, respecting toggles, in `internal/deliverability/adapters/feedback_worker_test.go`
- [X] T038 [P] [US2] Component test for the pre-send suppression gate in the campaign send-pipeline harness — start/batch workers skip suppressed recipients, transactional handler returns `ErrRecipientSuppressed` — in `internal/campaign/adapters/` and `internal/campaign/app/command/transactional_test.go`
- [X] T039 [P] [US2] Handler tests for the suppression and bounce-settings routes in `internal/api/`

### Implementation for User Story 2

- [X] T040 [P] [US2] Implement the `SuppressionEntry` aggregate (`NewSuppressionEntry`, `NewManualSuppression`, `HydrateSuppressionEntry`) in `internal/deliverability/domain/suppression.go`
- [X] T041 [P] [US2] Implement the `BounceSettings` type (`Default()`, `ShouldSuppressHardBounce()`, `ShouldSuppressComplaint()`) in `internal/deliverability/domain/settings.go`
- [X] T042 [P] [US2] Implement the `SuppressionRepository` adapter in `internal/deliverability/adapters/suppression_pg.go` (depends on T007)
- [X] T043 [P] [US2] Implement the `SettingsRepository` adapter in `internal/deliverability/adapters/settings_pg.go` (depends on T007)
- [X] T044 [US2] Declare the `SuppressionChecker` port in `internal/campaign/domain/messenger.go` per contracts/ports.md
- [X] T045 [US2] Implement the `SuppressionChecker` adapter in `internal/deliverability/adapters/suppression_checker.go` (depends on T042, T044)
- [X] T046 [US2] Extend the `feedback.process` worker / `ProcessFeedback` command to upsert `suppression_list` for bounce/complaint events per `bounce_settings` in `internal/deliverability/app/command/feedback.go` (depends on T042, T043)
- [X] T047 [P] [US2] Implement `AddSuppression`, `RemoveSuppression`, and `UpdateBounceSettings` commands in `internal/deliverability/app/command/` (audit-log entries `suppression.added`/`suppression.removed`/`bounce_settings.updated`)
- [X] T048 [P] [US2] Implement `ListSuppressions` and `GetBounceSettings` queries in `internal/deliverability/app/query/`
- [X] T049 [US2] Apply the pre-send suppression check in the campaign `start_worker` (write `skipped` recipients with reason in `failure_reason`, exclude from send count) — `internal/campaign/adapters/start_worker.go` (depends on T045)
- [X] T050 [US2] Re-check suppression before each provider call in `internal/campaign/adapters/batch_worker.go` (depends on T045)
- [X] T051 [US2] Apply the pre-send suppression check in `SendTransactionalHandler`, returning `ErrRecipientSuppressed` — `internal/campaign/app/command/transactional.go` (depends on T045)
- [X] T052 [US2] Implement the suppression-list and bounce-settings HTTP handlers in `internal/api/suppression_handlers.go` (`GET`/`POST`/`DELETE /suppressions`, `GET`/`PUT /bounce-settings`) (depends on T047, T048)
- [X] T053 [US2] Mount the suppression and bounce-settings routes in `internal/api/server.go` and map new error slugs in `internal/api/errmap.go`
- [X] T054 [US2] Wire the suppression slice (repos, checker, commands/queries) into `internal/service/application.go`

**Checkpoint**: Suppression is automatic and enforced on every send path; operators can manage the list and bounce actions. US1 + US2 both work independently.

---

## Phase 5: User Story 3 - Campaign analytics and dashboard (Priority: P2)

**Goal**: Per-campaign analytics and a workspace dashboard, served from a periodically-refreshed `campaign_analytics` summary table — fast even for large campaigns and strictly tenant-scoped.

**Independent Test**: Run a campaign producing opens/clicks/bounces/complaints, refresh analytics, open the campaign analytics view and confirm counts/rates match the events; open the workspace dashboard and confirm it summarizes the campaign; confirm the view renders quickly for a 100k-recipient campaign.

### Tests for User Story 3 ⚠️

> Write these tests FIRST and confirm they FAIL before implementing.

- [X] T055 [P] [US3] Domain tests for `CampaignAnalytics` / `Dashboard` rate derivation (zero denominator → 0.0) in `internal/deliverability/domain/analytics_test.go`
- [X] T056 [P] [US3] Integration test for `analytics_pg` refresh correctness — six counts aggregated from `delivery_events`/`campaign_recipients`, idempotent re-run, cross-tenant isolation — in `internal/deliverability/adapters/analytics_pg_test.go`
- [X] T057 [P] [US3] Component test for the `analytics.refresh` worker in `internal/deliverability/adapters/analytics_worker_test.go`
- [X] T058 [P] [US3] Handler tests for the campaign-analytics and dashboard routes (including tenant scoping) in `internal/api/`

### Implementation for User Story 3

- [X] T059 [P] [US3] Implement the `CampaignAnalytics` and `Dashboard` read-model value objects (counts + derived rates) in `internal/deliverability/domain/analytics.go`
- [X] T060 [US3] Implement the `AnalyticsRepository` adapter — `GetCampaign`, `GetDashboard`, and `Refresh` (full per-tenant recompute, `INSERT … ON CONFLICT DO UPDATE`) — in `internal/deliverability/adapters/analytics_pg.go` (depends on T008)
- [X] T061 [US3] Implement the `RefreshAnalytics` command in `internal/deliverability/app/command/` (depends on T060)
- [X] T062 [US3] Implement the `analytics.refresh` River worker in `internal/deliverability/adapters/analytics_worker.go` (depends on T061)
- [X] T063 [P] [US3] Implement `GetCampaignAnalytics` and `GetDashboard` queries in `internal/deliverability/app/query/`
- [X] T064 [US3] Implement the campaign-analytics and dashboard HTTP handlers in `internal/api/analytics_handlers.go` and mount the routes in `internal/api/server.go` (depends on T063)
- [X] T065 [US3] Register `AnalyticsRefreshWorker` in `cmd/worker/main.go` and add the periodic per-tenant `analytics.refresh` enqueue tick (with `UniqueOpts{ByArgs:true}`) to `cmd/scheduler/main.go` (depends on T062, T013)
- [X] T066 [US3] Wire the analytics slice (repo, command/query, worker) into `internal/service/application.go`

**Checkpoint**: All three user stories are independently functional.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Verification, documentation, and final cleanup across all stories.

- [X] T067 Apply the standard logging/metrics decorators to every new `deliverability` command and query handler
- [X] T068 [P] Update `CLAUDE.md` / project docs with the new `cmd/consumer` service and the four-service deployment topology
- [X] T069 Run `quickstart.md` validation end-to-end against a real Postgres
- [X] T070 Run `make test` (`go test ./...`) and the linter; confirm a green suite and a clean migration apply

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — start immediately.
- **Foundational (Phase 2)**: Depends on Setup — BLOCKS all user stories.
- **User Story 1 (Phase 3)**: Depends on Foundational. No dependency on US2/US3.
- **User Story 2 (Phase 4)**: Depends on Foundational. The suppression-on-bounce task (T046) integrates with the US1 `feedback.process` worker; the rest is independent.
- **User Story 3 (Phase 5)**: Depends on Foundational. Reads `delivery_events` produced by US1 but is independently testable with seeded events.
- **Polish (Phase 6)**: Depends on all targeted user stories.

### Within Each User Story

- Tests are written and confirmed failing before implementation.
- Domain types → repositories/adapters → commands/queries → workers/handlers → wiring.
- Story complete and validated before moving to the next priority.

### Parallel Opportunities

- Setup: T002, T003, T005 run in parallel.
- Foundational: T006, T007, T008 (migrations) run in parallel; T010, T011 run in parallel.
- All `[P]` test tasks within a story run in parallel.
- Domain types within a story marked `[P]` run in parallel.
- With staff, US1, US2, and US3 can be developed in parallel once Foundational completes (one developer per story).

---

## Parallel Example: User Story 1

```bash
# Tests first (all parallel):
Task: "Domain tests for DeliveryEvent/InboundNotification in internal/deliverability/domain/event_test.go"
Task: "Notification parse tests in internal/deliverability/adapters/notification_parse_test.go"
Task: "events_pg integration test in internal/deliverability/adapters/events_pg_test.go"
Task: "IngestNotification command test in internal/deliverability/app/command/feedback_test.go"
Task: "feedback.process worker test in internal/deliverability/adapters/feedback_worker_test.go"

# Then parallel implementation of independent units:
Task: "InboundNotification value type in internal/deliverability/domain/inbound.go"
Task: "DeliveryEvent aggregate in internal/deliverability/domain/event.go"
Task: "Notification parsing in internal/deliverability/adapters/notification_parse.go"
Task: "YDB datastreams reader in internal/platform/datastreams/reader.go"
```

---

## Implementation Strategy

### MVP First (User Story 1 only)

1. Complete Phase 1: Setup.
2. Complete Phase 2: Foundational (CRITICAL — blocks all stories).
3. Complete Phase 3: User Story 1.
4. **STOP and VALIDATE**: feedback is ingested and attributed; unattributed events stored; duplicates de-duplicated.
5. Deploy/demo the `cmd/consumer` ingestion slice.

### Incremental Delivery

1. Setup + Foundational → foundation ready.
2. US1 (P1) → ingestion works → ship MVP.
3. US2 (P1) → automatic suppression + pre-send gate → ship.
4. US3 (P2) → analytics + dashboard → ship.
5. Each story adds value without breaking the previous ones.

### Parallel Team Strategy

After Foundational completes: Developer A on US1, Developer B on US2, Developer C on US3. The only cross-story touchpoint is T046 (suppression applied inside the US1 worker) — coordinate that one task.

---

## Notes

- `[P]` tasks touch different files with no incomplete-task dependencies.
- `[Story]` labels map tasks to spec.md user stories for traceability.
- This phase is backend-only; the dashboard UI is a later increment.
- Soft bounces, reputation scoring, and provider failover are explicitly out of scope.
- Commit after each task or logical group; verify tests fail before implementing.
