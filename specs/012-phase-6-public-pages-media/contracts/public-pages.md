# Contract: Public Pages & RSS (unauthenticated)

Server-rendered `html/template` pages and one XML feed, served by `cmd/api`
outside the `/t/{slug}/api` authenticated group. No session or cookie is
required. Tenant is resolved from the URL slug (`resolvePublicTenant`) or from
the token row. Every page renders inside the tenant's branding wrapper
(`.nv-public` + sanitised custom CSS). All responses are `text/html;
charset=utf-8` except RSS.

## Subscription

### `GET /t/{slug}/subscribe/{page-slug}`

Render the subscription form for an active `SubscriptionPage`: title, email
field, and the configured extra `fields`.

- `200` — form rendered.
- `404` — unknown slug/page-slug, or the page or its tenant is inactive →
  branded "not available" page.

### `POST /t/{slug}/subscribe/{page-slug}`

Form-encoded submission (`email` + configured fields).

- `200` — accepted: a `pending_subscription` is created (or its token
  refreshed), the `optin.send` job is enqueued, and a branded "check your
  email" page is shown. The **same** `200` page is shown whether or not the
  address was already a subscriber (FR-007 — never disclose subscription
  state).
- `200` (with inline errors) — invalid email or missing required field → the
  form is re-rendered with messages; no pending row, no email.
- `200` (neutral notice) — rate limit exceeded → "please try again shortly";
  no email sent (FR-009).
- `404` — page/tenant inactive.

### `GET /c/{token}`

Confirmation landing. Tenant resolved from the token row.

- `200` — valid, unexpired token: the pending subscription is promoted (
  subscriber upserted, target memberships set `confirmed`), the pending row is
  deleted, branded success page shown. Idempotent: a second visit shows a
  benign "already confirmed" page.
- `200` (expired notice) — token expired: branded page explaining expiry with
  a "resend confirmation" action.
- `200` (suppressed notice) — address is suppressed: neutral page; membership
  not confirmed (FR-008).
- `404` — unknown/garbage token.

### `POST /c/{token}/resend`

Issue a fresh confirmation token + expiry for an expired pending row and
re-enqueue `optin.send`.

- `200` — new confirmation email enqueued; branded "check your email" page.
- `404` — no pending row for the token.

## Preferences & unsubscribe

### `GET /p/{token}`

Preference page for the subscriber identified by the preference token.

- `200` — current profile fields and per-list membership state, plus an
  "unsubscribe from all" action. No login.
- `404` — invalid/tampered token, or the subscriber no longer exists → branded
  page, no subscriber data exposed (FR-015, edge case).

### `POST /p/{token}`

Form-encoded preference update: profile fields and per-list
subscribe/unsubscribe checkboxes.

- `200` — changes applied immediately (FR-013); page re-rendered with a saved
  confirmation.
- `200` (with inline errors) — invalid field value → re-rendered with messages.
- `404` — invalid token / missing subscriber.

### `POST /u/{token}` — one-click unsubscribe (RFC 8058)

Target of the `List-Unsubscribe` / `List-Unsubscribe-Post` header the sending
pipeline adds to campaign mail. Body is the fixed `List-Unsubscribe=One-Click`
form field; no page interaction needed.

- `200` — the subscriber is unsubscribed from the campaign's list(s) and added
  to the tenant suppression scope; minimal branded confirmation body (FR-014).
- `404` — invalid token.

### `GET /u/{token}` — single-click unsubscribe

Plain link target (e.g. an email footer "unsubscribe" link) for clients that
follow it as a GET.

- `200` — branded "you have been unsubscribed" page; same effect as the POST.
- `404` — invalid token.

## Archive & RSS

### `GET /t/{slug}/archive`

Public archive index: archive-visible campaigns newest-first (by `archived_at`)
with title, send date, and link. Branded.

- `200` — list rendered (may be empty).
- `404` — unknown/inactive tenant.

### `GET /t/{slug}/archive/{campaign-id}`

A single archived campaign rendered as a standalone branded page from its
stored body HTML.

- `200` — archive-visible campaign rendered.
- `404` — campaign not found, is a draft, or is not `archive_visible`
  (FR-020) — no distinction leaked between "missing" and "hidden".

### `GET /t/{slug}/feed.xml`

Per-tenant RSS 2.0 feed of archive-visible campaigns (title, link,
`pubDate`, summary). `Content-Type: application/rss+xml; charset=utf-8`.

- `200` — valid feed; valid-but-empty for a tenant with no archived campaigns
  (SC-006).
- `404` — unknown/inactive tenant.

## Cross-cutting

- Every page is tenant-scoped; no route can surface another tenant's lists,
  subscribers, campaigns, or media (FR-030, SC-004).
- Subscribe, confirm, preference-change, and unsubscribe actions are recorded
  for audit (FR-031).
- Errors render a branded `error.html`; the binary never returns a raw stack
  trace or framework error page to a public visitor.
