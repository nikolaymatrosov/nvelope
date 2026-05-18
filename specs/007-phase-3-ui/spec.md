# Feature Specification: Phase 3 — Sending Pipeline — Frontend UI

**Feature Branch**: `007-phase-3-ui`

**Created**: 2026-05-18

**Status**: Draft

**Input**: User description: "I want UI for the phase — Phase 3 Sending Pipeline (delivered): River job queue; sending_domains schema with Postbox domain provisioning and domain.verify polling; Postbox SES-compatible messenger; Redis-coordinated rate limiting; templates and campaigns schema with the campaign.start → campaign.batch send pipeline and open-pixel / click-tracking links; transactional tx API endpoint authenticated by a scoped API key."

## User Scenarios & Testing *(mandatory)*

<!--
  Phase 3 (Sending Pipeline) is already implemented in the backend: a tenant can
  verify a sending domain, author templates and campaigns, send a campaign
  through Yandex Postbox with open/click tracking, and send transactional mail
  through a scoped-API-key endpoint. Today there is no web interface for any of
  it. This feature delivers that interface.

  It extends the existing tenant workspace app shell delivered by the Phase 1 & 2
  UI (the persistent sidebar layout). The "users" here are tenant operators —
  marketers and administrators inside a workspace — and, for transactional
  sending, tenant developers who manage API keys and integrate the tx endpoint.

  Backend constraints reflected by this UI: campaigns and transactional sends
  require a verified sending domain; access is gated by the permission strings
  `sending:*`, `campaigns:*`, and `transactional:send`; bounce/complaint
  handling, suppression, the visual email editor, campaign scheduling, and
  open/click analytics dashboards are out of scope for this phase.
-->

### User Story 1 - Verify a sending domain (Priority: P1)

A tenant operator opens a "Sending domains" area in the workspace, adds the domain they want to send mail from, and is shown the exact DNS records (DKIM, SPF, DMARC) to publish at their DNS provider. Each record can be copied with one action. The domain appears in a list with a clear status — pending, verified, or failed. The operator can trigger an immediate re-check, and the status updates on its own as the platform's periodic verification runs. A failed domain shows an actionable reason.

**Why this priority**: No campaign and no transactional message can be sent without a verified sending domain. This is the gate for everything else in the phase and the first independently shippable slice of the UI.

**Independent Test**: As an operator in a workspace, open the sending domains area, add a domain, confirm the DNS records are displayed and copyable, publish them on a real test domain, trigger a re-check, and confirm the domain reaches "verified" without further action. Confirm a failed domain shows a reason.

**Acceptance Scenarios**:

1. **Given** an operator in a workspace with no sending domains, **When** they open the sending domains area, **Then** an explicit empty state with an "add domain" action is shown.
2. **Given** the add-domain form, **When** the operator submits a domain name, **Then** the domain is created in "pending" status and its DKIM, SPF, and DMARC DNS records are displayed, each individually copyable.
3. **Given** a domain in "pending" status, **When** the operator triggers an immediate re-check, **Then** the action shows a busy state and the latest status is reflected on completion.
4. **Given** a domain whose DNS records have been published correctly, **When** the platform's periodic verification runs, **Then** the domain's status updates to "verified" in the interface without the operator reloading or re-checking manually.
5. **Given** a domain in "failed" status, **When** the operator views it, **Then** an actionable failure reason is shown.
6. **Given** an operator without the sending-domains permission, **When** they open the workspace, **Then** the sending domains navigation entry and add-domain action are hidden or disabled.

---

### User Story 2 - Author, send, and monitor a campaign (Priority: P1)

A tenant operator opens a "Campaigns" area, creates a campaign, optionally starting from a reusable template, edits its subject and content, chooses a verified sending domain as the "from" address, and targets one or more lists or segments. They review a summary, then start the send. After starting, the campaign moves through its lifecycle (draft → running → paused/finished/cancelled) and the operator watches live send progress — counts of messages sent, failed, and remaining. They can pause, resume, or cancel a running campaign. The interface prevents starting a campaign that has no verified domain selected.

**Why this priority**: Authoring and sending a campaign with tracking is the core deliverable of the phase. It is independently demonstrable: create a campaign, start it, and watch it deliver.

**Independent Test**: As an operator with a verified domain and a list of test subscribers, create a campaign (optionally from a template), set its content, pick the verified domain, target the list, start it, and confirm send progress counts update toward completion. Confirm that a campaign without a verified domain cannot be started.

**Acceptance Scenarios**:

1. **Given** the campaigns area, **When** the operator creates a campaign, **Then** it appears in the campaigns list in "draft" status.
2. **Given** the campaign editor, **When** the operator chooses to start from an existing template, **Then** the campaign is pre-filled with the template's subject and content, which the operator can then override.
3. **Given** a draft campaign, **When** the operator selects a sending domain, **Then** only the workspace's verified domains are offered as choices.
4. **Given** a draft campaign, **When** the operator targets recipients, **Then** they can select one or more of the workspace's lists or segments.
5. **Given** a draft campaign that has no verified sending domain selected, **When** the operator attempts to start it, **Then** the start action is disabled or rejected with a clear reason.
6. **Given** a fully configured draft campaign, **When** the operator starts it, **Then** they are asked to confirm, and on confirmation the campaign moves to "running".
7. **Given** a running campaign, **When** the operator views it, **Then** live send progress is shown — messages sent, failed, and remaining — and updates without a manual reload.
8. **Given** a running campaign, **When** the operator pauses, resumes, or cancels it, **Then** the campaign's status changes accordingly and the available actions update to match the new status.
9. **Given** a campaign auto-paused by the backend after accumulating send errors, **When** the operator views it, **Then** the "paused" status and the reason are shown with the option to resume or cancel.
10. **Given** an operator without the campaigns permission, **When** they open the workspace, **Then** the campaigns navigation entry and authoring actions are hidden or disabled.

---

### User Story 3 - Manage reusable templates (Priority: P2)

A tenant operator opens a "Templates" area, creates templates with a name, subject, and content, and marks each as intended for campaign use or transactional use. They edit and delete existing templates. Campaign templates are offered when authoring a campaign; transactional templates are referenced by the transactional sending integration.

**Why this priority**: Templates make campaign and transactional authoring reusable, but a campaign can be authored without one, so this follows the core send flow. It is independently testable in its own area.

**Independent Test**: As an operator, create a campaign template and a transactional template, edit one, delete one, and confirm the campaign template appears as a starting point in the campaign editor and the transactional template appears as a reference choice for transactional sending.

**Acceptance Scenarios**:

1. **Given** the templates area with no templates, **When** the operator opens it, **Then** an explicit empty state with a "create template" action is shown.
2. **Given** the create-template form, **When** the operator supplies a name, subject, content, and a type (campaign or transactional), **Then** the template is created and listed.
3. **Given** an existing template, **When** the operator edits its name, subject, or content, **Then** the changes are persisted and shown on the next view.
4. **Given** an existing template, **When** the operator deletes it after confirming, **Then** it is removed from the list.
5. **Given** a campaign template exists, **When** the operator authors a campaign, **Then** that template is offered as a starting point.
6. **Given** an operator without the campaigns permission, **When** they open the workspace, **Then** template authoring actions are hidden or disabled.

---

### User Story 4 - Set up transactional sending via API key (Priority: P3)

A workspace administrator prepares the workspace for transactional email integration. They issue an API key scoped for transactional sending (`transactional:send`), see the key secret exactly once with a non-retrievable warning, and revoke keys that are no longer needed. The interface shows the transactional endpoint details a developer needs — endpoint address, required transactional template, and the recipient/variable payload shape — so the integration can be wired up outside the UI.

**Why this priority**: Transactional sending is a developer integration surface; the UI's role is limited to issuing the scoped credential and surfacing integration details. It depends on verified domains and transactional templates already existing, so it follows them.

**Independent Test**: As an administrator, issue an API key scoped for transactional sending, confirm the secret is shown once with a warning, revoke it, and confirm it disappears from the active keys list. Confirm the transactional endpoint and payload details are presented for reference.

**Acceptance Scenarios**:

1. **Given** an administrator, **When** they issue an API key and select the transactional-sending scope, **Then** the key is created and its secret is displayed exactly once with a clear non-retrievable warning.
2. **Given** an active transactional API key, **When** the administrator revokes it, **Then** it is removed from the active keys list.
3. **Given** the transactional sending area, **When** an administrator or developer views it, **Then** the endpoint address, the requirement to reference a transactional template, and the recipient/variable payload shape are presented for reference.
4. **Given** a workspace with no verified sending domain, **When** the transactional sending area is viewed, **Then** the interface explains that a verified domain is required before transactional sends will succeed.
5. **Given** a user without permission to manage API keys, **When** they open the transactional sending area, **Then** key issuance and revocation controls are unavailable and the restriction is explained.

---

### Edge Cases

- A domain verification status changes (pending → verified/failed) while the operator is viewing the domains list — the view reflects the new status without a manual reload.
- An operator opens a running campaign, navigates away, and returns — current send progress and status are re-fetched and displayed.
- A campaign's selected sending domain loses verification before the campaign is started — the interface blocks the start and explains why.
- A campaign targets lists that are empty at start time — the campaign completes immediately with zero messages and a clear final state shown.
- A backend request fails or returns an error (e.g., provider temporarily unavailable when adding a domain) — a readable, non-technical message is shown with a retry path; the screen never goes blank.
- A session expires mid-task — the interface routes the user to sign in without losing context where reasonable.
- A user with reduced permissions opens a deep link to the campaigns, domains, or transactional area — the interface explains the restriction rather than failing silently.
- The API key secret dialog is closed before the operator copies the secret — the interface makes clear the secret cannot be retrieved again and the key must be revoked and reissued.
- A campaign is paused, resumed, or cancelled by another operator while a teammate views it — the stale view recovers gracefully on the next action or refresh.

## Requirements *(mandatory)*

### Functional Requirements

#### Workspace shell & navigation

- **FR-001**: The workspace sidebar app shell MUST be extended with navigation entries for Sending Domains, Templates, Campaigns, and Transactional Sending.
- **FR-002**: Each new navigation entry and its in-page actions MUST be hidden or disabled when the current user's permissions (`sending:*`, `campaigns:*`, `transactional:send`, or API-key management) do not allow the corresponding capability.
- **FR-003**: The new screens MUST use the same shared visual design system, component set, and loading/empty/error/populated state conventions as the existing Phase 1 & 2 UI.

#### Sending domains

- **FR-004**: The interface MUST let permitted operators add a sending domain by name and MUST list the workspace's sending domains with their status (pending, verified, failed).
- **FR-005**: For a registered domain, the interface MUST display the DKIM, SPF, and DMARC DNS records to publish, each individually copyable.
- **FR-006**: The interface MUST let an operator trigger an immediate verification re-check for a pending domain, showing a busy state during the request.
- **FR-007**: The interface MUST reflect domain status changes driven by the platform's periodic verification without requiring a manual page reload.
- **FR-008**: For a failed domain, the interface MUST display an actionable failure reason.

#### Templates

- **FR-009**: The interface MUST let permitted operators create, view, edit, and delete templates, with delete requiring explicit confirmation.
- **FR-010**: A template MUST carry a name, subject, content, and a type indicating campaign or transactional use, all editable in the interface.

#### Campaigns

- **FR-011**: The interface MUST let permitted operators create a campaign and list the workspace's campaigns with their lifecycle status (draft, running, paused, finished, cancelled).
- **FR-012**: The campaign editor MUST let the operator optionally start from a campaign template, pre-filling subject and content while allowing override.
- **FR-013**: The campaign editor MUST let the operator edit the campaign's subject and content.
- **FR-014**: The campaign editor MUST let the operator select a sending domain, offering only the workspace's verified domains.
- **FR-015**: The campaign editor MUST let the operator target one or more lists or segments as recipients.
- **FR-016**: The interface MUST prevent starting a campaign that has no verified sending domain selected, with a clear explanation.
- **FR-017**: Starting a campaign MUST require an explicit confirmation step.
- **FR-018**: For a running campaign, the interface MUST display live send progress — counts of messages sent, failed, and remaining — updating without a manual reload.
- **FR-019**: The interface MUST let the operator pause, resume, and cancel a campaign as permitted by its current status, updating the available actions to match the new status.
- **FR-020**: The interface MUST surface a campaign auto-paused by the backend (after accumulating send errors) with its paused status and reason.

#### Transactional sending

- **FR-021**: The interface MUST let an administrator issue an API key scoped for transactional sending and display the key secret exactly once with a clear non-retrievable warning.
- **FR-022**: The interface MUST let an administrator revoke an active transactional API key and remove it from the active keys list.
- **FR-023**: The transactional sending area MUST present the endpoint address, the transactional-template requirement, and the recipient/variable payload shape for developer reference.
- **FR-024**: When the workspace has no verified sending domain, the transactional sending area MUST explain that a verified domain is required before transactional sends will succeed.

#### Cross-cutting

- **FR-025**: All destructive actions (deleting a template, revoking an API key, cancelling a campaign) MUST require explicit confirmation before proceeding.
- **FR-026**: All forms MUST show inline validation, a busy/disabled state during submission, and readable, non-technical error messages on failure.
- **FR-027**: Every asynchronous data view MUST present distinct loading, empty, error, and populated states.
- **FR-028**: When the backend denies an action for lack of permission, the interface MUST show a clear authorization message and leave data unchanged.
- **FR-029**: When a request returns an unauthenticated response, the interface MUST route the user to sign in rather than showing a broken screen.

### Key Entities *(include if feature involves data)*

- **Sending domain**: A domain a tenant sends mail from — name, verification status (pending / verified / failed), the DKIM/SPF/DMARC DNS records to publish, and a failure reason when failed.
- **Template**: Reusable message content within a workspace — name, subject, content, and a type (campaign or transactional).
- **Campaign**: An authored message — name, subject, content, an optional source template, a selected verified sending domain, targeted lists/segments, a lifecycle status (draft, running, paused, finished, cancelled), and send progress counts (sent, failed, remaining).
- **API key (transactional)**: A workspace-scoped credential carrying the transactional-sending scope; its secret is shown only once.
- **Send progress**: The reported counts and status of an in-flight or completed campaign send.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: An operator can add a sending domain, copy its DNS records, and see it reach "verified" status entirely through the interface, with no manual step beyond publishing the records.
- **SC-002**: An operator can author a campaign, target a list, select a verified domain, and start the send in under 5 minutes without external help.
- **SC-003**: 100% of attempts to start a campaign without a verified sending domain are blocked with a clear explanation rather than failing after the fact.
- **SC-004**: An operator viewing a running campaign sees send progress (sent/failed/remaining) advance toward completion without manually reloading the page.
- **SC-005**: An administrator can issue a transactional API key and see its secret exactly once, with the non-retrievable warning understood (verified by the secret never reappearing after the dialog is closed).
- **SC-006**: 100% of asynchronous views display an explicit loading, empty, or error state rather than a blank screen.
- **SC-007**: 100% of destructive actions require a confirmation step before taking effect.
- **SC-008**: Actions the current user lacks permission for (`sending:*`, `campaigns:*`, `transactional:send`, API-key management) are never presented as available without explanation.
- **SC-009**: Every Phase 3 screen uses the shared design system consistent with the Phase 1 & 2 UI, with no screen retaining a minimal or unstyled appearance.
- **SC-010**: A session expiry during any Phase 3 task routes the user to sign in with a clear message rather than a broken screen.

## Assumptions

- The Phase 3 backend (sending domains, templates, campaigns, send pipeline, tracking, and the transactional `tx` endpoint) is fully implemented and stable; this feature builds only the web interface against the existing tenant-scoped API.
- The Phase 1 & 2 UI — the persistent sidebar workspace app shell, the shared design system, session-cookie authentication, and permission-aware UI gating — already exists and is extended, not rebuilt.
- The current frontend stack and component library already present in the project are reused; no new framework is introduced.
- Permission gating in the UI uses the Phase 3 permission strings `sending:*`, `campaigns:*`, and `transactional:send`, mirroring the backend; the backend remains the source of truth and is always re-checked server-side.
- API-key management UI (issue / show-once / revoke) was introduced by the Phase 1 & 2 UI; this feature reuses it and only adds the transactional-sending scope as a selectable scope.
- A visual / WYSIWYG email editor is out of scope (Phase 7); template and campaign content is edited as plain HTML and/or text in a basic editor.
- Campaign scheduling is out of scope (Phase 7); campaigns are started immediately ("send now"), not scheduled for a future time.
- Open/click analytics dashboards are out of scope (Phase 4); campaign monitoring in this phase shows send-progress counts (sent / failed / remaining) and lifecycle status only, not aggregated open/click rates.
- Bounce/complaint handling and suppression-list management are out of scope (Phase 4) and have no UI in this feature.
- Live status updates (domain verification, campaign progress) are delivered by re-fetching on an interval or on view focus; no specific real-time transport is mandated.
- The interface targets modern desktop browsers, consistent with the Phase 1 & 2 UI; responsive/mobile layouts are a nice-to-have, not a requirement.
- Sending domains are managed at the workspace level; SPF and DMARC records are composed by the platform and DKIM tokens come from the provider — the UI displays whatever records the backend returns without computing them.

## Dependencies

- Phase 3 (Sending Pipeline) backend — sending domains, templates, campaigns, send pipeline, tracking, and the transactional endpoint.
- Phase 1 & 2 UI — the workspace app shell, shared design system, authentication, and permission-aware gating that this feature extends.
- The tenant-scoped API exposing sending-domain, template, campaign, and API-key operations.
