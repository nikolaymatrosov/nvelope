// Loads a single campaign and polls while it is running (FR-018), so live
// send progress advances without a manual reload. Re-opening the view re-runs
// the query, so progress is restart-safe.

import { useQuery } from "@tanstack/react-query"
import type { CampaignView } from "@/lib/api-types"
import { api } from "@/lib/api"
import { queryKeys } from "@/lib/query"

const POLL_INTERVAL_MS = 3000

export type CampaignProgress = {
  sent: number
  failed: number
  remaining: number
  total: number
}

export function campaignProgress(c: CampaignView): CampaignProgress {
  const remaining = Math.max(
    0,
    c.recipient_count - c.sent_count - c.failed_count,
  )
  return {
    sent: c.sent_count,
    failed: c.failed_count,
    remaining,
    total: c.recipient_count,
  }
}

// autoPaused is true when the campaign was paused by the backend after its
// send errors reached the configured threshold (research Decision 5).
export function isAutoPaused(c: CampaignView): boolean {
  return (
    c.status === "paused" &&
    c.max_send_errors > 0 &&
    c.failed_count >= c.max_send_errors
  )
}

export function useCampaign(slug: string, id: string) {
  const query = useQuery({
    queryKey: queryKeys.campaign(slug, id),
    queryFn: async () => (await api.getCampaign(slug, id)).data,
    staleTime: 0,
    refetchInterval: (q) =>
      q.state.data?.status === "running" ? POLL_INTERVAL_MS : false,
  })

  return {
    campaign: query.data,
    isLoading: query.isLoading,
    isError: query.isError,
    error: query.error,
    refetch: query.refetch,
    query,
  }
}
