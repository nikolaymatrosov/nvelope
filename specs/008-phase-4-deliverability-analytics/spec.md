# Feature Specification: Phase 4 — Deliverability & Analytics

**Feature Branch**: `008-phase-4-deliverability-analytics`

**Created**: 2026-05-18

**Status**: Draft

**Input**: User description: "Phase 4 — Deliverability & Analytics: Postbox bounce/complaint webhook ingestion with signature verification; suppression_list, configurable bounce actions, and pre-send suppression checks; campaign analytics (opens/clicks/bounces/complaints) and dashboard materialized views. Exit criteria: bounces/complaints are attributed and suppressed automatically; analytics and dashboard render. (Completes Epic F.)"

## User Scenarios & Testing *(mandatory)*

<!--
  Phase 4 builds directly on the Phase 3 sending pipeline. By now a tenant can
  verify a domain and send a campaign or transactional message through the mail
  provider (Yandex Postbox), with open-pixel and click-tracking already in place.

  What Phase 4 adds is the feedback loop. Today, when a message bounces or a
  recipient marks it as spam, the platform never hears about it — it keeps mailing
  dead and hostile addresses, which silently destroys sender reputation and
  deliverability. Phase 4 closes that loop: it ingests delivery feedback from the
  mail provider, automatically stops mailing addresses that bounce or complain,
  and gives operators a dashboard that shows how each campaign actually performed.

  The "users" remain tenant operators — marketers and administrators who run a
  workspace. Every event, suppression entry, and analytics figure in this phase is
  confined to a single tenant; the Phase 1 isolation guarantee is a hard
  precondition. The mail provider is an external actor that delivers feedback by
  writing notifications to a Yandex Data Streams stream the platform consumes.
-->

### User Story 1 - Ingest and attribute delivery feedback (Priority: P1)

The mail provider records a notification whenever a message it sent bounces (the
recipient's mail server rejected it), generates a complaint (the recipient marked
it as spam), is delivered, or is opened or clicked. It writes these notifications
to a Yandex Data Streams stream. The platform runs a consumer that reads the
stream and records each event against the specific message, recipient, and
campaign (or transactional send) it relates to. Operators can see, per campaign
and per recipient, that a bounce or complaint occurred and what kind it was.

**Why this priority**: Without trustworthy, attributed feedback there is nothing
to suppress and nothing to report on — every other story in this phase depends on
it. It is also the first independently demonstrable slice: feedback arriving and
being correctly tied to a message is observable on its own.

**Independent Test**: Send a message through the pipeline to a known-bad address,
have the mail provider write a bounce notification to the stream, and confirm the
consumer reads it and attributes the event to the correct message, recipient, and
campaign. Separately, write a notification referencing an unknown message and
confirm it is stored as unattributed rather than discarded.

**Acceptance Scenarios**:

1. **Given** a message was sent for a campaign and later bounces, **When** the
   mail provider writes a bounce notification to the stream, **Then** the
   platform's consumer reads it, records a bounce event, and attributes it to the
   originating message, recipient, and campaign.
2. **Given** a recipient marks a sent message as spam, **When** the mail provider
   writes a complaint notification to the stream, **Then** the platform records a
   complaint event attributed to the originating message, recipient, and campaign.
3. **Given** the mail provider re-delivers a notification it already wrote (a
   duplicate, identified by its event id), **When** the consumer reads it,
   **Then** the platform records the event only once.
4. **Given** a notification references a message the platform cannot match to a
   known send, **When** the consumer reads it, **Then** the platform stores the
   event but flags it as unattributed rather than discarding it.

---

### User Story 2 - Automatic suppression and pre-send checks (Priority: P1)

When an address hard-bounces or a recipient complains, the platform adds that
address to the tenant's suppression list so it will not be mailed again. Before
any future campaign or transactional message goes out, the platform checks every
intended recipient against the suppression list and skips the suppressed ones.
Operators can view the suppression list, see why each address was suppressed, and
manually add or remove entries. Operators can also configure how the platform
reacts to bounces — whether hard-bounce and complaint suppression are enabled.

**Why this priority**: Suppression is the deliverability protection the phase
exists to deliver. Continuing to mail bouncing and complaining addresses is what
damages sender reputation, so automatic suppression must ship alongside ingestion
for the phase to provide real value.

**Independent Test**: Trigger a hard bounce for an address, confirm it appears on
the tenant's suppression list with the correct reason, then start a new campaign
that includes that address and confirm the message is skipped and the skip is
recorded. Manually remove the entry and confirm the address becomes mailable
again.

**Acceptance Scenarios**:

1. **Given** a bounce event classified as a hard (permanent) bounce, **When** it
   is ingested, **Then** the bounced address is added to the tenant's suppression
   list with reason "hard bounce" and the originating event recorded.
2. **Given** a complaint event, **When** it is ingested, **Then** the complaining
   address is added to the tenant's suppression list with reason "complaint".
3. **Given** a campaign about to send to a list that includes a suppressed
   address, **When** the send is prepared, **Then** the suppressed recipient is
   skipped, no message is dispatched to it, and the skip is recorded with the
   suppression reason.
4. **Given** a suppressed address, **When** an operator removes it from the
   suppression list, **Then** the address becomes eligible for future sends again.
5. **Given** an operator wants to pre-empt mailing a known-bad address, **When**
   they manually add an address to the suppression list, **Then** that address is
   skipped on all future sends until removed.
6. **Given** a tenant has changed the bounce-action configuration, **When** later
   bounces are ingested, **Then** suppression behavior follows the updated
   configuration.

---

### User Story 3 - Campaign analytics and dashboard (Priority: P2)

Operators open a campaign and see how it performed: how many messages were sent,
delivered, opened, and clicked, and how many bounced or generated complaints,
expressed both as counts and as rates. A workspace-level dashboard summarizes
recent campaign activity and overall deliverability health. These figures stay
responsive even as event volume grows, because they are served from pre-computed
summaries rather than recomputed from raw events on every view.

**Why this priority**: Analytics turn the feedback loop into something operators
can act on, but the suppression behavior in Stories 1 and 2 protects
deliverability even before the dashboard exists, so reporting is valuable yet not
the blocking capability.

**Independent Test**: Run a campaign that produces a mix of opens, clicks,
bounces, and complaints, then open the campaign analytics view and confirm the
counts and rates match the underlying events. Open the workspace dashboard and
confirm it summarizes that campaign. Confirm the views render quickly for a
campaign with a large recipient count.

**Acceptance Scenarios**:

1. **Given** a campaign that has been sent and has accumulated open, click,
   bounce, and complaint events, **When** an operator opens the campaign analytics
   view, **Then** they see sent, delivered, opened, clicked, bounced, and
   complained counts plus the corresponding rates.
2. **Given** several campaigns sent recently, **When** an operator opens the
   workspace dashboard, **Then** they see a summary of recent campaign activity
   and overall deliverability health.
3. **Given** new events arrive after a campaign's figures were last computed,
   **When** the summaries are next refreshed, **Then** the analytics view reflects
   the new events within the expected refresh window.
4. **Given** a campaign with a large recipient count, **When** an operator opens
   its analytics view, **Then** the figures render within the expected response
   time without recomputing from raw events.
5. **Given** an operator of one tenant, **When** they view analytics, **Then**
   they see only their own tenant's campaigns and figures.

---

### Edge Cases

- A complaint or bounce arrives for a transactional (non-campaign) send — the
  event is attributed to the transactional message and the address is still
  suppressed, even though it has no campaign.
- An address bounces, is suppressed, then later the same recipient subscribes
  again or is re-imported — the suppression entry still applies until explicitly
  removed.
- The same address bounces under one tenant but is healthy under another — the
  suppression list is per-tenant, so it remains mailable for the other tenant.
- A notification arrives for a message sent before Phase 4 existed (no tracking
  context) — it is stored as an unattributed event.
- The stream carries a burst of notifications far above normal volume — the
  consumer keeps up and every event is processed without loss.
- The same notification is read more than once (the consumer re-reads after a
  restart, or the provider writes a duplicate) — it is de-duplicated by its event
  id and not double-counted.
- The stream also carries event types the platform does not act on (Send,
  DeliveryDelay, Unsubscribe) — the consumer reads past them without error.
- An open or click event arrives for a recipient who later complained — both
  events are counted; the complaint still drives suppression.
- A campaign analytics view is opened immediately after sending, before any
  feedback has arrived — it shows sent counts with zero opens/clicks/bounces.

## Clarifications

### Session 2026-05-18

- Q: Does Postbox deliver bounce/complaint feedback via an HTTP webhook or some
  other channel? → A: Postbox writes notifications to a Yandex Data Streams stream
  (YDB-style topic) as JSON; the platform runs a consumer that reads the stream.
  There is no inbound HTTP webhook.
- Q: How is notification authenticity established without a per-notification
  signature? → A: The stream is a trusted, access-controlled channel; the platform
  consumes it with its own IAM credentials. There is no per-record signature to
  verify — only the provider can write to the stream.
- Q: How should soft (transient) bounces be handled, given Postbox `Bounce`
  events are always `Permanent`? → A: Drop soft bounces from this phase. Only hard
  bounces and complaints drive suppression; there is no soft-bounce tally or
  threshold, and `DeliveryDelay` events are ignored.
- Q: Which source feeds campaign open/click analytics? → A: Switch to the stream's
  `Open` and `Click` events; Phase 3's tracking pixel and click-redirect are no
  longer the analytics source for opens and clicks.
- Q: Where does the long-lived stream consumer run? → A: A new dedicated
  `cmd/consumer` service.

## Requirements *(mandatory)*

### Functional Requirements

#### Feedback ingestion (4.1)

- **FR-001**: System MUST run a consumer that reads delivery-feedback
  notifications from the mail provider's Yandex Data Streams stream.
- **FR-002**: System MUST consume the stream over an authenticated,
  access-controlled channel using the platform's own credentials; the stream is
  a trusted source, so there is no per-notification signature to verify.
- **FR-003**: System MUST record each bounce event read from the stream. Every
  provider bounce is a permanent (hard) failure; there is no soft-bounce
  classification.
- **FR-004**: System MUST record each complaint event read from the stream.
- **FR-005**: System MUST attribute each event to the originating message and
  recipient, and to the originating campaign or transactional send where one
  exists.
- **FR-006**: System MUST store events that cannot be matched to a known send as
  unattributed rather than discarding them.
- **FR-007**: System MUST process re-read or duplicate notifications idempotently
  — identified by the provider event id — recording each distinct event only once.
- **FR-008**: System MUST confine every ingested event to the tenant that owns the
  originating send.
- **FR-009**: System MUST make unattributed events and consumer processing
  failures observable for monitoring.
- **FR-010**: System MUST track its position in the stream so that, after a
  restart, it resumes without losing or reprocessing-as-new any notifications.

#### Suppression (4.2)

- **FR-011**: System MUST maintain a per-tenant suppression list of email
  addresses that should not be mailed.
- **FR-012**: System MUST automatically add an address to the suppression list
  when it generates a bounce or a complaint, recording the reason and the
  originating event.
- **FR-013**: System MUST allow operators to configure bounce actions for their
  tenant — whether hard-bounce suppression and complaint suppression are each
  enabled.
- **FR-014**: System MUST check every intended recipient against the suppression
  list before sending, for both campaign and transactional sends.
- **FR-015**: System MUST skip suppressed recipients at send time without
  dispatching a message to them, and MUST record each skip with its suppression
  reason.
- **FR-016**: Operators MUST be able to view their tenant's suppression list,
  including each entry's address, reason, and the date it was suppressed.
- **FR-017**: Operators MUST be able to manually add an address to the suppression
  list.
- **FR-018**: Operators MUST be able to manually remove an address from the
  suppression list, after which it becomes eligible for future sends.
- **FR-019**: System MUST apply the suppression list independently per tenant, so
  an address suppressed for one tenant remains mailable for others.

#### Analytics & dashboard (4.3)

- **FR-020**: System MUST aggregate, per campaign, the counts of messages sent,
  delivered, opened, clicked, bounced, and complained.
- **FR-021**: System MUST express campaign performance as rates (e.g., open rate,
  click rate, bounce rate, complaint rate) in addition to raw counts.
- **FR-022**: System MUST provide a per-campaign analytics view presenting these
  counts and rates.
- **FR-023**: System MUST provide a workspace-level dashboard summarizing recent
  campaign activity and overall deliverability health.
- **FR-024**: System MUST serve analytics from pre-computed summaries that are
  refreshed periodically, rather than recomputing from raw events on each view.
- **FR-025**: System MUST refresh analytics summaries so that newly arrived events
  are reflected within the expected refresh window.
- **FR-026**: System MUST derive opened and clicked counts from the provider's
  `Open` and `Click` stream events, and delivered counts from `Delivery` stream
  events.
- **FR-027**: System MUST scope all analytics views and figures to the requesting
  operator's tenant.

### Key Entities *(include if feature involves data)*

- **Delivery event**: A single piece of feedback about a sent message — a bounce,
  a complaint, a delivery, an open, or a click — read from the provider stream.
  Holds the event type, the affected email address, the time it occurred, the
  attributed message, recipient, and campaign or transactional send, an
  attributed/unattributed flag, and the provider event id used for de-duplication.
- **Suppression entry**: An email address that must not be mailed for a given
  tenant. Holds the tenant, the address, the suppression reason (hard bounce,
  complaint, manual), the date suppressed, and a reference to the originating
  event where applicable.
- **Bounce-action configuration**: Per-tenant settings governing suppression
  behavior — toggles for hard-bounce and complaint suppression.
- **Campaign analytics summary**: A pre-computed, per-campaign roll-up of sent,
  delivered, opened, clicked, bounced, and complained counts and derived rates,
  used to serve the analytics view and dashboard without recomputation.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 100% of bounce and complaint notifications written to the stream
  are read and recorded; the consumer loses no notifications across restarts.
- **SC-002**: At least 99% of bounce and complaint events are attributed to the
  correct originating message, recipient, and campaign or transactional send.
- **SC-003**: Duplicate notifications never produce a duplicate event — each
  distinct event is counted exactly once.
- **SC-004**: An address that hard-bounces or complains appears on the tenant's
  suppression list and is excluded from sends within one send cycle of the event
  being ingested.
- **SC-005**: No message is dispatched to a suppressed address; every suppressed
  recipient in a send is skipped and the skip is recorded.
- **SC-006**: Campaign analytics counts and rates match the underlying recorded
  events exactly when the summaries are up to date.
- **SC-007**: A campaign analytics view renders in under 2 seconds even for a
  campaign with 100,000 recipients.
- **SC-008**: Newly arrived events are reflected in analytics within 5 minutes.
- **SC-009**: Operators only ever see suppression entries, events, and analytics
  belonging to their own tenant.
- **SC-010**: The stream consumer keeps pace with notification volume under
  normal and burst load, so the backlog of unread notifications stays bounded.

## Assumptions

- The mail provider (Yandex Postbox) delivers all delivery feedback — bounces,
  complaints, deliveries, opens, and clicks — by writing JSON notifications to a
  Yandex Data Streams stream. The platform consumes the stream with its own
  credentials; the stream is a trusted, access-controlled channel, so there is no
  per-notification signature to verify.
- The notification format follows the documented Postbox schema: a top-level
  `eventType`, a `mail` object carrying the provider `messageId`, and an
  `eventId` used as the de-duplication key. Provider `Bounce` events are always
  permanent (hard); transient delays arrive as separate `DeliveryDelay` events,
  which this phase ignores.
- Open and click events come from the provider stream's `Open` and `Click`
  notifications; Phase 4 sources analytics opens/clicks from the stream rather
  than from Phase 3's tracking pixel and click-redirect.
- Suppression lists are per-tenant, consistent with the Phase 1 tenant-isolation
  guarantee; there is no cross-tenant or global shared suppression list.
- Every provider bounce and every complaint triggers immediate suppression when
  the tenant's bounce-action toggles allow it; there is no soft-bounce tally or
  threshold in this phase.
- Analytics summaries are refreshed on a periodic schedule (near-real-time, within
  minutes) rather than synchronously on every event; brief lag between an event
  and its appearance in analytics is acceptable.
- "Delivered" counts are derived from the provider's `Delivery` stream events.
- The transactional sending API and campaign batch pipeline from Phase 3 are the
  send paths that pre-send suppression checks integrate into.
- Feedback handling is asynchronous: the stream consumer reads notifications and
  enqueues them quickly, and attribution/suppression work is processed by
  background jobs.
- This phase completes Epic F; no further deliverability scope (e.g., reputation
  scoring or provider failover) is included here.
