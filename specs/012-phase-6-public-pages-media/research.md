# Phase 0 Research: Public Pages & Media

All decisions below are resolved. No `NEEDS CLARIFICATION` markers remain.

## D1. Public page rendering — server-rendered Go templates

**Decision**: Public pages (subscription form, confirmation result, preference
page, unsubscribe result, archive index, archived campaign page, error pages)
are server-rendered with the standard-library `html/template`, served by
`cmd/api` from an `embed.FS` template set. The React SPA in `frontend/` is
**not** extended with public routes.

**Rationale**: Public pages must work with no client-side JavaScript, be
crawlable, render fast for anonymous visitors, and host per-tenant custom CSS
without leaking it into the admin app. `html/template` auto-escapes by default,
which directly mitigates the injection edge cases in the spec. RSS is XML and
must be server-generated regardless, so a server surface already exists.
Keeping the SPA admin-only avoids shipping JS bundles and auth machinery to
unauthenticated visitors. Confirmed with the user.

**Alternatives considered**: Extending the TanStack Router SPA with public
routes (rejected — ships JS to anonymous users, complicates tenant-CSS
isolation, still needs a server RSS endpoint); a hybrid Go + React-islands
approach (rejected — two rendering stacks for one small surface, YAGNI).

## D2. Media serving — unguessable tenant-prefixed capability URLs

**Decision**: Media bytes are stored in an S3-compatible bucket under keys of
the form `media/{tenantID}/{assetID}/{filename}`, where `assetID` is a random
UUID. Objects are world-readable by key; the bucket itself is **not** publicly
listable. Browsers and email clients fetch media directly from object storage
via these stable, unguessable URLs. The media-library *listing* used by the
admin UI is served from the RLS-protected `media_assets` metadata table, never
by enumerating the bucket. Confirmed with the user.

**Rationale**: Images embedded in campaign emails must be fetchable by email
clients that carry no session or credentials — an authenticated URL cannot
render in an inbox. Archived campaign pages reuse the same long-lived URLs. A
random per-asset UUID in the key path (122 bits of entropy) makes keys
non-enumerable, and a non-listable bucket means the only way to reach another
tenant's object is to already possess its full key. FR-028's "regardless of how
the storage path is requested" is satisfied: the path cannot be guessed, and
the listing API is RLS-scoped. The tenant prefix also satisfies FR-024.

**Alternatives considered**: App-proxied authorised serving (rejected — cannot
serve anonymous email-client fetches at all); presigned expiring URLs (rejected
— archived campaigns and already-delivered emails break when signatures
expire). Recorded as a deliberate, justified deviation in the plan's Complexity
Tracking.

## D3. S3 client — `aws-sdk-go-v2/service/s3`

**Decision**: Implement the `BlobStore` adapter with
`github.com/aws/aws-sdk-go-v2/service/s3`. `aws-sdk-go-v2` is already a direct
dependency (used by the Postbox SES-compatible client), so only the `s3`
service module and a static-credentials/endpoint-resolver setup are added.

**Rationale**: Reuses an in-tree dependency and the team's existing AWS-SDK
familiarity; Yandex Object Storage is S3-API-compatible. The domain depends
only on the `BlobStore` interface, so the concrete SDK never leaks past the
adapter and can be swapped.

**Alternatives considered**: `minio-go` (rejected — a second object-storage SDK
when `aws-sdk-go-v2` is already present); hand-rolled SigV4 (rejected —
needless re-implementation).

## D4. RSS generation — stdlib `encoding/xml`

**Decision**: Generate the per-tenant RSS 2.0 feed by marshalling a small set
of Go structs with `encoding/xml`, in `internal/api/rss_handler.go`. No feed
library is added.

**Rationale**: An RSS 2.0 document is a fixed, small shape; `encoding/xml`
covers it with no new dependency, consistent with the constitution's bias
against speculative dependencies. The feed is a live query over archive-visible
campaigns, so it always reflects current state (SC-005, SC-006).

**Alternatives considered**: `gorilla/feeds` (rejected — a dependency for a
~30-line struct); Atom instead of RSS (RSS chosen as the spec names it; the
struct shape is trivially extendable to Atom later if needed).

## D5. Confirmation & preference links — stored hashed tokens

**Decision**: Reuse `internal/token` (`New()` → 32-byte URL-safe value;
`Hash()` → SHA-256 hex). A confirmation token is stored as a hash on the
`pending_subscriptions` row with an `expires_at`; it is single-use (the row is
deleted on confirmation). A preference token is stored as a hash in a new
`preference_token_hash` column on `subscribers`; it is long-lived (so campaign
footers can carry a stable link) and rotates only on explicit regeneration or
subscriber deletion. The one-click `List-Unsubscribe` endpoint accepts the same
preference token.

**Rationale**: Matches the established session/invitation token pattern — the
raw token only ever lives in the link, the database holds only its hash, so a
DB leak yields no usable links (Principle IV). Stored tokens are revocable and
expirable, which a stateless HMAC is not. Confirmation single-use + expiry
satisfies FR-004/FR-006; preference token tamper-resistance satisfies
FR-010/FR-015.

**Alternatives considered**: Stateless HMAC-signed tokens (rejected — not
revocable, expiry only via embedded timestamp, and the project already has a
stored-token convention).

## D6. Confirmation email delivery — durable `optin.send` River job

**Decision**: Add an `OptinSendArgs{TenantID, PendingSubscriptionID}` River job
type to `internal/platform/jobs`. `SubmitPublicSubscription` creates the
`pending_subscription` row and enqueues the job in the same transaction; a new
`OptinWorker` (in `audience/adapters`) renders the confirmation email from a Go
template and sends it through the existing `campaign` `Messenger` (Postbox).
The confirmation email is platform-generated content, not a tenant-authored
template.

**Rationale**: Decouples the subscribe HTTP response from email delivery (fast
response, SC-001) and makes delivery durable and retry-capable so a restarted
worker never drops or double-sends a confirmation (Principle V). Enqueue-in-the
same-transaction guarantees the job exists iff the pending row exists.

**Alternatives considered**: Synchronous send inside the HTTP handler (rejected
— blocks the response and loses the confirmation on a transient send failure);
a tenant-authored confirmation template (rejected as YAGNI for this phase — the
confirmation copy is fixed platform content; branding still applies).

## D7. Confirmation/preference email sending identity — subscription-page config

**Decision**: The `SubscriptionPage` config carries a `sending_domain_id` plus
a from-name and from-local-part, mirroring how campaigns choose their sending
identity. Confirmation and preference-related emails are sent from that
verified domain. If the configured domain is unverified at send time the job
fails with a typed error and retries.

**Rationale**: Opt-in email must come from a verified sending domain for
deliverability; the page is the natural place to bind that identity since one
tenant may run several pages. Reuses the existing `sending` context's domain
verification.

**Alternatives considered**: A single tenant-wide default sender (rejected —
less flexible and still needs a verified-domain reference; per-page config is a
small superset).

## D8. Public tenant resolution — slug-path middleware without a session

**Decision**: Add a `resolvePublicTenant` middleware (in
`internal/api/public_middleware.go`) that resolves `/t/{slug}/...` to a tenant
**without** requiring an authenticated user, then opens the tenant-scoped
transaction. Token-addressed routes (`/c/{token}`, `/p/{token}`, `/u/{token}`)
resolve the tenant from the token row instead. The existing `resolveTenant`
middleware (which cross-checks an authenticated user's membership) is left
unchanged for the admin API.

**Rationale**: Public pages have no user to authorise; the tenant is still
resolved so every query runs inside the correct RLS scope. A deleted or
inactive tenant/list yields a clean "not available" page (spec edge case)
rather than an error.

**Alternatives considered**: Reusing `resolveTenant` (rejected — it requires a
session and would 401 every anonymous visitor).

## D9. Custom CSS sanitisation — scoped + filtered on save

**Decision**: Tenant custom CSS is sanitised and stored by the `TenantBranding`
domain entity on save: the CSS is rejected if it contains `</style`, `@import`,
`expression(`, `javascript:`, or `url(` referencing non-https schemes, and at
render time it is emitted inside a single `<style>` block whose rules are all
prefixed/scoped to a `.nv-public` wrapper class so they cannot affect platform
chrome. The value is rendered with `template.CSS` only after passing this
check; otherwise the save is rejected with a typed validation error.

**Rationale**: Modern browsers do not execute script from CSS, so the residual
risks are markup break-out (`</style>`), remote `@import`, and data
exfiltration via `url()`; filtering those plus container-scoping satisfies
FR-022 ("cannot execute arbitrary code", "never affect another tenant's
pages"). Doing it on save (not render) means a stored value is always safe and
the check runs once.

**Alternatives considered**: A full CSS-parser/sanitiser dependency (rejected
as oversized for a scoped allowlist); rendering CSS untrusted at request time
(rejected — repeats the work and risks a render-path bypass).

## D10. Rate limiting public submissions — existing Redis sliding window

**Decision**: Reuse `internal/platform/ratelimit` to limit subscription-form
submissions, keyed by both the submitted email address and the source IP, with
conservative per-window caps. An over-limit submission returns the form page
with a neutral "please try again shortly" message and sends no email.

**Rationale**: FR-009 and the spec's flooding edge case require this; the
sliding-window limiter already exists and is used by transactional sending, so
no new mechanism is introduced.

**Alternatives considered**: A new per-page counter table (rejected —
duplicates existing infrastructure, against the constitution's "shared
infrastructure lives once").
