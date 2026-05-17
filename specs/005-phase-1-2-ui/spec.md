# Feature Specification: Phases 1 & 2 — Frontend UI

**Feature Branch**: `005-phase-1-2-ui`

**Created**: 2026-05-17

**Status**: Draft

**Input**: User description: "Add UI for Phase 1 (Tenancy Core) and Phase 2 (Subscribers, Lists & Auth) — a cohesive web interface covering platform onboarding, tenant workspaces, lists & subscribers, RBAC, import/export, and account security."

## User Scenarios & Testing *(mandatory)*

<!--
  This feature delivers the web interface for capabilities already implemented in
  the backend. Phase 1 (Tenancy Core) and Phase 2 (Subscribers, Lists & Auth)
  expose a platform API and a tenant-scoped API; today only thin, minimally
  styled Phase 1 screens exist. This work redesigns those screens into one
  cohesive interface and adds full Phase 2 coverage.

  Two kinds of "user" appear here:
  - Platform users — people who sign up, own platform accounts, and create or
    join workspaces (tenants).
  - Tenant operators — the same people acting inside a workspace: marketers,
    list managers, and administrators who manage audiences and access.
-->

### User Story 1 - Onboard onto the platform and open a workspace (Priority: P1)

A new person registers a platform account, signs in, and creates a workspace (tenant). They land in that workspace through a consistent, polished interface with a persistent sidebar that orients them around everything the workspace can do. Returning users sign in and pick a workspace from the ones they belong to.

**Why this priority**: Nothing else in the product is reachable without an account and a workspace to enter. This story replaces the existing minimally styled Phase 1 screens with the cohesive design system and the sidebar app shell that hosts every later story. It is the first independently shippable slice.

**Independent Test**: On a clean environment, register an account, sign in, create a workspace, and confirm you arrive inside that workspace with a working sidebar layout. Sign out and back in, and confirm the workspace appears in your list and can be re-entered.

**Acceptance Scenarios**:

1. **Given** no account, **When** a person registers with a valid email, password, and name, **Then** the account is created, they are signed in, and they are guided to create their first workspace.
2. **Given** registration is attempted with an email that already has an account, **When** the form is submitted, **Then** a clear, non-technical error is shown and no second account is created.
3. **Given** a signed-in user with no workspace, **When** they create a workspace with a name and a unique address (slug), **Then** the workspace is created and they enter it inside the sidebar app shell.
4. **Given** a signed-in user who belongs to one or more workspaces, **When** they open the home screen, **Then** all their workspaces are listed and selecting one enters that workspace.
5. **Given** valid credentials, **When** a returning user signs in, **Then** an authenticated session is established; **and** with invalid credentials a non-specific error is shown.
6. **Given** a signed-in user, **When** they sign out, **Then** the session ends and they are returned to the signed-out entry screen.
7. **Given** a user who is not a member of a workspace, **When** they attempt to open that workspace's address, **Then** access is refused with a clear "not found / no access" screen rather than a raw error.

---

### User Story 2 - Manage lists and subscribers (Priority: P1)

A tenant operator builds their audience inside a workspace. They create and maintain lists (named groupings of contacts), create and edit subscribers, attach custom attributes, move subscribers on and off lists, change subscription state, and find subscribers by searching or by query/segment criteria.

**Why this priority**: Lists and subscribers are the central objects of the product; every later capability operates on them. This is the core of Phase 2 and the first demonstrable workspace slice beyond onboarding.

**Independent Test**: As an operator in a workspace, create a list, create several subscribers, attach them to the list with custom attributes, edit a subscriber, change a subscriber's subscription state, run a query/segment search, and delete a subscriber and a list. Confirm every change is reflected in the relevant views.

**Acceptance Scenarios**:

1. **Given** an operator in a workspace, **When** they create a list with a name and description, **Then** the list appears in the workspace's list view.
2. **Given** a list, **When** the operator views it, **Then** they see its subscribers, their subscription state, and a count.
3. **Given** an operator, **When** they create a subscriber with an email and optional name and custom attributes, **Then** the subscriber is created and visible in the subscribers view.
4. **Given** an email that already belongs to a subscriber in the workspace, **When** the operator tries to create another with that email, **Then** creation is refused with a clear message.
5. **Given** an existing subscriber, **When** the operator edits the name or custom attributes, **Then** the updated values are persisted and shown on the next view.
6. **Given** a subscriber, **When** the operator adds them to or removes them from a list, or changes their subscription state on a list (e.g., unsubscribed), **Then** the change is reflected in both the subscriber view and the list view.
7. **Given** many subscribers, **When** the operator searches by email/name or builds a query/segment by attribute criteria, **Then** only matching subscribers are shown, along with a total count.
8. **Given** an existing subscriber or list, **When** the operator deletes it after confirming, **Then** it is removed and untargeted records are unaffected.

---

### User Story 3 - Invite teammates and manage roles (Priority: P2)

A workspace administrator brings teammates into the workspace and controls what each can do. They invite people by email, see and manage pending invitations and current members, create roles as named permission sets, assign roles to members at the workspace level, and grant per-list roles that widen a member's access on specific lists only. The interface reflects each user's permissions — actions they cannot perform are hidden or disabled.

**Why this priority**: Collaboration and access control are required for any multi-operator workspace, but a workspace and its audience (Stories 1 and 2) must exist first. This story unifies Phase 1 team invites with Phase 2 RBAC into one "people & access" area.

**Independent Test**: As an administrator, invite an email, accept the invitation as that person, create a role with a limited permission set, assign it to the new member, and confirm the member can perform permitted actions and is blocked from disallowed ones. Grant a per-list role and confirm the member's access widens for that one list.

**Acceptance Scenarios**:

1. **Given** an administrator, **When** they invite an email address to the workspace, **Then** a pending invitation appears in the members area and the invitee can accept it via an invitation screen.
2. **Given** an invitation screen opened from an invite link, **When** the invitee accepts (creating an account if they have none), **Then** they become a member and can enter the workspace.
3. **Given** a pending or expired invitation, **When** an administrator revokes it or an invitee opens an invalid one, **Then** the interface shows a clear state and acceptance is impossible.
4. **Given** an administrator, **When** they create a role and select its permissions, **Then** the role appears and can be assigned to members.
5. **Given** a member with an assigned role, **When** they use the workspace, **Then** actions covered by their permissions are available and actions not covered are hidden or disabled, and a denied attempt shows a clear authorization message.
6. **Given** a member, **When** an administrator grants them a per-list role on one list, **Then** their access widens for that list only and is unchanged elsewhere.
7. **Given** a non-administrator, **When** they open the people & access area, **Then** role creation and assignment controls are not available to them.

---

### User Story 4 - Import and export subscribers (Priority: P2)

A tenant operator brings an existing audience into the workspace and gets it back out. They upload a CSV file (optionally compressed as a ZIP), choose which lists to attach the imported subscribers to, and watch the import progress to a result summary. They also export subscribers — all of them, a list, or a query/segment — and download the resulting CSV file.

**Why this priority**: Import/export depends on lists, subscribers, and query selection (Story 2) already existing. It is required for the Phase 2 exit criteria but is not the first slice.

**Independent Test**: Upload a CSV with a mix of new and existing email addresses, choose a target list, and confirm the result summary reports created/updated/failed counts and that the subscribers appear on the list. Then export the list and download a CSV containing the expected subscribers and attributes.

**Acceptance Scenarios**:

1. **Given** an operator, **When** they upload a CSV (or a ZIP containing a CSV) and select target lists, **Then** an import job starts and its progress is shown.
2. **Given** a running import or export job, **When** the operator views it, **Then** they see progress and, on completion, a result summary (created, updated, skipped/failed counts).
3. **Given** an import file with malformed rows, **When** the import completes, **Then** valid rows are reported as imported and invalid rows are reported as failed, without blocking the whole import.
4. **Given** a completed export job, **When** the operator opens it, **Then** they can download the resulting CSV file.
5. **Given** an operator without import/export permission, **When** they open the import/export area, **Then** the upload and export controls are unavailable and the restriction is explained.

---

### User Story 5 - Secure the account and workspace (Priority: P3)

A tenant operator protects their account and integrations. They enrol in two-factor authentication (TOTP) by scanning a code and confirming it, and are then prompted for a TOTP code when opening a workspace session. Administrators issue scoped API keys for programmatic access and revoke them when no longer needed, review an audit trail of workspace activity, and manage workspace settings.

**Why this priority**: Security hardening, API keys, audit visibility, and settings round out Phases 1 & 2 but are not needed to demonstrate the core onboarding and audience-management flows.

**Independent Test**: Enrol in TOTP and confirm a code; sign out and back in and confirm the TOTP challenge appears and accepts a valid code. As an administrator, issue an API key, see it once, revoke it, and confirm it disappears from the active list. Open the audit trail and confirm recent actions are listed.

**Acceptance Scenarios**:

1. **Given** an operator without 2FA, **When** they start TOTP enrolment, **Then** they are shown an enrolment code/QR and must confirm a generated code to finish.
2. **Given** an operator with TOTP enabled, **When** they open a workspace session, **Then** they are prompted for a TOTP code and only proceed once a valid code is entered.
3. **Given** an operator with TOTP enabled, **When** they disable it, **Then** future sessions no longer prompt for a code.
4. **Given** an administrator, **When** they issue a scoped API key, **Then** the key's secret is shown once with a clear warning that it cannot be retrieved again.
5. **Given** an active API key, **When** an administrator revokes it, **Then** it is removed from the active keys list.
6. **Given** an administrator, **When** they open the audit trail, **Then** recent workspace actions are listed with actor, action, and time.
7. **Given** an administrator, **When** they update workspace settings, **Then** the changes are saved and reflected on the next view.

---

### Edge Cases

- A session expires while the operator is mid-task — the interface detects the unauthenticated response and routes the user to sign in without losing unsaved context where reasonable.
- A workspace address (slug) in the URL does not exist or the user is not a member — a clear "not found / no access" screen is shown, not a raw error.
- A network request fails or the backend returns an error — the interface shows a readable, non-technical message and allows retry; it never shows a blank or broken screen.
- A long-running import/export job is open when the operator navigates away and back — the job's current status is re-fetched and displayed.
- A list or subscriber is deleted by another operator while a teammate is viewing it — the stale view recovers gracefully on the next action.
- A user with reduced permissions opens a deep link to an area they cannot use — the interface explains the restriction rather than failing silently.
- Custom attributes contain large or deeply nested JSON — the editor remains usable and validates structure before saving.
- A subscriber's email is already present during import — the row is upserted (updated), and the summary distinguishes created from updated.
- The TOTP code entered during a session challenge is wrong or expired — a clear error is shown and the operator can retry.

## Requirements *(mandatory)*

### Functional Requirements

#### Platform onboarding & workspaces (Phase 1)

- **FR-001**: The interface MUST let a person register a platform account with email, password, and name, and show clear validation and duplicate-email errors.
- **FR-002**: The interface MUST let a registered person sign in and sign out, and MUST present a non-specific error on invalid credentials.
- **FR-003**: The interface MUST let a signed-in user create a workspace with a name and a unique address (slug), surfacing slug-conflict errors clearly.
- **FR-004**: The interface MUST show a signed-in user the list of workspaces they belong to and let them enter any of them.
- **FR-005**: The interface MUST present an invitation-acceptance screen reachable from an invite link, allowing an invitee to accept and, if needed, create an account as part of accepting.
- **FR-006**: The interface MUST detect an unauthenticated or unauthorized response and route the user to sign in or to a clear access-denied screen.

#### Workspace shell & navigation

- **FR-007**: An authenticated workspace MUST be presented inside a persistent sidebar app shell that provides navigation to Subscribers, Lists, People & Access, Import/Export, Audit, and Settings, plus the current workspace name and an account/sign-out control.
- **FR-008**: The interface MUST visually indicate the active section and the current workspace at all times within the shell.
- **FR-009**: Navigation entries and in-page actions MUST be hidden or disabled when the current user's permissions do not allow the corresponding capability.

#### Lists & subscribers (Phase 2)

- **FR-010**: The interface MUST let operators create, view, edit, and delete lists, with delete requiring explicit confirmation.
- **FR-011**: A list view MUST show the list's subscribers, each subscriber's subscription state, and a total count.
- **FR-012**: The interface MUST let operators create, view, edit, and delete subscribers, with delete requiring explicit confirmation.
- **FR-013**: The interface MUST let operators edit subscriber custom attributes as structured JSON, validating structure before saving.
- **FR-014**: The interface MUST let operators add a subscriber to a list, remove a subscriber from a list, and change a subscriber's subscription state on a list.
- **FR-015**: The interface MUST let operators search subscribers by email/name and build query/segment selections by attribute criteria, showing matching results and a total count.
- **FR-016**: Lists and subscriber views MUST handle large result sets without becoming unusable (e.g., via paging or incremental loading).

#### People & access / RBAC (Phases 1 & 2)

- **FR-017**: The interface MUST let an administrator invite people by email, and MUST show pending invitations and current members with their roles.
- **FR-018**: The interface MUST let an administrator revoke a pending invitation.
- **FR-019**: The interface MUST let an administrator create, edit, and delete roles, selecting each role's permissions from the available permission set.
- **FR-020**: The interface MUST let an administrator assign a role to a member at the workspace level and grant or remove a per-list role for a member on a specific list.
- **FR-021**: When the backend denies an action for lack of permission, the interface MUST show a clear authorization message and leave data unchanged.

#### Import & export (Phase 2)

- **FR-022**: The interface MUST let permitted operators upload a CSV or ZIP file and choose target lists for the imported subscribers.
- **FR-023**: The interface MUST start import and export jobs and display their progress and a final result summary (created, updated, skipped/failed counts).
- **FR-024**: The interface MUST let operators start an export of all subscribers, a list, or a query/segment, and download the resulting file when the job completes.
- **FR-025**: The interface MUST re-fetch and display current job status when an in-progress job view is re-opened.

#### Account & workspace security (Phases 1 & 2)

- **FR-026**: The interface MUST let an operator enrol in TOTP two-factor authentication by displaying an enrolment code/QR and requiring confirmation of a generated code, and MUST let them disable it.
- **FR-027**: When opening a workspace session for an operator with TOTP enabled, the interface MUST present a TOTP challenge and proceed only on a valid code, with a clear retry path on failure.
- **FR-028**: The interface MUST let an administrator issue a scoped API key, display its secret exactly once with a non-retrievable warning, and let them revoke active keys.
- **FR-029**: The interface MUST let an administrator view an audit trail listing recent workspace actions with actor, action, and time.
- **FR-030**: The interface MUST let an administrator view and update workspace settings and confirm successful saves.

#### Cross-cutting

- **FR-031**: All destructive actions MUST require explicit confirmation before proceeding.
- **FR-032**: All forms MUST show inline validation, a busy/disabled state during submission, and readable error messages on failure.
- **FR-033**: The interface MUST present a consistent visual design system (shared components, spacing, typography) across every Phase 1 and Phase 2 screen, replacing the current minimally styled screens.
- **FR-034**: Every asynchronous data view MUST present distinct loading, empty, error, and populated states.

### Key Entities *(include if feature involves data)*

- **Platform account**: A person's identity across the platform — email, name; owns sessions and workspace memberships.
- **Workspace (tenant)**: An isolated workspace with a name and unique address (slug); the container for all audience and access data.
- **Membership**: The link between a platform account and a workspace, carrying the member's workspace-level role.
- **Invitation**: A pending request, addressed to an email, to join a workspace; has a status and an expiry.
- **List**: A named, described grouping of subscribers within a workspace.
- **Subscriber**: A contact within a workspace — email, optional name, custom JSON attributes; can belong to multiple lists.
- **List membership / subscription**: The link between a subscriber and a list, carrying a subscription state (e.g., subscribed, unsubscribed).
- **Query / segment**: A set of attribute criteria used to select subscribers dynamically.
- **Role**: A named permission set; assignable at the workspace level or per list.
- **API key**: A scoped credential for programmatic access; its secret is shown only once.
- **Job**: A running or completed import or export, with progress and a result summary.
- **Audit entry**: A recorded workspace action with actor, action, and timestamp.
- **Workspace settings**: Configurable workspace-level options.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A new person can register, sign in, create a workspace, and arrive inside the workspace shell in under 3 minutes without external help.
- **SC-002**: An operator can create a list and add their first subscriber to it in under 1 minute.
- **SC-003**: 95% of operators can complete a query/segment search and identify the matching subscriber count on their first attempt.
- **SC-004**: An administrator can invite a teammate, create a role, and assign it in under 2 minutes.
- **SC-005**: An operator can upload an import file and reach the result summary, seeing created/updated/failed counts, with no ambiguity about the outcome.
- **SC-006**: 100% of asynchronous views display an explicit loading, empty, or error state rather than a blank screen.
- **SC-007**: 100% of destructive actions require a confirmation step before taking effect.
- **SC-008**: Actions the current user lacks permission for are never presented as available without explanation — verified across every Phase 2 area.
- **SC-009**: Every Phase 1 and Phase 2 screen uses the shared design system, with no screen retaining the previous minimal styling.
- **SC-010**: A session-expiry during any task routes the user to sign in with a clear message rather than a broken screen.

## Assumptions

- The Phase 1 and Phase 2 backend APIs (platform API and tenant-scoped API) are already implemented and stable; this feature builds only the web interface against them.
- The existing minimally styled Phase 1 screens (signup, login, tenant creation, workspace/invites) will be redesigned and extended into the new cohesive interface, not kept as-is.
- The UI covers all four Phase 2 capability areas — lists & subscribers, RBAC, import/export, and security (2FA, API keys, audit) — in this iteration.
- The authenticated workspace uses a persistent sidebar app shell with a top bar.
- The interface is a web application targeting modern desktop browsers; responsive/mobile layouts are a nice-to-have but not a requirement for this iteration.
- The current frontend stack and component library already present in the project are reused; no new framework is introduced.
- Authentication relies on the existing session-cookie mechanism; the UI does not manage tokens directly.
- Permission-aware UI gating mirrors the backend's authorization; the backend remains the source of truth and is always re-checked server-side.
- Email delivery of invitations is handled by the backend; the UI surfaces invite links/status but does not send email.
- Workspace settings content is whatever the backend settings endpoint exposes; the UI renders and edits those fields generically.
