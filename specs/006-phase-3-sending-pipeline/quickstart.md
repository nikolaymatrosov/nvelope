# Quickstart: Phase 3 — Sending Pipeline

How to build, configure, and exercise the sending pipeline locally.

## Prerequisites

- A running Docker daemon — integration tests start `postgres:17` and a Redis
  container via testcontainers automatically.
- Yandex Postbox staging credentials (access key ID + secret, region, endpoint)
  to exercise real domain verification and sending. Routine tests use a fake
  messenger and need no Postbox account.

## Configuration

Add to `.env` (all `NVELOPE_`-prefixed; secrets must not be logged):

```
NVELOPE_POSTBOX_REGION=ru-central1
NVELOPE_POSTBOX_ENDPOINT=https://postbox.cloud.yandex.net
NVELOPE_POSTBOX_ACCESS_KEY_ID=...
NVELOPE_POSTBOX_SECRET_ACCESS_KEY=...
NVELOPE_REDIS_URL=redis://localhost:6379/0
NVELOPE_WORKER_SEND_QUEUE=sending
NVELOPE_GLOBAL_SEND_RATE_LIMIT=500
NVELOPE_GLOBAL_SEND_RATE_WINDOW=1s
NVELOPE_DEFAULT_TENANT_SEND_RATE_LIMIT=50
NVELOPE_DEFAULT_TENANT_SEND_RATE_WINDOW=1s
NVELOPE_SENDING_DOMAIN_VERIFY_INTERVAL=15m
NVELOPE_SENDING_DOMAIN_VERIFY_WINDOW=72h
NVELOPE_CAMPAIGN_BATCH_SIZE=500
```

`config.Load` fails fast if a required Postbox or Redis value is missing.

## Build & migrate

```sh
make build                 # builds cmd/api, cmd/worker, cmd/scheduler
go run ./cmd/migrate up    # applies 000008–000010 + River's own migrations
```

## Run the services

```sh
go run ./cmd/api        # serves the HTTP API + public tracking routes
go run ./cmd/worker     # consumes the "sending" queue: verify/start/batch workers
go run ./cmd/scheduler  # periodically re-arms domain.verify recovery sweeps
```

## Exercise the flow (US1 → US2 → US3)

Assumes a tenant `acme` and a logged-in session from Phases 1–2.

### 1. Verify a sending domain

```sh
# Register a domain — returns DNS records to publish
curl -X POST .../t/acme/api/sending-domains -d '{"domain":"mail.acme.com"}'
# Publish the returned DKIM/SPF/DMARC records in DNS, then either wait for the
# polling job or force a re-check:
curl -X POST .../t/acme/api/sending-domains/{id}/recheck
curl .../t/acme/api/sending-domains/{id}        # expect "status":"verified"
```

### 2. Create and send a campaign

```sh
curl -X POST .../t/acme/api/templates -d '{"name":"Newsletter","kind":"campaign",...}'
curl -X POST .../t/acme/api/campaigns -d '{"name":"May","template_id":"...",
  "sending_domain_id":"<verified>","from_local_part":"news","list_ids":["..."]}'
curl -X POST .../t/acme/api/campaigns/{id}/start
curl .../t/acme/api/campaigns/{id}      # watch sent_count / failed_count climb
```

Each delivered message has rewritten `/l/{uuid}` links and an `/o/{uuid}` pixel.

### 3. Send a transactional message

```sh
curl -X POST .../t/acme/api/tx \
  -H 'Authorization: Bearer <api-key>' \
  -d '{"template_id":"<tx-template>","to":"sam@example.com",
       "sending_domain_id":"<verified>","from_local_part":"noreply",
       "variables":{"name":"Sam"}}'
```

## Verification bundle (phase exit gate)

```sh
make test          # go test ./... — unit + integration (Postgres, Redis, River)
make lint
go run ./cmd/migrate up   # clean apply on a fresh database
```

Phase-specific checks the suite must cover (constitution II):

- **Isolation**: tenant A cannot read/send from / attribute events to tenant B's
  domains, templates, campaigns, links, or views — even with an app-level filter
  omitted (RLS backstop).
- **Sending**: a campaign reaches 100% of recipients, each exactly once
  (`UNIQUE (campaign_id, email)`), with a pixel and rewritten links.
- **Resumability**: cancel the worker context mid-campaign; on restart the
  campaign finishes with zero recipients in `sent` twice.
- **Rate limiting**: concurrent goroutines never exceed the per-tenant or global
  window; a `tx` call over limit returns `429` with `Retry-After`.
- **Auto-pause**: a campaign past `max_send_errors` transitions to `paused` and
  remaining batches no-op.
- **Domain verification**: a domain that never publishes records ends `failed`
  after the window; a correct one reaches `verified`.

The opt-in real-Postbox integration test runs only when
`NVELOPE_POSTBOX_INTEGRATION=1`.
