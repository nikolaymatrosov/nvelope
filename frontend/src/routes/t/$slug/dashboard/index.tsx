import { Link, createFileRoute } from "@tanstack/react-router"
import { useQuery } from "@tanstack/react-query"
import { ChevronRightIcon } from "lucide-react"
import { useTranslation } from "react-i18next"
import type { DashboardView } from "@/lib/api-types"
import { api } from "@/lib/api"
import { queryKeys } from "@/lib/query"
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import { AsyncState } from "@/components/common/async-state"
import { MetricTile } from "@/components/common/metric-tile"
import { RateValue } from "@/components/common/rate-value"

export const Route = createFileRoute("/t/$slug/dashboard/")({
  component: DashboardPage,
})

export function DashboardPage() {
  const { slug } = Route.useParams()
  const { t } = useTranslation("dashboard")

  const query = useQuery({
    queryKey: queryKeys.dashboard(slug),
    queryFn: async () => (await api.analytics.dashboard(slug)).data,
    retry: false,
  })

  return (
    <div className="flex flex-col gap-6">
      <header>
        <h1 className="text-2xl font-semibold">{t("index.title")}</h1>
        <p className="text-sm text-muted-foreground">
          {t("index.description")}
        </p>
      </header>

      <AsyncState
        query={query}
        isEmpty={(d) =>
          d.totals.sent === 0 && d.recentCampaigns.length === 0
        }
        emptyTitle={t("index.emptyTitle")}
        emptyMessage={t("index.emptyMessage")}
      >
        {(data) => <DashboardBody slug={slug} data={data} />}
      </AsyncState>
    </div>
  )
}

function DashboardBody({
  slug,
  data,
}: {
  slug: string
  data: DashboardView
}) {
  const { t } = useTranslation("dashboard")
  const { totals, deliverability, recentCampaigns } = data
  return (
    <div className="flex flex-col gap-6">
      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
        <MetricTile label={t("metrics.sent")} value={totals.sent} />
        <MetricTile label={t("metrics.delivered")} value={totals.delivered} />
        <MetricTile label={t("metrics.opened")} value={totals.opened} />
        <MetricTile label={t("metrics.clicked")} value={totals.clicked} />
        <MetricTile
          label={t("metrics.bounced")}
          value={totals.bounced}
          rate={<RateValue value={deliverability.bounceRate} />}
        />
        <MetricTile
          label={t("metrics.complained")}
          value={totals.complained}
          rate={<RateValue value={deliverability.complaintRate} />}
        />
      </div>

      <Card>
        <CardHeader>
          <CardTitle>{t("recentCampaigns.title")}</CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-2">
          {recentCampaigns.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              {t("recentCampaigns.empty")}
            </p>
          ) : (
            recentCampaigns.map((c) => (
              <Link
                key={c.campaignId}
                to="/t/$slug/campaigns/$id/analytics"
                params={{ slug, id: c.campaignId }}
                className="flex items-center gap-3 rounded-lg border p-3 hover:bg-muted/50"
              >
                <div className="flex-1">
                  <p className="text-sm font-medium">{c.name}</p>
                  <p className="text-xs text-muted-foreground">
                    {t("recentCampaigns.sentCount", {
                      count: c.sent,
                    })}
                  </p>
                </div>
                <div className="text-right text-xs text-muted-foreground tabular-nums">
                  <p>
                    {t("recentCampaigns.openRate")}{" "}
                    <RateValue value={c.openRate} />
                  </p>
                  <p>
                    {t("recentCampaigns.bounceRate")}{" "}
                    <RateValue value={c.bounceRate} /> ·{" "}
                    {t("recentCampaigns.complaintRate")}{" "}
                    <RateValue value={c.complaintRate} />
                  </p>
                </div>
                <ChevronRightIcon className="size-4 text-muted-foreground" />
              </Link>
            ))
          )}
        </CardContent>
      </Card>
    </div>
  )
}
