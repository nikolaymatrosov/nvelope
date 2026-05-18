# Feature Specification: Phase 4 — Deliverability & Analytics — Frontend UI

**Feature Branch**: `009-phase-4-ui`

**Created**: 2026-05-18

**Status**: Draft

**Input**: User description: "I want you to add UI for the features implemented in the prev spec" — i.e. Phase 4 — Deliverability & Analytics (008): delivery-feedback ingestion, per-tenant suppression list with configurable bounce actions, and campaign analytics with a workspace dashboard.

## User Scenarios & Testing *(mandatory)*

<!--
  Phase 4 (Deliverability & Analytics) is already implemented in the backend: the
  platform ingests bounce, complaint, delivery, open, and click feedback from the
  mail provider; automatically suppresses bouncing and complaining addresses;
  checks recipients against a per-tenant suppression list before every send; and
  rolls up per-campaign analytics into pre-computed summaries and a workspace
  deliverability view. Today none of this is visible to operators — there is no
  web interface for the suppression list, bounce-action configuration, campaign
  analytics, or the workspace dashboard. This feature delivers that interface.

  It extends the existing tenant workspace app shell (the persistent sidebar
  layout from the Phase 1 & 2 UI) and sits alongside the Phase 3 UI for sending
  domains and campaigns. The "users" are tenant operators — marketers and
  administrators inside a workspace.

  Backend constraints reflected by this UI: every suppression entry, event, and
  analytics figure is confined to a single tenant; access is gated by the
  existing permission strings (e.g. `sending:*`, `campaigns:*`); analytics are
  served from periodically refreshed summaries, so figures may lag live events by
  a few minutes; the feedback consumer itself is a background service with no
  operator-facing controls.
-->

### User Story 1 - View campaign analytics (Priority: P1)

A tenant operator opens a campaign they have sent and sees how it performed: the
number of messages sent, delivered, opened, clicked, bounced, and generated
complaints, shown both as counts and as rates (open rate, click rate, bounce
rate, complaint rate). The view indicates when the figures were last refreshed,
so the operator understands the figures are near-real-time rather than live. A
campaign opened immediately after sending — before any feedback has arrived —
shows its sent count with zeroes for the rest and a clear "awaiting data" state.

**Why this priority**: Analytics is the visible payoff of the whole phase — it
turns the feedback loop into something an operator can read and act on. It is
the first independently demonstrable slice of this UI: open a sent campaign and
see real numbers.

**Independent Test**: As an operator with a campaign that has accumulated a mix
of opens, clicks, bounces, and complaints, open the campaign analytics view and
confirm every count and rate matches the underlying activity. Open a
just-sent campaign and confirm it shows the sent count with zeroes and an
awaiting-data state.

**Acceptance Scenarios**:

1. **Given** a sent campaign with accumulated open, click, bounce, and complaint
   activity, **When** the operator opens its analytics view, **Then** they see
   sent, delivered, opened, clicked, bounced, and complained counts plus open,
   click, bounce, and complaint rates.
2. **Given** a campaign analytics view, **When** it is displayed, **Then** it
   shows when the figures were last refreshed, or indicates the figures have not
   yet been computed.
3. **Given** a campaign opened immediately after sending with no feedback yet,
   **When** the operator views its analytics, **Then** the sent count is shown,
   the remaining figures are zero, and an explicit "awaiting data" state is
   presented rather than an error or blank screen.
4. **Given** new events have arrived since the figures were last refreshed,
   **When** the summaries are next refreshed and the operator reopens or
   refreshes the view, **Then** the updated figures are shown.
5. **Given** an operator of one tenant, **When** they view campaign analytics,
   **Then** they only ever see their own tenant's campaigns and figures.

---

### User Story 2 - Manage the suppression list (Priority: P1)

A tenant operator opens a "Suppression list" area in the workspace and sees every
address that will not be mailed, each row showing the address, why it was
suppressed (hard bounce, complaint, or a manual entry), and the date it was
suppressed. The operator can filter the list by reason and search by address.
They can manually add an address — to pre-empt mailing a known-bad recipient —
and can remove an address, after which it becomes mailable again. Removal asks
for confirmation, since it re-enables sending to that address.

**Why this priority**: The suppression list is the operator's window into the
deliverability protection the phase delivers, and the only place they can
intervene — adding a known-bad address ahead of time or correcting an
over-suppression. It is independently demonstrable: view the list, add an
address, remove it.

**Independent Test**: As an operator, open the suppression list and confirm
addresses suppressed by bounces and complaints appear with the correct reason
and date. Add an address manually and confirm it appears with reason "manual".
Remove an entry, confirm the confirmation prompt, and confirm the address leaves
the list. Filter by reason and search by address and confirm the list narrows.

**Acceptance Scenarios**:

1. **Given** a tenant with suppressed addresses, **When** the operator opens the
   suppression list, **Then** each entry shows its address, suppression reason,
   and suppression date.
2. **Given** a tenant with no suppressed addresses, **When** the operator opens
   the suppression list, **Then** an explicit empty state is shown.
3. **Given** the suppression list, **When** the operator filters by a reason or
   searches by an address fragment, **Then** the list narrows to matching
   entries.
4. **Given** the add-address action, **When** the operator submits a valid email
   address, **Then** it is added to the list with reason "manual" and appears in
   the list.
5. **Given** the add-address action, **When** the operator submits an invalid
   email address, **Then** an inline validation message is shown and nothing is
   added.
6. **Given** an existing suppression entry, **When** the operator removes it and
   confirms the prompt, **Then** the entry leaves the list and the address
   becomes mailable again.
7. **Given** a long suppression list, **When** the operator scrolls or pages
   through it, **Then** further entries load without losing the current filter or
   search.

---

### User Story 3 - Workspace deliverability dashboard (Priority: P2)

A tenant operator opens a workspace dashboard that summarizes recent sending
health: aggregate counts of messages sent, delivered, opened, clicked, bounced,
and complained across the workspace, the overall bounce and complaint rates, and
a list of the most recently sent campaigns with each one's sent count, open
rate, bounce rate, and complaint rate. From a campaign row the operator can open
that campaign's full analytics view.

**Why this priority**: The dashboard gives operators an at-a-glance read on
deliverability health and a launch point into individual campaigns, but the
per-campaign analytics in Story 1 already deliver the core reporting value, so
the dashboard is valuable yet not the blocking capability.

**Independent Test**: As an operator with several recently sent campaigns, open
the workspace dashboard and confirm the aggregate counts, the overall bounce and
complaint rates, and the recent-campaign list all reflect the underlying
activity. Click a recent campaign and confirm it opens that campaign's analytics
view.

**Acceptance Scenarios**:

1. **Given** a workspace with recent sending activity, **When** the operator
   opens the dashboard, **Then** they see aggregate sent, delivered, opened,
   clicked, bounced, and complained counts and the overall bounce and complaint
   rates.
2. **Given** the dashboard, **When** it is displayed, **Then** it lists the most
   recently sent campaigns, each with its sent count, open rate, bounce rate, and
   complaint rate, ordered with the most recent first.
3. **Given** the recent-campaign list, **When** the operator selects a campaign,
   **Then** that campaign's full analytics view opens.
4. **Given** a workspace with no sending activity yet, **When** the operator
   opens the dashboard, **Then** an explicit empty state is shown instead of a
   grid of zeroes presented as real results.
5. **Given** an operator of one tenant, **When** they view the dashboard,
   **Then** all figures and campaigns shown belong only to their own tenant.

---

### User Story 4 - Configure bounce actions (Priority: P3)

A tenant operator opens a settings area and sees two toggles controlling how the
platform reacts to delivery feedback: whether a hard bounce automatically
suppresses the address, and whether a complaint automatically suppresses the
address. Both default to on. The operator can change either toggle and save; the
new configuration governs how later bounces and complaints are handled. The
interface explains what each toggle does so the operator understands the
deliverability trade-off of turning it off.

**Why this priority**: The defaults (both toggles on) are the safe, recommended
configuration and serve the vast majority of tenants without any operator
action, so exposing the toggles is a refinement rather than a launch
requirement.

**Independent Test**: As an operator, open the bounce-action settings, confirm
both toggles default to on, turn one off and save, reload the page, and confirm
the change persisted.

**Acceptance Scenarios**:

1. **Given** a tenant that has never changed its bounce-action configuration,
   **When** the operator opens the settings, **Then** both the hard-bounce and
   complaint suppression toggles are shown as on.
2. **Given** the bounce-action settings, **When** the operator changes a toggle
   and saves, **Then** the change is confirmed and persists across reloads.
3. **Given** each toggle, **When** the operator views it, **Then** an explanation
   of what it does and the deliverability consequence of disabling it is shown.

---

### Edge Cases

- A campaign analytics view is opened for a campaign that has never been sent
  (still a draft) — the view either is unavailable or shows an explicit
  not-yet-sent state rather than zeroes presented as results.
- A campaign whose analytics summary has never been refreshed — the view shows
  the sent count with zeroes elsewhere and an "awaiting data" indication, and
  states the figures have not yet been computed.
- An operator opens analytics for a campaign id that does not exist or belongs to
  another tenant — a not-found state is shown, never another tenant's data.
- The operator removes a suppression entry that another session already removed —
  the interface reconciles to the current state without a hard error.
- The operator manually adds an address that is already on the suppression list —
  the action succeeds idempotently and the existing entry remains.
- A bounce or complaint rate is computed against a zero denominator (nothing
  delivered or sent yet) — the rate is shown as 0% rather than an error or a
  blank.
- An operator without the relevant permission opens the workspace — the
  suppression list, dashboard, analytics, and bounce-action settings entries are
  hidden or disabled, consistent with the Phase 3 UI permission gating.
- The suppression list or recent-campaign list is very long — it pages or loads
  incrementally without freezing or losing the active filter.

## Requirements *(mandatory)*

### Functional Requirements

#### Campaign analytics view

- **FR-001**: The interface MUST provide a per-campaign analytics view, reachable
  from a sent campaign, presenting the campaign's sent, delivered, opened,
  clicked, bounced, and complained counts.
- **FR-002**: The analytics view MUST present open rate, click rate, bounce rate,
  and complaint rate alongside the raw counts.
- **FR-003**: The analytics view MUST display when the figures were last
  refreshed, and MUST indicate when figures have not yet been computed.
- **FR-004**: The analytics view MUST show a campaign with no feedback yet as its
  sent count plus zeroes, with an explicit "awaiting data" state, rather than an
  error or blank screen.
- **FR-005**: The analytics view MUST present a 0% rate (not an error or blank)
  when a rate is computed against a zero denominator.
- **FR-006**: The interface MUST show a not-found state when analytics are
  requested for a campaign that does not exist within the operator's tenant, and
  MUST never display another tenant's analytics.

#### Suppression list management

- **FR-007**: The interface MUST provide a suppression-list area listing the
  tenant's suppressed addresses, each row showing the address, the suppression
  reason (hard bounce, complaint, or manual), and the date suppressed.
- **FR-008**: The suppression list MUST present an explicit empty state when the
  tenant has no suppressed addresses.
- **FR-009**: The interface MUST let operators filter the suppression list by
  reason and search it by an address fragment.
- **FR-010**: The interface MUST let operators manually add an email address to
  the suppression list, recording it with reason "manual".
- **FR-011**: The interface MUST validate the email address on manual add and
  show an inline validation message for an invalid address without adding
  anything.
- **FR-012**: The interface MUST let operators remove an address from the
  suppression list, and MUST require a confirmation step before removal because
  removal re-enables sending to that address.
- **FR-013**: The interface MUST load long suppression lists incrementally
  (paging or infinite scroll) without losing the active filter or search.
- **FR-014**: The interface MUST reconcile to the current server state without a
  hard error when an add or remove conflicts with a concurrent change (e.g. the
  entry was already removed, or the address is already suppressed).

#### Workspace deliverability dashboard

- **FR-015**: The interface MUST provide a workspace dashboard presenting
  aggregate sent, delivered, opened, clicked, bounced, and complained counts and
  the overall bounce and complaint rates for the tenant.
- **FR-016**: The dashboard MUST list the most recently sent campaigns, each with
  its sent count, open rate, bounce rate, and complaint rate, ordered most-recent
  first.
- **FR-017**: The dashboard MUST let operators open a listed campaign's full
  analytics view from its row.
- **FR-018**: The dashboard MUST present an explicit empty state when the tenant
  has no sending activity yet.

#### Bounce-action settings

- **FR-019**: The interface MUST provide a settings area with toggles for
  hard-bounce suppression and complaint suppression, showing both as on when the
  tenant has not changed the configuration.
- **FR-020**: The interface MUST let operators change either toggle and save, and
  MUST persist the change across reloads.
- **FR-021**: The settings area MUST explain what each toggle does and the
  deliverability consequence of disabling it.

#### Cross-cutting

- **FR-022**: The interface MUST scope every figure, suppression entry, and
  campaign shown to the operator's current tenant.
- **FR-023**: The interface MUST hide or disable the suppression list, dashboard,
  campaign analytics, and bounce-action settings entries for operators who lack
  the relevant permission, consistent with the Phase 3 UI permission gating.
- **FR-024**: Every new area MUST live within the existing tenant workspace app
  shell (persistent sidebar layout) and follow the navigation, loading, empty,
  and error-state conventions established by the Phase 1–3 UI.
- **FR-025**: The interface MUST present clear, actionable messages for failed
  operations (e.g. add, remove, save) rather than silent failures.

### Key Entities *(include if feature involves data)*

- **Suppression entry (as shown)**: A suppressed email address presented to the
  operator with its address, reason (hard bounce, complaint, manual), and the
  date it was suppressed.
- **Campaign analytics (as shown)**: A per-campaign roll-up presented to the
  operator — sent, delivered, opened, clicked, bounced, and complained counts;
  open, click, bounce, and complaint rates; and the time the figures were last
  refreshed.
- **Workspace deliverability summary (as shown)**: Aggregate counts and overall
  bounce/complaint rates for the tenant, plus a list of recent campaigns each
  with a sent count and key rates.
- **Bounce-action configuration (as shown)**: The two operator-facing toggles —
  hard-bounce suppression and complaint suppression.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: An operator can open a sent campaign's analytics view and read its
  counts and rates without consulting any other tool or page.
- **SC-002**: A campaign analytics view renders within 2 seconds even for a
  campaign with 100,000 recipients.
- **SC-003**: An operator can locate a specific suppressed address using the
  filter and search controls in under 30 seconds, regardless of list size.
- **SC-004**: An operator can manually add an address to the suppression list in
  under 1 minute, and the address is excluded from subsequent sends.
- **SC-005**: 100% of suppression removals require an explicit confirmation step
  before taking effect.
- **SC-006**: An operator can determine overall workspace deliverability health
  (bounce and complaint rates) within 10 seconds of opening the dashboard.
- **SC-007**: Operators only ever see suppression entries, analytics, and
  campaigns belonging to their own tenant.
- **SC-008**: Newly refreshed analytics figures appear in the interface the next
  time an operator opens or refreshes the relevant view, with the refresh time
  clearly shown.
- **SC-009**: An operator can change and save bounce-action settings, and the
  change is still in effect after reloading the page.

## Assumptions

- The Phase 4 backend (008) is delivered and exposes the tenant-scoped HTTP
  endpoints for the suppression list, bounce-action settings, per-campaign
  analytics, and the workspace dashboard; this feature builds the UI on top of
  those endpoints and adds no new backend capability.
- The interface extends the existing tenant workspace web application and its
  sidebar app shell, reusing the navigation, permission-gating, loading, empty,
  and error-state patterns established by the Phase 1–3 UI.
- Analytics are served from periodically refreshed summaries; a lag of a few
  minutes between an event and its appearance in the UI is acceptable and is
  communicated to the operator via the displayed refresh time.
- The delivery-feedback consumer is a background service with no operator-facing
  controls; monitoring of unattributed events and consumer failures is an
  operations concern and is out of scope for this operator UI.
- Per-recipient delivery-event detail (showing an individual subscriber's bounce
  or complaint history) is out of scope for this phase; operators see suppression
  outcomes via the suppression list and aggregate outcomes via analytics.
- Campaign analytics are reached from the existing campaign views delivered by
  the Phase 3 UI; this feature adds the analytics view but does not redesign the
  campaign list or detail pages.
- Desktop web is the primary target, consistent with the existing workspace UI;
  a dedicated mobile layout is out of scope.
- Rates are displayed as percentages; a zero denominator yields 0%.
