# nvelope — Implementation Plan

Phased implementation plan for the multi-tenant newsletter SaaS. Full listmonk feature parity is
the target; it is delivered across phases so each phase is independently shippable. Each phase
ends with passing tests and a working deployment.

See `architecture.md` for design detail and `user-stories.md` for the requirements each phase
satisfies.

---

## Phase 0 — Foundations & Docs

- **0.1** Create the `nvelope` repository; commit `docs/architecture.md`, `docs/user-stories.md`,
  and this plan.
- **0.2** Scaffold the three Go services (`cmd/api`, `cmd/worker`, `cmd/scheduler`) with shared
  config loading and structured logging.
- **0.3** Set up PostgreSQL, `golang-migrate` migration tooling, and a React/Vite frontend
  skeleton.
- **0.4** CI pipeline (build, test, lint), Dockerfiles for each service, and base Kubernetes/Helm
  manifests.

**Exit criteria:** all three services build and start; CI is green; a migration applies cleanly.

---

## Phase 1 — Tenancy Core

- **1.1** Control-plane schema: `tenants`, `platform_users`, `platform_user_tenants`.
- **1.2** RLS pattern: per-request transaction helper that runs `SET LOCAL app.tenant_id`;
  connect as a non-superuser, non-`BYPASSRLS` role.
- **1.3** Tenant resolution middleware (path `/t/{slug}/...`) with a cross-check against the
  authenticated session.
- **1.4** Platform signup/login, tenant creation, and team invites.
- **1.5** Automated tests proving cross-tenant isolation — tenant A cannot read or write tenant
  B's rows even when an application-level filter is omitted.

**Exit criteria:** a user can sign up, create a tenant, and invite a teammate; isolation tests
pass. *(Satisfies Epic A.)*

---

## Phase 2 — Subscribers, Lists & Auth

- **2.1** Tenant-plane schema: `lists`, `subscribers`, `subscriber_lists`, `roles`, `users`,
  `sessions`, `settings` — each with `tenant_id` + RLS.
- **2.2** Tenant RBAC: user-level and per-list roles, permission strings, scoped API keys, 2FA
  (TOTP).
- **2.3** Subscriber and list CRUD, custom JSON attributes, query/segment-based selection.
- **2.4** CSV/ZIP subscriber import (with upsert) and export.

**Exit criteria:** a tenant user can manage lists and subscribers, import/export, and the RBAC
gates work. *(Satisfies Epic D, part of Epic H.)*

---

## Phase 3 — Sending Pipeline — **delivered**

- **3.1** River integration; job queue definitions and worker registration. Three new job
  kinds (`domain.verify`, `campaign.start`, `campaign.batch`) on a dedicated `sending` queue.
- **3.2** `sending_domains` schema; Postbox domain provisioning and the `domain.verify` polling
  job, with a scheduler recovery sweep that re-arms lost verification jobs.
- **3.3** Postbox SES-compatible messenger with AWS SigV4 request signing.
- **3.4** Redis-coordinated per-tenant and global sliding-window rate limiting.
- **3.5** Templates and campaigns schema; the `campaign.start` → `campaign.batch` send
  pipeline with per-recipient dedup/resumability; open-pixel and click-tracking link generation
  served from public, tenant-resolving endpoints.
- **3.6** Transactional `tx` API endpoint authenticated by a scoped API key.

**Exit criteria:** a tenant can verify a domain and send a campaign through Postbox with
tracking. *(Satisfies Epic C, core of Epic E and Epic F.)* — **met.**

> **Delivered notes / divergences from the original outline.** The `tx` endpoint and the
> sending-domain/campaign routes use a small set of new permission strings
> (`sending:*`, `campaigns:*`, `transactional:send`) added to the IAM catalogue rather than
> reusing existing scopes. Usage events (`usage_events`) are deferred to Phase 5, where the
> usage table and rollup actually live; the send pipeline does not emit them yet. SPF and
> DMARC records are composed by the platform (Postbox returns only DKIM tokens).

---

## Phase 4 — Deliverability & Analytics

- **4.1** Postbox bounce/complaint webhook ingestion with signature verification.
- **4.2** `suppression_list`, configurable bounce actions, and pre-send suppression checks.
- **4.3** Campaign analytics (opens/clicks/bounces/complaints) and dashboard materialized views.

**Exit criteria:** bounces/complaints are attributed and suppressed automatically; analytics and
dashboard render. *(Completes Epic F.)*

---

## Phase 5 — Billing & Metering

- **5.1** `plans`, `tenant_subscriptions`, `invoices`, `invoice_line_items`,
  `payment_attempts`; the `PaymentGateway` interface with a deterministic `MockGateway`.
- **5.2** In-house subscription engine: lifecycle state machine, `billing.sweep` /
  `billing.charge` jobs, invoice generation, and dunning (retries → suspension).
- **5.3** `usage_events`, the `usage.rollup` job, and `usage_counters`.
- **5.4** Quota enforcement at campaign start and transactional send (`block` vs `meter`
  overage modes); tenant suspension on payment failure.

**Exit criteria:** a tenant can subscribe to a plan, recurring renewals charge through the
mock gateway, usage is metered, quotas are enforced, and payment failure runs dunning then
suspends sending. *(Satisfies Epic B.)* Real Russian payment-provider integration is a
later phase.

---

## Phase 6 — Public Pages & Media

- **6.1** Public subscription page, double-opt-in flow, and preference management.
- **6.2** Campaign archive and RSS feed with per-tenant branding/CSS.
- **6.3** Media library backed by S3-compatible object storage, tenant-prefixed.

**Exit criteria:** subscribers can self-serve via public pages; media uploads work.
*(Satisfies Epic G, completes Epic H.)*

---

## Phase 7 — Parity Completion & Hardening

- **7.1** Visual email editor, A/B testing, and advanced segmentation.
- **7.2** Platform admin console and audit-log UI.
- **7.3** Load testing; security review (RLS, SigV4, webhook signatures, API keys);
  observability (metrics, tracing, alerting).
- **7.4** Production Kubernetes rollout, backups, and operational runbooks.

**Exit criteria:** full listmonk feature parity reached; security and load reviews signed off;
production-ready. *(Completes Epic E and Epic A.)*

---

## Verification Strategy

- **Isolation:** automated tests create two tenants and assert, via RLS, that tenant A cannot
  read or write tenant B's rows even with a missing application-level filter.
- **Sending:** end-to-end test sends a campaign through Postbox in a staging account; confirm
  DKIM-signed delivery, open/click tracking, and bounce/complaint attribution.
- **Domains:** add a real test domain; verify DKIM/SPF/DMARC detection and status transitions.
- **Billing:** subscribe a tenant and drive renewals through the `MockGateway`; simulate
  usage; confirm quota enforcement, overage line items, and the dunning → suspension path on
  a declined charge.
- **Jobs:** kill a worker pod mid-campaign; confirm River retries and the campaign resumes
  without duplicate sends.
- **Load:** simulate concurrent campaigns across multiple tenants; confirm per-tenant fairness
  and that global rate limits protect the Postbox account.
- **Standard, every phase:** `go test ./...`, frontend tests, lint, and a clean migration apply.

---

## Risks & Mitigations

| Risk | Mitigation |
|---|---|
| RLS misconfiguration leaks data | Non-`BYPASSRLS` role; isolation tests in CI; explicit app-level filters as belt-and-suspenders |
| Postbox API differs from AWS SES in edge cases | Thin messenger abstraction; integration tests against a real Postbox staging account |
| One large tenant starves others when sending | Per-tenant River queues/priorities; Redis per-tenant rate limits |
| Quota races across worker pods | Centralize counters in Redis; enforce at campaign start, not just per-message |
| Full parity in v1 is large | Phased delivery — each phase ships; parity is the Phase 7 end state, not a Phase 1 gate |
