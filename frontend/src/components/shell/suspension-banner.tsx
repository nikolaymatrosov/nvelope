// Workspace-wide banner shown on every page while the tenant is suspended for
// non-payment (Phase 5 US5). Reads the shared subscription query; renders
// nothing unless the subscription state is `suspended`.

import { Link } from "@tanstack/react-router"
import { useQuery } from "@tanstack/react-query"
import { AlertTriangleIcon } from "lucide-react"
import { api } from "@/lib/api"
import { queryKeys } from "@/lib/query"

export function SuspensionBanner({ slug }: { slug: string }) {
  const query = useQuery({
    queryKey: queryKeys.subscription(slug),
    queryFn: async () => (await api.billing.getSubscription(slug)).data,
    retry: false,
  })

  if (query.data?.subscription.state !== "suspended") return null

  return (
    <div
      data-testid="suspension-banner"
      className="flex items-center gap-3 rounded-lg border border-destructive/40 bg-destructive/10 px-4 py-3 text-sm text-destructive"
    >
      <AlertTriangleIcon className="size-4 shrink-0" />
      <span className="flex-1">
        This workspace is suspended for non-payment. Sending is disabled until
        the outstanding balance is settled.
      </span>
      <Link
        to="/t/$slug/billing"
        params={{ slug }}
        className="font-medium underline underline-offset-2"
      >
        Settle balance
      </Link>
    </div>
  )
}
