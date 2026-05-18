# Phase 1 Data Model: Phase 3 Sending Pipeline — Frontend UI

This feature introduces **no new persisted entities**. It consumes existing
backend views over the tenant API and adds typed mirrors of them in
`frontend/src/lib/api-types.ts`. The shapes below are the JSON contracts the
UI depends on; field casing matches the backend's `json:` tags.

## SendingDomain (`DomainView`)

Source: `internal/sending/app/query/domains.go`.

| Field | Type | Notes |
|-------|------|-------|
| `id` | string | Domain UUID |
| `domain` | string | The domain name |
| `status` | `"pending" \| "verified" \| "failed"` | Lifecycle |
| `dkim_records` | `DNSRecord[]` | Each `{ type, name, value }`, individually copyable |
| `spf_record` | string | SPF TXT value |
| `dmarc_record` | string | DMARC TXT value |
| `failure_reason` | string? | Present only when `status === "failed"` |
| `created_at` | string (RFC3339) | |
| `verified_at` | string? | Present once verified |
| `last_checked_at` | string? | Last periodic/manual check |

**DNSRecord**: `{ type: string; name: string; value: string }`.

State transitions (backend-driven; UI is read-only on status):
`pending → verified`, `pending → failed`, `failed → pending` (on re-check).

## Template (`TemplateView`)

Source: `internal/campaign/app/query/templates.go`.

| Field | Type | Notes |
|-------|------|-------|
| `id` | string | |
| `name` | string | |
| `kind` | `"campaign" \| "transactional"` | Editable only at create time |
| `subject` | string | |
| `body_html` | string | Plain HTML editing (no WYSIWYG, per Assumptions) |
| `body_text` | string | |
| `created_at` | string | |
| `updated_at` | string | |

Validation (mirrors backend command rules; enforced in the form before submit):
`name`, `subject` non-empty; `kind` is one of the two values; at least one of
`body_html` / `body_text` non-empty.

## Campaign (`CampaignView`)

Source: `internal/campaign/app/query/campaigns.go`.

| Field | Type | Notes |
|-------|------|-------|
| `id` | string | |
| `name` | string | |
| `subject` | string | |
| `body_html` | string | |
| `body_text` | string | |
| `from_name` | string | Display name of the sender |
| `from_local_part` | string | Local part; the domain comes from the sending domain |
| `sending_domain_id` | string? | Must reference a **verified** domain to start |
| `template_id` | string? | Optional source template |
| `status` | `"draft" \| "running" \| "paused" \| "finished" \| "cancelled"` | |
| `max_send_errors` | number | Auto-pause threshold |
| `sent_count` | number | Progress |
| `failed_count` | number | Progress |
| `recipient_count` | number | Total targeted; `remaining = recipient_count - sent_count - failed_count` |
| `created_at` / `updated_at` | string | |
| `started_at` / `finished_at` | string? | |

Derived UI values:
- **Remaining** = `recipient_count − sent_count − failed_count` (clamped ≥ 0).
- **Auto-paused** = `status === "paused" && failed_count >= max_send_errors`.

Lifecycle / allowed UI actions per status:

| Status | Editable | Allowed actions |
|--------|----------|-----------------|
| `draft` | yes | edit, start (only if a verified domain is selected) |
| `running` | no | pause, cancel |
| `paused` | no | resume, cancel |
| `finished` | no | — |
| `cancelled` | no | — |

## CampaignCreate / CampaignUpdate (request bodies)

Source: `internal/api/campaign_handlers.go`.

`name`, `template_id?`, `subject`, `body_html`, `body_text`, `from_name`,
`from_local_part`, `sending_domain_id?`, `list_ids: string[]`,
`segments: Node[]` (segment AST, same shape `SegmentBuilder` already emits),
`max_send_errors?` (create only).

## APIKey (existing, reused)

`APIKey` / `IssuedAPIKey` types already exist in `api-types.ts`. The
transactional area reuses them; the only change is that `transactional:send`
becomes a selectable permission once added to the `Permission` union and
`ALL_PERMISSIONS` (research Decision 3).

## SendProgress (derived, not persisted)

A view-model only: `{ sent, failed, remaining, total, status }` computed from
`CampaignView` for the running-campaign progress display (FR-018). No backend
entity.
