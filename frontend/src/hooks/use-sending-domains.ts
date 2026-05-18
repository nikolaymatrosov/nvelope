// Lists the workspace's sending domains and polls while any domain is still
// pending verification (FR-007), so backend-driven status changes appear
// without a manual reload. Mirrors the useJobStatus polling pattern.

import { useQuery } from "@tanstack/react-query"
import type { DomainView } from "@/lib/api-types"
import { api } from "@/lib/api"
import { queryKeys } from "@/lib/query"

const POLL_INTERVAL_MS = 5000

function hasPending(domains: Array<DomainView>): boolean {
  return domains.some((d) => d.status === "pending")
}

export function useSendingDomains(slug: string) {
  const query = useQuery({
    queryKey: queryKeys.sendingDomains(slug),
    queryFn: async () => (await api.listSendingDomains(slug)).data.domains,
    staleTime: 0,
    refetchInterval: (q) => {
      const data = q.state.data
      if (!data) return POLL_INTERVAL_MS
      return hasPending(data) ? POLL_INTERVAL_MS : false
    },
  })

  return {
    domains: query.data,
    isLoading: query.isLoading,
    isError: query.isError,
    error: query.error,
    refetch: query.refetch,
    query,
  }
}
