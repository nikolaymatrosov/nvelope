# Phase 1 Data Model: Deliverability & Analytics

All tenant-plane tables carry `tenant_id` and `ENABLE`/`FORCE ROW LEVEL SECURITY`
with the standard `tenant_isolation` policy
(`tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid`),
identical to migrations 000004–000010. `inbound_feedback_events` is the one
control-plane (non-tenant) table — it stages raw notifications before the owning
tenant is known.

Migrations: `000011_delivery_feedback`, `000012_suppression`,
`000013_campaign_analytics`.

## Migration 000011 — delivery feedback

### Table: `inbound_feedback_events` (control-plane, no RLS)

Staging record for every notification the stream consumer reads, before
attribution.

| Column | Type | Notes |
| --- | --- | --- |
| `id` | `uuid` PK | `gen_random_uuid()` |
| `dedupe_key` | `text` NOT NULL | `UNIQUE` — the provider `eventId`; the idempotency key |
| `event_kind` | `text` NOT NULL | `CHECK IN ('bounce','complaint','delivery','open','click')` |
| `provider_message_id` | `text` NOT NULL | `mail.messageId` of the originating send |
| `recipient_email` | `text` NOT NULL | |
| `occurred_at` | `timestamptz` NOT NULL | provider-reported event time |
| `raw_payload` | `jsonb` NOT NULL | the verified notification body, retained for audit |
| `status` | `text` NOT NULL DEFAULT `'pending'` | `CHECK IN ('pending','attributed','unattributed','failed')` |
| `received_at` | `timestamptz` NOT NULL DEFAULT `now()` | |
| `processed_at` | `timestamptz` | set by `feedback.process` |

Not RLS-protected and not used for tenant queries; written/read by the consumer
and the `feedback.process` worker through the pool, since the tenant is not yet
known at ingestion. `nvelope_app` is granted `SELECT, INSERT, UPDATE`. Indexed on
`status` for the worker and on `received_at` for monitoring.

### Table: `delivery_events` (tenant-plane, RLS)

The attributed feedback events — bounces, complaints, deliveries, opens, and
clicks. This is the single feedback table all analytics counts (except *sent*)
aggregate from.

| Column | Type | Notes |
| --- | --- | --- |
| `id` | `uuid` PK | `gen_random_uuid()` |
| `tenant_id` | `uuid` NOT NULL | FK `tenants(id)` ON DELETE CASCADE |
| `inbound_event_id` | `uuid` NOT NULL | FK `inbound_feedback_events(id)`; `UNIQUE` — one delivery event per staged notification |
| `event_kind` | `text` NOT NULL | `CHECK IN ('bounce','complaint','delivery','open','click')` |
| `recipient_email` | `text` NOT NULL | |
| `campaign_id` | `uuid` | FK `campaigns(id)` ON DELETE SET NULL; null for transactional |
| `campaign_recipient_id` | `uuid` | FK `campaign_recipients(id)` ON DELETE SET NULL |
| `transactional_message_id` | `uuid` | FK `transactional_messages(id)` ON DELETE SET NULL |
| `provider_message_id` | `text` NOT NULL | |
| `occurred_at` | `timestamptz` NOT NULL | |
| `created_at` | `timestamptz` NOT NULL DEFAULT `now()` | |

`CHECK`: exactly one of `campaign_recipient_id` / `transactional_message_id` is
set. Indexed on `(campaign_id, event_kind)` for the analytics aggregate and on
`(tenant_id, recipient_email)`. There is no `bounce_type` column — every provider
bounce is permanent (hard).

### Table: `transactional_messages` (tenant-plane, RLS)

A record of each transactional send, so a transactional bounce/complaint/open can
be attributed. Written by the `SendTransactional` handler.

| Column | Type | Notes |
| --- | --- | --- |
| `id` | `uuid` PK | `gen_random_uuid()` |
| `tenant_id` | `uuid` NOT NULL | FK `tenants(id)` ON DELETE CASCADE |
| `template_id` | `uuid` | FK `templates(id)` ON DELETE SET NULL |
| `provider_message_id` | `text` NOT NULL | `UNIQUE` |
| `recipient_email` | `text` NOT NULL | |
| `sent_at` | `timestamptz` NOT NULL DEFAULT `now()` | |

### Alter: `campaign_recipients`

- Add `provider_message_id text` — the provider reference returned at send time;
  populated by `batch_worker` on a successful send.
- Add `'skipped'` to the `status` `CHECK` constraint (now
  `IN ('pending','sent','failed','skipped')`); a recipient skipped by the
  pre-send suppression check is `skipped` with the reason in `failure_reason`.
- Index `campaign_recipients (provider_message_id)` for attribution lookups.

### Function: `feedback_tenant_for_message` (`SECURITY DEFINER`)

Resolves the owning tenant for a provider message id without an `app.tenant_id`
bound, mirroring the Phase 3 `tracking_tenant_for_*` functions. Returns one
`tenant_id` by checking `campaign_recipients` then `transactional_messages`;
returns null when unmatched. `REVOKE ALL … FROM PUBLIC`, `GRANT EXECUTE … TO
nvelope_app`.

## Migration 000012 — suppression

### Table: `suppression_list` (tenant-plane, RLS)

| Column | Type | Notes |
| --- | --- | --- |
| `id` | `uuid` PK | `gen_random_uuid()` |
| `tenant_id` | `uuid` NOT NULL | FK `tenants(id)` ON DELETE CASCADE |
| `email` | `text` NOT NULL | stored lower-cased |
| `reason` | `text` NOT NULL | `CHECK IN ('hard_bounce','complaint','manual')` |
| `source_event_id` | `uuid` | FK `delivery_events(id)` ON DELETE SET NULL; null for manual entries |
| `suppressed_at` | `timestamptz` NOT NULL DEFAULT `now()` | |
| `note` | `text` NOT NULL DEFAULT `''` | optional operator note for manual entries |

`UNIQUE (tenant_id, email)` — an address is suppressed at most once per tenant; a
later event of a different reason does not duplicate the row. There is no
`soft_bounce_threshold` reason — soft bounces are out of scope this phase.

### Table: `bounce_settings` (tenant-plane, RLS)

Per-tenant bounce-action configuration. A row is created lazily; until then the
defaults below apply.

| Column | Type | Notes |
| --- | --- | --- |
| `tenant_id` | `uuid` PK | FK `tenants(id)` ON DELETE CASCADE |
| `suppress_on_hard_bounce` | `boolean` NOT NULL DEFAULT `true` | |
| `suppress_on_complaint` | `boolean` NOT NULL DEFAULT `true` | |
| `updated_at` | `timestamptz` NOT NULL DEFAULT `now()` | |

There is no `soft_bounce_threshold` column and no `soft_bounce_counters` table —
soft bounces are dropped from this phase (clarified 2026-05-18).

## Migration 000013 — campaign analytics

### Table: `campaign_analytics` (tenant-plane, RLS)

Pre-computed per-campaign roll-up. **Not** a native materialized view — see
research R4. Refreshed by the `analytics.refresh` job.

| Column | Type | Notes |
| --- | --- | --- |
| `campaign_id` | `uuid` PK | FK `campaigns(id)` ON DELETE CASCADE |
| `tenant_id` | `uuid` NOT NULL | FK `tenants(id)` ON DELETE CASCADE |
| `sent_count` | `integer` NOT NULL DEFAULT `0` | `campaign_recipients.status = 'sent'` |
| `delivered_count` | `integer` NOT NULL DEFAULT `0` | distinct recipients with a `delivery` event |
| `opened_count` | `integer` NOT NULL DEFAULT `0` | distinct recipients with an `open` event |
| `clicked_count` | `integer` NOT NULL DEFAULT `0` | distinct recipients with a `click` event |
| `bounced_count` | `integer` NOT NULL DEFAULT `0` | distinct recipients with a `bounce` event |
| `complained_count` | `integer` NOT NULL DEFAULT `0` | distinct recipients with a `complaint` event |
| `refreshed_at` | `timestamptz` NOT NULL DEFAULT `now()` | last refresh time |

Indexed on `(tenant_id)` for the workspace dashboard. Rates (open/click/bounce/
complaint) are derived on read, not stored. All six event counts come from
`delivery_events`; only `sent_count` comes from `campaign_recipients`.

## Domain entities

- **DeliveryEvent** — a verified, attributed feedback event. Unexported fields;
  validating constructor `NewDeliveryEvent` rejects an unknown kind or an event
  attributed to neither a campaign recipient nor a transactional message.
  Separate `HydrateDeliveryEvent` ("persistence only, not a constructor")
  rebuilds a row from storage. Behaviour: `SuppressionReason()` maps a bounce to
  `hard_bounce` and a complaint to `complaint` (and reports `ok=false` for
  delivery/open/click), `IsBounce()`, `IsComplaint()`.
- **InboundNotification** — the parsed notification before attribution.
  Constructed from a stream record's JSON; carries the dedupe key (`eventId`),
  kind, provider message id, recipient, occurred-at, and the raw payload.
- **SuppressionEntry** — an address suppressed for a tenant. Validating
  constructor `NewSuppressionEntry` lower-cases and validates the email and
  requires a known reason (`hard_bounce`, `complaint`, `manual`).
  `NewManualSuppression` is the manual-add path. Separate
  `HydrateSuppressionEntry` for loads.
- **BounceSettings** — per-tenant configuration. Constructor and a `Default()`
  yielding both toggles on. Behaviour: `ShouldSuppressHardBounce()`,
  `ShouldSuppressComplaint()`.
- **CampaignAnalytics** / **Dashboard** — read-model value objects: the six
  counts plus derived rates (zero denominator → 0.0); built by the analytics
  query handlers from `campaign_analytics` rows. No mutating behaviour.

## State transitions

- **`inbound_feedback_events.status`**: `pending` → `attributed` (a
  `delivery_events` row was written and, for a bounce/complaint, suppression
  applied) or `unattributed` (no send matched the provider message id; FR-006)
  or `failed` (a transient processing error; the River job retries).
- **`campaign_recipients.status`**: the Phase 3 lifecycle
  `pending` → `sent` | `failed` gains `pending` → `skipped`, set by the pre-send
  suppression check before any provider call.
- **`suppression_list`**: an address is inserted once on a hard bounce, a
  complaint, or a manual add; manual removal deletes the row, after which the
  address is mailable again.
