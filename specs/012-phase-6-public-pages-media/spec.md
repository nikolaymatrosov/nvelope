# Feature Specification: Phase 6 — Public Pages & Media

**Feature Branch**: `012-phase-6-public-pages-media`

**Created**: 2026-05-19

**Status**: Draft

**Input**: User description: "Phase 6 — Public Pages & Media. 6.1 Public subscription page, double-opt-in flow, and preference management. 6.2 Campaign archive and RSS feed with per-tenant branding/CSS. 6.3 Media library backed by S3-compatible object storage, tenant-prefixed. Exit criteria: subscribers can self-serve via public pages; media uploads work. (Satisfies Epic G, completes Epic H.)"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Self-serve subscription with double opt-in (Priority: P1)

A visitor lands on a tenant's public subscription page, enters their email address (and any optional fields the tenant has configured), and submits. The platform records a pending subscription and sends a confirmation email. When the visitor clicks the confirmation link, their subscription becomes active and they are added to the tenant's selected list(s). If they never confirm, they are not counted as an active subscriber and receive no campaigns.

**Why this priority**: This is the core of the exit criterion "subscribers can self-serve via public pages." Without it, tenants must import subscribers manually. Double opt-in is also a deliverability and compliance prerequisite for everything else in this phase.

**Independent Test**: Open a tenant's public subscription page in a browser, submit a new email address, retrieve the confirmation email, click the link, and verify the subscriber appears as active on the tenant's list. Submitting without confirming leaves the subscriber pending.

**Acceptance Scenarios**:

1. **Given** a tenant has a public subscription page for a list, **When** a visitor submits a valid, previously-unknown email address, **Then** a pending subscription is created, a confirmation email is sent, and the visitor sees a "check your email" message.
2. **Given** a pending subscription, **When** the visitor clicks the confirmation link, **Then** the subscription becomes active, the subscriber is added to the target list(s), and the visitor sees a success page.
3. **Given** a pending subscription whose confirmation link has expired, **When** the visitor clicks it, **Then** they see an explanatory message and an option to request a new confirmation email.
4. **Given** an email address that is already an active subscriber on the target list, **When** it is submitted again, **Then** no duplicate is created and the visitor sees a neutral confirmation message (without disclosing existing subscription status).
5. **Given** an email address that previously unsubscribed or was suppressed, **When** it is submitted, **Then** the platform follows the suppression rules and does not silently re-subscribe a suppressed address.

---

### User Story 2 - Preference management and self-serve unsubscribe (Priority: P1)

An existing subscriber follows a personalized link (from a campaign footer or confirmation email) to a preference page where they can update their profile fields, change which lists they belong to, and unsubscribe entirely. Changes take effect immediately and are reflected in future sends.

**Why this priority**: Self-serve preference and unsubscribe management is part of the same exit criterion and is a legal requirement for sending email. It must ship alongside subscription so subscribers are never trapped.

**Independent Test**: Generate a preference link for a known subscriber, open it, change a list membership and a profile field, save, and verify the changes persist. Use the unsubscribe action and verify the subscriber stops receiving campaigns.

**Acceptance Scenarios**:

1. **Given** a valid preference link, **When** the subscriber opens it, **Then** the page shows their current profile fields and list memberships without requiring a login.
2. **Given** the preference page is open, **When** the subscriber changes list memberships or profile fields and saves, **Then** the changes persist and are visible on reload.
3. **Given** the preference page is open, **When** the subscriber chooses to unsubscribe from all lists, **Then** they are marked unsubscribed, added to the tenant's suppression scope, and excluded from future campaigns.
4. **Given** an invalid, tampered, or expired preference link, **When** it is opened, **Then** access is denied and no subscriber data is exposed.
5. **Given** a one-click unsubscribe request originating from an email client, **When** it is received, **Then** the subscriber is unsubscribed without requiring them to load and interact with the preference page.

---

### User Story 3 - Public campaign archive and RSS feed (Priority: P2)

A tenant exposes a public archive of campaigns it has sent. Visitors can browse the archive, open an individual archived campaign as a web page, and subscribe to an RSS feed of new campaigns. The archive and individual pages reflect the tenant's branding (logo, colors, custom CSS).

**Why this priority**: The archive and RSS feed satisfy Epic G content but are not required for a subscriber to subscribe or manage preferences. They add reach and discoverability once the P1 self-serve flows exist.

**Independent Test**: Mark a sent campaign as archive-visible, open the tenant's public archive index, confirm the campaign is listed and renders as a standalone page with tenant branding, and confirm the RSS feed validates and includes the campaign.

**Acceptance Scenarios**:

1. **Given** a tenant has sent campaigns marked as archive-visible, **When** a visitor opens the public archive index, **Then** those campaigns are listed newest-first with title, date, and link.
2. **Given** an archived campaign, **When** a visitor opens its public page, **Then** the campaign content renders as a standalone web page with the tenant's branding and custom CSS applied.
3. **Given** a tenant has an RSS feed enabled, **When** a feed reader fetches it, **Then** it returns a valid feed containing archive-visible campaigns with title, link, publication date, and summary.
4. **Given** a campaign that has not been marked archive-visible (or is a draft), **When** a visitor attempts to access it by guessing a URL, **Then** access is denied.
5. **Given** a tenant has configured branding and custom CSS, **When** any public page for that tenant is rendered, **Then** that branding is applied and one tenant's CSS never affects another tenant's pages.

---

### User Story 4 - Tenant media library (Priority: P2)

A tenant team member opens the media library, uploads images and other supported files, browses previously uploaded media, and references those assets when composing a campaign. Each tenant's media is isolated from every other tenant's.

**Why this priority**: The media library satisfies the second exit criterion ("media uploads work") and completes Epic H. It is independent of the public-page flows and can be built and tested on its own.

**Independent Test**: Upload an image through the media library, confirm it appears in the library listing with a usable URL, reference it in a campaign, and confirm a different tenant cannot see or access that file.

**Acceptance Scenarios**:

1. **Given** the media library is open, **When** a team member uploads a supported file within the size limit, **Then** the file is stored, appears in the library listing, and has a stable URL usable in campaign content.
2. **Given** a file that exceeds the size limit or has an unsupported type, **When** an upload is attempted, **Then** the upload is rejected with a clear error and nothing is stored.
3. **Given** stored media exists, **When** a team member browses the library, **Then** they see only their own tenant's media, with previews for images.
4. **Given** a media file is no longer needed, **When** a team member deletes it, **Then** it is removed from the library listing.
5. **Given** a file stored by Tenant A, **When** Tenant B requests it by guessing its storage path, **Then** the request is denied and the file is not served.

---

### Edge Cases

- A visitor submits the subscription form repeatedly in a short window — the system rate-limits and avoids sending a flood of confirmation emails to the same address.
- A confirmation or preference link is used after the subscriber has already been deleted — the page handles the missing subscriber gracefully without an error page.
- A visitor submits a malformed or disposable-domain email address — the form validates and rejects it before creating a pending subscription.
- A campaign in the archive references a media asset that was later deleted — the archived page still renders without breaking layout.
- A tenant's custom CSS contains markup that could escape its container or inject script — the platform sanitizes/sandboxes it so it cannot run arbitrary code or affect platform chrome.
- Two team members upload files with identical names — both are stored without one overwriting the other.
- An upload connection drops midway — no partial file appears in the library listing.
- The RSS feed is requested for a tenant with zero archive-visible campaigns — a valid but empty feed is returned.
- A subscription page is loaded for a list that was deleted or deactivated — the visitor sees a clear "not available" message rather than an error.

## Requirements *(mandatory)*

### Functional Requirements

#### Public subscription & double opt-in

- **FR-001**: System MUST provide each tenant with a public, unauthenticated subscription page bound to one or more of the tenant's lists.
- **FR-002**: System MUST allow tenants to configure which profile fields (beyond email) appear on the subscription page and which are required.
- **FR-003**: System MUST validate submitted email addresses for format and reject obviously invalid submissions before creating a pending subscription.
- **FR-004**: System MUST create subscriptions in a pending state and send a confirmation email containing a unique, time-limited confirmation link.
- **FR-005**: System MUST activate a subscription only after the confirmation link is followed, then add the subscriber to the configured target list(s).
- **FR-006**: System MUST allow a visitor to request a fresh confirmation email when a link has expired.
- **FR-007**: System MUST NOT disclose, on the public subscription page, whether a submitted address is already subscribed.
- **FR-008**: System MUST honor existing suppression and unsubscribe state and not silently re-subscribe a suppressed address through the public page.
- **FR-009**: System MUST rate-limit subscription submissions per address and per source to prevent abuse and confirmation-email flooding.

#### Preference management & unsubscribe

- **FR-010**: System MUST provide a per-subscriber preference page reachable through a unique, tamper-resistant link that does not require account login.
- **FR-011**: Subscribers MUST be able to view and update their profile fields and list memberships from the preference page.
- **FR-012**: Subscribers MUST be able to unsubscribe from individual lists or from all of a tenant's communications from the preference page.
- **FR-013**: System MUST apply preference and unsubscribe changes immediately so they affect any subsequent campaign send.
- **FR-014**: System MUST support one-click unsubscribe initiated from an email client without requiring page interaction.
- **FR-015**: System MUST deny access and expose no subscriber data when a preference link is invalid, tampered, or expired.

#### Campaign archive & RSS

- **FR-016**: System MUST allow tenants to mark individual sent campaigns as visible or hidden in the public archive.
- **FR-017**: System MUST provide a public archive index per tenant listing archive-visible campaigns newest-first with title, send date, and link.
- **FR-018**: System MUST render each archive-visible campaign as a standalone public web page.
- **FR-019**: System MUST provide a per-tenant RSS feed of archive-visible campaigns that validates against the RSS standard.
- **FR-020**: System MUST deny public access to campaigns that are drafts or not marked archive-visible.
- **FR-021**: System MUST allow each tenant to configure branding (logo, colors) and custom CSS applied to its public pages (subscription, preference, archive).
- **FR-022**: System MUST isolate per-tenant custom CSS so it cannot affect other tenants' pages or the platform's own interface, and MUST sanitize it so it cannot execute arbitrary code.

#### Media library

- **FR-023**: System MUST let authenticated tenant team members upload media files into a per-tenant media library.
- **FR-024**: System MUST store every media file under a storage path prefixed by the owning tenant so files are namespaced per tenant.
- **FR-025**: System MUST enforce a maximum file size and an allowed-file-type list, rejecting non-conforming uploads with a clear error.
- **FR-026**: System MUST list a tenant's media with image previews and provide a stable URL for each asset usable in campaign content.
- **FR-027**: System MUST allow team members to delete media from their tenant's library.
- **FR-028**: System MUST prevent any tenant from listing, viewing, or downloading another tenant's media regardless of how the storage path is requested.
- **FR-029**: System MUST ensure interrupted or failed uploads do not leave partial or inaccessible files in the library listing.

#### Cross-cutting

- **FR-030**: All public pages MUST be scoped to the correct tenant and MUST never expose another tenant's lists, subscribers, campaigns, or media.
- **FR-031**: System MUST record self-serve actions (subscribe, confirm, preference change, unsubscribe) so they are auditable and attributable.

### Key Entities *(include if data involved)*

- **Subscription Page**: A tenant-owned public page configuration — target list(s), visible/required profile fields, branding reference. One tenant may have several.
- **Pending Subscription**: A not-yet-confirmed subscription created from a public submission; holds the submitted email/fields, a confirmation token, and an expiry; becomes a Subscriber on confirmation.
- **Confirmation Token**: A unique, time-limited credential tying a pending subscription (or re-confirmation request) to a confirmation action.
- **Preference Link / Token**: A unique, tamper-resistant credential identifying a subscriber for self-serve preference and unsubscribe access without login.
- **Archive Entry**: A sent campaign exposed publicly — references the campaign content, send date, and an archive-visible flag.
- **RSS Feed**: A per-tenant generated feed derived from that tenant's archive entries.
- **Tenant Branding**: Per-tenant logo, colors, and custom CSS applied across the tenant's public pages.
- **Media Asset**: An uploaded file owned by a tenant — original filename, content type, size, tenant-prefixed storage path, and a stable reference URL.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A visitor can go from opening a tenant's subscription page to a confirmed, active subscription in under 2 minutes (excluding their own email-client delay).
- **SC-002**: 100% of subscriptions created through the public page require confirmation before the subscriber receives any campaign.
- **SC-003**: A subscriber can update preferences or unsubscribe in under 1 minute from opening their preference link, and the change is reflected in the very next campaign send.
- **SC-004**: 0% of public pages, archives, RSS feeds, or media expose data belonging to a different tenant under access-isolation testing.
- **SC-005**: A tenant's public archive index and RSS feed reflect a newly archived campaign within 5 minutes of it being marked archive-visible.
- **SC-006**: Generated RSS feeds validate cleanly against a standard feed validator for tenants with zero, one, and many campaigns.
- **SC-007**: A team member can upload a media file and reference it in a campaign within 1 minute, and uploads that violate size or type rules are rejected 100% of the time.
- **SC-008**: All four user stories can be demonstrated end-to-end without manual data setup beyond normal tenant configuration, satisfying both exit criteria.

## Assumptions

- Public pages are hosted by the platform on per-tenant URLs (e.g., a tenant-scoped path or subdomain). Fully custom/vanity domains for public pages are out of scope for this phase.
- The existing tenancy, subscriber, list, suppression, and campaign-sending capabilities from prior phases are reused; this phase adds the public-facing and media layers on top of them.
- Confirmation and preference links use unique, time-limited, tamper-resistant tokens; default link lifetime follows industry-standard practice (on the order of a few days) unless a tenant configures otherwise.
- Double opt-in is the default and required behavior for public subscription pages in this phase (single opt-in is not offered for public pages).
- The media library accepts common image formats plus a small set of document types, with a per-file size limit consistent with email-asset use (assume ~10 MB default) — exact lists are configurable.
- S3-compatible object storage is available to the platform; media is served either directly from storage or via a platform-mediated URL, with tenant isolation enforced regardless.
- Public pages must remain usable on mobile browsers and meet baseline accessibility expectations.
- Campaign content rendered in the archive reuses the same content produced by the sending pipeline; no separate authoring surface is introduced here.
- Localization of public-page copy beyond the tenant's default language is out of scope for this phase.
