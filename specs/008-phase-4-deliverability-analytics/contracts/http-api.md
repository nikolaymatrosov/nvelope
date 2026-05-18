# Contract: HTTP API — Deliverability & Analytics

New routes added in Phase 4. All are authenticated tenant routes under
`/t/{slug}/api`, behind the existing `requireUser` → `resolveTenant` → `authz`
chain (see `internal/api/server.go`). **There is no public webhook route** —
Postbox delivers feedback by writing to a Yandex Data Streams topic, which the
`cmd/consumer` service reads; the platform exposes no HTTP endpoint to receive
notifications. Error responses use the existing envelope; new error slugs are
mapped in `api/errmap.go`.

## Suppression list

### `GET /t/{slug}/api/suppressions`

List the tenant's suppression entries. Query params: `cursor`, `limit`
(default 50), optional `reason` filter, optional `email` substring filter.

**Response `200`**

```json
{
  "items": [
    {
      "email": "user@example.com",
      "reason": "hard_bounce | complaint | manual",
      "suppressedAt": "RFC 3339",
      "note": "string"
    }
  ],
  "nextCursor": "string | null"
}
```

### `POST /t/{slug}/api/suppressions`

Manually add an address. Body: `{ "email": "user@example.com", "note": "" }`.

| Status | When |
| --- | --- |
| `201 Created` | Address added (or already present — idempotent, returns the entry). |
| `422` | Invalid email (`validation_failed`). |

Writes an `audit_log` entry (`suppression.added`).

### `DELETE /t/{slug}/api/suppressions/{email}`

Remove an address; it becomes mailable again. `{email}` is URL-encoded.

| Status | When |
| --- | --- |
| `204 No Content` | Removed. |
| `404` | No such entry (`suppression_not_found`). |

Writes an `audit_log` entry (`suppression.removed`).

## Bounce-action settings

### `GET /t/{slug}/api/bounce-settings`

Returns the tenant's effective configuration; if no row exists, returns the
defaults (both toggles on).

```json
{
  "suppressOnHardBounce": true,
  "suppressOnComplaint": true
}
```

### `PUT /t/{slug}/api/bounce-settings`

Body is the same shape.

| Status | When |
| --- | --- |
| `200 OK` | Updated; returns the new settings. |

Writes an `audit_log` entry (`bounce_settings.updated`). There is no
soft-bounce threshold — soft bounces are out of scope this phase.

## Analytics

### `GET /t/{slug}/api/campaigns/{id}/analytics`

Per-campaign analytics, served from the `campaign_analytics` summary table.

**Response `200`**

```json
{
  "campaignId": "uuid",
  "counts": {
    "sent": 0, "delivered": 0, "opened": 0,
    "clicked": 0, "bounced": 0, "complained": 0
  },
  "rates": {
    "openRate": 0.0, "clickRate": 0.0,
    "bounceRate": 0.0, "complaintRate": 0.0
  },
  "refreshedAt": "RFC 3339 | null"
}
```

Rates are fractions of `delivered` (open/click) or `sent` (bounce/complaint),
computed on read; a zero denominator yields `0.0`. `refreshedAt` is `null` and
counts are zero before the first refresh.

| Status | When |
| --- | --- |
| `200 OK` | Campaign exists in the tenant. |
| `404` | No such campaign (`campaign-not-found`). |

### `GET /t/{slug}/api/dashboard`

Workspace-level deliverability summary.

**Response `200`**

```json
{
  "totals": {
    "sent": 0, "delivered": 0, "opened": 0,
    "clicked": 0, "bounced": 0, "complained": 0
  },
  "deliverability": {
    "bounceRate": 0.0, "complaintRate": 0.0
  },
  "recentCampaigns": [
    {
      "campaignId": "uuid",
      "name": "string",
      "sent": 0, "openRate": 0.0,
      "bounceRate": 0.0, "complaintRate": 0.0
    }
  ]
}
```

`recentCampaigns` lists the most recently sent campaigns (default 10), ordered by
send time descending. All figures come from `campaign_analytics` and are scoped
to the tenant by RLS.

## New error slugs (mapped in `api/errmap.go`)

| Slug | HTTP status |
| --- | --- |
| `suppression_not_found` | `404` |
| `recipient_suppressed` | `409` |
| `campaign-not-found` | `404` (reused from the campaign context) |
| `validation_failed` | `422` |
