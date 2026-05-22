# Phase 1 Data Model: App Internationalization

**Feature**: 015-app-i18n-language-switcher | **Date**: 2026-05-22

## Locale (value object — `internal/auth/domain/locale.go`)

A `Locale` is a supported interface language. It exists so an unsupported locale is
unrepresentable inside the domain (Constitution VI — always-valid construction).

- **Supported set**: `en` (English), `ru` (Russian).
- **Default**: `en`.
- **Text direction**: both `ltr`. A `dir` lookup exists so adding an RTL locale later
  is a data change, not a structural one (FR-011).
- **Constructor**: `NewLocale(code string) (Locale, error)` — trims, lowercases, and
  rejects anything outside the supported set with an `apperr` incorrect-input error
  (kind `unsupported_locale`).
- **Zero value**: `Locale{}` is the "unset" state — maps to SQL NULL on the write path
  and to JSON `null` on the wire. A separate `Locale.IsZero()` reports it.
- **Hydration**: `HydrateLocale(code string) Locale` — persistence-only, no validation;
  an unrecognised stored value hydrates to the zero (unset) Locale so FR-014 degrades
  to the default without error.

| Field/concept | Type | Notes |
|---|---|---|
| code | string | one of `en`, `ru`; empty = unset |
| direction | `ltr`/`rtl` | derived from code; `ltr` for both launch locales |

## User (entity change — `internal/auth/domain/user.go`)

The existing platform `User` gains an optional locale.

- New unexported field `locale Locale`.
- `NewUser` is unchanged — a brand-new user has an unset locale (NULL in DB).
- New behaviour `SetLocale(l Locale)` — assigns the (already-valid) locale; this is the
  only mutation path. There is no setter that accepts a raw string.
- `HydrateUser` gains a `locale` parameter (persistence-only path).
- Accessor `Locale() Locale`.

State: a user's locale moves `unset → en` or `unset → ru` and may change freely between
supported locales thereafter. It never returns to `unset`.

## platform_users.locale (schema — migration `000021_user_locale`)

```sql
-- up
ALTER TABLE platform_users
    ADD COLUMN locale text
    CHECK (locale IS NULL OR locale IN ('en', 'ru'));

-- down
ALTER TABLE platform_users DROP COLUMN locale;
```

- **Nullable, no default** — NULL is the "never explicitly chosen" state required by
  FR-008 (see research.md D2).
- `platform_users` is a control-plane table — **no Row-Level Security**, consistent
  with migration `000003_control_plane`.
- The `CHECK` constraint is defence-in-depth; the `Locale` value object is the primary
  guard. Adding a locale later means widening this `CHECK` in a new migration.

### Adapter read/write (`internal/auth/adapters/users_pg.go`)

- `GetByID`, `LookupByEmail`, `GetCredentials` SELECT lists gain `locale`; the scanned
  value is a nullable string passed to `HydrateUser`.
- New `UpdateLocale(ctx, userID string, locale Locale) error` — `UPDATE platform_users
  SET locale = $1, updated_at = now() WHERE id = $2` (positional params — 2
  placeholders, per the project's `< 4` convention). Returns `ErrUserNotFound` when no
  row matches.

## AuthenticatedUser (read model — `internal/auth/app/query`)

The flat read model returned by the `AuthenticateSession` query gains `Locale string`
(empty string = unset). It flows into the request context via `userFromContext`, so
handlers — including `handleMe` — can emit the stored locale without an extra query.

## Visitor Language Preference — `nv_locale` cookie

Not a database entity; the signed-out / SSR locale carrier.

| Property | Value |
|---|---|
| Name | `nv_locale` |
| Value | a supported locale code (`en` / `ru`) |
| Path | `/` |
| SameSite | `Lax` |
| HttpOnly | `false` — the client detector and `useLocale` read it |
| Max-Age | ~1 year (persistent across sessions, per FR-008) |
| Written by | Go API on `login`/`signup`/`accept-invitation`/`PUT /me`; client `i18next-browser-languageDetector` cache |
| Read by | TanStack Start SSR entry (no-flash seed); client language detector |

Relationship: the cookie is a **mirror** of the effective locale. For a signed-in user
the DB column is authoritative and the cookie is kept in sync; for a signed-out visitor
the cookie *is* the stored preference until they sign in (then FR-008 adoption applies).

## Frontend view-model changes (`frontend/src/lib/api-types.ts`)

- `PlatformUser` gains `locale: string | null`.
- New `AccountLocaleInput = { locale: string }` — the `PUT /api/platform/me` body.
