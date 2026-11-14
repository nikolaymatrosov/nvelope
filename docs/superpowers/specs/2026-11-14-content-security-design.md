# Email content security hardening — Design

Date: 2026-11-14
Status: Implemented

## Goal

Close the content-injection holes found in the 2026-11-14 audit of the email
content pipeline: tenant-authored raw HTML, subscriber- and API-supplied merge
values, and the public pages that re-serve campaign content.

This is **track A of two**. Track B (private media library, publish-on-send
copies, image optimization) gets its own spec and builds on this one.

## Findings this design closes

| #  | Severity | Finding | Evidence |
|----|----------|---------|----------|
| F1 | Critical | Raw-HTML campaigns and templates are stored unsanitized. The public archive serves them as `template.HTML` on the app origin with no CSP. Result: stored XSS against any logged-in user who opens an archive link, including other tenants' admins — the session cookie is `Path=/`, so a same-origin `fetch` carries it. | Only `save_visual_campaign.go:109` and `save_visual_template.go:85` call `Sanitize`; raw paths `campaigns.go:61-85,154`, `templates.go:43,81` do not. `archive_handlers.go:78`. No CSP anywhere in `internal/api`. |
| F2 | High | `{{ subscriber.* }}` values are substituted unescaped into the HTML part, *after* sanitization. Subscribing a victim's address to a single opt-in list with the name `<a href=…>Verify your account</a>` delivers phishing markup from the tenant's DKIM-signed domain. | `sending/domain/substitution.go:47-61`, `batch_worker.go:219` |
| F3 | High | Transactional `{{key}}` values are substituted unescaped; tenants typically pass their end users' data. `ApplyVariables` also replaces key by key over a map, so a value containing `{{other}}` is re-expanded in random order. | `campaign/domain/tracking.go:14-16`, `transactional.go:119-121` |
| F4 | Medium | Tenant custom CSS is deny-list only and may `url(https://any-host)`. Public pages have no CSP, so tenant CSS can beacon from (or exfiltrate via attribute selectors on) the subscribe and preference forms. | `tenant/domain/branding.go:105-123`, `layout.html:29` |
| F5 | Low | Click/open tracking accepts any `?s=` recipient that exists in the tenant (cross-campaign attribution). A missing, foreign-tenant, or malformed `s` makes the insert fail, and the click handler returns an error instead of redirecting — the link in the email breaks. | `tracking_pg.go:56-86` (`recipient_id` FK to `campaign_recipients`), `tracking_handlers.go:42-56` |
| F6 | Low | Campaign and template bodies have no size cap; only uploads are bounded. | `openapi_wiring.go:88` |

Already mitigated, unchanged here: admin previews (`srcdoc` + `sandbox=""`),
header injection (Q-encoded Subject/From name), RSS (`encoding/xml`), no
server-side fetch of content URLs.

Deferred to track B (media): SVG/PDF served without `Content-Disposition` and
with a client-declared Content-Type; the `mediaRef` prefix check is
platform-wide, not per tenant (`service/application.go:445`); EXIF/GPS kept
in uploaded photos; decompression bombs once images are decoded.

## Decisions

- **Sanitize at save on every write path; contain at display with CSP.** One
  origin.
  - Rejected: a separate origin for public pages. It needs a second
    domain, certificate, and routing in both deployments for little gain over
    CSP `sandbox`. Kept as the upgrade path.
  - Rejected: sanitize on output only. The editor would show something other
    than what gets sent.
- **Raw HTML uses a document-preserving email policy by default.** A tenant
  may switch sanitization off after explicitly accepting the risk.
- **Merge values are HTML-escaped in the HTML part.** Transactional templates
  get `{{{key}}}` for intentional raw HTML.
- **No production data exists**: no backfill, no feature flag, no
  compatibility window for the transactional `{{key}}` semantics change.

## Design

### 1. Raw HTML sanitization

**Setting.** `TenantSettings` gains `rawHTMLSanitization`: `standard`
(default) or `off`.

- Migration: `ALTER TABLE tenant_settings ADD COLUMN raw_html_sanitization
  text NOT NULL DEFAULT 'standard' CHECK (raw_html_sanitization IN
  ('standard', 'off'))`.
- Domain: `SetRawHTMLSanitization(level, acceptRisk bool) error`. Switching to
  `off` with `acceptRisk == false` returns an incorrect-input error, code
  `risk_acceptance_required`. Switching to `standard` needs no flag.
- HTTP: a dedicated `PUT /t/{slug}/api/settings/raw-html-sanitization` with
  body `{"level", "accept_risk"}`, registered in the `authz` group and gated
  on `settings:manage`. It is separate from `PUT /settings` because that
  endpoint replaces every field and checks no permission.
  `GET /settings` returns the level. On change the handler calls
  `recordAudit(ctx, "settings.raw_html_sanitization", tenantID,
  {"from": old, "to": new})`.
- UI: a toggle on the tenant settings page. Turning it off opens a
  confirmation dialog with an explicit checkbox. The dialog lists what `off`
  lets through to recipients' inboxes: scripts, forms, iframes, event
  handlers. Mail clients' own filtering becomes the only guard, and some
  providers penalise such mail.

**Policy.** `visualrender.SanitizeDocument(html string) (string,
[]domain.RenderWarning)`, a second bluemonday policy next to the existing
`emailPolicy`:

- Keeps:
  - Document skeleton: `html`, `head`, `body`, `title`.
  - `style` elements, with content passed through unparsed.
  - Comments, so MSO conditionals survive.
  - UGC elements, plus `center` and `font` (`color`, `face`, `size`).
  - Tables and legacy presentational attributes: `bgcolor`, `align`,
    `valign`, `width`, `height`, `border`, `cellpadding`, `cellspacing`.
    `background` only with an `http(s)` value.
  - `style`, `class`, and `id` on every element. `style` has no
    per-property filter.
  - `meta` with only `charset`; `name` in {viewport, format-detection,
    x-apple-disable-message-reformatting, color-scheme,
    supported-color-schemes} plus `content`; or `http-equiv` in
    {content-type, x-ua-compatible} plus `content`.
- Removes, element and content: `script`, `iframe`, `frame`, `frameset`,
  `object`, `embed`, `applet`, `form`, `input`, `button`, `select`,
  `textarea`, `base`, `link`, `svg`, `math`.
- Removes every `on*` attribute. A `meta http-equiv="refresh"` loses its
  `http-equiv`.
- URL schemes: `http`, `https`, `mailto`, `tel`; `data:image/*` in `src`
  only.
- Why `<style>` content is safe to pass through: current engines execute no
  script from CSS, and bluemonday's tokenizer applies the browser's raw-text
  rules, so the first `</style>` always ends the element. Keeping `style`
  elements needs bluemonday's `AllowUnsafe(true)`; `script` stays off the
  allow-list. Tests pin both facts.
- bluemonday drops `<!DOCTYPE>`. `SanitizeDocument` re-prepends the input's
  leading `<!DOCTYPE html …>` (matched with `[^<>]*`, so it cannot carry
  markup); without it the archive renders in quirks mode.
- Warnings: the same `sanitizer_stripped` signal the visual path emits
  (regex detection of a visible disallowed construct), with the detection
  regex adjusted for this policy. `style` and `data:image` are no longer
  flagged.

**Call sites.**

- `CreateCampaignHandler`, `UpdateCampaignHandler`, `CreateTemplateHandler`,
  and `UpdateTemplateHandler` sanitize `BodyHTML` before the domain
  constructor or `Recompose`.
- They read the level through a consumer-owned port in
  `campaign/app/command`:

  ```go
  type RawHTMLPolicy interface {
      SanitizesRawHTML(ctx context.Context, tenantID string) (bool, error)
  }
  ```

  A bridge in `internal/service/bridges.go` implements it over tenant
  settings.
- `level == off` stores verbatim. `BodyText` is never sanitized.
- A lookup failure fails the save. It fails closed: never store unsanitized
  HTML because the setting could not be read.
- Command results gain `Warnings`. The hand-written HTTP handlers add a
  `warnings` array to the create/update campaign and template responses (these
  operations are not in the OpenAPI contract yet).
- The visual save path is unchanged: always the strict `Sanitize`,
  whatever the level.
- The level applies at save time. Switching `off` → `standard` affects
  later saves; stored bodies are not rewritten.

**Size cap.** Campaign and template constructors and `Recompose` reject a
`bodyHTML` or `bodyText` over 1 MiB with code `body-too-large` (the campaign
context's hyphenated code style). The same check
runs in `Campaign.ApplyVisualSave` and `Template.ApplyVisualSave`.

### 2. Public surface: headers and archive containment

A new middleware, `publicSecurityHeaders`, wraps every anonymous HTML route:
the `mountPublicRoutes` group (`/t/{slug}/subscribe`, `/confirm`, `/archive`,
`/feed.xml`) and the token routes (`/p`, `/u`). The tracking pixel `/o` and the
click redirect `/l` render no HTML and are left alone.

Default headers, for every public response:

```
Content-Security-Policy: default-src 'none'; style-src 'unsafe-inline';
  img-src 'self' https: data:; font-src 'self' https: data:;
  form-action 'self'; base-uri 'none'; frame-ancestors 'none'
X-Content-Type-Options: nosniff
Referrer-Policy: no-referrer
```

- There is no `script-src`, so it falls back to `'none'`. Public templates
  ship no JS: `internal/api/templates` contains no `<script>`.
- The policy is static. Images and fonts may come from any `https:` host:
  the tenant logo is an arbitrary https URL, and narrowing `img-src` per tenant
  would buy little (see §5).
- `Referrer-Policy: no-referrer` keeps `/p/{token}` and `/u/{token}` out of
  `Referer` headers sent to the logo host and to outbound links.

The archive campaign page (`/t/{slug}/archive/{campaignId}`) replaces the
CSP:

```
Content-Security-Policy: sandbox allow-popups allow-popups-to-escape-sandbox;
  default-src 'none'; style-src 'unsafe-inline'; img-src https: data:;
  font-src https: data:; form-action 'none'; base-uri 'none';
  frame-ancestors 'none'
```

- `sandbox` gives the document an opaque origin. Content that slips past the
  sanitizer, or was saved under `off`, cannot run script or reach the API
  with the visitor's session.
- `img-src https:` is needed because campaign images may live on tenant
  CDNs.
- Links still navigate top-level, and `target=_blank` works through
  `allow-popups-to-escape-sandbox`.

Out of scope: a CSP for the admin SPA. Nitro serves it, and it needs nonce
plumbing for the TanStack Start hydration scripts. That is a separate task.

### 3. Merge values

**Campaign sends.** `sending.Substitute(html, text, …)` keeps its signature;
the HTML part now gets `html.EscapeString(value)` and the text part stays raw.
A new `SubstituteSubject(subject, …)` substitutes raw values; the messenger
Q-encodes the subject later.

- Today `batch_worker.go:220` feeds the subject through the `html` slot. With
  escaping that would turn `&` into `&amp;` in subjects, so that call switches
  to `SubstituteSubject`.
- Escaping applies to every namespace, `campaign.unsubscribe_url` included:
  `&` → `&amp;` is correct inside an attribute.

**Transactional sends.** `ApplyVariables` is replaced by a single-pass
replacer over `\{\{\{(key)\}\}\}|\{\{(key)\}\}`, keeping today's exact,
whitespace-free syntax:

- `{{key}}` in HTML is escaped. In Subject and Text it is raw.
- `{{{key}}}` in HTML is inserted raw. When at least one triple-brace
  placeholder was substituted, the whole HTML then goes through
  `SanitizeDocument`, whatever the tenant's level: the level governs HTML the
  tenant authored, while variables carry runtime data from the API caller.
  In Subject and Text `{{{key}}}` behaves like `{{key}}`.
- Single pass: inserted values are never rescanned, which removes the
  map-order re-expansion.
- Unknown keys stay literal, as today.
- The transactional template editor's variable hint documents `{{{key}}}`
  (add the hint if there is none).

### 4. Click and open attribution

`RecordClick` and `RecordView` insert only when the recipient belongs to the
campaign:

```sql
INSERT INTO link_clicks (tenant_id, link_id, campaign_id, recipient_id)
SELECT @tenant_id, @link_id, @campaign_id, r.id
FROM campaign_recipients r
WHERE r.id = @recipient_id AND r.campaign_id = @campaign_id
```

`campaign_views` uses the same pattern.

- A missing or non-UUID `s` is checked with `uuid.Parse` before the query,
  and the insert is skipped.
- The click handler always redirects when the link exists. Attribution
  failures are logged, never surfaced.
- An unknown link stays a 404.

### 5. Tenant CSS and logo

- The CSS deny-list stays. CSP (§2) blocks script but lets `url()` reach any
  `https:` host, so CSS beacons and attribute-selector reads stay possible;
  see Accepted risks.
- `SetLogoURL` requires `https://`; today it accepts `http(s)://`. An `http`
  logo would be mixed content and would widen the CSP.

### Accepted risks (no change)

- **Unsubscribe and preference tokens never expire and are not scoped to a
  campaign.**
  - Unsubscribe links must keep working long after the send: RFC 8058, and
    CAN-SPAM's 30-day minimum.
  - A forwarded email lets the person it was forwarded to manage that
    subscriber's preferences.
  - After §3, a renamed subscriber can no longer inject markup.
- **Tracking links are unsigned.** Link and recipient ids are UUIDv4 and
  appear only in that recipient's email; §4 removes cross-campaign
  attribution.
- **Tenant CSS may load `https:` resources on public pages.** The tenant can
  already point its logo at any host, so it learns about page visits either
  way. Reading form values through attribute selectors would only expose the
  tenant's own subscribers to a tenant member with `branding:manage`.
- **Level `off`.** The tenant's own recipients may receive active content.
  Platform surfaces stay contained by §2 and the admin preview sandbox.

## Error handling

| Case | Result |
|------|--------|
| `off` without `accept_risk` | 422 `risk_acceptance_required` |
| Body over 1 MiB | 422 `body-too-large` |
| Sanitizer finds disallowed content | Never an error: strips it and returns a `sanitizer_stripped` warning |
| Level lookup fails on save | Save fails (5xx); nothing is stored |
| Tracking attribution fails | Pixel or redirect still served; failure logged |

## Testing

- **`visualrender`**: table test for `SanitizeDocument`.
  - Keeps a `<style>` with a media query, an MSO conditional comment,
    `bgcolor`, a viewport `meta`, and `data:image/png` in `src`.
  - Strips `script`, `onerror`, a `javascript:` href, `form`, `iframe`,
    `meta refresh`, `base`, `svg`, and `data:text/html` in `href`.
  - Idempotent: sanitizing twice equals sanitizing once.
- **Campaign commands** (in-memory repos):
  - Raw create/update under `standard` strips and returns a warning.
  - Under `off` the body is stored verbatim.
  - The visual save stays strict under `off`.
  - A lookup error fails the save.
- **Tenant domain**: `off` without acceptance errors; `standard` needs no
  flag.
- **API**: the settings change writes an audit row.
- **`sending.Substitute`**: with a value containing `&` and `<`, HTML is
  escaped while Text and Subject are raw.
- **Transactional**:
  - `{{k}}` is escaped.
  - `{{{k}}}` is raw and then sanitized.
  - A value containing `{{other}}` is not expanded.
- **API e2e**:
  - The archive response carries the sandbox CSP.
  - Public pages carry the default CSP.
  - A `<script>` saved in a raw campaign under `standard` never reaches the
    archive HTML.
- **Tracking integration** (Postgres): a foreign recipient gives a 302 and
  no row; a missing `s` gives a 302 and no row; a malformed `s` gives a 302.

## Implementation order

1. §1 sanitization and §2 headers. Together they close F1; ship them first.
2. §3 merge values (F2, F3).
3. §4 attribution (F5), §5 CSS/logo (F4), and the size cap (F6).
