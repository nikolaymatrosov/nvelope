import { Link, createFileRoute } from "@tanstack/react-router"
import { useQuery } from "@tanstack/react-query"
import { AlertCircleIcon, ArrowLeftIcon } from "lucide-react"
import type { CampaignAnalytics } from "@/lib/api-types"
import { api } from "@/lib/api"
import { queryKeys } from "@/lib/query"
import { isNotFound } from "@/lib/errors"
import { formatDateTime } from "@/lib/format"
import { Button } from "@/components/ui/button"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty"
import { AsyncState } from "@/components/common/async-state"
import { MetricTile } from "@/components/common/metric-tile"
import { RateValue } from "@/components/common/rate-value"

export const Route = createFileRoute("/t/$slug/campaigns/$id/analytics")({
  component: CampaignAnalyticsView,
})

export function CampaignAnalyticsView() {
  const { slug, id } = Route.useParams()

  const query = useQuery({
    queryKey: queryKeys.campaignAnalytics(slug, id),
    queryFn: async () => (await api.analytics.campaign(slug, id)).data,
    retry: false,
  })

  return (
    <div className="flex flex-col gap-6">
      <header className="flex flex-col gap-2">
        <Button variant="ghost" size="sm" className="w-fit -ml-2" asChild>
          <Link to="/t/$slug/campaigns/$id" params={{ slug, id }}>
            <ArrowLeftIcon /> Back to campaign
          </Link>
        </Button>
        <div>
          <h1 className="text-2xl font-semibold">Campaign analytics</h1>
          <p className="text-sm text-muted-foreground">
            How this campaign performed across delivery, opens, and clicks.
          </p>
        </div>
      </header>

      {isNotFound(query.error) ? (
        <Empty data-testid="analytics-not-found" className="border">
          <EmptyHeader>
            <EmptyMedia variant="icon">
              <AlertCircleIcon className="text-destructive" />
            </EmptyMedia>
            <EmptyTitle>Campaign not found</EmptyTitle>
            <EmptyDescription>
              This campaign does not exist in this workspace.
            </EmptyDescription>
          </EmptyHeader>
        </Empty>
      ) : (
        <AsyncState query={query}>
          {(data) => <AnalyticsBody data={data} />}
        </AsyncState>
      )}
    </div>
  )
}

function AnalyticsBody({ data }: { data: CampaignAnalytics }) {
  const { counts, rates, refreshedAt } = data
  return (
    <div className="flex flex-col gap-4">
      {refreshedAt === null ? (
        <Alert data-testid="analytics-awaiting">
          <AlertTitle>Awaiting data</AlertTitle>
          <AlertDescription>
            Delivery feedback has not been processed for this campaign yet.
            Counts and rates will appear here once the first refresh runs.
          </AlertDescription>
        </Alert>
      ) : (
        <p className="text-xs text-muted-foreground">
          Figures last refreshed {formatDateTime(refreshedAt)}.
        </p>
      )}

      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
        <MetricTile label="Sent" value={counts.sent} />
        <MetricTile label="Delivered" value={counts.delivered} />
        <MetricTile
          label="Opened"
          value={counts.opened}
          rate={<RateValue value={rates.openRate} />}
        />
        <MetricTile
          label="Clicked"
          value={counts.clicked}
          rate={<RateValue value={rates.clickRate} />}
        />
        <MetricTile
          label="Bounced"
          value={counts.bounced}
          rate={<RateValue value={rates.bounceRate} />}
        />
        <MetricTile
          label="Complained"
          value={counts.complained}
          rate={<RateValue value={rates.complaintRate} />}
        />
      </div>
    </div>
  )
}
