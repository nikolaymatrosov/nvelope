// TanStack Query setup (research.md Decision 1). One QueryClient, a key factory
// keyed by slug + resource, and a global error handler that delegates auth
// routing to the single error-mapping point (src/lib/errors.ts).

import { QueryCache, QueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { ApiError, errorMessage, isServerError, routeAuthError } from "./errors"

// Surface a non-auth failure once, globally. 401 is routed by routeAuthError;
// 403/404 are rendered in-place by the screen that issued the query.
function reportError(error: unknown) {
  if (routeAuthError(error)) return
  if (error instanceof ApiError && (error.status === 403 || error.status === 404)) {
    return
  }
  if (isServerError(error) || !(error instanceof ApiError)) {
    toast.error(errorMessage(error))
  }
}

export const queryClient = new QueryClient({
  queryCache: new QueryCache({
    onError: reportError,
  }),
  defaultOptions: {
    queries: {
      retry: (failureCount, error) => {
        if (error instanceof ApiError && error.status < 500) return false
        return failureCount < 2
      },
      staleTime: 30_000,
      refetchOnWindowFocus: false,
    },
    mutations: {
      onError: reportError,
    },
  },
})

// ── Key factory ──────────────────────────────────────────────────────────────
// All tenant-scoped keys lead with the slug so a workspace switch invalidates
// cleanly. Resource keys are arrays so partial prefixes invalidate sub-trees.

export const queryKeys = {
  me: () => ["me"] as const,
  tenants: () => ["tenants"] as const,
  invitationLookup: (token: string) => ["invitation", token] as const,

  workspace: (slug: string) => ["t", slug] as const,
  tenant: (slug: string) => ["t", slug, "tenant"] as const,
  settings: (slug: string) => ["t", slug, "settings"] as const,
  invitations: (slug: string) => ["t", slug, "invitations"] as const,

  lists: (slug: string) => ["t", slug, "lists"] as const,
  listsPage: (slug: string, limit: number, offset: number) =>
    ["t", slug, "lists", { limit, offset }] as const,
  list: (slug: string, id: string) => ["t", slug, "lists", id] as const,

  subscribers: (slug: string) => ["t", slug, "subscribers"] as const,
  subscribersSearch: (
    slug: string,
    q: string,
    limit: number,
    offset: number,
  ) => ["t", slug, "subscribers", "search", { q, limit, offset }] as const,
  subscribersQuery: (slug: string, segment: unknown, page: unknown) =>
    ["t", slug, "subscribers", "query", { segment, page }] as const,
  subscriber: (slug: string, id: string) =>
    ["t", slug, "subscribers", id] as const,

  roles: (slug: string) => ["t", slug, "roles"] as const,
  apiKeys: (slug: string) => ["t", slug, "api-keys"] as const,
  audit: (slug: string, limit: number, offset: number) =>
    ["t", slug, "audit", { limit, offset }] as const,
  job: (slug: string, id: string) => ["t", slug, "jobs", id] as const,
}
