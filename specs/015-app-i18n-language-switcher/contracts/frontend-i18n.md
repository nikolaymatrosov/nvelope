# Contract: Frontend i18n Layout & Conventions

**Feature**: 015-app-i18n-language-switcher | **Date**: 2026-05-22

Defines the catalog layout, key conventions, and runtime behaviour the frontend
implementation must follow so translation work and tests are consistent.

## Catalog layout

```text
frontend/src/locales/
├── en/                      # default locale — source of truth for keys
│   ├── common.json          # shell, nav, generic buttons, shared labels
│   ├── auth.json            # login, signup, invitation acceptance
│   ├── account.json         # the Account settings page + language switcher
│   ├── settings.json        # workspace settings
│   ├── lists.json
│   ├── subscribers.json
│   ├── campaigns.json
│   ├── templates.json
│   ├── analytics.json
│   ├── billing.json
│   ├── media.json
│   └── errors.json          # user-facing error/validation messages
└── ru/                      # Russian — same filenames, same keys
    └── …
```

- One JSON file per **namespace**; namespaces correspond to feature areas.
- `en/` is the **source of truth**: every key that exists in any `ru/` file MUST exist
  in the matching `en/` file. `i18next-cli` lint enforces this in CI.
- English (default) namespaces are bundled eagerly. Russian namespaces are loaded as a
  lazy Vite `import()` chunk fetched only when Russian is the active locale.

## Key naming

- Dot-namespaced, `camelCase` segments: `settings.title`, `auth.login.submit`,
  `common.actions.save`.
- Keys describe *meaning/location*, never the English text.
- Interpolation uses i18next `{{var}}` placeholders: `"greeting": "Hello, {{name}}"`.
- Pluralization uses i18next native plural suffixes (`_one`, `_few`, `_many`, `_other`)
  — correct for Russian via `Intl.PluralRules`; no ICU plugin.

## Runtime configuration (`frontend/src/i18n/`)

`config.ts`:

```ts
export const SUPPORTED_LOCALES = ["en", "ru"] as const
export type Locale = (typeof SUPPORTED_LOCALES)[number]
export const DEFAULT_LOCALE: Locale = "en"
export const LOCALE_COOKIE = "nv_locale"
export const localeDir: Record<Locale, "ltr" | "rtl"> = { en: "ltr", ru: "ltr" }
export const localeLabel: Record<Locale, string> = { en: "English", ru: "Русский" }
```

i18next instance (`index.ts`) — required options:

- `fallbackLng: "en"` — satisfies FR-010 (missing key → English).
- `supportedLngs: ["en", "ru"]`, `nonExplicitSupportedLngs: true`.
- `interpolation.escapeValue: false` (React already escapes).
- `returnEmptyString: false` — an empty string is treated as missing → falls back.
- `react.useSuspense: false` — avoid suspending the existing app shell.
- A `missingKeyHandler` (dev only) that throws/logs so a raw key never ships;
  production silently falls back. **A raw `namespace:key` string MUST never be rendered**
  (FR-010, SC-003).
- Language detector: `i18next-browser-languageDetector`, `order: ["cookie","navigator"]`,
  `lookupCookie: "nv_locale"`, `caches: ["cookie"]`.

## Effective-locale resolution (`detect.ts`)

Implements research.md D4 precedence:

1. signed-in DB preference (from `GET /me` → `user.locale`), when non-null & supported;
2. `nv_locale` cookie;
3. browser — `Accept-Language` (server) / `navigator.languages` (client), first
   supported match;
4. `DEFAULT_LOCALE`.

- **Server (TanStack Start SSR entry)**: resolve from cookie → `Accept-Language` →
  default; seed the i18next instance `lng` and render `<html lang>`/`dir` accordingly.
- **Client**: the detector covers steps 2–4; after `GET /me` resolves, if the DB
  preference differs from the active language, call `i18n.changeLanguage()` and update
  the `nv_locale` cookie.

## `useLocale` hook (`frontend/src/hooks/use-locale.ts`)

Exposes `{ locale, setLocale, supportedLocales }`.

- `setLocale(next)` — calls `i18n.changeLanguage(next)`, updates the `nv_locale` cookie,
  updates `<html lang>`/`dir`, and (when signed in) calls `api.updateMyLocale(next)` so
  the choice persists to the account. On API failure it reverts the active language and
  surfaces an error toast (spec Story 1 scenario 4 — previous language remains).
- The change applies with **no route navigation and no full reload** (FR-005/FR-006).

## `<html>` attributes (`__root.tsx`)

`RootDocument` sets `lang={locale}` and `dir={localeDir[locale]}` from the
SSR-resolved locale (replacing the current hardcoded `lang="en"`), keeping server and
client markup identical to avoid a hydration mismatch (FR-011).

## Testing obligations (Constitution II)

- **Catalog parity**: a Vitest test (or `i18next-cli` lint in CI) asserting every `ru/`
  namespace key set equals the matching `en/` set.
- **No raw keys**: a test asserting the i18next config never returns a `ns:key` string
  for a missing key (returns the English fallback instead).
- **Switch + persist**: changing locale via `useLocale` updates the UI, the cookie, and
  issues `PUT /api/platform/me`; an API failure reverts.
- **Precedence**: detection picks DB > cookie > browser > default across the relevant
  signed-in / signed-out / first-visit combinations.
