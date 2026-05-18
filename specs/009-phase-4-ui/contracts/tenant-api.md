# Contract: Tenant API — Phase 4 UI

The Phase 4 UI consumes five existing tenant-scoped endpoints. **No endpoint is
added, changed, or removed by this feature** — this file documents the surface
the frontend depends on. All routes sit under `/t/{slug}/api`, behind the
existing `requireUser` → `resolveTenant` → `authz` chain, and use the standard
error envelope. Source of truth:
`specs/008-phase-4-deliverability-analytics/contracts/http-api.md` and
`internal/api/server.go` (lines 169–178).

The frontend reaches every route through the typed client in
`frontend/src/lib/api.ts`, which prefixes the tenant path via `tp(slug, …)`.

## Suppression list

### `GET /t/{slug}/api/suppressions`

Permission: `sending:get`. Query params: `cursor`, `limit` (default 50),
optional `reason` filter (`hard_bounce` | `complaint` | `manual`), optional
`email` substring filter.

Response `200`:

```json
{
  "items": [
    { "email": "user@example.com", "reason": "hard_bounce", "suppressedAt": "RFC 3339", "note": "" }
  ],
  "nextCursor": "string | null"
}
```

UI use: the suppression-list page (US2). `reason` and `email` are bound to the
filter and search controls; `nextCursor` drives incremental loading.

### `POST /t/{slug}/api/suppressions`

Permission: `sending:manage`. Body: `{ "email": "user@example.com", "note": "" }`.

| Status | UI handling |
| --- | --- |
| `201 Created` | Address added (idempotent — returns the entry whether new or pre-existing). Invalidate the suppression list query. |
| `422 validation_failed` | Render an inline form error on the email field (FR-011). |

### `DELETE /t/{slug}/api/suppressions/{email}`

Permission: `sending:manage`. `{email}` is URL-encoded.

| Status | UI handling |
| --- | --- |
| `204 No Content` | Removed — invalidate the list query. |
| `404 suppression_not_found` | Treated as success-equivalent: invalidate and reconcile silently, no error toast (FR-014). |

The UI requires an explicit confirmation dialog before issuing this call
(FR-012).

## Bounce-action settings

### `GET /t/{slug}/api/bounce-settings`

Permission: `sending:get`. Returns the effective configuration; both toggles
default to `true` when no row exists.

```json
{ "suppressOnHardBounce": true, "suppressOnComplaint": true }
```

### `PUT /t/{slug}/api/bounce-settings`

Permission: `sending:manage`. Body is the same shape.

| Status | UI handling |
| --- | --- |
| `200 OK` | Returns the new settings — confirm save, update the cache (FR-020). |

UI use: the bounce-settings panel (US4).

## Campaign analytics

### `GET /t/{slug}/api/campaigns/{id}/analytics`

Permission: `campaigns:get`. Served from the pre-computed `campaign_analytics`
summary table.

```json
{
  "campaignId": "uuid",
  "counts": { "sent": 0, "delivered": 0, "opened": 0, "clicked": 0, "bounced": 0, "complained": 0 },
  "rates": { "openRate": 0.0, "clickRate": 0.0, "bounceRate": 0.0, "complaintRate": 0.0 },
  "refreshedAt": "RFC 3339 | null"
}
```

Rates are fractions; a zero denominator yields `0.0`. `refreshedAt` is `null`
and counts are zero before the first refresh.

| Status | UI handling |
| --- | --- |
| `200 OK` | Render counts + rates. `refreshedAt === null` → "awaiting data" state (FR-004). |
| `404 campaign-not-found` | Render the not-found state (FR-006). |

UI use: the per-campaign analytics route (US1) and dashboard drill-down.

## Workspace dashboard

### `GET /t/{slug}/api/dashboard`

Permission: `campaigns:get`. Workspace-level summary; all figures from
`campaign_analytics`, RLS-scoped to the tenant.

```json
{
  "totals": { "sent": 0, "delivered": 0, "opened": 0, "clicked": 0, "bounced": 0, "complained": 0 },
  "deliverability": { "bounceRate": 0.0, "complaintRate": 0.0 },
  "recentCampaigns": [
    { "campaignId": "uuid", "name": "string", "sent": 0, "openRate": 0.0, "bounceRate": 0.0, "complaintRate": 0.0 }
  ]
}
```

`recentCampaigns` is the most recently sent campaigns (default 10), send-time
descending.

| Status | UI handling |
| --- | --- |
| `200 OK` | Render totals + deliverability + recent campaigns. `totals.sent === 0` with no recent campaigns → empty state (FR-018). |

UI use: the workspace dashboard route (US3).

## Error slugs the UI maps

| Slug | HTTP status | UI treatment |
| --- | --- | --- |
| `suppression_not_found` | `404` | Silent reconcile on removal race (FR-014). |
| `campaign-not-found` | `404` | Not-found state on the analytics view. |
| `validation_failed` | `422` | Inline form error on the add-address field. |

`401` is routed to sign-in by the existing global handler; `403` is rendered in
place by the screen that issued the request — both reuse `src/lib/errors.ts`
and need no Phase 4-specific handling.

## Client method groups added to `lib/api.ts`

```text
api.suppressions.list(slug, { cursor?, limit?, reason?, email? })
api.suppressions.add(slug, { email, note? })
api.suppressions.remove(slug, email)
api.bounceSettings.get(slug)
api.bounceSettings.update(slug, { suppressOnHardBounce, suppressOnComplaint })
api.analytics.campaign(slug, campaignId)
api.analytics.dashboard(slug)
```

Every method takes `slug` as its first argument and routes through `tp(slug, …)`
so a call site cannot omit tenant scope.
