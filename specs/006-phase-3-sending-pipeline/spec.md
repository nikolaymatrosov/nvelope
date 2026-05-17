# Feature Specification: Phase 3 — Sending Pipeline

**Feature Branch**: `006-phase-3-sending-pipeline`

**Created**: 2026-05-17

**Status**: Draft

**Input**: User description: "Phase 3 — Sending Pipeline: River integration and job queue definitions; sending_domains schema with Postbox domain provisioning and domain.verify polling; Postbox SES-compatible messenger with AWS SigV4 signing; Redis-coordinated per-tenant and global sliding-window rate limiting; templates and campaigns schema with the campaign.batch send pipeline and open-pixel / click-tracking link generation; transactional tx API endpoint authenticated by API key. Exit criteria: a tenant can verify a domain and send a campaign through Postbox with tracking."

## User Scenarios & Testing *(mandatory)*

<!--
  Phase 3 builds on the tenant, audience, and access foundations delivered by
  Phases 1 and 2. The "users" here are tenant operators — marketers and
  administrators who run a workspace — and tenant developers who integrate the
  transactional sending API into their own applications. Every domain, template,
  campaign, and message in this phase is confined to a single tenant; the
  isolation guarantee from Phase 1 is a hard precondition.

  This phase delivers the capability to actually send mail. Until now a tenant
  could build an audience but not reach it. Sending happens through Yandex
  Postbox, an external mail provider; before any campaign can go out, the tenant
  must prove they own the domain they want to send from.
-->

### User Story 1 - Verify a sending domain (Priority: P1)

A tenant operator adds the domain they want to send mail from. The platform
provisions a sending identity for that domain with the mail provider and shows
the operator the exact DNS records (for authentication: DKIM, SPF, DMARC) they
must add to their domain. The operator copies those records into their DNS
provider. The platform then periodically re-checks the domain and surfaces a
clear status — pending, verified, or failed — without the operator having to do
anything further. Once verified, the domain becomes available as a "send from"
choice.

**Why this priority**: No campaign and no transactional message can be sent
without a verified domain. This story is the gate that everything else in the
phase depends on, and it is the first independently demonstrable slice — a
tenant can add a domain, see records, and watch it reach "verified".

**Independent Test**: As an operator in a clean tenant, add a sending domain,
confirm the platform returns DNS records to publish, publish them on a real test
domain, and confirm the domain transitions to "verified" automatically within
the expected polling window. Confirm an unverified or failed domain cannot be
used to send.

**Acceptance Scenarios**:

1. **Given** a tenant with no sending domains, **When** the operator adds a
   domain, **Then** a sending identity is provisioned with the mail provider and
   the platform returns the DKIM, SPF, and DMARC DNS records to publish, with the
   domain in "pending" status.
2. **Given** a domain in "pending" status with the correct DNS records
   published, **When** the platform's periodic verification check runs, **Then**
   the domain transitions to "verified" and becomes selectable as a send-from
   address.
3. **Given** a domain in "pending" status, **When** the operator requests an
   immediate re-check, **Then** verification is re-run on demand and the latest
   status is shown.
4. **Given** a domain whose DNS records are missing or incorrect after the
   maximum verification window, **When** the platform evaluates it, **Then** the
   domain is marked "failed" with a reason the operator can act on.
5. **Given** a domain belonging to tenant A, **When** tenant B lists or attempts
   to send from domains, **Then** tenant A's domain is never visible or usable to
   tenant B.

---

### User Story 2 - Create and send a campaign with tracking (Priority: P1)

A tenant operator authors a campaign, optionally starting from a reusable
template, picks the verified domain to send from, targets one or more lists or
segments of subscribers, and starts the send. The platform delivers the campaign
to every targeted recipient in batches, respects the tenant's sending rate
limit, and rewrites the campaign's links and embeds a tracking pixel so that
opens and clicks can later be attributed. The operator can watch send progress
and see how many messages have gone out, failed, or remain.

**Why this priority**: This is the core deliverable of the phase and the primary
half of the exit criterion. It is independently demonstrable: create a campaign,
start it, and confirm recipients receive a tracked message.

**Independent Test**: As an operator with a verified domain and a list of test
subscribers, create a campaign from a template, target the list, start the send,
and confirm every subscriber receives the message, that links are rewritten for
click tracking, that a tracking pixel is present, and that send progress is
reported accurately.

**Acceptance Scenarios**:

1. **Given** a verified sending domain and a list with subscribers, **When** the
   operator creates a campaign, targets the list, and starts it, **Then** the
   campaign is split into batches and every targeted subscriber is sent exactly
   one message.
2. **Given** a reusable template, **When** the operator creates a campaign from
   that template, **Then** the campaign inherits the template's content and the
   operator can override it before sending.
3. **Given** a running campaign, **When** messages are sent, **Then** each
   message's links are rewritten so clicks are recorded and a tracking pixel is
   embedded so opens are recorded, scoped to the originating tenant.
4. **Given** a campaign targeting more recipients than the tenant's rate limit
   allows per interval, **When** the send runs, **Then** delivery is paced within
   the rate limit and still completes without dropping recipients.
5. **Given** a campaign that has not selected a verified sending domain, **When**
   the operator attempts to start it, **Then** the start is rejected with a
   clear reason.
6. **Given** a recipient who appears on more than one targeted list, **When** the
   campaign sends, **Then** that recipient receives the campaign only once.
7. **Given** a worker process restarts mid-send, **When** sending resumes,
   **Then** the campaign continues without sending duplicate messages to
   recipients already delivered.

---

### User Story 3 - Send transactional email via API (Priority: P2)

A tenant developer integrates their own application with the platform to send
transactional messages — password resets, receipts, confirmations. They
authenticate with a scoped API key, reference a transactional template, supply
the recipient and the variable data, and the platform sends a single message
immediately through the verified domain, tracked and counted toward the tenant's
usage.

**Why this priority**: Transactional sending is part of the exit criterion and a
distinct integration surface, but it depends on the same domain verification and
messenger machinery as campaigns, so it follows them. It is independently
testable through the API.

**Independent Test**: With a valid scoped API key and a transactional template,
call the transactional send endpoint with a recipient and variables, and confirm
a single message is delivered through the verified domain and that an invalid or
unscoped key is rejected.

**Acceptance Scenarios**:

1. **Given** a valid API key scoped for transactional sending, **When** the
   developer calls the transactional endpoint with a template, recipient, and
   variables, **Then** one message is rendered and sent immediately through the
   tenant's verified domain.
2. **Given** a request with a missing, invalid, or wrongly scoped API key,
   **When** the transactional endpoint is called, **Then** the request is
   rejected and no message is sent.
3. **Given** an API key belonging to tenant A, **When** it is used to send,
   **Then** the message can only use tenant A's templates and domains and counts
   only against tenant A's usage.
4. **Given** a transactional send request referencing a template that does not
   exist or a domain that is not verified, **When** the endpoint is called,
   **Then** the request is rejected with a clear, actionable error.

---

### Edge Cases

- What happens when the mail provider is temporarily unavailable or returns an
  error mid-campaign? Sending should retry failed batches with backoff rather
  than dropping recipients, and a campaign that accumulates too many send errors
  should auto-pause for operator review.
- What happens when a tenant deletes or loses verification on a domain that an
  in-progress campaign is sending from? Sending from a no-longer-valid domain
  must stop rather than silently continue.
- How does the system behave when a single large tenant starts a huge campaign?
  Per-tenant rate limits and fair batch scheduling must prevent one tenant from
  starving others, and a global cap must protect the shared provider account.
- What happens when the same recipient is targeted by overlapping lists or
  segments? Each recipient receives a campaign at most once.
- What happens to a campaign whose targeted lists are empty at send time? The
  campaign completes immediately with zero messages sent and a clear final
  state.
- How are tracking links and pixels handled for recipients who open the message
  much later, or repeatedly? Tracking endpoints remain resolvable and attribute
  each event to the correct tenant, campaign, and subscriber.
- What happens when a transactional API request arrives while the tenant is at
  its rate limit? The request is paced or rejected predictably rather than
  silently dropped.
- What happens when verification DNS records are published correctly but
  propagation is slow? The domain stays "pending" and continues to be re-checked
  until the maximum window, then is marked "failed" only if still unverified.

## Requirements *(mandatory)*

### Functional Requirements

#### Background job processing

- **FR-001**: The platform MUST process sending and verification work
  asynchronously through a durable job queue so that operators are not blocked
  while large sends run.
- **FR-002**: Jobs MUST be retried automatically with increasing delay when they
  fail transiently, and MUST stop retrying after a defined limit.
- **FR-003**: Every job that touches tenant data MUST carry the owning tenant's
  identity and operate strictly within that tenant's isolation boundary.
- **FR-004**: The platform MUST support the following job kinds: campaign send
  batches, campaign start, domain verification polling, and transactional and
  notification mail dispatch as needed by this phase.
- **FR-005**: If a worker process stops or restarts mid-job, the job MUST resume
  or be re-attempted without producing duplicate sends to recipients already
  delivered.

#### Sending domains

- **FR-006**: A tenant MUST be able to register a sending domain, which provisions
  a sending identity for that domain with the mail provider.
- **FR-007**: The platform MUST return the DKIM, SPF, and DMARC DNS records the
  tenant must publish for a registered domain.
- **FR-008**: Each sending domain MUST have an observable status of pending,
  verified, or failed.
- **FR-009**: The platform MUST periodically re-check pending domains and
  automatically transition them to verified or failed based on the provider's
  verification result.
- **FR-010**: A tenant MUST be able to trigger an immediate verification re-check
  for a pending domain.
- **FR-011**: A pending domain that remains unverified beyond the maximum
  verification window MUST be marked failed with an actionable reason.
- **FR-012**: Sending domains MUST be tenant-scoped and isolated; one tenant MUST
  NOT be able to see or send from another tenant's domains.

#### Mail delivery

- **FR-013**: The platform MUST deliver email through the external mail provider
  using the provider's required request authentication.
- **FR-014**: A campaign or transactional message MUST only be sendable from a
  domain that is verified and owned by the sending tenant.
- **FR-015**: Each delivered message MUST carry attribution metadata identifying
  the tenant, and where applicable the campaign and subscriber, so that later
  delivery events can be mapped back to their origin.
- **FR-016**: When the mail provider returns an error for a message or batch, the
  platform MUST retry transient failures and record permanent failures without
  losing track of which recipients were sent.

#### Rate limiting

- **FR-017**: The platform MUST enforce a per-tenant sending rate limit derived
  from the tenant's plan, measured over a sliding time window.
- **FR-018**: The platform MUST enforce a global sliding-window sending cap that
  protects the shared mail-provider account across all tenants.
- **FR-019**: Rate-limit counters MUST be shared and consistent across all
  concurrently running worker processes, so the limit holds regardless of how
  many workers are sending.
- **FR-020**: When a tenant reaches its rate limit, sending MUST be paced to stay
  within the limit rather than dropping or duplicating recipients.
- **FR-021**: Concurrent campaigns from multiple tenants MUST be scheduled fairly
  so that one large tenant cannot starve others' sends.

#### Templates and campaigns

- **FR-022**: A tenant MUST be able to create, edit, and reuse templates,
  including templates intended for transactional use.
- **FR-023**: A tenant MUST be able to create a campaign, optionally from a
  template, and target it to one or more lists or segments.
- **FR-024**: A tenant MUST be able to start a campaign, which sends it to every
  targeted recipient.
- **FR-025**: A recipient targeted by more than one of a campaign's lists or
  segments MUST receive that campaign at most once.
- **FR-026**: The platform MUST report campaign send progress, including counts
  of messages sent, failed, and remaining.
- **FR-027**: A campaign that accumulates send errors beyond a configured
  threshold MUST be automatically paused for operator review.
- **FR-028**: Templates and campaigns MUST be tenant-scoped and isolated.

#### Open and click tracking

- **FR-029**: For each sent message, the platform MUST embed a tracking pixel so
  that message opens can be recorded.
- **FR-030**: For each sent message, the platform MUST rewrite outbound links so
  that clicks can be recorded before the recipient is forwarded to the original
  destination.
- **FR-031**: Tracking endpoints MUST resolve the originating tenant, campaign,
  and subscriber from the tracking identifier alone, without requiring the
  recipient to be authenticated.
- **FR-032**: Recorded open and click events MUST be attributed to the correct
  tenant and isolated from other tenants.

#### Transactional API

- **FR-033**: The platform MUST expose an API endpoint for sending a single
  transactional message immediately.
- **FR-034**: The transactional endpoint MUST be authenticated by a tenant-scoped
  API key, and MUST reject missing, invalid, or wrongly scoped keys without
  sending.
- **FR-035**: A transactional send MUST reference a transactional template and
  supply the recipient and variable data needed to render it.
- **FR-036**: Every transactional send MUST be counted toward the tenant's usage,
  consistent with how campaign sends are counted.
- **FR-037**: A transactional request referencing a non-existent template or an
  unverified domain MUST be rejected with a clear, actionable error.

### Key Entities

- **Sending Domain**: A domain a tenant wants to send mail from. Has a name, a
  verification status (pending / verified / failed), the DNS records the tenant
  must publish, a reference to the provisioned identity at the mail provider, and
  belongs to exactly one tenant.
- **Template**: Reusable message content, optionally typed as campaign or
  transactional, owned by one tenant.
- **Campaign**: A message authored by a tenant, optionally derived from a
  template, targeted at one or more lists or segments, sendable from a verified
  domain, with a lifecycle state (draft, running, paused, finished, cancelled)
  and send progress counts.
- **Send Job / Batch**: A unit of asynchronous sending work for a slice of a
  campaign's recipients, carrying the owning tenant's identity.
- **Tracking Link**: A rewritten outbound link that records a click and forwards
  to an original destination, identified so its tenant, campaign, and subscriber
  can be resolved.
- **Tracking Pixel**: A per-message tracking reference that records a message
  open, resolvable to its tenant, campaign, and subscriber.
- **API Key**: A tenant-scoped credential with defined scopes used to authenticate
  transactional send requests (established in Phase 2; consumed here).
- **Usage Event**: A record that a message was sent, attributed to a tenant and
  counted toward plan usage.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A tenant can add a sending domain and reach "verified" status with
  no manual intervention beyond publishing the provided DNS records.
- **SC-002**: A tenant can create a campaign, start it, and have it delivered to
  100% of targeted recipients, with each recipient receiving the message exactly
  once.
- **SC-003**: 100% of campaign messages are delivered with a working tracking
  pixel and rewritten click-tracking links.
- **SC-004**: Open and click events are correctly attributed to the right tenant,
  campaign, and subscriber in 100% of test cases, with zero cross-tenant
  attribution leakage.
- **SC-005**: When a worker process is stopped mid-campaign and restarted, the
  campaign resumes and completes with zero duplicate sends.
- **SC-006**: Per-tenant sending stays within the tenant's plan rate limit, and
  total sending stays within the global cap, even when multiple tenants send
  concurrently across multiple worker processes.
- **SC-007**: When multiple tenants run large campaigns at once, no tenant's send
  is starved — every tenant's campaign makes continuous progress.
- **SC-008**: A tenant developer can send a transactional message through the API
  in a single request, and the message is delivered through the tenant's verified
  domain.
- **SC-009**: Requests with missing, invalid, or wrongly scoped API keys are
  rejected 100% of the time with no message sent.
- **SC-010**: No tenant can see, send from, or attribute events to another
  tenant's domains, templates, campaigns, or tracking data in any test scenario.
- **SC-011**: A campaign that accumulates send errors beyond the configured
  threshold is automatically paused rather than continuing to fail silently.
- **SC-012**: The full automated test suite passes and database schema migrations
  apply cleanly, including integration coverage of domain verification, campaign
  sending, and rate limiting against real boundaries.

## Assumptions

- Phases 1 and 2 are complete and in place: tenant isolation, platform/tenant
  authentication, lists, subscribers, segments, roles, scoped API keys, and the
  tenant-plane schema with row-level isolation all already exist and are reused.
- The external mail provider is Yandex Postbox, accessed through its
  SES-compatible API; a usable Postbox account and credentials are available for
  development and integration testing.
- Domain verification depends on the tenant correctly publishing DNS records and
  on DNS propagation; verification timing is bounded by a maximum polling window
  after which a domain is marked failed.
- Per-tenant rate limits are derived from plan definitions; full billing, quota
  enforcement, and overage handling are out of scope for this phase and arrive in
  Phase 5. This phase only needs the rate-limit values, not enforcement of paid
  quotas.
- Bounce and complaint webhook ingestion, the suppression list, pre-send
  suppression checks, and campaign analytics dashboards are out of scope for this
  phase and are delivered in Phase 4.
- The visual email editor, A/B testing, campaign scheduling UI, and advanced
  segmentation are out of scope for this phase (Phase 7); this phase covers
  authoring campaigns and starting sends, not the full editing experience.
- Tracking events recorded in this phase are stored for later aggregation;
  building the analytics dashboards on top of them is Phase 4 work.
- Standard delivery expectations apply: campaign sends complete within a
  reasonable window subject to rate limits, and transactional sends are dispatched
  promptly on request.

## Dependencies

- Phase 1 (Tenancy Core) — tenant isolation and resolution.
- Phase 2 (Subscribers, Lists & Auth) — lists, subscribers, segments, RBAC, and
  scoped API keys.
- An external mail provider (Yandex Postbox) account reachable via its
  SES-compatible API.
- A shared coordination store for cross-process rate-limit counters.
- A durable, transactional background job queue.
