// The runtime flags from GET /api/platform/config. Until the answer arrives,
// or if it fails, the SaaS defaults apply, so SaaS never flickers.

import { useQuery } from "@tanstack/react-query"
import { api } from "@/lib/api"
import { queryKeys } from "@/lib/query"

export function usePlatformConfig() {
  const query = useQuery({
    queryKey: queryKeys.platformConfig(),
    queryFn: async () => (await api.config()).data,
    staleTime: Infinity,
  })
  return {
    billing: query.data?.features.billing ?? true,
    multiTenant: query.data?.features.multi_tenant ?? true,
    setupRequired: query.data?.setup_required ?? false,
  }
}
