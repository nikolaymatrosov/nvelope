# Contract: HTTP API

New routes added in `internal/api`. Existing routes are unchanged. Request and
response bodies are JSON unless noted. Errors use the existing typed-error
envelope (`{ "error": "<slug>", "message": "<human text>" }`) mapped to a status
code in `api/errmap.go`.

## Tenant-scoped routes — sending domains (US1)

Mounted under `/t/{slug}/api`, behind `requireUser` → `resolveTenant` → `authz`,
exactly like the Phase 2 audience routes. Permission checks reuse the existing
permission-string mechanism.

### `POST /t/{slug}/api/sending-domains`
Register a sending domain. Provisions a Postbox identity synchronously and
returns the DNS records to publish.

Request: `{ "domain": "mail.acme.com" }`

`201` response:
```json
{
  "id": "uuid",
  "domain": "mail.acme.com",
  "status": "pending",
  "dkim_records": [{ "type": "CNAME", "name": "...", "value": "..." }],
  "spf_record": "v=spf1 include:... ~all",
  "dmarc_record": "v=DMARC1; p=none; ...",
  "created_at": "..."
}
```
Errors: `domain-invalid` (422), `domain-already-exists` (409),
`provisioning-failed` (502).

### `GET /t/{slug}/api/sending-domains`
List the tenant's sending domains. `200` → `{ "domains": [ <domain view>... ] }`.

### `GET /t/{slug}/api/sending-domains/{id}`
Get one domain, including current status and DNS records. `404` →
`domain-not-found`.

### `POST /t/{slug}/api/sending-domains/{id}/recheck`
Trigger an immediate verification re-check (enqueues a `domain.verify` job).
`202` → `{ "status": "pending" }`. Errors: `domain-not-found` (404),
`domain-not-pending` (409) when the domain is already `verified` or `failed`.

## Tenant-scoped routes — templates & campaigns (US2)

### `POST /t/{slug}/api/templates`
Create a template. Request: `{ "name", "kind": "campaign"|"transactional",
"subject", "body_html", "body_text" }`. `201` → template view.
Errors: `template-name-taken` (409), `template-invalid` (422).

### `GET /t/{slug}/api/templates` — list. `PUT .../templates/{id}` — update.
### `GET /t/{slug}/api/templates/{id}` — get one.

### `POST /t/{slug}/api/campaigns`
Create a campaign, optionally from a template. Request:
`{ "name", "template_id"?, "subject"?, "body_html"?, "body_text"?, "from_name",
"from_local_part", "sending_domain_id"?, "list_ids": [...], "segments": [...] }`.
When `template_id` is given, omitted content fields inherit from the template.
`201` → campaign view. Errors: `template-not-found` (404),
`template-kind-mismatch` (422), `campaign-invalid` (422).

### `GET /t/{slug}/api/campaigns` — list with status + progress counts.
### `GET /t/{slug}/api/campaigns/{id}` — get one, including `sent_count`,
`failed_count`, `recipient_count`, `status`.
### `PUT /t/{slug}/api/campaigns/{id}` — update; allowed only while `draft`.
Errors: `campaign-not-editable` (409).

### `POST /t/{slug}/api/campaigns/{id}/start`
Start sending. Enqueues a `campaign.start` job. `202` → `{ "status": "running" }`.
Errors: `campaign-not-found` (404), `campaign-not-draft` (409),
`sending-domain-required` (422) when no verified domain is selected,
`campaign-no-recipients` (422) when no list/segment targets are set.

### `POST /t/{slug}/api/campaigns/{id}/pause`
Manually pause a running campaign. `200` → `{ "status": "paused" }`.

### `POST /t/{slug}/api/campaigns/{id}/resume`
Resume a paused campaign; re-enqueues batches for `pending` recipients.
`200` → `{ "status": "running" }`.

## API-key-authenticated route — transactional send (US3)

### `POST /t/{slug}/api/tx`
Send one transactional message immediately. Authenticated by an API key, **not**
a session — mounted on a sibling route group using the new
`apikey_middleware.go` (reads `Authorization: Bearer <key>`, resolves via the
Phase 2 `AuthenticateAPIKey` query, checks the key belongs to the resolved
tenant and carries the transactional-send scope).

Request:
```json
{
  "template_id": "uuid",
  "to": "person@example.com",
  "sending_domain_id": "uuid",
  "from_name": "Acme",
  "from_local_part": "noreply",
  "variables": { "name": "Sam", "reset_url": "https://..." }
}
```
`202` → `{ "message_id": "provider-ref" }`.

Errors: `unauthorized` (401) — missing/invalid key; `forbidden` (403) — key
lacks the transactional scope or belongs to another tenant;
`template-not-found` (404); `template-kind-mismatch` (422) — not a
`transactional` template; `sending-domain-not-verified` (422);
`rate-limited` (429) — includes a `Retry-After` header.

## Public, unauthenticated routes — tracking (US2)

Mounted at the router root (no tenant in the path; tenant is resolved from the
UUID's owning row, then `app.tenant_id` is set for the event write).

### `GET /o/{campaignId}?s={recipientId}`
Open-tracking pixel. Records a `campaign_views` row. Responds `200` with a 1×1
transparent GIF and `Cache-Control: no-store`. Unknown or malformed IDs still
return the pixel (never an error to the mail client) but record nothing.

### `GET /l/{linkId}?s={recipientId}`
Click-tracking link. Records a `link_clicks` row, then `302` redirects to the
link's original URL. An unknown `linkId` returns `404`.

## Route mounting summary (`internal/api/server.go`)

```text
/t/{slug}/api                         (requireUser → resolveTenant)
  ├── (authz group)
  │   ├── POST   /sending-domains
  │   ├── GET    /sending-domains
  │   ├── GET    /sending-domains/{id}
  │   ├── POST   /sending-domains/{id}/recheck
  │   ├── POST   /templates    GET /templates
  │   ├── GET    /templates/{id}    PUT /templates/{id}
  │   ├── POST   /campaigns    GET /campaigns
  │   ├── GET    /campaigns/{id}    PUT /campaigns/{id}
  │   ├── POST   /campaigns/{id}/start
  │   ├── POST   /campaigns/{id}/pause
  │   └── POST   /campaigns/{id}/resume
  └── (apikey group)
      └── POST   /tx
/o/{campaignId}                        (public)
/l/{linkId}                            (public)
```
