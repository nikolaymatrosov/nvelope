# nvelope — Architecture Documentation

> A managed, multi-tenant SaaS newsletter / email-marketing platform.
> Mail provider: **Yandex Postbox** (AWS SES-compatible API).

## Background

This platform is a multi-tenant SaaS reimagining of [listmonk](https://listmonk.app), a mature
**single-tenant, self-hosted** newsletter manager (Go + Vue). listmonk runs one organization per
instance: global settings, no tenant isolation, DB-polling campaign workers, and SMTP sending.

nvelope is a **greenfield rewrite**. It keeps listmonk's proven domain model and sending
algorithms as a reference, but is built from the ground up for tenancy, billing, and cloud
operation.

### Locked-in decisions

| Decision | Choice |
|---|---|
| Codebase | Greenfield rewrite (listmonk as reference, not fork) |
| Stack | Go backend + React/TypeScript frontend, PostgreSQL |
| Tenant isolation | Shared schema, `tenant_id` on every tenant table, Postgres RLS |
| Tenant routing | Single domain, path-based (`/t/{slug}/...`) |
| Mail provider | Yandex Postbox via SES-compatible HTTP API (SigV4) |
| Sending domains | Per-tenant verified domains (DKIM/SPF/DMARC) provisioned in Postbox |
| Billing | In scope — plans, usage metering, in-house subscription engine + pluggable payment gateway |
| Jobs | Dedicated queue (River, Postgres-backed) |
| Deployment | Containers on Kubernetes |
| Feature target | Full listmonk feature parity + multi-tenancy (delivered in phases) |

---

## 1. System Overview

nvelope is a horizontally scalable, stateless set of Go services backed by one PostgreSQL
database, fronted by a React SPA, sending mail through Yandex Postbox.

```
                 ┌──────────────────────────────────────────┐
   Browser ──────┤  React SPA (admin app + platform area)    │
   Subscriber ───┤  Public pages (subscribe / optin / archive)│
                 └───────────────────┬──────────────────────┘
                                     │ HTTPS
                          ┌──────────▼──────────┐
                          │   API service       │  tenant resolution, REST,
                          │   (Go, Echo/chi)    │  public pages, webhooks
                          └──┬───────┬───────┬──┘
            enqueue jobs     │       │       │  read/write (RLS tx)
                  ┌──────────▼──┐ ┌──▼─────┐ │
                  │ Worker svc  │ │Scheduler│ │
                  │ (River)     │ │ svc     │ │
                  └──┬───────┬──┘ └──┬──────┘ │
                     │       │       │        │
        ┌────────────▼─┐ ┌───▼────┐ ┌▼────────▼─────┐
        │ PostgreSQL   │ │ Redis  │ │ Object storage│
        │ (RLS + River)│ │counters│ │ S3-compatible │
        └──────────────┘ │ cache  │ │  media        │
                          └────────┘ └───────────────┘
                     │
        ┌────────────▼────────────┐   ┌──────────────────────┐
        │ Yandex Postbox (SES API)│   │ Payment gateway       │
        │ + bounce/complaint hooks│   │ (pluggable; mock dev) │
        └─────────────────────────┘   └──────────────────────┘
```

### Services (three Go deployables, all stateless)

1. **API service** — REST API for the admin SPA and platform area; public subscription /
   opt-in / archive / RSS pages; bounce & complaint webhook receivers. Resolves the tenant
   on every request and opens an RLS-scoped DB transaction.
2. **Worker service** — River job consumers: campaign batch sending, subscriber import,
   double-opt-in mail, domain-verification polling, usage rollups, stats refresh,
   webhook payload processing.
3. **Scheduler service** — promotes scheduled campaigns to running, enqueues periodic jobs
   (usage rollups, materialized-view refresh, unconfirmed-subscription cleanup, domain re-check,
   `billing.sweep` for due subscription renewals and dunning retries).

Worker and Scheduler are separate so sending capacity scales independently of cron-style work;
both are thin and can be co-deployed in early stages.

### Supporting infrastructure

- **PostgreSQL** — single shared database; hosts application tables (RLS) and River job tables.
- **Redis** — cross-pod rate-limit counters (per-tenant + global sliding window), hot caches.
- **Object storage** — S3-compatible bucket for media uploads, keyed by `tenant_id` prefix.
- **Yandex Postbox** — SES-compatible API for sending; notifications for bounces/complaints sent thought YDB topic.
- **Payment gateway** — external acquirer reached through a pluggable `PaymentGateway`
  interface (tokenize card, charge token, refund); a deterministic mock is used in dev.
  Subscription lifecycle, dunning, overage, and invoices are owned in-house, not by the gateway.

---

## 2. Multi-Tenancy Model

- **Two logical schemas in one database:**
  - **Control plane** (no RLS): `tenants`, `plans`, `tenant_subscriptions`, `usage_events`,
    `usage_counters`, `platform_users`, `audit_log`.
  - **Tenant plane** (RLS-enforced): every table carries a non-null `tenant_id`.
- **Row-Level Security as the isolation backstop.** Each API request opens a transaction and
  runs `SET LOCAL app.tenant_id = '<uuid>'`. RLS policies
  (`USING tenant_id = current_setting('app.tenant_id')::uuid`) filter every read and write, so a
  forgotten `WHERE tenant_id = …` cannot leak or corrupt another tenant's data. The application
  also filters explicitly — RLS is defense in depth.
- **Connection role:** the app connects as a non-superuser, non-`BYPASSRLS` role so policies
  always apply.
- **Tenant resolution:** tenant slug comes from the URL path (`/t/{slug}/api/...` for admin,
  `/t/{slug}/subscription` etc. for public pages). The resolved tenant is cross-checked against
  the authenticated session / API key; mismatches are rejected.
- **Worker tenancy:** every River job payload includes `tenant_id`; the worker sets the same
  `SET LOCAL` before touching tenant data.

---

## 3. Data Model

### Control-plane tables (global, no RLS)

| Table | Purpose / key columns |
|---|---|
| `tenants` | id (uuid), slug (unique), name, status (active/suspended/deleted), plan_id, created_at |
| `plans` | id, name, price_amount (minor units, RUB), billing_interval, limits JSONB (max_subscribers, emails_per_month, max_domains, max_users), overage_mode (block/meter), overage_rates JSONB, active |
| `tenant_subscriptions` | tenant_id, plan_id, status (trialing/active/past_due/suspended/canceled), current_period_start/end, card_token, card_last4/brand, dunning_attempt, next_retry_at, cancel_at_period_end |
| `invoices` | id, tenant_id, subscription_id, number, status (draft/paid/failed/void), period_start/end, currency, total_amount, issued_at, paid_at |
| `invoice_line_items` | id, invoice_id, kind (plan/overage), description, quantity, unit_amount, amount |
| `payment_attempts` | id, invoice_id, gateway, gateway_payment_id, status (pending/succeeded/failed), amount, error_code, created_at |
| `usage_events` | tenant_id, type (email_sent/import/...), quantity, campaign_id, occurred_at |
| `usage_counters` | tenant_id, period, emails_sent, subscribers_count, … (rollup target) |
| `platform_users` | id, email, password_hash, name — owners/admins for signup & billing |
| `platform_user_tenants` | platform_user_id, tenant_id, role (owner/admin) — user↔tenant link |
| `audit_log` | tenant_id (nullable), actor, action, target, meta JSONB, created_at |

### Tenant-plane tables (all have `tenant_id` + RLS)

Mirrors listmonk's model: `subscribers`, `subscriber_lists`, `lists`, `campaigns`,
`campaign_lists`, `campaign_media`, `templates`, `media`, `links`, `link_clicks`,
`campaign_views`, `bounces`, `users`, `roles`, `sessions`, `settings`.

New tenant-plane tables:

| Table | Purpose |
|---|---|
| `sending_domains` | tenant_id, domain, status (pending/verified/failed), dkim/spf/dmarc records & check state, postbox_identity_ref |
| `api_keys` | tenant_id, name, hashed key, scopes, last_used_at |
| `suppression_list` | tenant_id, email, reason (hard_bounce/complaint/unsubscribe/manual), created_at — checked before every send |

Per-tenant materialized views (or filtered aggregate tables) for dashboard counts and 30-day
charts, refreshed by a scheduled job.

### Migrations

`tenant_id` is included from the first migration — no retrofit. Migrations are versioned
(e.g. `golang-migrate`) and applied in a pre-deploy job. River's tables are created by River's
own migrations.

---

## 4. Email Sending via Yandex Postbox

- **Postbox messenger:** a Go component implementing a `Messenger` interface
  (`Name / Send / Close`), calling Postbox's SES-compatible `SendRawEmail` endpoint with
  **AWS SigV4** request signing using per-environment Postbox credentials.
- **Per-tenant sending domains:**
  1. Tenant adds a From domain in the UI.
  2. Worker job provisions the domain identity in Postbox and stores DKIM/SPF/DMARC records.
  3. UI shows the DNS records the tenant must add.
  4. A `domain.verify` job polls Postbox until the identity is verified; status surfaced in UI.
  5. Campaigns can only send from a verified domain owned by the tenant.
- **Attribution:** each message carries a Postbox **configuration set / message tag** plus
  `X-Tenant`, `X-Campaign`, `X-Subscriber` headers, so bounce/complaint notifications map back
  to the exact tenant, campaign, and subscriber.
- **Rate limiting:** per-tenant send rate derived from the tenant's plan, enforced via Redis
  counters shared across worker pods, plus a global sliding-window cap to protect the Postbox
  account. Mirrors listmonk's sliding-window algorithm but coordinated cross-pod.
- **Tracking:** open pixel and click-tracking links generated per message (listmonk-style),
  scoped by tenant; tracking endpoints resolve tenant from the link UUID.
- **Transactional API:** `POST /t/{slug}/api/tx` for transactional sends using `tx`-type
  templates, authenticated by API key, counted against usage.

---

## 5. Job Processing

- **River** (Postgres-backed Go job queue): jobs are enqueued in the same transaction as the
  data change, with retries + exponential backoff, scheduled jobs, and unique-job support.
- **Per-tenant fairness:** a campaign send is split into many `campaign.batch` jobs; the worker
  uses River queues/priorities so one large tenant cannot starve others.
- **Job types:** `campaign.batch`, `campaign.start`, `import.subscribers`, `optin.send`,
  `domain.verify`, `webhook.process`, `usage.rollup`, `stats.refresh`, `cleanup.unconfirmed`,
  `billing.charge`.
- **Failure handling:** `max_send_errors` per campaign → campaign auto-paused, like listmonk.

---

## 6. Billing & Usage Metering

- **Plans** define quotas: max subscribers, emails/month, sending domains, team members,
  custom-domain support, feature flags. Each plan also sets an `overage_mode` — `block` or
  `meter` — and `overage_rates`.
- Every send / import emits a `usage_event`; `usage.rollup` aggregates into `usage_counters`
  per billing period.
- **Quota enforcement** at campaign start and transactional send: `block`-mode plans
  hard-block over limit; `meter`-mode plans allow over-limit sends and accrue overage.
  Soft-warn near limit (UI banner + email).
- **In-house subscription engine.** Subscription lifecycle, dunning, overage, and invoices
  are owned by nvelope, not an external billing provider. The `PaymentGateway` interface
  (tokenize / charge / refund) is the only provider-specific seam; a deterministic mock is
  used in dev, with a Russian acquirer integrated in a later phase.
- **Renewals.** A periodic `billing.sweep` (Scheduler) finds subscriptions due to renew or
  due for a dunning retry and enqueues unique `billing.charge` jobs (Worker). `billing.charge`
  builds an invoice (plan + overage line items), charges the saved card token, and advances
  the period on success.
- **Dunning.** A failed renewal charge moves the subscription to `past_due` and schedules
  retries (days 1/3/5/7). After 4 failed attempts the subscription becomes `suspended` →
  `tenants.status = suspended`, which blocks sending but preserves data. Adding a working
  card retries the charge immediately.

---

## 7. Authentication & Authorization

- **Platform users** — sign up, own/manage tenants, billing. Stored in `platform_users`,
  linked to tenants via `platform_user_tenants`.
- **Tenant users** — listmonk-style RBAC inside a tenant: `roles` (user-level + per-list),
  permission string arrays, `users` of type `user` or `api`. 2FA (TOTP) and optional OIDC.
- **Sessions** for the SPA; **scoped API keys** (per tenant) for the public/transactional API.
- **Platform admin** — internal staff role for cross-tenant support. `audit_log` records every
  cross-tenant action; access via a `BYPASSRLS`-capable path is explicit and logged.

---

## 8. Bounce & Complaint Handling

- Postbox bounce/complaint notifications received at a webhook endpoint, **signature-verified**,
  enqueued as `webhook.process` jobs.
- Worker attributes the event to tenant/campaign/subscriber via the configuration set tag /
  headers, writes a `bounces` row, and adds hard bounces + complaints to the tenant's
  `suppression_list`.
- Configurable per-tenant bounce-action thresholds (e.g. N hard bounces → blocklist subscriber),
  mirroring listmonk's bounce actions.

---

## 9. Frontend

- **React + TypeScript** SPA (Vite). Two areas:
  - **Platform area** — signup/login, tenant creation, billing & plan, sending domains, team.
  - **Tenant admin app** — dashboard, campaigns + editor, subscribers + import, lists,
    templates, media library, analytics, settings, users/roles, bounces, logs.
- **Public pages** — subscription management, double-opt-in confirmation, campaign archive +
  RSS; tenant-scoped by path, lightweight render, customizable per-tenant CSS/branding.
- API client is a typed REST layer; tenant slug is part of every admin route.

---

## 10. Repository Structure (target)

Each bounded context is split into a calibrated three layers — `domain` (pure
rules, validating constructors, consumer-owned repository interfaces, typed
errors), `app` (command/ and query/ handlers), and `adapters` (pgx
repositories) — with one shared transport layer (`api`) and a single
composition root (`service.NewApplication`). The full per-aggregate
`ports/app/domain/adapters` split is intentionally not used: nvelope is one
HTTP service, so a per-context `ports/` directory would be ceremony without
payoff. The inward dependency rule is enforced in CI with `go-cleanarch`.

```
nvelope/
  cmd/{api,worker,scheduler}/main.go
  internal/
    platform/      shared building blocks: apperr (typed errors), decorator (CQRS)
    auth/          platform identity — bounded context
      domain/        User, Session, credential value objects, repository interfaces
      app/           command/ (SignUp, LogIn, LogOut) + query/ handlers
      adapters/      pgx repositories, bcrypt password hasher
    tenant/        tenancy & RLS — bounded context
      domain/        Tenant, Membership, Invitation, TenantSettings, interfaces
      app/           command/ + query/ handlers
      adapters/      pgx repositories, the RLS-bound transaction helper
    api/           the single HTTP transport layer (router, middleware, errmap)
    service/       composition root: NewApplication wires every layer
    config/ db/ health/ logging/ token/ dbtest/   shared infrastructure
  test/            cross-tenant isolation + migration suites
  frontend/        React + TypeScript SPA
  deploy/          Dockerfiles, K8s manifests / Helm chart
  docs/            architecture.md, user-stories.md, implementation-plan.md
```

Future contexts (billing, subscribers, lists, campaigns, …) follow the same
`domain` / `app` / `adapters` shape under `internal/<ctx>/`.

---

## 11. Key Differences from listmonk

| Aspect | listmonk (single-tenant) | nvelope (multi-tenant SaaS) |
|---|---|---|
| Tenancy | One org per instance | Many tenants, shared DB + RLS |
| Settings | Global `settings` table | Per-tenant settings + control-plane plans |
| Sending | SMTP messenger | Postbox SES-compatible API + SigV4 |
| Sending domains | Global from-address | Per-tenant verified domains in Postbox |
| Jobs | DB-polling campaign workers | River queue with per-tenant fairness |
| Billing | None | Plans, usage metering, in-house subscription engine + pluggable gateway |
| Routing | Single instance | Path-based `/t/{slug}/...` |
| Frontend | Vue 2 | React + TypeScript |
| Deployment | Single binary / VM | Stateless containers on Kubernetes |
