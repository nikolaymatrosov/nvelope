// The tenant-plane transport. The generated schema's path keys are relative
// (/lists, not /t/{slug}/api/lists) because tenant.yaml declares a templated
// servers: base — so the slug lives in the client's baseUrl. That keeps the
// tenant-isolation property of the old client: a call site cannot construct a
// tenant request without naming a workspace.

import createClient from "openapi-fetch"
import { normalizeError } from "./errors"
import type { Client } from "openapi-fetch"
import type { paths } from "./api-schema.gen"

// A client is a stateless bag of closures over `baseUrl` — building one per
// call costs far less than the request it is about to make, and keeps the
// module free of cross-call state (a memoized client would capture whatever
// `globalThis.fetch` happened to be bound at first use).
export function tenantApi(slug: string): Client<paths> {
  return createClient<paths>({
    baseUrl: `/t/${slug}/api`,
    credentials: "include",
  })
}

// Structurally identical to api.ts's `ApiResult<T>`; declared here rather than
// imported so the transport does not depend on the facade it serves.
export type UnwrappedResult<T> = { status: number; ok: boolean; data: T }

// `ApiError.path` is what routeAuthError (errors.ts) matches `/t/{slug}/api/`
// against to re-open a lapsed workspace session on a 401, so it has to be the
// real request path. Any response that came off the network carries it in
// `response.url`; one that did not (a hand-built stub) leaves it "", and
// `new URL("")` throws — which would bury the actual API failure behind a
// TypeError. Fall back to an empty path instead.
function pathFromResponse(response: Response): string {
  if (!response.url) return ""
  try {
    return new URL(response.url).pathname
  } catch {
    return ""
  }
}

// openapi-fetch resolves rather than throws on a non-2xx response. unwrap
// restores the throwing contract every screen and the global query error
// handler already depend on.
export async function unwrap<T>(
  call: Promise<{ data?: T; error?: unknown; response: Response }>,
): Promise<UnwrappedResult<T>> {
  const { data, error, response } = await call
  if (error !== undefined || !response.ok) {
    throw normalizeError(response.status, error, pathFromResponse(response))
  }
  return { status: response.status, ok: true, data: data as T }
}
