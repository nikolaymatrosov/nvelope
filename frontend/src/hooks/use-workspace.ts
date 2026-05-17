// The current workspace (tenant) the user is viewing. Backed by
// GET /t/{slug}/api/tenant.

import { useQuery } from "@tanstack/react-query"
import { api } from "@/lib/api"
import { queryKeys } from "@/lib/query"

export function useWorkspace(slug: string) {
  const query = useQuery({
    queryKey: queryKeys.tenant(slug),
    queryFn: async () => (await api.tenant(slug)).data,
  })

  return {
    workspace: query.data,
    name: query.data?.tenant.name,
    members: query.data?.members ?? [],
    isLoading: query.isLoading,
    isError: query.isError,
    error: query.error,
  }
}
