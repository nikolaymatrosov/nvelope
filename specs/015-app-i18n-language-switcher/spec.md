# Feature Specification: App Internationalization with Settings-Based Language Switcher

**Feature Branch**: `015-app-i18n-language-switcher`

**Created**: 2026-05-22

**Status**: Draft

**Input**: User description: "look at @docs/intl.md and plan how to internationalize our react app. I don't want to put lang as part of url. User should be able to switch lang in the settings"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Switch the interface language from settings (Priority: P1)

A signed-in user opens their settings, picks a language from a list of supported languages, and saves. The entire application interface — navigation, buttons, labels, form hints, validation messages, empty states, toasts, and error messages — immediately re-renders in the chosen language. The choice sticks: when the user returns later, on the same or a different device, the interface is still in their chosen language. The URL never changes when the language changes.

**Why this priority**: This is the core capability the user asked for. Without it there is no way to use the app in a non-default language and no value is delivered. It is also the slice that proves the whole translation infrastructure works end to end.

**Independent Test**: Sign in, change the language in settings, confirm the visible interface text changes and the URL is unchanged; sign out and back in (and from a second device/browser) and confirm the language is retained.

**Acceptance Scenarios**:

1. **Given** a signed-in user viewing the app in the default language, **When** they select a different supported language in settings and save, **Then** the visible interface text re-renders in that language without a full-page reload and without any change to the URL.
2. **Given** a user who previously set a non-default language, **When** they sign in again later in a new browser session, **Then** the interface is presented in their previously chosen language.
3. **Given** a user who set a non-default language on one device, **When** they sign in on a second device, **Then** the interface is presented in the same chosen language.
4. **Given** a user changing the language, **When** the save fails, **Then** the previous language remains in effect and the user is told the change was not saved.

---

### User Story 2 - Sensible default language for new and signed-out visitors (Priority: P2)

A first-time visitor who has never chosen a language — including someone on the login or signup page who has no account yet — sees the app in a language that matches their browser's preferred language when that language is supported. If their preferred language is not supported, they see the app in the default language. They are never shown a blank or broken interface for lack of a stored preference.

**Why this priority**: A good first impression matters, and a visitor cannot rely on the settings switcher (Story 1) until they have an account and are signed in. This makes the feature usable for the pre-authentication surface and for brand-new users. It depends on the translation infrastructure from Story 1 but is independently demonstrable.

**Independent Test**: With browser language set to a supported non-default language, open the login page in a fresh browser (no stored preference) and confirm it appears in that language; repeat with browser language set to an unsupported language and confirm the default language is shown.

**Acceptance Scenarios**:

1. **Given** a visitor with no stored language preference and a browser preferred language that is supported, **When** they load any pre-authentication page, **Then** the interface is shown in that supported language.
2. **Given** a visitor with no stored language preference and a browser preferred language that is not supported, **When** they load the app, **Then** the interface is shown in the default language.
3. **Given** a signed-out visitor who manually picked a language before, **When** they return, **Then** that earlier choice is honored over the browser preferred language.
4. **Given** a visitor who chose a language while signed out and then signs in, **When** their account has its own stored preference, **Then** the account preference takes precedence; if the account has no preference yet, the signed-out choice is adopted as the account preference.

---

### User Story 3 - Graceful fallback for missing translations (Priority: P3)

When the app is displayed in a chosen language but a particular piece of text has not been translated yet, the user still sees readable, meaningful text (in the default language) rather than a blank space, a raw translation key, or a crash. The experience degrades gracefully so that partially translated languages remain usable.

**Why this priority**: Translations are added incrementally and will never be 100% complete at every moment. Without a fallback, any gap produces a visibly broken interface. It is P3 because the app is still valuable with full translations; this protects quality during rollout and ongoing maintenance.

**Independent Test**: Display the app in a supported language with at least one deliberately missing translation entry and confirm the affected text appears in the default language while the rest of the page renders normally.

**Acceptance Scenarios**:

1. **Given** the app displayed in a non-default language, **When** a piece of text has no translation for that language, **Then** the default-language text is shown in its place.
2. **Given** the app displayed in any supported language, **When** the page renders, **Then** no raw translation identifiers/keys are ever visible to the user.

---

### Edge Cases

- A user's stored preference references a language that is later removed from the supported set → the app falls back to the default language and the settings switcher shows the default as selected.
- The browser reports multiple preferred languages → the first supported language in that ordered list is used.
- Locale-dependent content such as dates, times, and numbers is displayed → it is formatted according to the active language's conventions (see FR-009).
- A user changes the language in one open tab → other already-open tabs reflect the change on their next navigation or refresh, not necessarily instantly.
- The app is displayed in a right-to-left language → page layout and text direction adjust accordingly (only relevant if an RTL language is in the supported set; see Assumptions).
- The pre-authentication surface (login, signup, invitation acceptance) must honor the same language detection and switching rules as the rest of the app.
- A user with assistive technology → the document's language is correctly announced so screen readers pronounce content properly.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The system MUST support displaying the entire user-facing interface in two languages at launch: English and Russian.
- **FR-002**: The system MUST designate English as the default language, used whenever no other language can be determined.
- **FR-003**: Users MUST be able to change the interface language from a settings screen, choosing from the list of supported languages.
- **FR-004**: The system MUST persist a signed-in user's chosen language as a personal account-level preference so it applies on every device and browser where that user signs in.
- **FR-005**: The system MUST apply a chosen language change to the interface without requiring the user to manually reload the page, and without altering the URL.
- **FR-006**: The system MUST NOT encode the language in the URL path or query string; language selection MUST be invisible to the URL structure.
- **FR-007**: For visitors with no stored preference, the system MUST detect the preferred language from the browser's reported language settings and use the first supported match, falling back to the default language when none match.
- **FR-008**: The system MUST remember a language chosen by a signed-out visitor for the duration of and across their browser sessions until they sign in or change it; upon sign-in this choice MUST be superseded by an existing account preference, or adopted as the account preference when none exists.
- **FR-009**: The system MUST format locale-sensitive content (dates, times, numbers) according to the conventions of the active language.
- **FR-010**: When a translation is missing for the active language, the system MUST display the default-language text instead and MUST never display raw translation keys/identifiers to the user.
- **FR-011**: The system MUST set the document's language indicator so assistive technologies and browsers correctly identify the active language, and MUST set text direction appropriately for the active language.
- **FR-012**: All user-facing interface text — including navigation, buttons, labels, placeholders, hints, validation messages, empty/loading/error states, and notifications — MUST be translatable; only user-generated content (e.g., campaign names, subscriber data, email body content authored by users) is out of scope.
- **FR-013**: The settings screen MUST clearly indicate which language is currently active.
- **FR-014**: The system MUST handle a stored preference that is no longer a supported language by falling back to the default language without error.

### Key Entities *(include if feature involves data)*

- **Supported Language**: A language the interface can be displayed in. Attributes: identifier, human-readable name (shown in the switcher), whether it is the default, text direction. The set of supported languages is a fixed, curated list.
- **User Language Preference**: A signed-in user's chosen interface language, stored against their account. Optional — absent until the user makes an explicit choice. Relates one-to-one to a user account.
- **Visitor Language Preference**: A language choice held for a signed-out visitor and tied to their browser only (not an account). Superseded by a User Language Preference once the visitor signs in.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A signed-in user can change the interface language and see the change applied in under 5 seconds, with no URL change and no manual page reload.
- **SC-002**: A user's chosen language is retained across 100% of subsequent sign-ins on any device or browser.
- **SC-003**: 100% of user-facing interface text (per FR-012 scope) is translatable; no raw translation keys are ever visible in any supported language.
- **SC-004**: A first-time visitor whose browser preferred language is supported sees the app in that language on first load without taking any action.
- **SC-005**: When a language is partially translated, every screen still renders fully readable content with no blank text, using the default-language fallback for any gaps.
- **SC-006**: 95% of users can locate and change the language setting on their first attempt without assistance.

## Assumptions

- "Internationalize our React app" refers to the web frontend interface only; localizing backend-generated content (e.g., system emails, exported files) is out of scope for this feature unless later specified.
- User-generated content (campaign content, list names, subscriber fields, etc.) is displayed as authored and is not translated.
- The language preference is a per-user personal setting (the user said "user should be able to switch"), not a per-workspace setting; it therefore lives in a user/account-level settings area rather than the existing workspace settings.
- The launch language set is English and Russian, with English as the default (confirmed). Both are left-to-right; FR-011's direction handling still applies if a right-to-left language is added later.
- A signed-out visitor's choice is remembered via standard browser-side storage; no account is required to hold it.
- Existing user accounts and the authentication system are reused; this feature adds a preference to them rather than introducing new account concepts.
- Only one language is active for the whole interface at a time; there is no per-section or mixed-language mode.
