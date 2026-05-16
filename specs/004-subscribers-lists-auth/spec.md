# Feature Specification: Phase 2 — Subscribers, Lists & Auth

**Feature Branch**: `004-subscribers-lists-auth`

**Created**: 2026-05-17

**Status**: Draft

**Input**: User description: "Phase 2 — Subscribers, Lists & Auth: tenant-plane schema (lists, subscribers, subscriber_lists, roles, users, sessions, settings) each tenant-scoped and isolated; tenant RBAC with user-level and per-list roles, permission strings, scoped API keys, and TOTP two-factor auth; subscriber and list CRUD with custom JSON attributes and query/segment-based selection; CSV/ZIP subscriber import (with upsert) and export. Exit criteria: a tenant user can manage lists and subscribers, import/export, and the RBAC gates work."

## User Scenarios & Testing *(mandatory)*

<!--
  Phase 2 builds inside the tenant established by Phase 1 (Tenancy Core). The
  "users" here are tenant operators — the people who run a workspace's audience:
  marketers, list managers, and administrators. Every record they touch is
  confined to their own tenant; Phase 1's isolation guarantee is a hard
  precondition for everything below.
-->

### User Story 1 - Manage lists and subscribers (Priority: P1)

A tenant operator builds their audience. They create one or more lists (named groupings of contacts), add subscribers to those lists, and maintain subscriber records over time — editing details, attaching custom attributes, changing subscription state, and removing contacts that should no longer be held. All of this happens entirely within their own tenant's workspace.

**Why this priority**: Lists and subscribers are the central objects of the platform — every later capability (campaigns, sending, segmentation) operates on them. The exit criterion "a tenant user can manage lists and subscribers" depends directly on this story, and it is the first independently demonstrable slice of Phase 2.

**Independent Test**: As an operator in a clean tenant, create a list, create several subscribers, attach them to the list with custom attributes, edit a subscriber, change a subscriber's state, and delete a subscriber and a list. Confirm every record is visible only inside that tenant.

**Acceptance Scenarios**:

1. **Given** an operator in a tenant, **When** they create a list with a name and description, **Then** the list exists within that tenant and is listed in that tenant only.
2. **Given** a list, **When** the operator creates a subscriber with an email address and adds them to the list, **Then** the subscriber and the list membership are recorded with an initial subscription state.
3. **Given** an existing subscriber, **When** the operator edits the subscriber's name or custom attributes, **Then** the updated values are persisted and returned on the next read.
4. **Given** an email address that already belongs to a subscriber in the tenant, **When** the operator creates another subscriber with that same email, **Then** creation is refused with a clear message and no duplicate is created.
5. **Given** a subscriber on a list, **When** the operator changes the subscriber's membership state (for example, unsubscribed), **Then** the new state is recorded and reflected in list views.
6. **Given** an existing subscriber or list, **When** the operator deletes it, **Then** it is removed from the tenant and its memberships are cleaned up, while subscribers/lists not targeted are unaffected.
7. **Given** two tenants each with their own lists and subscribers, **When** an operator of one tenant reads or searches, **Then** they see only their own tenant's records and never the other tenant's.

---

### User Story 2 - Role-based access gates (Priority: P1)

A tenant administrator controls who can do what. They define roles as named sets of permissions, assign roles to the tenant's users at the tenant level, and additionally grant per-list roles so a user can be limited to specific lists. When a user attempts an action, the system allows it only if their effective permissions cover it; otherwise the action is denied.

**Why this priority**: The exit criterion explicitly requires that "the RBAC gates work." Access control is a Security & Consent by Design principle and must be structural, not retrofitted. It is P1 because no multi-operator tenant is safe to use without it.

**Independent Test**: As an administrator, create a role with a limited permission set, assign it to a second user, and confirm that the second user can perform permitted actions and is denied disallowed actions. Then grant that user a per-list role on one list and confirm their access widens for that list only.

**Acceptance Scenarios**:

1. **Given** an administrator, **When** they create a role with a defined set of permissions, **Then** the role exists in the tenant and can be assigned.
2. **Given** a role assigned to a user at the tenant level, **When** that user performs an action covered by the role's permissions, **Then** the action is allowed.
3. **Given** a user whose role does not cover an action, **When** that user attempts it, **Then** the action is denied with a clear authorization message and no change is made.
4. **Given** a user with limited tenant-level permissions, **When** they are granted a per-list role on a specific list, **Then** their permissions widen for that list only and remain unchanged for all other lists.
5. **Given** a user with both a tenant-level role and a per-list role, **When** they act on that list, **Then** their effective permissions are the combination of both.
6. **Given** a role currently assigned to users, **When** an administrator changes the role's permissions, **Then** the change applies to those users on their next action.
7. **Given** a non-administrator user, **When** they attempt to create or assign roles, **Then** the attempt is denied.

---

### User Story 3 - Import and export subscribers (Priority: P2)

A tenant operator brings an existing audience into the platform and gets it back out. They upload a CSV file (optionally compressed as a ZIP) of subscribers; the system creates new subscribers and updates existing ones matched by email (upsert), optionally attaching them to chosen lists. They can also export subscribers — all of them, those on a list, or those matching a query — to a CSV file.

**Why this priority**: The exit criterion requires import/export. It depends on Stories 1 and 4 (subscribers, lists, and selection must exist first), so it is P2 — valuable and required for exit, but not the first slice.

**Independent Test**: Upload a CSV containing a mix of new and already-existing email addresses; confirm new subscribers are created, existing ones are updated, and all are attached to the chosen list. Then export the list to CSV and confirm the file contains the expected subscribers and attributes.

**Acceptance Scenarios**:

1. **Given** a CSV file of subscribers, **When** the operator imports it and selects target lists, **Then** each row becomes a subscriber attached to those lists.
2. **Given** an import file containing an email that already exists in the tenant, **When** the import runs in upsert mode, **Then** the existing subscriber is updated rather than duplicated.
3. **Given** a ZIP file containing a CSV, **When** the operator imports it, **Then** the contained CSV is processed identically to a plain CSV upload.
4. **Given** an import file with malformed rows (missing or invalid email), **When** the import runs, **Then** valid rows are imported, invalid rows are skipped, and the operator receives a summary of how many rows succeeded, were updated, and failed.
5. **Given** a list or a saved query, **When** the operator exports it, **Then** a CSV file is produced containing the matching subscribers and their attributes.
6. **Given** an import or export in progress, **When** the operator views its status, **Then** they see progress and a final result (counts, errors).
7. **Given** an operator without import/export permission, **When** they attempt an import or export, **Then** the attempt is denied.

---

### User Story 4 - Segment-based subscriber selection (Priority: P2)

A tenant operator targets a precise slice of their audience. They define a query over subscriber fields, custom attributes, and list membership — for example, "subscribers on List A whose custom attribute `country` is `DE` and who are confirmed" — and the system returns the matching set. The query can be used to view, count, export, or act on that segment.

**Why this priority**: Segmentation makes the audience useful and is depended on by export (Story 3) and by later campaign phases. It is P2 because basic CRUD (Story 1) must exist before selection has anything to operate on.

**Independent Test**: With a population of subscribers carrying varied custom attributes and list memberships, define a query combining an attribute condition and a list condition, and confirm the returned set exactly matches the expected subscribers and the reported count is correct.

**Acceptance Scenarios**:

1. **Given** subscribers with custom attributes, **When** the operator queries by an attribute value, **Then** only subscribers whose attribute matches are returned.
2. **Given** subscribers across multiple lists, **When** the operator queries by list membership and subscription state, **Then** only subscribers matching both are returned.
3. **Given** a query combining several conditions, **When** it is run, **Then** the result reflects all conditions together and an accurate match count is reported.
4. **Given** a query that matches no subscribers, **When** it is run, **Then** an empty result and a zero count are returned without error.
5. **Given** a query, **When** the operator uses it to drive an export, **Then** the exported file contains exactly the queried set.
6. **Given** an invalid or malformed query, **When** it is submitted, **Then** it is rejected with a clear message and no partial result.

---

### User Story 5 - Scoped API keys and two-factor authentication (Priority: P3)

A tenant operator hardens access. An administrator issues API keys for programmatic access, each carrying only a chosen subset of permissions (scoped, least-privilege) and revocable at any time. Individually, users protect their own sign-in by enabling time-based one-time-password (TOTP) two-factor authentication, so login requires both their password and a current code.

**Why this priority**: Scoped credentials and 2FA are explicit Phase 2 deliverables and Security by Design requirements, but the exit-criteria demo (manage, import/export, RBAC) can be shown without them. They are P3 — required for the phase to be complete, layered on after the core slices.

**Independent Test**: Create an API key scoped to read-only permissions, use it to perform a read (allowed) and a write (denied), then revoke it and confirm it no longer works. Separately, enable TOTP for a user, sign out, and confirm sign-in now requires a valid current code.

**Acceptance Scenarios**:

1. **Given** an administrator, **When** they create an API key and choose a subset of permissions, **Then** a key is issued whose access is limited to exactly those permissions.
2. **Given** a scoped API key, **When** it is used for an action outside its permissions, **Then** the action is denied.
3. **Given** an existing API key, **When** an administrator revokes it, **Then** subsequent use of that key is rejected.
4. **Given** a user, **When** they enable TOTP two-factor authentication, **Then** they are shown a way to register an authenticator and future sign-ins require a current code in addition to the password.
5. **Given** a user with TOTP enabled, **When** they sign in with the correct password but a missing or incorrect code, **Then** sign-in is refused.
6. **Given** a user with TOTP enabled, **When** they disable it after re-authenticating, **Then** future sign-ins no longer require a code.

---

### Edge Cases

- What happens when an import file is very large — does the operator get progress and a bounded, non-blocking experience rather than a frozen request?
- How does the system handle an import row whose email is valid but whose custom-attribute column contains malformed data?
- What happens when an operator deletes a list that still has subscribers — are the subscribers kept (only the membership removed) or also deleted?
- How does the system handle a subscriber who is on multiple lists and is unsubscribed from one — do other memberships remain intact?
- What happens to a user's in-flight session and API keys when their role is downgraded or removed?
- How does the system behave when an administrator deletes a role that is still assigned to users?
- What happens when a user loses access to their authenticator after enabling TOTP — is there a recovery path?
- How does the system handle two concurrent imports targeting the same subscriber email?
- What happens when a query references a custom attribute that no subscriber has?
- How is an export of a very large segment delivered without timing out?

## Requirements *(mandatory)*

### Functional Requirements

#### Tenant-plane data & isolation

- **FR-001**: System MUST maintain, scoped to each tenant, the audience and access objects: lists, subscribers, subscriber-to-list memberships, roles, tenant users, sessions, and tenant settings.
- **FR-002**: System MUST confine every read and write of these objects to the acting user's tenant, enforced at the data-storage layer so that an omitted application-level filter still cannot expose or modify another tenant's data.
- **FR-003**: System MUST carry automated tests proving that a user of one tenant cannot read or write another tenant's lists, subscribers, roles, users, sessions, or settings.
- **FR-004**: System MUST keep per-tenant settings that operators can read and update without affecting other tenants.

#### Lists & subscribers

- **FR-005**: Users MUST be able to create, view, edit, and delete lists within their tenant, each with at least a name and description.
- **FR-006**: Users MUST be able to create, view, edit, and delete subscribers within their tenant, each identified by an email address unique within the tenant.
- **FR-007**: System MUST reject creation of a subscriber whose email already exists in the tenant, with a clear message and no duplicate.
- **FR-008**: Users MUST be able to attach subscribers to lists and remove them from lists, each membership carrying a subscription state.
- **FR-009**: System MUST support a subscriber state (e.g., enabled, disabled, blocklisted) and a per-list subscription state (e.g., unconfirmed, confirmed, unsubscribed), and MUST let permitted users change them.
- **FR-010**: System MUST allow each subscriber to carry custom attributes as free-form structured key/value data, with no fixed schema, and MUST persist and return them on read.
- **FR-011**: System MUST clean up list memberships when a subscriber or list is deleted, without affecting unrelated subscribers or lists.
- **FR-012**: System MUST provide paginated listing and search of subscribers and lists within a tenant.

#### Segmentation

- **FR-013**: Users MUST be able to define a query selecting subscribers by subscriber fields, custom attributes, list membership, and subscription state.
- **FR-014**: System MUST return the matching subscriber set and an accurate match count for a query.
- **FR-015**: System MUST reject an invalid or malformed query with a clear message and produce no partial result.
- **FR-016**: System MUST allow a query to drive views, counts, and exports over exactly the matching set.

#### Import & export

- **FR-017**: Users MUST be able to import subscribers from an uploaded CSV file and from a ZIP file containing a CSV.
- **FR-018**: System MUST upsert on import — creating subscribers that are new and updating existing subscribers matched by email — and MUST optionally attach imported subscribers to operator-selected lists.
- **FR-019**: System MUST skip invalid import rows (e.g., missing or invalid email), continue processing valid rows, and report a summary of created, updated, and failed counts.
- **FR-020**: Users MUST be able to export subscribers — all, by list, or by query — to a CSV file including their attributes.
- **FR-021**: System MUST process imports and exports without blocking the operator, and MUST expose progress and a final result for an in-progress or completed job.

#### Roles, permissions & access gates

- **FR-022**: Administrators MUST be able to create, edit, and delete roles within their tenant, each role being a named set of permissions expressed as permission strings.
- **FR-023**: Administrators MUST be able to assign a role to a tenant user at the tenant level and additionally grant a user a role scoped to a specific list.
- **FR-024**: System MUST compute a user's effective permissions for an action as the combination of their tenant-level role and any list-scoped role relevant to the targeted list.
- **FR-025**: System MUST allow an action only when the acting user's effective permissions cover it, and MUST deny it otherwise with a clear authorization message and no state change.
- **FR-026**: System MUST apply changes to a role's permissions to all users holding that role on their subsequent actions.
- **FR-027**: System MUST restrict role management and role assignment to users with administrative permission.
- **FR-028**: System MUST audit privileged actions — role creation/changes, role assignment, API key issuance and revocation — in a way that is attributable to the acting user.

#### Authentication & credentials

- **FR-029**: System MUST authenticate tenant users and maintain authenticated sessions scoped to a single tenant.
- **FR-030**: Administrators MUST be able to issue API keys for programmatic access, each scoped to a chosen subset of permissions (least privilege).
- **FR-031**: System MUST deny any API-key request for an action outside that key's scoped permissions.
- **FR-032**: Administrators MUST be able to revoke an API key, after which any further use of it is rejected.
- **FR-033**: Users MUST be able to enable time-based one-time-password (TOTP) two-factor authentication on their own account and to disable it after re-authenticating.
- **FR-034**: System MUST require a valid current TOTP code in addition to the password when signing in a user who has TOTP enabled, and MUST refuse sign-in when the code is missing or incorrect.
- **FR-035**: System MUST provide a recovery path for a user who has lost access to their TOTP authenticator.

### Key Entities *(include if feature involves data)*

- **List**: A named grouping of subscribers within a tenant. Key attributes: name, description, type/visibility, opt-in style, tags. Belongs to exactly one tenant.
- **Subscriber**: An audience contact within a tenant, identified by a tenant-unique email address. Key attributes: email, name, subscriber state, custom attributes (free-form structured data). Belongs to exactly one tenant; may belong to many lists.
- **Subscriber-List Membership**: The link between a subscriber and a list, carrying a per-list subscription state. Belongs to one tenant.
- **Role**: A named set of permissions (permission strings) within a tenant. May be assigned at the tenant level or scoped to a specific list.
- **Tenant User**: An operator account that acts within a single tenant. Holds a tenant-level role and optionally list-scoped roles; may have TOTP enabled.
- **Session**: An authenticated session for a tenant user, scoped to one tenant.
- **API Key**: A revocable programmatic credential scoped to a subset of permissions, belonging to a tenant.
- **Tenant Settings**: Per-tenant configuration key/values.
- **Import/Export Job**: A unit of bulk subscriber processing with status, progress, and a result summary (created/updated/failed counts).
- **Audit Record**: An attributable log entry for a privileged action (role and key management).

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A tenant operator can create a list, add subscribers, and edit and delete records end-to-end without assistance.
- **SC-002**: 100% of cross-tenant access attempts against lists, subscribers, roles, users, sessions, and settings are denied, including when an application-level tenant filter is deliberately omitted in a test.
- **SC-003**: A user is allowed every action covered by their effective permissions and denied every action that is not, across both tenant-level and per-list role assignments — verified by automated tests covering allow and deny paths.
- **SC-004**: An operator can import a CSV (or ZIP-wrapped CSV) of 50,000 subscribers, with new rows created and existing rows updated by email, and receive an accurate created/updated/failed summary.
- **SC-005**: An import of 50,000 subscribers completes without blocking the operator's session, and progress is observable while it runs.
- **SC-006**: A segment query combining a custom-attribute condition and a list-membership condition returns exactly the matching subscribers with a correct count.
- **SC-007**: Subscribers exported by list or by query produce a CSV whose contents exactly match the selected set.
- **SC-008**: A scoped API key succeeds for in-scope actions and is denied for out-of-scope actions; a revoked key fails on every subsequent use.
- **SC-009**: A user with TOTP enabled cannot sign in without a valid current code, and can sign in with one.
- **SC-010**: Every privileged action (role change, role assignment, API key issuance/revocation) produces an audit record attributable to the acting user.
- **SC-011**: The phase exits with a green automated test suite and a clean schema migration.

## Assumptions

- Phase 1 (Tenancy Core) is in place: tenants exist, and the row-level isolation pattern and tenant-resolution mechanism are available for Phase 2 objects to build on.
- "Tenant users" are operator accounts scoped to a single tenant and are distinct from Phase 1 control-plane platform accounts; the mechanism by which a platform account becomes a tenant operator is inherited from Phase 1 membership and not re-specified here.
- Subscriber identity within a tenant is the email address; upsert on import matches on email.
- Custom attributes are free-form per subscriber (no tenant-defined attribute schema); validation is limited to the data being well-formed structured key/value content.
- Per-list and tenant-level roles combine permissively: a user's effective permission for an action on a list is the union of their tenant-level role and any list-scoped role for that list.
- Two-factor authentication is opt-in per user for this phase; a tenant-wide policy to require 2FA for all users is out of scope for Phase 2.
- TOTP recovery uses one-time recovery codes issued when 2FA is enabled; alternative recovery channels are out of scope.
- Import and export run as asynchronous jobs using the Phase 0 job-processing foundation; very large exports are delivered as a downloadable file rather than an inline response.
- CSV import expects a header row mapping columns to subscriber fields and custom attributes; the exact column-mapping UX is an implementation detail left to planning.
- Deleting a list removes memberships but does not delete the subscribers themselves; deleting a subscriber removes that subscriber from all its lists.
- Permission strings follow a resource:action convention (e.g., `lists:manage`, `subscribers:import`); the full catalogue of permission strings is finalized during planning.
- Email sending, campaigns, double-opt-in confirmation flows, and subscriber-facing self-service (preference pages, public sign-up) are out of scope for Phase 2 and handled in later phases.
