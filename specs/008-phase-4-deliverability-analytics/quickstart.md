# Quickstart: Deliverability & Analytics

How to build, configure, run, and verify Phase 4 locally. Assumes the Phase 3
sending pipeline is already working (a tenant can verify a domain and send a
campaign through Postbox).

## Prerequisites

- Go 1.26, a running Docker daemon (testcontainers spins up `postgres:17`).
- The Phase 3 environment: Postbox credentials and the Redis DSN already
  configured for `cmd/api`, `cmd/worker`, and `cmd/scheduler`.
- Access to the Yandex Data Streams topic Postbox writes feedback to, and
  credentials authorised to read it.

## New dependency

Phase 4 adds `github.com/ydb-platform/ydb-go-sdk/v3`, used by
`internal/platform/datastreams` to consume the feedback topic:

```sh
go get github.com/ydb-platform/ydb-go-sdk/v3
go mod tidy
```

## New configuration

Add to the environment / `.env` consumed by `knadh/koanf`:

| Key | Purpose | Default |
| --- | --- | --- |
| `NVELOPE_FEEDBACK_STREAM_ENDPOINT` | Data Streams / YDB endpoint (e.g. `grpcs://…:2135`) | — (required) |
| `NVELOPE_FEEDBACK_STREAM_DATABASE` | YDB database path the topic lives in | — (required) |
| `NVELOPE_FEEDBACK_STREAM_TOPIC` | Topic path Postbox writes notifications to | — (required) |
| `NVELOPE_FEEDBACK_STREAM_CONSUMER` | Registered consumer name (offsets are kept server-side under it) | — (required) |
| `NVELOPE_FEEDBACK_STREAM_CREDENTIALS_FILE` | Yandex Cloud service-account key JSON file for stream auth; empty uses the instance IAM metadata credentials | "" |
| `NVELOPE_ANALYTICS_REFRESH_INTERVAL` | How often the scheduler enqueues `analytics.refresh` per tenant | `60s` |

The stream credentials are secret config — never logged. There is no webhook
secret and no soft-bounce threshold: Postbox delivers feedback over the topic,
not a webhook, and soft bounces are out of scope this phase.

## Migrate

```sh
make migrate-up        # applies 000011, 000012, 000013 + River queue tables
```

A clean apply is a phase exit criterion. `make migrate-down` must also reverse
all three.

## Run

```sh
go run ./cmd/api        # serves the new suppression + analytics tenant routes
go run ./cmd/worker     # runs feedback.process and analytics.refresh
go run ./cmd/scheduler  # enqueues analytics.refresh per tenant on the interval
go run ./cmd/consumer   # reads the Postbox feedback topic
```

## Verify each user story

### US1 — feedback ingestion

1. Send a campaign (Phase 3) so `campaign_recipients` rows have
   `provider_message_id` set.
2. Have Postbox (or a test fixture) write a `Bounce` notification to the topic
   whose `mail.messageId` is one of those `provider_message_id` values. Confirm
   `cmd/consumer` reads it, an `inbound_feedback_events` row is staged, and once
   the `feedback.process` job runs a `delivery_events` bounce row appears
   attributed to the campaign and recipient.
3. Write the same notification again (same `eventId`) → still only one
   `delivery_events` row (idempotent).
4. Write a notification whose `mail.messageId` matches no send → the
   `inbound_feedback_events` row ends `unattributed`.
5. Restart `cmd/consumer` mid-stream → it resumes from the committed topic
   offset, losing and re-counting nothing.

### US2 — suppression & pre-send checks

1. After the bounce in US1, confirm the address is in
   `GET /t/{slug}/api/suppressions` with reason `hard_bounce`.
2. Start a new campaign whose list includes that address; confirm its
   `campaign_recipients` row is `skipped` and no message was sent to it.
3. `DELETE /t/{slug}/api/suppressions/{email}` (URL-encoded); re-run a send and
   confirm the address is mailed again.
4. `PUT /t/{slug}/api/bounce-settings` with `suppressOnComplaint: false`; write a
   `Complaint` notification and confirm the address is **not** suppressed.
5. `POST /t/{slug}/api/suppressions` to add an address manually; confirm it is
   skipped on the next send.

### US3 — analytics

1. With a sent campaign, write `Delivery`, `Open`, `Click`, and `Bounce`
   notifications to the topic; after one refresh interval (or running the
   worker), `GET /t/{slug}/api/campaigns/{id}/analytics`. Confirm the counts
   match the events and the rates are derived correctly.
2. `GET /t/{slug}/api/dashboard` — confirm the campaign appears under
   `recentCampaigns` and the totals add up.
3. As a second tenant, confirm neither endpoint exposes the first tenant's
   campaigns or figures.

## Test

```sh
make test       # go test ./... — includes the new component & integration tests
```

Phase exit: green `go test ./...`, the cross-tenant isolation tests for every new
repository and the analytics query pass, idempotent-ingestion and
worker-resumability tests pass, and `migrate up` / `migrate down` are clean.
