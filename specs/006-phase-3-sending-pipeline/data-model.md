# Phase 1 Data Model: Sending Pipeline

All tables below are **tenant-plane** tables: each carries `tenant_id` and is
protected by Row-Level Security with the standard policy already used in Phases 1
and 2:

```sql
ALTER TABLE <t> ENABLE ROW LEVEL SECURITY;
ALTER TABLE <t> FORCE  ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON <t>
    USING      (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);
GRANT SELECT, INSERT, UPDATE, DELETE ON <t> TO nvelope_app;
```

Migrations: `000008_sending_domains`, `000009_templates_campaigns`,
`000010_campaign_tracking`.

---

## Migration 000008 — `sending_domains`

### Table: `sending_domains`

| Column | Type | Notes |
| --- | --- | --- |
| `id` | uuid PK | `gen_random_uuid()` |
| `tenant_id` | uuid NOT NULL | FK → `tenants(id)` ON DELETE CASCADE |
| `domain` | text NOT NULL | the From domain, e.g. `mail.acme.com` |
| `status` | text NOT NULL | `pending` \| `verified` \| `failed`; default `pending` |
| `dkim_records` | jsonb NOT NULL | DKIM CNAME/TXT records returned by Postbox; default `[]` |
| `spf_record` | text NOT NULL | platform-composed SPF record; default `''` |
| `dmarc_record` | text NOT NULL | platform-composed DMARC record; default `''` |
| `postbox_identity_ref` | text NOT NULL | provider identity reference; default `''` |
| `failure_reason` | text NOT NULL | actionable reason when `status = failed`; default `''` |
| `created_at` | timestamptz NOT NULL | `now()` — also the verification-window anchor |
| `verified_at` | timestamptz NULL | set when `status` → `verified` |
| `last_checked_at` | timestamptz NULL | updated on every `domain.verify` poll |

**Constraints / indexes**: `UNIQUE (tenant_id, domain)`; `CHECK (status IN
('pending','verified','failed'))`; index on `(tenant_id, status)`.

**Validation rules** (enforced in the `SendingDomain` constructor):
- `domain` is a syntactically valid, lowercased domain name.
- A new domain is always created `pending` with `verified_at` null.

**State transitions** (`SendingDomain` methods):
- `pending → verified` — `MarkVerified(at)`; sets `verified_at`. Only from `pending`.
- `pending → failed` — `MarkFailed(reason)`; sets `failure_reason`. Only from `pending`.
- `RecordCheck(at)` — updates `last_checked_at` without changing `status`.
- `verified` and `failed` are terminal for the polling job; a tenant re-check is
  rejected unless `status = pending`.

---

## Migration 000009 — `templates`, `campaigns`, `campaign_lists`, `campaign_recipients`

### Table: `templates`

| Column | Type | Notes |
| --- | --- | --- |
| `id` | uuid PK | |
| `tenant_id` | uuid NOT NULL | FK → `tenants(id)` ON DELETE CASCADE |
| `name` | text NOT NULL | |
| `kind` | text NOT NULL | `campaign` \| `transactional` |
| `subject` | text NOT NULL | subject line, may contain variable placeholders |
| `body_html` | text NOT NULL | HTML body |
| `body_text` | text NOT NULL | plain-text alternative; default `''` |
| `created_at` / `updated_at` | timestamptz NOT NULL | |

**Constraints**: `UNIQUE (tenant_id, name)`; `CHECK (kind IN
('campaign','transactional'))`.

**Validation**: `name` non-empty; `subject` non-empty; at least one of
`body_html` / `body_text` non-empty. A campaign may only be built from a
`campaign`-kind template; a transactional send may only use a `transactional`
template.

### Table: `campaigns`

| Column | Type | Notes |
| --- | --- | --- |
| `id` | uuid PK | |
| `tenant_id` | uuid NOT NULL | FK → `tenants(id)` ON DELETE CASCADE |
| `name` | text NOT NULL | |
| `subject` | text NOT NULL | |
| `body_html` | text NOT NULL | |
| `body_text` | text NOT NULL | default `''` |
| `from_name` | text NOT NULL | sender display name |
| `sending_domain_id` | uuid NULL | FK → `sending_domains(id)`; required to start |
| `from_local_part` | text NOT NULL | local part of the From address, e.g. `news` |
| `template_id` | uuid NULL | FK → `templates(id)` ON DELETE SET NULL — origin template |
| `status` | text NOT NULL | `draft` \| `running` \| `paused` \| `finished` \| `cancelled`; default `draft` |
| `max_send_errors` | integer NOT NULL | auto-pause threshold; default `100` |
| `sent_count` | integer NOT NULL | default `0` |
| `failed_count` | integer NOT NULL | default `0` |
| `recipient_count` | integer NOT NULL | total resolved recipients; default `0` |
| `created_at` / `updated_at` | timestamptz NOT NULL | |
| `started_at` / `finished_at` | timestamptz NULL | |

**Constraints**: `CHECK (status IN
('draft','running','paused','finished','cancelled'))`; index on `(tenant_id,
status)`.

**Validation** (`Campaign` constructor): `name`, `subject` non-empty; at least
one body non-empty; `from_local_part` is a valid address local part.

**State transitions** (`Campaign` methods):
- `draft → running` — `Start()`; requires a non-null `sending_domain_id` whose
  domain is `verified`, and at least one targeted list/segment. Sets
  `started_at`, `recipient_count`.
- `running → paused` — `Pause(reason)`; triggered automatically when
  `failed_count > max_send_errors`, or manually by an operator.
- `paused → running` — `Resume()`; re-arms batches for still-`pending` recipients.
- `running → finished` — `Finish()`; set when no `pending` recipients remain.
- `draft|running|paused → cancelled` — `Cancel()`.
- `RecordProgress(sent, failed)` adjusts the counters.
- Editing body/subject/domain is allowed only while `draft`.

### Table: `campaign_lists`

Join table linking a campaign to its targeted lists and segments.

| Column | Type | Notes |
| --- | --- | --- |
| `campaign_id` | uuid NOT NULL | FK → `campaigns(id)` ON DELETE CASCADE |
| `tenant_id` | uuid NOT NULL | FK → `tenants(id)` ON DELETE CASCADE |
| `list_id` | uuid NULL | FK → `lists(id)` ON DELETE CASCADE — set for a list target |
| `segment_query` | jsonb NULL | set for a segment target (reuses Phase 2 segment shape) |

**Constraints**: `CHECK ((list_id IS NOT NULL) <> (segment_query IS NOT NULL))`
— exactly one of list / segment per row; `UNIQUE (campaign_id, list_id)`.

### Table: `campaign_recipients`

One row per unique recipient of a campaign — the unit of send progress and the
dedup guarantee.

| Column | Type | Notes |
| --- | --- | --- |
| `id` | uuid PK | also the per-recipient tracking token (`s=` parameter) |
| `tenant_id` | uuid NOT NULL | FK → `tenants(id)` ON DELETE CASCADE |
| `campaign_id` | uuid NOT NULL | FK → `campaigns(id)` ON DELETE CASCADE |
| `subscriber_id` | uuid NOT NULL | FK → `subscribers(id)` ON DELETE CASCADE |
| `email` | text NOT NULL | snapshot of the recipient address at resolution time |
| `status` | text NOT NULL | `pending` \| `sent` \| `failed`; default `pending` |
| `failure_reason` | text NOT NULL | default `''` |
| `sent_at` | timestamptz NULL | |

**Constraints**: `UNIQUE (campaign_id, email)` — the database-level "each
recipient at most once" guarantee (FR-025); `CHECK (status IN
('pending','sent','failed'))`; index on `(campaign_id, status)` for batch
selection.

**Resumability**: `campaign.batch` selects `WHERE campaign_id = $1 AND status =
'pending'`; a row already `sent` is never re-selected, so worker redelivery is
idempotent.

---

## Migration 000010 — `links`, `link_clicks`, `campaign_views`

### Table: `links`

One row per distinct tracked URL in a campaign body (deduped per campaign).

| Column | Type | Notes |
| --- | --- | --- |
| `id` | uuid PK | the `{uuid}` in `/l/{uuid}` |
| `tenant_id` | uuid NOT NULL | FK → `tenants(id)` ON DELETE CASCADE |
| `campaign_id` | uuid NOT NULL | FK → `campaigns(id)` ON DELETE CASCADE |
| `url` | text NOT NULL | the original destination |
| `created_at` | timestamptz NOT NULL | |

**Constraints**: `UNIQUE (campaign_id, url)`.

### Table: `link_clicks`

| Column | Type | Notes |
| --- | --- | --- |
| `id` | uuid PK | |
| `tenant_id` | uuid NOT NULL | FK → `tenants(id)` ON DELETE CASCADE |
| `link_id` | uuid NOT NULL | FK → `links(id)` ON DELETE CASCADE |
| `campaign_id` | uuid NOT NULL | FK → `campaigns(id)` ON DELETE CASCADE |
| `recipient_id` | uuid NOT NULL | FK → `campaign_recipients(id)` ON DELETE CASCADE |
| `clicked_at` | timestamptz NOT NULL | `now()` |

**Indexes**: `(campaign_id)`, `(recipient_id)` — for Phase 4 analytics rollups.

### Table: `campaign_views`

Records message opens (the tracking pixel).

| Column | Type | Notes |
| --- | --- | --- |
| `id` | uuid PK | |
| `tenant_id` | uuid NOT NULL | FK → `tenants(id)` ON DELETE CASCADE |
| `campaign_id` | uuid NOT NULL | FK → `campaigns(id)` ON DELETE CASCADE |
| `recipient_id` | uuid NOT NULL | FK → `campaign_recipients(id)` ON DELETE CASCADE |
| `viewed_at` | timestamptz NOT NULL | `now()` |

**Indexes**: `(campaign_id)`, `(recipient_id)`.

> Open and click events are recorded but **not aggregated** in this phase.
> Building per-campaign analytics on top of these rows is Phase 4 work.

---

## Non-persisted entities

- **River job rows** — `campaign.start`, `campaign.batch`, `domain.verify` —
  live in River's own queue tables (installed by River's migrator), not the
  application schema. Payloads carry only `tenant_id` + aggregate IDs.
- **Rate-limit counters** — Redis sorted sets keyed `rl:tenant:{id}` and
  `rl:global`; ephemeral, TTL of one window, never durable state.
- **Rendered message** — the final MIME message with rewritten links and the
  open pixel is built in memory per recipient and handed to the messenger; it
  is not stored.

## Entity → spec mapping

| Spec Key Entity | Persistence |
| --- | --- |
| Sending Domain | `sending_domains` |
| Template | `templates` |
| Campaign | `campaigns` + `campaign_lists` |
| Send Job / Batch | River queue rows (`campaign.start`, `campaign.batch`) |
| Tracking Link | `links` (+ `link_clicks` for events) |
| Tracking Pixel | `campaign_views` (per-message pixel keyed by campaign + recipient) |
| API Key | `api_keys` — existing Phase 2 table, consumed here, unchanged |
| Usage Event | recorded via the existing usage path; counted per send |
| (recipient progress) | `campaign_recipients` |
