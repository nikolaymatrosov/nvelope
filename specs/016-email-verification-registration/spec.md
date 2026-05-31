# Feature Specification: Email Verification on Registration

**Feature Branch**: `016-email-verification-registration`

**Created**: 2026-05-22

**Status**: Draft

**Input**: User description: "I want to verify users email he provides on registration via sending him email with link. Emails should be sent from service domain nvelope.ru (provided via config) also via Postbox. Also I want to be able to limit registrations from given domains provided via config."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Verify email before account is usable (Priority: P1)

A new person signs up for the platform with their email address, a password, and a name. Instead of being signed straight in, they are told to check their inbox. They receive an email from the service ("from" address on the configured service domain) containing a verification link. Clicking the link confirms ownership of the address and marks the account as verified. Only then can the person sign in and use the platform.

**Why this priority**: This is the core of the feature. Without it there is no proof that a registrant controls the address they entered, and the platform — which sends email on behalf of its users — cannot trust that contact channel. It delivers standalone value the moment it works.

**Independent Test**: Register a new account, confirm no session is granted, confirm a verification email is delivered, click the link, then confirm the account can now sign in. Fully testable on its own.

**Acceptance Scenarios**:

1. **Given** a visitor on the registration page, **When** they submit a valid email, password, and name, **Then** the account is created in an unverified state, no session is issued, and a verification email is sent to that address.
2. **Given** an unverified account, **When** the person attempts to sign in, **Then** sign-in is refused with a message explaining the email must be verified first.
3. **Given** a valid, unexpired verification link, **When** the person opens it, **Then** the account becomes verified and the person can sign in normally afterward.
4. **Given** a verification link that has already been used, **When** the person opens it again, **Then** the system reports the account is already verified and does not error.
5. **Given** a verification link, **When** it is opened after its validity window has elapsed, **Then** verification is refused and the person is offered a way to request a new link.

### User Story 2 - Restrict registration to permitted email domains (Priority: P2)

The platform operator maintains a configured list of permitted email domains. When someone tries to register with an email address whose domain is not on that list, registration is refused before any account is created and before any email is sent. When the list permits their domain, registration proceeds normally.

**Why this priority**: Lets the operator confine sign-ups to a known population (e.g. a single company or set of partner organisations). It is valuable but independent of verification — the platform is still useful with verification alone — so it ranks below P1.

**Independent Test**: With a domain allowlist configured, attempt to register with an in-list domain (succeeds) and an out-of-list domain (refused with a clear message). Testable without the verification flow.

**Acceptance Scenarios**:

1. **Given** a configured allowlist of permitted domains, **When** a visitor registers with an email whose domain is on the list, **Then** registration proceeds.
2. **Given** a configured allowlist, **When** a visitor registers with an email whose domain is not on the list, **Then** registration is refused with a message stating their email domain is not permitted, no account is created, and no email is sent.
3. **Given** no allowlist configured (empty list), **When** a visitor registers with any valid email, **Then** registration proceeds unrestricted.
4. **Given** an allowlist, **When** the domain comparison is performed, **Then** it is case-insensitive (e.g. `Example.COM` matches `example.com`).

### User Story 3 - Request a new verification email (Priority: P3)

A person who registered but did not verify in time — link expired, email lost, or never arrived — can request that a fresh verification email be sent to the address on their account.

**Why this priority**: A recovery path that materially reduces support load and abandoned sign-ups, but the feature still functions without it (a person could re-register). Lowest priority of the three.

**Independent Test**: For an unverified account, trigger a resend, confirm a new email arrives, confirm the new link verifies the account, and confirm any earlier link no longer works.

**Acceptance Scenarios**:

1. **Given** an unverified account, **When** the person requests a new verification email, **Then** a fresh link is sent to the account's email address and the most recent link supersedes earlier ones.
2. **Given** an already-verified account, **When** a resend is requested, **Then** no email is sent and the person is told the account is already verified.
3. **Given** repeated resend requests in a short period, **When** the requests exceed a reasonable rate, **Then** further sends are throttled to prevent inbox flooding and abuse.

### Edge Cases

- A person registers with an email that already belongs to an existing account → registration is refused with the existing "email already in use" behaviour; no verification email is sent.
- The verification email fails to send (delivery provider unavailable) → the account remains unverified and the person can use the resend path once the provider recovers; the registration itself is not rolled back.
- A verification link is tampered with or does not correspond to any pending verification → verification is refused with a generic "invalid or expired link" message that does not reveal whether an account exists.
- The configured service sender domain is missing or misconfigured → registration that would trigger an email is refused or surfaces an operator-facing error rather than silently creating unverifiable accounts.
- A domain on the allowlist is entered with surrounding whitespace or mixed case → it is normalised before comparison.
- A person whose domain was on the allowlist when they registered is later removed from the list → their existing account is unaffected; the allowlist is only evaluated at registration time.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST allow a visitor to register with an email address, password, and name.
- **FR-002**: On successful registration, System MUST create the account in an "unverified" state and MUST NOT issue an authenticated session.
- **FR-003**: System MUST send a verification email containing a unique, single-use link to the address provided at registration.
- **FR-004**: The verification email's sender address MUST use the service sender domain, which is supplied via configuration (e.g. `nvelope.ru`).
- **FR-005**: System MUST deliver the verification email through the platform's existing transactional email delivery channel (Postbox).
- **FR-006**: Each verification link MUST be valid only for a bounded time window, after which it is rejected.
- **FR-007**: When a person opens a valid, unexpired verification link, System MUST mark the corresponding account as verified.
- **FR-008**: System MUST refuse sign-in for accounts that are not yet verified and MUST return a message that explains verification is required.
- **FR-009**: System MUST allow sign-in normally once an account is verified.
- **FR-010**: Opening an already-used or expired link MUST NOT error; the system MUST report a clear, non-sensitive outcome (already verified, or invalid/expired with an offer to resend).
- **FR-011**: System MUST allow a person with an unverified account to request a new verification email for that account's address.
- **FR-012**: Issuing a new verification link MUST invalidate any previously issued, still-pending link for the same account.
- **FR-013**: System MUST throttle verification-email sends per account to prevent inbox flooding and abuse.
- **FR-014**: System MUST evaluate the registrant's email domain against a configured allowlist of permitted domains before creating an account.
- **FR-015**: When an allowlist is configured and the registrant's email domain is not on it, System MUST refuse registration before creating any account or sending any email, returning a message that the domain is not permitted.
- **FR-016**: When no allowlist is configured (empty list), System MUST permit registration from any otherwise-valid email address.
- **FR-017**: Domain matching against the allowlist MUST be case-insensitive and tolerant of surrounding whitespace in configured values.
- **FR-018**: System MUST NOT reveal, through verification or resend responses, whether a given email address corresponds to an existing account.
- **FR-019**: System MUST keep the allowlist evaluation a registration-time check only; changes to the list MUST NOT retroactively affect already-created accounts.
- **FR-020**: System MUST record when an account became verified.

### Key Entities *(include if data involved)*

- **User Account**: An existing platform identity (email, name, password, locale). Gains a verification state — unverified vs verified — and the time verification occurred.
- **Email Verification Request**: A pending, time-bounded, single-use proof-of-ownership challenge tied to one account. Has an issued time, an expiry time, a used/unused state, and the secret embedded in the verification link.
- **Service Sender Domain**: An operator-configured value identifying the domain that verification (and other service) emails are sent from.
- **Registration Domain Allowlist**: An operator-configured list of email domains permitted to register. Empty means unrestricted.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 100% of newly registered accounts begin in the unverified state with no active session.
- **SC-002**: A verification email is dispatched within 1 minute of a successful registration for every registration that passes the domain check.
- **SC-003**: A person who completes registration and clicks a valid link can sign in within 5 minutes of starting, without operator assistance.
- **SC-004**: 0% of registrations with a disallowed email domain result in a created account or a sent email.
- **SC-005**: At least 90% of registrants who start the flow reach the verified state on their first verification email.
- **SC-006**: An expired or already-used link never produces an error page; it always yields an actionable outcome (resend offered or "already verified").
- **SC-007**: No verification or resend response discloses whether an email address is registered.

## Assumptions

- The registration entry point is the existing platform sign-up flow; this feature changes its outcome (no immediate session) rather than introducing a new flow.
- "Limit registrations from given domains" is implemented as an allowlist: only the listed domains may register, and an empty list disables the restriction.
- Until they verify, an account holder is fully blocked from signing in; no session is issued at registration time.
- The verification link validity window is 24 hours unless the operator configures otherwise; this is a reasonable default for transactional verification emails.
- The verification link directs the recipient to a platform web page that performs the verification and then guides them to sign in.
- Verification emails reuse the platform's existing Postbox transactional delivery integration; no new email provider is introduced.
- The service sender domain is already (or will be) configured for sending in Postbox; deliverability/domain authentication setup is out of scope for this feature.
- The email domain is the portion of the address after the `@`; addresses are already validated for basic format by the existing email value object.
- Existing accounts created before this feature ships are out of scope; this specification governs new registrations only.
- Email content is localised using the platform's existing internationalisation support where a locale is known.
