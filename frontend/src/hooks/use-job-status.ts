// Polls a server-side job (import or export) while its status is non-terminal
// (FR-023, FR-025, research.md Decision 10). Re-opening a job view simply
// re-runs this query, so progress is restart-safe.

import { useQuery } from "@tanstack/react-query"
import { api } from "@/lib/api"
import { queryKeys } from "@/lib/query"
import { isTerminalJobStatus } from "@/lib/api-types"

const POLL_INTERVAL_MS = 2000

export function useJobStatus(slug: string, jobId: string | null) {
  const query = useQuery({
    queryKey: queryKeys.job(slug, jobId ?? ""),
    queryFn: async () => (await api.jobStatus(slug, jobId as string)).data,
    enabled: jobId !== null,
    staleTime: 0,
    refetchInterval: (q) => {
      const data = q.state.data
      if (!data) return POLL_INTERVAL_MS
      return isTerminalJobStatus(data.Status) ? false : POLL_INTERVAL_MS
    },
  })

  const job = query.data
  return {
    job,
    isPolling:
      jobId !== null &&
      (job === undefined || !isTerminalJobStatus(job.Status)),
    isTerminal: job !== undefined && isTerminalJobStatus(job.Status),
    isLoading: query.isLoading,
    isError: query.isError,
    error: query.error,
  }
}
