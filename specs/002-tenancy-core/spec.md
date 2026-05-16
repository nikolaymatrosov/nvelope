# Feature Specification: Phase 1 — Tenancy Core

**Feature Branch**: `002-tenancy-core`

**Created**: 2026-05-16

**Status**: Draft

**Input**: User description: "Phase 1 — Tenancy Core: control-plane schema (tenants, platform users, membership); a row-level isolation pattern enforced per request; tenant resolution middleware on `/t/{slug}/...` paths cross-checked against the authenticated session; platform signup/login, tenant creation, and team invites; automated tests proving cross-tenant isolation even when an application-level filter is omitted. Exit criteria: a user can sign up, create a tenant, and invite a teammate; isolation tests pass."

## User Scenarios & Testing *(mandatory)*

<!--
  Phase 1 establishes the multi-tenant foundation of nvelope. The "users" here are
  real end users of the platform (people who sign up and run a workspace) and the
  engineers who depend on the isolation guarantee for every later phase.
-->

### User Story 1 - Sign up and create a workspace (Priority: P1)

A new person registers a platform account, signs in, and creates a tenant — their own isolated workspace. They become the first member of that tenant and can reach it at a dedicated workspace address that is unique to that tenant.

**Why this priority**: Nothing else in the platform exists without a tenant to contain it. Every later phase (sending, billing, etc.) operates inside a tenant. The exit criterion "a user can sign up and create a tenant" depends entirely on this story, and it is independently shippable as the first usable slice.

**Independent Test**: On a clean environment, register a new account, log in, and create a tenant. Confirm the account is authenticated, the tenant exists with its own unique workspace address, and the creator is recorded as a member.

**Acceptance Scenarios**:

1. **Given** no existing account, **When** a person registers with a valid email and password, **Then** an account is created and they can authenticate.
2. **Given** an email that already has an account, **When** a person attempts to register with that same email, **Then** registration is refused with a clear message and no second account is created.
3. **Given** an authenticated user with no tenant, **When** they create a tenant, **Then** the tenant is created with a unique workspace address and the user is recorded as its first member.
4. **Given** an authenticated user, **When** they create a second tenant, **Then** both tenants exist independently and the user is a member of each.
5. **Given** valid credentials, **When** a registered user logs in, **Then** an authenticated session is established; **and** with invalid credentials, login is refused with a non-specific error.

---

### User Story 2 - Invite a teammate (Priority: P2)

A member of a tenant invites another person to that tenant by email address. The invited person receives an invitation, accepts it, and gains access to the same tenant workspace. The two can then operate side by side within that one tenant.

**Why this priority**: Collaboration is the reason tenants exist, and the exit criterion explicitly requires inviting a teammate. It is P2 rather than P1 because a tenant and its creator (Story 1) must exist before anyone can be invited into it.

**Independent Test**: As a member of an existing tenant, invite a second email address, accept the invitation as that second person, and confirm the second person becomes a member of the same tenant and can reach its workspace.

**Acceptance Scenarios**:

1. **Given** a tenant member, **When** they invite an email address to the tenant, **Then** a pending invitation for that tenant is created and addressed to that email.
2. **Given** a pending invitation, **When** the invited person accepts it, **Then** they become a member of the inviting tenant; **and** if they have no account, they can create one as part of accepting.
3. **Given** an invitation addressed to an email that is already a member of the tenant, **When** the invite is created, **Then** the system reports the person is already a member and creates no duplicate membership.
4. **Given** an invitation that has been revoked or has expired, **When** the invited person attempts to accept it, **Then** acceptance is refused with a clear message and no membership is granted.
5. **Given** a tenant with two members, **When** each member opens the tenant workspace, **Then** both see the same tenant and the same tenant-scoped data.

---

### User Story 3 - Tenant data stays isolated (Priority: P1)

Whenever any user operates inside a tenant workspace, every record they read or write is confined to that one tenant. A user authenticated for tenant A who attempts to reach tenant B's workspace is denied. The isolation is enforced at the data-storage layer, so even a programming defect that omits a tenant filter in application code cannot leak or corrupt another tenant's data.

**Why this priority**: This is the core security guarantee of a multi-tenant platform. A leak between tenants is a critical breach, and every later phase trusts this boundary blindly. The exit criterion "isolation tests pass" depends on this story. It is P1 because the guarantee must hold from the very first tenant onward.

**Independent Test**: Seed data for two tenants. Perform reads and writes on behalf of tenant A — including operations where the application-level tenant filter is deliberately omitted — and confirm tenant B's data is never returned, modified, or deleted. Attempt to access tenant B's workspace while authenticated only for tenant A and confirm denial.

**Acceptance Scenarios**:

1. **Given** data belonging to tenant A and tenant B, **When** a request scoped to tenant A reads data, **Then** only tenant A's records are returned.
2. **Given** a request scoped to tenant A, **When** application code issues a read that omits the tenant filter, **Then** no tenant B records are returned.
3. **Given** a request scoped to tenant A, **When** application code issues a write or delete that omits the tenant filter, **Then** no tenant B records are modified or removed.
4. **Given** a user authenticated as a member of tenant A only, **When** they request tenant B's workspace address, **Then** access is denied without revealing whether tenant B exists.
5. **Given** a user who is a member of both tenant A and tenant B, **When** they operate in tenant A's workspace, **Then** they see only tenant A's data, and switching to tenant B's workspace shows only tenant B's data.
6. **Given** the automated isolation test suite, **When** it runs, **Then** every cross-tenant read and write attempt fails to cross the boundary and the suite reports a pass.

---

### Edge Cases

- An invitation is sent to an email that already has a platform account → the existing account is used; no second account is created.
- An invitation is accepted twice → the second acceptance is a no-op and does not create a duplicate membership.
- A user is removed from a tenant while holding an active session for it → subsequent requests to that tenant workspace are denied.
- Two people request the same tenant workspace address simultaneously → each request is independently resolved and scoped without affecting the other.
- The same user makes near-simultaneous requests to two different tenants → each request is scoped to its own tenant with no bleed-through.
- A requested tenant workspace address does not correspond to any tenant → the request is denied with the same response as an unauthorized tenant, so existence is not revealed.
- Application code forgets to apply the tenant filter on a query → the storage-layer guarantee still prevents any cross-tenant data exposure or modification.
- The last remaining member of a tenant leaves or is removed → the system handles the now-memberless tenant predictably (see Assumptions).
- A workspace address (slug) is requested that collides with an already-used one during tenant creation → creation is refused and the user is prompted to choose another.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST allow a person to create a platform account using an email address and a password.
- **FR-002**: System MUST prevent more than one platform account from existing for the same email address.
- **FR-003**: System MUST allow a registered user to authenticate and MUST maintain an authenticated session for that user.
- **FR-004**: System MUST allow an authenticated user to create a new tenant, and MUST record the creator as the tenant's first member.
- **FR-005**: Each tenant MUST have a unique, human-readable workspace identifier (slug) used to address its workspace, and the system MUST reject creation of a tenant whose identifier is already in use.
- **FR-006**: A platform user MUST be able to be a member of more than one tenant, and a tenant MUST be able to have more than one member.
- **FR-007**: System MUST allow a member of a tenant to invite another person to that tenant by email address.
- **FR-008**: System MUST allow an invited person to accept an invitation and thereby become a member of the inviting tenant; if the invited person has no platform account, they MUST be able to create one as part of accepting.
- **FR-009**: Invitations MUST have an observable lifecycle (at minimum: pending, accepted, and revoked/expired), and an invitation that is not pending MUST NOT grant membership when acted upon.
- **FR-010**: System MUST NOT create a duplicate membership when an invitation targets a person who is already a member of the tenant.
- **FR-011**: System MUST determine which tenant a request targets from the requested workspace address.
- **FR-012**: System MUST grant access to a tenant workspace only when the authenticated user is a member of that tenant, cross-checking the resolved tenant against the authenticated session.
- **FR-013**: When an authenticated user requests a tenant workspace they are not a member of — or one that does not exist — the system MUST deny access without revealing whether that tenant exists.
- **FR-014**: Every tenant-owned record MUST be associated with exactly one tenant.
- **FR-015**: System MUST scope every data operation within a request to a single tenant for the full duration of that request.
- **FR-016**: System MUST enforce tenant data isolation at the data-storage layer such that a request scoped to one tenant cannot read, modify, or delete another tenant's records, independent of any application-level filtering.
- **FR-017**: An omission or defect in application-level tenant filtering MUST NOT result in one tenant's data being exposed to, modified by, or deleted by another tenant.
- **FR-018**: The storage-layer isolation MUST be enforced through a path that application code cannot disable or bypass during normal request handling.
- **FR-019**: The project MUST include automated tests that prove cross-tenant isolation by demonstrating that reads and writes performed on behalf of one tenant cannot reach another tenant's data even when application-level tenant filters are deliberately omitted.
- **FR-020**: Authentication and invitation errors (incorrect credentials, duplicate email, invalid or expired invitation) MUST produce clear messages to the user without disclosing information that aids account enumeration or unauthorized access.

### Key Entities

- **Platform User**: An individual identity that can authenticate to the platform. Holds an email address, authentication credentials, and basic profile data. Exists independently of any tenant and may belong to many tenants.
- **Tenant**: An isolated workspace (organization/account) that owns all business data created within it. Holds a display name, a unique workspace identifier (slug), and creation metadata.
- **Membership**: The association that links a platform user to a tenant and conveys access to that tenant's workspace. A user-tenant pair is unique; a user may have many memberships and a tenant may have many members.
- **Invitation**: A pending grant of membership in a specific tenant, addressed to an email address. Has a status (pending, accepted, revoked/expired), the issuing tenant, the inviting member, and the target email.
- **Session**: The authenticated context for a platform user, used to verify identity and to cross-check tenant access on each request.
- **Tenant-Scoped Record**: Any business data row owned by, and only ever visible within, a single tenant.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A brand-new visitor can go from no account to a created, addressable tenant workspace in under 3 minutes.
- **SC-002**: A tenant member can invite a teammate, and that teammate can accept and reach the shared workspace, in under 2 minutes of active effort.
- **SC-003**: 100% of automated cross-tenant isolation tests pass: operations performed on behalf of tenant A never read, modify, or delete tenant B's data.
- **SC-004**: In automated testing, queries that deliberately omit the application-level tenant filter return zero records belonging to any other tenant across every operation tested (0 leaks).
- **SC-005**: An authenticated user attempting to access a tenant they do not belong to is denied 100% of the time, and the denial response is indistinguishable from that for a non-existent tenant.
- **SC-006**: A user who belongs to multiple tenants sees only the active tenant's data in each workspace, with zero cross-tenant contamination observed across all tested workspace switches.
- **SC-007**: The full exit-criteria journey — sign up, create a tenant, invite a teammate, teammate joins — completes end to end in a single demonstrable run, with the cross-tenant isolation suite reporting green.

## Assumptions

- Authentication uses email-and-password credentials with a server-managed session. Single sign-on, social login, multi-factor authentication, password reset, and email-address verification are out of scope for this phase.
- Invitations are delivered to the invitee's email address; the existence of an email-delivery capability is assumed, but rich email templating and deliverability tuning are out of scope for this phase.
- A tenant's workspace identifier (slug) is either supplied by the creator or derived from the tenant name; the system validates it for uniqueness and for being safe to use in a workspace address.
- The tenant role model for this phase is minimal: every member of a tenant may invite other people to that tenant. Granular role-based permissions (admin/member tiers, scoped capabilities) are deferred to a later phase.
- A memberless tenant (after its last member leaves) is retained but inaccessible until a member is re-added; full tenant deletion/offboarding is out of scope for this phase.
- This feature builds on the Phase 0 foundations (running services, a PostgreSQL database, and the migration workflow); no new infrastructure provisioning is required.
- The data-storage isolation mechanism relies on the request operating under a restricted, non-privileged data-access identity so that application code cannot escalate past the isolation boundary.
- No sending, billing, or other business features are in scope; Phase 1 delivers only the tenancy boundary and the accounts, tenants, memberships, and invitations needed to demonstrate it.
