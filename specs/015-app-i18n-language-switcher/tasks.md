---
description: "Task list for App Internationalization with Settings-Based Language Switcher"
---

# Tasks: App Internationalization with Settings-Based Language Switcher

**Input**: Design documents from `specs/015-app-i18n-language-switcher/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/

**Tests**: Test tasks ARE included — Constitution II (Test-Backed Delivery) is
non-negotiable, and `contracts/frontend-i18n.md` defines explicit testing obligations.

**Organization**: Tasks are grouped by user story so each story can be implemented and
tested independently.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[Story]**: US1 / US2 / US3 — maps to the spec's user stories
- Every task lists exact file paths.

## Path Conventions

Web app: Go backend under `internal/`, TanStack Start frontend under `frontend/`.

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Pull in the i18n toolchain and create the configuration skeleton.

- [X] T001 Add `i18next`, `react-i18next`, `i18next-browser-languageDetector` to dependencies and `i18next-cli` to devDependencies in `frontend/package.json` (run `pnpm add` from `frontend/`).
- [X] T002 [P] Create `frontend/i18next.config.ts` — the `i18next-cli` config for typed-resource generation and missing/unused-key linting (locales `en`, `ru`; catalogs under `src/locales`).
- [X] T003 [P] Create `frontend/src/i18n/config.ts` exporting `SUPPORTED_LOCALES`, `DEFAULT_LOCALE`, `LOCALE_COOKIE` (`"nv_locale"`), `localeDir`, and `localeLabel` per `contracts/frontend-i18n.md`.
- [X] T004 [P] Add `i18n:types` (run `i18next-cli`) and `i18n:lint` scripts to `frontend/package.json`, and include the generated `src/i18n/resources.d.ts` in `frontend/tsconfig.json`.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: The i18n runtime, the backend locale schema/domain, and the test scaffolding every story builds on.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete.

### Backend — schema & domain

- [X] T005 Create migration `internal/db/migrations/000021_user_locale.up.sql` and `000021_user_locale.down.sql` — add nullable `locale text CHECK (locale IS NULL OR locale IN ('en','ru'))` to `platform_users` (see data-model.md).
- [X] T006 [P] Create the `Locale` value object in `internal/auth/domain/locale.go` — supported set `en`/`ru`, default `en`, `NewLocale`/`HydrateLocale`/`IsZero`/direction lookup; `NewLocale` rejects unsupported codes with an `apperr` incorrect-input error of kind `unsupported_locale`.
- [X] T007 Extend the `User` entity in `internal/auth/domain/user.go` — add the unexported `locale Locale` field, a validating `SetLocale(Locale)` mutator, a `Locale()` accessor, and a `locale` parameter on `HydrateUser` (depends on T006).
- [X] T008 Add `UpdateLocale(ctx, userID string, locale Locale) error` to the `UserRepository` interface in `internal/auth/domain/repository.go`.
- [X] T009 In `internal/auth/adapters/users_pg.go` implement `UpdateLocale` (positional `$1/$2`, `ErrUserNotFound` when no row) and add `locale` to the `GetByID`/`LookupByEmail`/`GetCredentials` SELECT lists, scanning a nullable string into `HydrateUser` (depends on T005, T007, T008).
- [X] T010 Add `Locale string` to the `AuthenticatedUser` read model in `internal/auth/app/query/authenticate_session.go` and populate it from `user.Locale()` (depends on T007).

### Frontend — i18n runtime

- [X] T011 [P] Create seed catalogs `frontend/src/locales/en/common.json` and `frontend/src/locales/ru/common.json` with the shared shell keys (matching key sets).
- [X] T012 Create the i18next instance in `frontend/src/i18n/index.ts` — `fallbackLng: "en"`, `supportedLngs`, `returnEmptyString: false`, `react.useSuspense: false`, a dev-only `missingKeyHandler`, and `i18next-browser-languageDetector` (`order: ["cookie","navigator"]`, `lookupCookie: "nv_locale"`, `caches: ["cookie"]`); English eager, Russian lazy `import()` (depends on T003, T011).
- [X] T013 Create `frontend/src/i18n/detect.ts` — effective-locale resolution implementing the research.md D4 precedence (DB → cookie → browser → default), with separate server and client entry points (depends on T003).
- [X] T014 Wrap the app in `I18nextProvider` and make `<html lang>`/`dir` dynamic from the resolved locale in `frontend/src/routes/__root.tsx` (depends on T012, T013).
- [X] T015 Generate `frontend/src/i18n/resources.d.ts` typed-resource augmentation by running the `i18n:types` script, and **commit the generated file** (it is version-controlled; a CI drift check in T044 keeps it fresh) (depends on T002, T011).

### Test scaffolding

- [X] T016 [P] Unit test for the `Locale` value object in `internal/auth/domain/locale_test.go` — supported set, rejection of unsupported codes, `HydrateLocale` of an unknown value → zero/unset (depends on T006).
- [X] T017 [P] Integration test for `UpdateLocale` in `internal/auth/adapters/users_pg_test.go` — round-trips a locale, returns `ErrUserNotFound` for an unknown id (depends on T009).

**Checkpoint**: i18n runtime live with English-fallback; backend can store a locale. User stories can now begin.

---

## Phase 3: User Story 1 - Switch the interface language from settings (Priority: P1) 🎯 MVP

**Goal**: A signed-in user changes the language on a new Account settings page; the UI re-renders with no URL change and no reload, and the choice persists per-account across devices.

**Independent Test**: Sign in, open `/account`, switch to Russian and save → UI text changes, URL unchanged, no reload. Sign in again in a second browser → still Russian. Force the save to fail → language reverts with an error.

### Tests for User Story 1

- [X] T018 [P] [US1] Contract test for `PUT /api/platform/me` and `GET /api/platform/me` (locale field) in `internal/api/platform_handlers_test.go` — 200 on a supported locale, 422 `unsupported_locale` otherwise, `nv_locale` cookie set on success, one user cannot affect another.
- [X] T019 [P] [US1] Test for the `SetLocale` command in `internal/auth/app/command/set_locale_test.go` — valid locale persists, unsupported locale returns the typed error, unknown user returns `ErrUserNotFound`.
- [X] T020 [P] [US1] Test for the `useLocale` hook in `frontend/src/hooks/use-locale.test.ts` — `setLocale` changes the active language, updates the cookie, calls `updateMyLocale`, and reverts the language on API failure.
- [X] T021 [P] [US1] Test for the Account page in `frontend/src/routes/account/index.test.tsx` — renders the switcher with the current locale marked, selecting + saving issues the update.

### Implementation for User Story 1 — backend

- [X] T022 [US1] Create the `SetLocale` command handler in `internal/auth/app/command/set_locale.go` — validates the locale via `NewLocale`, loads the user, calls `User.SetLocale`, persists via `UserRepository.UpdateLocale` (depends on T007, T008).
- [X] T023 [US1] Add `SetLocale` to `Commands` in `internal/auth/app/application.go`.
- [X] T024 [US1] Wire `SetLocale` through `decorator.ApplyResultCommandDecorators` in the composition root `internal/service/application.go` (depends on T022, T023).
- [X] T025 [US1] In `internal/api/platform_handlers.go`, extend `handleMe` to emit `user.locale` and add `handleUpdateMe` for `PUT /api/platform/me` — decode `{locale}`, call `SetLocale`, set the `nv_locale` cookie, return the updated user (depends on T024).
- [X] T026 [US1] Register `PUT /api/platform/me` under the `requireUser` group in `internal/api/server.go` (depends on T025).

### Implementation for User Story 1 — frontend

- [X] T027 [P] [US1] Add `PlatformUser.locale` and `AccountLocaleInput` to `frontend/src/lib/api-types.ts` and an `updateMyLocale` method to `frontend/src/lib/api.ts`.
- [X] T028 [US1] Create the `useLocale` hook in `frontend/src/hooks/use-locale.ts` — exposes `{ locale, setLocale, supportedLocales }`; `setLocale` changes the language, updates the cookie + `<html lang>`/`dir`, persists via `updateMyLocale` when signed in, and reverts + toasts on failure (depends on T012, T027).
- [X] T029 [P] [US1] Create the `LanguageSelect` component in `frontend/src/components/settings/language-select.tsx` using `useLocale` and `localeLabel` (depends on T028).
- [X] T030 [US1] Create the platform-plane Account route + page at `frontend/src/routes/account/index.tsx` hosting `LanguageSelect` (depends on T029).
- [X] T031 [P] [US1] Create `account` namespace catalogs `frontend/src/locales/en/account.json` and `frontend/src/locales/ru/account.json`.
- [X] T032 [US1] Add an Account nav entry and migrate the app-shell strings to `t()` in `frontend/src/components/shell/app-shell.tsx`, `sidebar.tsx`, and `top-bar.tsx` (depends on T030).
- [X] T033 [US1] On `me()` resolving, override the detector when the signed-in user's stored `locale` is set — sync it into i18next and the cookie (in `frontend/src/i18n/detect.ts` or the root data loader) (depends on T028).

**Checkpoint**: User Story 1 fully functional — the MVP. Language switch works, persists per-account, no URL change.

---

## Phase 4: User Story 2 - Sensible default language for new and signed-out visitors (Priority: P2)

**Goal**: First-time and signed-out visitors (including on the login/signup pages) get a language matching their browser when supported, default English otherwise; an earlier manual choice is honored; a signed-out choice is adopted on sign-in when the account has none.

**Independent Test**: Clear cookies, set the browser to Russian, open `/login` → Russian; set the browser to an unsupported language → English; a prior `nv_locale` cookie wins over the browser.

### Tests for User Story 2

- [X] T034 [P] [US2] Test detection precedence in `frontend/src/i18n/detect.test.ts` — cookie beats browser, browser first-supported-match, unsupported browser language → default.
- [X] T035 [P] [US2] Backend test in `internal/api/platform_handlers_test.go` — `login`/`signup` set the `nv_locale` cookie, and FR-008 adoption persists a cookie-supplied locale only when the account's stored locale is NULL.

### Implementation for User Story 2

- [X] T036 [US2] Implement `Accept-Language` negotiation in the server path of `frontend/src/i18n/detect.ts` — first supported match, default fallback (depends on T013).
- [X] T037 [US2] In `internal/api/platform_handlers.go`, set the `nv_locale` cookie to the effective locale on `handleLogin`/`handleSignup`/`handleAcceptInvitation`, and adopt the request's `nv_locale` cookie as the account preference when the user's stored `locale` is NULL (depends on T009).
- [X] T038 [P] [US2] Create `auth` namespace catalogs `frontend/src/locales/en/auth.json` and `frontend/src/locales/ru/auth.json`.
- [X] T039 [US2] Migrate the strings in `frontend/src/routes/login.tsx`, `signup.tsx`, and the invitation route to `t()` (depends on T038).
- [X] T040 [US2] Seed the i18next `lng` from the cookie / `Accept-Language` in the TanStack Start SSR server entry so the first server render has the right language with no flash (depends on T013, T014).

**Checkpoint**: User Stories 1 AND 2 both work — signed-in switching and signed-out detection.

---

## Phase 5: User Story 3 - Graceful fallback for missing translations (Priority: P3)

**Goal**: Missing translations render the English text, never a blank or a raw key, so partially translated languages stay usable.

**Independent Test**: Remove a key from a `ru/*.json` file → that text shows in English, the rest of the page is unaffected, no raw `ns:key` appears anywhere.

### Tests for User Story 3

- [X] T041 [P] [US3] Catalog-parity test in `frontend/src/i18n/catalog-parity.test.ts` — every key in each `ru/*.json` exists in the matching `en/*.json`.
- [X] T042 [P] [US3] Fallback test in `frontend/src/i18n/fallback.test.ts` — a key missing in `ru` resolves to the English value and never returns a raw `ns:key` string.

### Implementation for User Story 3

- [X] T043 [US3] Confirm/finalize `fallbackLng: "en"`, `returnEmptyString: false`, and the `missingKeyHandler` (dev-throw, prod-silent) in `frontend/src/i18n/index.ts`.
- [X] T044 [US3] Update the frontend job in `.github/workflows/ci.yml` to add, after `Install`: a generated-types drift check `pnpm i18n:types && git diff --exit-code` (fails the build if the committed `src/i18n/resources.d.ts` is stale), a `pnpm typecheck` step (currently missing from CI — gates the typed `t()` keys), and a `pnpm i18n:lint` step (missing/unused keys, en/ru parity). Final frontend job order: install → i18n drift check → typecheck → lint → i18n:lint → test → build (depends on T002, T004, T015).

**Checkpoint**: All three user stories independently functional.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Complete FR-012 coverage by migrating every remaining feature area's strings, plus final verification. The English fallback (US3) keeps these increments safe.

- [ ] T045 [P] Add `settings` catalogs and migrate `frontend/src/routes/t/$slug/settings/` strings to `t()`.
- [ ] T046 [P] Add `lists` catalogs and migrate the lists route strings to `t()`.
- [ ] T047 [P] Add `subscribers` catalogs and migrate the subscribers route strings to `t()`.
- [ ] T048 [P] Add `campaigns` + `templates` catalogs and migrate those route strings to `t()`.
- [ ] T049 [P] Add `analytics` + `billing` catalogs and migrate those route strings to `t()`.
- [ ] T050 [P] Add `media` catalogs and migrate the media route strings to `t()`.
- [ ] T051 [P] Add `errors` catalogs and route user-facing error/validation messages through `t()` (`frontend/src/lib/errors.ts` and consumers).
- [X] T052 Add a `ci` target to the root `Makefile` (and to `.PHONY`) that reproduces `.github/workflows/ci.yml` locally, running every step in the same order: backend — `go build ./...` → `go test ./...` → `make lint-arch` → `golangci-lint run`; then frontend — `cd frontend && pnpm install --frozen-lockfile && pnpm i18n:types && git diff --exit-code && pnpm typecheck && pnpm lint && pnpm i18n:lint && pnpm test && pnpm build`. Keep it a faithful mirror of `ci.yml` so a green `make ci` predicts a green pipeline (depends on T044).
- [ ] T053 Run the `quickstart.md` manual verification for all three stories; update `docs/` if any guidance changed.
- [ ] T054 Run `make ci` end-to-end and confirm it passes (backend + tenant-isolation tests, clean migration apply, frontend typecheck/lint/i18n:lint/test/build) (depends on T052).

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: no dependencies — start immediately.
- **Foundational (Phase 2)**: depends on Setup — **blocks all user stories**.
- **User Stories (Phase 3–5)**: all depend on Foundational. US1 is the MVP; US2 reuses the foundational repository for adoption; US3 depends only on the i18n runtime.
- **Polish (Phase 6)**: depends on the i18n runtime + US3's fallback being in place.

### User Story Dependencies

- **US1 (P1)**: after Foundational. Self-contained — the MVP.
- **US2 (P2)**: after Foundational. Independently testable for detection; the FR-008 adoption scenario reuses the foundational `UpdateLocale` repository.
- **US3 (P3)**: after Foundational. Independent — exercises the i18n runtime's fallback only.

### Within Each User Story

- Tests are written before implementation and must fail first.
- Backend: domain → command → composition → handler → route.
- Frontend: api/types → hook → component → route → string migration.

### Parallel Opportunities

- Setup: T002, T003, T004 in parallel.
- Foundational: T006 with T011; T016 and T017 in parallel after their deps.
- US1 tests T018–T021 in parallel; T027, T029, T031 in parallel.
- US2 tests T034, T035 in parallel; T038 parallel with backend work.
- US3 tests T041, T042 in parallel.
- Polish: T045–T051 are all `[P]` — independent catalog/route files.

---

## Parallel Example: User Story 1

```bash
# Tests for User Story 1 together:
Task: "Contract test for PUT/GET /api/platform/me in internal/api/platform_handlers_test.go"
Task: "SetLocale command test in internal/auth/app/command/set_locale_test.go"
Task: "useLocale hook test in frontend/src/hooks/use-locale.test.ts"
Task: "Account page test in frontend/src/routes/account/index.test.tsx"

# Independent frontend files together:
Task: "api-types.ts + api.ts updateMyLocale"
Task: "LanguageSelect component"
Task: "account namespace catalogs"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Phase 1: Setup.
2. Phase 2: Foundational (blocks everything).
3. Phase 3: User Story 1.
4. **STOP and VALIDATE**: switch language on `/account`, confirm persistence across devices, no URL change.
5. Deploy/demo.

### Incremental Delivery

1. Setup + Foundational → i18n runtime + locale storage ready.
2. US1 → per-account switching (MVP) → demo.
3. US2 → signed-out / first-visit detection → demo.
4. US3 → fallback guarantee + CI key-lint → demo.
5. Polish → migrate remaining feature areas to complete FR-012; the English fallback keeps each increment safe.

---

## Notes

- `[P]` = different files, no dependency on incomplete tasks.
- `[Story]` labels trace tasks to spec user stories; Setup/Foundational/Polish carry none.
- Constitution II: every user story exits with its tests green; T054 runs the full `make ci` bundle.
- Constitution IV: the locale endpoint mutates only the session-resolved user — keep that invariant in T025.
- Commit after each task or logical group.
