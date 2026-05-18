# Phase 1 Data Model: Phase 4 — Deliverability & Analytics — Frontend UI

This feature persists nothing. It introduces no database tables and no domain
entities. The "model" here is the set of **view shapes** the frontend consumes
from the existing Phase 4 endpoints, declared as TypeScript types in
`frontend/src/lib/api-types.ts`. All shapes mirror the 008 HTTP contract
(`specs/008-phase-4-deliverability-analytics/contracts/http-api.md`).

## SuppressionEntry

One suppressed address as returned by `GET /suppressions`.

| Field | Type | Notes |
| --- | --- | --- |
| `email` | `string` | The suppressed address. |
| `reason` | `"hard_bounce" \| "complaint" \| "manual"` | Why the address was suppressed. |
| `suppressedAt` | `string` | RFC 3339 timestamp. |
| `note` | `string` | Optional operator note; empty string when none. |

List response: `{ items: SuppressionEntry[]; nextCursor: string | null }`.

**Display rules**:
- `reason` is rendered as a human label — "Hard bounce", "Complaint", "Manual".
- `suppressedAt` is rendered with the existing date formatter (`lib/format.ts`).
- An empty `items` array with no active filter → the empty state (FR-008).

## BounceSettings

The tenant's bounce-action configuration, from `GET /bounce-settings`. The same
shape is the body of `PUT /bounce-settings`.

| Field | Type | Notes |
| --- | --- | --- |
| `suppressOnHardBounce` | `boolean` | Toggle — defaults to `true`. |
| `suppressOnComplaint` | `boolean` | Toggle — defaults to `true`. |

The endpoint returns the defaults (both `true`) when the tenant has never saved
a configuration, so the UI never has to special-case an absent row.

## CampaignAnalytics

Per-campaign roll-up, from `GET /campaigns/{id}/analytics`.

| Field | Type | Notes |
| --- | --- | --- |
| `campaignId` | `string` | UUID. |
| `counts.sent` | `number` | Messages dispatched. |
| `counts.delivered` | `number` | From provider `Delivery` events. |
| `counts.opened` | `number` | From provider `Open` events. |
| `counts.clicked` | `number` | From provider `Click` events. |
| `counts.bounced` | `number` | Hard bounces. |
| `counts.complained` | `number` | Spam complaints. |
| `rates.openRate` | `number` | Fraction 0.0–1.0 of `delivered`. |
| `rates.clickRate` | `number` | Fraction 0.0–1.0 of `delivered`. |
| `rates.bounceRate` | `number` | Fraction 0.0–1.0 of `sent`. |
| `rates.complaintRate` | `number` | Fraction 0.0–1.0 of `sent`. |
| `refreshedAt` | `string \| null` | RFC 3339; `null` before the first refresh. |

**Display rules**:
- `refreshedAt === null` → "awaiting data" state (FR-004): show `counts.sent`,
  render the rest as zero, and state the figures have not been computed yet.
- Rates are fractions; the `RateValue` component renders them as percentages.
  A zero denominator already arrives as `0.0` → rendered `0%` (FR-005).
- A `404` (`campaign-not-found`) → the not-found state (FR-006).

## DashboardView

Workspace deliverability summary, from `GET /dashboard`.

| Field | Type | Notes |
| --- | --- | --- |
| `totals.sent` | `number` | Aggregate across the tenant. |
| `totals.delivered` | `number` | |
| `totals.opened` | `number` | |
| `totals.clicked` | `number` | |
| `totals.bounced` | `number` | |
| `totals.complained` | `number` | |
| `deliverability.bounceRate` | `number` | Fraction 0.0–1.0. |
| `deliverability.complaintRate` | `number` | Fraction 0.0–1.0. |
| `recentCampaigns` | `RecentCampaign[]` | Most recently sent first, default 10. |

### RecentCampaign

| Field | Type | Notes |
| --- | --- | --- |
| `campaignId` | `string` | UUID — used to link to the analytics view. |
| `name` | `string` | Campaign name. |
| `sent` | `number` | Sent count. |
| `openRate` | `number` | Fraction 0.0–1.0. |
| `bounceRate` | `number` | Fraction 0.0–1.0. |
| `complaintRate` | `number` | Fraction 0.0–1.0. |

**Display rules**:
- `totals.sent === 0` and `recentCampaigns` empty → the empty state (FR-018).
- Each `recentCampaigns` row links to
  `/t/{slug}/campaigns/{campaignId}/analytics` (FR-017).

## Relationships

- `CampaignAnalytics` and `RecentCampaign` both reference a campaign by id; the
  campaign itself is the existing Phase 3 `CampaignView`. Analytics is reached
  from the campaign detail page and from dashboard rows.
- `SuppressionEntry.reason` and `BounceSettings` are coupled by behaviour: the
  toggles govern which future bounces/complaints produce `hard_bounce` /
  `complaint` entries, but there is no foreign-key relationship in the UI.

## State

The UI holds no persistent client state. Transient UI state only:
- Suppression list: the active `reason` filter, the `email` search term, and
  the accumulated cursor pages — all reset on a filter/search change.
- Bounce settings: an in-form dirty copy of the two toggles until saved.
- Confirmation: a transient "confirm removal" dialog state for FR-012.
