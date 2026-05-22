# Quickstart: App Internationalization

**Feature**: 015-app-i18n-language-switcher | **Date**: 2026-05-22

A working orientation for implementing and verifying the i18n feature. See
[plan.md](plan.md) for the full plan and [research.md](research.md) for the decisions.

## 1. Install dependencies

```bash
cd frontend
pnpm add i18next react-i18next i18next-browser-languageDetector
pnpm add -D i18next-cli
```

## 2. Backend: schema + endpoint

- Add migration `internal/db/migrations/000021_user_locale.{up,down}.sql` (see
  [data-model.md](data-model.md)).
- Add the `Locale` value object, extend `User`, add `UserRepository.UpdateLocale`,
  add the `SetLocale` command, extend the `AuthenticateSession` query read model.
- Add `handleUpdateMe` + register `PUT /api/platform/me` in `server.go`; extend
  `handleMe` to emit `locale`; set the `nv_locale` cookie on auth endpoints.
- Apply migrations and run backend tests:

```bash
make test    # testcontainers spins up postgres:17 automatically
```

## 3. Frontend: i18n module

- Create `frontend/src/i18n/{config,index,detect}.ts` and the i18next instance per
  [contracts/frontend-i18n.md](contracts/frontend-i18n.md) — `fallbackLng: "en"`,
  language detector on the `nv_locale` cookie.
- Create `frontend/src/locales/{en,ru}/common.json` (and other namespaces as areas are
  migrated).
- Wrap the app in `I18nextProvider` and make `<html lang>`/`dir` dynamic in
  `src/routes/__root.tsx`.
- Add the `useLocale` hook, the `LanguageSelect` component, and the `/account` route.

## 4. Adding a translatable string

1. Pick the namespace for the feature area (e.g. `account`).
2. Add the key to **`en/account.json`** (source of truth) and `ru/account.json`:

   ```json
   // en/account.json            // ru/account.json
   { "language.label": "Language" }   { "language.label": "Язык" }
   ```

3. Use it in the component:

   ```tsx
   import { useTranslation } from "react-i18next"
   const { t } = useTranslation("account")
   return <label>{t("language.label")}</label>
   ```

4. Regenerate typed keys: `pnpm exec i18next-cli` (or the configured script).

## 5. Verify

```bash
cd frontend
pnpm typecheck && pnpm lint && pnpm test
pnpm dev          # then exercise the UI in a browser
```

Manual checks (map to spec acceptance scenarios):

- **Story 1** — sign in, open `/account`, switch to Russian, Save: the UI re-renders in
  Russian, the URL is unchanged, no full reload. Reload and sign in from a second
  browser → still Russian. Force the `PUT` to fail → language reverts, error shown.
- **Story 2** — clear cookies, set the browser language to Russian, open `/login` →
  page is Russian. Set the browser to an unsupported language → page is English.
- **Story 3** — temporarily remove a key from `ru/*.json` → that text shows in English,
  the rest of the page is unaffected, no raw `ns:key` appears anywhere.

## Key files

| Path | Role |
|---|---|
| `internal/db/migrations/000021_user_locale.*` | `platform_users.locale` column |
| `internal/auth/domain/locale.go` | `Locale` value object (supported set) |
| `internal/auth/app/command/set_locale.go` | `SetLocale` command |
| `internal/api/platform_handlers.go` | `handleMe` (+locale), `handleUpdateMe` |
| `frontend/src/i18n/` | i18next setup, config, detection |
| `frontend/src/locales/{en,ru}/` | translation catalogs |
| `frontend/src/hooks/use-locale.ts` | active locale + change + persist |
| `frontend/src/routes/account/index.tsx` | Account page hosting the switcher |
