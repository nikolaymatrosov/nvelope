# Contract: Tenant API surface consumed by the Phase 3 UI

All paths are tenant-scoped under `/t/{slug}/api` and authenticated by the
workspace session cookie (`credentials: "include"`). Each operation is gated
server-side by a permission; the UI mirrors the gate advisorily.

## Sending domains (exists)

| Method & path | Permission | Request | Response |
|---|---|---|---|
| `POST /sending-domains` | `sending:manage` | `{ domain: string }` | `201` `DomainView` |
| `GET /sending-domains` | `sending:get` | — | `200` `{ domains: DomainView[] }` |
| `GET /sending-domains/{id}` | `sending:get` | — | `200` `DomainView` |
| `POST /sending-domains/{id}/recheck` | `sending:manage` | — | `202` `{ status: "pending" }` |

## Templates (mostly exists)

| Method & path | Permission | Request | Response |
|---|---|---|---|
| `POST /templates` | `campaigns:manage` | `{ name, kind, subject, body_html, body_text }` | `201` `TemplateView` |
| `GET /templates` | `campaigns:get` | `?limit&offset` | `200` `{ templates: TemplateView[], total }` |
| `GET /templates/{id}` | `campaigns:get` | — | `200` `TemplateView` |
| `PUT /templates/{id}` | `campaigns:manage` | `{ name, subject, body_html, body_text }` | `200` `TemplateView` |
| **`DELETE /templates/{id}`** *(NEW)* | `campaigns:manage` | — | `204` |

## Campaigns (mostly exists)

| Method & path | Permission | Request | Response |
|---|---|---|---|
| `POST /campaigns` | `campaigns:manage` | `CampaignCreate` | `201` `CampaignView` |
| `GET /campaigns` | `campaigns:get` | `?limit&offset` | `200` `{ campaigns: CampaignView[], total }` |
| `GET /campaigns/{id}` | `campaigns:get` | — | `200` `CampaignView` |
| `PUT /campaigns/{id}` | `campaigns:manage` | `CampaignUpdate` | `200` `CampaignView` |
| `POST /campaigns/{id}/start` | `campaigns:manage` | — | `202` `{ status: "running" }` |
| `POST /campaigns/{id}/pause` | `campaigns:manage` | — | `200` `{ status: "paused" }` |
| `POST /campaigns/{id}/resume` | `campaigns:manage` | — | `200` `{ status: "running" }` |
| **`POST /campaigns/{id}/cancel`** *(NEW)* | `campaigns:manage` | — | `200` `{ status: "cancelled" }` |

## API keys (exists, reused by the transactional area)

| Method & path | Permission | Request | Response |
|---|---|---|---|
| `POST /api-keys` | `apikeys:manage` | `{ name, permissions: Permission[] }` | `201` `{ id, token }` |
| `GET /api-keys` | `apikeys:get` | — | `200` `{ api_keys: APIKey[] }` |
| `DELETE /api-keys/{id}` | `apikeys:manage` | — | `204` |

## Transactional send endpoint (reference only — not called by the UI)

`POST /t/{slug}/api/tx`, authenticated by a scoped **API key** (not the
session). The UI only displays this contract for developer reference:

```json
{
  "template_id": "string",
  "to": "string",
  "sending_domain_id": "string",
  "from_name": "string",
  "from_local_part": "string",
  "variables": { "key": "value" }
}
```

Responses: `202 { "message_id": "..." }`; `429` with `Retry-After` when
rate-limited.

## Two NEW backend endpoints — implementation notes

### `DELETE /templates/{id}` → `DeleteTemplate` command

- New command in `internal/campaign/app/command/templates.go`:
  `DeleteTemplate{ TenantID, TemplateID }` with a handler that calls a repo
  delete. Wired in `internal/campaign/app` and `internal/api/server.go`.
- New handler `handleDeleteTemplate` in `campaign_handlers.go`, gated by
  `PermCampaignsManage`, returns `204`.
- Templates are copied into a campaign at create time (subject/body are
  snapshotted), so deleting a template does not affect existing campaigns.

### `POST /campaigns/{id}/cancel` → `CancelCampaign` command

- New command `CancelCampaign{ TenantID, CampaignID }` in
  `internal/campaign/app/command/campaigns.go`, mirroring `PauseCampaign`.
  The domain method `Campaign.Cancel()` already exists and validates the
  transition (rejects finished/cancelled campaigns).
- New handler `handleCancelCampaign` in `campaign_handlers.go`, gated by
  `PermCampaignsManage`, returns `200 { "status": "cancelled" }`.

Both new endpoints follow the existing typed-error → transport-status mapping
(`s.fail`), so no transport-level error branching is added (Principle VI).

## Error contract (all endpoints)

Non-2xx bodies are `{ error: { code, message } }`, normalized by
`frontend/src/lib/errors.ts` into `ApiError`. `401` → routed to sign-in;
`403` → in-place authorization message; `404` → in-place not-found;
`5xx`/network → global toast.
