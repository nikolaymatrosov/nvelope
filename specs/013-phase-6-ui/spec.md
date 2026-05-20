# Feature Specification: Phase 6 — Public Pages & Media — Frontend UI

**Feature Branch**: `013-phase-6-ui`

**Created**: 2026-05-20

**Status**: Draft

**Input**: User description: "I want to add UI for the features from previous spec" — i.e. Phase 6 — Public Pages & Media (012): public subscription pages with double opt-in, per-subscriber preference and unsubscribe pages, public campaign archive and RSS feed with per-tenant branding/CSS, and a tenant media library backed by S3-compatible object storage.

## User Scenarios & Testing *(mandatory)*

<!--
  Phase 6 (Public Pages & Media) is already implemented in the backend: tenants
  expose public subscription pages bound to one or more lists; new submissions
  enter a pending state and receive a confirmation email; confirmation links
  activate the subscription; per-subscriber preference links allow self-serve
  list and profile updates and unsubscribe; sent campaigns can be flagged
  archive-visible and exposed via a public per-tenant archive index, individual
  archive pages, and an RSS feed; each tenant configures branding (logo, colors,
  custom CSS) applied to all of its public pages; an S3-backed, tenant-prefixed
  media library stores uploaded assets that can be referenced from campaign
  content. None of this is visible to either a visitor/subscriber or a tenant
  administrator yet — there is no web interface to host the public flows or to
  configure them from inside the workspace. This feature delivers both halves of
  that interface: the unauthenticated public pages on per-tenant URLs, and the
  authenticated workspace surfaces that configure and operate them.

  It extends the existing tenant workspace app shell (the persistent sidebar
  layout from the Phase 1 & 2 UI) for the authenticated side, and introduces a
  new public-pages surface — rendered on per-tenant URLs, with no workspace
  chrome, and themed by the tenant's branding and custom CSS — for the
  unauthenticated side. The "users" are two distinct audiences: end-user
  visitors and subscribers (anyone on the open internet who interacts with a
  tenant's public pages, with no account), and tenant administrators and
  campaign authors inside the workspace.

  Backend constraints reflected by this UI: every public page is bound to and
  scoped by exactly one tenant; one tenant's branding, CSS, lists, subscribers,
  campaigns, or media MUST NEVER appear on or affect another tenant's pages;
  the public subscription flow is double opt-in only (single opt-in is not
  available); confirmation and preference links carry unique, time-limited,
  tamper-resistant tokens; suppression and existing-subscriber rules are
  enforced server-side and the UI must not leak whether an address is already
  known; media is tenant-prefixed in storage and never served across tenants
  regardless of how the URL is requested; archive visibility is per-campaign
  and per-tenant.
-->

### User Story 1 - Subscribe through a tenant's public page (Priority: P1)

A visitor opens a tenant's public subscription page on a per-tenant URL. The page is themed with the tenant's logo, colors, and custom CSS and shows the tenant's configured profile fields plus the required email field. The visitor enters their address (and any required fields), submits, and lands on a "check your email" page that names the address the confirmation was sent to. They open the confirmation email, follow the link, and land on a confirmation success page that names the list they have joined. If the link has expired they instead land on a clear expired-link page that lets them request a fresh confirmation email. The page never reveals whether an address is already subscribed.

**Why this priority**: This is the entry point of the entire phase — without a public subscription page that visitors can actually use, the double opt-in flow, preference management, archive, and even the eventual exit-criterion "subscribers can self-serve" all have nothing to surface. It is independently demonstrable end to end: open the public URL, submit, confirm via the email link, land on success.

**Independent Test**: Open a tenant's public subscription URL in a private browser session, submit a new address, retrieve and click the confirmation link from the resulting email, and confirm the success page is shown and the address now appears as an active subscriber on the tenant's list. Repeat with an expired token to confirm the expired-link path. Repeat with an already-active address and confirm the neutral "check your email" message is shown without disclosing the existing subscription.

**Acceptance Scenarios**:

1. **Given** a tenant has a published public subscription page bound to a list, **When** a visitor opens its per-tenant URL, **Then** the page renders with the tenant's branding (logo, colors, custom CSS) and shows the configured visible profile fields plus the email field.
2. **Given** the subscription page, **When** the visitor submits a valid, previously-unknown email address (and fills any required fields), **Then** they land on a "check your email" page that names the submitted address and explains that the subscription is pending confirmation.
3. **Given** a fresh confirmation link from the confirmation email, **When** the visitor opens it, **Then** they land on a success page that names the list they have joined and offers a link to manage preferences.
4. **Given** an expired confirmation link, **When** the visitor opens it, **Then** they land on an explanatory page that offers a one-click action to request a new confirmation email for the same address.
5. **Given** an already-active subscriber address (or a suppressed address), **When** it is submitted on the page, **Then** the same neutral "check your email" page is shown and the page does not reveal whether the address was already subscribed or suppressed.
6. **Given** a submission with a malformed email address or a missing required field, **When** it is attempted, **Then** the page rejects it inline with a clear, field-level message and no pending subscription is created.
7. **Given** a subscription page bound to a list that has been deleted or deactivated, **When** the per-tenant URL is opened, **Then** a clear "not available" page is shown — not an application error.

---

### User Story 2 - Manage preferences or unsubscribe via a personal link (Priority: P1)

An existing subscriber opens a personalized preference link (delivered in campaign footers and confirmation emails) and lands on a preference page themed by the tenant's branding. The page shows their current profile fields and list memberships, lets them update either, lets them unsubscribe from a single list or from the tenant entirely, and confirms each change. The page never asks them to log in and never exposes any other subscriber's data. An invalid, tampered, or expired link lands on an access-denied page that exposes no subscriber data. One-click unsubscribe initiated by an email client succeeds without any page interaction and shows a minimal confirmation if the browser does load.

**Why this priority**: Self-serve preference management and unsubscribe are part of the same exit criterion as Story 1 and are a legal requirement — they cannot ship later than the subscription flow they support. They are independently demonstrable: open a real preference link, change something, save, reload, and confirm the change persisted; unsubscribe and confirm the subscriber is removed from future sends.

**Independent Test**: Generate a preference link for a known subscriber from the tenant workspace (or pull it from a real campaign footer), open it in a private browser session, change a list membership and a profile field, save, reload, and confirm the changes persisted. Use the unsubscribe-from-all action and confirm the subscriber stops receiving future sends. Open a tampered or expired link and confirm the access-denied page exposes no data.

**Acceptance Scenarios**:

1. **Given** a valid preference link, **When** the subscriber opens it, **Then** the preference page renders with the tenant's branding and shows the subscriber's current profile fields and list memberships without requiring a login.
2. **Given** the preference page is open, **When** the subscriber updates list memberships and/or profile fields and saves, **Then** the page confirms the change and the same values are visible on reload.
3. **Given** the preference page is open, **When** the subscriber chooses unsubscribe-from-all, **Then** the page shows an explicit confirmation that the subscriber has been unsubscribed and will receive no further campaigns.
4. **Given** an invalid, tampered, or expired preference link, **When** it is opened, **Then** the page shows a generic access-denied state and exposes no subscriber identity, list membership, or profile data.
5. **Given** a one-click unsubscribe request from an email client, **When** it is dispatched, **Then** the unsubscribe completes without any page interaction; if the browser loads the link the page shows a minimal "you have been unsubscribed" confirmation.
6. **Given** a preference link belonging to a subscriber who has since been deleted, **When** it is opened, **Then** the same access-denied page is shown rather than an error page.

---

### User Story 3 - Browse a tenant's public campaign archive and RSS feed (Priority: P2)

A visitor opens a tenant's public archive index URL and sees a list of the tenant's archive-visible campaigns, newest first, each with title, send date, and a link to its standalone page. Opening one renders the campaign as a standalone public web page with the tenant's branding and custom CSS applied. A feed reader pointed at the tenant's RSS URL receives a valid feed of the same archive-visible campaigns. Tenants with zero archive-visible campaigns show an explicit empty archive page and a valid empty feed. Attempting to access a draft, hidden, or other-tenant campaign by guessing a URL lands on a not-found page.

**Why this priority**: The public archive and RSS feed satisfy the rest of Epic G but are not required for the self-serve subscribe and preference flows, so they ship after Stories 1 and 2. They are independently demonstrable: mark a sent campaign archive-visible and confirm it appears at the public archive index, as a standalone page, and in the RSS feed.

**Independent Test**: Mark a sent campaign as archive-visible from the workspace, open the tenant's public archive index URL, confirm the campaign is listed with title and send date, open its standalone page and confirm tenant branding is applied, point a feed validator at the tenant's RSS URL and confirm it parses cleanly and includes the campaign. Mark the campaign hidden and confirm it disappears from both surfaces within a few minutes.

**Acceptance Scenarios**:

1. **Given** a tenant with one or more archive-visible campaigns, **When** a visitor opens the public archive index URL, **Then** the listed campaigns appear newest-first with title, send date, and a link to each campaign's standalone page, themed by the tenant's branding.
2. **Given** an archive-visible campaign, **When** a visitor opens its standalone page URL, **Then** the campaign content renders as a standalone public web page with the tenant's branding and custom CSS applied.
3. **Given** a tenant with no archive-visible campaigns, **When** a visitor opens the public archive index URL, **Then** an explicit empty-archive page is shown — not an error.
4. **Given** a campaign that is a draft, scheduled, or not flagged archive-visible, **When** a visitor attempts to open its standalone page URL by guessing it, **Then** a not-found page is shown and no campaign content is exposed.
5. **Given** the tenant's RSS feed URL, **When** a feed reader fetches it, **Then** it returns a valid feed containing the archive-visible campaigns with title, link, publication date, and summary; the feed is valid even when the tenant has zero campaigns.
6. **Given** two tenants with different branding and custom CSS, **When** a visitor moves between their archive pages, **Then** each tenant's branding is applied to its own pages and one tenant's CSS never affects the other's.

---

### User Story 4 - Configure public pages and branding from the workspace (Priority: P1)

A tenant administrator opens a "Public pages" area in the workspace and configures everything that drives Stories 1–3 from the tenant side: which list(s) a subscription page is bound to and which profile fields appear on it (and which are required); the tenant's branding (logo, colors) and custom CSS applied to all of its public pages; and, on each sent campaign, whether it is archive-visible. The area shows the per-tenant public URLs (subscription, preference, archive, RSS) so the administrator can copy and share them, and offers a "preview" that opens the public page in a new browser context to verify the result. Custom CSS is captured here but the platform's sanitization is enforced server-side; the editor explains that markup is sanitized and shows the sanitized preview, not the raw input.

**Why this priority**: Without this configuration surface, Stories 1–3 have no inputs — there is no public URL to open and no branding to render. It is independently demonstrable: create a subscription page, set branding, copy the public URL, and confirm Story 1 works against it; toggle archive-visibility on a sent campaign and confirm Story 3's surfaces reflect the change.

**Independent Test**: As an administrator, open the public-pages configuration area, create a subscription page bound to a list with one required custom field, set the tenant's branding (logo, primary color, custom CSS), copy the subscription URL, and open it in a private browser session to confirm Story 1 renders with the branding and the configured fields. Toggle archive-visibility on a sent campaign and confirm Story 3's index and RSS feed reflect the change.

**Acceptance Scenarios**:

1. **Given** the public-pages area is open, **When** the administrator creates a subscription page, **Then** they can pick the target list(s), pick which profile fields appear and which are required, and save; the area then shows the public URL of the saved subscription page.
2. **Given** an existing subscription page, **When** the administrator edits its bound lists or field configuration and saves, **Then** the change is reflected the next time the public URL is opened.
3. **Given** the branding section, **When** the administrator uploads a logo, picks colors, and pastes custom CSS, then saves, **Then** the saved branding is applied to every one of the tenant's public pages on the next render.
4. **Given** the branding section, **When** the administrator pastes CSS or markup containing disallowed constructs (script, escape-from-container, etc.), **Then** the area communicates that the input is sanitized and shows the sanitized preview rather than silently dropping content with no feedback.
5. **Given** a sent campaign, **When** the administrator toggles its archive-visible flag, **Then** the campaign appears or disappears in the tenant's archive index, RSS feed, and standalone page within the platform's archive freshness window.
6. **Given** the public-pages area, **When** the administrator views it, **Then** the per-tenant public URLs (subscription, preference link template, archive index, RSS feed) are shown and individually copyable.
7. **Given** the administrator lacks the public-pages permission, **When** they navigate the workspace, **Then** the public-pages area and its navigation entry are hidden or disabled, consistent with the existing UI permission gating.

---

### User Story 5 - Manage media in the tenant library (Priority: P2)

A tenant team member opens a "Media" area in the workspace and sees the tenant's previously uploaded media in a browsable library with image previews. They upload new files via a clear upload control with progress feedback; uploads that exceed the size limit or use a disallowed type are rejected up front with a specific reason and nothing is stored. Each uploaded asset has a stable, copyable URL usable in campaign content. They can delete an asset they no longer need; deletion is confirmed and the asset disappears from the listing. They never see, search, or download another tenant's media regardless of how a URL is requested. Within the campaign authoring surfaces from prior phases, an "insert from media library" picker reuses the same library and inserts a reference to the chosen asset.

**Why this priority**: The media library is the second exit criterion of Phase 6 and completes Epic H. It is independent of the public-page flows in Stories 1–4 and can be built and tested on its own, so it sits at P2 alongside Story 3 and below the P1 self-serve and configuration stories.

**Independent Test**: Open the media library as a team member, upload an image, confirm it appears with a preview and a copyable URL, paste that URL into campaign content from the existing authoring surface (or use the media picker) and confirm the image renders, delete the asset and confirm it disappears from the listing. Attempt an upload that violates the size or type limit and confirm it is rejected with a specific reason. As a member of a different tenant, attempt to access the first tenant's asset URL and confirm the request is denied.

**Acceptance Scenarios**:

1. **Given** the media library is open, **When** a team member uploads a file within the configured size and type limits, **Then** progress is shown, the file appears in the library on completion with a preview (for image types), and a stable URL is copyable from its detail view.
2. **Given** an upload attempt that violates the size limit or uses a disallowed type, **When** it is started, **Then** it is rejected up front with a specific reason and nothing is stored.
3. **Given** the media library is open, **When** the team member browses it, **Then** only the operator's own tenant's media is listed; another tenant's media is never visible.
4. **Given** an asset in the library, **When** the team member deletes it after a confirmation prompt, **Then** the asset disappears from the listing.
5. **Given** the campaign authoring surface from prior phases, **When** the author opens the media picker, **Then** the same tenant-scoped library is browsable from within the picker and selecting an asset inserts its reference into the campaign content.
6. **Given** an upload that fails or is interrupted (connection drop, server rejection), **When** it does not complete, **Then** the library listing does not show a partial or broken entry for it.
7. **Given** the team member lacks the media permission, **When** they navigate the workspace, **Then** the media area and its navigation entry are hidden or disabled, consistent with the existing UI permission gating.

---

### Edge Cases

- The public subscription page is opened on a slow mobile connection — content remains readable as soon as the page's text is delivered, and the submit control is not enabled until validation is ready.
- A visitor submits the subscription form repeatedly within a short window — the page surfaces a clear rate-limited message instead of silently dropping the submission or creating duplicates.
- A confirmation link is opened twice (e.g. once by an email-scanner pre-fetch and once by the visitor) — the second open lands on a benign "already confirmed" page rather than an error.
- A preference link is opened in a shared/public computer — the page does not cache subscriber-identifying information visibly after the tab is closed; refreshing the link must still re-fetch fresh data.
- A tenant's custom CSS is changed while a public page is open in another tab — the page picks up the new branding on the next navigation or refresh, not mid-render.
- The archive index for a tenant contains a campaign whose content references a media asset that was later deleted — the standalone archive page renders without a broken layout (a placeholder for the missing asset is acceptable).
- The RSS feed is requested with a feed reader that does conditional GETs — the feed responds correctly to revalidation rather than always returning a fresh body.
- The media library is opened by a team member with no prior uploads — an explicit empty state is shown with the upload control, not a blank pane.
- A media upload completes but the asset is removed by another team member before the first viewer's browser tab refreshes — the missing asset is handled gracefully (placeholder + clear "no longer available" message) rather than as an error.
- The public-pages configuration area is opened by an administrator for a tenant that has no subscription pages yet — an explicit empty state and a clear "create your first subscription page" path are shown, not a blank pane.
- The administrator pastes very large custom CSS that exceeds a reasonable limit — the editor communicates the limit and rejects the save rather than silently truncating.
- The archive index for a tenant is very long — it pages or loads incrementally without freezing the public page.
- A subscriber's preference link is regenerated server-side (token rotation) while they hold an old link — opening the old link lands on the access-denied page, not on the previous subscriber's view.

## Requirements *(mandatory)*

### Functional Requirements

#### Public subscription page (visitor-facing)

- **FR-001**: The interface MUST render a tenant's public subscription page on a per-tenant URL without requiring authentication.
- **FR-002**: The subscription page MUST render the tenant's branding (logo, colors, custom CSS) and MUST NOT render any other tenant's branding, lists, fields, or content.
- **FR-003**: The subscription page MUST show the email field plus the tenant-configured visible profile fields, marking the required ones, and MUST validate the address format and required-field presence client-side before submission.
- **FR-004**: On a valid submission, the interface MUST land the visitor on a neutral "check your email" page that names the submitted address — the same page whether the address is new, already subscribed, or suppressed (no disclosure of existing state).
- **FR-005**: On an invalid submission, the interface MUST surface inline, field-level errors and MUST NOT submit.
- **FR-006**: The interface MUST present an "expired link" page when a confirmation token is no longer valid, offering a one-click action to request a fresh confirmation email for the same address.
- **FR-007**: The interface MUST present a "confirmation success" page when a confirmation link is followed successfully, naming the list(s) joined and offering a link to manage preferences.
- **FR-008**: The interface MUST present an "already confirmed" page if a valid confirmation link is opened a second time (e.g. due to email-scanner pre-fetch), not an error.
- **FR-009**: The interface MUST present a "not available" page if the subscription URL belongs to a list that has been deleted or deactivated, rather than an application error.
- **FR-010**: The interface MUST surface a clear "too many attempts, try again later" message when the server rate-limits a submission, instead of silently failing or duplicating.

#### Preference & unsubscribe pages (subscriber-facing)

- **FR-011**: The interface MUST render the per-subscriber preference page on a per-tenant URL, themed by the tenant's branding, without requiring a login.
- **FR-012**: The preference page MUST show the subscriber's current profile field values and list memberships and MUST let them update either and save.
- **FR-013**: The preference page MUST offer an unsubscribe action per list and an unsubscribe-from-all action, and MUST present an explicit confirmation page after either is performed.
- **FR-014**: The interface MUST present a generic access-denied page when a preference link is invalid, tampered, expired, or belongs to a deleted subscriber, and MUST NOT expose any subscriber identity, list membership, or profile data on that page.
- **FR-015**: The interface MUST support the one-click unsubscribe path so that an email client's POST/GET completes without page interaction; if a browser does load the link, a minimal "you have been unsubscribed" confirmation MUST be shown.
- **FR-016**: The interface MUST re-fetch fresh subscriber data on every preference-page load (no client-side caching of subscriber-identifying state across reloads).

#### Public archive & RSS (visitor-facing)

- **FR-017**: The interface MUST render a per-tenant public archive index on a per-tenant URL, listing archive-visible campaigns newest-first with title, send date, and a link to each standalone campaign page, themed by the tenant's branding.
- **FR-018**: The interface MUST render each archive-visible campaign as a standalone public web page themed by the tenant's branding.
- **FR-019**: The interface MUST present an explicit empty-archive page when the tenant has no archive-visible campaigns, rather than an error or blank page.
- **FR-020**: The interface MUST present a not-found page when a campaign URL is opened that is a draft, scheduled, hidden, deleted, or owned by another tenant — and MUST NOT expose campaign content in any of those cases.
- **FR-021**: The interface MUST expose a per-tenant RSS feed URL whose response is a feed valid against the RSS standard for tenants with zero, one, or many archive-visible campaigns.
- **FR-022**: The interface MUST surface a tenant's archive index and individual archive pages without applying any other tenant's CSS, branding, or assets, regardless of URL guessing or cross-linking.
- **FR-023**: The interface MUST load a long archive index incrementally (paging or infinite scroll) without freezing the public page.

#### Public-pages configuration (workspace, administrator-facing)

- **FR-024**: The workspace MUST provide a "Public pages" area, gated by an existing permission, where administrators configure subscription pages, branding, and archive visibility for the tenant.
- **FR-025**: The area MUST let an administrator create, edit, and delete subscription pages, specifying the bound list(s) and the visible/required profile fields per page.
- **FR-026**: The area MUST show the public URL of each subscription page and provide a copy-to-clipboard control for it.
- **FR-027**: The area MUST provide a branding section where an administrator uploads a logo, picks colors, and pastes custom CSS; saves MUST be applied to subsequent renders of the tenant's public pages.
- **FR-028**: The branding section MUST communicate that custom CSS is sanitized server-side, show a sanitized preview, and surface a clear limit if the input exceeds a maximum size — rather than silently truncating.
- **FR-029**: The area MUST expose a per-campaign archive-visible toggle on sent campaigns (either inline in the existing sending UI or within the public-pages area), and toggling it MUST cause the campaign to appear in or disappear from the archive surfaces within the platform's archive freshness window.
- **FR-030**: The area MUST display, in one place, the per-tenant public URLs the administrator may want to share — the subscription page URL(s), the preference-link template, the archive index URL, and the RSS feed URL — each individually copyable.
- **FR-031**: The area MUST present an explicit empty state with a "create your first subscription page" call to action when no subscription pages exist yet.
- **FR-032**: The area MUST offer a "preview" control for each subscription page, preference page (with a test subscriber's link), and archive page that opens the public page in a new browser context so the administrator can verify the result.

#### Media library (workspace, team-facing)

- **FR-033**: The workspace MUST provide a "Media" area, gated by an existing permission, where team members upload, browse, and delete the tenant's media.
- **FR-034**: The area MUST list the tenant's media with image previews where applicable and MUST never list another tenant's media regardless of how the underlying storage path is requested.
- **FR-035**: The area MUST provide an upload control with progress feedback, MUST reject files that exceed the configured size limit or use a disallowed type up front with a specific reason, and MUST store nothing for a rejected upload.
- **FR-036**: The area MUST present each asset's stable, copyable reference URL on its detail view for use in campaign content.
- **FR-037**: The area MUST let a team member delete an asset behind a confirmation prompt, removing it from the listing on success.
- **FR-038**: The area MUST present an explicit empty state with the upload control when the tenant has no media yet.
- **FR-039**: The area MUST handle interrupted or failed uploads so that no partial or broken entry appears in the listing, and MUST surface the failure clearly to the uploader.
- **FR-040**: The campaign authoring surfaces from prior phases MUST integrate a "media picker" that browses the same tenant-scoped library and inserts a reference to the chosen asset into the campaign content.
- **FR-041**: The area MUST handle the case where an asset referenced from a campaign was deleted by another team member, rendering a placeholder with a clear "no longer available" message rather than treating it as an error.

#### Cross-cutting

- **FR-042**: All public pages — subscription, preference, confirmation, archive index, standalone archive, RSS, and any informational pages (expired-link, access-denied, not-available, not-found) — MUST be scoped to a single tenant per URL and MUST never expose another tenant's lists, subscribers, campaigns, branding, or media.
- **FR-043**: Public pages MUST be rendered without the workspace sidebar/app shell; they are standalone, themed only by the tenant's branding and custom CSS.
- **FR-044**: Public pages MUST be usable on mobile browsers (responsive layout) and MUST meet baseline accessibility expectations consistent with the existing workspace UI (keyboard navigation, focus, labels, contrast).
- **FR-045**: The workspace surfaces added by this feature (public-pages configuration, media library, media picker, archive-visible toggle) MUST live within the existing tenant workspace app shell and follow the navigation, loading, empty, and error-state conventions established by the Phase 1–5 UI.
- **FR-046**: The interface MUST hide or disable the public-pages and media areas and their navigation entries for operators who lack the relevant permission, consistent with the existing UI permission gating.
- **FR-047**: The interface MUST surface clear, actionable messages for every failed operation (subscribe, confirm, preference save, unsubscribe, fresh-confirmation request, branding save, archive toggle, media upload, media delete) rather than silent failures or generic errors.

### Key Entities *(include if feature involves data)*

- **Subscription page (as shown)**: A tenant-owned public page configuration — target list(s), visible/required profile fields, branding reference, and a per-tenant public URL. One tenant may have several.
- **Pending subscription (as shown)**: A not-yet-confirmed submission represented in the UI by the "check your email" page and the eventual confirmation-success / expired-link outcomes — no public list of pending subscriptions is exposed.
- **Preference page (as shown)**: A subscriber-scoped public view of profile fields and list memberships, reachable only through a unique, time-limited, tamper-resistant token; never identifies a subscriber until the token is validated server-side.
- **Tenant branding (as shown)**: The tenant's logo, colors, and (sanitized) custom CSS applied to every one of its public pages.
- **Archive entry (as shown)**: A sent campaign exposed to the public archive — its title, send date, content, and archive-visible flag.
- **RSS feed (as shown)**: A per-tenant URL whose response is a valid RSS feed of the tenant's archive entries.
- **Media asset (as shown)**: An uploaded file owned by a tenant — original filename, content type, size, image preview (where applicable), and a stable, copyable reference URL.
- **Public URL bundle (as shown)**: The set of per-tenant URLs exposed in the public-pages configuration area — subscription page URL(s), preference link template, archive index URL, RSS feed URL — each individually copyable.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A visitor can go from opening a tenant's subscription URL to landing on the confirmation-success page in under 2 minutes (excluding their own email-client delay), without contacting the tenant.
- **SC-002**: 100% of submissions made through the public subscription page result in either a "check your email" page (regardless of whether the address was new, already subscribed, or suppressed) or an inline validation error — never a disclosure of existing subscription status.
- **SC-003**: A subscriber can open a preference link, change a preference or unsubscribe, and see an explicit confirmation in under 1 minute, and the change is reflected in the very next campaign send.
- **SC-004**: 0% of public pages, archive pages, RSS feeds, or media assets expose data belonging to a different tenant under access-isolation testing, regardless of URL guessing or cross-tenant cross-linking.
- **SC-005**: A campaign newly marked archive-visible appears at the public archive index, on its standalone page, and in the RSS feed within 5 minutes of the toggle.
- **SC-006**: The per-tenant RSS feed validates cleanly against a standard feed validator for tenants with zero, one, and many archive-visible campaigns.
- **SC-007**: A team member can go from opening the media library to having an uploaded asset usable in campaign content in under 1 minute; 100% of uploads violating the size or type rules are rejected up front with a specific reason and nothing is stored.
- **SC-008**: An administrator can configure a new subscription page (lists, fields, branding) and open its public URL in a new browser context within 5 minutes, with the branding visibly applied.
- **SC-009**: Every failed operation in the new workspace areas (branding save, archive toggle, media upload, media delete) surfaces a clear, named reason — operators are never left with a silent failure or a generic error.
- **SC-010**: All five user stories can be demonstrated end-to-end against the tenant workspace and the per-tenant public URLs without manual data setup beyond normal tenant configuration, satisfying both Phase 6 exit criteria from the UI side.

## Assumptions

- The Phase 6 backend (012) is delivered and exposes the tenant-scoped HTTP endpoints for subscription pages, pending and confirmed subscriptions, preference and unsubscribe tokens, branding (logo, colors, sanitized CSS), the archive index, individual archive pages, the RSS feed, and the media library; this feature builds the UI on top of those endpoints and adds no new backend capability.
- The authenticated workspace surfaces extend the existing tenant workspace web application and its sidebar app shell, reusing navigation, permission-gating, loading, empty, and error-state patterns established by the Phase 1–5 UI.
- The public pages are a new surface rendered on per-tenant URLs (a tenant-scoped path or subdomain) without the workspace sidebar/app shell; fully custom/vanity domains are out of scope for this phase.
- Double opt-in is the only public subscription mode; no UI is provided for single opt-in.
- Confirmation and preference links carry tokens whose lifetimes and rotation are decided server-side; the UI surfaces expired-link and access-denied states but does not configure token lifetimes.
- CSS sanitization, file-type and size limits, and tenant-storage prefixing are enforced server-side; the UI communicates the limits and the sanitized result but is not the source of truth for them.
- The campaign authoring surfaces from prior phases exist and can host a media picker; this feature adds the picker into those surfaces rather than introducing a new authoring tool.
- Desktop web is the primary target for the authenticated workspace surfaces, consistent with the existing workspace UI; the public pages MUST also work on mobile browsers (responsive), because visitors arrive on whatever device they happen to use.
- A real payment-provider UI, multi-language public pages, and a public-page WYSIWYG/page-builder are explicitly out of scope; the only authoring surface added is the subscription-page field configuration and the branding section.
- Archive freshness is bounded by an existing platform window (the success criterion gives 5 minutes); no separate cache-invalidation UI is introduced.
