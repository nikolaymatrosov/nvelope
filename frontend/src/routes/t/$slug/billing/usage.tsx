// Current-period usage against the plan allowance (Phase 5 US3).

import { Link, createFileRoute } from "@tanstack/react-router"
import { useQuery } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"
import { InfoIcon } from "lucide-react"
import type { SubscriptionResponse } from "@/lib/api-types"
import { api } from "@/lib/api"
import { queryKeys } from "@/lib/query"
import { errorMessage, isNotFound } from "@/lib/errors"
import { formatDate } from "@/lib/format"
import { UsageMeter } from "@/components/common/usage-meter"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import {
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyTitle,
} from "@/components/ui/empty"
import { BillingNav } from "@/components/billing/billing-nav"

export const Route = createFileRoute("/t/$slug/billing/usage")({
  component: UsagePage,
})

export function UsagePage() {
  const { slug } = Route.useParams()
  const { t } = useTranslation("billing")

  const query = useQuery({
    queryKey: queryKeys.subscription(slug),
    queryFn: async () => (await api.billing.getSubscription(slug)).data,
    retry: false,
  })

  const noSubscription =
    query.isError &&
    isNotFound(query.error) &&
    query.error.slug === "no_subscription"

  return (
    <div className="flex flex-col gap-6">
      <header>
        <h1 className="text-2xl font-semibold">{t("usage.title")}</h1>
        <p className="text-sm text-muted-foreground">
          {t("usage.description")}
        </p>
      </header>

      <BillingNav slug={slug} />

      {query.isLoading && (
        <div className="flex flex-col gap-3" data-testid="usage-loading">
          <Skeleton className="h-32 w-full" />
        </div>
      )}

      {noSubscription && (
        <Empty data-testid="usage-no-subscription" className="border">
          <EmptyHeader>
            <EmptyTitle>{t("usage.noSubscription.title")}</EmptyTitle>
            <EmptyDescription>
              {t("usage.noSubscription.description")}
            </EmptyDescription>
          </EmptyHeader>
          <EmptyContent>
            <Button asChild>
              <Link to="/t/$slug/billing/plans" params={{ slug }}>
                {t("usage.noSubscription.browsePlans")}
              </Link>
            </Button>
          </EmptyContent>
        </Empty>
      )}

      {query.isError && !noSubscription && (
        <Empty data-testid="usage-error" className="border">
          <EmptyHeader>
            <EmptyTitle>{t("usage.loadError.title")}</EmptyTitle>
            <EmptyDescription>{errorMessage(query.error)}</EmptyDescription>
          </EmptyHeader>
        </Empty>
      )}

      {query.data && <UsageBody slug={slug} data={query.data} />}
    </div>
  )
}

function UsageBody({
  slug,
  data,
}: {
  slug: string
  data: SubscriptionResponse
}) {
  const { subscription, usage } = data
  const { t } = useTranslation("billing")
  const blockMode = subscription.plan.overageMode === "block"
  const exhausted =
    usage.includedSends > 0 && usage.remainingSends <= 0

  return (
    <div className="flex flex-col gap-4">
      <Card>
        <CardHeader>
          <CardTitle>{t("usage.thisPeriod.title")}</CardTitle>
          <CardDescription>
            {formatDate(subscription.currentPeriodStart)} –{" "}
            {formatDate(subscription.currentPeriodEnd)}
          </CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          <UsageMeter
            used={usage.usedSends}
            included={usage.includedSends}
          />

          {exhausted && blockMode && (
            <Alert variant="destructive" data-testid="usage-blocked">
              <AlertTitle>{t("usage.blocked.title")}</AlertTitle>
              <AlertDescription>
                {t("usage.blocked.description")}
              </AlertDescription>
            </Alert>
          )}

          {!blockMode && (
            <Alert data-testid="usage-overage">
              <AlertTitle>{t("usage.overage.title")}</AlertTitle>
              <AlertDescription>
                {t("usage.overage.description", {
                  count: usage.overageSends,
                  formattedCount: usage.overageSends.toLocaleString(),
                })}
              </AlertDescription>
            </Alert>
          )}
        </CardContent>
      </Card>

      <Alert data-testid="usage-refresh-note">
        <InfoIcon />
        <AlertTitle>{t("usage.refreshNote.title")}</AlertTitle>
        <AlertDescription>
          {t("usage.refreshNote.description")}
        </AlertDescription>
      </Alert>

      <p className="text-sm text-muted-foreground">
        {t("usage.closedPeriodsPrefix")}
        <Link
          to="/t/$slug/billing/invoices"
          params={{ slug }}
          className="text-primary hover:underline"
        >
          {t("usage.closedPeriodsLink")}
        </Link>
        {t("usage.closedPeriodsSuffix")}
      </p>
    </div>
  )
}
