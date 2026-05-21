// Single error-mapping point (research.md Decision 4, Constitution Principle
// VI). The typed client raises `ApiError`; screens and the global query
// handler branch on the normalized `status`/`slug`, never on message text.

export type ErrorEnvelope = { error?: string; message?: string }

export class ApiError extends Error {
  readonly status: number
  readonly slug: string
  readonly path: string
  // Full parsed response body, when one came back as JSON. Screens read it to
  // recover error-specific payload fields like the 409 stale_row's
  // `currentUpdatedAt`. The status / slug fields above remain the
  // canonical branching surface.
  readonly data: Record<string, unknown> | null

  constructor(
    status: number,
    slug: string,
    message: string,
    path: string,
    data: Record<string, unknown> | null = null,
  ) {
    super(message)
    this.name = "ApiError"
    this.status = status
    this.slug = slug
    this.path = path
    this.data = data
  }
}

// Normalize a non-2xx response body into an ApiError. `body` is whatever the
// transport parsed from the response text (object, null, or a raw string).
export function normalizeError(
  status: number,
  body: unknown,
  path: string,
): ApiError {
  let slug = ""
  let message = ""
  let data: Record<string, unknown> | null = null
  if (body && typeof body === "object") {
    const env = body as ErrorEnvelope
    slug = env.error ?? ""
    message = env.message ?? ""
    data = body as Record<string, unknown>
  } else if (typeof body === "string") {
    message = body
  }
  if (!message) message = defaultMessageFor(status)
  return new ApiError(status, slug, message, path, data)
}

function defaultMessageFor(status: number): string {
  switch (status) {
    case 401:
      return "You need to sign in to continue."
    case 403:
      return "You do not have permission to do that."
    case 404:
      return "The requested resource was not found."
    case 409:
      return "That conflicts with something that already exists."
    case 422:
      return "Some of the information provided is not valid."
    case 500:
      return "Something went wrong on our end. Please try again."
    default:
      return "The request could not be completed."
  }
}

// ── Status-category helpers (internal/api/errmap.go) ─────────────────────────

export const isUnauthorized = (e: unknown): e is ApiError =>
  e instanceof ApiError && e.status === 401
export const isForbidden = (e: unknown): e is ApiError =>
  e instanceof ApiError && e.status === 403
export const isNotFound = (e: unknown): e is ApiError =>
  e instanceof ApiError && e.status === 404
export const isConflict = (e: unknown): e is ApiError =>
  e instanceof ApiError && e.status === 409
export const isValidation = (e: unknown): e is ApiError =>
  e instanceof ApiError && e.status === 422
export const isServerError = (e: unknown): e is ApiError =>
  e instanceof ApiError && e.status >= 500

// A human-readable message for any thrown value.
export function errorMessage(e: unknown): string {
  if (e instanceof ApiError) return e.message
  if (e instanceof Error) return e.message
  return "An unexpected error occurred."
}

// True for tenant-plane paths (`/t/{slug}/api/*`).
function isTenantPath(path: string): boolean {
  return /^\/t\/[^/]+\/api\//.test(path)
}

// The session/auth error router (research.md Decision 4). Returns true when the
// error was fully handled by routing (caller should suppress further UI).
//
// - 401 on the platform plane → redirect to /login.
// - 401 on the tenant plane → the workspace session lapsed; bounce through the
//   workspace route so its loader re-opens the session / TOTP challenge.
// - 403 / 404 are left for the screen to render (authorization message /
//   not-found screen) — they are not routed away from.
export function routeAuthError(e: unknown): boolean {
  if (!(e instanceof ApiError)) return false
  if (e.status !== 401) return false
  if (typeof window === "undefined") return true

  if (isTenantPath(e.path)) {
    const m = e.path.match(/^\/t\/([^/]+)\/api\//)
    const slug = m?.[1]
    if (slug) {
      // Re-enter the workspace route; its loader re-opens the session.
      window.location.assign(`/t/${slug}`)
      return true
    }
  }
  window.location.assign("/login")
  return true
}
