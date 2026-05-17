// The current platform account (research.md Decision 3). Backed by GET
// /api/platform/me; an unauthenticated 401 is routed by the global handler.

import { useQuery } from "@tanstack/react-query"
import type { PlatformAccount } from "@/lib/api-types"
import { api } from "@/lib/api"
import { queryKeys } from "@/lib/query"

export function useSession() {
  const query = useQuery({
    queryKey: queryKeys.me(),
    queryFn: async () => (await api.me()).data,
    retry: false,
  })

  const account: PlatformAccount | undefined = query.data
  return {
    account,
    user: account?.user,
    tenants: account?.tenants ?? [],
    isLoading: query.isLoading,
    isError: query.isError,
    error: query.error,
  }
}
