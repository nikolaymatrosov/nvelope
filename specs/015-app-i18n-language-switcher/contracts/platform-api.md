# Contract: Platform API — User Locale

**Feature**: 015-app-i18n-language-switcher | **Date**: 2026-05-22

Two changes to the platform-plane API. Both routes are under `/api/platform` and behind
the `requireUser` session middleware. Responses are snake_case (platform-plane
convention).

## GET /api/platform/me — extended

The existing `handleMe` response gains `user.locale`.

**Response 200**:

```json
{
  "user": {
    "id": "0f9b…",
    "name": "Ada Lovelace",
    "email": "ada@example.com",
    "locale": "ru"
  },
  "tenants": [ /* unchanged */ ]
}
```

- `user.locale` is `"en"` | `"ru"` | `null`.
- `null` means the user has never explicitly chosen a language — the client then falls
  through the D4 precedence (cookie → browser → default).
- No request-shape change; existing callers that ignore the new field are unaffected.

## PUT /api/platform/me — new

Updates the **authenticated** user's interface-language preference. The target user is
always the caller resolved from the session — there is no user id in the path or body,
so one user can never write another's locale (Constitution IV).

**Request body**:

```json
{ "locale": "ru" }
```

| Field | Type | Rules |
|---|---|---|
| `locale` | string | Required. Must be a supported locale code: `en` or `ru`. |

**Response 200** — the updated user, same shape as `GET /me`'s `user` object:

```json
{
  "user": {
    "id": "0f9b…",
    "name": "Ada Lovelace",
    "email": "ada@example.com",
    "locale": "ru"
  }
}
```

**Side effect**: the response sets the `nv_locale` cookie (see data-model.md) to the
new locale, so the next server-side render is in the chosen language.

**Errors** (the standard error envelope produced by `Server.fail`):

| Status | kind | When |
|---|---|---|
| 400 | `invalid_body` | body is not valid JSON |
| 422 | `unsupported_locale` | `locale` is missing, empty, or not in the supported set |
| 401 | `unauthorized` | no valid session (from `requireUser`) |

The `unsupported_locale` kind originates from the `Locale` value-object constructor and
is mapped to HTTP once, in the existing transport error-mapping site — domain/app code
stays unaware of status codes (Constitution VI).

## Cookie behaviour on existing auth endpoints

`POST /api/platform/login`, `POST /api/platform/signup`, and
`POST /api/platform/invitations/{token}/accept` additionally:

- Set the `nv_locale` cookie to the user's effective locale on success.
- **FR-008 adoption**: if the authenticated user's stored `locale` is `NULL` and the
  request carried an `nv_locale` cookie with a supported value, persist that value as
  the account preference before responding. An existing non-NULL preference is left
  untouched.

These endpoints' JSON response shapes are otherwise unchanged.

## Backend use-case surface

- **Command** `SetLocale { UserID string; Locale string }` in
  `internal/auth/app/command/set_locale.go`, wrapped by
  `decorator.ApplyResultCommandDecorators` (or `ApplyCommandDecorators` if it returns no
  result). Validates `Locale` via the value-object constructor, loads the user, calls
  `User.SetLocale`, and persists via `UserRepository.UpdateLocale`.
- Wired in the composition root `internal/service/application.go` alongside `SignUp` /
  `LogIn` / `LogOut`.

## Frontend client (`frontend/src/lib/api.ts`)

```ts
// me() return type gains user.locale (api-types.ts).
updateMyLocale: (locale: string) =>
  request<{ user: PlatformUser }>("PUT", "/api/platform/me", { locale }),
```
