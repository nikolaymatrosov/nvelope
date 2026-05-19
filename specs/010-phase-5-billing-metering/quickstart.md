# Quickstart: Phase 5 — Billing & Metering

Backend-only phase. It adds the `internal/billing` context, two migrations,
three River jobs, billing HTTP routes, and quota enforcement on the send paths.

## Prerequisites

- Go 1.26, a running Docker daemon (testcontainers spins up `postgres:17`).
- The Phase 1–4 schema and code in place (this branch builds on them).

## Build & test

```sh
make test            # full suite, incl. new billing integration tests
go build ./...       # all four services compile
```

Integration tests start a `postgres:17` container automatically. To run against
an existing database, set `NVELOPE_MIGRATE_DATABASE_URL`.

## Apply migrations

```sh
go run ./cmd/migrate up      # applies 000014_billing + 000015_usage_metering
```

A clean down-migration must also work:

```sh
go run ./cmd/migrate down 2  # drops usage_metering then billing
```

## Configuration (new keys, `NVELOPE_` prefix)

| Key | Default | Purpose |
|---|---|---|
| `BILLING_SWEEP_INTERVAL` | `1h` | how often the scheduler enqueues `billing.sweep` |
| `USAGE_ROLLUP_INTERVAL` | `15m` | how often it enqueues per-tenant `usage.rollup` |
| `DUNNING_MAX_ATTEMPTS` | `3` | failed charges before suspension |
| `DUNNING_RETRY_INTERVAL` | `72h` | spacing between dunning retries |

The `MockGateway` needs no configuration — it approves by default and is
programmed in-process by tests.

## Run the services

```sh
go run ./cmd/api         # serves /t/{slug}/api/plans, /subscription, /invoices
go run ./cmd/worker      # runs billing.sweep / billing.charge / usage.rollup
go run ./cmd/scheduler   # enqueues billing.sweep + per-tenant usage.rollup
```

## Manual verification (the spec's exit criteria)

Seed a `published` plan in `plans`, then as a tenant admin:

1. **Subscribe** — `POST /t/{slug}/api/subscription {"planId": "..."}` →
   `201`, subscription `active`, first invoice `paid` (US1).
2. **Meter** — send a campaign, then a transactional message. Run `usage.rollup`
   (or wait a tick) → `GET /subscription` shows `usedSends` rising (US3).
3. **Enforce** — on a `block`-mode plan, exhaust `includedSends`; the next
   campaign start fails with `quota_exceeded`. On a `meter`-mode plan it
   proceeds and `overageSends` rises (US4).
4. **Renew** — advance a subscription's `current_period_end` into the past, run
   `billing.sweep` → a new invoice is generated and charged, the period
   advances (US2).
5. **Dunning → suspension** — program the `MockGateway` to decline the tenant's
   charges; run the sweep `DUNNING_MAX_ATTEMPTS` times across
   `DUNNING_RETRY_INTERVAL` → subscription `suspended`, sends now rejected with
   `tenant_suspended` (US5).
6. **Reinstate** — `POST /invoices/{id}/settle` with the gateway approving →
   invoice `paid`, subscription `active`, sending re-enabled.

## What this phase does NOT include

- A real Russian payment-provider integration (later phase — additive behind
  the same `PaymentGateway` port).
- Proration, mid-cycle plan upgrades/downgrades.
- The billing UI (later frontend increment).

## Definition of done

- `go test ./...` green, including new billing integration and cross-tenant
  isolation tests.
- `cmd/migrate up` then `down` both clean.
- All five user stories demonstrable per the steps above.
- Constitution Check still PASS (see `plan.md`).
