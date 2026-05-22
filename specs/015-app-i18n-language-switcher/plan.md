# Implementation Plan: App Internationalization with Settings-Based Language Switcher

**Branch**: `015-app-i18n-language-switcher` | **Date**: 2026-05-22 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/015-app-i18n-language-switcher/spec.md`

## Summary

Make the React frontend display its entire interface in English or Russian, with English
as the default. The active language is a **per-user account preference** persisted in
Postgres (`platform_users.locale`) so it follows the user across devices. A signed-in
user changes it from a new platform-plane **Account** settings page; the change applies
without a page reload and **never** appears in the URL. First-time and signed-out
visitors are served a language negotiated from their browser; partially translated
languages stay usable because every missing string falls back to English.

Technical approach: adopt **react-i18next** (no URL-locale coupling — see
[research.md](research.md) D1). UI strings move into static JSON catalogs bundled by
Vite. A `nv_locale` cookie mirrors the effective locale so TanStack Start's SSR render
picks the right language with no flash. The Go `auth` context gains a `Locale` value
object, a `SetLocale` command, and a `PUT /api/platform/me` endpoint; `GET /me` starts
returning the stored locale.

## Technical Context

**Language/Version**: TypeScript 5.x (frontend); Go 1.25 (backend, unchanged).

**Primary Dependencies**: Frontend — React 19.2, TanStack Start 1.166 (SSR via Nitro),
TanStack Router/Query, Vite 7, Vitest 3. **New**: `i18next`, `react-i18next`,
`i18next-browser-languageDetector`; `i18next-cli` (devDependency). Backend — chi, pgx
(unchanged).

**Storage**: PostgreSQL. One new nullable column `platform_users.locale` (control-plane
table, no RLS). NULL means "never explicitly chosen".

**Testing**: Vitest + Testing Library (frontend); `go test ./...` with testcontainers
PostgreSQL (backend).

**Target Platform**: Web application (server-rendered SPA behind a Nitro BFF + Go API).

**Project Type**: Web — Go backend (`internal/`, `cmd/`) + TanStack Start frontend
(`frontend/`).

**Performance Goals**: Language switch reflected in the UI in well under the 5 s budget
of SC-001 (in practice immediate — catalogs are bundled, no network fetch for English,
one lazy chunk for Russian). SSR first paint shows the correct language with no flash.

**Constraints**: Language MUST NOT appear in the URL path or query string (FR-006). No
manual page reload on switch (FR-005). Missing translations MUST fall back to English,
never show raw keys (FR-010). `<html lang>`/`dir` MUST track the active locale (FR-011).

**Scale/Scope**: 2 locales (en, ru). ~50 screens / one frontend app. ~Hundreds of UI
strings migrated from hardcoded JSX into namespaced catalogs.

## Constitution Check

*GATE: evaluated against `.specify/memory/constitution.md` v1.1.0.*

| Principle | Assessment |
|-----------|------------|
| I. Tenant Isolation | **Pass / not engaged.** `locale` lives on the control-plane `platform_users` table (a personal account attribute), not on any tenant-scoped table. No RLS surface is touched and no tenant data is read or written. The clean control-plane vs tenant-plane split is preserved. |
| II. Test-Backed Delivery | **Pass (with required tasks).** The `PUT /api/platform/me` endpoint and the `SetLocale` command get integration coverage against a real DB; a catalog-parity test asserts `ru` has every key `en` has; frontend tests cover the switch, persistence, browser-detection precedence, and English fallback. |
| III. Incremental, Shippable Phases | **Pass.** Three independently shippable user stories. The English fallback (Story 3) is `fallbackLng: 'en'` — it ships with the foundational setup, so partial per-namespace translation rollout is safe from day one. |
| IV. Security & Consent by Design | **Pass.** The locale endpoint is behind `requireUser`; it mutates only the *authenticated* user resolved from the session (`userFromContext`) — a user cannot write another user's locale. No new external service or credential. The change goes through the audited command path. |
| V. Operable & Observable | **Pass.** `SetLocale` is wrapped by the standard `decorator.ApplyResultCommandDecorators`, so it gets logging/metrics like every other use case. Services stay stateless — the locale is in the DB and a cookie, never in process memory. |
| VI. Layered Architecture & Domain Integrity | **Pass.** A `Locale` value object makes an unsupported locale unrepresentable (validating constructor). `User` gains the field via a validating setter, hydration path unchanged. The new `UpdateLocale` repository method is declared on the domain-owned `UserRepository` interface and implemented by the pgx adapter. Errors use `apperr` typed kinds, mapped to HTTP once in `Server.fail`. Wiring stays in the single composition root `internal/service/application.go`. |

**Result: PASS — no violations. Complexity Tracking table omitted (nothing to justify).**

## Project Structure

### Documentation (this feature)

```text
specs/015-app-i18n-language-switcher/
├── plan.md              # This file
├── research.md          # Phase 0 — library choice, SSR/no-flash, precedence rules
├── data-model.md        # Phase 1 — Locale value object, schema, cookie
├── quickstart.md        # Phase 1 — setup, adding a string, verification
├── contracts/
│   ├── platform-api.md  # GET /me (+locale) and PUT /api/platform/me
│   └── frontend-i18n.md # catalog layout, namespaces, key conventions, fallback
├── checklists/
│   └── requirements.md  # Spec quality checklist (from /speckit-specify)
└── tasks.md             # Phase 2 — created later by /speckit-tasks
```

### Source Code (repository root)

```text
frontend/
├── i18next.config.ts                 # NEW — i18next-cli config (type-gen + lint)
├── src/
│   ├── i18n/
│   │   ├── index.ts                  # NEW — i18next instance + react-i18next init
│   │   ├── config.ts                 # NEW — SUPPORTED_LOCALES, DEFAULT_LOCALE, dir map
│   │   ├── detect.ts                 # NEW — server/client effective-locale resolution
│   │   └── resources.d.ts            # NEW — generated typed-resource augmentation
│   ├── locales/
│   │   ├── en/                       # NEW — English catalogs (one JSON per namespace)
│   │   │   ├── common.json
│   │   │   ├── auth.json
│   │   │   ├── account.json
│   │   │   └── …                     # settings, lists, subscribers, campaigns, …
│   │   └── ru/                       # NEW — Russian catalogs, same namespaces
│   ├── hooks/
│   │   └── use-locale.ts             # NEW — read/active locale + change + persist to DB
│   ├── components/
│   │   └── settings/language-select.tsx  # NEW — the switcher control
│   ├── routes/
│   │   ├── __root.tsx                # EDIT — I18nextProvider, dynamic <html lang>/dir
│   │   └── account/index.tsx         # NEW — platform-plane Account settings page
│   ├── lib/
│   │   ├── api.ts                    # EDIT — add updateMyLocale(); me() returns locale
│   │   └── api-types.ts              # EDIT — PlatformUser.locale, AccountLocaleInput
│   └── server/
│       └── …                         # locale cookie read in TanStack Start SSR entry
└── (existing components)             # EDIT incrementally — hardcoded strings → t()

internal/
├── db/migrations/
│   ├── 000021_user_locale.up.sql     # NEW — ALTER TABLE platform_users ADD locale
│   └── 000021_user_locale.down.sql   # NEW
├── auth/
│   ├── domain/
│   │   ├── locale.go                 # NEW — Locale value object (supported set)
│   │   ├── user.go                   # EDIT — locale field, validating setter, hydrate
│   │   └── repository.go             # EDIT — UserRepository.UpdateLocale
│   ├── adapters/
│   │   └── users_pg.go               # EDIT — UpdateLocale + locale in SELECTs
│   └── app/
│       ├── application.go            # EDIT — Commands.SetLocale
│       ├── command/set_locale.go     # NEW — SetLocale command handler
│       └── query/authenticate_session.go  # EDIT — AuthenticatedUser.Locale
├── api/
│   ├── platform_handlers.go          # EDIT — handleMe returns locale; handleUpdateMe
│   └── server.go                     # EDIT — register PUT /api/platform/me
└── service/application.go            # EDIT — wire SetLocale into the auth Application
```

**Structure Decision**: Web layout. The frontend gains a self-contained `src/i18n/`
module and a `src/locales/` catalog tree; existing components are edited in place to
replace hardcoded strings with `t()` calls. The backend change is confined to the
existing `auth` bounded context plus one migration and one route registration — no new
context, consistent with "layer scope proportional to need".

## Phase 0 — Research

See [research.md](research.md). Decisions resolved:

- **D1** — i18n library: **react-i18next** (vs Intlayer). Rationale: the no-URL-locale
  requirement makes Intlayer's routing-centric TanStack Start integration dead weight;
  react-i18next's no-routing model is a native fit. Plugin set kept minimal
  (`i18next-browser-languageDetector`, `i18next-cli`); runtime backends rejected as
  YAGNI since catalogs are static bundled assets.
- **D2** — Persistence: nullable `platform_users.locale` column. NULL distinguishes
  "never chose" (needed for the FR-008 adopt-on-sign-in rule) from an explicit English
  choice.
- **D3** — SSR no-flash: a `nv_locale` cookie mirrors the effective locale; TanStack
  Start's server entry reads it to seed i18next `lng` and `<html lang>`.
- **D4** — Locale precedence: DB preference (signed-in) → `nv_locale` cookie → browser
  (`Accept-Language` on server / `navigator` on client) → default English.
- **D5** — Switcher location: a new platform-plane `Account` page (`/account`), since
  locale is a personal preference and the existing `/t/$slug/settings` page is
  workspace-scoped.

## Phase 1 — Design & Contracts

- **Data model** — [data-model.md](data-model.md): the `Locale` value object and its
  supported set, the `platform_users.locale` column, the `User` entity change, and the
  `nv_locale` cookie.
- **Contracts** — [contracts/platform-api.md](contracts/platform-api.md) (the extended
  `GET /api/platform/me` and the new `PUT /api/platform/me`) and
  [contracts/frontend-i18n.md](contracts/frontend-i18n.md) (catalog layout, namespace
  list, key-naming convention, fallback and interpolation rules).
- **Quickstart** — [quickstart.md](quickstart.md): installing the deps, the i18next
  init, adding a translatable string, and verifying the switch + fallback.
- **Agent context**: the `<!-- SPECKIT -->` block in `CLAUDE.md` is repointed to this
  plan.

## Complexity Tracking

No constitution violations — section intentionally empty.
